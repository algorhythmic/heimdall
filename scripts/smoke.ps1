# Exercise the compiled Windows CLI and crash/restart recovery using synthetic data.
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $projectRoot 'bin/heimdall.exe'
if (-not (Test-Path -LiteralPath $binary)) { throw 'Build bin/heimdall.exe first.' }
$smokeDir = Join-Path ([System.IO.Path]::GetTempPath()) ('heimdall-smoke-' + [guid]::NewGuid().ToString('N'))
[void][System.IO.Directory]::CreateDirectory($smokeDir)
$pinnedTime = '2026-09-04T18:00:00Z'
function Invoke-Heimdall {
    $result = & $binary @args --data-dir $smokeDir --now $pinnedTime
    if ($LASTEXITCODE -ne 0) { throw "Heimdall failed: $args" }
    return ($result -join "`n").Trim()
}
function Start-SmokeDaemon([string]$suffix) {
    $process = Start-Process -FilePath $binary -ArgumentList @('start','--data-dir',('"' + $smokeDir + '"'),'--now',$pinnedTime) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $smokeDir "daemon-$suffix.out") -RedirectStandardError (Join-Path $smokeDir "daemon-$suffix.err")
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($process.HasExited) { throw "Daemon exited: $(Get-Content -LiteralPath (Join-Path $smokeDir "daemon-$suffix.err") -Raw)" }
        if (Test-Path -LiteralPath (Join-Path $smokeDir 'endpoint.json')) {
            & $binary doctor --data-dir $smokeDir 2>$null | Out-Null
            if ($LASTEXITCODE -eq 0) { return $process }
        }
        Start-Sleep -Milliseconds 100
    }
    Stop-Process -Id $process.Id -ErrorAction SilentlyContinue
    throw 'Daemon readiness timed out.'
}
Invoke-Heimdall init | Out-Null
$running = $null
try {
    $running = Start-SmokeDaemon 'first'
    Invoke-Heimdall import-tasks (Join-Path $projectRoot 'testdata/tasks.yaml') | Out-Null
    Invoke-Heimdall add 'Review core' --id review-core --status active | Out-Null
    Invoke-Heimdall update review-core --title 'Review replay behavior' | Out-Null
    Invoke-Heimdall capture 'heimdall-core/reference: source notes' --pointer 'https://example.test/design' | Out-Null
    Invoke-Heimdall complete 'heimdall-core#store' | Out-Null
    Invoke-Heimdall complete 'heimdall-core#tasks' | Out-Null
    $proposals = @(Invoke-Heimdall ratify | ConvertFrom-Json)
    if ($proposals.Count -ne 1) { throw 'Expected one completion proposal.' }
    Invoke-Heimdall ratify $proposals[0].id --accept | Out-Null
    $before = Invoke-Heimdall state
    Invoke-Heimdall replay | Out-Null
    if ($before -cne (Invoke-Heimdall state)) { throw 'Replay changed serialized state.' }
    Stop-Process -Id $running.Id
    $running.WaitForExit()
    $running = Start-SmokeDaemon 'restart'
    if ($before -cne (Invoke-Heimdall state)) { throw 'Crash/restart changed serialized state.' }
    [pscustomobject]@{status='passed';checks=@('compiled CLI','task edits','capture','step completion','proposal acceptance','byte-identical replay','crash/restart recovery');data_dir=$smokeDir} | ConvertTo-Json -Depth 4
} finally {
    if ($running -and -not $running.HasExited) { Stop-Process -Id $running.Id; $running.WaitForExit() }
}
