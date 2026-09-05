$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$taskGo = $env:HEIMDALL_GO
if (-not $taskGo) {
    $localGo = Join-Path $projectRoot '.tools/go/bin/go.exe'
    $onPath = Get-Command go -ErrorAction SilentlyContinue
    $siblingTools = Join-Path (Split-Path -Parent $projectRoot) 'Braid Retrieval Engine/.tools'
    $siblingGo = Join-Path $siblingTools 'go/bin/go.exe'
    if (Test-Path -LiteralPath $localGo) { $taskGo = $localGo }
    elseif ($onPath) { $taskGo = $onPath.Source }
    elseif (Test-Path -LiteralPath $siblingGo) {
        $taskGo = $siblingGo
        $env:GOPATH = Join-Path $siblingTools 'gopath'
    } else { throw 'Install Go 1.27.1 or set HEIMDALL_GO to its executable.' }
}
$env:GOCACHE = Join-Path $projectRoot '.tools/gocache'
& $taskGo @args
exit $LASTEXITCODE
