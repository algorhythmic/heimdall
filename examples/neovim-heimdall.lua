-- Example LazyVim/lazy.nvim plugin spec. Change these explicit local paths.
-- Save as lua/plugins/heimdall.lua in your chosen editor configuration.
return {
  dir = '/path/to/heimdall/integrations/neovim',
  name = 'heimdall',
  cmd = {
    'HeimdallSelect',
    'HeimdallResume',
    'HeimdallCheckpointDraft',
    'HeimdallCheckpointOpen',
    'HeimdallCheckpointSubmit',
    'HeimdallArtifact',
    'HeimdallSessionCheck',
    'HeimdallReview',
    'HeimdallTUI',
    'HeimdallProgress',
    'HeimdallProgressReview',
  },
  opts = {
    executable = '/path/to/heimdall/bin/heimdall',
    data_dir = '/path/to/heimdall-data',
    -- Optional: keep drafts outside observed project/resource trees.
    -- draft_dir = '/private/path/to/checkpoint-drafts',
  },
}
