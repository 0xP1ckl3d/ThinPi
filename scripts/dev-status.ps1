[CmdletBinding()]
param()

$root = Split-Path -Parent $PSScriptRoot
$env:DOCKER_CONFIG = Join-Path $root ".docker-config"
$compose = Join-Path $root "deploy\controller\compose.dev.yml"
& docker compose --project-name thinpi-dev --file $compose ps

$pidFile = Join-Path $root ".thinpi-dev\agent.pid"
if (Test-Path -LiteralPath $pidFile) {
    $agentPid = Get-Content -LiteralPath $pidFile -Raw
    $agent = Get-Process -Id ([int]$agentPid.Trim()) -ErrorAction SilentlyContinue
    if ($agent) { Write-Host "Agent: running (PID $($agent.Id))" } else { Write-Host "Agent: stopped" }
} else {
    Write-Host "Agent: not started"
}

try {
    $health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -TimeoutSec 2
    Write-Host "Controller API: $($health.status)"
} catch {
    Write-Host "Controller API: unavailable"
}
