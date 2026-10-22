[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidateSet("frontend", "backend")][string]$Role,
    [Parameter(Mandatory=$true)][string]$ProjectRoot,
    [Parameter(Mandatory=$true)][string]$StateFile,
    [Parameter(Mandatory=$true)][string]$GoExecutable,
    [Parameter(Mandatory=$true)][string]$PnpmExecutable
)
$ErrorActionPreference = "Stop"
$ProjectRoot = (Resolve-Path $ProjectRoot).Path
$roleRoot = if ($Role -eq "frontend") { Join-Path $ProjectRoot "frontend" } else { Join-Path $ProjectRoot "backend" }
$logRoot = Split-Path -Parent $StateFile
New-Item -ItemType Directory -Path $logRoot -Force | Out-Null
$stdout = Join-Path $logRoot ($Role + "-stdout.log")
$stderr = Join-Path $logRoot ($Role + "-stderr.log")
$port = if ($Role -eq "frontend") { 3014 } else { 8422 }
$start = New-Object System.Diagnostics.ProcessStartInfo
$start.WorkingDirectory = $roleRoot
$start.UseShellExecute = $false
$start.CreateNoWindow = $true
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
$start.FileName = if ($Role -eq "frontend") { $env:ComSpec } else { $GoExecutable }
if ($Role -eq "frontend") {
    $start.Arguments = '/d /s /c ""{0}" dev --port {1} --strictPort"' -f $PnpmExecutable, $port
    $start.EnvironmentVariables["VITE_BACKEND_URL"] = "http://127.0.0.1:8422"
} else {
    $start.Arguments = "run ./cmd/server"
    $start.EnvironmentVariables["APP_SECRET"] = "please-change-me-to-a-long-random-secret-32bytes-min"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_HOST"] = "127.0.0.1"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_PORT"] = "5432"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_USER"] = "gatewayops"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_PASSWORD"] = "change-me"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_NAME"] = "gatewayops"
    $start.EnvironmentVariables["GATEWAYOPS_DATABASE_SSLMODE"] = "disable"
    $start.EnvironmentVariables["GATEWAYOPS_SERVER_PORT"] = "8422"
    $start.EnvironmentVariables["GATEWAYOPS_SERVER_MODE"] = "debug"
    $start.EnvironmentVariables["AUTH_ENABLED"] = "false"
}
foreach ($entry in $start.EnvironmentVariables.GetEnumerator()) {
    [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, "Process")
}
try {
    $process = Start-Process -FilePath $start.FileName -ArgumentList $start.Arguments -WorkingDirectory $roleRoot -RedirectStandardOutput $stdout -RedirectStandardError $stderr -WindowStyle Hidden -PassThru
} catch {
    [IO.File]::WriteAllText($stderr, $_.Exception.ToString(), [Text.UTF8Encoding]::new($false))
    throw
}
$record = [ordered]@{
    ProjectRoot = $ProjectRoot
    Role = $Role
    WorkerId = $PID
    WorkerStartTicks = (Get-Process -Id $PID).StartTime.ToUniversalTime().Ticks
    ChildId = $process.Id
    ChildStartTicks = $process.StartTime.ToUniversalTime().Ticks
    ChildExecutable = (Get-CimInstance Win32_Process -Filter ("ProcessId = " + $process.Id)).ExecutablePath
    ChildCommandLine = (Get-CimInstance Win32_Process -Filter ("ProcessId = " + $process.Id)).CommandLine
    Port = $port
}
$tempState = $StateFile + ".tmp"
ConvertTo-Json -InputObject $record -Depth 4 | Set-Content -LiteralPath $tempState -Encoding UTF8
Move-Item -LiteralPath $tempState -Destination $StateFile -Force
$process.WaitForExit()
exit $process.ExitCode
