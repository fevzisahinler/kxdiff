// Package model holds kxdiff's immutable domain types and their pure
// constructors. Nothing in this package performs I/O.
package model

import (
	"errors"
	"fmt"
	"strings"
)

// errEmptyEnvironment is returned when a --from/--to value is blank.
var errEmptyEnvironment = errors.New("environment must not be empty")

// Environment identifies where to look: a kube context and a namespace.
// It is an immutable value — build a new one rather than mutating it.
type Environment struct {
	// Context is the kubeconfig context name. Empty means the current context.
	Context string
	// Namespace is the namespace to compare. Empty means all namespaces.
	Namespace string
}

// ParseEnvironment turns a --from/--to argument into an Environment.
//
// Accepted forms (surrounding whitespace is ignored):
//
//	"ns"      -> namespace "ns" in the current context
//	"ctx/ns"  -> namespace "ns" in context "ctx"
//	"ctx/"    -> context "ctx", across all namespaces
//	"/ns"     -> namespace "ns" in the current context
//
// A bare token is treated as a namespace (the common same-cluster
// staging<->prod case); use a trailing slash to name a context instead.
func ParseEnvironment(raw string) (Environment, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Environment{}, errEmptyEnvironment
	}

	parts := strings.Split(s, "/")
	switch len(parts) {
	case 1:
		// Bare token: a namespace in the current context.
		return Environment{Namespace: strings.TrimSpace(parts[0])}, nil
	case 2:
		return Environment{
			Context:   strings.TrimSpace(parts[0]),
			Namespace: strings.TrimSpace(parts[1]),
		}, nil
	default:
		return Environment{}, fmt.Errorf(
			"invalid environment %q: expected [context]/[namespace]", raw)
	}
}
