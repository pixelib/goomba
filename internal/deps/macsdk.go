package deps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"goomba/internal/embeddedsk"
)

type MacSDK struct {
	Path string
}

func EnsureMacSDK(ctx context.Context) (MacSDK, error) {
	cacheRoot, err := cacheDir()
	if err != nil {
		return MacSDK{}, err
	}
	installDir := filepath.Join(cacheRoot, "macos-sdk")
	
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		if !embeddedsk.IsAvailable() {
			return MacSDK{}, fmt.Errorf("this build of goomba doesn't support macOS targets...")
		}
		fmt.Println(">> Extracting embedded Apple SDK...")
		if err := embeddedsk.Extract(installDir); err != nil {
			return MacSDK{}, fmt.Errorf("failed to extract embedded SDK: %w", err)
		}
	}

	return MacSDK{Path: installDir}, nil
}
