#!/bin/sh
# Installs dnsforvpn: binary + config under /usr/local/dnsforvpn, launchd
# service via the binary's own `service install`.
# Usage: sudo ./install.sh   (from the directory this script is in)
set -eu

PREFIX=/usr/local/dnsforvpn
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

echo ">> creating /Applications/DNSforVPN.app launcher"
APP=/Applications/DNSforVPN.app
mkdir -p "$APP/Contents/MacOS"
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>DNSforVPN</string>
    <key>CFBundleDisplayName</key><string>DNSforVPN</string>
    <key>CFBundleIdentifier</key><string>dev.dnsforvpn.ui</string>
    <key>CFBundleVersion</key><string>1.0</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleExecutable</key><string>launch</string>
</dict>
</plist>
PLIST
cat > "$APP/Contents/MacOS/launch" <<'LAUNCH'
#!/bin/sh
open http://127.0.0.1:8080
LAUNCH
chmod 0755 "$APP/Contents/MacOS/launch"

echo ">> registering launchd service"
"$PREFIX/dnsforvpn" service install --config "$PREFIX/config.toml"
"$PREFIX/dnsforvpn" service start

cat <<EOF

dnsforvpn installed.
  Web UI:  http://127.0.0.1:8080
  DNS:     0.0.0.0:5553 — upstream-only; point your system DNS at it.
  Config:  $PREFIX/config.toml
  Service: $PREFIX/dnsforvpn service status
  Remove:  sudo $PREFIX/uninstall.sh
EOF
