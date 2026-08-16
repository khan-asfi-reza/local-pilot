//go:build !windows

package orchestrator

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group so the whole tree
// (npm → ts-node → node) can be killed together after a boot check.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills the command's process group.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
