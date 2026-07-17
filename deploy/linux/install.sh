#!/usr/bin/env bash
# Installs dnsforvpn on a Linux desktop/server:
#   - binary + config under /opt/dnsforvpn
#   - systemd service via the binary's own `service install` (Model B)
#   - app-menu launcher that opens the web UI
#
# Usage: sudo deploy/linux/install.sh <path-to-dnsforvpn-binary>
set -euo pipefail
cd "$(dirname "$0")/../.."

BIN=${1:?"usage: sudo deploy/linux/install.sh <path-to-dnsforvpn-binary>"}
PREFIX=/opt/dnsforvpn

if [ "$EUID" -ne 0 ]; then
	echo "please run as root (sudo)" >&2
	exit 1
fi

echo ">> installing to $PREFIX"
install -d "$PREFIX" "$PREFIX/rules" "$PREFIX/data"
install -m 0755 "$BIN" "$PREFIX/dnsforvpn"
if [ ! -f "$PREFIX/config.toml" ]; then
	install -m 0644 configs/config.toml "$PREFIX/config.toml"
fi
# Rule seed: install only if absent (the router refreshes it in place).
if [ ! -f "$PREFIX/rules/gfwlist.txt" ]; then
	install -m 0644 configs/rules/gfwlist.txt "$PREFIX/rules/gfwlist.txt"
fi

echo ">> installing icon + launcher"
install -Dm 0644 frontend/public/icons/icon-512.png \
	/usr/share/icons/hicolor/512x512/apps/dnsforvpn.png
install -Dm 0644 deploy/linux/dnsforvpn.desktop \
	/usr/share/applications/dnsforvpn.desktop

echo ">> registering systemd service"
"$PREFIX/dnsforvpn" service install --config "$PREFIX/config.toml"
"$PREFIX/dnsforvpn" service start

cat <<EOF

dnsforvpn installed.
  Web UI:  http://127.0.0.1:8080
  Config:  $PREFIX/config.toml
  Service: $PREFIX/dnsforvpn service {status|stop|start|restart|uninstall}
           (or systemctl status dnsforvpn)
EOF
