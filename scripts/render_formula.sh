#!/bin/sh
# Fills in the Homebrew formula for a release: the version and the SHA-256 of each archive, from the release's
# checksums.txt. The formula is printed. A third argument, yagi-tailscale, makes the one for the build with the
# built-in Tailscale node (the default is yagi).
#
#   sh scripts/render_formula.sh 2026.10.17 checksums.txt > yagi.rb
#   sh scripts/render_formula.sh 2026.10.17 checksums.txt yagi-tailscale > yagi-tailscale.rb
set -eu

[ $# -eq 2 ] || [ $# -eq 3 ] || {
	echo "usage: sh scripts/render_formula.sh VERSION CHECKSUMS_FILE [yagi|yagi-tailscale]" >&2
	exit 2
}
version=$1
sums=$2
formula=${3:-yagi}
case $formula in
yagi) suffix= ;;
yagi-tailscale) suffix=_tailscale ;;
*)
	echo "render_formula: '$formula' isn't a formula here (yagi or yagi-tailscale)" >&2
	exit 2
	;;
esac
here=$(cd "$(dirname "$0")/.." && pwd)

printf '%s\n' "$version" | grep -Eq '^[0-9]{4}\.[0-9]{1,2}\.[0-9]+$' || {
	echo "render_formula: '$version' isn't a release number" >&2
	exit 1
}

sha_for() {
	file=yagi_${version}_$1$suffix.tar.gz
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

sed -e "s/@VERSION@/$version/g" \
	-e "s/@SHA_DARWIN_ARM64@/$darwin_arm64/" \
	-e "s/@SHA_DARWIN_AMD64@/$darwin_amd64/" \
	-e "s/@SHA_LINUX_ARM64@/$linux_arm64/" \
	-e "s/@SHA_LINUX_AMD64@/$linux_amd64/" \
	"$here/packaging/homebrew/$formula.rb"
