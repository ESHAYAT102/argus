package api

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func copyToClipboard(value string) error {
	switch runtime.GOOS {
	case "darwin":
		return pipeCommand(value, "pbcopy")
	case "windows":
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard -Value $args[0]", value).Run()
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-copy"); err == nil {
				return pipeCommand(value, "wl-copy")
			}
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return pipeCommand(value, "xclip", "-selection", "clipboard")
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return pipeCommand(value, "xsel", "--clipboard", "--input")
		}
	}
	return errors.New("no supported clipboard command found")
}

func pipeCommand(value, name string, arguments ...string) error {
	command := exec.Command(name, arguments...)
	command.Stdin = strings.NewReader(value)
	return command.Run()
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "linux":
		command = exec.Command("xdg-open", target)
	default:
		return errors.New("unsupported operating system")
	}
	return command.Start()
}
