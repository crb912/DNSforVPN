@echo off
rem Installs dnsforvpn: binary + config under %ProgramFiles%\DNSforVPN,
rem Windows service via the binary's own "service install".
rem Usage: right-click -> Run as administrator (from the directory this file is in)
setlocal
set "PREFIX=%ProgramFiles%\DNSforVPN"
set "SRC=%~dp0"

net session >nul 2>&1
if errorlevel 1 (
    echo please run as administrator >&2
    exit /b 1
)

echo ^>^> installing to %PREFIX%
if not exist "%PREFIX%" mkdir "%PREFIX%"
if not exist "%PREFIX%\rules" mkdir "%PREFIX%\rules"
if not exist "%PREFIX%\data" mkdir "%PREFIX%\data"
copy /y "%SRC%dnsforvpn.exe" "%PREFIX%\dnsforvpn.exe" >nul
copy /y "%SRC%uninstall.bat" "%PREFIX%\uninstall.bat" >nul
rem Keep an existing config/rules on reinstall (Web UI edits survive upgrades).
if not exist "%PREFIX%\config.toml" copy "%SRC%config.toml" "%PREFIX%\config.toml" >nul
if not exist "%PREFIX%\rules\gfwlist.txt" copy "%SRC%rules\gfwlist.txt" "%PREFIX%\rules\gfwlist.txt" >nul

echo ^>^> creating desktop shortcut
(
    echo [InternetShortcut]
    echo URL=http://127.0.0.1:8080
) > "%USERPROFILE%\Desktop\DNSforVPN.url"

echo ^>^> registering windows service
"%PREFIX%\dnsforvpn.exe" service install --config "%PREFIX%\config.toml"
"%PREFIX%\dnsforvpn.exe" service start

echo.
echo dnsforvpn installed.
echo   Web UI:  http://127.0.0.1:8080
echo   DNS:     0.0.0.0:5553 - upstream-only; point your system DNS at it.
echo   Config:  %PREFIX%\config.toml
echo   Service: sc query dnsforvpn
echo   Remove:  run %PREFIX%\uninstall.bat as administrator
