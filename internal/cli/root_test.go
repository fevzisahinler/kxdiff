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
