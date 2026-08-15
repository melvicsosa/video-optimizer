# vopt installer for Windows PowerShell: downloads the right prebuilt binary
# from the latest GitHub release, verifies its checksum and puts it on your
# PATH. No Go required.
#
#   irm https://raw.githubusercontent.com/melvicsosa/video-optimizer/main/scripts/install.ps1 | iex
#
# Options via environment variables:
#   VOPT_VERSION      tag to install (default: latest release, e.g. v0.1.2)
#   VOPT_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\vopt)
$ErrorActionPreference = "Stop"

$repo = "melvicsosa/video-optimizer"

# --- platform ---------------------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# --- version ----------------------------------------------------------------
$version = $env:VOPT_VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
    if (-not $version) { throw "could not resolve the latest release tag" }
}
$bare = $version.TrimStart("v")

# --- download and verify ----------------------------------------------------
$archive = "vopt_${bare}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "vopt-install-$([System.Guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "downloading vopt $version (windows/$arch)..."
    Invoke-WebRequest -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive)
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    $expected = (Select-String -Path (Join-Path $tmp "checksums.txt") -Pattern ([regex]::Escape($archive))).Line.Split(" ")[0]
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($expected -ne $actual) { throw "checksum verification failed for $archive" }

    Expand-Archive -Force (Join-Path $tmp $archive) $tmp

    # --- install ------------------------------------------------------------
    $dir = $env:VOPT_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA "Programs\vopt" }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item -Force (Join-Path $tmp "vopt.exe") (Join-Path $dir "vopt.exe")

    $installed = & (Join-Path $dir "vopt.exe") -version
    Write-Host "installed $installed to $dir\vopt.exe"

    # Add the install dir to the user PATH when it is not there yet.
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ";") -notcontains $dir) {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
        Write-Host "added $dir to your user PATH. Open a new terminal to pick it up."
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# --- runtime dependency -----------------------------------------------------
if (-not (Get-Command ffmpeg -ErrorAction SilentlyContinue)) {
    Write-Host "note: vopt needs ffmpeg at runtime. Install it with: winget install ffmpeg"
}
