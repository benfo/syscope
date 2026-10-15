#!/usr/bin/env bash
# Builds syscope for every supported OS/CPU into dist/: a .zip for Windows,
# a .tar.gz elsewhere, plus SHA256SUMS. Usage: scripts/release.sh v0.1.0
set -euo pipefail

version=${1:?usage: scripts/release.sh <version, e.g. v0.1.0>}
targets="windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64"

cd "$(dirname "$0")/.."
rm -rf dist && mkdir -p dist

# zipdir writes a .zip without needing Info-ZIP: Windows and macOS ship
# bsdtar, which can.
zipdir() {
	local out=$1 dir=$2
	if command -v zip >/dev/null; then
		(cd "$dir" && zip -q -r "../$(basename "$out")" .)
	elif command -v bsdtar >/dev/null; then
		bsdtar -a -cf "$out" -C "$dir" .
	else
		/c/Windows/System32/tar.exe -a -cf "$out" -C "$dir" .
	fi
}

# targz archives one executable with a neutral owner (never the build
# machine's account) and the executable bit set, which Windows file systems
# can't record themselves.
targz() {
	local out=$1 dir=$2 file=$3
	if tar --version 2>/dev/null | grep -q GNU; then
		tar -czf "$out" -C "$dir" --owner=0 --group=0 --numeric-owner --mode=755 "$file"
	else # bsdtar (macOS)
		chmod 755 "$dir/$file"
		tar -czf "$out" -C "$dir" --uid 0 --gid 0 --uname root --gname root "$file"
	fi
}

for t in $targets; do
	os=${t%/*} arch=${t#*/}
	name="syscope_${version#v}_${os}_${arch}"
	exe=syscope
	[ "$os" = windows ] && exe=syscope.exe
	mkdir -p "dist/$name"
	# -trimpath keeps the build machine's paths (and user name) out of the binary.
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X github.com/benfo/syscope/internal/cli.version=$version" \
		-o "dist/$name/$exe" ./cmd/syscope
	if [ "$os" = windows ]; then
		zipdir "dist/$name.zip" "dist/$name"
	else
		targz "dist/$name.tar.gz" "dist/$name" "$exe"
	fi
	rm -rf "dist/$name"
	echo "built $name"
done

(cd dist && sha256sum syscope_* > SHA256SUMS)
ls -l dist
