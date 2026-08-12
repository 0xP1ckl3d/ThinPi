[CmdletBinding()]
param(
    [string]$Username = "daughter",
    [string]$Password = "thinpi-dev"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$base = "http://127.0.0.1:8080"

function Send-AgentRequest([hashtable]$Request) {
    $pipe = [IO.Pipes.NamedPipeClientStream]::new(".", "thinpi-agent-dev", [IO.Pipes.PipeDirection]::InOut)
    try {
        $pipe.Connect(3000)
        $writer = [IO.StreamWriter]::new($pipe)
        $writer.AutoFlush = $true
        $reader = [IO.StreamReader]::new($pipe)
        $writer.WriteLine(($Request | ConvertTo-Json -Compress))
        $line = $reader.ReadLine()
        if (-not $line) { throw "The agent returned an empty response." }
        return $line | ConvertFrom-Json
    } finally {
        $pipe.Dispose()
    }
}

$health = Invoke-RestMethod -Uri "$base/healthz"
$login = Invoke-RestMethod -Method Post -Uri "$base/api/v1/auth/login" -ContentType "application/json" -Body (@{ username = $Username; password = $Password } | ConvertTo-Json)
$headers = @{ Authorization = "Bearer $($login.token)" }
$connections = Invoke-RestMethod -Headers $headers -Uri "$base/api/v1/connections"
$connection = @($connections.items)[0]
if (-not $connection) { throw "$Username has no development connections." }

$agentBefore = Send-AgentRequest @{ action = "status" }
$launch = Invoke-RestMethod -Method Post -Headers $headers -Uri "$base/api/v1/connections/$($connection.id)/launch" -ContentType "application/json" -Body (@{ device_identifier = "dev-device" } | ConvertTo-Json)
$accepted = Send-AgentRequest @{ action = "launch"; ticket = $launch.ticket }
if (-not $accepted.accepted) { throw "The agent rejected the launch ticket." }
Start-Sleep -Milliseconds 250
$agentActive = Send-AgentRequest @{ action = "status" }
if ($agentActive.status.state -ne "active") { throw "The mock session did not enter the active state." }
$deadline = [DateTime]::UtcNow.AddSeconds(18)
do {
    Start-Sleep -Milliseconds 250
    $agentAfter = Send-AgentRequest @{ action = "status" }
} while ($agentAfter.status.state -ne "idle" -and [DateTime]::UtcNow -lt $deadline)
if ($agentAfter.status.state -ne "idle") { throw "The mock session did not return to idle." }

Write-Host "Controller health: $($health.status)"
Write-Host "Authenticated as:  $($login.user.display_name)"
Write-Host "Connection:        $($connection.name) ($($connection.protocol))"
Write-Host "Agent device:      $($agentBefore.device_identifier)"
Write-Host "Mock session:      $($agentActive.status.state)"
Write-Host "Final agent state: $($agentAfter.status.state)"
Write-Host "End-to-end launch flow passed." -ForegroundColor Green
