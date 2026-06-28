package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNewRootCmd_Metadata(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "1.2.3"})

	if cmd.Use == "" {
		t.Error("Use should be set")
	}
	if cmd.Short == "" {
		t.Error("Short description should be set")
	}
	for _, name := range []string{"from", "to"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s flag to be defined", name)
		}
	}
}

func TestRootCmd_Help(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help should not error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "kxdiff") {
		t.Errorf("help output missing tool name; got:\n%s", got)
	}
}

func TestRootCmd_Version(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-06-27"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version should not error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"1.2.3", "abc123", "2026-06-27"} {
		if !strings.Contains(got, want) {
			t.Errorf("version output missing %q; got: %s", want, got)
		}
	}
}

// The skeleton must fail loudly until the engine is implemented.
func TestRootCmd_StubReturnsNotImplemented(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if !errors.Is(err, errNotImplemented) {
		t.Fatalf("expected errNotImplemented, got: %v", err)
	}
}
