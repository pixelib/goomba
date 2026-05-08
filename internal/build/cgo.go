package build

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"goomba/internal/deps"
)

func cgoEnabled(ctx context.Context, goTool deps.GoTool) (bool, error) {
	if value, ok := os.LookupEnv("CGO_ENABLED"); ok {
		return value == "1", nil
	}
	cmd := exec.CommandContext(ctx, goTool.Bin, "env", "CGO_ENABLED")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "1", nil
}
