package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestOpsShell_Describe(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodGet, "/api/ops/shell", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("describe shell: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload struct {
		Cwds           map[string]string `json:"cwds"`
		AllowedCommand []string          `json:"allowedCommands"`
		Presets        []map[string]any  `json:"presets"`
	}
	decodeJSON(t, w, &payload)
	if payload.Cwds["data"] != env.dir {
		t.Fatalf("expected data cwd %q, got %q", env.dir, payload.Cwds["data"])
	}
	if len(payload.AllowedCommand) == 0 || len(payload.Presets) == 0 {
		t.Fatalf("expected non-empty shell description, got %+v", payload)
	}
}

func TestOpsShell_RunPwdAndCat(t *testing.T) {
	env := newTestEnv(t)
	configPath := filepath.Join(env.dir, "sample.txt")
	if err := os.WriteFile(configPath, []byte("hello shell"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	w := do(t, env.handler, http.MethodPost, "/api/ops/shell", map[string]any{
		"command": "pwd",
		"cwd":     "data",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("run pwd: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var pwdResp struct {
		Cwd      string `json:"cwd"`
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exitCode"`
	}
	decodeJSON(t, w, &pwdResp)
	if pwdResp.ExitCode != 0 {
		t.Fatalf("expected pwd exit code 0, got %d", pwdResp.ExitCode)
	}
	if strings.TrimSpace(pwdResp.Stdout) != env.dir {
		t.Fatalf("expected pwd stdout %q, got %q", env.dir, pwdResp.Stdout)
	}

	w = do(t, env.handler, http.MethodPost, "/api/ops/shell", map[string]any{
		"command": "cat sample.txt",
		"cwd":     "data",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("run cat: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var catResp struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exitCode"`
	}
	decodeJSON(t, w, &catResp)
	if catResp.ExitCode != 0 {
		t.Fatalf("expected cat exit code 0, got %d", catResp.ExitCode)
	}
	if strings.TrimSpace(catResp.Stdout) != "hello shell" {
		t.Fatalf("unexpected cat stdout: %q", catResp.Stdout)
	}
}

func TestOpsShell_RejectsUnsupportedCommandAndPath(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodPost, "/api/ops/shell", map[string]any{
		"command": "rm -rf data",
		"cwd":     "repo",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported command: expected 400, got %d", w.Code)
	}

	w = do(t, env.handler, http.MethodPost, "/api/ops/shell", map[string]any{
		"command": "cat /etc/hosts",
		"cwd":     "repo",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("outside path: expected 400, got %d", w.Code)
	}
}

func TestOpsTerminal_WebSocketSession(t *testing.T) {
	env := newTestEnv(t)
	server := httptest.NewServer(env.handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ops/terminal/ws?cwd=data&shell=sh&cols=80&rows=24"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial terminal websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var ready struct {
		Type  string `json:"type"`
		Cwd   string `json:"cwd"`
		Shell string `json:"shell"`
	}
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("read ready event: %v", err)
	}
	if ready.Type != "ready" {
		t.Fatalf("expected ready event, got %+v", ready)
	}
	if ready.Cwd != env.dir {
		t.Fatalf("expected cwd %q, got %q", env.dir, ready.Cwd)
	}

	if err := conn.WriteJSON(map[string]any{
		"type": "input",
		"data": "printf 'hello from terminal\\n'\r",
	}); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}

	var output strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(output.String(), "hello from terminal") && time.Now().Before(deadline) {
		var message struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read terminal output: %v", err)
		}
		if message.Type == "output" {
			output.WriteString(message.Data)
		}
	}
	if !strings.Contains(output.String(), "hello from terminal") {
		t.Fatalf("expected terminal output to contain hello, got %q", output.String())
	}

	if err := conn.WriteJSON(map[string]any{
		"type": "input",
		"data": "exit\r",
	}); err != nil {
		t.Fatalf("write exit input: %v", err)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var message struct {
			Type     string `json:"type"`
			ExitCode int    `json:"exitCode"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read exit event: %v", err)
		}
		if message.Type == "exit" {
			if message.ExitCode != 0 {
				t.Fatalf("expected exit code 0, got %d", message.ExitCode)
			}
			return
		}
	}

	t.Fatal("timed out waiting for terminal exit event")
}

func TestOpsTerminal_AcceptsAbsoluteCwd(t *testing.T) {
	env := newTestEnv(t)
	server := httptest.NewServer(env.handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ops/terminal/ws?cwd=" + env.dir + "&shell=sh&cols=80&rows=24"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial terminal websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var ready struct {
		Type string `json:"type"`
		Cwd  string `json:"cwd"`
	}
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("read ready event: %v", err)
	}
	if ready.Type != "ready" {
		t.Fatalf("expected ready event, got %+v", ready)
	}
	if ready.Cwd != env.dir {
		t.Fatalf("expected cwd %q, got %q", env.dir, ready.Cwd)
	}
}
