// Package diff computes the field-level differences between two normalized
// objects. It is pure: same input, same output, no side effects. From/To in
// each FieldDiff carry the raw values (typed), with an absent side as nil;
// formatting for display is the renderer's job.
package diff

import (
	"fmt"
	"reflect"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/fevzisahinler/kxdiff/internal/model"
)

// Objects compares two objects and returns the fields that differ, sorted by
// path. Lists whose elements all have a "name" are matched by name (so
// containers/env/volumes don't show spurious diffs when reordered); other lists
// of maps are matched positionally.
func Objects(from, to *unstructured.Unstructured) []model.FieldDiff {
	var diffs []model.FieldDiff
	walk("", from.Object, to.Object, &diffs)
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Path < diffs[j].Path })
	return diffs
}

func walk(path string, from, to any, diffs *[]model.FieldDiff) {
	if fromMap, ok := from.(map[string]any); ok {
		if toMap, ok := to.(map[string]any); ok {
			walkMaps(path, fromMap, toMap, diffs)
			return
		}
	}
	if fromSlice, ok := from.([]any); ok {
		if toSlice, ok := to.([]any); ok {
			walkSlices(path, fromSlice, toSlice, diffs)
			return
		}
	}
	if !reflect.DeepEqual(from, to) {
		*diffs = append(*diffs, model.FieldDiff{Path: path, From: from, To: to})
	}
}

func walkMaps(path string, from, to map[string]any, diffs *[]model.FieldDiff) {
	for _, key := range unionKeys(from, to) {
		fv, fok := from[key]
		tv, tok := to[key]
		child := joinPath(path, key)
		switch {
		case fok && tok:
			walk(child, fv, tv, diffs)
		case fok:
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: fv, To: nil})
		default:
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: nil, To: tv})
		}
	}
}

func walkSlices(path string, from, to []any, diffs *[]model.FieldDiff) {
	switch {
	case isNamedList(from) && isNamedList(to):
		walkNamedSlices(path, from, to, diffs)
	case isMapList(from) && isMapList(to):
		walkIndexedSlices(path, from, to, diffs)
	default:
		if !reflect.DeepEqual(from, to) {
			*diffs = append(*diffs, model.FieldDiff{Path: path, From: from, To: to})
		}
	}
}

// walkNamedSlices matches list elements by their "name" field.
func walkNamedSlices(path string, from, to []any, diffs *[]model.FieldDiff) {
	fromByName, toByName := byName(from), byName(to)
	for _, name := range unionKeys(fromByName, toByName) {
		fv, fok := fromByName[name]
		tv, tok := toByName[name]
		child := fmt.Sprintf("%s[%s]", path, name)
		switch {
		case fok && tok:
			walk(child, fv, tv, diffs)
		case fok:
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: fv, To: nil})
		default:
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: nil, To: tv})
		}
	}
}

// walkIndexedSlices matches list-of-maps elements positionally — used for lists
// whose elements have no "name" (e.g. RBAC rules).
func walkIndexedSlices(path string, from, to []any, diffs *[]model.FieldDiff) {
	n := len(from)
	if len(to) > n {
		n = len(to)
	}
	for i := 0; i < n; i++ {
		child := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case i < len(from) && i < len(to):
			walk(child, from[i], to[i], diffs)
		case i < len(from):
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: from[i], To: nil})
		default:
			*diffs = append(*diffs, model.FieldDiff{Path: child, From: nil, To: to[i]})
		}
	}
}

// isNamedList reports whether every element is a map carrying a string "name".
func isNamedList(list []any) bool {
	if len(list) == 0 {
		return false
	}
	for _, el := range list {
		m, ok := el.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := m["name"].(string); !ok {
			return false
		}
	}
	return true
}

// isMapList reports whether every element is a map (a list of objects).
func isMapList(list []any) bool {
	if len(list) == 0 {
		return false
	}
	for _, el := range list {
		if _, ok := el.(map[string]any); !ok {
			return false
		}
	}
	return true
}

func byName(list []any) map[string]any {
	out := make(map[string]any, len(list))
	for _, el := range list {
		m := el.(map[string]any) // safe: callers guard with isNamedList
		out[m["name"].(string)] = el
	}
	return out
}

func unionKeys(a, b map[string]any) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}
