//go:build !windows

package backend

import "os"

func terminateProcess(process *os.Process) error {
	return process.Signal(os.Interrupt)
}
