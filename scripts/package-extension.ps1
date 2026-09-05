$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$manifest = Get-Content -LiteralPath (Join-Path $projectRoot 'extension/manifest.json') -Raw | ConvertFrom-Json
$output = Join-Path $projectRoot ('bin/heimdall-extension-' + $manifest.version + '.zip')
[void][System.IO.Directory]::CreateDirectory((Split-Path -Parent $output))
$files = @('manifest.json','extension-id.txt','worker.js','controller.js','outbox.js','popup.html','popup.js','popup.css','launch.html') | ForEach-Object { Join-Path $projectRoot ('extension/' + $_) }
Compress-Archive -LiteralPath $files -DestinationPath $output -Force
Write-Output $output
