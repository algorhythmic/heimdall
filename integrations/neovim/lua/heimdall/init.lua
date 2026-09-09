local M = {}
local api, uv = vim.api, vim.uv
local client, view = require('heimdall.client'), require('heimdall.view')
local config, selected, generation, resume_request = nil, nil, 0, 0
local drafts, busy, resume_buffer = {}, {}, nil

local function tell(text, level)
  vim.notify(view.text(text), level or vim.log.levels.INFO, { title = 'Heimdall' })
end
local function valid_target(target)
  return type(target) == 'string'
    and #target <= 257
    and target:match('^[a-z0-9][a-z0-9_#%-]*$')
    and not target:find('##', 1, true)
end
local function current(target, epoch)
  return selected == target and generation == epoch
end
local function selection()
  if not config then
    tell('Call require("heimdall").setup() first.', vim.log.levels.ERROR)
    return
  end
  if not selected then
    tell('Select a task with :HeimdallSelect first.', vim.log.levels.WARN)
    return
  end
  return selected, generation
end
local function request(args, done)
  return client.request(config, args, function(err, result)
    if err then
      tell(err, vim.log.levels.ERROR)
      if done then
        done(err)
      end
      return
    end
    if done then
      done(nil, result)
    end
  end)
end
local function split_buffer(buf)
  api.nvim_cmd({ cmd = 'vsplit', mods = { noautocmd = true } }, {})
  api.nvim_win_set_buf(0, buf)
end
local function show(name, lines)
  local buf = api.nvim_create_buf(false, true)
  api.nvim_buf_set_name(buf, 'heimdall://' .. name .. '/' .. buf)
  vim.bo[buf].bufhidden = 'wipe'
  vim.bo[buf].swapfile, vim.bo[buf].undofile, vim.bo[buf].modeline = false, false, false
  api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
  local reused = false
  if resume_buffer and api.nvim_buf_is_valid(resume_buffer) then
    for _, win in ipairs(api.nvim_list_wins()) do
      if api.nvim_win_get_buf(win) == resume_buffer then
        api.nvim_win_set_buf(win, buf)
        reused = true
        break
      end
    end
  end
  if not reused then
    split_buffer(buf)
  end
  resume_buffer = buf
  return buf
end

local function resume_data(target, done)
  request({ 'resume', target }, function(err, v)
    if err then
      return done(err)
    end
    if
      v.version ~= 1
      or v.target ~= target
      or type(v.task) ~= 'table'
      or type(v.task.task) ~= 'table'
      or type(v.contracts) ~= 'table'
      or type(v.resources) ~= 'table'
      or type(v.issues) ~= 'table'
    then
      tell('Unsupported Heimdall resume response.', vim.log.levels.ERROR)
      return done('invalid response')
    end
    done(nil, v)
  end)
end

function M.resume()
  local target, epoch = selection()
  if not target then
    return
  end
  resume_request = resume_request + 1
  local sequence = resume_request
  resume_data(target, function(err, v)
    if not current(target, epoch) or sequence ~= resume_request then
      return
    end
    if err then
      show(
        'resume/' .. target,
        { 'Task: ' .. target, 'Current context unavailable. Run :HeimdallResume to retry.' }
      )
      return
    end
    local ok, lines = pcall(view.resume, v)
    if not ok then
      tell('Unsupported Heimdall resume fields.', vim.log.levels.ERROR)
      return
    end
    show('resume/' .. target, lines)
  end)
end

function M.select(target)
  if not config then
    tell('Call setup() first.', vim.log.levels.ERROR)
    return
  end
  generation = generation + 1
  local epoch = generation
  request({ 'ls' }, function(err, document)
    if err or epoch ~= generation then
      return
    end
    local choices = {}
    for _, task in ipairs(document.tasks or {}) do
      choices[#choices + 1] = { target = task.id, title = task.title }
      for _, step in ipairs(task.subtasks or {}) do
        choices[#choices + 1] =
          { target = task.id .. '#' .. step.id, title = task.title .. ' / ' .. step.title }
      end
    end
    local function choose(item)
      if not item or epoch ~= generation or not valid_target(item.target) then
        return
      end
      selected = item.target
      tell('Selected ' .. selected)
      M.resume()
    end
    if target and target ~= '' then
      for _, item in ipairs(choices) do
        if item.target == target then
          choose(item)
          return
        end
      end
      tell('Task or step not found: ' .. target, vim.log.levels.ERROR)
    else
      vim.ui.select(choices, {
        prompt = 'Heimdall task',
        format_item = function(item)
          return view.text(item.title) .. ' (' .. item.target .. ')'
        end,
      }, choose)
    end
  end)
end

local function read_draft(path)
  local stat = uv.fs_lstat(path)
  if not stat or stat.type ~= 'file' or stat.size > 65536 then
    return nil, 'Draft must be a regular file of at most 64 KiB.'
  end
  local file = uv.fs_open(path, 'r', 0)
  if not file then
    return nil, 'Draft cannot be read.'
  end
  local content = uv.fs_read(file, 65537, 0)
  uv.fs_close(file)
  local ok, value = pcall(vim.json.decode, content or '')
  if
    not ok
    or type(value) ~= 'table'
    or (value.version ~= 1 and value.version ~= 2)
    or value.op ~= 'checkpoint.record'
    or not valid_target(value.target)
    or type(value.id) ~= 'string'
    or not value.id:match('^[a-f0-9]+$')
    or #value.id ~= 32
    or type(value.checkpoint) ~= 'table'
  then
    return nil, 'Not a Heimdall checkpoint draft.'
  end
  return value
end
local function identity(draft)
  return {
    draft.version,
    draft.id,
    draft.op,
    draft.target,
    draft.expected_task_revision,
    draft.checkpoint.previous,
    draft.checkpoint.contract_id,
    draft.checkpoint.artifacts,
  }
end
local function open_draft(path, target)
  local draft, err = read_draft(path)
  if err or draft.target ~= target then
    tell(err or 'Draft belongs to another target.', vim.log.levels.ERROR)
    return
  end
  local buf = vim.fn.bufadd(path)
  vim.bo[buf].modeline, vim.bo[buf].swapfile, vim.bo[buf].undofile = false, false, false
  vim.fn.bufload(buf)
  drafts[buf] = { path = path, target = target, original = identity(draft) }
  split_buffer(buf)
  vim.bo[buf].filetype = 'json'
  tell(
    'Edit summary and next action, :write, then :HeimdallCheckpointSubmit. Keep the request identity and preconditions.'
  )
end

function M.checkpoint_open(path)
  local target = selection()
  if not target then
    return
  end
  open_draft(vim.fn.fnamemodify(path, ':p'), target)
end

function M.checkpoint_draft()
  local target, epoch = selection()
  if not target then
    return
  end
  vim.fn.mkdir(config.draft_dir, 'p', 448)
  local path = config.draft_dir .. '/' .. target:gsub('#', '--') .. '-' .. uv.hrtime() .. '.json'
  request({ 'checkpoint', 'draft', target, '--output', path }, function(err, value)
    if err then
      return
    end
    if value.target ~= target or value.draft_file ~= path then
      tell('Unexpected draft response.', vim.log.levels.ERROR)
      return
    end
    if current(target, epoch) then
      open_draft(path, target)
    else
      tell('Draft retained for ' .. target .. ': ' .. path)
    end
  end)
end

function M.checkpoint_submit()
  local target = selection()
  if not target then
    return
  end
  local buf = api.nvim_get_current_buf()
  local draft = drafts[buf]
  if not draft then
    tell(
      'Open a draft with :HeimdallCheckpointDraft or :HeimdallCheckpointOpen.',
      vim.log.levels.WARN
    )
    return
  end
  if draft.target ~= target then
    tell("Select the draft's original target before submitting.", vim.log.levels.ERROR)
    return
  end
  if api.nvim_buf_get_name(buf) ~= draft.path then
    tell(
      'Draft buffer was renamed. Open the intended draft explicitly before submitting.',
      vim.log.levels.ERROR
    )
    return
  end
  if vim.bo[buf].modified then
    tell('Write this draft before submitting; unsaved edits are retained.', vim.log.levels.WARN)
    return
  end
  if busy[draft.path] then
    tell('This draft is already being submitted.')
    return
  end
  local value, err = read_draft(draft.path)
  if err or not vim.deep_equal(identity(value), draft.original) then
    tell(
      err or 'Draft identity or preconditions changed. Open a new draft after reviewing context.',
      vim.log.levels.ERROR
    )
    return
  end
  local decoded, visible =
    pcall(vim.json.decode, table.concat(api.nvim_buf_get_lines(buf, 0, -1, false), '\n'))
  if not decoded or not vim.deep_equal(visible, value) then
    tell(
      'Draft changed on disk. Reload and review it before submitting; buffer edits are retained.',
      vim.log.levels.ERROR
    )
    return
  end
  busy[draft.path] = true
  request(
    { 'checkpoint', 'submit', target, '--file', draft.path, '--request-id', value.id },
    function(problem)
      busy[draft.path] = nil
      if not problem then
        tell('Checkpoint saved for ' .. target .. '. Draft retained for an exact retry.')
      end
    end
  )
end

-- Open only an explicit bound file, or an explicit relative file inside a bound
-- tree. Never interpret task text as Ex, a shell command, a URL or a recipe.
local function artifact_path(resource, relative)
  if type(resource.root) ~= 'string' or resource.root:sub(1, 1) ~= '/' then
    return nil, 'Only local POSIX resources are supported.'
  end
  local root = uv.fs_realpath(resource.root)
  if not root or root ~= resource.root then
    return nil, 'Resource root changed or is unavailable.'
  end
  local base = vim.fs.normalize(root .. '/' .. resource.path)
  local path = resource.kind == 'tree' and (base .. '/' .. (relative or '')) or base
  if path:find('%z') or (relative and (relative:sub(1, 1) == '/' or relative:match('^%a+:'))) then
    return nil, 'Choose a relative file inside this resource.'
  end
  local canonical = uv.fs_realpath(path)
  local prefix = base == '/' and '/' or base .. '/'
  if not canonical or canonical ~= vim.fs.normalize(path) then
    return nil, 'Missing file or symlink resource refused.'
  end
  if resource.kind == 'tree' and canonical:sub(1, #prefix) ~= prefix then
    return nil, 'File is outside the selected resource.'
  end
  if resource.kind == 'tree' then
    local excluded = { ['.git'] = true }
    for _, name in ipairs(resource.exclude or {}) do
      excluded[name] = true
    end
    for segment in canonical:sub(#prefix + 1):gmatch('[^/]+') do
      if excluded[segment] then
        return nil, 'File is excluded from this resource.'
      end
    end
  end
  local root_prefix = root == '/' and '/' or root .. '/'
  if canonical ~= root and canonical:sub(1, #root_prefix) ~= root_prefix then
    return nil, 'File is outside the resource root.'
  end
  local stat = uv.fs_stat(canonical)
  if not stat or stat.type ~= 'file' then
    return nil, 'Choose a regular file, not a directory or device.'
  end
  return canonical
end

function M.artifact(resource_id, relative)
  local target, epoch = selection()
  if not target then
    return
  end
  resume_data(target, function(err, data)
    if err or not current(target, epoch) then
      return
    end
    local choices = {}
    for _, r in ipairs(data.resources) do
      if r.resource.active and (r.resource.kind == 'file' or r.resource.kind == 'tree') then
        choices[#choices + 1] = r.resource
      end
    end
    local function choose(resource)
      if not resource or not current(target, epoch) then
        return
      end
      local function open(path)
        if path == nil or not current(target, epoch) then
          return
        end
        local resolved, problem = artifact_path(resource, path)
        if not resolved then
          tell(problem, vim.log.levels.ERROR)
          return
        end
        local buf = vim.fn.bufadd(resolved)
        vim.bo[buf].modeline = false
        vim.fn.bufload(buf)
        split_buffer(buf)
      end
      if resource.kind == 'tree' and relative == nil then
        vim.ui.input({
          prompt = 'File relative to ' .. view.text(resource.root .. '/' .. resource.path) .. ': ',
        }, open)
      else
        open(relative or '')
      end
    end
    if resource_id then
      for _, r in ipairs(choices) do
        if r.id == resource_id then
          choose(r)
          return
        end
      end
      tell('Active resource not found in the selected task context.', vim.log.levels.ERROR)
    else
      vim.ui.select(choices, {
        prompt = 'Open bound artifact',
        format_item = function(r)
          return view.text(r.root .. '/' .. r.path)
        end,
      }, choose)
    end
  end)
end

function M.session_check(surface)
  local target, epoch = selection()
  if not target then
    return
  end
  if not surface or #surface ~= 32 or not surface:match('^[a-f0-9]+$') then
    tell('An explicit 32-character surface ID is required.', vim.log.levels.ERROR)
    return
  end
  request({ 'session', 'refresh', target, '--surface', surface }, function(err, result)
    if err or not current(target, epoch) then
      return
    end
    show('session/' .. target, {
      'Task: ' .. target,
      'Surface: ' .. surface,
      'Session: ' .. view.text(result.status),
      'Checked: ' .. view.text(result.checked_at),
      'Issues: ' .. view.text(table.concat(result.issues or {}, ', ')),
      'This check does not rebind or publish metadata.',
    })
  end)
end

function M.review()
  local target, epoch = selection()
  if not target then
    return
  end
  if vim.fn.has('clipboard') ~= 1 then
    tell(
      'A clipboard provider is required for review. Run heimdall ui ROOT_TASK for a terminal handoff.',
      vim.log.levels.ERROR
    )
    return
  end
  resume_data(target, function(err, data)
    if err or not current(target, epoch) then
      return
    end
    local root = data.task.task.id
    for _, ancestor in ipairs(data.ancestors or {}) do
      if not ancestor.task.parent then
        root = ancestor.task.id
      end
    end
    request({ 'ui', root }, function(problem, result)
      if problem or not current(target, epoch) then
        return
      end
      if
        type(result.url) ~= 'string'
        or not result.url:match('^http://127%.0%.0%.1:%d+/ui/?$')
        or type(result.code) ~= 'string'
        or not result.code:match('^[A-Za-z0-9%-]+$')
      then
        tell('Unexpected review handoff response.', vim.log.levels.ERROR)
        return
      end
      -- The explicit review action transfers only a five-minute, single-use GUI
      -- code through the clipboard. No command-line echo: UI plugins can retain
      -- even non-history echoes in editor buffers.
      local copied, copy_result = pcall(vim.fn.setreg, '+', result.code, 'v')
      if not copied or copy_result ~= 0 then
        tell(
          'Could not copy the review code. Run heimdall ui for a terminal handoff.',
          vim.log.levels.ERROR
        )
        return
      end
      local _, open_error = vim.ui.open(result.url)
      if open_error then
        tell('Browser could not open. Run heimdall ui for a fresh handoff.', vim.log.levels.ERROR)
        return
      end
      tell(
        'Review for '
          .. root
          .. ': sign-in code copied. Paste it in the browser; it expires in five minutes.'
      )
    end)
  end)
end

function M.selected()
  return selected
end

function M.setup(opts)
  opts = opts or {}
  assert(vim.fn.has('nvim-0.11') == 1, 'Heimdall requires Neovim 0.11+')
  assert(
    type(opts.data_dir) == 'string' and opts.data_dir ~= '',
    'Heimdall requires an explicit data_dir'
  )
  config = {
    executable = opts.executable or 'heimdall',
    data_dir = vim.fs.normalize(vim.fn.fnamemodify(opts.data_dir, ':p')),
    draft_dir = vim.fs.normalize(
      vim.fn.fnamemodify(opts.draft_dir or (vim.fn.stdpath('state') .. '/heimdall/drafts'), ':p')
    ),
    timeout_ms = opts.timeout_ms or 10000,
  }
  assert(
    type(config.executable) == 'string' and config.executable ~= '',
    'executable must be one program path'
  )
  assert(
    type(config.timeout_ms) == 'number' and config.timeout_ms >= 100 and config.timeout_ms <= 60000,
    'timeout_ms must be 100–60000'
  )
  generation, selected, drafts = generation + 1, nil, {}
  local commands = {
    HeimdallSelect = {
      function(c)
        M.select(c.args)
      end,
      { nargs = '?' },
    },
    HeimdallResume = { M.resume, {} },
    HeimdallCheckpointDraft = { M.checkpoint_draft, {} },
    HeimdallCheckpointOpen = {
      function(c)
        M.checkpoint_open(c.fargs[1])
      end,
      { nargs = 1, complete = 'file' },
    },
    HeimdallCheckpointSubmit = { M.checkpoint_submit, {} },
    HeimdallArtifact = {
      function()
        M.artifact()
      end,
      {},
    },
    HeimdallSessionCheck = {
      function(c)
        M.session_check(c.args)
      end,
      { nargs = 1 },
    },
    HeimdallReview = { M.review, {} },
  }
  for name, command in pairs(commands) do
    api.nvim_create_user_command(name, command[1], command[2])
  end
end

return M
