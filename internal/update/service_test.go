package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckDetectsNewRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRelease(t, w, r, nil, nil)
	}))
	defer server.Close()

	svc := &Service{
		CurrentVersion: "v0.2.0",
		ReleaseAPIURL:  server.URL + "/release",
		HTTPClient:     server.Client(),
	}
	result, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.UpdateAvailable {
		t.Fatalf("expected update to be available: %+v", result)
	}
	if result.LatestVersion != "v0.2.1" {
		t.Fatalf("latest version = %q", result.LatestVersion)
	}
}

func TestInstallReplacesExecutableFromVerifiedRelease(t *testing.T) {
	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("windows cannot replace a running executable in-place")
	}

	dir := t.TempDir()
	executable := filepath.Join(dir, "storyforge")
	if err := os.WriteFile(executable, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := buildArchive(t, "new-binary")
	sum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(sum[:]) + "  " + assetName + "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			writeRelease(t, w, r, archive, checksums)
		case "/asset/archive":
			_, _ = w.Write(archive)
		case "/asset/checksums":
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := &Service{
		CurrentVersion: "v0.2.0",
		ReleaseAPIURL:  server.URL + "/release",
		ExecutablePath: executable,
		HTTPClient:     server.Client(),
	}
	result, err := svc.Install(context.Background(), "v0.2.1")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	updated, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "new-binary" {
		t.Fatalf("executable = %q, want new-binary", updated)
	}
	if result.InstalledVersion != "v0.2.1" || !result.RestartRequired {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.BackupPath == "" {
		t.Fatalf("expected backup path")
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "old-binary" {
		t.Fatalf("backup = %q, want old-binary", backup)
	}
}

func writeRelease(t *testing.T, w http.ResponseWriter, r *http.Request, archive, checksums []byte) {
	t.Helper()
	baseURL := "http://" + r.Host
	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if archive == nil {
		archive = []byte("placeholder")
	}
	if checksums == nil {
		checksums = []byte("placeholder")
	}
	resp := releaseInfo{
		TagName: "v0.2.1",
		HTMLURL: baseURL + "/releases/tag/v0.2.1",
		Assets: []releaseAsset{
			{
				Name:               assetName,
				URL:                baseURL + "/asset/archive",
				BrowserDownloadURL: baseURL + "/download/" + assetName,
			},
			{
				Name:               "checksums.txt",
				URL:                baseURL + "/asset/checksums",
				BrowserDownloadURL: baseURL + "/download/checksums.txt",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatal(err)
	}
	_, _ = archive, checksums
}

func buildArchive(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	data := []byte(content)
	if err := tw.WriteHeader(&tar.Header{
		Name: "storyforge",
		Mode: 0o755,
		Size: int64(len(data)),
	}); err != nil {
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
	if buf.Len() == 0 {
		t.Fatal(fmt.Errorf("empty archive"))
	}
	return buf.Bytes()
}
