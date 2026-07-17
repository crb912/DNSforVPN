#!/usr/bin/env bash
# Removes dnsforvpn installed by deploy/linux/install.sh.
set -euo pipefail

PREFIX=/opt/dnsforvpn

if [ "$EUID" -ne 0 ]; then
	echo "please run as root (sudo)" >&2
	exit 1
fi

if [ -x "$PREFIX/dnsforvpn" ]; then
	"$PREFIX/dnsforvpn" service stop || true
	"$PREFIX/dnsforvpn" service uninstall || true
fi

rm -f /usr/share/applications/dnsforvpn.desktop
rm -f /usr/share/icons/hicolor/512x512/apps/dnsforvpn.png

# config.toml and data/ are kept unless explicitly requested.
if [ "${1:-}" = "--purge" ]; then
	rm -rf "$PREFIX"
	echo "dnsforvpn removed (including config and cache)."
else
	rm -f "$PREFIX/dnsforvpn"
	echo "dnsforvpn removed; kept $PREFIX/config.toml and data (use --purge to remove)."
fi
