// Package match pairs the objects of two environments by identity, splitting
// them into "only in from", "only in to" and "in both". It is pure: no I/O.
package match

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Pair is an object present in both environments, matched by identity.
type Pair struct {
	From *unstructured.Unstructured
	To   *unstructured.Unstructured
}

// Result groups matched objects into the three diff buckets.
type Result struct {
	OnlyFrom []*unstructured.Unstructured
	OnlyTo   []*unstructured.Unstructured
	Both     []Pair
}

// Match pairs from/to objects by identity.
//
// When includeNamespace is true the namespace is part of the identity (used for
// whole-context comparisons across all namespaces); otherwise it is ignored, so
// that namespace<->namespace comparisons across differently-named namespaces
// still line up by GVK + name.
func Match(from, to []*unstructured.Unstructured, includeNamespace bool) Result {
	toByKey := make(map[string]*unstructured.Unstructured, len(to))
	for _, o := range to {
		toByKey[identity(o, includeNamespace)] = o
	}

	matched := make(map[string]bool, len(to))
	var res Result

	for _, f := range from {
		key := identity(f, includeNamespace)
		if t, ok := toByKey[key]; ok {
			res.Both = append(res.Both, Pair{From: f, To: t})
			matched[key] = true
		} else {
			res.OnlyFrom = append(res.OnlyFrom, f)
		}
	}

	for _, t := range to {
		if !matched[identity(t, includeNamespace)] {
			res.OnlyTo = append(res.OnlyTo, t)
		}
	}

	sortObjects(res.OnlyFrom)
	sortObjects(res.OnlyTo)
	sortPairs(res.Both)
	return res
}

// identity is the match key for an object.
func identity(o *unstructured.Unstructured, includeNamespace bool) string {
	gvk := o.GroupVersionKind()
	base := gvk.Group + "/" + gvk.Version + "/" + gvk.Kind
	if includeNamespace {
		return base + "/" + o.GetNamespace() + "/" + o.GetName()
	}
	return base + "/" + o.GetName()
}

// sortKey is a stable ordering key (always includes namespace) for output.
func sortKey(o *unstructured.Unstructured) string {
	return identity(o, true)
}

func sortObjects(objs []*unstructured.Unstructured) {
	sort.Slice(objs, func(i, j int) bool { return sortKey(objs[i]) < sortKey(objs[j]) })
}

func sortPairs(pairs []Pair) {
	sort.Slice(pairs, func(i, j int) bool { return sortKey(pairs[i].From) < sortKey(pairs[j].From) })
}
