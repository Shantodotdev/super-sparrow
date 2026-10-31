#!/usr/bin/env bash
# Prints the path of a Doppler CLI, installing the pinned release when the machine has none. A CLI
# that is already installed is used as it is, so a developer's own installation is never replaced.
# The release is verified against the checksum below before it is installed.
# Usage: install-doppler.sh
set -euo pipefail

readonly VERSION="3.77.1"
readonly SHA256_AMD64="001f89a42629f9cb827cd86683c4c2dbf87c6299a8d6130643b98044bf5ed80a"
readonly SHA256_ARM64="a7f4be5b30295921dfe1cdc34f905cd017210ec3264a47d46e315ab09cb03e1c"

if command -v doppler >/dev/null; then
  command -v doppler
  exit 0
fi

case "$(uname -m)" in
  x86_64)
    arch=amd64
    expected="$SHA256_AMD64"
    ;;
  aarch64 | arm64)
    arch=arm64
    expected="$SHA256_ARM64"
    ;;
  *)
    echo "No pinned Doppler CLI for $(uname -m)" >&2
    exit 1
    ;;
esac

install_dir="$HOME/.local/bin"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

archive="doppler_${VERSION}_linux_${arch}.tar.gz"
curl --fail --silent --show-error --location --retry 3 --max-time 120 \
  --output "$work/$archive" \
  "https://github.com/DopplerHQ/cli/releases/download/${VERSION}/${archive}"

if [ "$(sha256sum "$work/$archive" | cut -d' ' -f1)" != "$expected" ]; then
  echo "The downloaded Doppler CLI does not match its pinned checksum" >&2
  exit 1
fi

tar -xzf "$work/$archive" -C "$work" doppler
install -m 755 -D "$work/doppler" "$install_dir/doppler"
echo "$install_dir/doppler"
