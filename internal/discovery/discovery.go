// Package discovery enumerates the resource types a cluster exposes via the
// Kubernetes discovery API. It is the basis of kxdiff's "diff everything"
// behaviour: built-ins, CRDs and custom resources are all found dynamically.
package discovery

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sdiscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// ResourceType is one listable resource type exposed by a cluster.
type ResourceType struct {
	Group      string
	Version    string
	Resource   string // plural name, e.g. "deployments"
	Kind       string
	Namespaced bool
	Singular   string   // singular name, e.g. "deployment"
	ShortNames []string // e.g. ["deploy"]
}

// Selector is a parsed TYPE[/NAME] positional argument.
type Selector struct {
	Type string // resource, kind, singular, short name, or "resource.group"
	Name string // optional specific object name ("" = all of this type)
}

// Matches reports whether token refers to this type. token may be the plural
// resource, the singular, the kind, a short name, or "resource.group" — all
// case-insensitive, the way kubectl resolves a type.
func (rt ResourceType) Matches(token string) bool {
	t := strings.ToLower(token)
	switch t {
	case strings.ToLower(rt.Resource), strings.ToLower(rt.Singular), strings.ToLower(rt.Kind):
		return true
	}
	for _, sn := range rt.ShortNames {
		if t == strings.ToLower(sn) {
			return true
		}
	}
	return rt.Group != "" && t == strings.ToLower(rt.Resource+"."+rt.Group)
}

// GroupVersion renders the "group/version" (or just "version" for core types).
func (rt ResourceType) GroupVersion() string {
	if rt.Group == "" {
		return rt.Version
	}
	return rt.Group + "/" + rt.Version
}

// GroupVersionResource returns the GVR used to query the dynamic client.
func (rt ResourceType) GroupVersionResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: rt.Group, Version: rt.Version, Resource: rt.Resource}
}

// String renders the type for display, e.g. "deployments (apps/v1, namespaced)".
func (rt ResourceType) String() string {
	scope := "cluster"
	if rt.Namespaced {
		scope = "namespaced"
	}
	return fmt.Sprintf("%s (%s, %s)", rt.Resource, rt.GroupVersion(), scope)
}

// Result is the outcome of discovery: the types found plus any non-fatal
// warnings (for example, API groups whose discovery failed).
type Result struct {
	Types    []ResourceType
	Warnings []string
}

// ListResourceTypes connects to the cluster described by rc and returns every
// resource type that supports listing.
//
// Partial failures — some API groups unreachable, common with broken aggregated
// API services — are returned as warnings alongside the types that were found.
// A total failure (cannot reach or authenticate to the cluster) returns an
// error and no types.
func ListResourceTypes(rc *rest.Config) (Result, error) {
	dc, err := k8sdiscovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return Result{}, fmt.Errorf("creating discovery client: %w", err)
	}

	lists, err := dc.ServerPreferredResources()
	if err != nil {
		var groupErr *k8sdiscovery.ErrGroupDiscoveryFailed
		if !errors.As(err, &groupErr) {
			return Result{}, fmt.Errorf("listing server resources: %w", err)
		}
		// Partial discovery: keep the lists we got, record the rest as warnings.
		return Result{
			Types:    filterListable(lists),
			Warnings: groupWarnings(groupErr),
		}, nil
	}

	return Result{Types: filterListable(lists)}, nil
}

// groupWarnings turns failed API groups into sorted, human-readable warnings.
func groupWarnings(err *k8sdiscovery.ErrGroupDiscoveryFailed) []string {
	out := make([]string, 0, len(err.Groups))
	for gv := range err.Groups {
		out = append(out, fmt.Sprintf("skipped API group %q (discovery failed)", gv.String()))
	}
	sort.Strings(out)
	return out
}

// filterListable keeps only listable, non-subresource types and sorts them
// deterministically. It is pure, so it carries the unit tests.
func filterListable(lists []*metav1.APIResourceList) []ResourceType {
	var out []ResourceType
	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue // skip a malformed group/version rather than failing
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // subresource, e.g. pods/status
			}
			if !canList(r.Verbs) {
				continue
			}
			out = append(out, ResourceType{
				Group:      gv.Group,
				Version:    gv.Version,
				Resource:   r.Name,
				Kind:       r.Kind,
				Namespaced: r.Namespaced,
				Singular:   r.SingularName,
				ShortNames: r.ShortNames,
			})
		}
	}
	sortTypes(out)
	return out
}

func canList(verbs metav1.Verbs) bool {
	for _, v := range verbs {
		if v == "list" {
			return true
		}
	}
	return false
}

func sortTypes(types []ResourceType) {
	sort.Slice(types, func(i, j int) bool {
		a, b := types[i], types[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Resource < b.Resource
	})
}
