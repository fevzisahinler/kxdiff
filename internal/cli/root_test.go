package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/fevzisahinler/kxdiff/internal/config"
)

// testKubeconfig is an injected, deterministic kubeconfig for runDiff tests —
// no real file or cluster involved.
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

func TestRunDiff_ResolvesContexts(t *testing.T) {
	var out bytes.Buffer
	if err := runDiff(&out, testKubeconfig, "eks-staging/ns-x", "eks-prod/ns-y"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"eks-staging", "ns-x", "eks-prod", "ns-y"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRunDiff_BareNamespaceUsesCurrentContext(t *testing.T) {
	var out bytes.Buffer
	if err := runDiff(&out, testKubeconfig, "staging", "prod"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	// bare token => namespace; context resolves to the current context
	for _, want := range []string{"eks-staging", "staging", "prod"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRunDiff_UnknownContextSuggests(t *testing.T) {
	err := runDiff(io.Discard, testKubeconfig, "eks-stagin/ns", "eks-prod/ns")
	if err == nil {
		t.Fatal("expected an error for an unknown context")
	}
	msg := err.Error()
	for _, want := range []string{"did you mean", "eks-staging"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
}

func TestRunDiff_InvalidEnvironment(t *testing.T) {
	if err := runDiff(io.Discard, testKubeconfig, "a/b/c", "eks-prod"); err == nil {
		t.Fatal("expected an error for an invalid environment")
	}
}
