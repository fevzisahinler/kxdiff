package config

import (
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// defaultTimeout bounds how long we wait on the API server, so an unreachable
// cluster fails fast instead of hanging.
const defaultTimeout = 30 * time.Second

// RestConfig builds a *rest.Config for a specific context, using standard
// kubeconfig resolution and inheriting that context's authentication —
// including exec-credential plugins such as EKS, GKE and OIDC.
func RestConfig(kubeconfigPath, context string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: context}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

	rc, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building client config for context %q: %w", context, err)
	}
	rc.Timeout = defaultTimeout
	return rc, nil
}
