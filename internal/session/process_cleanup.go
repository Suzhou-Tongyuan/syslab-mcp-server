package session

import (
	"os/exec"
	"time"
)

func terminateProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	err := killProcessTree(pid)
	waitCh := make(chan struct{}, 1)
	go func() {
		_, _ = cmd.Process.Wait()
		waitCh <- struct{}{}
	}()
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Release()
	}
	releaseProcessLifetime(pid)
	return err
}
