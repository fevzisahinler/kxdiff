package config

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

const fakeKubeconfig = `apiVersion: v1
kind: Config
current-context: eks-staging
clusters:
- cluster:
    server: https://staging.example
  name: staging-cluster
- cluster:
    server: https://prod.example
  name: prod-cluster
contexts:
- context:
    cluster: staging-cluster
    namespace: default
  name: eks-staging
- context:
    cluster: prod-cluster
    namespace: default
  name: eks-prod
users: []
`

func TestFromAPIConfig(t *testing.T) {
	cfg, err := clientcmd.Load([]byte(fakeKubeconfig))
	if err != nil {
		t.Fatalf("loading fake kubeconfig: %v", err)
	}

	kc := fromAPIConfig(cfg)

	if kc.CurrentContext != "eks-staging" {
		t.Errorf("CurrentContext = %q, want %q", kc.CurrentContext, "eks-staging")
	}
	want := []string{"eks-prod", "eks-staging"} // sorted
	if len(kc.Contexts) != len(want) {
		t.Fatalf("Contexts = %v, want %v", kc.Contexts, want)
	}
	for i := range want {
		if kc.Contexts[i] != want[i] {
			t.Errorf("Contexts[%d] = %q, want %q", i, kc.Contexts[i], want[i])
		}
	}
}

func TestLoadKubeconfig_FromExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(fakeKubeconfig), 0o600); err != nil {
		t.Fatalf("writing temp kubeconfig: %v", err)
	}

	kc, err := LoadKubeconfig(path)
	if err != nil {
		t.Fatalf("LoadKubeconfig: %v", err)
	}
	if kc.CurrentContext != "eks-staging" {
		t.Errorf("CurrentContext = %q, want %q", kc.CurrentContext, "eks-staging")
	}
	if len(kc.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %v", kc.Contexts)
	}
}

func TestLoadKubeconfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist")

	if _, err := LoadKubeconfig(path); err == nil {
		t.Fatal("expected an error for a missing kubeconfig file")
	}
}
