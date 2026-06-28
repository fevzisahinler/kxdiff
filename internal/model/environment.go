// Package model holds kxdiff's immutable domain types. Nothing here performs
// I/O; resolving a value against the kubeconfig lives in internal/config.
package model

// Environment identifies where to look: a kube context and a namespace.
// It is an immutable value — build a new one rather than mutating it.
type Environment struct {
	// Context is the kubeconfig context name. Empty means the current context.
	Context string
	// Namespace is the namespace to compare. Empty means all namespaces.
	Namespace string
}
