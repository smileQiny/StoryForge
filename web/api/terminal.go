package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	terminalWriteWait = 10 * time.Second
	terminalPongWait  = 70 * time.Second
	terminalPingWait  = 30 * time.Second
)

var terminalUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   4096,
	WriteBufferSize:  4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type terminalShellSpec struct {
	Name string
	Path string
	Args []string
}

type terminalInbound struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

type terminalOutbound struct {
	Type     string `json:"type"`
	Data     string `json:"data,omitempty"`
	Error    string `json:"error,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Shell    string `json:"shell,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
}

func (h *opsShellHandler) describeTerminal() terminalDescription {
	shells := h.availableTerminalShells()
	names := make([]string, 0, len(shells))
	defaultShell := "sh"
	for index, shell := range shells {
		names = append(names, shell.Name)
		if index == 0 {
			defaultShell = shell.Name
		}
	}
	return terminalDescription{
		Enabled:       len(names) > 0,
		WebSocketPath: "/api/ops/terminal/ws",
		DefaultShell:  defaultShell,
		Shells:        names,
	}
}

func (h *opsShellHandler) terminalWS(w http.ResponseWriter, r *http.Request) {
	cwd, err := h.resolveTerminalCwd(r.URL.Query().Get("cwd"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	shell, err := h.resolveTerminalShell(r.URL.Query().Get("shell"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cols, rows := parseTerminalSize(r.URL.Query().Get("cols"), r.URL.Query().Get("rows"))
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	cmd := exec.Command(shell.Path, shell.Args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		_ = conn.WriteJSON(terminalOutbound{Type: "error", Error: err.Error()})
		_ = conn.Close()
		return
	}

	var once sync.Once
	done := make(chan struct{})
	shutdown := func() {
		once.Do(func() {
			close(done)
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = conn.Close()
		})
	}
	defer shutdown()

	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(terminalPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(terminalPongWait))
	})

	outgoing := make(chan terminalOutbound, 64)
	enqueue := func(message terminalOutbound) {
		select {
		case outgoing <- message:
		case <-done:
		default:
			return
		}
	}

	enqueue(terminalOutbound{
		Type:  "ready",
		Cwd:   cwd,
		Shell: shell.Name,
	})

	go func() {
		buffer := make([]byte, 4096)
		for {
			count, readErr := ptmx.Read(buffer)
			if count > 0 {
				enqueue(terminalOutbound{
					Type: "output",
					Data: string(buffer[:count]),
				})
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrClosed) {
					enqueue(terminalOutbound{Type: "error", Error: readErr.Error()})
				}
				return
			}
		}
	}()

	go func() {
		waitErr := cmd.Wait()
		exitCode := 0
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
				enqueue(terminalOutbound{Type: "error", Error: waitErr.Error()})
			}
		}
		enqueue(terminalOutbound{Type: "exit", ExitCode: exitCode})
	}()

	go func() {
		for {
			var message terminalInbound
			if err := conn.ReadJSON(&message); err != nil {
				shutdown()
				return
			}
			switch message.Type {
			case "input":
				if message.Data == "" {
					continue
				}
				if _, err := ptmx.Write([]byte(message.Data)); err != nil {
					enqueue(terminalOutbound{Type: "error", Error: err.Error()})
					shutdown()
					return
				}
			case "resize":
				if err := pty.Setsize(ptmx, &pty.Winsize{
					Cols: clampTerminalDimension(message.Cols, 120, 20, 320),
					Rows: clampTerminalDimension(message.Rows, 32, 8, 120),
				}); err != nil {
					enqueue(terminalOutbound{Type: "error", Error: err.Error()})
					shutdown()
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(terminalPingWait)
	defer ticker.Stop()

	for {
		select {
		case message := <-outgoing:
			if err := conn.SetWriteDeadline(time.Now().Add(terminalWriteWait)); err != nil {
				return
			}
			if err := conn.WriteJSON(message); err != nil {
				return
			}
			if message.Type == "exit" {
				return
			}
		case <-ticker.C:
			if err := conn.SetWriteDeadline(time.Now().Add(terminalWriteWait)); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func (h *opsShellHandler) resolveTerminalCwd(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch value {
	case "", "repo":
		return h.repoRoot, nil
	case "data":
		return h.dataDir, nil
	case "frontend":
		return h.frontendDir, nil
	}

	if !filepath.IsAbs(value) {
		value = filepath.Join(h.repoRoot, value)
	}
	resolved, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("terminal cwd must be a directory")
	}
	return resolved, nil
}

func (h *opsShellHandler) availableTerminalShells() []terminalShellSpec {
	candidates := []terminalShellSpec{
		{Name: "bash", Path: "bash", Args: []string{"-l"}},
		{Name: "zsh", Path: "zsh", Args: []string{"-l"}},
		{Name: "sh", Path: "sh"},
	}

	result := make([]terminalShellSpec, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate.Path)
		if err != nil {
			continue
		}
		if _, ok := seen[candidate.Name]; ok {
			continue
		}
		seen[candidate.Name] = struct{}{}
		result = append(result, terminalShellSpec{
			Name: candidate.Name,
			Path: path,
			Args: candidate.Args,
		})
	}
	return result
}

func (h *opsShellHandler) resolveTerminalShell(raw string) (terminalShellSpec, error) {
	shells := h.availableTerminalShells()
	if len(shells) == 0 {
		return terminalShellSpec{}, errors.New("no supported shell found on server")
	}

	if strings.TrimSpace(raw) == "" {
		return shells[0], nil
	}

	for _, shell := range shells {
		if shell.Name == raw {
			return shell, nil
		}
	}
	return terminalShellSpec{}, errors.New("unsupported shell")
}

func parseTerminalSize(rawCols, rawRows string) (uint16, uint16) {
	cols, _ := strconv.Atoi(strings.TrimSpace(rawCols))
	rows, _ := strconv.Atoi(strings.TrimSpace(rawRows))
	return clampTerminalDimension(cols, 120, 20, 320), clampTerminalDimension(rows, 32, 8, 120)
}

func clampTerminalDimension(value, fallback, minValue, maxValue int) uint16 {
	if value <= 0 {
		value = fallback
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return uint16(value)
}
