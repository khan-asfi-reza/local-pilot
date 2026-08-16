//go:build windows

package orchestrator

import (
	"os/exec"
	"strconv"
)

// setProcGroup is a no-op on Windows (process-group semantics differ).
func setProcGroup(cmd *exec.Cmd) {}

// killGroup kills the command and its child tree via taskkill.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	_ = cmd.Process.Kill()
}
