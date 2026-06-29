# kxdiff

Read-only diff between two Kubernetes environments.

[![Go Reference](https://pkg.go.dev/badge/github.com/fevzisahinler/kxdiff.svg)](https://pkg.go.dev/github.com/fevzisahinler/kxdiff)
[![Go Report Card](https://goreportcard.com/badge/github.com/fevzisahinler/kxdiff)](https://goreportcard.com/report/github.com/fevzisahinler/kxdiff)
[![Release](https://img.shields.io/github/v/release/fevzisahinler/kxdiff)](https://github.com/fevzisahinler/kxdiff/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

`kxdiff` compares two live Kubernetes environments — a context and a namespace
on each side — and reports what differs. It discovers every resource type the
cluster serves (built-ins, CRDs, and custom resources), strips the
server-generated noise, and shows a clean, field-level diff as text, JSON, or
markdown.

It answers a question the usual tools don't: *"What exists in staging but not in
prod — or exists in both but behaves differently?"*

<!-- Recorded with VHS; regenerate with `vhs docs/demo.tape`. -->
![kxdiff demo](docs/demo.gif)

## Highlights

- **Compares two live environments**, not a manifest against a cluster.
  Namespace ↔ namespace in one cluster, or context ↔ context across clusters.
- **Dynamic discovery.** Diffs every listable type the API server exposes —
  Deployments and ConfigMaps, but also CRDs and custom resources (Argo
  Applications, Gateway API routes, sealed secrets, …). No fixed resource list.
- **Clean by default.** Server-managed fields (`resourceVersion`, `uid`,
  `managedFields`, `status`, …), controller-owned objects, and high-churn types
  (Pods, Events, ReplicaSets) are filtered out so only meaningful drift remains.
- **Secret-safe.** Secret values are hashed, never printed. Opt in with
  `--reveal-secrets`.
- **Strictly read-only.** Only `get`/`list` are ever used. kxdiff never creates,
  updates, or deletes anything, and never changes your current context.
- **Field-level, list-aware diffs.** Container, env, and port lists are matched
  by name, so reordering doesn't produce false differences.
- **Built for pipelines and humans.** Stable exit codes and `--output json` for
  CI; coloured, TTY-aware text for the terminal.

## Installation

The command is `kxdiff`. It is also a kubectl plugin: when the binary is on your
`PATH`, kubectl runs it as `kubectl kxdiff`.

### krew (kubectl plugin)

```bash
kubectl krew install kxdiff
kubectl kxdiff --help
```

### Homebrew

```bash
brew install fevzisahinler/tap/kxdiff
```

### go install

```bash
go install github.com/fevzisahinler/kxdiff/cmd/kxdiff@latest
```

### From source

```bash
git clone https://github.com/fevzisahinler/kxdiff
cd kxdiff
make build           # produces ./bin/kxdiff
```

## Quick start

```bash
# Same cluster: compare the staging namespace against prod.
kxdiff --from staging --to prod

# Across clusters: compare two whole contexts.
kxdiff --from dev-cluster --to prod-cluster

# Specific context and namespace on each side.
kxdiff --from dev/web --to prod/web
```

## Concepts

### An environment is `[context][/namespace]`

`--from` and `--to` each take one environment. kxdiff resolves it against your
kubeconfig, so the form is flexible:

| You write     | Means                                                  |
|---------------|--------------------------------------------------------|
| `staging`     | namespace `staging` in the current context             |
| `dev-cluster` | the whole `dev-cluster` context (all namespaces)       |
| `dev/web`     | namespace `web` in context `dev`                       |
| `/web`        | namespace `web` in the current context                 |

A bare value that matches a context name is treated as a context; otherwise it
is a namespace. Context names that contain slashes (such as EKS ARNs,
`arn:aws:eks:…:cluster/dev`) are matched in full. Mistype a context and kxdiff
suggests the closest match.

The *type* to compare is separate from the environment, and applies to both
sides — see below.

### Choosing what to compare

By default kxdiff compares every resource. Narrow it with positional
`TYPE[/NAME]` arguments, exactly as you would with kubectl:

```bash
kxdiff --from staging --to prod deploy             # only Deployments
kxdiff --from staging --to prod deploy svc cm      # several types
kxdiff --from staging --to prod deploy/api         # a single object
kxdiff --from staging --to prod applications.argoproj.io  # a CRD
```

## Output

The default output is grouped into *only in `--from`*, *only in `--to`*, and
*differs*, with a field-level breakdown of each changed resource:

```
ENVIRONMENTS: dev/demo  <->  prod/demo
only in dev/demo: 1 | only in prod/demo: 1 | differs: 2 | same: 3

only in dev/demo:
  ConfigMap/legacy

only in prod/demo:
  ConfigMap/newfeature

differs:
  Deployment/web
      spec.replicas  2 → 5
      spec.template.spec.containers[web].image  nginx:1.25 → nginx:1.27
  ConfigMap/app
      data.DB_HOST  dev-db → prod-db
```

### Machine-readable formats

```bash
kxdiff --from staging --to prod -o json | jq '.differs[].name'
kxdiff --from staging --to prod -o markdown > drift.md
```

JSON values keep their type (numbers stay numbers, lists stay lists), so the
output is easy to post-process. Markdown is ready to paste into a pull request
or chat message.

## Using kxdiff in CI

kxdiff follows a stable exit-code contract, so it works as a drift gate:

| Exit code | Meaning              |
|-----------|----------------------|
| `0`       | no differences       |
| `1`       | differences found    |
| `2`       | error                |

```bash
# Fail the job if staging has drifted from prod.
kxdiff --from staging --to prod --quiet
```

## How kxdiff keeps the diff honest

A raw comparison of two clusters is mostly noise. kxdiff removes it in layers:

- **Static strip** — `resourceVersion`, `uid`, `creationTimestamp`,
  `generation`, `managedFields`, `selfLink`, and `status` are dropped from every
  object.
- **Runtime fields** — server-assigned values such as a Service's `clusterIP`
  are removed.
- **Resource filtering** — controller-owned objects (anything with an
  `ownerReference`), high-churn types (Pods, ReplicaSets, Events, Endpoints,
  Leases), and per-namespace system objects (the default ServiceAccount, the
  `kube-root-ca.crt` ConfigMap) are skipped. Add them back with
  `--include-generated`.
- **System namespaces** — when comparing whole contexts, `kube-system` and
  friends are excluded unless you pass `-A`.
- **Secrets** — values are SHA-256 hashed before comparison.

The result is verified by a self-diff oracle: comparing an environment with
itself produces zero differences.

## Comparison

| Tool             | Compares                              | CRDs | ns ↔ ns, same cluster | Read-only          |
|------------------|---------------------------------------|------|-----------------------|--------------------|
| **kxdiff**       | two live environments                 | Yes  | Yes                   | Yes                |
| `kubectl diff`   | local manifest vs live cluster        | N/A  | No                    | No (dry-run apply) |
| `dyff`           | two YAML/JSON files                   | N/A  | No                    | N/A                |
| ArgoCD/Flux      | Git repo vs live cluster              | Yes  | No                    | Yes                |

## Flags

| Flag                  | Description                                                        |
|-----------------------|-------------------------------------------------------------------|
| `--from`              | source environment (required)                                     |
| `--to`                | target environment (required)                                     |
| `-o, --output`        | `text` (default), `json`, or `markdown`                           |
| `-A, --all-namespaces`| include system namespaces in whole-context mode                  |
| `--only-from`         | show only resources present only in `--from`                      |
| `--only-to`           | show only resources present only in `--to`                        |
| `--only-diff`         | show only resources that differ                                   |
| `--include-generated` | include controller-managed and system objects                    |
| `--reveal-secrets`    | show raw Secret values instead of hashes                          |
| `--no-color`          | disable coloured output                                           |
| `-q, --quiet`         | print nothing; rely on the exit code                             |
| `--kubeconfig`        | path to the kubeconfig file                                       |

## Development

Requires Go 1.26+.

```bash
make test      # unit tests with the race detector
make lint      # golangci-lint
make build     # build ./bin/kxdiff
make check     # fmt + vet + lint + test + vuln
```

A local two-cluster test bed (kind) lives under `test-clusters/`; run
`./test-clusters/up.sh` to create it.

## License

Apache 2.0. See [LICENSE](LICENSE).
