[CmdletBinding()]
param([switch]$DryRun, [switch]$Stop, [switch]$NoBrowser)
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$frontendRoot = Join-Path $projectRoot "frontend"
$backendRoot = Join-Path $projectRoot "backend"
$frontendPort = 3014
$backendPort = 8422
$branch = (& git -C $projectRoot branch --show-current 2>$null).Trim()
if ($LASTEXITCODE -ne 0 -or $branch -ne "main") {
    throw "Refusing to start: expected branch main, got '$branch'."
}
if (!(Test-Path (Join-Path $frontendRoot "package.json")) -or !(Test-Path (Join-Path $backendRoot "go.mod"))) {
    throw "Frontend or backend files are missing from the selected worktree."
}
$hash = [System.Security.Cryptography.SHA256]::Create()
try { $key = [BitConverter]::ToString($hash.ComputeHash([Text.Encoding]::UTF8.GetBytes($projectRoot.ToLowerInvariant()))).Replace("-", "").Substring(0, 12).ToLowerInvariant() }
finally { $hash.Dispose() }
$schemaName = "gatewayops_pelican_test_$key"
$stateRoot = Join-Path $env:LOCALAPPDATA ("GatewayOpsTest\" + $key)
$workerScript = Join-Path $PSScriptRoot "run-test-service.ps1"
$entry = [ordered]@{
    ProjectRoot = $projectRoot
    Branch = $branch
    FrontendRoot = $frontendRoot
    BackendRoot = $backendRoot
    FrontendPort = $frontendPort
    BackendPort = $backendPort
    URL = "http://127.0.0.1:$frontendPort/relay-stations"
    TestSchema = $schemaName
    DatabaseNote = "An isolated schema inside the local PostgreSQL database; the public schema is untouched."
    StateRoot = $stateRoot
}
if ($DryRun) { [Console]::Out.WriteLine((ConvertTo-Json -InputObject $entry -Compress)); exit 0 }
if (!(Get-Command pnpm.cmd -ErrorAction SilentlyContinue)) { throw "pnpm.cmd was not found in PATH." }
if (!(Get-Command go.exe -ErrorAction SilentlyContinue)) { throw "go.exe was not found in PATH." }
$goExe = (Get-Command go.exe).Source
$env:GATEWAYOPS_DATABASE_SCHEMA = $schemaName
$env:GATEWAYOPS_DATABASE_HOST = "127.0.0.1"
$env:GATEWAYOPS_DATABASE_PORT = "5432"
$env:GATEWAYOPS_DATABASE_USER = "gatewayops"
$env:GATEWAYOPS_DATABASE_PASSWORD = "change-me"
$env:GATEWAYOPS_DATABASE_NAME = "gatewayops"
$env:GATEWAYOPS_DATABASE_SSLMODE = "disable"
$pnpmExe = (Get-Command pnpm.cmd).Source
New-Item -ItemType Directory -Path $stateRoot -Force | Out-Null
$mutexName = "Local\GatewayOpsPelicanTest_" + $key
$mutex = [Threading.Mutex]::new($false, $mutexName)
$locked = $false
try {
    try { $locked = $mutex.WaitOne([TimeSpan]::FromSeconds(10)) }
    catch [Threading.AbandonedMutexException] { $locked = $true }
    if (!$locked) { throw "Another test restart is already running." }
    function Get-CimProcessSnapshot {
        @(Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, ExecutablePath, CommandLine)
    }
    function Get-ProcessRecord([string]$role) {
        $path = Join-Path $stateRoot ($role + ".json")
        if (!(Test-Path $path)) { return $null }
        try { return (Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json) }
        catch { Remove-Item -LiteralPath $path -Force; return $null }
    }
    function Stop-TrackedRole([string]$role) {
        $record = Get-ProcessRecord $role
        if (!$record -or $record.ProjectRoot -ne $projectRoot -or $record.Role -ne $role) { return }
        $snapshot = Get-CimProcessSnapshot
        $worker = $snapshot | Where-Object { $_.ProcessId -eq [uint32]$record.WorkerId } | Select-Object -First 1
        $child = $snapshot | Where-Object { $_.ProcessId -eq [uint32]$record.ChildId } | Select-Object -First 1
        $workerValid = $false
        if ($worker) {
            try {
                $proc = Get-Process -Id $worker.ProcessId -ErrorAction Stop
                $cmd = [string]$worker.CommandLine
                $workerValid = ($proc.StartTime.ToUniversalTime().Ticks -eq [int64]$record.WorkerStartTicks) -and
                    ($worker.Name -match "^powershell(_ise)?\.exe$|^pwsh\.exe$") -and
                    ($cmd.IndexOf($workerScript, [StringComparison]::OrdinalIgnoreCase) -ge 0) -and
                    ($cmd.IndexOf($projectRoot, [StringComparison]::OrdinalIgnoreCase) -ge 0) -and
                    ($cmd.IndexOf("-Role $role", [StringComparison]::OrdinalIgnoreCase) -ge 0)
            } catch { $workerValid = $false }
        }
        $childValid = $false
        if ($child) {
            try {
                $proc = Get-Process -Id $child.ProcessId -ErrorAction Stop
                $childValid = ($proc.StartTime.ToUniversalTime().Ticks -eq [int64]$record.ChildStartTicks) -and
                    ([string]$child.ExecutablePath -eq [string]$record.ChildExecutable) -and
                    ([string]$child.CommandLine -eq [string]$record.ChildCommandLine) -and
                    (!$workerValid -or [uint32]$child.ParentProcessId -eq [uint32]$record.WorkerId)
            } catch { $childValid = $false }
        }
        if ($childValid) {
            $descendants = [Collections.Generic.List[object]]::new()
            function Add-ProcessDescendants([uint32]$parentId, $items, $target) {
                foreach ($item in $items) {
                    if ([uint32]$item.ParentProcessId -eq $parentId) {
                        Add-ProcessDescendants ([uint32]$item.ProcessId) $items $target
                        $target.Add($item)
                    }
                }
            }
            Add-ProcessDescendants ([uint32]$record.ChildId) $snapshot $descendants
            foreach ($descendant in $descendants) {
                try { Stop-Process -Id ([int]$descendant.ProcessId) -Force -ErrorAction Stop } catch {}
            }
            try { Stop-Process -Id ([int]$record.ChildId) -Force -ErrorAction Stop } catch {}
        }
        if ($workerValid) {
            try { Stop-Process -Id ([int]$record.WorkerId) -Force -ErrorAction Stop } catch {}
        }
        Remove-Item -LiteralPath (Join-Path $stateRoot ($role + ".json")) -Force -ErrorAction SilentlyContinue
    }
    Stop-TrackedRole "frontend"
    Stop-TrackedRole "backend"
    if ($Stop) { Write-Host "Test services stopped."; exit 0 }
    Write-Host "Preparing isolated test schema and copying channel/relay snapshots."
    Push-Location $backendRoot
    try { & $goExe run ./tools/seed_pelican_test_schema.go; if ($LASTEXITCODE -ne 0) { throw "Could not prepare the isolated test schema." } }
    finally { Pop-Location }
    Start-Sleep -Milliseconds 500
    $listeners = @(netstat -ano -p tcp | ForEach-Object {
        if ($_ -match (":{0}\s+\S+\s+LISTENING\s+(\d+)\s*$" -f $frontendPort)) { "frontend:$frontendPort PID $($Matches[1])" }
        elseif ($_ -match (":{0}\s+\S+\s+LISTENING\s+(\d+)\s*$" -f $backendPort)) { "backend:$backendPort PID $($Matches[1])" }
    })
    if ($listeners.Count) { throw "A test port is used by an unowned process: $($listeners -join ', '). Nothing was terminated." }
    function Start-TestRole([string]$role) {
        $stateFile = Join-Path $stateRoot ($role + ".json")
        $arguments = "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$workerScript`" -Role $role -ProjectRoot `"$projectRoot`" -StateFile `"$stateFile`" -GoExecutable `"$goExe`" -PnpmExecutable `"$pnpmExe`""
        $process = Start-Process -FilePath (Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe") -ArgumentList $arguments -PassThru -WindowStyle Hidden
        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        while ([DateTime]::UtcNow -lt $deadline) {
            if (Test-Path $stateFile) {
                $record = Get-ProcessRecord $role
                if ($record -and [uint32]$record.WorkerId -eq [uint32]$process.Id -and $record.ProjectRoot -eq $projectRoot) { return $record }
            }
            if ($process.HasExited) { throw "$role worker exited during startup. See $stateRoot\$role-stderr.log" }
            Start-Sleep -Milliseconds 150
        }
        throw "$role worker did not publish its process identity in time."
    }
    function Wait-Endpoint([string]$uri, [string]$role, [int]$seconds) {
        $deadline = [DateTime]::UtcNow.AddSeconds($seconds)
        while ([DateTime]::UtcNow -lt $deadline) {
            $state = Get-ProcessRecord $role
            if (!$state) { throw "$role stopped during startup. Logs: $stateRoot" }
            try { Get-Process -Id ([int]$state.WorkerId) -ErrorAction Stop | Out-Null } catch { throw "$role worker exited during startup. Logs: $stateRoot" }
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $uri -TimeoutSec 2
                if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) { return }
            } catch {}
            Start-Sleep -Milliseconds 500
        }
        throw "Timed out waiting for $role at $uri. Logs: $stateRoot"
    }
    try {
        $null = Start-TestRole "backend"
        $null = Start-TestRole "frontend"
        Wait-Endpoint "http://127.0.0.1:$backendPort/healthz" "backend" 60
        Wait-Endpoint "http://127.0.0.1:$frontendPort/relay-stations" "frontend" 45
    } catch {
        Stop-TrackedRole "frontend"
        Stop-TrackedRole "backend"
        throw
    }
    Write-Host "Test services restarted from main branch."
    Write-Host "Open: $($entry.URL)"
    Write-Host "Logs: $stateRoot"
    Write-Host "Database schema: $schemaName (isolated from the main app)."
    if (!$NoBrowser) { Start-Process $entry.URL | Out-Null }
} finally {
    if ($locked) { $mutex.ReleaseMutex() }
    $mutex.Dispose()
}
