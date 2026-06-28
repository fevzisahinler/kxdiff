package config

import (
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// defaultTimeout bounds how long we wait on the API server, so an
	// unreachable cluster fails fast instead of hanging.
	defaultTimeout = 30 * time.Second

	// QPS/Burst lift client-go's conservative client-side rate limit: kxdiff
	// only ever reads, but it lists many resource types at once, so the default
	// (5 QPS) would throttle discovery and fetch badly.
	defaultQPS   = 50
	defaultBurst = 100
)

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
	rc.QPS = defaultQPS
	rc.Burst = defaultBurst
	// Read-only tool: silence server-side deprecation warnings (e.g. Endpoints).
	rc.WarningHandler = rest.NoWarnings{}
	return rc, nil
}
