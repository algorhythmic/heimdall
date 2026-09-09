local M = {}

function M.text(value)
  local s = type(value) == 'string' and value or tostring(value or '')
  s = s:gsub('[%z\1-\31\127]', function(c)
    return string.format('\\x%02x', c:byte())
  end)
  -- Escape Unicode formatting controls that can reorder or hide task text.
  for _, range in ipairs({
    { 0x200b, 0x200f },
    { 0x2028, 0x202e },
    { 0x2060, 0x206f },
    {
      0xfeff,
      0xfeff,
    },
  }) do
    for cp = range[1], range[2] do
      s = s:gsub(vim.fn.nr2char(cp), string.format('\\u%04x', cp))
    end
  end
  local bounded = vim.fn.strcharpart(s, 0, 8192)
  return #bounded < #s and (bounded .. ' [text truncated]') or bounded
end

function M.resume(v)
  local task = v.task.task
  local lines = {}
  local function add(label, value)
    lines[#lines + 1] = label .. M.text(value)
  end
  add('Task: ', task.title .. ' (' .. v.target .. ')')
  local status = task.status
  for _, step in ipairs(task.subtasks or {}) do
    if v.target == task.id .. '#' .. step.id then
      add('Step: ', step.title)
      status = step.status
    end
  end
  add('Status: ', status .. ' | Resume: ' .. v.resume_status)
  lines[#lines + 1] = ''
  lines[#lines + 1] = 'Accepted direction'
  if #v.contracts == 0 then
    add('  ', 'No accepted contract')
  end
  for _, c in ipairs(v.contracts) do
    add('  ', c.target .. ': ' .. c.objective)
    if c.acceptance and c.acceptance.text ~= '' then
      add('  Acceptance: ', c.acceptance.text)
    end
    for _, check in ipairs((c.acceptance or {}).checks or {}) do
      add('  Check: ', check.id .. ' (' .. check.kind .. ')')
    end
    for _, x in ipairs(c.constraints or {}) do
      add('  Constraint: ', x)
    end
  end
  for _, d in ipairs(v.decisions or {}) do
    add('  Decision: ', d.text)
  end
  if #(v.progress or {}) > 0 then
    lines[#lines + 1] = ''
    lines[#lines + 1] = 'Planning review'
    for _, p in ipairs(v.progress) do
      add('  ', p.kind .. ' | ' .. p.status .. ' | ' .. p.freshness .. ' | ' .. p.target)
      add('    ', p.text)
      add('    Proposal: ', p.id)
    end
    add('  ', ':HeimdallProgress inspects a proposal; :HeimdallProgressReview opens GUI review.')
  end
  lines[#lines + 1] = ''
  lines[#lines + 1] = 'Saved progress'
  if v.checkpoint then
    add('  Checkpoint: ', v.checkpoint.at)
    add('  Summary: ', v.checkpoint.summary)
    if v.checkpoint.current_step then
      add('  Current step: ', v.checkpoint.current_step)
    end
    add('  Next action (checkpoint): ', v.checkpoint.next_action)
    for _, b in ipairs(v.checkpoint.blockers or {}) do
      add('  Blocker: ', b)
    end
  else
    add('  ', 'No saved checkpoint')
    add('  Next action (task): ', task.next_action)
  end
  lines[#lines + 1] = ''
  lines[#lines + 1] = 'Resources'
  for _, r in ipairs(v.resources) do
    add('  ', r.resource.root .. '/' .. r.resource.path .. ' | ' .. r.status)
    if r.detail then
      add('    ', r.detail)
    end
  end
  lines[#lines + 1] = ''
  if #(v.artifacts or {}) > 0 then
    lines[#lines + 1] = 'Pinned artifacts'
    for _, a in ipairs(v.artifacts) do
      add('  ', a.artifact.name .. ' | ' .. a.status)
      add('    Version: ', a.record.id)
      add('    SHA-256: ', a.record.observation.digest)
    end
    lines[#lines + 1] = ''
  end
  lines[#lines + 1] = 'Needs attention'
  if #v.issues == 0 then
    add('  ', 'No context drift reported by this check')
  end
  for _, issue in ipairs(v.issues) do
    add('  ', issue.detail)
  end
  lines[#lines + 1] = ''
  local review = v.reviews or {}
  add(
    'Recorded review: ',
    string.format(
      '%d proposals; evidence %d failed, %d stale, %d unknown',
      review.pending_proposals or 0,
      review.failed_evidence or 0,
      review.stale_evidence or 0,
      review.unknown_evidence or 0
    )
  )
  add('Context checked: ', os.date('!%Y-%m-%dT%H:%M:%SZ'))
  add(
    '',
    'Saved progress and recorded evidence are not completion approval. Run :HeimdallResume to check again.'
  )
  return lines
end

function M.progress(v)
  local p = v.proposal
  local lines = {
    'Task: ' .. M.text(p.target),
    'Proposal: ' .. M.text(p.id),
    'Kind: ' .. M.text(p.kind),
    'Review: ' .. M.text(v.status) .. ' | Freshness: ' .. M.text(v.freshness),
    '',
    M.text(p.text),
    '',
    'Contract: ' .. M.text(p.contract_id),
    'Proposal digest: ' .. M.text(p.digest),
  }
  for _, a in ipairs(p.artifacts or {}) do
    lines[#lines + 1] = 'Artifact: ' .. M.text(a.artifact_id)
    lines[#lines + 1] = '  Version: ' .. M.text(a.version_id)
    lines[#lines + 1] = '  SHA-256: ' .. M.text(a.observation.digest)
  end
  for _, a in ipairs(v.artifacts or {}) do
    lines[#lines + 1] = 'Checked: ' .. M.text(a.artifact.name .. ' | ' .. a.status)
  end
  if v.review then
    lines[#lines + 1] = 'Last review: ' .. M.text(v.review.status .. ' | ' .. v.review.actor)
    lines[#lines + 1] = '  ' .. M.text(v.review.note)
  end
  lines[#lines + 1] = ''
  lines[#lines + 1] = ':HeimdallProgressReview opens the scoped GUI review controls.'
  lines[#lines + 1] = 'This inspection does not accept a proposal or complete work.'
  return lines
end

return M
