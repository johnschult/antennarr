#!/bin/sh
# Fills in the Homebrew formula for a release: the version and the SHA-256 of each archive, from the release's
# checksums.txt. The formula is printed.
#
#   sh scripts/render_formula.sh 2026.10.17 checksums.txt > yagi.rb
set -eu

[ $# -eq 2 ] || {
	echo "usage: sh scripts/render_formula.sh VERSION CHECKSUMS_FILE" >&2
	exit 2
}
version=$1
sums=$2
here=$(cd "$(dirname "$0")/.." && pwd)

printf '%s\n' "$version" | grep -Eq '^[0-9]{4}\.[0-9]{1,2}\.[0-9]+$' || {
	echo "render_formula: '$version' isn't a release number" >&2
	exit 1
}

sha_for() {
	file=yagi_${version}_$1.tar.gz
	sha=$(awk -v f="$file" '$2 == f { print $1; exit }' "$sums")
	printf '%s\n' "$sha" | grep -Eq '^[0-9a-f]{64}$' || {
		echo "render_formula: no SHA-256 for $file in $sums" >&2
		exit 1
	}
	printf '%s' "$sha"
}

darwin_arm64=$(sha_for darwin_arm64)
darwin_amd64=$(sha_for darwin_amd64)
linux_arm64=$(sha_for linux_arm64)
linux_amd64=$(sha_for linux_amd64)

sed -e "s/@VERSION@/$version/" \
	-e "s/@SHA_DARWIN_ARM64@/$darwin_arm64/" \
	-e "s/@SHA_DARWIN_AMD64@/$darwin_amd64/" \
	-e "s/@SHA_LINUX_ARM64@/$linux_arm64/" \
	-e "s/@SHA_LINUX_AMD64@/$linux_amd64/" \
	"$here/packaging/homebrew/yagi.rb"
