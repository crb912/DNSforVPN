#!/bin/sh
# Installs dnsforvpn: binary + config under /opt/dnsforvpn, systemd service
# via the binary's own `service install`.
# Usage: sudo ./install.sh   (from the directory this script is in)
set -eu

PREFIX=/opt/dnsforvpn
SRC=$(cd "$(dirname "$0")" && pwd)

if [ "$(id -u)" -ne 0 ]; then
    echo "please run as root (sudo)" >&2
    exit 1
fi

echo ">> installing to $PREFIX"
install -d "$PREFIX" "$PREFIX/rules" "$PREFIX/data"
install -m 0755 "$SRC/dnsforvpn" "$PREFIX/dnsforvpn"
install -m 0755 "$SRC/uninstall.sh" "$PREFIX/uninstall.sh"
# Keep an existing config/rules on reinstall (Web UI edits survive upgrades).
if [ ! -f "$PREFIX/config.toml" ]; then
    install -m 0644 "$SRC/config.toml" "$PREFIX/config.toml"
fi
if [ ! -f "$PREFIX/rules/gfwlist.txt" ]; then
    install -m 0644 "$SRC/rules/gfwlist.txt" "$PREFIX/rules/gfwlist.txt"
fi

echo ">> installing app-menu launcher"
install -Dm 0644 "$SRC/icon.png" /usr/share/icons/hicolor/512x512/apps/dnsforvpn.png
cat > /usr/share/applications/dnsforvpn.desktop <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=DNSforVPN
Comment=DoH proxy with GFWList routing and a web UI
Exec=xdg-open http://127.0.0.1:8080
Icon=dnsforvpn
Terminal=false
Categories=Network;
DESKTOP

echo ">> registering systemd service"
"$PREFIX/dnsforvpn" service install --config "$PREFIX/config.toml"
"$PREFIX/dnsforvpn" service start

cat <<EOF

dnsforvpn installed.
  Web UI:  http://127.0.0.1:8080
  DNS:     0.0.0.0:5553 — upstream-only; point your system/dnsmasq DNS at it.
  Config:  $PREFIX/config.toml
  Service: systemctl status dnsforvpn
  Remove:  sudo $PREFIX/uninstall.sh
EOF
