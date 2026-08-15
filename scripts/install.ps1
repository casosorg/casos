# CasOS one-step installer, adapted from OpenAgent's Apache-2.0 installer.
# Usage:
#   irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
#
# Optional environment variables:
#   CASOS_VERSION  release tag such as v1.32.0 (default: latest)
#   CASOS_REPOSITORY GitHub release repository (default: casosorg/casos)
#   INSTALL_DIR    binary directory (default: $env:LOCALAPPDATA\CasOS\bin)

$ErrorActionPreference = 'Stop'

$Repo = if ($env:CASOS_REPOSITORY) { $env:CASOS_REPOSITORY } else { 'casosorg/casos' }
$Version = if ($env:CASOS_VERSION) { $env:CASOS_VERSION } else { 'latest' }
$InstallDir = if ($env:INSTALL_DIR) {
    if ($env:INSTALL_DIR.Contains([System.IO.Path]::PathSeparator)) {
        throw 'INSTALL_DIR must not contain a PATH separator.'
    }
    [System.IO.Path]::GetFullPath($env:INSTALL_DIR)
} else {
    Join-Path $env:LOCALAPPDATA 'CasOS\bin'
}

if ($Repo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw "Invalid GitHub repository: $Repo" }

if ($Version -eq 'latest') {
    $ReleaseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
    if ($Version -notmatch '^v[0-9A-Za-z._-]+$') { throw "Invalid release version: $Version" }
    $ReleaseUrl = "https://github.com/$Repo/releases/download/$Version"
}

$Architecture = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$ArchName = switch ($Architecture) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $Architecture" }
}

$Filename = "casos_windows_${ArchName}.exe"
$TempDir = Join-Path $env:TEMP "casos_install_$([guid]::NewGuid().ToString('N'))"
$PendingExe = $null
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $ExePath = Join-Path $TempDir $Filename
    $ChecksumsPath = Join-Path $TempDir 'SHA256SUMS'
    Write-Host "Downloading CasOS $Version..."
    Invoke-WebRequest -Uri "$ReleaseUrl/$Filename" -OutFile $ExePath -UseBasicParsing
    Invoke-WebRequest -Uri "$ReleaseUrl/SHA256SUMS" -OutFile $ChecksumsPath -UseBasicParsing

    $ChecksumLine = Get-Content $ChecksumsPath |
        Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$([regex]::Escape($Filename))$" } |
        Select-Object -First 1
    if (-not $ChecksumLine) { throw "Release checksum for $Filename was not found." }
    $Expected = ($ChecksumLine -split '\s+')[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ExePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { throw 'Download checksum verification failed.' }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $InstallDir = [System.IO.Path]::GetFullPath($InstallDir)
    $InstalledExe = Join-Path $InstallDir 'casos.exe'
    $PendingExe = Join-Path $InstallDir 'casos.new.exe'

    if (Test-Path $InstalledExe) {
        $RunningCasOS = Get-CimInstance Win32_Process -Filter "Name = 'casos.exe'" -ErrorAction SilentlyContinue |
            Where-Object { $_.ExecutablePath -and ([System.IO.Path]::GetFullPath($_.ExecutablePath) -eq $InstalledExe) }
        if ($RunningCasOS) {
            throw 'CasOS is currently running. Exit CasOS, then run the installer again to upgrade.'
        }
    }

    Copy-Item -Path $ExePath -Destination $PendingExe -Force
    Move-Item -Path $PendingExe -Destination $InstalledExe -Force
}
finally {
    Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    if ($PendingExe) { Remove-Item -Force $PendingExe -ErrorAction SilentlyContinue }
}

$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
$PathEntries = @($UserPath -split ';' | Where-Object { $_ })
if ($PathEntries -notcontains $InstallDir) {
    [Environment]::SetEnvironmentVariable('PATH', (@($PathEntries) + $InstallDir) -join ';', 'User')
    Write-Host "Added $InstallDir to your user PATH."
}
$env:PATH = "$env:PATH;$InstallDir"

Write-Host "CasOS $Version installed at $InstalledExe"
Write-Host "Run 'casos' and open http://localhost:9000"
