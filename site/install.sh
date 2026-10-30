#!/bin/sh
# gurl installer: https://gurl.somosingogo.dev
#
#   curl -fsSL https://gurl.somosingogo.dev/install.sh | sh
#
# Environment:
#   GURL_VERSION      release to install, e.g. 0.1.0 (default: latest)
#   GURL_INSTALL_DIR  where to put the binary (default: /usr/local/bin if
#                     writable or sudo is available, otherwise ~/.local/bin)
#
# The script is POSIX sh, so it runs under dash (Debian/Ubuntu), ash (Alpine)
# and macOS sh alike. Everything is wrapped in main() and called on the last
# line: if the download is cut off halfway, a truncated script defines a
# function and never runs it, instead of executing half a command.

set -eu

REPO="CSingogo/gurl"

say() { printf '%s\n' "$*"; }
die() { printf 'gurl install: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

download() { # download <url> <dest>
  if have curl; then
    curl -fsSL "$1" -o "$2"
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

  # mktemp -d makes a private directory; the trap deletes it however we exit.
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT INT TERM

  say "Downloading $archive ..."
  download "$base/$archive" "$tmp/$archive"
  download "$base/checksums.txt" "$tmp/checksums.txt"

  want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)
  [ -n "$want" ] || die "no checksum listed for $archive"
  got=$(sha256_of "$tmp/$archive")
  [ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"
  say "Checksum OK."

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
  mkdir -p "$dir" 2>/dev/null || $sudo mkdir -p "$dir"
  $sudo install -m 755 "$tmp/gurl" "$dir/gurl"

  say "Installed gurl to $dir/gurl"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say "Note: $dir is not on your PATH. Add this to your shell profile:"
       say "  export PATH=\"$dir:\$PATH\"" ;;
  esac
  "$dir/gurl" --version
}

main "$@"
