package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const defaultRepo = "smileQiny/StoryForge"

type Service struct {
	CurrentVersion string
	Repo           string
	ReleaseAPIURL  string
	ExecutablePath string
	HTTPClient     *http.Client
}

type CheckResult struct {
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   string    `json:"latestVersion"`
	UpdateAvailable bool      `json:"updateAvailable"`
	ReleaseURL      string    `json:"releaseUrl,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
}

type InstallResult struct {
	CurrentVersion   string    `json:"currentVersion"`
	InstalledVersion string    `json:"installedVersion"`
	ExecutablePath   string    `json:"executablePath"`
	BackupPath       string    `json:"backupPath,omitempty"`
	RestartRequired  bool      `json:"restartRequired"`
	ReleaseURL       string    `json:"releaseUrl,omitempty"`
	InstalledAt      time.Time `json:"installedAt"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func NewService(currentVersion string) *Service {
	return &Service{
		CurrentVersion: strings.TrimSpace(currentVersion),
		Repo:           envOrDefault("STORYFORGE_UPDATE_REPO", defaultRepo),
		ReleaseAPIURL:  strings.TrimSpace(os.Getenv("STORYFORGE_UPDATE_API_URL")),
		ExecutablePath: strings.TrimSpace(os.Getenv("STORYFORGE_UPDATE_EXECUTABLE")),
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				// GitHub downloads can be flaky behind some proxies with HTTP/2.
				TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
			},
		},
	}
}

func (s *Service) Check(ctx context.Context) (*CheckResult, error) {
	release, err := s.fetchRelease(ctx, "")
	if err != nil {
		return nil, err
	}
	current := normalizeVersion(s.CurrentVersion)
	latest := normalizeVersion(release.TagName)
	return &CheckResult{
		CurrentVersion:  valueOr(s.CurrentVersion, "dev"),
		LatestVersion:   release.TagName,
		UpdateAvailable: current != "" && latest != "" && compareVersions(latest, current) > 0,
		ReleaseURL:      release.HTMLURL,
		CheckedAt:       time.Now().UTC(),
	}, nil
}

func (s *Service) Install(ctx context.Context, targetVersion string) (*InstallResult, error) {
	release, err := s.fetchRelease(ctx, targetVersion)
	if err != nil {
		return nil, err
	}
	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	archiveAsset, ok := findAsset(release.Assets, assetName)
	if !ok {
		return nil, fmt.Errorf("release asset %s not found", assetName)
	}
	checksumsAsset, ok := findAsset(release.Assets, "checksums.txt")
	if !ok {
		return nil, fmt.Errorf("release asset checksums.txt not found")
	}
	executable, err := s.executablePath()
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("self-update on Windows requires manual replacement after download")
	}

	workDir, err := os.MkdirTemp("", "storyforge-update-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	archivePath := filepath.Join(workDir, assetName)
	checksumsPath := filepath.Join(workDir, "checksums.txt")
	if err := s.downloadAsset(ctx, archiveAsset, archivePath); err != nil {
		return nil, err
	}
	if err := s.downloadAsset(ctx, checksumsAsset, checksumsPath); err != nil {
		return nil, err
	}
	if err := verifyChecksum(archivePath, checksumsPath, assetName); err != nil {
		return nil, err
	}
	extracted, err := extractStoryForgeBinary(archivePath, workDir)
	if err != nil {
		return nil, err
	}
	backup, err := replaceExecutable(executable, extracted)
	if err != nil {
		return nil, err
	}
	return &InstallResult{
		CurrentVersion:   valueOr(s.CurrentVersion, "dev"),
		InstalledVersion: release.TagName,
		ExecutablePath:   executable,
		BackupPath:       backup,
		RestartRequired:  true,
		ReleaseURL:       release.HTMLURL,
		InstalledAt:      time.Now().UTC(),
	}, nil
}

func (s *Service) fetchRelease(ctx context.Context, targetVersion string) (*releaseInfo, error) {
	apiURL := s.ReleaseAPIURL
	if apiURL == "" {
		repo := valueOr(s.Repo, defaultRepo)
		if strings.TrimSpace(targetVersion) == "" || strings.TrimSpace(targetVersion) == "latest" {
			apiURL = "https://api.github.com/repos/" + repo + "/releases/latest"
		} else {
			apiURL = "https://api.github.com/repos/" + repo + "/releases/tags/" + strings.TrimSpace(targetVersion)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("release lookup failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("release response missing tag_name")
	}
	return &release, nil
}

func (s *Service) downloadAsset(ctx context.Context, asset releaseAsset, out string) error {
	urls := []struct {
		url    string
		accept string
	}{
		{asset.URL, "application/octet-stream"},
		{asset.BrowserDownloadURL, ""},
	}
	var lastErr error
	for _, candidate := range urls {
		if strings.TrimSpace(candidate.url) == "" {
			continue
		}
		if err := s.downloadURL(ctx, candidate.url, candidate.accept, out); err != nil {
			lastErr = err
			_ = os.Remove(out)
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("asset %s has no download URL", asset.Name)
	}
	return lastErr
}

func (s *Service) downloadURL(ctx context.Context, rawURL, accept, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := out + ".part"
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resumeFrom := partialSize(tmp)
		if resumeFrom > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeFrom))
		}
		resp, err := s.client().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && resumeFrom > 0 {
			_ = resp.Body.Close()
			if err := os.Rename(tmp, out); err != nil {
				return err
			}
			return nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return fmt.Errorf("download failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
		}
		flags := os.O_CREATE | os.O_WRONLY
		if resumeFrom > 0 && resp.StatusCode == http.StatusPartialContent {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(tmp, flags, 0o644)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
		_, copyErr := io.Copy(file, resp.Body)
		closeErr := file.Close()
		_ = resp.Body.Close()
		if copyErr != nil {
			lastErr = copyErr
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		if err := os.Rename(tmp, out); err != nil {
			return err
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download failed after retries")
	}
	return lastErr
}

func partialSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (s *Service) executablePath() (string, error) {
	if strings.TrimSpace(s.ExecutablePath) != "" {
		return filepath.Abs(strings.TrimSpace(s.ExecutablePath))
	}
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (s *Service) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return http.DefaultClient
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func releaseAssetName(goos, goarch string) (string, error) {
	var osName string
	switch goos {
	case "darwin":
		osName = "Darwin"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
	var archName string
	switch goarch {
	case "amd64":
		archName = "x86_64"
	case "arm64":
		archName = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return "storyforge_" + osName + "_" + archName + "." + ext, nil
}

func verifyChecksum(archivePath, checksumsPath, assetName string) error {
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	var expected string
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s not found", assetName)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func extractStoryForgeBinary(archivePath, workDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractStoryForgeFromZip(archivePath, workDir)
	}
	return extractStoryForgeFromTarGz(archivePath, workDir)
}

func extractStoryForgeFromTarGz(archivePath, workDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != "storyforge" {
			continue
		}
		out := filepath.Join(workDir, "storyforge-new")
		if err := writeExtractedFile(out, header.FileInfo().Mode(), tr); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("archive does not contain storyforge binary")
}

func extractStoryForgeFromZip(archivePath, workDir string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != "storyforge.exe" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return "", err
		}
		out := filepath.Join(workDir, "storyforge-new.exe")
		err = writeExtractedFile(out, file.FileInfo().Mode(), reader)
		_ = reader.Close()
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("archive does not contain storyforge.exe binary")
}

func writeExtractedFile(path string, mode os.FileMode, reader io.Reader) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func replaceExecutable(executable, replacement string) (string, error) {
	info, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	backup := executable + ".bak-" + time.Now().UTC().Format("20060102150405")
	if err := copyFile(executable, backup, info.Mode()); err != nil {
		return "", err
	}
	tmp := executable + ".new"
	if err := copyFile(replacement, tmp, info.Mode()|0o700); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, executable); err != nil {
		return "", err
	}
	return backup, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" || version == "dev" {
		return ""
	}
	return version
}

func compareVersions(a, b string) int {
	aa := versionParts(a)
	bb := versionParts(b)
	max := len(aa)
	if len(bb) > max {
		max = len(bb)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(aa) {
			av = aa[i]
		}
		if i < len(bb) {
			bv = bb[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func versionParts(version string) []int {
	version = normalizeVersion(version)
	if version == "" {
		return nil
	}
	pieces := strings.Split(version, ".")
	parts := make([]int, 0, len(pieces))
	for _, piece := range pieces {
		n, _ := strconv.Atoi(piece)
		parts = append(parts, n)
	}
	return parts
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
