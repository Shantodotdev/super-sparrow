//go:build windows

package cli

import (
	"os"
	"os/exec"
)

// terminateProcess stops a child process. Windows has no SIGTERM, so the process is killed.
func terminateProcess(process *os.Process) error {
	return process.Kill()
}

// exitStatus returns the status this program should exit with for a finished child.
func exitStatus(exit *exec.ExitError) int {
	if code := exit.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
