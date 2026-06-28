package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fevzisahinler/kxdiff/internal/config"
)

// testKubeconfig is an injected, deterministic kubeconfig for the resolution
// tests — no real file or cluster involved.
var testKubeconfig = config.Kubeconfig{
	Contexts:       []string{"eks-prod", "eks-staging"},
	CurrentContext: "eks-staging",
}

func TestNewRootCmd_Metadata(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "1.2.3"})

	if cmd.Use == "" {
		t.Error("Use should be set")
	}
	if cmd.Short == "" {
		t.Error("Short description should be set")
	}
	for _, name := range []string{"from", "to", "kubeconfig"} {
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

func TestResolveBoth_OK(t *testing.T) {
	fromEnv, toEnv, err := resolveBoth(testKubeconfig, "eks-staging/ns-x", "eks-prod/ns-y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fromEnv.Context != "eks-staging" || fromEnv.Namespace != "ns-x" {
		t.Errorf("from = %+v", fromEnv)
	}
	if toEnv.Context != "eks-prod" || toEnv.Namespace != "ns-y" {
		t.Errorf("to = %+v", toEnv)
	}
}

func TestResolveBoth_FromError(t *testing.T) {
	_, _, err := resolveBoth(testKubeconfig, "ghost/ns", "eks-prod")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "--from") {
		t.Errorf("error should mention --from: %v", err)
	}
}

func TestResolveBoth_ToError(t *testing.T) {
	_, _, err := resolveBoth(testKubeconfig, "eks-staging", "ghost/ns")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "--to") {
		t.Errorf("error should mention --to: %v", err)
	}
}

func TestViewSections(t *testing.T) {
	if f, to, d := (viewOptions{}).sections(); !f || !to || !d {
		t.Errorf("default should show all sections, got %v/%v/%v", f, to, d)
	}
	if f, to, d := (viewOptions{onlyDiff: true}).sections(); f || to || !d {
		t.Errorf("only-diff should show diff only, got %v/%v/%v", f, to, d)
	}
	if f, to, d := (viewOptions{onlyFrom: true, onlyTo: true}).sections(); !f || !to || d {
		t.Errorf("only-from+only-to should hide diff, got %v/%v/%v", f, to, d)
	}
}

func TestLineDiff(t *testing.T) {
	removed, added := lineDiff("a\nb\nc", "a\nx\nc")
	if len(removed) != 1 || removed[0] != "b" {
		t.Errorf("removed = %v, want [b]", removed)
	}
	if len(added) != 1 || added[0] != "x" {
		t.Errorf("added = %v, want [x]", added)
	}
}

func TestPalette(t *testing.T) {
	on := palette{enabled: true}
	if got := on.red("x"); got != "\x1b[31mx\x1b[0m" {
		t.Errorf("enabled red = %q", got)
	}

	off := palette{enabled: false}
	for _, got := range []string{off.red("x"), off.green("x"), off.yellow("x"), off.bold("x")} {
		if got != "x" {
			t.Errorf("disabled palette should be plain, got %q", got)
		}
	}
}
