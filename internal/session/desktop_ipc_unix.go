//go:build !windows

package session

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type unixDesktopAcceptor struct {
	path     string
	listener *net.UnixListener
}

func newDesktopAcceptor() (desktopAcceptor, error) {
	name := fmt.Sprintf("SyslabTestAPI-%d-%d.sock", os.Getpid(), time.Now().UnixNano())
	path := filepath.Join(os.TempDir(), name)
	_ = os.Remove(path)
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	return &unixDesktopAcceptor{path: path, listener: ln}, nil
}

func (a *unixDesktopAcceptor) Endpoint() string { return a.path }

func (a *unixDesktopAcceptor) Accept(ctx context.Context) (desktopDeadlineConn, error) {
	type result struct {
		conn *net.UnixConn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := a.listener.AcceptUnix()
		ch <- result{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = a.Close()
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.conn, nil
	}
}

func (a *unixDesktopAcceptor) Close() error {
	if a.listener != nil {
		_ = a.listener.Close()
	}
	if a.path != "" {
		_ = os.Remove(a.path)
	}
	return nil
}

func connectDesktopEndpoint(ctx context.Context, endpoint string) (desktopDeadlineConn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", endpoint)
	if err != nil {
		return nil, err
	}
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected unix connection type %T", conn)
	}
	return unixConn, nil
}
