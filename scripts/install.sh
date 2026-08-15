#!/usr/bin/env bash
# vopt installer: downloads the right prebuilt binary from the latest GitHub
# release, verifies its checksum and puts it on your PATH. No Go required.
# Works on macOS, Linux and Git Bash on Windows.
#
#   curl -fsSL https://raw.githubusercontent.com/melvicsosa/video-optimizer/main/scripts/install.sh | bash
#
# Options via environment variables:
#   VOPT_VERSION      tag to install (default: latest release, e.g. v0.1.2)
#   VOPT_INSTALL_DIR  target directory (default: /usr/local/bin if writable,
#                     otherwise ~/.local/bin; ~/bin on Windows)
set -euo pipefail

REPO="melvicsosa/video-optimizer"

say()  { printf '%s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

extract_zip() {
  # Git Bash usually ships without unzip and its GNU tar cannot read zip
  # files, so fall through: unzip, then the libarchive tar.exe Windows 10+
  # ships, then PowerShell.
  if command -v unzip >/dev/null; then
    unzip -o -q "$tmp/$archive" "$bin" -d "$tmp" && return
  fi
  if [ -x /c/Windows/System32/tar.exe ]; then
    /c/Windows/System32/tar.exe -xf "$(cygpath -w "$tmp/$archive")" \
      -C "$(cygpath -w "$tmp")" && return
  fi
  if command -v powershell.exe >/dev/null; then
    powershell.exe -NoProfile -Command \
      "Expand-Archive -Force '$(cygpath -w "$tmp/$archive")' '$(cygpath -w "$tmp")'" && return
  fi
  fail "could not extract $archive: no unzip, Windows tar.exe or powershell.exe available"
}

main() {
  # --- platform -------------------------------------------------------------
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  ext="tar.gz"
  bin="vopt"
  case "$os" in
    darwin|linux) ;;
    mingw*|msys*|cygwin*)
      # Git Bash and friends: install the Windows binary.
      os="windows"; ext="zip"; bin="vopt.exe" ;;
    *) fail "unsupported OS: $os. Grab a build from https://github.com/$REPO/releases/latest" ;;
  esac

  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) fail "unsupported architecture: $arch" ;;
  esac

  # --- version --------------------------------------------------------------
  version="${VOPT_VERSION:-}"
  if [ -z "$version" ]; then
    version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
      grep -m1 '"tag_name"' | cut -d '"' -f 4)
    [ -n "$version" ] || fail "could not resolve the latest release tag"
  fi
  bare=${version#v}

  # --- download and verify --------------------------------------------------
  archive="vopt_${bare}_${os}_${arch}.${ext}"
  base="https://github.com/$REPO/releases/download/$version"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  say "downloading vopt $version ($os/$arch)..."
  curl -fsSL -o "$tmp/$archive" "$base/$archive" ||
    fail "download failed: $base/$archive"
  curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" ||
    fail "download failed: $base/checksums.txt"

  (
    cd "$tmp"
    grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1 ||
      grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null ||
      fail "checksum verification failed for $archive"
  )

  if [ "$ext" = "zip" ]; then
    extract_zip
  else
    tar -xzf "$tmp/$archive" -C "$tmp" vopt
  fi
  [ -f "$tmp/$bin" ] || fail "extraction finished but $bin is missing"

  # --- install --------------------------------------------------------------
  dir="${VOPT_INSTALL_DIR:-}"
  if [ -z "$dir" ]; then
    if [ "$os" = "windows" ]; then
      dir="$HOME/bin" # Git Bash puts ~/bin on the PATH when it exists
    elif [ -w /usr/local/bin ]; then
      dir=/usr/local/bin
    else
      dir="$HOME/.local/bin"
    fi
  fi
  mkdir -p "$dir"
  install -m 0755 "$tmp/$bin" "$dir/$bin"
  say "installed $("$dir/$bin" -version) to $dir/$bin"

  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say "note: $dir is not on your PATH. Add it with:"
       say "  export PATH=\"$dir:\$PATH\"" ;;
  esac

  # --- runtime dependency ---------------------------------------------------
  if ! command -v ffmpeg >/dev/null; then
    if [ "$os" = "windows" ]; then
      say "note: vopt needs ffmpeg at runtime. Install it with: winget install ffmpeg"
    else
      say "note: vopt needs ffmpeg at runtime. Install it with: brew install ffmpeg"
    fi
  fi
}

main "$@"
