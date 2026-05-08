package util

import (
	"os"
	"strings"
	"testing"
)

func TestEnvWithOverrides(t *testing.T) {
	base := []string{"FOO=bar", "KEEP=ok", "EMPTY="}
	overrides := map[string]string{
		"FOO":   "override",
		"NEW":   "value",
		"EMPTY": "set",
	}
	got := EnvWithOverrides(base, overrides)
	joined := strings.Join(got, "\n")

	if !strings.Contains(joined, "FOO=override") {
		t.Fatalf("expected FOO override, got %v", got)
	}
	if !strings.Contains(joined, "KEEP=ok") {
		t.Fatalf("expected KEEP to be preserved, got %v", got)
	}
	if !strings.Contains(joined, "NEW=value") {
		t.Fatalf("expected NEW to be appended, got %v", got)
	}
	if !strings.Contains(joined, "EMPTY=set") {
		t.Fatalf("expected EMPTY override, got %v", got)
	}
}

func TestEnvWithOverridesInheritsParent(t *testing.T) {
	// Use a unique key to avoid clobbering real environment entries.
	key := "GOOMBA_ENV_TEST"
	if err := os.Setenv(key, "parent"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer os.Unsetenv(key)

	got := EnvWithOverrides(os.Environ(), map[string]string{
		key: "child",
	})
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, key+"=child") {
		t.Fatalf("expected override for %s, got %v", key, got)
	}

	// Spot check that some other parent env key is retained.
	if len(os.Environ()) == 0 {
		return
	}
	parts := strings.SplitN(os.Environ()[0], "=", 2)
	if len(parts) != 2 {
		return
	}
	if !strings.Contains(joined, parts[0]+"=") {
		t.Fatalf("expected parent env key %s preserved, got %v", parts[0], got)
	}
}
