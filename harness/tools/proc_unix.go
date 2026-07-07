//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group so the whole tree can
// be signalled at once.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess kills the command's process group.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
