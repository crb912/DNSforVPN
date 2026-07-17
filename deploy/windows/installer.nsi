; DNSforVPN Windows installer — NSIS 3.x
;
; Build:
;   makensis /DBINARY=path\to\dnsforvpn-windows-amd64.exe /DVERSION=0.2.0 deploy\windows\installer.nsi
;
; Model B: DNS runs as a system service (registered during install);
; the Start Menu / Desktop icons just open the web UI in the browser.

!define APPNAME "DNSforVPN"
!ifndef BINARY
	!define BINARY "..\..\dnsforvpn-windows-amd64.exe"
!endif
!ifndef VERSION
	!define VERSION "0.2.0"
!endif
!define WEBURL "http://127.0.0.1:8080"

Name "${APPNAME}"
OutFile "dnsforvpn-${VERSION}-setup.exe"
InstallDir "$PROGRAMFILES64\${APPNAME}"
RequestExecutionLevel admin

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
	SetOutPath "$INSTDIR"

	; Upgrade path: stop/remove the old service before replacing files.
	IfFileExists "$INSTDIR\dnsforvpn.exe" 0 +3
		ExecWait '"$INSTDIR\dnsforvpn.exe" service stop'
		ExecWait '"$INSTDIR\dnsforvpn.exe" service uninstall'

	File "/oname=dnsforvpn.exe" "${BINARY}"
	File "dnsforvpn.ico"

	; Config + rule seed go to %PROGRAMDATA% (writable by the service).
	; Existing files are kept on upgrade.
	SetOutPath "$PROGRAMDATA\DNSforVPN"
	IfFileExists "$PROGRAMDATA\DNSforVPN\config.toml" +2 0
	File "..\..\configs\config.toml"
	SetOutPath "$PROGRAMDATA\DNSforVPN\rules"
	IfFileExists "$PROGRAMDATA\DNSforVPN\rules\gfwlist.txt" +2 0
	File "..\..\configs\rules\gfwlist.txt"

	; Web UI launcher shortcut (.url opens in the default browser).
	WriteIniStr "$INSTDIR\${APPNAME}.url" "InternetShortcut" "URL" "${WEBURL}"
	CreateDirectory "$SMPROGRAMS\${APPNAME}"
	CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${APPNAME}.url" "" "$INSTDIR\dnsforvpn.ico"
	CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${APPNAME}.url" "" "$INSTDIR\dnsforvpn.ico"

	; Register and start the system service.
	ExecWait '"$INSTDIR\dnsforvpn.exe" service install --config "$PROGRAMDATA\DNSforVPN\config.toml"'
	ExecWait '"$INSTDIR\dnsforvpn.exe" service start'

	WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
	ExecWait '"$INSTDIR\dnsforvpn.exe" service stop'
	ExecWait '"$INSTDIR\dnsforvpn.exe" service uninstall'

	Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
	RMDir "$SMPROGRAMS\${APPNAME}"
	Delete "$DESKTOP\${APPNAME}.lnk"

	Delete "$INSTDIR\dnsforvpn.exe"
	Delete "$INSTDIR\dnsforvpn.ico"
	Delete "$INSTDIR\${APPNAME}.url"
	Delete "$INSTDIR\uninstall.exe"
	; %PROGRAMDATA%\DNSforVPN (config/rules/data) is intentionally kept.
	RMDir "$INSTDIR"
SectionEnd
