package normalize

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNormalize_StripsNoiseAndKeepsMeaning(t *testing.T) {
	in := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":              "app",
			"namespace":         "demo",
			"resourceVersion":   "123",
			"uid":               "abc",
			"creationTimestamp": "2020-01-01T00:00:00Z",
			"managedFields":     []interface{}{map[string]interface{}{"x": "y"}},
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": "{...}",
				"keep-me": "yes",
			},
		},
		"data":   map[string]interface{}{"k": "v"},
		"status": map[string]interface{}{"phase": "x"},
	}}

	out := Normalize(in, Options{DropNamespace: true})

	md, _, _ := unstructured.NestedMap(out.Object, "metadata")
	for _, gone := range []string{"resourceVersion", "uid", "creationTimestamp", "managedFields", "namespace"} {
		if _, ok := md[gone]; ok {
			t.Errorf("metadata.%s should be stripped", gone)
		}
	}
	if _, ok := out.Object["status"]; ok {
		t.Error("status should be stripped")
	}

	ann, _, _ := unstructured.NestedStringMap(out.Object, "metadata", "annotations")
	if _, ok := ann["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Error("last-applied annotation should be stripped")
	}
	if ann["keep-me"] != "yes" {
		t.Error("meaningful annotations must be kept")
	}

	// Input must not be mutated.
	if _, ok := in.Object["status"]; !ok {
		t.Error("input object was mutated (status removed)")
	}
}

func TestNormalize_StripsServiceRuntimeFields(t *testing.T) {
	in := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "api"},
		"spec": map[string]any{
			"clusterIP":  "10.96.0.1",
			"clusterIPs": []any{"10.96.0.1"},
			"ports": []any{
				map[string]any{"name": "http", "port": int64(80), "nodePort": int64(30080)},
			},
		},
	}}

	out := Normalize(in, Options{})
	spec, _, _ := unstructured.NestedMap(out.Object, "spec")
	if _, ok := spec["clusterIP"]; ok {
		t.Error("clusterIP should be stripped")
	}
	if _, ok := spec["clusterIPs"]; ok {
		t.Error("clusterIPs should be stripped")
	}

	ports, _, _ := unstructured.NestedSlice(out.Object, "spec", "ports")
	port := ports[0].(map[string]any)
	if _, ok := port["nodePort"]; ok {
		t.Error("nodePort should be stripped")
	}
	if _, ok := port["port"]; !ok {
		t.Error("port should be kept")
	}
}

func TestNormalize_MasksSecretsByDefault(t *testing.T) {
	const raw = "c3VwZXItc2VjcmV0"
	in := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "s"},
		"data":       map[string]interface{}{"password": raw},
	}}

	masked := Normalize(in, Options{})
	md, _, _ := unstructured.NestedStringMap(masked.Object, "data")
	if md["password"] == raw {
		t.Error("secret value must be masked by default")
	}
	if md["password"] == "" {
		t.Error("masked value (hash) should be present")
	}

	revealed := Normalize(in, Options{RevealSecrets: true})
	rd, _, _ := unstructured.NestedStringMap(revealed.Object, "data")
	if rd["password"] != raw {
		t.Error("with RevealSecrets the raw value must be kept")
	}
}
