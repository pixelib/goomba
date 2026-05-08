package embeddedsk

import (
	"archive/zip"
	"bytes"
	"embed"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed sdk*
var Data embed.FS

func IsAvailable() bool {
	_, err := Data.ReadFile("sdk.zip")
	return err == nil
}

func Extract(dest string) error {
	zipData, err := Data.ReadFile("sdk.zip")
	if err != nil {
		return err
	}

	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return repairFrameworks(dest)
}

func repairFrameworks(sdkRoot string) error {
	frameworksDir := filepath.Join(sdkRoot, "System", "Library", "Frameworks")
	entries, err := os.ReadDir(frameworksDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".framework") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".framework")
		base := filepath.Join(frameworksDir, entry.Name())
		versionsA := filepath.Join("Versions", "A")

		tbdSource := filepath.Join(versionsA, name+".tbd")
		if _, err := os.Stat(filepath.Join(base, tbdSource)); err == nil {
			// Link: CoreFoundation.framework/CoreFoundation -> Versions/A/CoreFoundation.tbd
			link(tbdSource, filepath.Join(base, name))
			// Link: CoreFoundation.framework/CoreFoundation.tbd -> Versions/A/CoreFoundation.tbd
			link(tbdSource, filepath.Join(base, name+".tbd"))
		}

		headersSource := filepath.Join(versionsA, "Headers")
		if _, err := os.Stat(filepath.Join(base, headersSource)); err == nil {
			// Link: CoreFoundation.framework/Headers -> Versions/A/Headers
			link(headersSource, filepath.Join(base, "Headers"))
		}
	}
	return nil
}

func link(target, linkName string) {
	if _, err := os.Lstat(linkName); os.IsNotExist(err) {
		_ = os.Symlink(target, linkName)
	}
}
