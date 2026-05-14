package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syslab-mcp/internal/config"
	"syslab-mcp/internal/syslabenv"
	"time"
)

const (
	desktopPipeEnvVar          = "SYSLAB_TEST_API_PIPE_NAME"
	desktopAttachPipeEnvVar    = "SYSLAB_API_PIPE"
	desktopReadyCommand        = "syslab.hasStarted"
	desktopStartREPLCommand    = "language-julia.startREPL"
	desktopStopREPLCommand     = "language-julia.stopREPL"
	desktopEvalCommand         = "language-julia.executeInREPL"
	desktopTerminalDataCommand = "syslab.action.getAllTerminalData"
	desktopOpenFolderCommand   = "syslab.action.openFolder"
	desktopOpenFileCommand     = "syslab.action.openFile"
	desktopExecFileCommand     = "syslab.executeActiveFile"
	desktopExecFileCommand2    = "syslab.action.executeActiveFile"
)

type desktopDeadlineConn interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
}

type desktopAcceptor interface {
	Endpoint() string
	Accept(context.Context) (desktopDeadlineConn, error)
	Close() error
}

type desktopMessage struct {
	Command string `json:"command"`
	Args    any    `json:"args,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

type desktopSession struct {
	cfg         config.Config
	logger      *log.Logger
	diagLogger  *log.Logger
	diagFile    *os.File
	workingDir  string
	cmd         *exec.Cmd
	conn        desktopDeadlineConn
	codec       *desktopJSONCodec
	replStarted bool
}

type desktopEnvStatus struct {
	Display            string
	WaylandDisplay     string
	XAuthority         string
	DBusSessionBusAddr string
	XDGSessionType     string
}

func (m *Manager) ensureDesktopStartedLocked(ctx context.Context, key string, workingDirOverride string) (*bridgeSession, error) {
	if sess, ok := m.sessions[key]; ok && sess.Desktop != nil {
		return sess, nil
	}

	wd := m.workingDirForKey(key, workingDirOverride)

	if endpoint := strings.TrimSpace(os.Getenv(desktopAttachPipeEnvVar)); endpoint != "" {
		ds, err := attachDesktopSession(ctx, m.cfg, wd, endpoint, m.logger)
		if err == nil {
			return m.storeDesktopSessionLocked(key, wd, ds), nil
		}
		if m.logger != nil {
			m.logger.Printf("attach to existing Syslab desktop via %s failed, falling back to local startup checks: %v", desktopAttachPipeEnvVar, err)
		}
	}

	hasDefaultSyslabEnv, err := syslabenv.DefaultExists()
	if err != nil {
		return nil, err
	}
	if !hasDefaultSyslabEnv {
		if m.logger != nil {
			m.logger.Printf("default syslab-env.ini not found and %s is unavailable; falling back to nodesktop mode", desktopAttachPipeEnvVar)
		}
		m.cfg.SyslabDisplayMode = "nodesktop"
		return m.ensureStartedLocked(ctx, key, workingDirOverride)
	}

	ds, err := launchDesktopSession(ctx, m.cfg, wd, m.logger)
	if err != nil {
		return nil, err
	}
	return m.storeDesktopSessionLocked(key, wd, ds), nil
}

func (m *Manager) storeDesktopSessionLocked(key, wd string, ds *desktopSession) *bridgeSession {
	sess := &bridgeSession{
		Key:        key,
		WorkingDir: wd,
		Cmd:        ds.cmd,
		StartedAt:  time.Now().Format(time.RFC3339),
		Desktop:    ds,
	}
	m.sessions[key] = sess
	return sess
}

func (m *Manager) callDesktopLocked(ctx context.Context, key, method, cwd, payload string) (BridgeResult, error) {
	sess, err := m.ensureDesktopStartedLocked(ctx, key, "")
	if err != nil {
		return BridgeResult{}, err
	}
	result, err := m.callDesktopWithSession(ctx, sess, method, cwd, payload)
	if err == nil {
		return result, nil
	}
	if m.logger != nil {
		m.logger.Printf("desktop call failed, resetting session and retrying once: %v", err)
	}
	m.resetLocked(key)
	sess, restartErr := m.ensureDesktopStartedLocked(ctx, key, "")
	if restartErr != nil {
		return BridgeResult{}, restartErr
	}
	return m.callDesktopWithSession(ctx, sess, method, cwd, payload)
}

func (m *Manager) restartDesktopLocked(ctx context.Context, key string, workingDir string) (SessionInfo, error) {
	sess, err := m.ensureDesktopStartedLocked(ctx, key, workingDir)
	if err != nil {
		return SessionInfo{}, err
	}
	if strings.TrimSpace(workingDir) != "" {
		normalized := normalizePath(workingDir)
		if err := sess.Desktop.SyncWorkingDirectory(ctx, normalized); err != nil {
			return SessionInfo{}, err
		}
		sess.WorkingDir = normalized
	}
	if err := sess.Desktop.RestartJuliaREPL(ctx); err == nil {
		return sessionInfoFromState(sess), nil
	}
	if m.logger != nil {
		m.logger.Printf("desktop restart failed, resetting session and retrying once")
	}
	m.resetLocked(key)
	sess, err = m.ensureDesktopStartedLocked(ctx, key, workingDir)
	if err != nil {
		return SessionInfo{}, err
	}
	if strings.TrimSpace(workingDir) != "" {
		normalized := normalizePath(workingDir)
		if err := sess.Desktop.SyncWorkingDirectory(ctx, normalized); err != nil {
			return SessionInfo{}, err
		}
		sess.WorkingDir = normalized
	}
	if err := sess.Desktop.RestartJuliaREPL(ctx); err != nil {
		return SessionInfo{}, err
	}
	return sessionInfoFromState(sess), nil
}

func (m *Manager) callDesktopWithSession(ctx context.Context, sess *bridgeSession, method, cwd, payload string) (BridgeResult, error) {
	if cwd != "" {
		if err := sess.Desktop.SyncWorkingDirectory(ctx, cwd); err != nil {
			return BridgeResult{}, err
		}
		sess.WorkingDir = cwd
	}
	return sess.Desktop.Call(ctx, method, sess.WorkingDir, payload)
}

func launchDesktopSession(ctx context.Context, cfg config.Config, wd string, logger *log.Logger) (*desktopSession, error) {
	envStatus := currentDesktopEnvStatus()
	if err := validateDesktopEnvironment(runtime.GOOS, envStatus); err != nil {
		return nil, err
	}

	acceptor, err := newDesktopAcceptor()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = acceptor.Close()
	}()

	syslabExe, err := resolveSyslabDesktopExecutable(cfg.SyslabRoot)
	if err != nil {
		return nil, err
	}

	diagLogger, diagFile := newDesktopDiagLogger()
	if diagLogger != nil {
		diagLogger.Printf("desktop session start requested: syslab=%s cwd=%s endpoint=%s", syslabExe, wd, acceptor.Endpoint())
		diagLogger.Printf("desktop startup config: display_mode=%s syslab_root=%s julia_root=%s temp_dir=%s", cfg.SyslabDisplayMode, cfg.SyslabRoot, cfg.JuliaRoot, os.TempDir())
		diagLogger.Printf("desktop environment: %s", envStatus.Summary())
	}

	cmd := buildDesktopCommand(ctx, syslabExe, wd, acceptor.Endpoint(), diagLogger)
	if logger != nil {
		logger.Printf("starting Syslab desktop mode: %s", syslabExe)
	}
	if diagLogger != nil {
		diagLogger.Printf("starting process: %s", cmd.String())
		diagLogger.Printf("desktop startup env: %s=%s", desktopPipeEnvVar, acceptor.Endpoint())
	}
	if err := cmd.Start(); err != nil {
		if diagLogger != nil {
			diagLogger.Printf("process start failed: %v", err)
		}
		if diagFile != nil {
			_ = diagFile.Close()
		}
		return nil, fmt.Errorf("start syslab desktop: %w", err)
	}
	if diagLogger != nil && cmd.Process != nil {
		diagLogger.Printf("process started: pid=%d", cmd.Process.Pid)
	}

	acceptCtx := ctx
	cancelAccept := func() {}
	if derivedCtx, cancel := withOptionalTimeout(ctx, cfg.DesktopStartupTimeout); cancel != nil {
		acceptCtx = derivedCtx
		cancelAccept = cancel
	}
	defer cancelAccept()
	if diagLogger != nil {
		if deadline, ok := acceptCtx.Deadline(); ok {
			diagLogger.Printf("waiting for desktop client connection: endpoint=%s deadline=%s", acceptor.Endpoint(), deadline.Format(time.RFC3339))
		} else {
			diagLogger.Printf("waiting for desktop client connection: endpoint=%s deadline=none", acceptor.Endpoint())
		}
	}
	conn, err := acceptor.Accept(acceptCtx)
	if err != nil {
		if diagLogger != nil {
			diagLogger.Printf("desktop accept failed: %v", err)
		}
		_ = terminateProcess(cmd)
		if diagFile != nil {
			_ = diagFile.Close()
		}
		return nil, fmt.Errorf("accept syslab desktop connection: %w", err)
	}
	if diagLogger != nil {
		diagLogger.Printf("desktop client connected")
	}

	ds := &desktopSession{
		cfg:        cfg,
		logger:     logger,
		diagLogger: diagLogger,
		diagFile:   diagFile,
		workingDir: "",
		cmd:        cmd,
		conn:       conn,
		codec:      newDesktopJSONCodec(conn, func(format string, args ...any) { dsLog(diagLogger, format, args...) }),
	}
	if err := ds.waitReady(cfg.DesktopReadyTimeout); err != nil {
		ds.logf("desktop ready handshake failed: %v", err)
		_ = ds.Close()
		return nil, err
	}
	if err := ds.SyncWorkingDirectory(ctx, wd); err != nil {
		ds.logf("desktop working directory sync failed: %v", err)
		_ = ds.Close()
		return nil, err
	}
	ds.logf("desktop session ready")
	return ds, nil
}

func attachDesktopSession(ctx context.Context, cfg config.Config, wd, endpoint string, logger *log.Logger) (*desktopSession, error) {
	diagLogger, diagFile := newDesktopDiagLogger()
	if diagLogger != nil {
		diagLogger.Printf("desktop attach requested: endpoint=%s cwd=%s", endpoint, wd)
	}

	attachCtx := ctx
	cancelAttach := func() {}
	if derivedCtx, cancel := withOptionalTimeout(ctx, cfg.DesktopAttachTimeout); cancel != nil {
		attachCtx = derivedCtx
		cancelAttach = cancel
	}
	defer cancelAttach()

	conn, err := connectDesktopEndpoint(attachCtx, endpoint)
	if err != nil {
		if diagFile != nil {
			_ = diagFile.Close()
		}
		return nil, fmt.Errorf("connect existing syslab desktop endpoint %q: %w", endpoint, err)
	}

	ds := &desktopSession{
		cfg:        cfg,
		logger:     logger,
		diagLogger: diagLogger,
		diagFile:   diagFile,
		workingDir: "",
		cmd:        nil,
		conn:       conn,
	}
	ds.codec = newDesktopJSONCodec(conn, func(format string, args ...any) { dsLog(diagLogger, format, args...) })
	if err := ds.SyncWorkingDirectory(ctx, wd); err != nil {
		ds.logf("desktop attach working directory sync failed: %v", err)
		_ = ds.Close()
		return nil, err
	}
	ds.logf("desktop session attached")
	return ds, nil
}

func (s *desktopSession) waitReady(timeout time.Duration) error {
	s.logf("waitReady: begin timeout=%s", timeout)
	msg, err := s.codec.Recv(timeout)
	if err != nil {
		return fmt.Errorf("wait for syslab ready: %w", err)
	}
	s.logf("waitReady recv: command=%s result=%v error=%s", msg.Command, msg.Result, msg.Error)
	if strings.EqualFold(msg.Command, desktopReadyCommand) && (strings.EqualFold(fmt.Sprint(msg.Result), "true") || strings.EqualFold(fmt.Sprint(msg.Result), "True")) {
		return nil
	}
	if strings.TrimSpace(msg.Error) != "" {
		return errors.New(msg.Error)
	}
	return fmt.Errorf("unexpected syslab ready message: command=%s result=%v", msg.Command, msg.Result)
}

func (s *desktopSession) Call(ctx context.Context, method, cwd, payload string) (BridgeResult, error) {
	switch method {
	case "health":
		return BridgeResult{Result: "ok"}, nil
	case "detect_environment":
		return BridgeResult{Result: s.detectEnvironmentText(cwd)}, nil
	case "evaluate":
		if err := s.EnsureJuliaREPL(ctx); err != nil {
			return BridgeResult{}, err
		}
		return s.callJuliaREPL(ctx, cwd, payload)
	case "run_file":
		if err := s.EnsureJuliaREPL(ctx); err != nil {
			return BridgeResult{}, err
		}
		return s.runJuliaFile(ctx, payload)
	default:
		return BridgeResult{}, fmt.Errorf("unsupported desktop bridge method: %s", method)
	}
}

func (s *desktopSession) EnsureJuliaREPL(ctx context.Context) error {
	if s.replStarted {
		s.logf("EnsureJuliaREPL: already started")
		return nil
	}
	s.logf("EnsureJuliaREPL: start")
	_, err := s.sendCommand(ctx, desktopStartREPLCommand, "", s.cfg.DesktopREPLTimeout)
	if err != nil {
		s.logf("EnsureJuliaREPL: start failed: %v", err)
		return err
	}
	s.replStarted = true
	s.logf("EnsureJuliaREPL: start ok")
	return nil
}

func (s *desktopSession) RestartJuliaREPL(ctx context.Context) error {
	s.logf("RestartJuliaREPL: begin")
	if s.replStarted {
		if _, err := s.sendCommand(ctx, desktopStopREPLCommand, "", s.cfg.DesktopREPLTimeout); err != nil && s.logger != nil {
			s.logger.Printf("desktop stopREPL failed before restart: %v", err)
			s.logf("RestartJuliaREPL: stop failed: %v", err)
		}
		s.replStarted = false
	}
	if _, err := s.sendCommand(ctx, desktopStartREPLCommand, "", s.cfg.DesktopREPLTimeout); err != nil {
		s.logf("RestartJuliaREPL: start failed: %v", err)
		return err
	}
	s.replStarted = true
	s.logf("RestartJuliaREPL: ok")
	return nil
}

func (s *desktopSession) SyncWorkingDirectory(ctx context.Context, cwd string) error {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	if samePath(s.workingDir, abs) {
		return nil
	}
	s.logf("SyncWorkingDirectory: %s", abs)
	if _, err := s.sendCommand(ctx, desktopOpenFolderCommand, abs, s.cfg.DesktopControlTimeout); err != nil {
		s.logf("SyncWorkingDirectory failed: %v", err)
		return err
	}
	s.workingDir = abs
	return nil
}

func (s *desktopSession) runJuliaFile(ctx context.Context, scriptPath string) (BridgeResult, error) {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return BridgeResult{}, err
	}
	if _, err := s.sendCommand(ctx, desktopOpenFileCommand, abs, s.cfg.DesktopControlTimeout); err != nil {
		return BridgeResult{}, err
	}
	reply, err := s.sendCommand(ctx, desktopExecFileCommand, "", 0)
	if err != nil {
		reply, err = s.sendCommand(ctx, desktopExecFileCommand2, "", 0)
		if err != nil {
			return BridgeResult{}, err
		}
	}
	return s.withTerminalOutput(ctx, desktopReplyToBridgeResult(reply)), nil
}

func (s *desktopSession) callJuliaREPL(ctx context.Context, cwd, payload string) (BridgeResult, error) {
	reply, err := s.sendCommand(ctx, desktopEvalCommand, payload, 0)
	if err != nil {
		return BridgeResult{}, err
	}
	return s.withTerminalOutput(ctx, desktopReplyToBridgeResult(reply)), nil
}

func (s *desktopSession) withTerminalOutput(ctx context.Context, result BridgeResult) BridgeResult {
	reply, err := s.sendCommand(ctx, desktopTerminalDataCommand, "", s.cfg.DesktopControlTimeout)
	if err != nil {
		s.logf("get terminal output failed: %v", err)
		return result
	}
	return mergeDesktopTerminalOutput(result, reply)
}

func (s *desktopSession) sendCommand(ctx context.Context, command, args string, timeout time.Duration) (desktopMessage, error) {
	if err := ctx.Err(); err != nil {
		return desktopMessage{}, err
	}
	s.logf("sendCommand: command=%s args=%q timeout=%s", command, args, timeout)
	if err := s.codec.Send(desktopMessage{Command: command, Args: args}); err != nil {
		s.logf("sendCommand write failed: command=%s err=%v", command, err)
		return desktopMessage{}, fmt.Errorf("send desktop command %s: %w", command, err)
	}
	msg, err := s.waitForCommand(ctx, command, timeout)
	if err != nil {
		s.logf("sendCommand wait failed: command=%s err=%v", command, err)
		return desktopMessage{}, err
	}
	if strings.TrimSpace(msg.Error) != "" {
		s.logf("sendCommand reply error: command=%s err=%s", command, msg.Error)
		return desktopMessage{}, errors.New(msg.Error)
	}
	s.logf("sendCommand reply ok: command=%s result=%v", command, msg.Result)
	return msg, nil
}

func (s *desktopSession) waitForCommand(ctx context.Context, command string, timeout time.Duration) (desktopMessage, error) {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if err := ctx.Err(); err != nil {
			return desktopMessage{}, err
		}
		recvTimeout := 200 * time.Millisecond
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return desktopMessage{}, fmt.Errorf("timeout waiting for desktop reply: %s", command)
			}
			recvTimeout = minDuration(remaining, recvTimeout)
		}
		msg, err := s.codec.Recv(recvTimeout)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "timeout") {
				continue
			}
			s.logf("waitForCommand recv failed: command=%s err=%v", command, err)
			return desktopMessage{}, err
		}
		s.logf("waitForCommand recv: want=%s got=%s result=%v error=%s", command, msg.Command, msg.Result, msg.Error)
		if strings.EqualFold(msg.Command, command) {
			return msg, nil
		}
	}
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, nil
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, nil
	}
	return context.WithTimeout(ctx, timeout)
}

func (s *desktopSession) detectEnvironmentText(cwd string) string {
	lines := []string{
		"session_pid: " + strconv.Itoa(s.processPID()),
		"julia_version: ",
		"threads: ",
		"pwd: " + firstNonEmpty(strings.TrimSpace(cwd), strings.TrimSpace(s.workingDir)),
		"bindir: " + filepath.Join(s.cfg.JuliaRoot, "bin"),
		"depot_path: " + os.Getenv("JULIA_DEPOT_PATH"),
		"python: " + os.Getenv("PYTHON"),
		"ty_conda3: " + os.Getenv("TY_CONDA3"),
		"julia_depot_path: " + os.Getenv("JULIA_DEPOT_PATH"),
	}
	return strings.Join(lines, "\n")
}

func (s *desktopSession) processPID() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

func (s *desktopSession) Close() error {
	s.logf("desktop session close")
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.cmd != nil {
		_ = terminateProcess(s.cmd)
	}
	if s.diagFile != nil {
		_ = s.diagFile.Close()
	}
	return nil
}

func (s *desktopSession) Detach() error {
	s.logf("desktop session detach")
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		releaseProcessLifetime(s.cmd.Process.Pid)
	}
	if s.diagFile != nil {
		_ = s.diagFile.Close()
	}
	return nil
}

func buildDesktopCommand(_ context.Context, executable, cwd, endpoint string, diagLogger *log.Logger) *exec.Cmd {
	name := executable
	var args []string
	if runtime.GOOS != "windows" && strings.EqualFold(filepath.Ext(executable), ".sh") {
		// Match the Python probe launcher on Unix and avoid relying on execve shebang handling.
		name = "bash"
		args = []string{executable}
	}

	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), desktopPipeEnvVar+"="+endpoint)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	// Desktop mode is started from a stdio MCP server, so inheriting the server's stdin
	// can make the child consume MCP transport bytes or react to premature EOF.
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout = newDesktopLogWriter(diagLogger, "desktop-stdout")
	cmd.Stderr = newDesktopLogWriter(diagLogger, "desktop-stderr")
	applyDesktopProcessAttrs(cmd)
	return cmd
}

func currentDesktopEnvStatus() desktopEnvStatus {
	return desktopEnvStatus{
		Display:            strings.TrimSpace(os.Getenv("DISPLAY")),
		WaylandDisplay:     strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")),
		XAuthority:         strings.TrimSpace(os.Getenv("XAUTHORITY")),
		DBusSessionBusAddr: strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")),
		XDGSessionType:     strings.TrimSpace(os.Getenv("XDG_SESSION_TYPE")),
	}
}

func validateDesktopEnvironment(goos string, status desktopEnvStatus) error {
	if goos == "windows" {
		return nil
	}
	if status.Display != "" || status.WaylandDisplay != "" {
		return nil
	}
	return fmt.Errorf(
		"linux desktop mode requires a graphical session environment; missing DISPLAY and WAYLAND_DISPLAY (env: %s). Start the MCP host from a desktop terminal or explicitly pass through DISPLAY/XAUTHORITY/DBUS_SESSION_BUS_ADDRESS",
		status.Summary(),
	)
}

func (s desktopEnvStatus) Summary() string {
	return fmt.Sprintf(
		"DISPLAY=%q WAYLAND_DISPLAY=%q XAUTHORITY=%q DBUS_SESSION_BUS_ADDRESS=%q XDG_SESSION_TYPE=%q",
		s.Display,
		s.WaylandDisplay,
		s.XAuthority,
		s.DBusSessionBusAddr,
		s.XDGSessionType,
	)
}

func resolveSyslabDesktopExecutable(syslabRoot string) (string, error) {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(syslabRoot, "Bin", "syslab.exe"),
		}
	} else {
		candidates = []string{
			filepath.Join(syslabRoot, "syslab.sh"),
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not locate Syslab desktop executable under %s", syslabRoot)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(strings.TrimSpace(a)), filepath.Clean(strings.TrimSpace(b)))
}

func desktopReplyToBridgeResult(msg desktopMessage) BridgeResult {
	stdout, result := desktopResultText(msg.Result)
	return BridgeResult{
		Stdout: stdout,
		Result: result,
	}
}

func mergeDesktopTerminalOutput(result BridgeResult, msg desktopMessage) BridgeResult {
	stdout, _ := desktopResultText(msg.Result)
	if strings.TrimSpace(stdout) == "" {
		return result
	}
	result.Stdout = stdout
	return result
}

func desktopResultText(v any) (string, string) {
	switch value := v.(type) {
	case map[string]any:
		if all, ok := value["all"].(string); ok {
			trimmed := stripMarkdownCodeFence(all)
			if trimmed == "" {
				trimmed = all
			}
			return all, trimmed
		}
		body, _ := json.Marshal(value)
		text := string(body)
		return text, text
	case string:
		return value, stripMarkdownCodeFence(value)
	case bool:
		text := strconv.FormatBool(value)
		return text, text
	default:
		body, _ := json.Marshal(value)
		text := string(body)
		return text, text
	}
}

func stripMarkdownCodeFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		if len(lines) >= 1 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return trimmed
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newDesktopDiagLogger() (*log.Logger, *os.File) {
	path := filepath.Join(os.TempDir(), "syslab-mcp-desktop.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil
	}
	return log.New(f, "desktop-backend: ", log.LstdFlags|log.Lmicroseconds), f
}

type desktopLogWriter struct {
	logger *log.Logger
	label  string
}

func newDesktopLogWriter(logger *log.Logger, label string) io.Writer {
	if logger == nil {
		return io.Discard
	}
	return &desktopLogWriter{logger: logger, label: label}
}

func (w *desktopLogWriter) Write(p []byte) (int, error) {
	if w == nil || w.logger == nil {
		return len(p), nil
	}
	text := strings.TrimRight(string(p), "\r\n")
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.logger.Printf("%s: %s", w.label, line)
	}
	return len(p), nil
}

func (s *desktopSession) logf(format string, args ...any) {
	if s != nil && s.diagLogger != nil {
		s.diagLogger.Printf(format, args...)
	}
}

func dsLog(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
