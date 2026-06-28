// Package fetch pulls live objects from a cluster with the dynamic client.
//
// It is strictly read-only by construction: it depends on a Lister that only
// exposes List, so create/update/delete simply cannot be called from here.
package fetch

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/fevzisahinler/kxdiff/internal/discovery"
	"github.com/fevzisahinler/kxdiff/internal/model"
)

// maxConcurrentLists bounds parallel List calls so large clusters don't get
// hammered (and we don't get throttled).
const maxConcurrentLists = 12

// generatedResources are controller-managed, high-churn types that are noise in
// a diff. They are skipped by default (use Options.IncludeGenerated to keep
// them). Skipping "events" by resource name also drops the duplicate that the
// core and events.k8s.io groups would otherwise both return.
var generatedResources = map[string]bool{
	"events":         true,
	"pods":           true,
	"replicasets":    true,
	"endpoints":      true,
	"endpointslices": true,
}

// Options tunes what Fetch pulls.
type Options struct {
	// IncludeGenerated keeps controller-managed and system objects that are
	// filtered out by default (Pods, ReplicaSets, Events, owned objects, the
	// default ServiceAccount, the kube-root-ca.crt ConfigMap, ...).
	IncludeGenerated bool
}

// Lister is the read-only slice of the dynamic client that fetch needs.
// Depending on this — rather than dynamic.Interface — makes write verbs
// impossible to call: the read-only guarantee holds at compile time.
type Lister interface {
	List(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error)
}

// Result is the outcome of a fetch: the objects pulled plus any non-fatal
// warnings (resource types skipped because of, say, forbidden access).
type Result struct {
	Objects  []*unstructured.Unstructured
	Warnings []string
}

// Fetch pulls every object of the given types from the environment.
//
// With env.Namespace set, only namespaced types are fetched (from that
// namespace); with it empty, namespaced types are fetched across all namespaces
// and cluster-scoped types cluster-wide. Listing runs in parallel with bounded
// concurrency. Types the caller cannot list (forbidden), that no longer exist,
// or that do not support listing are skipped with a warning rather than failing
// the whole fetch. Generated/system noise is dropped unless opts says otherwise.
func Fetch(ctx context.Context, lister Lister, env model.Environment, types []discovery.ResourceType, opts Options) (Result, error) {
	selected := selectTypes(types, env.Namespace != "", opts.IncludeGenerated)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentLists)

	var (
		mu       sync.Mutex
		objects  []*unstructured.Unstructured
		warnings []string
	)

	for _, rt := range selected {
		g.Go(func() error {
			ns := ""
			if rt.Namespaced {
				ns = env.Namespace
			}

			list, err := lister.List(ctx, rt.GroupVersionResource(), ns)
			if err != nil {
				if isSkippable(err) {
					mu.Lock()
					warnings = append(warnings, fmt.Sprintf("skipped %s: %v", rt.Resource, err))
					mu.Unlock()
					return nil
				}
				return fmt.Errorf("listing %s: %w", rt.Resource, err)
			}

			mu.Lock()
			for i := range list.Items {
				item := &list.Items[i]
				if opts.IncludeGenerated || keepObject(item) {
					objects = append(objects, item)
				}
			}
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return Result{}, err
	}

	sortObjects(objects)
	sort.Strings(warnings)
	return Result{Objects: objects, Warnings: warnings}, nil
}

// selectTypes chooses which resource types to fetch: only namespaced types when
// a namespace is set, and (by default) excluding generated noise types.
func selectTypes(types []discovery.ResourceType, namespaceSet, includeGenerated bool) []discovery.ResourceType {
	out := make([]discovery.ResourceType, 0, len(types))
	for _, t := range types {
		if namespaceSet && !t.Namespaced {
			continue
		}
		if !includeGenerated && generatedResources[t.Resource] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// keepObject reports whether an object is meaningful to diff: controller-owned
// objects and well-known system objects are dropped.
func keepObject(o *unstructured.Unstructured) bool {
	if len(o.GetOwnerReferences()) > 0 {
		return false // created by a controller, not the user
	}
	return !isSystemObject(o)
}

// isSystemObject matches per-namespace objects the API server injects itself.
func isSystemObject(o *unstructured.Unstructured) bool {
	switch o.GetKind() {
	case "ServiceAccount":
		return o.GetName() == "default"
	case "ConfigMap":
		return o.GetName() == "kube-root-ca.crt"
	}
	return false
}

// isSkippable reports whether a List error is an expected, non-fatal condition
// (no access, type gone, listing unsupported) that should be skipped.
func isSkippable(err error) bool {
	return apierrors.IsForbidden(err) ||
		apierrors.IsNotFound(err) ||
		apierrors.IsMethodNotSupported(err)
}

func sortObjects(objs []*unstructured.Unstructured) {
	sort.Slice(objs, func(i, j int) bool {
		return objectKey(objs[i]) < objectKey(objs[j])
	})
}

func objectKey(o *unstructured.Unstructured) string {
	gvk := o.GroupVersionKind()
	return gvk.Group + "/" + gvk.Version + "/" + gvk.Kind + "/" + o.GetNamespace() + "/" + o.GetName()
}

// dynamicLister adapts a dynamic.Interface to the read-only Lister.
type dynamicLister struct {
	client dynamic.Interface
}

// NewLister returns a read-only Lister backed by an existing dynamic client.
func NewLister(client dynamic.Interface) Lister {
	return dynamicLister{client: client}
}

// NewListerForConfig builds a dynamic client from rc and wraps it as a Lister.
func NewListerForConfig(rc *rest.Config) (Lister, error) {
	client, err := dynamic.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	return NewLister(client), nil
}

func (d dynamicLister) List(ctx context.Context, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	ri := d.client.Resource(gvr)
	if namespace != "" {
		return ri.Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	return ri.List(ctx, metav1.ListOptions{})
}
