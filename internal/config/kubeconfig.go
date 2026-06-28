package config

import (
	"fmt"
	"sort"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Kubeconfig is the subset of the user's kubeconfig that kxdiff needs in order
// to resolve environments: the available context names and the current one.
type Kubeconfig struct {
	Contexts       []string
	CurrentContext string
}

// LoadKubeconfig reads the kubeconfig using standard resolution: the explicit
// path when non-empty, otherwise the KUBECONFIG environment variable (which may
// merge several files) and finally the default ~/.kube/config. It only reads.
func LoadKubeconfig(explicitPath string) (Kubeconfig, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}

	cfg, err := rules.Load()
	if err != nil {
		return Kubeconfig{}, fmt.Errorf("loading kubeconfig: %w", err)
	}
	return fromAPIConfig(cfg), nil
}

// fromAPIConfig extracts the context names (sorted for determinism) and the
// current context from a loaded kubeconfig. It is pure and carries the tests.
func fromAPIConfig(cfg *clientcmdapi.Config) Kubeconfig {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	return Kubeconfig{
		Contexts:       names,
		CurrentContext: cfg.CurrentContext,
	}
}
