#Requires -Version 5.1
<#
.SYNOPSIS
    Install Codex Subscription Router on Windows.
.DESCRIPTION
    Fetch or update the source, install the locked build dependencies, create an
    independent copy of the official ChatGPT desktop and launch it. The official app
    itself is never modified.
#>

[CmdletBinding()]
param(
    [string] $SourceDirectory = $env:CODEX_SUBSCRIPTION_ROUTER_SOURCE_DIR
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryUrl = 'https://github.com/developer-nagi/codex-subscription-router-win.git'
$DefaultSourceDirectory = Join-Path $env:USERPROFILE '.codex-subscription-router\source'
$DestinationApp = Join-Path $env:LOCALAPPDATA 'Programs\Codex Subscription Router'
$PackageName = 'OpenAI.Codex'

function Write-Step {
    param([Parameter(Mandatory)][string] $Message)
    Write-Host ''
    Write-Host "==> $Message"
}

function Stop-Install {
    param([Parameter(Mandatory)][string] $Message)
    throw "install failed: $Message"
}

function Test-CommandAvailable {
    param([Parameter(Mandatory)][string] $Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Assert-Prerequisites {
    if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
        Stop-Install 'Codex Subscription Router is Windows only.'
    }
    if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
        Stop-Install "Only x64 is supported (detected: $env:PROCESSOR_ARCHITECTURE)."
    }

    $package = Get-AppxPackage -Name $PackageName -ErrorAction SilentlyContinue |
        Sort-Object Version | Select-Object -Last 1
    if ($null -eq $package) {
        Stop-Install 'Install the official ChatGPT desktop from the Microsoft Store first.'
    }

    $missing = @()
    foreach ($command in @('git', 'go', 'node', 'npm', 'python')) {
        if (-not (Test-CommandAvailable $command)) { $missing += $command }
    }
    if ($missing.Count -ne 0) {
        Stop-Install ("Missing prerequisites: {0}. Install Git, Go 1.26+, Node.js 22.12+ and Python 3.11+, then run again." -f ($missing -join ', '))
    }

    $nodeVersion = [Version](& node -p 'process.versions.node')
    if ($nodeVersion -lt [Version]'22.12') {
        Stop-Install "Node.js 22.12 or newer is required (detected: $nodeVersion)."
    }

    $goRaw = (& go env GOVERSION) -replace '^go', ''
    $goVersion = [Version](($goRaw -split '-')[0] -replace '^(\d+\.\d+(\.\d+)?).*$', '$1')
    if ($goVersion -lt [Version]'1.26') {
        Stop-Install "Go 1.26 or newer is required (detected: go$goRaw)."
    }

    Write-Host "Official ChatGPT desktop: $($package.Version)"
}

function Resolve-SourceDirectory {
    $scriptDirectory = Split-Path -Parent $PSCommandPath
    if ($scriptDirectory -and (Test-Path (Join-Path $scriptDirectory 'scripts\patch_app.py'))) {
        return $scriptDirectory
    }

    $directory = if ([string]::IsNullOrWhiteSpace($SourceDirectory)) { $DefaultSourceDirectory } else { $SourceDirectory }

    if (Test-Path (Join-Path $directory '.git')) {
        if (& git -C $directory status --porcelain) {
            Stop-Install "$directory has local changes. Stash or commit them before updating."
        }
        if ((& git -C $directory branch --show-current) -ne 'main') {
            Stop-Install "$directory is not on main. Switch branches or set CODEX_SUBSCRIPTION_ROUTER_SOURCE_DIR."
        }
        Write-Step 'Updating the source'
        & git -C $directory pull --ff-only origin main
        if ($LASTEXITCODE -ne 0) { Stop-Install 'Could not update the source.' }
    }
    elseif (Test-Path $directory) {
        Stop-Install "$directory exists but is not a Git repository."
    }
    else {
        Write-Step 'Fetching the source'
        $parent = Split-Path -Parent $directory
        if (-not (Test-Path $parent)) { New-Item -ItemType Directory -Force $parent | Out-Null }
        & git clone --depth 1 --branch main $RepositoryUrl $directory
        if ($LASTEXITCODE -ne 0) { Stop-Install 'Could not fetch the source.' }
    }

    return $directory
}

function Stop-InstalledApp {
    param([Parameter(Mandatory)][string] $Path)
    $processes = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like "$Path*" })
    foreach ($process in $processes) {
        try { Stop-Process -Id $process.ProcessId -Force -ErrorAction Stop } catch { }
    }
    for ($attempt = 0; $attempt -lt 10; $attempt++) {
        $remaining = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like "$Path*" })
        if ($remaining.Count -eq 0) { return }
        Start-Sleep -Seconds 1
    }
    Stop-Install "Could not stop the processes belonging to $Path."
}

Write-Step 'Checking this PC'
Assert-Prerequisites

$projectDirectory = Resolve-SourceDirectory
Set-Location $projectDirectory

Write-Step 'Installing the locked build dependencies'
& npm ci --ignore-scripts --no-audit --no-fund
if ($LASTEXITCODE -ne 0) { Stop-Install 'Could not install the build dependencies.' }

$patchArguments = @()
if (Test-Path $DestinationApp) {
    Write-Step 'Stopping the existing installation'
    Stop-InstalledApp -Path $DestinationApp
    $patchArguments += '--force'
}

Write-Step 'Building Codex Subscription Router'
$env:PYTHONIOENCODING = 'utf-8'
& python scripts/patch_app.py @patchArguments
if ($LASTEXITCODE -ne 0) { Stop-Install 'Patching failed.' }

Write-Step 'Launching Codex Subscription Router'
Start-Process -FilePath (Join-Path $DestinationApp 'CodexSubscriptionRouter.exe')

Write-Host ''
Write-Host "Installed: $DestinationApp"
