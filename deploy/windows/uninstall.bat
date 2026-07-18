@echo off
rem Removes dnsforvpn: stops + unregisters the service, deletes the binary,
rem configuration and cache database.
setlocal
set "PREFIX=%ProgramFiles%\DNSforVPN"

net session >nul 2>&1
if errorlevel 1 (
    echo please run as administrator >&2
    exit /b 1
)

if exist "%PREFIX%\dnsforvpn.exe" (
    "%PREFIX%\dnsforvpn.exe" service stop >nul 2>&1
    "%PREFIX%\dnsforvpn.exe" service uninstall >nul 2>&1
)

del /q "%USERPROFILE%\Desktop\DNSforVPN.url" >nul 2>&1

echo dnsforvpn removed (service, binary, config, cache, shortcut).
rmdir /s /q "%PREFIX%"
