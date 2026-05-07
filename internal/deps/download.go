package deps

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

func downloadGo(ctx context.Context, version, cacheRoot string) (string, error) {
	archiveName := fmt.Sprintf("go%s.%s-%s", version, runtime.GOOS, runtime.GOARCH)
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	archiveName += ext
	url := fmt.Sprintf("https://go.dev/dl/%s", archiveName)
	return downloadFile(ctx, url, filepath.Join(cacheRoot, "downloads", archiveName))
}

func downloadZig(ctx context.Context, version, cacheRoot string) (string, error) {
	platform := zigPlatform()
	archiveName := fmt.Sprintf("zig-%s-%s", platform, version)
	ext := ".tar.xz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	archiveName += ext
	url := fmt.Sprintf("https://ziglang.org/download/%s/%s", version, archiveName)
	return downloadFile(ctx, url, filepath.Join(cacheRoot, "downloads", archiveName))
}

func downloadMacSDK(ctx context.Context, version, cacheRoot string) (string, error) {
	archiveName := fmt.Sprintf("MacOSX%s.sdk.tar.xz", version)
	url := fmt.Sprintf("https://github.com/phracker/MacOSX-SDKs/releases/download/%s/%s", version, archiveName)
	return downloadFile(ctx, url, filepath.Join(cacheRoot, "downloads", archiveName))
}

func downloadFile(ctx context.Context, url, dest string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	file, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}

func zigPlatform() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "darwin" {
		goos = "macos"
	}
	if goarch == "amd64" {
		goarch = "x86_64"
	}
	if goarch == "arm64" {
		goarch = "aarch64"
	}
	return fmt.Sprintf("%s-%s", goos, goarch)
}

func zigBinName() string {
	if runtime.GOOS == "windows" {
		return "zig.exe"
	}
	return "zig"
}

func zigCc(zigPath, goos, goarch string) string {
	return fmt.Sprintf("%s cc -target %s", zigPath, zigTarget(goos, goarch))
}

func zigCxx(zigPath, goos, goarch string) string {
	return fmt.Sprintf("%s c++ -target %s", zigPath, zigTarget(goos, goarch))
}

func zigTarget(goos, goarch string) string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	osPart := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[goos]
	if goos == "darwin" {
		return fmt.Sprintf("%s-%s", arch, osPart)
	}
	return fmt.Sprintf("%s-%s-gnu", arch, osPart)
}
