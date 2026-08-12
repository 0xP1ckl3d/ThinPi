[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$launcher = Join-Path $root "build\launcher-dev\thinpi-launcher.exe"
if (-not (Test-Path -LiteralPath $launcher)) {
    throw "The launcher has not been built. Run .\scripts\dev-up.ps1 first."
}

$qtPrefix = if ($env:QT_ROOT) { $env:QT_ROOT } else {
    Get-ChildItem -LiteralPath "C:\Qt" -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending |
        ForEach-Object { Join-Path $_.FullName "mingw_64" } |
        Where-Object { Test-Path -LiteralPath (Join-Path $_ "bin\Qt6Core.dll") } |
        Select-Object -First 1
}
if (-not $qtPrefix) { throw "Qt 6 was not found. Set QT_ROOT to the desktop kit directory." }

$env:Path = "$(Join-Path $qtPrefix 'bin');$env:Path"
$env:THINPI_DEV_MODE = "1"
$env:THINPI_API_URL = "http://127.0.0.1:8080"
$env:THINPI_AGENT_SOCKET = "thinpi-agent-dev"
$env:THINPI_DEVICE_ID = "dev-device"

$process = Start-Process -FilePath $launcher -WorkingDirectory $root -PassThru
Write-Host "ThinPi launcher opened (PID $($process.Id)). Sign in with daughter / thinpi-dev."
