#!/usr/bin/env bash
# Creates DNSforVPN.app — a lightweight launcher that opens the web UI.
#
# The DNS service itself runs via launchd:
#   dnsforvpn service install --config /usr/local/etc/dnsforvpn/config.toml
#   dnsforvpn service start
#
# Run this on macOS (uses sips/iconutil for the icon):
#   deploy/macos/make-app.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

APP="DNSforVPN.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

cat > "$APP/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>            <string>DNSforVPN</string>
	<key>CFBundleDisplayName</key>     <string>DNSforVPN</string>
	<key>CFBundleIdentifier</key>      <string>com.dnsforvpn.app</string>
	<key>CFBundleVersion</key>         <string>0.2.0</string>
	<key>CFBundleExecutable</key>      <string>launcher</string>
	<key>CFBundleIconFile</key>        <string>icon</string>
	<key>CFBundlePackageType</key>     <string>APPL</string>
	<key>LSUIElement</key>             <true/>
</dict>
</plist>
EOF

cat > "$APP/Contents/MacOS/launcher" <<'EOF'
#!/bin/bash
open http://127.0.0.1:8080
EOF
chmod +x "$APP/Contents/MacOS/launcher"

if command -v iconutil >/dev/null 2>&1; then
	TMP=$(mktemp -d)
	trap 'rm -rf "$TMP"' EXIT
	ICONSET="$TMP/icon.iconset"
	mkdir -p "$ICONSET"
	sips -z 512 512 frontend/public/icons/icon-512.png --out "$ICONSET/icon_512x512.png" >/dev/null
	sips -z 256 256 frontend/public/icons/icon-512.png --out "$ICONSET/icon_256x256.png" >/dev/null
	sips -z 128 128 frontend/public/icons/icon-512.png --out "$ICONSET/icon_128x128.png" >/dev/null
	iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/icon.icns"
else
	echo "warning: iconutil not found — app will have no icon" >&2
fi

echo "wrote $APP — copy it to /Applications"
