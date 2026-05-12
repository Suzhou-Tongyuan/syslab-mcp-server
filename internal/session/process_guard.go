package session

import (
	"log"
	"os/exec"
)

func guardChildProcess(cmd *exec.Cmd, logger *log.Logger) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := attachProcessLifetime(cmd.Process.Pid); err != nil && logger != nil {
		logger.Printf("attach child process lifetime failed for pid=%d: %v", cmd.Process.Pid, err)
	}
}
