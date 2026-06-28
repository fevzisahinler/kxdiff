package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestConfig_FromContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(fakeKubeconfig), 0o600); err != nil {
		t.Fatalf("writing temp kubeconfig: %v", err)
	}

	rc, err := RestConfig(path, "eks-prod")
	if err != nil {
		t.Fatalf("RestConfig: %v", err)
	}
	if rc.Host != "https://prod.example" {
		t.Errorf("Host = %q, want %q", rc.Host, "https://prod.example")
	}
	if rc.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want %v", rc.Timeout, defaultTimeout)
	}
}

func TestRestConfig_UnknownContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(fakeKubeconfig), 0o600); err != nil {
		t.Fatalf("writing temp kubeconfig: %v", err)
	}

	if _, err := RestConfig(path, "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown context")
	}
}
