#!/bin/sh
# Installs dnsforvpn on OpenWrt: binary to /usr/bin, config to /etc/dnsforvpn,
# procd service to /etc/init.d. Runs ON the router (busybox sh).
# Usage: scp this whole directory to the router, then: sh install.sh
set -eu

SRC=$(cd "$(dirname "$0")" && pwd)
PREFIX=/etc/dnsforvpn

if [ "$(id -u)" -ne 0 ]; then
    echo "please run as root" >&2
    exit 1
fi

echo ">> installing"
mkdir -p "$PREFIX/rules" "$PREFIX/data"
cp "$SRC/dnsforvpn" /usr/bin/dnsforvpn
chmod 0755 /usr/bin/dnsforvpn
cp "$SRC/uninstall.sh" "$PREFIX/uninstall.sh"
chmod 0755 "$PREFIX/uninstall.sh"
# Keep an existing config/rules on reinstall (Web UI edits survive upgrades).
if [ ! -f "$PREFIX/config.toml" ]; then
    cp "$SRC/config.toml" "$PREFIX/config.toml"
    chmod 0644 "$PREFIX/config.toml"
fi
if [ ! -f "$PREFIX/rules/gfwlist.txt" ]; then
    cp "$SRC/rules/gfwlist.txt" "$PREFIX/rules/gfwlist.txt"
    chmod 0644 "$PREFIX/rules/gfwlist.txt"
fi
cp "$SRC/dnsforvpn.init" /etc/init.d/dnsforvpn
chmod 0755 /etc/init.d/dnsforvpn

/etc/init.d/dnsforvpn enable
/etc/init.d/dnsforvpn start

cat <<EOF

dnsforvpn installed.
  Web UI:  http://<router-ip>:8080 — SET [web] password in $PREFIX/config.toml FIRST!
  DNS:     127.0.0.1:5553 — forward dnsmasq with:
             uci add_list dhcp.@dnsmasq[0].server='127.0.0.1#5553'
             uci set dhcp.@dnsmasq[0].noresolv='1'
             uci commit dhcp && /etc/init.d/dnsmasq restart
  Remove:  sh $PREFIX/uninstall.sh
EOF
