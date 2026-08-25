#Requires -Version 5.1
<#
.SYNOPSIS
    リポジトリ内の PowerShell スクリプトを構文検査する。
.DESCRIPTION
    Windows PowerShell 5.1 は BOM の無い UTF-8 を ANSI として読むため、
    非 ASCII を含むスクリプトには BOM が必要になる。構文と併せて検査する。
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$scripts = Get-ChildItem -Path $root -Filter '*.ps1' -Recurse -File |
    Where-Object { $_.FullName -notmatch '\\node_modules\\' }

$failed = $false
foreach ($script in $scripts) {
    $relative = $script.FullName.Substring($root.Length + 1)

    $bytes = [System.IO.File]::ReadAllBytes($script.FullName)
    $hasBom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    $isAscii = -not ($bytes | Where-Object { $_ -gt 0x7F })
    if (-not $hasBom -and -not $isAscii) {
        Write-Host "NG  $relative : 非 ASCII を含むため UTF-8 BOM が必要"
        $failed = $true
        continue
    }

    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$null, [ref]$errors)
    if ($errors -and $errors.Count -gt 0) {
        Write-Host "NG  $relative"
        $errors | ForEach-Object { Write-Host "      $($_.Message)" }
        $failed = $true
        continue
    }

    Write-Host "OK  $relative"
}

if ($failed) {
    Write-Error 'PowerShell スクリプトの検査に失敗した'
    exit 1
}

Write-Host "PowerShell scripts checked: $($scripts.Count)"
