package cli

import (
	"bytes"
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

func TestRootCmd_ParsesFromAndTo(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--from", "ctx-a/ns-x", "--to", "ctx-b/ns-y"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"ctx-a", "ns-x", "ctx-b", "ns-y"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRootCmd_RequiresFromAndTo(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--from", "staging"}) // --to missing

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error when --to is missing")
	}
}

func TestRootCmd_RejectsInvalidEnvironment(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--from", "a/b/c", "--to", "prod"}) // --from invalid

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an invalid --from value")
	}
}
