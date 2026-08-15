package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// previewCommand returns the command used to show a captured frame to the user.
// In-terminal viewers win when available, otherwise we fall back to the system
// image viewer.
func previewCommand(path string) *exec.Cmd {
	quoted := shellQuote(path)

	if bin, err := exec.LookPath("chafa"); err == nil {
		script := fmt.Sprintf(
			"clear; %s --size=90x38 %s; printf '\\n  press enter to go back '; read _",
			shellQuote(bin), quoted,
		)
		return exec.Command("sh", "-c", script)
	}
	if bin, err := exec.LookPath("viu"); err == nil {
		script := fmt.Sprintf(
			"clear; %s -h 38 %s; printf '\\n  press enter to go back '; read _",
			shellQuote(bin), quoted,
		)
		return exec.Command("sh", "-c", script)
	}

	switch runtime.GOOS {
	case "darwin":
		// QuickLook blocks until the preview window is closed.
		return exec.Command("sh", "-c", fmt.Sprintf("qlmanage -p %s >/dev/null 2>&1", quoted))
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path)
	default:
		return exec.Command("sh", "-c", fmt.Sprintf("xdg-open %s >/dev/null 2>&1", quoted))
	}
}

// openDirCommand opens a directory in the system file browser.
func openDirCommand(dir string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", dir)
	case "windows":
		return exec.Command("cmd", "/c", "start", "", dir)
	default:
		return exec.Command("sh", "-c", fmt.Sprintf("xdg-open %s >/dev/null 2>&1", shellQuote(dir)))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
