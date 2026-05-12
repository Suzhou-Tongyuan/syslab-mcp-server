//go:build !windows

package session

func attachProcessLifetime(pid int) error {
	return nil
}

func releaseProcessLifetime(pid int) {}
