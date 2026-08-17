package contract

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// UploadDir returns the project upload directory independent of the process cwd.
func UploadDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "uploads"
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "uploads"))
}

// LocalFilePath converts stored file paths or public static URLs back to a
// server-local upload path. Older rows may contain /api/static/... or
// uploads/api/static/... instead of the real filesystem path.
func LocalFilePath(storedPath string) string {
	path := strings.TrimSpace(storedPath)
	if path == "" {
		return ""
	}

	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path = parsed.Path
	}
	path = strings.Split(path, "?")[0]
	path = strings.Split(path, "#")[0]
	slashPath := filepath.ToSlash(path)

	if idx := strings.Index(slashPath, "/api/static/"); idx >= 0 {
		return filepath.Join(UploadDir(), slashPath[idx+len("/api/static/"):])
	}
	if strings.HasPrefix(slashPath, "api/static/") {
		return filepath.Join(UploadDir(), strings.TrimPrefix(slashPath, "api/static/"))
	}
	if strings.HasPrefix(slashPath, "/uploads/") {
		return filepath.Join(UploadDir(), strings.TrimPrefix(slashPath, "/uploads/"))
	}
	if strings.HasPrefix(slashPath, "uploads/") {
		return filepath.Join(UploadDir(), strings.TrimPrefix(slashPath, "uploads/"))
	}

	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	if _, err := os.Stat(cleaned); err == nil {
		return cleaned
	}
	return filepath.Join(UploadDir(), filepath.Base(cleaned))
}

// StaticFileURL returns the public URL served by the Hertz static route.
func StaticFileURL(storedPath string) string {
	localPath := LocalFilePath(storedPath)
	if localPath == "" {
		return ""
	}
	return "/api/static/" + filepath.Base(localPath)
}

// DownloadURL returns the authenticated API URL for a contract file. Contract
// bytes must never be exposed through a public static directory.
func DownloadURL(contractID uint64) string {
	if contractID == 0 {
		return ""
	}
	return fmt.Sprintf("/api/contract/download/%d", contractID)
}
