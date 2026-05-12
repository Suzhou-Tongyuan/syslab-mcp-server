//go:build windows

package session

import (
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008

func applyDesktopProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
