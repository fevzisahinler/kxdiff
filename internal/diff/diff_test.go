package diff

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func u(m map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: m}
}

func TestObjects_MapScalars(t *testing.T) {
	from := u(map[string]any{
		"data": map[string]any{"DB_HOST": "dev-db", "LOG_LEVEL": "debug", "SAME": "x"},
	})
	to := u(map[string]any{
		"data": map[string]any{"DB_HOST": "prod-db", "LOG_LEVEL": "info", "SAME": "x"},
	})

	diffs := Objects(from, to)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %+v", diffs)
	}
	// sorted by path: data.DB_HOST, data.LOG_LEVEL
	if diffs[0].Path != "data.DB_HOST" || diffs[0].From != "dev-db" || diffs[0].To != "prod-db" {
		t.Errorf("unexpected diff[0]: %+v", diffs[0])
	}
	if diffs[1].Path != "data.LOG_LEVEL" {
		t.Errorf("unexpected diff[1] path: %q", diffs[1].Path)
	}
}

func TestObjects_NamedListMatchedByName(t *testing.T) {
	mk := func(image string) *unstructured.Unstructured {
		return u(map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "web", "image": image},
				},
			},
		})
	}

	diffs := Objects(mk("nginx:1.25"), mk("nginx:1.27"))
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %+v", diffs)
	}
	if diffs[0].Path != "spec.containers[web].image" {
		t.Errorf("path = %q, want spec.containers[web].image", diffs[0].Path)
	}
	if diffs[0].From != "nginx:1.25" || diffs[0].To != "nginx:1.27" {
		t.Errorf("unexpected values: %+v", diffs[0])
	}
}

func TestObjects_AddedAndRemoved(t *testing.T) {
	from := u(map[string]any{"data": map[string]any{"only_from": "1"}})
	to := u(map[string]any{"data": map[string]any{"only_to": "2"}})

	diffs := Objects(from, to)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %+v", diffs)
	}
	if diffs[0].To != absent {
		t.Errorf("only_from should be absent on the to side: %+v", diffs[0])
	}
	if diffs[1].From != absent {
		t.Errorf("only_to should be absent on the from side: %+v", diffs[1])
	}
}

func TestObjects_IndexedMapListAndReadableScalars(t *testing.T) {
	mk := func(resources, verbs []any) *unstructured.Unstructured {
		return u(map[string]any{
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": resources,
					"verbs":     verbs,
				},
			},
		})
	}
	from := mk([]any{"configmaps"}, []any{"get", "list"})
	to := mk([]any{"configmaps", "secrets"}, []any{"get", "list", "watch"})

	diffs := Objects(from, to)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %+v", diffs)
	}
	if diffs[0].Path != "rules[0].resources" {
		t.Errorf("path = %q, want rules[0].resources", diffs[0].Path)
	}
	if diffs[0].From != "[configmaps]" || diffs[0].To != "[configmaps, secrets]" {
		t.Errorf("readable scalar list expected, got %+v", diffs[0])
	}
	if diffs[1].Path != "rules[0].verbs" {
		t.Errorf("path = %q, want rules[0].verbs", diffs[1].Path)
	}
}

func TestObjects_Identical(t *testing.T) {
	o := u(map[string]any{"spec": map[string]any{"replicas": int64(3)}})
	if d := Objects(o, o); len(d) != 0 {
		t.Errorf("identical objects should have no diff, got %+v", d)
	}
}
