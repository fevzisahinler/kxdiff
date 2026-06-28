// Package normalize strips server-generated noise from objects so that diffs
// show only meaningful differences. It never mutates its input (deep copy).
package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Options controls normalization.
type Options struct {
	// DropNamespace removes metadata.namespace so namespace<->namespace
	// comparisons don't report the (expected) namespace difference.
	DropNamespace bool
	// RevealSecrets, when true, leaves Secret values untouched. By default
	// (false) Secret values are hashed so raw secrets never reach the output.
	RevealSecrets bool
}

// strippedMetadata are server-managed metadata fields that are pure noise.
var strippedMetadata = []string{
	"managedFields", "resourceVersion", "uid", "creationTimestamp",
	"generation", "selfLink",
}

// noiseAnnotations are well-known controller annotations that are not
// meaningful to diff. Everything else under annotations is kept.
var noiseAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
	"deployment.kubernetes.io/revision",
}

// Normalize returns a cleaned deep copy of obj. The input is never mutated.
func Normalize(obj *unstructured.Unstructured, opts Options) *unstructured.Unstructured {
	out := obj.DeepCopy()

	for _, f := range strippedMetadata {
		unstructured.RemoveNestedField(out.Object, "metadata", f)
	}
	for _, a := range noiseAnnotations {
		unstructured.RemoveNestedField(out.Object, "metadata", "annotations", a)
	}
	unstructured.RemoveNestedField(out.Object, "status")
	if opts.DropNamespace {
		unstructured.RemoveNestedField(out.Object, "metadata", "namespace")
	}

	stripRuntimeFields(out)
	if !opts.RevealSecrets && out.GetKind() == "Secret" {
		maskSecretData(out)
	}
	return out
}

// stripRuntimeFields removes spec fields the API server assigns at runtime,
// which differ between clusters but are not meaningful to diff (currently the
// Service cluster IPs and node ports).
func stripRuntimeFields(o *unstructured.Unstructured) {
	if o.GetKind() != "Service" {
		return
	}
	unstructured.RemoveNestedField(o.Object, "spec", "clusterIP")
	unstructured.RemoveNestedField(o.Object, "spec", "clusterIPs")

	ports, found, err := unstructured.NestedSlice(o.Object, "spec", "ports")
	if err != nil || !found {
		return
	}
	for _, p := range ports {
		if m, ok := p.(map[string]any); ok {
			delete(m, "nodePort")
		}
	}
	_ = unstructured.SetNestedSlice(o.Object, ports, "spec", "ports")
}

// maskSecretData replaces Secret data/stringData values with short hashes, so
// equal secrets compare equal without ever exposing the raw value.
func maskSecretData(o *unstructured.Unstructured) {
	for _, field := range []string{"data", "stringData"} {
		m, found, err := unstructured.NestedMap(o.Object, field)
		if err != nil || !found {
			continue
		}
		masked := make(map[string]interface{}, len(m))
		for k, v := range m {
			masked[k] = hashValue(v)
		}
		_ = unstructured.SetNestedMap(o.Object, masked, field)
	}
}

func hashValue(v interface{}) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v", v)))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}
