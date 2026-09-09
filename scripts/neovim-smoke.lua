local api, uv = vim.api, vim.uv
local function read(path)
  return table.concat(vim.fn.readfile(path), '\n')
end
local fixture = vim.json.decode(read(vim.env.HEIMDALL_NVIM_FIXTURE))
if fixture.mode == 'lazy' then
  vim.go.loadplugins = true
  vim.opt.runtimepath:prepend(fixture.lazy_path)
  local spec = dofile(fixture.root .. '/examples/neovim-heimdall.lua')
  spec.dir = fixture.root .. '/integrations/neovim'
  spec.opts = { executable = fixture.exe, data_dir = fixture.data, draft_dir = fixture.drafts }
  require('lazy').setup({ spec }, {
    local_spec = false,
    install = { missing = false },
    checker = { enabled = false },
    change_detection = { enabled = false },
    pkg = { enabled = false },
    rocks = { enabled = false },
    readme = { enabled = false },
  })
  api.nvim_cmd({ cmd = 'HeimdallSelect', args = { 'alpha' } }, {})
  assert(
    require('lazy.core.config').plugins.heimdall._.loaded,
    'example command did not lazy-load the plugin'
  )
else
  vim.opt.runtimepath:prepend(fixture.root .. '/integrations/neovim')
end
local h, client = require('heimdall'), require('heimdall.client')
local messages, terminal = {}, nil
vim.notify = function(message)
  messages[#messages + 1] = message
end
local real_jobstart = vim.fn.jobstart
vim.fn.jobstart = function(argv, options)
  if options and options.term then
    terminal = { argv = argv, options = options }
    return 1
  end
  return real_jobstart(argv, options)
end
local function wait(predicate)
  assert(vim.wait(15000, predicate, 10), 'timed out: ' .. table.concat(messages, '\n'))
end
local function buffers()
  local lines = {}
  for _, b in ipairs(api.nvim_list_bufs()) do
    if api.nvim_buf_is_loaded(b) then
      vim.list_extend(lines, api.nvim_buf_get_lines(b, 0, -1, false))
    end
  end
  return table.concat(lines, '\n')
end
local function current_text()
  return table.concat(api.nvim_buf_get_lines(0, 0, -1, false), '\n')
end
local function cli(args)
  local argv = { fixture.exe }
  vim.list_extend(argv, args)
  vim.list_extend(argv, { '--data-dir', fixture.data })
  local result = vim.system(argv, { text = true }):wait(10000)
  assert(result.code == 0, result.stderr)
  return vim.json.decode(result.stdout)
end
local function choose(target)
  local before = #messages
  h.select(target)
  wait(function()
    return h.selected() == target and #messages > before
  end)
  wait(function()
    return buffers():find('(' .. target .. ')', 1, true) ~= nil
  end)
end
local function draft(summary)
  local old = api.nvim_get_current_buf()
  h.checkpoint_draft()
  wait(function()
    return api.nvim_get_current_buf() ~= old
      and api.nvim_buf_get_name(0):sub(1, #fixture.drafts) == fixture.drafts
  end)
  local buf = api.nvim_get_current_buf()
  local path = api.nvim_buf_get_name(buf)
  local value = vim.json.decode(current_text())
  value.checkpoint.summary = summary
  value.checkpoint.next_action = 'Review saved alpha notes'
  api.nvim_buf_set_lines(buf, 0, -1, false, { vim.json.encode(value) })
  api.nvim_cmd({ cmd = 'write' }, {})
  return buf, path
end
local function expect_message(action, pattern)
  local before = #messages
  action()
  wait(function()
    for i = before + 1, #messages do
      if messages[i]:find(pattern) then
        return true
      end
    end
    return false
  end)
end
local function submit(buf, pattern)
  api.nvim_set_current_buf(buf)
  expect_message(h.checkpoint_submit, pattern)
end
local function run()
  if fixture.mode ~= 'lazy' then
    h.setup({ executable = fixture.exe, data_dir = fixture.data, draft_dir = fixture.drafts })
  end
  if fixture.mode == 'lazy' then
    wait(function()
      return h.selected() == 'alpha' and buffers():find('(alpha)', 1, true)
    end)
    assert(
      vim.fn.exists(':HeimdallCheckpointSubmit') == 2 and vim.fn.exists(':HeimdallReview') == 2
    )
    return
  end
  if fixture.mode == 'retry' then
    choose('alpha')
    h.checkpoint_open(fixture.winning)
    submit(api.nvim_get_current_buf(), 'Checkpoint saved')
    assert(#cli({ 'checkpoint', 'list', 'alpha' }) == 1)
    assert(read(fixture.winning) == fixture.bytes)
    return
  end
  local before = cli({ 'events' })
  choose('alpha')
  assert(buffers():find('\\u202e', 1, true))
  vim.ui.select = function(items, _, done)
    for _, item in ipairs(items) do
      if item.target == 'beta' then
        done(item)
        return
      end
    end
    error('beta not in picker')
  end
  h.select()
  wait(function()
    return h.selected() == 'beta'
  end)
  wait(function()
    return buffers():find('Beta planning', 1, true)
  end)
  assert(vim.deep_equal(before, cli({ 'events' })), 'selection/resume wrote events')
  assert(buffers():find('Pinned artifacts', 1, true) and buffers():find('Beta artifact', 1, true))
  assert(
    buffers():find('Planning review', 1, true) and buffers():find('beta planning review', 1, true)
  )
  h.progress(fixture.proposals.beta)
  wait(function()
    return current_text():find('Proposal digest:', 1, true) ~= nil
  end)
  assert(
    current_text():find(fixture.proposals.beta, 1, true) and current_text():find('\\u202e', 1, true)
  )
  assert(vim.deep_equal(before, cli({ 'events' })), 'progress inspection wrote events')
  expect_message(function()
    h.progress('invalid')
  end, 'proposal ID is required')
  -- Pending inspection must not replace the view after a task switch.
  local original_request, delayed = client.request, nil
  client.request = function(config, args, done)
    if args[1] == 'progress' then
      delayed = done
      return
    end
    return original_request(config, args, done)
  end
  h.progress(fixture.proposals.beta)
  choose('alpha')
  delayed(nil, { proposal = { id = fixture.proposals.beta, target = 'beta' } })
  client.request = original_request
  assert(not current_text():find('Proposal digest:', 1, true))
  choose('beta')
  local beta_buf, beta_path = draft('Reviewed pinned beta artifact')
  local beta_original = read(beta_path)
  local beta_draft = vim.json.decode(beta_original)
  assert(beta_draft.version == 2 and #beta_draft.checkpoint.artifacts == 1)
  beta_draft.checkpoint.artifacts[1].version_id = string.rep('f', 32)
  api.nvim_buf_set_lines(beta_buf, 0, -1, false, { vim.json.encode(beta_draft) })
  api.nvim_cmd({ cmd = 'write' }, {})
  submit(beta_buf, 'identity or preconditions changed')
  api.nvim_buf_set_lines(beta_buf, 0, -1, false, { beta_original })
  api.nvim_cmd({ cmd = 'write' }, {})
  submit(beta_buf, 'Checkpoint saved')
  choose('alpha')
  -- Hold a completed alpha response, switch task, then deliver it late.
  local real_request, late = client.request, nil
  client.request = function(config, args, done)
    return real_request(config, args, function(err, value)
      late = function()
        done(err, value)
      end
    end)
  end
  h.resume()
  wait(function()
    return late ~= nil
  end)
  client.request = real_request
  choose('beta')
  late()
  assert(h.selected() == 'beta')
  local active_resume
  for _, win in ipairs(api.nvim_list_wins()) do
    local buf = api.nvim_win_get_buf(win)
    if api.nvim_buf_get_name(buf):find('heimdall://resume/', 1, true) then
      active_resume = table.concat(api.nvim_buf_get_lines(buf, 0, -1, false), '\n')
    end
  end
  assert(active_resume:find('Beta planning', 1, true) and not active_resume:find('Alpha', 1, true))
  choose('alpha')
  local old = api.nvim_get_current_buf()
  h.artifact(fixture.resource, fixture.filename)
  wait(function()
    return api.nvim_buf_get_name(0) == fixture.work .. '/' .. fixture.filename
  end)
  assert(vim.g.heimdall_injected == nil and vim.bo.modeline == false)
  expect_message(function()
    h.artifact(fixture.resource, '../outside.md')
  end, 'outside the selected resource')
  expect_message(function()
    h.artifact(fixture.resource, '.git/config')
  end, 'excluded from this resource')
  uv.fs_symlink(fixture.work .. '/' .. fixture.filename, fixture.work .. '/alias.md')
  expect_message(function()
    h.artifact(fixture.resource, 'alias.md')
  end, 'symlink resource refused')
  uv.fs_unlink(fixture.work .. '/alias.md')
  h.session_check(fixture.surface)
  wait(function()
    return buffers():find('Session: disconnected', 1, true)
  end)
  local losing, losing_path = draft('Notes retained after a conflict')
  local winning, winning_path = draft('Reviewed planning artifact')
  submit(winning, 'Checkpoint saved')
  api.nvim_buf_set_name(winning, winning_path .. '.renamed')
  submit(winning, 'buffer was renamed')
  api.nvim_buf_set_name(winning, winning_path)
  local original = read(winning_path)
  local changed = vim.json.decode(original)
  changed.checkpoint.summary = 'External edit'
  vim.fn.writefile({ vim.json.encode(changed) }, winning_path)
  submit(winning, 'changed on disk')
  changed.expected_task_revision = 999
  vim.fn.writefile({ vim.json.encode(changed) }, winning_path)
  submit(winning, 'identity or preconditions changed')
  vim.fn.writefile({ original }, winning_path)
  local bytes = read(losing_path)
  submit(losing, 'Heimdall conflict')
  assert(read(losing_path) == bytes and current_text() == bytes)
  choose('beta')
  submit(losing, 'original target')
  choose('alpha')
  api.nvim_set_current_buf(winning)
  api.nvim_buf_set_lines(winning, 0, 0, false, { ' ' })
  expect_message(h.checkpoint_submit, 'Write this draft')
  api.nvim_buf_set_lines(
    winning,
    0,
    -1,
    false,
    vim.split(read(winning_path), '\n', { plain = true })
  )
  vim.bo[winning].modified = false
  -- Review opens the selected target in an argv-only terminal buffer.
  choose('child-task')
  h.review()
  assert(terminal and terminal.options.term == true)
  assert(vim.deep_equal(terminal.argv, {
    fixture.exe,
    'tui',
    'child-task',
    '--data-dir',
    fixture.data,
  }))
  terminal = nil
  h.progress_review()
  assert(terminal and terminal.argv[2] == 'tui')
  assert(vim.fn.exists(':HeimdallTUI') == 2)
  assert(cli({ 'state', 'alpha' }).task.status == 'active')
  choose('alpha')
  api.nvim_set_current_buf(winning)
  local saved = read(winning_path)
  vim.fn.writefile({ vim.json.encode({ winning = winning_path, bytes = saved }) }, fixture.result)
  uv.kill(fixture.daemon_pid, 9)
  expect_message(h.checkpoint_submit, 'command failed')
  assert(read(winning_path) == saved)
  h.resume()
  wait(function()
    return buffers():find('Current context unavailable', 1, true)
  end)
  -- Exercise the actual subprocess wrapper, not a mock of its output limits.
  local probe = fixture.drafts .. '/probe.py'
  vim.fn.writefile({
    '#!/usr/bin/env python3',
    'import sys, time',
    'if sys.argv[1] == "large": print("x" * 1100000)',
    'elif sys.argv[1] == "slow": time.sleep(5)',
    'else: print("not json")',
  }, probe)
  uv.fs_chmod(probe, 448)
  for _, case in ipairs({
    { 'large', 'limit' },
    { 'slow', 'timed out' },
    { 'json', 'invalid JSON' },
  }) do
    local complete = false
    client.request(
      { executable = probe, data_dir = fixture.data, timeout_ms = 200 },
      { case[1] },
      function(err)
        assert(err and err:find(case[2]), err)
        complete = true
      end
    )
    wait(function()
      return complete
    end)
  end
end
local ok, err = xpcall(run, debug.traceback)
if not ok then
  io.stderr:write(err .. '\n')
  vim.cmd('cquit 1')
end
print('HEIMDALL_NVIM_PASSED')
vim.cmd('qa!')
