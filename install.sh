#!/bin/sh
# Installs syscope on Linux or macOS from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/benfo/syscope/master/install.sh | sh
#
# Environment:
#   SYSCOPE_VERSION      release tag to install (default: the newest, e.g. v0.1.0)
#   SYSCOPE_INSTALL_DIR  where to put the executable (default: ~/.local/bin)
#
# Run it again to upgrade. To uninstall, delete the executable.
set -eu

repo=benfo/syscope
install_dir=${SYSCOPE_INSTALL_DIR:-$HOME/.local/bin}

fail() {
	echo "syscope install: $*" >&2
	exit 1
}

# fetch prints a URL's body, or saves it to a file when given one.
fetch() {
	if command -v curl >/dev/null 2>&1; then
		if [ $# -eq 2 ]; then curl -fsSL -o "$2" "$1"; else curl -fsSL "$1"; fi
	elif command -v wget >/dev/null 2>&1; then
		if [ $# -eq 2 ]; then wget -qO "$2" "$1"; else wget -qO- "$1"; fi
	else
		fail "needs curl or wget"
	fi
}

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported OS $(uname -s); on Windows use install.ps1" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported CPU $(uname -m)" ;;
esac

# The Linux build loads the NVIDIA driver library at run time, so it needs
# glibc's loader. musl systems (Alpine) can add it with gcompat.
if [ "$os" = linux ]; then
	case $arch in
	amd64) loader=/lib64/ld-linux-x86-64.so.2 ;;
	arm64) loader=/lib/ld-linux-aarch64.so.1 ;;
	esac
	[ -e "$loader" ] || fail "syscope needs glibc ($loader is missing); on Alpine run: apk add gcompat"
fi

# The newest full release, or the newest pre-release while there are only
# pre-releases (GitHub's "latest" skips those).
tag_of() { sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1; }
version=${SYSCOPE_VERSION:-}
if [ -z "$version" ]; then
	version=$(fetch "https://api.github.com/repos/$repo/releases/latest" 2>/dev/null | tag_of) || true
fi
if [ -z "$version" ]; then
	version=$(fetch "https://api.github.com/repos/$repo/releases?per_page=1" | tag_of)
fi
[ -n "$version" ] || fail "couldn't find a release of $repo"

archive="syscope_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading syscope $version for $os/$arch..."
fetch "$base/$archive" "$tmp/$archive" || fail "couldn't download $base/$archive"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "couldn't download the checksums"

# Lines are "<hash>  <file>", or "<hash> *<file>" in binary mode.
want=$(grep -E " \*?$archive\$" "$tmp/SHA256SUMS" | cut -d ' ' -f 1)
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
else
	got=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
fi
[ -n "$want" ] && [ "$want" = "$got" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$install_dir"
# Replace by renaming, so upgrading works while syscope is running.
mv "$tmp/syscope" "$install_dir/syscope.new"
mv -f "$install_dir/syscope.new" "$install_dir/syscope"

echo "Installed $("$install_dir/syscope" --version) to $install_dir/syscope"
case ":$PATH:" in
*":$install_dir:"*) echo "Run: syscope" ;;
*)
	echo
	echo "$install_dir isn't on your PATH. Add this line to your shell's startup file"
	echo "(~/.bashrc, ~/.zshrc, ...), then open a new terminal:"
	echo
	echo "  export PATH=\"$install_dir:\$PATH\""
	;;
esac
