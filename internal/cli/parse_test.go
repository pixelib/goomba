package cli

import "testing"

func TestSplitList(t *testing.T) {
	if got := splitList(""); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
	got := splitList(" linux, macos, ,windows ")
	want := []string{"linux", "macos", "windows"}
	if len(got) != len(want) {
		t.Fatalf("expected %d items, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSplitArgs(t *testing.T) {
	input := "-tags \"foo bar\" -ldflags='-s -w' -x"
	got, err := splitArgs(input)
	if err != nil {
		t.Fatalf("split args: %v", err)
	}
	want := []string{"-tags", "foo bar", "-ldflags=-s -w", "-x"}
	if len(got) != len(want) {
		t.Fatalf("expected %d args, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSplitArgsEscapes(t *testing.T) {
	input := "-ldflags=-s\\ -w -o\\ out"
	got, err := splitArgs(input)
	if err != nil {
		t.Fatalf("split args: %v", err)
	}
	want := []string{"-ldflags=-s -w", "-o out"}
	if len(got) != len(want) {
		t.Fatalf("expected %d args, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSplitArgsErrors(t *testing.T) {
	cases := []string{
		"-tags \"foo",
		"-tags foo\\",
	}
	for _, input := range cases {
		if _, err := splitArgs(input); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}
