// Package config resolves a parsed Environment against the user's kubeconfig:
// validating contexts, applying the current-context fallback and suggesting
// corrections for typos. Loading the kubeconfig itself lives in a separate file.
package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/fevzisahinler/kxdiff/internal/model"
)

// maxSuggestionDistance is the largest edit distance for which a mistyped
// context name still earns a "did you mean" suggestion.
const maxSuggestionDistance = 3

var (
	errNoContexts       = errors.New("no contexts found in kubeconfig")
	errNoCurrent        = errors.New("no current context is set; pass a context explicitly")
	errEmptyEnvironment = errors.New("environment must not be empty")
)

// ResolveEnvironment turns a raw --from/--to value into a fully-resolved
// Environment. Context names are matched against the kubeconfig first, so names
// that themselves contain slashes (e.g. EKS ARNs) are handled correctly.
//
// Resolution order (surrounding whitespace is ignored):
//
//	"/ns"         -> namespace "ns" in the current context
//	"<ctx>"       -> context "<ctx>", all namespaces (whole value is a context)
//	"<ctx>/<ns>"  -> namespace "ns" in context "<ctx>" (longest context match)
//	"<ns>"        -> namespace "ns" in the current context (bare token)
//	otherwise     -> error, suggesting the closest context
func ResolveEnvironment(kc Kubeconfig, raw string) (model.Environment, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return model.Environment{}, errEmptyEnvironment
	}

	// Explicit namespace in the current context.
	if rest, ok := strings.CutPrefix(s, "/"); ok {
		return inCurrentContext(kc, rest)
	}

	// The whole value is a known context (handles ARN names with slashes).
	if contains(kc.Contexts, s) {
		return model.Environment{Context: s}, nil
	}

	// A known context followed by "/<namespace>" (longest match wins).
	if ctx, ns, ok := splitByKnownContext(s, kc.Contexts); ok {
		return model.Environment{Context: ctx, Namespace: ns}, nil
	}

	// No context matched and there is no separator: a bare namespace.
	if !strings.Contains(s, "/") {
		return inCurrentContext(kc, s)
	}

	// Looks like "<context>/..." but no context matched: unknown context.
	if suggestion, ok := closestMatch(s, kc.Contexts); ok {
		return model.Environment{}, fmt.Errorf("no context matches %q; did you mean %q", s, suggestion)
	}
	return model.Environment{}, fmt.Errorf("no context matches %q; available contexts: %s",
		s, strings.Join(sortedCopy(kc.Contexts), ", "))
}

// inCurrentContext builds an Environment for namespace ns in the kubeconfig's
// current context, validating that a current context actually exists.
func inCurrentContext(kc Kubeconfig, ns string) (model.Environment, error) {
	ctx, err := ResolveContext(kc.Contexts, kc.CurrentContext, "")
	if err != nil {
		return model.Environment{}, err
	}
	return model.Environment{Context: ctx, Namespace: ns}, nil
}

// splitByKnownContext finds the longest known context c such that s is
// "c/<namespace>", returning c, the namespace and whether a match was found.
func splitByKnownContext(s string, contexts []string) (string, string, bool) {
	best, bestNS := "", ""
	for _, c := range contexts {
		rest, ok := strings.CutPrefix(s, c+"/")
		if ok && len(c) > len(best) {
			best, bestNS = c, rest
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, bestNS, true
}

// ResolveContext picks the kube context to use.
//
// requested is the context the user asked for ("" means "use the current
// context"). available is the set of context names defined in the kubeconfig
// and current is its current-context. It returns the resolved context name or
// an explanatory error.
func ResolveContext(available []string, current, requested string) (string, error) {
	if len(available) == 0 {
		return "", errNoContexts
	}

	if requested == "" {
		if current == "" {
			return "", errNoCurrent
		}
		if !contains(available, current) {
			return "", fmt.Errorf("current context %q is not defined in kubeconfig", current)
		}
		return current, nil
	}

	if contains(available, requested) {
		return requested, nil
	}

	if suggestion, ok := closestMatch(requested, available); ok {
		return "", fmt.Errorf("context %q not found; did you mean %q", requested, suggestion)
	}
	return "", fmt.Errorf("context %q not found; available contexts: %s",
		requested, strings.Join(sortedCopy(available), ", "))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// sortedCopy returns a sorted copy, leaving the input untouched (immutability).
func sortedCopy(list []string) []string {
	out := make([]string, len(list))
	copy(out, list)
	sort.Strings(out)
	return out
}

// closestMatch returns the available name nearest to target by edit distance,
// and whether it is close enough to be worth suggesting.
func closestMatch(target string, available []string) (string, bool) {
	best := ""
	bestDist := -1
	for _, name := range sortedCopy(available) { // sorted => deterministic ties
		d := levenshtein(target, name)
		if bestDist == -1 || d < bestDist {
			bestDist, best = d, name
		}
	}
	if bestDist >= 0 && bestDist <= maxSuggestionDistance {
		return best, true
	}
	return "", false
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
