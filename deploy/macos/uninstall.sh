#!/bin/sh
# Removes dnsforvpn: stops + unregisters the service, deletes the binary,
# configuration and cache database.
set -eu

PREFIX=/usr/local/dnsforvpn

if [ "$(id -u)" -ne 0 ]; then
    echo "please run as root (sudo)" >&2
    exit 1
fi

if [ -x "$PREFIX/dnsforvpn" ]; then
    "$PREFIX/dnsforvpn" service stop || true
    "$PREFIX/dnsforvpn" service uninstall || true
fi

rm -rf /Applications/DNSforVPN.app
rm -rf "$PREFIX"
echo "dnsforvpn removed (service, binary, config, cache, launcher)."
