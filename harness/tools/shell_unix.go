//go:build !windows

package tools

import (
	"context"
	"os/exec"
)

// newShellCmd builds a command run through the system shell.
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
