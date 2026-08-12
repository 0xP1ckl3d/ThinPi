[CmdletBinding()]
param([switch]$ResetData)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$state = Join-Path $root ".thinpi-dev"
$pidFile = Join-Path $state "agent.pid"
$agentExe = Join-Path $root "bin\thinpi-agent.exe"

if (Test-Path -LiteralPath $pidFile) {
    $agentPid = 0
    if ([int]::TryParse((Get-Content -LiteralPath $pidFile -Raw).Trim(), [ref]$agentPid)) {
        $process = Get-CimInstance Win32_Process -Filter "ProcessId=$agentPid" -ErrorAction SilentlyContinue
        if ($process -and $process.ExecutablePath -and
            ([IO.Path]::GetFullPath($process.ExecutablePath) -eq [IO.Path]::GetFullPath($agentExe))) {
            Stop-Process -Id $agentPid -Force
            Wait-Process -Id $agentPid -Timeout 5 -ErrorAction SilentlyContinue
        }
    }
    Remove-Item -LiteralPath $pidFile -Force
}

$env:DOCKER_CONFIG = Join-Path $root ".docker-config"
$compose = Join-Path $root "deploy\controller\compose.dev.yml"
if ($ResetData) {
    & docker compose --project-name thinpi-dev --file $compose down --volumes
} else {
    & docker compose --project-name thinpi-dev --file $compose down
}
if ($LASTEXITCODE -ne 0) { throw "Docker Compose shutdown failed." }
Write-Host $(if ($ResetData) { "ThinPi development environment stopped and its test database removed." } else { "ThinPi development environment stopped; its test database was kept." })
