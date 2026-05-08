# ---- config ----------------------------------------------------------------
$Repo        = "pixelib/goomba"
$Binary      = "goomba"
$InstallDir  = "$Env:LOCALAPPDATA\Programs\$Binary"
# ----------------------------------------------------------------------------

$ErrorActionPreference = "Stop"

$Arch = $Env:PROCESSOR_ARCHITECTURE
switch ($Arch) {
    "AMD64" { $arch = "amd64" }
    "ARM64" { $arch = "arm64" }
    default {
        Write-Error "Unsupported architecture: $Arch"
        exit 1
    }
}

$Asset   = "$Binary-windows-$arch.exe"
$ApiUrl  = "https://api.github.com/repos/$Repo/releases/latest"

Write-Host "Fetching latest release from $Repo..."
$Release     = Invoke-RestMethod -Uri $ApiUrl -UseBasicParsing
$DownloadUrl = ($Release.assets | Where-Object { $_.name -eq $Asset } | Select-Object -First 1).browser_download_url

if (-not $DownloadUrl) {
    Write-Error "Could not find asset '$Asset' in the latest release."
    exit 1
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

$Dest = Join-Path $InstallDir "$Binary.exe"
Write-Host "Downloading $Asset..."
Invoke-WebRequest -Uri $DownloadUrl -OutFile $Dest -UseBasicParsing

# Add to PATH for this session
$Env:PATH = "$InstallDir;$Env:PATH"

# Persist in user PATH if not already present
$UserPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable(
        "PATH",
        "$InstallDir;$UserPath",
        "User"
    )
    Write-Host "Added $InstallDir to your user PATH (restart your shell to take effect)."
}

Write-Host "Installed: $Dest"
try { & "$Dest" --version } catch {}
