// Package config resolves a parsed Environment against the user's kubeconfig:
// validating contexts, applying the current-context fallback and suggesting
// corrections for typos. Loading the kubeconfig itself lives in a separate file.
package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxSuggestionDistance is the largest edit distance for which a mistyped
// context name still earns a "did you mean" suggestion.
const maxSuggestionDistance = 3

var (
	errNoContexts = errors.New("no contexts found in kubeconfig")
	errNoCurrent  = errors.New("no current context is set; pass a context explicitly")
)

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
