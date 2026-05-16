package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type opsShellHandler struct {
	repoRoot    string
	dataDir     string
	frontendDir string
}

type shellPreset struct {
	Label       string `json:"label"`
	Command     string `json:"command"`
	Cwd         string `json:"cwd"`
	Description string `json:"description"`
}

type terminalDescription struct {
	Enabled       bool     `json:"enabled"`
	WebSocketPath string   `json:"websocketPath"`
	DefaultShell  string   `json:"defaultShell"`
	Shells        []string `json:"shells"`
}

type shellRunRequest struct {
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

type shellRunResponse struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Cwd        string   `json:"cwd"`
	ExitCode   int      `json:"exitCode"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	DurationMS int64    `json:"durationMs"`
	TimedOut   bool     `json:"timedOut,omitempty"`
}

func newOpsShellHandler(repoRoot, dataDir string) *opsShellHandler {
	return &opsShellHandler{
		repoRoot:    repoRoot,
		dataDir:     dataDir,
		frontendDir: filepath.Join(repoRoot, "web", "frontend"),
	}
}

func (h *opsShellHandler) describe(w http.ResponseWriter, r *http.Request) {
	type shellDescription struct {
		Cwds           map[string]string   `json:"cwds"`
		AllowedCommand []string            `json:"allowedCommands"`
		Presets        []shellPreset       `json:"presets"`
		Terminal       terminalDescription `json:"terminal"`
	}

	writeJSON(w, http.StatusOK, shellDescription{
		Cwds: map[string]string{
			"repo":     h.repoRoot,
			"data":     h.dataDir,
			"frontend": h.frontendDir,
		},
		AllowedCommand: []string{
			"pwd",
			"ls [path]",
			"find [path] [-maxdepth N]",
			"cat <file>",
			"head [-n N] <file>",
			"tail [-n N] <file>",
			"wc <file>",
			"git status [-sb]",
			"go test ./...",
		},
		Presets: []shellPreset{
			{Label: "Repo root", Command: "pwd", Cwd: "repo", Description: "查看后端当前工作目录"},
			{Label: "Data files", Command: "find . -maxdepth 2", Cwd: "data", Description: "列出 data 目录下的文件"},
			{Label: "Project config", Command: "cat config.json", Cwd: "data", Description: "快速检查 data 目录下的项目配置"},
			{Label: "Frontend assets", Command: "ls dist", Cwd: "frontend", Description: "查看当前前端构建产物"},
			{Label: "Git status", Command: "git status -sb", Cwd: "repo", Description: "查看代码改动状态"},
			{Label: "Run tests", Command: "go test ./...", Cwd: "repo", Description: "执行后端测试"},
		},
		Terminal: h.describeTerminal(),
	})
}

func (h *opsShellHandler) run(w http.ResponseWriter, r *http.Request) {
	var input shellRunRequest
	if err := decodeBody(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(input.Command) == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	cwd, err := h.resolveCwd(input.Cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name, args, err := h.parseCommand(strings.TrimSpace(input.Command), cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	timeoutSec := input.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	if timeoutSec > 60 {
		timeoutSec = 60
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	started := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	runErr := cmd.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			exitCode = -1
			if stderr.Len() > 0 {
				stderr.WriteString("\n")
			}
			stderr.WriteString("command timed out")
		} else {
			writeError(w, http.StatusInternalServerError, runErr.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, shellRunResponse{
		Command:    name,
		Args:       args,
		Cwd:        cwd,
		ExitCode:   exitCode,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMS: time.Since(started).Milliseconds(),
		TimedOut:   errors.Is(ctx.Err(), context.DeadlineExceeded),
	})
}

func (h *opsShellHandler) resolveCwd(input string) (string, error) {
	switch strings.TrimSpace(input) {
	case "", "repo":
		return h.repoRoot, nil
	case "data":
		return h.dataDir, nil
	case "frontend":
		return h.frontendDir, nil
	default:
		return "", errors.New("unsupported cwd")
	}
}

func (h *opsShellHandler) parseCommand(line, cwd string) (string, []string, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil, errors.New("command is required")
	}

	name := fields[0]
	args := fields[1:]

	switch name {
	case "pwd":
		if len(args) != 0 {
			return "", nil, errors.New("pwd does not accept arguments")
		}
		return name, nil, nil
	case "ls":
		validated, err := h.validateLSArgs(args, cwd)
		return name, validated, err
	case "find":
		validated, err := h.validateFindArgs(args, cwd)
		return name, validated, err
	case "cat":
		validated, err := h.validateSinglePathArgs("cat", args, cwd)
		return name, validated, err
	case "wc":
		validated, err := h.validateSinglePathArgs("wc", args, cwd)
		return name, validated, err
	case "head", "tail":
		validated, err := h.validateHeadTailArgs(args, cwd)
		return name, validated, err
	case "git":
		validated, err := validateGitArgs(args)
		return name, validated, err
	case "go":
		validated, err := validateGoArgs(args)
		return name, validated, err
	default:
		return "", nil, errors.New("command is not allowed")
	}
}

func (h *opsShellHandler) validateLSArgs(args []string, cwd string) ([]string, error) {
	switch len(args) {
	case 0:
		return nil, nil
	case 1:
		if strings.HasPrefix(args[0], "-") {
			if args[0] != "-l" && args[0] != "-a" && args[0] != "-la" && args[0] != "-al" {
				return nil, errors.New("unsupported ls flag")
			}
			return args, nil
		}
		resolved, err := h.resolvePathArg(cwd, args[0], false)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	case 2:
		if args[0] != "-l" && args[0] != "-a" && args[0] != "-la" && args[0] != "-al" {
			return nil, errors.New("unsupported ls flag")
		}
		resolved, err := h.resolvePathArg(cwd, args[1], false)
		if err != nil {
			return nil, err
		}
		return []string{args[0], resolved}, nil
	default:
		return nil, errors.New("ls accepts at most one flag and one path")
	}
}

func (h *opsShellHandler) validateFindArgs(args []string, cwd string) ([]string, error) {
	if len(args) == 0 {
		return []string{"."}, nil
	}

	resolved, err := h.resolvePathArg(cwd, args[0], false)
	if err != nil {
		return nil, err
	}
	if len(args) == 1 {
		return []string{resolved}, nil
	}
	if len(args) == 3 && args[1] == "-maxdepth" {
		if _, err := strconv.Atoi(args[2]); err != nil {
			return nil, errors.New("find maxdepth must be a number")
		}
		return []string{resolved, "-maxdepth", args[2]}, nil
	}
	return nil, errors.New("find only supports: find [path] or find [path] -maxdepth N")
}

func (h *opsShellHandler) validateSinglePathArgs(command string, args []string, cwd string) ([]string, error) {
	if len(args) != 1 {
		return nil, errors.New(command + " requires exactly one path")
	}
	resolved, err := h.resolvePathArg(cwd, args[0], true)
	if err != nil {
		return nil, err
	}
	return []string{resolved}, nil
}

func (h *opsShellHandler) validateHeadTailArgs(args []string, cwd string) ([]string, error) {
	switch len(args) {
	case 1:
		resolved, err := h.resolvePathArg(cwd, args[0], true)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	case 3:
		if args[0] != "-n" {
			return nil, errors.New("head/tail only supports -n N <file>")
		}
		if _, err := strconv.Atoi(args[1]); err != nil {
			return nil, errors.New("head/tail line count must be a number")
		}
		resolved, err := h.resolvePathArg(cwd, args[2], true)
		if err != nil {
			return nil, err
		}
		return []string{"-n", args[1], resolved}, nil
	default:
		return nil, errors.New("head/tail only supports: <file> or -n N <file>")
	}
}

func validateGitArgs(args []string) ([]string, error) {
	switch len(args) {
	case 1:
		if args[0] == "status" {
			return args, nil
		}
	case 2:
		if args[0] == "status" && args[1] == "-sb" {
			return args, nil
		}
	}
	return nil, errors.New("git only supports: git status or git status -sb")
}

func validateGoArgs(args []string) ([]string, error) {
	if len(args) < 2 || args[0] != "test" {
		return nil, errors.New("go only supports go test")
	}
	for _, arg := range args[1:] {
		if !strings.HasPrefix(arg, "./") && !strings.HasPrefix(arg, "-") {
			return nil, errors.New("go test arguments must stay within the repository")
		}
	}
	return args, nil
}

func (h *opsShellHandler) resolvePathArg(cwd, raw string, expectFile bool) (string, error) {
	var candidate string
	if filepath.IsAbs(raw) {
		candidate = filepath.Clean(raw)
	} else {
		candidate = filepath.Join(cwd, raw)
	}
	candidate = filepath.Clean(candidate)
	if !h.isAllowedPath(candidate) {
		return "", errors.New("path is outside allowed roots")
	}
	if expectFile && strings.HasSuffix(raw, string(filepath.Separator)) {
		return "", errors.New("expected file path")
	}
	return candidate, nil
}

func (h *opsShellHandler) isAllowedPath(path string) bool {
	for _, root := range []string{h.repoRoot, h.dataDir, h.frontendDir} {
		if root == "" {
			continue
		}
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func decodeBody(r *http.Request, dest any) error {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return errors.New("invalid JSON: " + err.Error())
	}
	return nil
}
