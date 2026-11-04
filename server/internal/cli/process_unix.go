//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// terminateProcess asks a child process to stop, the way a terminal or service manager would.
func terminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

// exitStatus returns the status this program should exit with for a finished child: its own exit
// code, or 128 plus the signal number when a signal ended it, as shells report.
func exitStatus(exit *exec.ExitError) int {
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	if code := exit.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
