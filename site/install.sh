#!/bin/sh
# gurl installer: https://gurl.somosingogo.dev
#
#   curl -fsSL https://gurl.somosingogo.dev/install.sh | sh
#
# Environment:
#   GURL_VERSION      release to install, e.g. 0.1.3 (default: latest)
#   GURL_INSTALL_DIR  where to put the binary (default: /usr/local/bin if
#                     writable or sudo is available, otherwise ~/.local/bin)
#
# The script is POSIX sh, so it runs under dash (Debian/Ubuntu), ash (Alpine)
# and macOS sh alike. Everything is wrapped in main() and called on the last
# line: if the download is cut off halfway, a truncated script defines a
# function and never runs it, instead of executing half a command.

set -eu

REPO="CSingogo/gurl"

# Colour only when a person is watching (stdout is a terminal) and NO_COLOR is unset.
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  BLUE=$(printf '\033[38;5;38m'); BOLD=$(printf '\033[1m'); DIM=$(printf '\033[2m')
  RED=$(printf '\033[31m'); OFF=$(printf '\033[0m'); TTY=1
else
  BLUE=""; BOLD=""; DIM=""; RED=""; OFF=""; TTY=0
fi

say() { printf '%s\n' "$*"; }
ok() { printf '%s✓%s %s\n' "$BLUE" "$OFF" "$*"; }      # a finished step
step() { printf '%s→%s %s\n' "$DIM" "$OFF" "$*"; }     # a step starting
die() { printf '%sgurl install:%s %s\n' "$RED" "$OFF" "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

download() { # download <url> <dest> [show-progress]
  if have curl; then
    if [ "${3:-}" = progress ] && [ "$TTY" = 1 ]; then
      curl -fL --progress-bar "$1" -o "$2"
    else
      curl -fsSL "$1" -o "$2"
    fi
  elif have wget; then
    wget -qO "$2" "$1"
  else
    die "need curl or wget to download files"
  fi
}

sha256_of() { # sha256_of <file>: print just the hex digest
  if have sha256sum; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif have shasum; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    die "need sha256sum or shasum to verify the download"
  fi
}

main() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux | darwin) ;;
    *) die "unsupported OS '$os'. On Windows, download the .zip from https://github.com/$REPO/releases" ;;
  esac

  # uname reports x86_64 / aarch64 / arm64; release files use Go's names.
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "unsupported CPU '$(uname -m)'" ;;
  esac

  if [ -n "${GURL_VERSION:-}" ]; then
    base="https://github.com/$REPO/releases/download/v${GURL_VERSION#v}"
  else
    base="https://github.com/$REPO/releases/latest/download"
  fi
  archive="gurl_${os}_${arch}.tar.gz"

  # Human-friendly names for the messages (the release files keep Go's names).
  case "$os-$arch" in
    darwin-arm64) pretty="macOS (Apple Silicon)" ;;
    darwin-amd64) pretty="macOS (Intel)" ;;
    linux-arm64) pretty="Linux (ARM64)" ;;
    linux-amd64) pretty="Linux (x86-64)" ;;
  esac

  say ""
  say "${BOLD}gurl${OFF}${BLUE}.${OFF}  curl, but written in Go"
  say ""
  ok "Detected $pretty"

  # mktemp -d makes a private directory; the trap deletes it however we exit.
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT INT TERM

  step "Downloading gurl for $pretty"
  download "$base/$archive" "$tmp/$archive" progress
  download "$base/checksums.txt" "$tmp/checksums.txt"

  want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)
  [ -n "$want" ] || die "no checksum listed for $archive"
  got=$(sha256_of "$tmp/$archive")
  [ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"
  ok "Verified the sha256 checksum"

  tar -xzf "$tmp/$archive" -C "$tmp" gurl

  sudo=""
  if [ -n "${GURL_INSTALL_DIR:-}" ]; then
    dir="$GURL_INSTALL_DIR"
  elif [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  elif [ "$(id -u)" -ne 0 ] && have sudo; then
    dir=/usr/local/bin
    sudo=sudo
  else
    dir="$HOME/.local/bin"
  fi
  if [ -n "$sudo" ]; then
    step "Installing to $dir (sudo will ask for your password; set GURL_INSTALL_DIR=\$HOME/.local/bin to avoid that)"
  else
    step "Installing to $dir"
  fi
  mkdir -p "$dir" 2>/dev/null || $sudo mkdir -p "$dir"
  $sudo install -m 755 "$tmp/gurl" "$dir/gurl"
  ok "Installed $dir/gurl"

  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say ""
       say "Note: $dir is not on your PATH. Add this to your shell profile:"
       say "  export PATH=\"$dir:\$PATH\"" ;;
  esac
  say ""
  say "${BOLD}$("$dir/gurl" --version)${OFF} is ready. Try it:"
  say "  ${BLUE}\$${OFF} gurl https://gurl.somosingogo.dev/get"
  say ""
}

main "$@"
