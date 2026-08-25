// Command launcher starts the copied ChatGPT.exe with an isolated Chromium
// profile before Electron's main process runs. It replaces the macOS native
// launcher and is built with -H=windowsgui so no console window appears.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	targetExecutable   = "ChatGPT.exe"
	desktopProfileName = "Codex Subscription Router"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Codex Subscription Router launcher: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	launcher, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve launcher path: %w", err)
	}
	target := filepath.Join(filepath.Dir(launcher), targetExecutable)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("find %s: %w", targetExecutable, err)
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return errors.New("APPDATA is not set")
	}
	profile := "--user-data-dir=" + filepath.Join(appData, desktopProfileName)

	command := exec.Command(target, append([]string{profile}, os.Args[1:]...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		return err
	}
	return nil
}
