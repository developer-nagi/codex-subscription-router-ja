//go:build windows

package backend

import "os"

// Windows cannot deliver os.Interrupt to another process, so the child is
// stopped directly. The Codex app-server persists through SQLite and survives
// this termination path.
func terminateProcess(process *os.Process) error {
	return process.Kill()
}
