[CmdletBinding()]
param([switch]$RealClients)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$state = Join-Path $root ".thinpi-dev"
$compose = Join-Path $root "deploy\controller\compose.dev.yml"
$agentExe = Join-Path $root "bin\thinpi-agent.exe"
$agentConfig = Join-Path $state "agent.windows.json"
$deviceFile = Join-Path $state "device.json"
$pidFile = Join-Path $state "agent.pid"

function Assert-NativeSuccess([string]$Action) {
    if ($LASTEXITCODE -ne 0) {
        throw "$Action failed with exit code $LASTEXITCODE."
    }
}

function Find-QtPrefix {
    if ($env:QT_ROOT -and (Test-Path -LiteralPath (Join-Path $env:QT_ROOT "bin\Qt6Core.dll"))) {
        return $env:QT_ROOT
    }
    $prefix = Get-ChildItem -LiteralPath "C:\Qt" -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending |
        ForEach-Object { Join-Path $_.FullName "mingw_64" } |
        Where-Object { Test-Path -LiteralPath (Join-Path $_ "bin\Qt6Core.dll") } |
        Select-Object -First 1
    if (-not $prefix) {
        throw "Qt 6 with the MinGW desktop kit was not found. Install it under C:\Qt or set QT_ROOT."
    }
    return $prefix
}

function Find-Tool([string]$Preferred, [string]$Fallback) {
    if (Test-Path -LiteralPath $Preferred) { return $Preferred }
    $command = Get-Command $Fallback -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw "$Fallback was not found."
}

function Stop-PreviousAgent {
    if (-not (Test-Path -LiteralPath $pidFile)) { return }
    $oldPid = 0
    if (-not [int]::TryParse((Get-Content -LiteralPath $pidFile -Raw).Trim(), [ref]$oldPid)) { return }
    $process = Get-CimInstance Win32_Process -Filter "ProcessId=$oldPid" -ErrorAction SilentlyContinue
    if ($process -and $process.ExecutablePath -and
        ([IO.Path]::GetFullPath($process.ExecutablePath) -eq [IO.Path]::GetFullPath($agentExe))) {
        Stop-Process -Id $oldPid -Force
        Wait-Process -Id $oldPid -Timeout 5 -ErrorAction SilentlyContinue
    }
}

$env:DOCKER_CONFIG = Join-Path $root ".docker-config"
New-Item -ItemType Directory -Force -Path $env:DOCKER_CONFIG | Out-Null
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker CLI was not found. Install and start Docker Desktop."
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found. Install Go 1.25 or newer."
}
& docker version --format "{{.Server.Version}}" | Out-Null
Assert-NativeSuccess "Docker Engine check (is Docker Desktop running?)"
$goVersionText = (& go env GOVERSION).Trim() -replace '^go', ''
$goVersion = $null
if (-not [version]::TryParse($goVersionText, [ref]$goVersion) -or $goVersion -lt [version]'1.25') {
    throw "Go 1.25 or newer is required; found $goVersionText."
}
$qtPrefix = Find-QtPrefix

New-Item -ItemType Directory -Force -Path $state, (Split-Path -Parent $agentExe) | Out-Null
$deviceSettings = [ordered]@{ device_identifier = "dev-device"; device_token = "dev-device-token"; name = "Development Pi" }
$deviceSettings | ConvertTo-Json | Set-Content -LiteralPath $deviceFile -Encoding UTF8
$agentSettings = [ordered]@{
    controller_url = "http://127.0.0.1:8080"
    device_file = $deviceFile
    socket = "thinpi-agent-dev"
    mock_clients = (-not $RealClients)
    mock_duration_seconds = 12
    freerdp_binary = "auto"
    moonlight_binary = "auto"
    vnc_binary = "auto"
}
$agentSettings | ConvertTo-Json | Set-Content -LiteralPath $agentConfig -Encoding UTF8
Write-Host "Starting the local ThinPi controller..."
& docker compose --project-name thinpi-dev --file $compose up --detach --build
Assert-NativeSuccess "Docker Compose"

$ready = $false
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
        $response = Invoke-WebRequest -Uri "http://127.0.0.1:8080/healthz" -TimeoutSec 2
        if ($response.StatusCode -eq 200) { $ready = $true; break }
    } catch {
        Start-Sleep -Seconds 1
    }
}
if (-not $ready) {
    & docker compose --project-name thinpi-dev --file $compose logs --tail 100 controller
    throw "The ThinPi controller did not become ready on http://127.0.0.1:8080."
}

Write-Host "Building the Windows agent..."
Push-Location (Join-Path $root "agent")
try {
    & go build -trimpath -o $agentExe .\cmd\thinpi-agent
    Assert-NativeSuccess "Agent build"
} finally {
    Pop-Location
}

$cmake = Find-Tool "C:\Qt\Tools\CMake_64\bin\cmake.exe" "cmake"
if (Test-Path -LiteralPath "C:\Qt\Tools\CMake_64\bin\cmake.exe") {
    $cmake = "C:\Qt\Tools\CMake_64\bin\cmake.exe"
}
$ninja = Find-Tool "C:\Qt\Tools\Ninja\ninja.exe" "ninja"
$mingw = Get-ChildItem -LiteralPath "C:\Qt\Tools" -Directory -Filter "mingw*_64" -ErrorAction SilentlyContinue |
    Sort-Object Name -Descending | Select-Object -First 1
if (-not $mingw) { throw "The Qt MinGW toolchain was not found under C:\Qt\Tools." }
$launcherBuild = Join-Path $root "build\launcher-dev"

Write-Host "Building the Qt launcher..."
$env:Path = "$(Join-Path $qtPrefix 'bin');$(Join-Path $mingw.FullName 'bin');$env:Path"
& $cmake -S (Join-Path $root "launcher") -B $launcherBuild -G Ninja "-DCMAKE_PREFIX_PATH=$qtPrefix" "-DCMAKE_MAKE_PROGRAM=$ninja" "-DCMAKE_CXX_COMPILER=$(Join-Path $mingw.FullName 'bin\g++.exe')" -DBUILD_TESTING=ON
Assert-NativeSuccess "Launcher configuration"
& $cmake --build $launcherBuild
Assert-NativeSuccess "Launcher build"

Stop-PreviousAgent
$stdout = Join-Path $state "agent.out.log"
$stderr = Join-Path $state "agent.err.log"
Remove-Item -LiteralPath $stdout, $stderr -Force -ErrorAction SilentlyContinue
$agent = Start-Process -FilePath $agentExe -ArgumentList @("serve", "--config", $agentConfig) -WorkingDirectory $root -WindowStyle Hidden -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
Set-Content -LiteralPath $pidFile -Value $agent.Id
Start-Sleep -Milliseconds 750
if ($agent.HasExited) {
    Get-Content -LiteralPath $stdout, $stderr -ErrorAction SilentlyContinue
    throw "The local ThinPi agent exited during startup."
}

Write-Host ""
Write-Host "ThinPi development environment is ready." -ForegroundColor Green
Write-Host "Controller: http://127.0.0.1:8080"
Write-Host "Admin UI:  http://127.0.0.1:8080/admin/login"
Write-Host "Users:     admin, wife, daughter"
Write-Host "Password:  thinpi-dev"
Write-Host "Mode:      $(if ($RealClients) { 'Real native clients' } else { 'Safe 12-second demo sessions' })"
Write-Host ""
Write-Host "Run .\scripts\dev-client.ps1 to open the launcher."
Write-Host "Run .\scripts\dev-test.ps1 to exercise a complete mock launch."
