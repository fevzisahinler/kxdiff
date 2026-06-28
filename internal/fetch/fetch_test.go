package fetch

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fevzisahinler/kxdiff/internal/discovery"
	"github.com/fevzisahinler/kxdiff/internal/model"
)

type fakeLister struct {
	lists map[schema.GroupVersionResource]*unstructured.UnstructuredList
	errs  map[schema.GroupVersionResource]error
}

func (f fakeLister) List(_ context.Context, gvr schema.GroupVersionResource, _ string) (*unstructured.UnstructuredList, error) {
	if err := f.errs[gvr]; err != nil {
		return nil, err
	}
	if l, ok := f.lists[gvr]; ok {
		return l, nil
	}
	return &unstructured.UnstructuredList{}, nil
}

func obj(apiVersion, kind, ns, name string) unstructured.Unstructured {
	var o unstructured.Unstructured
	o.SetAPIVersion(apiVersion)
	o.SetKind(kind)
	o.SetNamespace(ns)
	o.SetName(name)
	return o
}

func gvr(group, version, resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
}

func kinds(objs []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, o.GetKind()+"/"+o.GetName())
	}
	return out
}

func TestFetch_CollectsAndSkips(t *testing.T) {
	lister := fakeLister{
		lists: map[schema.GroupVersionResource]*unstructured.UnstructuredList{
			gvr("apps", "v1", "deployments"): {Items: []unstructured.Unstructured{obj("apps/v1", "Deployment", "demo", "web")}},
			gvr("", "v1", "configmaps"):      {Items: []unstructured.Unstructured{obj("v1", "ConfigMap", "demo", "app")}},
		},
		errs: map[schema.GroupVersionResource]error{
			gvr("", "v1", "secrets"): apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("nope")),
		},
	}

	types := []discovery.ResourceType{
		{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true},
		{Version: "v1", Resource: "secrets", Kind: "Secret", Namespaced: true},
		{Version: "v1", Resource: "nodes", Kind: "Node", Namespaced: false}, // cluster-scoped: excluded for a namespaced env
	}

	env := model.Environment{Context: "kind-dev", Namespace: "demo"}
	res, err := Fetch(context.Background(), lister, env, types, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %v", kinds(res.Objects))
	}
	if len(res.Warnings) != 1 {
		t.Errorf("expected 1 warning for forbidden secrets, got %v", res.Warnings)
	}
}

func TestFetch_FatalErrorPropagates(t *testing.T) {
	lister := fakeLister{
		errs: map[schema.GroupVersionResource]error{
			gvr("", "v1", "configmaps"): errors.New("connection reset by peer"), // not skippable
		},
	}
	types := []discovery.ResourceType{
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true},
	}

	if _, err := Fetch(context.Background(), lister, model.Environment{Namespace: "demo"}, types, Options{}); err == nil {
		t.Fatal("expected a non-skippable error to propagate")
	}
}

func TestFetch_AllNamespacesIncludesClusterScoped(t *testing.T) {
	lister := fakeLister{
		lists: map[schema.GroupVersionResource]*unstructured.UnstructuredList{
			gvr("", "v1", "nodes"): {Items: []unstructured.Unstructured{obj("v1", "Node", "", "node-1")}},
		},
	}
	types := []discovery.ResourceType{
		{Version: "v1", Resource: "nodes", Kind: "Node", Namespaced: false},
	}

	res, err := Fetch(context.Background(), lister, model.Environment{Context: "kind-dev"}, types, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Objects) != 1 || res.Objects[0].GetKind() != "Node" {
		t.Errorf("expected the cluster-scoped Node, got %v", kinds(res.Objects))
	}
}

func TestFetch_FiltersGeneratedTypesAndObjects(t *testing.T) {
	owned := obj("v1", "ConfigMap", "demo", "child")
	owned.SetOwnerReferences([]metav1.OwnerReference{{Kind: "Foo", Name: "parent"}})

	lister := fakeLister{
		lists: map[schema.GroupVersionResource]*unstructured.UnstructuredList{
			gvr("apps", "v1", "deployments"): {Items: []unstructured.Unstructured{obj("apps/v1", "Deployment", "demo", "web")}},
			gvr("", "v1", "events"):          {Items: []unstructured.Unstructured{obj("v1", "Event", "demo", "evt1")}},  // generated type
			gvr("", "v1", "pods"):            {Items: []unstructured.Unstructured{obj("v1", "Pod", "demo", "web-xyz")}}, // generated type
			gvr("", "v1", "configmaps"): {Items: []unstructured.Unstructured{
				obj("v1", "ConfigMap", "demo", "app"),              // keep
				obj("v1", "ConfigMap", "demo", "kube-root-ca.crt"), // system: drop
				owned, // owned: drop
			}},
			gvr("", "v1", "serviceaccounts"): {Items: []unstructured.Unstructured{obj("v1", "ServiceAccount", "demo", "default")}}, // system: drop
		},
	}
	types := []discovery.ResourceType{
		{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
		{Version: "v1", Resource: "events", Kind: "Event", Namespaced: true},
		{Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true},
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true},
		{Version: "v1", Resource: "serviceaccounts", Kind: "ServiceAccount", Namespaced: true},
	}
	env := model.Environment{Namespace: "demo"}

	// Default: generated types + owned/system objects are filtered out.
	res, err := Fetch(context.Background(), lister, env, types, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := kinds(res.Objects)
	want := []string{"Deployment/web", "ConfigMap/app"}
	if len(got) != len(want) {
		t.Fatalf("default filter: got %v, want %v", got, want)
	}

	// IncludeGenerated: everything comes back.
	resAll, err := Fetch(context.Background(), lister, env, types, Options{IncludeGenerated: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resAll.Objects) != 7 {
		t.Errorf("IncludeGenerated: expected 7 objects, got %v", kinds(resAll.Objects))
	}
}

func TestFetch_TypeSelector(t *testing.T) {
	lister := fakeLister{
		lists: map[schema.GroupVersionResource]*unstructured.UnstructuredList{
			gvr("apps", "v1", "deployments"): {Items: []unstructured.Unstructured{obj("apps/v1", "Deployment", "demo", "web")}},
			gvr("", "v1", "configmaps"):      {Items: []unstructured.Unstructured{obj("v1", "ConfigMap", "demo", "app")}},
		},
	}
	types := []discovery.ResourceType{
		{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Singular: "deployment", ShortNames: []string{"deploy"}, Namespaced: true},
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Singular: "configmap", ShortNames: []string{"cm"}, Namespaced: true},
	}
	env := model.Environment{Namespace: "demo"}

	res, err := Fetch(context.Background(), lister, env, types, Options{Selectors: []discovery.Selector{{Type: "deploy"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := kinds(res.Objects); len(got) != 1 || got[0] != "Deployment/web" {
		t.Errorf("type selector: got %v, want [Deployment/web]", got)
	}
}

func TestFetch_NameSelector(t *testing.T) {
	lister := fakeLister{
		lists: map[schema.GroupVersionResource]*unstructured.UnstructuredList{
			gvr("", "v1", "configmaps"): {Items: []unstructured.Unstructured{
				obj("v1", "ConfigMap", "demo", "app"),
				obj("v1", "ConfigMap", "demo", "other"),
			}},
		},
	}
	types := []discovery.ResourceType{
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Singular: "configmap", Namespaced: true},
	}

	res, err := Fetch(context.Background(), lister, model.Environment{Namespace: "demo"}, types,
		Options{Selectors: []discovery.Selector{{Type: "configmap", Name: "app"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := kinds(res.Objects); len(got) != 1 || got[0] != "ConfigMap/app" {
		t.Errorf("name selector: got %v, want [ConfigMap/app]", got)
	}
}

func TestFetch_UnknownSelector(t *testing.T) {
	types := []discovery.ResourceType{
		{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true},
	}
	_, err := Fetch(context.Background(), fakeLister{}, model.Environment{Namespace: "demo"}, types,
		Options{Selectors: []discovery.Selector{{Type: "nonsense"}}})
	if err == nil {
		t.Fatal("expected an error for an unknown resource type")
	}
}
