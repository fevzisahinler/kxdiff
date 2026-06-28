package match

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func obj(apiVersion, kind, ns, name string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{}
	o.SetAPIVersion(apiVersion)
	o.SetKind(kind)
	o.SetNamespace(ns)
	o.SetName(name)
	return o
}

func names(objs []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, o.GetKind()+"/"+o.GetName())
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMatch_SplitsBuckets(t *testing.T) {
	from := []*unstructured.Unstructured{
		obj("v1", "ConfigMap", "demo", "app"),
		obj("v1", "ConfigMap", "demo", "legacy"),
		obj("apps/v1", "Deployment", "demo", "web"),
	}
	to := []*unstructured.Unstructured{
		obj("v1", "ConfigMap", "demo", "app"),
		obj("v1", "ConfigMap", "demo", "newfeature"),
		obj("apps/v1", "Deployment", "demo", "web"),
	}

	res := Match(from, to, false)

	if got := names(res.OnlyFrom); !equal(got, []string{"ConfigMap/legacy"}) {
		t.Errorf("OnlyFrom = %v, want [ConfigMap/legacy]", got)
	}
	if got := names(res.OnlyTo); !equal(got, []string{"ConfigMap/newfeature"}) {
		t.Errorf("OnlyTo = %v, want [ConfigMap/newfeature]", got)
	}
	if len(res.Both) != 2 {
		t.Fatalf("Both = %d, want 2", len(res.Both))
	}
}

func TestMatch_IgnoresNamespaceByDefault(t *testing.T) {
	// Same kind+name, different namespaces -> should still match.
	from := []*unstructured.Unstructured{obj("apps/v1", "Deployment", "ns-x", "web")}
	to := []*unstructured.Unstructured{obj("apps/v1", "Deployment", "ns-y", "web")}

	res := Match(from, to, false)
	if len(res.Both) != 1 || len(res.OnlyFrom) != 0 || len(res.OnlyTo) != 0 {
		t.Errorf("expected 1 match ignoring namespace; got both=%d onlyFrom=%d onlyTo=%d",
			len(res.Both), len(res.OnlyFrom), len(res.OnlyTo))
	}
}

func TestMatch_IncludesNamespaceWhenRequested(t *testing.T) {
	from := []*unstructured.Unstructured{obj("apps/v1", "Deployment", "ns-x", "web")}
	to := []*unstructured.Unstructured{obj("apps/v1", "Deployment", "ns-y", "web")}

	res := Match(from, to, true)
	if len(res.Both) != 0 || len(res.OnlyFrom) != 1 || len(res.OnlyTo) != 1 {
		t.Errorf("with namespace in key, different namespaces should not match; got both=%d onlyFrom=%d onlyTo=%d",
			len(res.Both), len(res.OnlyFrom), len(res.OnlyTo))
	}
}
