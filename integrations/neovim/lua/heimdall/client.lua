local M = {}

-- Only argv arrays reach the OS. Bound both output streams before JSON decoding;
-- never surface raw command output (the UI bootstrap response contains a code).
function M.request(config, args, done)
  local argv = { config.executable }
  vim.list_extend(argv, args)
  vim.list_extend(argv, { '--json', '--data-dir', config.data_dir })
  local chunks, errors, size, failed, process = {}, {}, 0, false, nil
  local function collect(destination)
    return function(err, data)
      if err then
        failed = true
      end
      if not data or failed then
        return
      end
      size = size + #data
      if size > 1024 * 1024 then
        failed = true
        if process then
          process:kill(9)
        end
        return
      end
      destination[#destination + 1] = data
    end
  end
  local ok, result = pcall(vim.system, argv, {
    text = true,
    timeout = config.timeout_ms,
    stdout = collect(chunks),
    stderr = collect(errors),
  }, function(exit)
    vim.schedule(function()
      if failed then
        return done('Heimdall response exceeded its limit or could not be read.')
      end
      if exit.code ~= 0 then
        local detail = table.concat(errors)
        if detail:find('409', 1, true) then
          return done(
            'Heimdall conflict: task, contract or checkpoint changed. Draft retained; review context and create a new draft.'
          )
        end
        if exit.code == 124 then
          return done(
            'Heimdall timed out. Draft retained; retry the unchanged file if submission was uncertain.'
          )
        end
        return done(
          'Heimdall command failed. Check the daemon, target and input with the CLI. Any draft is retained.'
        )
      end
      local decoded, value =
        pcall(vim.json.decode, table.concat(chunks), { luanil = { object = true, array = true } })
      if not decoded or type(value) ~= 'table' then
        return done('Heimdall returned invalid JSON.')
      end
      done(nil, value)
    end)
  end)
  if not ok then
    vim.schedule(function()
      done('Heimdall executable unavailable. Check setup().executable.')
    end)
    return
  end
  process = result
  return process
end

return M
