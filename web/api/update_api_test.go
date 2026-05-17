package api_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"storyforge/web/api"
)

func TestVersionEndpointReturnsBuildVersion(t *testing.T) {
	restoreVersion := api.BuildVersion
	api.BuildVersion = "v0.2.1"
	t.Cleanup(func() { api.BuildVersion = restoreVersion })

	env := newTestEnv(t)
	w := do(t, env.handler, http.MethodGet, "/api/version", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("version: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Version string `json:"version"`
	}
	decodeJSON(t, w, &payload)
	if payload.Version != "v0.2.1" {
		t.Fatalf("version = %q", payload.Version)
	}
}

func TestUpdateCheckDetectsAvailableRelease(t *testing.T) {
	restoreVersion := api.BuildVersion
	api.BuildVersion = "v0.2.0"
	t.Cleanup(func() { api.BuildVersion = restoreVersion })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeUpdateRelease(t, w, r, nil, nil)
	}))
	defer server.Close()
	t.Setenv("STORYFORGE_UPDATE_API_URL", server.URL+"/release")

	env := newTestEnv(t)
	w := do(t, env.handler, http.MethodGet, "/api/update/check", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("update check: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		CurrentVersion  string `json:"currentVersion"`
		LatestVersion   string `json:"latestVersion"`
		UpdateAvailable bool   `json:"updateAvailable"`
	}
	decodeJSON(t, w, &payload)
	if payload.CurrentVersion != "v0.2.0" || payload.LatestVersion != "v0.2.1" || !payload.UpdateAvailable {
		t.Fatalf("unexpected update payload: %+v", payload)
	}
}

func TestUpdateInstallEndpointReplacesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows cannot replace a running executable in-place")
	}
	restoreVersion := api.BuildVersion
	api.BuildVersion = "v0.2.0"
	t.Cleanup(func() { api.BuildVersion = restoreVersion })

	assetName := testUpdateAssetName(t)
	archive := testUpdateArchive(t, "new-storyforge")
	sum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(sum[:]) + "  " + assetName + "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			writeUpdateRelease(t, w, r, archive, checksums)
		case "/asset/archive":
			_, _ = w.Write(archive)
		case "/asset/checksums":
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "storyforge")
	if err := os.WriteFile(executable, []byte("old-storyforge"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORYFORGE_UPDATE_API_URL", server.URL+"/release")
	t.Setenv("STORYFORGE_UPDATE_EXECUTABLE", executable)

	env := newTestEnv(t)
	w := do(t, env.handler, http.MethodPost, "/api/update/install", map[string]string{"version": "v0.2.1"})
	if w.Code != http.StatusOK {
		t.Fatalf("update install: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "new-storyforge" {
		t.Fatalf("updated executable = %q", updated)
	}
	var payload struct {
		InstalledVersion string `json:"installedVersion"`
		RestartRequired  bool   `json:"restartRequired"`
	}
	decodeJSON(t, w, &payload)
	if payload.InstalledVersion != "v0.2.1" || !payload.RestartRequired {
		t.Fatalf("unexpected install response: %+v", payload)
	}
}

func writeUpdateRelease(t *testing.T, w http.ResponseWriter, r *http.Request, archive, checksums []byte) {
	t.Helper()
	baseURL := "http://" + r.Host
	resp := map[string]any{
		"tag_name": "v0.2.1",
		"html_url": baseURL + "/releases/tag/v0.2.1",
		"assets": []map[string]string{
			{
				"name":                 testUpdateAssetName(t),
				"url":                  baseURL + "/asset/archive",
				"browser_download_url": baseURL + "/download/archive",
			},
			{
				"name":                 "checksums.txt",
				"url":                  baseURL + "/asset/checksums",
				"browser_download_url": baseURL + "/download/checksums",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatal(err)
	}
	_, _ = archive, checksums
}

func testUpdateAssetName(t *testing.T) string {
	t.Helper()
	osName := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[runtime.GOOS]
	archName := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	if osName == "" || archName == "" {
		t.Skipf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return "storyforge_" + osName + "_" + archName + "." + ext
}

func testUpdateArchive(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	data := []byte(content)
	if err := tw.WriteHeader(&tar.Header{Name: "storyforge", Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
