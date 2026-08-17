//go:build !windows

package tools

import (
	"context"
	"os/exec"
	"syscall"
)

// newShellCmd builds a command run through the system shell. It runs in its own
// process group and, on context cancellation (timeout), kills the WHOLE group —
// so a command that backgrounds a server (`node dist/index.js &`) can't leave an
// orphan holding the output pipe and hanging the run.
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	return cmd
}
