//go:build !windows

package session

import (
	"os/exec"
	"syscall"
)

func applyDesktopProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
