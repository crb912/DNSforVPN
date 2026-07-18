#!/bin/sh
# Removes dnsforvpn from OpenWrt: stops + disables the procd service,
# deletes the binary, configuration and cache database.
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "please run as root" >&2
    exit 1
fi

/etc/init.d/dnsforvpn stop 2>/dev/null || true
/etc/init.d/dnsforvpn disable 2>/dev/null || true
rm -f /etc/init.d/dnsforvpn /usr/bin/dnsforvpn
echo "dnsforvpn removed (service, binary, config, cache)."
rm -rf /etc/dnsforvpn
