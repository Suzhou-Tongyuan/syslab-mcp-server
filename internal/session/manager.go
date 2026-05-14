package session

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syslab-mcp/internal/config"
	"syslab-mcp/internal/discovery"
	"time"
)

const (
	requestPrefix  = "SYSLABMCP-REQ"
	responsePrefix = "SYSLABMCP-RES"
)

type Manager struct {
	cfg    config.Config
	logger *log.Logger

	mu       sync.Mutex
	sessions map[string]*bridgeSession
	nextID   uint64
}

type bridgeSession struct {
	Key        string
	WorkingDir string
	Cmd        *exec.Cmd
	Stdin      io.WriteCloser
	Stdout     *bufio.Reader
	StartedAt  string
	Desktop    *desktopSession
}

type SessionInfo struct {
	Key        string `json:"key"`
	WorkingDir string `json:"working_dir"`
	Status     string `json:"status"`
	PID        int    `json:"pid,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

type BridgeResult struct {
	Stdout    string
	Stderr    string
	Result    string
	ErrorType string
	ErrorMsg  string
	Stack     string
}

func NewManager(cfg config.Config, logger *log.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger, sessions: make(map[string]*bridgeSession)}
}

func (m *Manager) LauncherPath() string {
	return m.cfg.SyslabLauncher
}

func (m *Manager) EnsureRuntimeConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureBridgeRuntimeConfigLocked()
}

func (m *Manager) EnsureStarted(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.usesDesktopMode() {
		_, err := m.ensureDesktopStartedLocked(ctx, "", "")
		return err
	}
	_, err := m.ensureStartedLocked(ctx, "", "")
	return err
}

func (m *Manager) Call(ctx context.Context, method, envPath, cwd, payload string) (BridgeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, _, normalizedCWD := m.resolveRequestContext(envPath, cwd)
	if m.usesDesktopMode() {
		return m.callDesktopLocked(ctx, key, method, normalizedCWD, payload)
	}
	sess, err := m.ensureStartedLocked(ctx, key, "")
	if err != nil {
		return BridgeResult{}, err
	}
	if normalizedCWD != "" {
		sess.WorkingDir = normalizedCWD
	}

	id := fmt.Sprintf("%d", atomic.AddUint64(&m.nextID, 1))
	line := strings.Join([]string{
		requestPrefix,
		id,
		method,
		base64.StdEncoding.EncodeToString([]byte(normalizedCWD)),
		base64.StdEncoding.EncodeToString([]byte(payload)),
	}, "\t") + "\n"

	if _, err := io.WriteString(sess.Stdin, line); err != nil {
		m.resetLocked(sess.Key)
		return BridgeResult{}, fmt.Errorf("write request to syslab bridge: %w", err)
	}

	return m.readResponseLocked(ctx, sess, "bridge call "+method)
}

func (m *Manager) Restart(ctx context.Context, workingDir string) (SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := ""
	if m.usesDesktopMode() {
		return m.restartDesktopLocked(ctx, key, workingDir)
	}
	m.resetLocked(key)
	sess, err := m.ensureStartedLocked(ctx, key, workingDir)
	if err != nil {
		return SessionInfo{}, err
	}
	return sessionInfoFromState(sess), nil
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.sessions))
	for key := range m.sessions {
		keys = append(keys, key)
	}
	for _, key := range keys {
		sess, ok := m.sessions[key]
		if !ok {
			continue
		}
		if sess.Desktop != nil {
			_ = sess.Desktop.Detach()
			delete(m.sessions, key)
			continue
		}
		m.resetLocked(key)
	}
}

func (m *Manager) readResponseLocked(ctx context.Context, sess *bridgeSession, op string) (BridgeResult, error) {
	type readResult struct {
		result BridgeResult
		err    error
	}

	resultCh := make(chan readResult, 1)
	go func() {
		for {
			raw, err := sess.Stdout.ReadString('\n')
			if err != nil {
				resultCh <- readResult{err: fmt.Errorf("read response from syslab bridge: %w", err)}
				return
			}

			raw = strings.TrimRight(raw, "\r\n")
			if !strings.HasPrefix(raw, responsePrefix+"\t") {
				if strings.TrimSpace(raw) != "" {
					m.logger.Printf("ignored bridge stdout: %s", raw)
				}
				continue
			}

			result, ok, err := parseResponse(raw)
			if err != nil {
				resultCh <- readResult{err: err}
				return
			}
			if !ok {
				continue
			}

			if result.ErrorMsg != "" {
				resultCh <- readResult{result: result, err: errors.New(result.ErrorMsg)}
				return
			}
			resultCh <- readResult{result: result}
			return
		}
	}()

	select {
	case <-ctx.Done():
		m.resetLocked(sess.Key)
		return BridgeResult{}, fmt.Errorf("%s canceled: %w", op, ctx.Err())
	case read := <-resultCh:
		if read.err != nil {
			m.resetLocked(sess.Key)
			return BridgeResult{}, read.err
		}
		return read.result, nil
	}
}

func (m *Manager) ensureStartedLocked(ctx context.Context, key string, workingDirOverride string) (*bridgeSession, error) {
	if sess, ok := m.sessions[key]; ok && sess.Cmd != nil {
		return sess, nil
	}

	if err := m.ensureBridgeRuntimeConfigLocked(); err != nil {
		return nil, err
	}

	if _, err := os.Stat(m.cfg.SyslabLauncher); err != nil {
		return nil, fmt.Errorf("syslab launcher not found: %w", err)
	}
	if _, err := os.Stat(m.cfg.BridgeScript); err != nil {
		return nil, fmt.Errorf("bridge script not found: %w", err)
	}

	cmd := buildBridgeCommand(ctx, m.cfg.SyslabLauncher, m.cfg.BridgeScript, key, m.cfg.PkgOffline)
	workingDir := m.workingDirForKey(key, workingDirOverride)
	cmd.Dir = workingDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open syslab stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open syslab stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open syslab stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start syslab bridge: %w", err)
	}
	guardChildProcess(cmd, m.logger)

	sess := &bridgeSession{
		Key:        key,
		WorkingDir: workingDir,
		Cmd:        cmd,
		Stdin:      stdin,
		Stdout:     bufio.NewReader(stdout),
		StartedAt:  time.Now().Format(time.RFC3339),
	}
	m.sessions[key] = sess

	go m.logPipe("bridge-stderr "+key, stderr)

	if err := m.writeRequestLocked(sess, "0", "health", workingDir, ""); err != nil {
		m.resetLocked(sess.Key)
		return nil, fmt.Errorf("syslab bridge health request failed: %w", err)
	}
	if _, err := m.readResponseLocked(ctx, sess, "bridge health check"); err != nil {
		m.resetLocked(sess.Key)
		return nil, fmt.Errorf("syslab bridge health check failed: %w", err)
	}

	return sess, nil
}

func (m *Manager) ensureBridgeRuntimeConfigLocked() error {
	juliaRoot, err := discovery.ResolveJuliaRoot(m.cfg.JuliaRoot, m.cfg.SyslabRoot)
	if err != nil {
		return err
	}
	launcher, err := discovery.ResolveSyslabLauncher(m.cfg.SyslabLauncher, juliaRoot)
	if err != nil {
		return err
	}
	m.cfg.JuliaRoot = juliaRoot
	m.cfg.SyslabLauncher = launcher
	return nil
}

func (m *Manager) writeRequestLocked(sess *bridgeSession, id, method, cwd, payload string) error {
	line := strings.Join([]string{
		requestPrefix,
		id,
		method,
		base64.StdEncoding.EncodeToString([]byte(cwd)),
		base64.StdEncoding.EncodeToString([]byte(payload)),
	}, "\t") + "\n"

	_, err := io.WriteString(sess.Stdin, line)
	return err
}

func (m *Manager) resetLocked(key string) {
	sess, ok := m.sessions[key]
	if !ok {
		return
	}
	if sess.Desktop != nil {
		_ = sess.Desktop.Detach()
		delete(m.sessions, key)
		return
	}
	if sess.Cmd != nil {
		_ = terminateProcess(sess.Cmd)
	}
	delete(m.sessions, key)
}

func (m *Manager) logPipe(name string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 8*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		m.logger.Printf("%s: %s", name, line)
	}
}

func parseResponse(raw string) (BridgeResult, bool, error) {
	parts := strings.Split(raw, "\t")
	if len(parts) != 9 {
		return BridgeResult{}, false, fmt.Errorf("invalid bridge response format")
	}

	decoded := make([]string, 0, 6)
	for _, idx := range []int{3, 4, 5, 6, 7, 8} {
		data, err := base64.StdEncoding.DecodeString(parts[idx])
		if err != nil {
			return BridgeResult{}, false, fmt.Errorf("decode bridge response: %w", err)
		}
		decoded = append(decoded, string(data))
	}

	return BridgeResult{
		Stdout:    decoded[0],
		Stderr:    decoded[1],
		Result:    decoded[2],
		ErrorType: decoded[3],
		ErrorMsg:  decoded[4],
		Stack:     decoded[5],
	}, true, nil
}

func (m *Manager) resolveRequestContext(_ string, cwd string) (string, string, string) {
	return "", "", m.normalizeWorkingDir(cwd)
}

func (m *Manager) normalizeWorkingDir(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	return normalizePath(cwd)
}

func (m *Manager) workingDirForKey(key string, workingDirOverride string) string {
	if strings.TrimSpace(workingDirOverride) != "" {
		return normalizePath(workingDirOverride)
	}
	if key == "" {
		return m.cfg.InitialWorkingFolder
	}
	return key
}

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func sessionInfoFromState(sess *bridgeSession) SessionInfo {
	info := SessionInfo{
		Key:        "global",
		WorkingDir: sess.WorkingDir,
		Status:     "running",
		StartedAt:  sess.StartedAt,
	}
	if sess.Cmd != nil && sess.Cmd.Process != nil {
		info.PID = sess.Cmd.Process.Pid
	}
	return info
}

func buildBridgeCommand(ctx context.Context, launcherPath, bridgeScript, envPath string, pkgOffline bool) *exec.Cmd {
	args := []string{"--startup-file=no"}
	if strings.TrimSpace(envPath) != "" {
		args = append(args, "--project="+envPath)
	}
	args = append(args, bridgeScript)
	env := append([]string{}, os.Environ()...)
	if pkgOffline {
		env = append(env, "JULIA_PKG_OFFLINE=true")
	} else {
		env = append(env, "JULIA_PKG_OFFLINE=false")
	}
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "cmd.exe", append([]string{"/d", "/c", launcherPath}, args...)...)
		cmd.Env = env
		return cmd
	}
	cmd := exec.CommandContext(ctx, launcherPath, args...)
	cmd.Env = env
	return cmd
}

func (m *Manager) usesDesktopMode() bool {
	return strings.EqualFold(strings.TrimSpace(m.cfg.SyslabDisplayMode), "desktop")
}
