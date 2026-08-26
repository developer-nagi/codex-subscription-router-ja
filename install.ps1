#Requires -Version 5.1
<#
.SYNOPSIS
    Codex Subscription Router を Windows へ導入する。
.DESCRIPTION
    ソースを取得または更新し、固定されたビルド依存を入れ、公式 ChatGPT デスクトップの
    独立コピーを作成して起動する。公式アプリ自体は決して変更しない。
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
    throw "インストール失敗: $Message"
}

function Test-CommandAvailable {
    param([Parameter(Mandatory)][string] $Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Assert-Prerequisites {
    if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
        Stop-Install 'Codex Subscription Router は Windows 専用。'
    }
    if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
        Stop-Install "現在 x64 のみ対応 (検出: $env:PROCESSOR_ARCHITECTURE)。"
    }

    $package = Get-AppxPackage -Name $PackageName -ErrorAction SilentlyContinue |
        Sort-Object Version | Select-Object -Last 1
    if ($null -eq $package) {
        Stop-Install '先に Microsoft Store から公式 ChatGPT デスクトップを導入する。'
    }

    $missing = @()
    foreach ($command in @('git', 'go', 'node', 'npm', 'python')) {
        if (-not (Test-CommandAvailable $command)) { $missing += $command }
    }
    if ($missing.Count -ne 0) {
        Stop-Install ("前提ツールが不足: {0}。Git、Go 1.26 以上、Node.js 22.12 以上、Python 3.11 以上を導入して再実行する。" -f ($missing -join ', '))
    }

    $nodeVersion = [Version](& node -p 'process.versions.node')
    if ($nodeVersion -lt [Version]'22.12') {
        Stop-Install "Node.js 22.12 以上が必要 (検出: $nodeVersion)。"
    }

    $goRaw = (& go env GOVERSION) -replace '^go', ''
    $goVersion = [Version](($goRaw -split '-')[0] -replace '^(\d+\.\d+(\.\d+)?).*$', '$1')
    if ($goVersion -lt [Version]'1.26') {
        Stop-Install "Go 1.26 以上が必要 (検出: go$goRaw)。"
    }

    Write-Host "公式 ChatGPT デスクトップ: $($package.Version)"
}

function Resolve-SourceDirectory {
    $scriptDirectory = Split-Path -Parent $PSCommandPath
    if ($scriptDirectory -and (Test-Path (Join-Path $scriptDirectory 'scripts\patch_app.py'))) {
        return $scriptDirectory
    }

    $directory = if ([string]::IsNullOrWhiteSpace($SourceDirectory)) { $DefaultSourceDirectory } else { $SourceDirectory }

    if (Test-Path (Join-Path $directory '.git')) {
        if (& git -C $directory status --porcelain) {
            Stop-Install "$directory にローカル変更がある。保存または commit してから更新する。"
        }
        if ((& git -C $directory branch --show-current) -ne 'main') {
            Stop-Install "$directory が main ブランチではない。切り替えるか CODEX_SUBSCRIPTION_ROUTER_SOURCE_DIR を設定する。"
        }
        Write-Step 'ソースを更新中'
        & git -C $directory pull --ff-only origin main
        if ($LASTEXITCODE -ne 0) { Stop-Install 'ソースの更新に失敗した。' }
    }
    elseif (Test-Path $directory) {
        Stop-Install "$directory は存在するが Git リポジトリではない。"
    }
    else {
        Write-Step 'ソースを取得中'
        $parent = Split-Path -Parent $directory
        if (-not (Test-Path $parent)) { New-Item -ItemType Directory -Force $parent | Out-Null }
        & git clone --depth 1 --branch main $RepositoryUrl $directory
        if ($LASTEXITCODE -ne 0) { Stop-Install 'ソースの取得に失敗した。' }
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
    Stop-Install "$Path に属するプロセスを終了できなかった。"
}

Write-Step 'この PC を確認中'
Assert-Prerequisites

$projectDirectory = Resolve-SourceDirectory
Set-Location $projectDirectory

Write-Step '固定されたビルド依存を導入中'
& npm ci --ignore-scripts --no-audit --no-fund
if ($LASTEXITCODE -ne 0) { Stop-Install 'ビルド依存の導入に失敗した。' }

$patchArguments = @()
if (Test-Path $DestinationApp) {
    Write-Step '既存のインストールを停止中'
    Stop-InstalledApp -Path $DestinationApp
    $patchArguments += '--force'
}

Write-Step 'Codex Subscription Router をビルド中'
$env:PYTHONIOENCODING = 'utf-8'
& python scripts/patch_app.py @patchArguments
if ($LASTEXITCODE -ne 0) { Stop-Install 'パッチ処理に失敗した。' }

Write-Step 'Codex Subscription Router を起動中'
Start-Process -FilePath (Join-Path $DestinationApp 'CodexSubscriptionRouter.exe')

Write-Host ''
Write-Host "インストール完了: $DestinationApp"
