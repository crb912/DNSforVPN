// Package browser opens a URL in the user's default browser,
// using the OS-native mechanism. No external dependencies.
package browser

import (
	"os/exec"
	"runtime"
)

// Open launches the system default browser at url. It returns once the
// launcher process has been started; failure is non-fatal for callers
// (the URL can always be typed manually).
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 url.dll avoids cmd.exe quoting issues with & in URLs.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, freebsd, ...
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
