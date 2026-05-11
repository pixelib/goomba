package build

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"goomba/internal/deps"
	"goomba/internal/tui"
	"goomba/internal/util"
)

func Run(ctx context.Context, cfg Config) error {
	start := time.Now()
	ui := tui.New(!cfg.NoTui, os.Stdout)
	if cfg.Verbose {
		ui.SetLogLimit(50)
	}
	defer ui.Close()

	preparePhase := ui.NewPhase("preparing golang", 1)
	goTool, err := deps.EnsureGo(ctx, cfg.GoVersion)
	if err != nil {
		preparePhase.Fail(err)
		return err
	}
	preparePhase.Advance()
	preparePhase.Done()

	if !cfg.NoValidation {
		validationPhase := ui.NewPhase("validation", 1)
		if err := runValidation(ctx, cfg, goTool, validationPhase); err != nil {
			validationPhase.Fail(err)
			return err
		}
		validationPhase.Advance()
		validationPhase.Done()
	}

	targets, err := BuildMatrix(cfg.Platforms, cfg.Archs)
	if err != nil {
		return err
	}

	cgoOn, err := cgoEnabled(ctx, goTool)
	if err != nil {
		return err
	}

	reqs := deps.Requirements{CgoEnabled: cgoOn}
	for _, target := range targets {
		if cgoOn && (target.GOOS != runtime.GOOS || target.GOARCH != runtime.GOARCH) {
			reqs.NeedZig = true
		}
		if cgoOn && target.GOOS == runtime.GOOS && target.GOARCH == runtime.GOARCH {
			if !deps.HasCCompiler() {
				reqs.NeedZig = true
			}
		}
		if target.GOOS == "darwin" && os.Getenv("SDKROOT") == "" {
			reqs.NeedMacSDK = true
		}
	}

	var zigTool *deps.ZigTool
	var macSDK *deps.MacSDK
	if reqs.Any() {
		downPhase := ui.NewPhase("Preparing toolchain...", 0)
		if reqs.NeedZig {
			if cfg.NoTui {
				fmt.Fprintln(os.Stdout, ">> Preparing zig toolchain...")
			}
			downPhase.Log("Preparing zig toolchain...")
			tool, err := deps.EnsureZig(ctx)
			if err != nil {
				downPhase.Fail(err)
				return err
			}
			zigTool = &tool
			downPhase.Advance()
		}
		if reqs.NeedMacSDK {
			if cfg.NoTui {
				fmt.Fprintln(os.Stdout, ">> Preparing macOS SDK...")
			}
			sdk, err := deps.EnsureMacSDK(ctx)
			downPhase.Log("Preparing macOS SDK...")
			if err != nil {
				downPhase.Fail(err)
				return err
			}
			macSDK = &sdk
			downPhase.Advance()
		}
		downPhase.Done()
	}

	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		labels = append(labels, buildDisplayLabel(target))
	}
	ui.BuildStart(labels)
	defer ui.BuildEnd()

	var errs []string

	if cfg.NoParallel || len(targets) < 2 {
		for _, target := range targets {
			label := buildDisplayLabel(target)
			ui.BuildUpdate(label, "running", "")
			if err := buildTarget(ctx, cfg, goTool, target, zigTool, macSDK, ui, label); err != nil {
				ui.BuildUpdate(label, "error", err.Error())
				errs = append(errs, fmt.Sprintf("%s: %v", label, err))
				continue
			}
			ui.BuildUpdate(label, "done", "")
		}
		printSummary(cfg, len(targets), errs, time.Since(start))
		return handleBuildErrors(cfg, errs)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(targets))
	for _, target := range targets {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()
			label := buildDisplayLabel(target)
			ui.BuildUpdate(label, "running", "")
			if err := buildTarget(ctx, cfg, goTool, target, zigTool, macSDK, ui, label); err != nil {
				ui.BuildUpdate(label, "error", err.Error())
				errCh <- fmt.Errorf("%s: %w", label, err)
				return
			}
			ui.BuildUpdate(label, "done", "")
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		errs = append(errs, err.Error())
	}
	printSummary(cfg, len(targets), errs, time.Since(start))
	return handleBuildErrors(cfg, errs)
}

func printSummary(cfg Config, total int, errs []string, duration time.Duration) {
	failed := len(errs)
	succeeded := total - failed
	status := "ok"
	if failed > 0 {
		status = "partial"
	}
	if cfg.Strict && failed > 0 {
		status = "failed"
	}

	fmt.Fprintf(os.Stdout, "summary: %s, total=%d, succeeded=%d, failed=%d, elapsed=%s\n", status, total, succeeded, failed, duration.Round(time.Second))
	if cfg.Strict && failed > 0 {
		fmt.Fprintf(os.Stdout, "summary: output removed due to strict mode (%s)\n", outputBaseLabel(cfg))
	}
}

func handleBuildErrors(cfg Config, errs []string) error {
	if len(errs) == 0 {
		return nil
	}

	if !cfg.Strict {
		fmt.Fprintf(os.Stderr, "skipped %d target(s) due to errors\n", len(errs))
		return nil
	}

	if err := os.RemoveAll(outputBaseDir(cfg)); err != nil {
		return err
	}
	return errors.New(strings.Join(errs, "; "))
}

func runValidation(ctx context.Context, cfg Config, goTool deps.GoTool, phase *tui.Phase) error {
	if len(cfg.ValidateCmd) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, goTool.Bin, cfg.ValidateCmd[1:]...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = util.EnvWithOverrides(os.Environ(), goTool.EnvOverrides())
	if cfg.JavaHome != "" {
		cmd.Env = util.EnvWithOverrides(cmd.Env, map[string]string{"JAVA_HOME": cfg.JavaHome})
	}
	if cfg.CgoEnabled {
		cmd.Env = util.EnvWithOverrides(cmd.Env, map[string]string{"CGO_ENABLED": "1"})
	}
	if env := jniCgoEnv(runtime.GOOS, cmd.Env); len(env) > 0 {
		cmd.Env = util.EnvWithOverrides(cmd.Env, env)
	}
	if cfg.CgoEnabled {
		if env := deps.CgoEnv(runtime.GOOS, runtime.GOARCH, nil, nil); len(env) > 0 {
			cmd.Env = util.EnvWithOverrides(cmd.Env, env)
		}
	}
	if cfg.NoTui {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return runCommandWithLogs(cmd, phase.Log, cfg.Verbose, []string{
		"GOROOT", "GOPATH", "GO111MODULE", "GOWORK", "GOPROXY", "CGO_ENABLED",
	})
}

func buildTarget(ctx context.Context, cfg Config, goTool deps.GoTool, target Target, zigTool *deps.ZigTool, macSDK *deps.MacSDK, ui *tui.UI, label string) error {
	outDir := filepath.Join(outputBaseDir(cfg), target.Label, target.GOARCH)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	binName := filepath.Base(cfg.WorkDir)
	if target.GOOS == "windows" {
		binName += ".exe"
	}
	outPath := filepath.Join(outDir, binName)

	args := []string{"build", "-o", outPath}
	args = append(args, expandArgs(cfg.GoArgs, target)...)
	args = append(args, ".")

	cmd := exec.CommandContext(ctx, goTool.Bin, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = util.EnvWithOverrides(os.Environ(), goTool.EnvOverrides())
	if cfg.JavaHome != "" {
		cmd.Env = util.EnvWithOverrides(cmd.Env, map[string]string{"JAVA_HOME": cfg.JavaHome})
	}
	if cfg.CgoEnabled {
		cmd.Env = util.EnvWithOverrides(cmd.Env, map[string]string{"CGO_ENABLED": "1"})
	}

	overrides := map[string]string{
		"GOOS":            target.GOOS,
		"GOARCH":          target.GOARCH,
		"GOOMBA_OS":       target.GOOS,
		"GOOMBA_ARCH":     target.GOARCH,
		"GOOMBA_PLATFORM": target.Label,
	}
	if env := deps.CgoEnv(target.GOOS, target.GOARCH, zigTool, macSDK); len(env) > 0 {
		for k, v := range env {
			overrides[k] = v
		}
	}
	if env := jniCgoEnv(target.GOOS, cmd.Env); len(env) > 0 {
		for k, v := range env {
			overrides[k] = v
		}
	}

	for key, val := range overrides {
		overrides[key] = expandPlaceholders(val, target)
	}
	cmd.Env = util.EnvWithOverrides(cmd.Env, overrides)
	if cfg.NoTui {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return runCommandWithLogs(cmd, func(line string) { ui.BuildLog(label, line) }, cfg.Verbose, []string{
		"GOOS", "GOARCH", "CGO_ENABLED", "CC", "CXX", "SDKROOT", "CGO_CFLAGS", "CGO_LDFLAGS",
		"GOROOT", "GOPATH", "GO111MODULE", "GOWORK", "GOPROXY", "JAVA_HOME",
		"ZIG_GLOBAL_CACHE_DIR", "ZIG_LOCAL_CACHE_DIR",
	})
}

func buildDisplayLabel(target Target) string {
	return fmt.Sprintf("Compiling os:%s arch:%s", target.Label, target.GOARCH)
}

func outputBaseDir(cfg Config) string {
	base := strings.TrimSpace(cfg.OutputBase)
	if base == "" {
		base = "dist"
	}
	if filepath.IsAbs(base) {
		return filepath.Clean(base)
	}
	return filepath.Clean(filepath.Join(cfg.WorkDir, base))
}

func outputBaseLabel(cfg Config) string {
	base := strings.TrimSpace(cfg.OutputBase)
	if base == "" {
		return "dist"
	}
	return base
}

func runCommandWithLogs(cmd *exec.Cmd, logFn func(string), verbose bool, envKeys []string) error {
	if verbose && logFn != nil {
		logFn("cmd: " + strings.Join(cmd.Args, " "))
		if cmd.Dir != "" {
			logFn("dir: " + cmd.Dir)
		}
		if len(envKeys) > 0 {
			for _, entry := range pickEnv(cmd.Env, envKeys) {
				logFn("env: " + entry)
			}
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	logReader := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			logFn(scanner.Text())
		}
		wg.Done()
	}

	wg.Add(2)
	go logReader(stdout)
	go logReader(stderr)
	if err := cmd.Wait(); err != nil {
		wg.Wait()
		if verbose && logFn != nil {
			logFn("error: " + err.Error() + "; With cmd: " + strings.Join(cmd.Args, " "))
		}
		return err
	}
	wg.Wait()
	return nil
}

func expandArgs(args []string, target Target) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, expandPlaceholders(arg, target))
	}
	return out
}

func expandPlaceholders(value string, target Target) string {
	replacer := strings.NewReplacer(
		"${GOOMBA_OS}", target.GOOS,
		"${GOOMBA_ARCH}", target.GOARCH,
		"${GOOMBA_PLATFORM}", target.Label,
	)
	return replacer.Replace(value)
}

func jniCgoEnv(goos string, env []string) map[string]string {
	if value, ok := envLookup(env, "CGO_ENABLED"); ok && value == "0" {
		return nil
	}
	javaHome, ok := envLookup(env, "JAVA_HOME")
	if !ok || javaHome == "" {
		return nil
	}
	includeDir := filepath.Join(javaHome, "include")
	if _, err := os.Stat(includeDir); err != nil {
		return nil
	}
	osInclude := jniOsInclude(goos)
	if osInclude == "" {
		return nil
	}
	osIncludeDir := filepath.Join(includeDir, osInclude)
	if _, err := os.Stat(osIncludeDir); err != nil {
		return nil
	}

	base := ""
	if value, ok := envLookup(env, "CGO_CFLAGS"); ok {
		base = value
	}
	flags := appendCFlags(base, includeDir, osIncludeDir)
	return map[string]string{"CGO_CFLAGS": flags}
}

func jniOsInclude(goos string) string {
	switch goos {
	case "darwin":
		return "darwin"
	case "windows":
		return "win32"
	case "linux":
		return "linux"
	default:
		return ""
	}
}

func appendCFlags(existing string, includeDirs ...string) string {
	flags := strings.TrimSpace(existing)
	for _, dir := range includeDirs {
		flags = strings.TrimSpace(flags + " -I" + dir)
	}
	return strings.TrimSpace(flags)
}

func envLookup(entries []string, key string) (string, bool) {
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == key {
			return parts[1], true
		}
	}
	return "", false
}

func pickEnv(entries, keys []string) []string {
	if len(entries) == 0 || len(keys) == 0 {
		return nil
	}
	lookup := map[string]bool{}
	for _, key := range keys {
		lookup[key] = true
	}
	var out []string
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if lookup[parts[0]] {
			out = append(out, entry)
		}
	}
	return out
}
