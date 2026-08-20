#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Installs the latest SetFree release for Windows.
.DESCRIPTION
    irm https://raw.githubusercontent.com/mindsdb/setfree/main/install.ps1 | iex

    Installs to a per-user location (no admin rights needed) and adds it to
    the user PATH if it isn't there already. Safe to re-run.
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = 'mindsdb/setfree'
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\SetFree'
$BinName = 'setfree.exe'

function Write-Info($Message) { Write-Host $Message }
function Write-ErrAndExit($Message) {
    Write-Host "setfree: $Message" -ForegroundColor Red
    exit 1
}

function Get-SetfreeArch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture
    switch ($arch) {
        'Arm64' { return 'arm64' }
        'X64' { return 'x86_64' }
        default { Write-ErrAndExit "unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    # Avoid the rate-limited GitHub API: /releases/latest redirects to
    # /releases/tag/<version>, so we just read where it points. Depending on
    # the PowerShell/.NET version, disabling redirects on a 3xx either
    # throws or just returns the response — handle both.
    $url = "https://github.com/$Repo/releases/latest"
    $location = $null
    try {
        $response = Invoke-WebRequest -Uri $url -MaximumRedirection 0 -ErrorAction Ignore -UseBasicParsing
        if ($response -and $response.Headers.Location) {
            $location = $response.Headers.Location
        }
    } catch {
        $location = $_.Exception.Response.Headers.Location
    }
    if (-not $location) {
        Write-ErrAndExit "couldn't reach GitHub to find the latest release."
    }
    $version = ($location -split '/')[-1]
    if (-not $version) {
        Write-ErrAndExit "couldn't determine the latest SetFree version from $url"
    }
    return $version
}

function Add-ToUserPath($Dir) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @()
    if ($userPath) { $entries = $userPath -split ';' }
    if ($entries -contains $Dir) {
        return $false
    }
    $newPath = if ($userPath) { "$userPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    $env:Path = "$env:Path;$Dir"
    return $true
}

function Main {
    $arch = Get-SetfreeArch
    $version = Get-LatestVersion
    Write-Info "Installing SetFree $version for Windows/$arch..."

    $asset = "setfree_Windows_$arch.zip"
    $baseUrl = "https://github.com/$Repo/releases/download/$version"

    $workDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $workDir | Out-Null
    try {
        $zipPath = Join-Path $workDir $asset
        try {
            Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile $zipPath -UseBasicParsing
        } catch {
            Write-ErrAndExit "couldn't download $asset from release $version."
        }

        $checksumsPath = Join-Path $workDir 'checksums.txt'
        $verified = $false
        try {
            Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath -UseBasicParsing
            $line = Select-String -Path $checksumsPath -Pattern ([regex]::Escape($asset)) | Select-Object -First 1
            if ($line) {
                $expected = ($line.Line -split '\s+')[0]
                $actual = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLower()
                if ($expected.ToLower() -ne $actual) {
                    Write-ErrAndExit "checksum mismatch for $asset — expected $expected, got $actual. Aborting."
                }
                $verified = $true
            }
        } catch {
            # checksums.txt not available for this release; fall through.
        }
        if (-not $verified) {
            Write-Host "setfree: warning: couldn't verify $asset's checksum for $version." -ForegroundColor Yellow
        }

        Expand-Archive -Path $zipPath -DestinationPath $workDir -Force
        $binSrc = Join-Path $workDir $BinName
        if (-not (Test-Path $binSrc)) {
            Write-ErrAndExit "$asset didn't contain the expected '$BinName' binary."
        }

        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Copy-Item -Path $binSrc -Destination (Join-Path $InstallDir $BinName) -Force
    } finally {
        Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    Write-Info "✓ Installed $BinName to $InstallDir\$BinName"

    $added = Add-ToUserPath $InstallDir
    Write-Host ''
    if ($added) {
        Write-Info "Added $InstallDir to your user PATH. Open a new terminal, then run 'setfree' to get started."
    } else {
        Write-Info "Run 'setfree' to get started."
    }
}

Main
