#!/usr/bin/env bash
# Cross-compiles dnsforvpn for a router target and assembles an .ipk
# package. No OpenWrt SDK or binutils required (pure Go toolchain).
#
# Usage: deploy/openwrt/build-ipk.sh <target> [version]
#   target:  arm64 | mipsle | x86_64
#   version: defaults to 0.2.0
#
# Output: dnsforvpn_<version>_<ipk-arch>.ipk in the repo root.
set -euo pipefail
cd "$(dirname "$0")/../.."

TARGET=${1:?"usage: build-ipk.sh <arm64|mipsle|x86_64> [version]"}
VERSION=${2:-0.2.0}

case "$TARGET" in
	arm64)  GOARCH=arm64;  MIPSVAR=;           IPKARCH=aarch64_generic ;;
	mipsle) GOARCH=mipsle; MIPSVAR=softfloat;  IPKARCH=mipsel_24kc ;;
	x86_64) GOARCH=amd64;  MIPSVAR=;           IPKARCH=x86_64 ;;
	*) echo "unknown target '$TARGET' (want arm64|mipsle|x86_64)" >&2; exit 2 ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo ">> building linux/$GOARCH ${MIPSVAR:+($MIPSVAR)}"
env GOOS=linux GOARCH="$GOARCH" ${MIPSVAR:+GOMIPS=$MIPSVAR} CGO_ENABLED=0 \
	go build -ldflags="-s -w" -o "$TMP/dnsforvpn" ./cmd/dnsforvpn

OUT="dnsforvpn_${VERSION}_${IPKARCH}.ipk"
echo ">> assembling $OUT"
go run ./tools/mkipk -binary "$TMP/dnsforvpn" -arch "$IPKARCH" -version "$VERSION" -out "$OUT"
ls -la "$OUT"
