package discovery

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sdiscovery "k8s.io/client-go/discovery"
)

func TestFilterListable(t *testing.T) {
	lists := []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "watch"}},
				{Name: "namespaces", Kind: "Namespace", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "bindings", Kind: "Binding", Namespaced: true, Verbs: metav1.Verbs{"create"}},     // no list -> skip
				{Name: "pods/status", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}}, // subresource -> skip
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
		{
			GroupVersion: "stable.example.com/v1",
			APIResources: []metav1.APIResource{
				{Name: "crontabs", Kind: "CronTab", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
		nil, // skipped safely
		{
			GroupVersion: "a/b/c", // malformed -> skipped
			APIResources: []metav1.APIResource{{Name: "x", Verbs: metav1.Verbs{"list"}}},
		},
	}

	got := filterListable(lists)

	want := []ResourceType{
		{Group: "", Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true},
		{Group: "", Version: "v1", Resource: "namespaces", Kind: "Namespace", Namespaced: false},
		{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
		{Group: "stable.example.com", Version: "v1", Resource: "crontabs", Kind: "CronTab", Namespaced: true},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d types, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("type[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResourceTypeString(t *testing.T) {
	cases := []struct {
		rt   ResourceType
		want string
	}{
		{ResourceType{Version: "v1", Resource: "namespaces", Namespaced: false}, "namespaces (v1, cluster)"},
		{ResourceType{Group: "apps", Version: "v1", Resource: "deployments", Namespaced: true}, "deployments (apps/v1, namespaced)"},
		{ResourceType{Group: "stable.example.com", Version: "v1", Resource: "crontabs", Namespaced: true}, "crontabs (stable.example.com/v1, namespaced)"},
	}
	for _, c := range cases {
		if got := c.rt.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestResourceTypeMatches(t *testing.T) {
	deploy := ResourceType{
		Group: "apps", Version: "v1", Resource: "deployments",
		Kind: "Deployment", Singular: "deployment", ShortNames: []string{"deploy"},
	}
	for _, token := range []string{"deployments", "deployment", "Deployment", "deploy", "deployments.apps", "DEPLOY"} {
		if !deploy.Matches(token) {
			t.Errorf("deployment should match %q", token)
		}
	}
	for _, token := range []string{"svc", "pods", "deployment.extensions"} {
		if deploy.Matches(token) {
			t.Errorf("deployment should NOT match %q", token)
		}
	}

	cr := ResourceType{Group: "stable.example.com", Version: "v1", Resource: "crontabs", Kind: "CronTab", Singular: "crontab"}
	for _, token := range []string{"crontabs", "crontab", "crontabs.stable.example.com"} {
		if !cr.Matches(token) {
			t.Errorf("crontab should match %q", token)
		}
	}
}

func TestGroupWarnings(t *testing.T) {
	err := &k8sdiscovery.ErrGroupDiscoveryFailed{
		Groups: map[schema.GroupVersion]error{
			{Group: "metrics.k8s.io", Version: "v1beta1"}: errors.New("the server is currently unable to handle the request"),
		},
	}

	got := groupWarnings(err)
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got %v", got)
	}
	if !strings.Contains(got[0], "metrics.k8s.io/v1beta1") {
		t.Errorf("warning should name the failed group: %q", got[0])
	}
}
