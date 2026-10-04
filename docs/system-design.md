<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for each Scenario in turn, provisions that Scenario's Topology into it, deploys a Workload, and reports the Placement once it is Settled. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no container image is ever pulled for a Pod.

The binary is a modulith of five modules under `internal/`, composed in `cmd/lab`. The `scenario` module holds a run's description and its observed outcome as immutable values, and imports no Kubernetes package. The `cluster` module owns the container lifecycle, every Kubernetes call, and the mapping between the two vocabularies. The `expect` module holds the assertion helper a Scenario and a `go test` suite share. The `scenarios` module holds the lab's own Scenarios, each a function over a provisioned cluster. The `modulerules` module holds the walk and scan the module-rule tests share.

A Scenario is an exported Go function over a provisioned cluster rather than a test method, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). The control plane is eventually consistent, so every reported Placement comes from `AwaitSettled`, which polls until two consecutive observations agree. Section details follow in Contracts and Scale and Load.

## Package Structure

<!-- The module map as it exists today. One line per module: name, responsibility. -->

```
.
├── cmd/
│   └── lab/            # composition root: provisions the cluster, runs the Scenario, prints the report
├── internal/
│   ├── cluster/        # Simulated Cluster: container lifecycle, Kubernetes calls, anti-corruption mapping
│   ├── expect/         # Expect(t, got).To(matcher) and its matchers
│   ├── modulerules/    # the package-graph walk and call-rule scan the module-rule tests share
│   ├── scenario/       # Topology, Workload, Placement values; no Kubernetes import
│   └── scenarios/      # the lab's Scenarios as functions; Even spread is the main one
├── .golangci.yml       # lint configuration carrying the module rules
├── Makefile            # the gate command and the targets it runs
├── go.mod              # the module path and the Go version
└── go.sum              # the committed checksum of every module in the graph
```

`internal/scenario` imports no `k8s.io/...` package, and a `golangci-lint` depguard rule denies it, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md). That rule names `internal/scenario` exactly, because `internal/scenarios` imports `internal/cluster` and reaches Kubernetes through it. No package imports `k8s.io/client-go/tools/clientcmd`, the home-directory helper, or a credential plugin, and a second depguard rule denies them, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). No package calls in-cluster configuration, and a type-resolved `forbidigo` pattern denies that call, per the same ADR. Both depguard rules are asserted a second time by the import-graph tests. A call-rule test over the module's own files asserts the `forbidigo` call rule a second time, per the same ADR. A home-relative engine socket candidate is composed with `os.UserHomeDir` and dialed, and no package reads a file under the home directory, per the same ADR. Kubernetes types stay inside `internal/cluster`, and only its mapper names both a Kubernetes type and a `scenario` type. The module-rule tests of `internal/cluster` and `internal/scenario` share their helpers through `internal/modulerules`, which imports the standard library alone. No production package imports it, and the import-graph test asserts that, per [ADR: The module-rule tests share one test-support package](adr/2026-10-04-module-rule-test-support.md).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository every pull and create names; no run input reaches it, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors when `--k8s-version` is absent, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The one home of the ordered engine socket paths probed when `DOCKER_HOST` does not resolve the engine, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits, one per letter of the derived Zone name |
| `probeTimeout` | `internal/cluster/runtime.go` | How long one candidate socket has to answer before the next is tried |
| `pullTimeout` | `internal/cluster/runtime.go` | The budget a pull of the pinned image runs within |
| `startTimeout` | `internal/cluster/runtime.go` | How long provisioning waits for the API server to answer its readiness probe |
| `settleTimeout`, `pollInterval` | `internal/cluster/simulated_cluster.go` | The budget `AwaitSettled` polls within, and the gap between polls |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The KWOK fake-Node annotation, the Zone and hostname labels, and the label carrying a Pod's Workload name |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable. Methods take value receivers, a type with invariants hides its fields behind one constructor returning an error, and slices are cloned at every boundary. `SimulatedCluster` is the aggregate root of a run — a Topology, a Workload, and an observation are reached through it alone. A Topology derives its own names: Zones are lettered from `zone-a`, and a Node is `<zone>-node-<n>`, numbered from one within its Zone. A Zone count above `maxZones` is refused at construction, naming the limit. Pod names come from the control plane, and an observed Pod's Zone is read from its Node's Zone label rather than from any name. One `DetectEngine` call per process resolves the engine, and every later engine call uses that one endpoint, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). That endpoint is an immutable value passed into each provisioning call, and no package holds mutable process-wide state. Each Scenario runs on a Simulated Cluster provisioned for it alone and removed when the Scenario ends, including when it fails; Scenarios run one after another, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md). One flag parse resolves the Kubernetes version, which the whole run then mirrors, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md). The engine surface is restricted by construction: a caller names a Kubernetes version, never an image and never a container, per the lab-provisioned ADR.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command: formatting, vet, lint, the dependency check, the tests and the build, failing on the first check that fails. Its test check runs the race detector on every machine, asserting no toolchain prerequisite of its own. Where the detector needs cgo and a C compiler, the Go toolchain's refusal fails the gate, which names how to install one, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md) | `Makefile` | REQ-SIM-004 |
| `make deps-check` | Fails and names the offender when a prohibited dependency class (§ Dependency Policy) appears in the module graph | `Makefile` | REQ-SIM-004 |
| depguard rules | Deny `k8s.io` imports from `internal/scenario`, and the kubeconfig, home-directory and credential-plugin imports in any package | `.golangci.yml` | REQ-SIM-004 |
| `forbidigo` rule | Denies the in-cluster configuration call in any package, matched through the type checker rather than the source text | `.golangci.yml` | REQ-SIM-004 |
| Import-graph tests | Assert both depguard rules from the module's own package graph, independently of the linter, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go` | REQ-SIM-004 |
| `modulerules` helpers | The one package-graph walk and call-rule scan both module-rule tests run, with its own test over a package graph and a file set | `internal/modulerules/` | REQ-SIM-001 |
| Call-rule test | Asserts the in-cluster configuration call is absent from the module's own files, independently of the lint configuration | `internal/cluster/callrule_test.go` | REQ-SIM-001 |
| `scenario.NodeCapacity` | What one Node can hold, so a Workload can exceed its Topology | `internal/scenario/topology.go` | REQ-SIM-001 |
| `scenario.Node` | One simulated machine, with the Zone it belongs to and its capacity | `internal/scenario/topology.go` | REQ-SIM-001 |
| `scenario.Topology` | The Zones, Nodes per Zone, and Node capacity a Scenario provisions; expands to its Nodes, each carrying the name derived for it | `internal/scenario/topology.go` | REQ-SIM-001 |
| Topology-naming test | Asserts the same Topology expands to the same Zone and Node names, and that a Zone count above the limit is refused | `internal/scenario/topology_test.go` | REQ-SIM-001 |
| `scenario.Workload` | A declared Pod count with the rules that constrain where its Pods may land | `internal/scenario/workload.go` | REQ-SIM-001 |
| `scenario.SpreadRule`, `scenario.SpreadDomain` | One Workload rule: how evenly its Pods sit over the Zones or over the Nodes, as a maximum Skew; the constructor refuses a Skew below one and a repeated domain, per [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md) | `internal/scenario/workload.go` | REQ-SIM-001 |
| Spread-mapping test | Asserts each declared rule becomes one enforced topology spread constraint at its Skew, on the Zone or hostname label, selecting the Workload's label and honouring taints; asserts no Pod affinity rule is set | `internal/cluster/resources_test.go` | REQ-SIM-001 |
| `scenario.Pod`, `scenario.PodPhase` | One observed replica, placed or unplaced, and the phase the control plane reports | `internal/scenario/placement.go` | REQ-SIM-001 |
| `scenario.Placement` | Where a Workload's Pods sit at one observation, against the Nodes that could hold them; counts per Zone and per Node include an empty one as zero, and Skew is the gap between the fullest and emptiest Zone | `internal/scenario/placement.go` | REQ-SIM-001 |
| Provisioned-address client | The one construction path to the control plane: it carries the provisioned loopback address and no credential | `internal/cluster/client.go` | REQ-SIM-001 |
| Client-target test | Asserts the client carries the provisioned address and that no credential or kubeconfig field is set | `internal/cluster/client_test.go` | REQ-SIM-001 |
| `cluster.DetectEngine` | Resolves the container engine socket once, returning the endpoint and the engine version that answered. It tries `DOCKER_HOST`, warns and falls through to the candidate list when that value is not a `unix://` path or does not answer, and errors naming every path tried with why each failed | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.Engine` | The restricted engine surface over one resolved unix socket: its exported methods take a context and a `KubernetesVersion`, and one of them pulls, creates and starts a cluster container | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.ClusterContainer` | The handle to the container this run created: it holds the container identifier unexported, and inspect, stop and remove act on that container alone | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors; the constructor admits a leading `v` followed by digits and dots alone, refusing every other value before a request | `internal/cluster/version.go` | REQ-SIM-001 |
| Engine-endpoint test | Asserts a `DOCKER_HOST` of any scheme but `unix` is skipped without a dial and the candidate list is probed instead, that a dead `unix://` value falls through to a live candidate, and that a `unix://` value that answers is used without probing one | `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| Engine-surface test | Asserts from the package's own declarations that no exported `Engine` or `ClusterContainer` method takes an image reference or a container identifier. Asserts `ParseKubernetesVersion` refuses a value that is not a version. Asserts a run beside another container creates and removes its own container alone | `internal/cluster/engine_surface_test.go` | REQ-SIM-001 |
| `cluster.Provision` | Starts the Simulated Cluster on a resolved engine for a resolved version, waits for its API server, and returns the handle, or an error naming what it could not reach. The container name carries the Scenario it serves and a suffix unique to the invocation | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| Cluster-removal test | Asserts a Scenario leaves no Simulated Cluster behind, whether its checks passed or failed, and that a second invocation alongside the first removes only its own cluster | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives: it applies a Topology, deploys a Workload, observes a Placement, and removes the cluster on close | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready with its capacity, then returns once the scheduler has placed and released one probe Pod on each of them, or an error naming the Nodes it never placed one on. Polls at the poll interval within the start timeout, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.AwaitSettled` | The one wait a Scenario or a test calls: polls one Workload's Placement until two consecutive observations agree, or the settle timeout expires. Provisioning's own wait is internal to `ApplyTopology` | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| Even spread test | Asserts the Scenario's Topology comes up with every Node Ready, that its Placement settles at nine Running Pods with three per Zone and one per Node, that a run reports only its own Nodes and Pods, and that a Workload larger than its Topology reports Pending Pods | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `expect.T`, `expect.Expect` | The assertion entry point, over the minimal interface `*testing.T` and the Scenario runner both satisfy | `internal/expect/expect.go` | REQ-SIM-001 |
| `expect.Equal`, `expect.HaveLen` | The matchers this slice asserts through; `Equal` reports a `go-cmp` diff on failure | `internal/expect/expect.go` | REQ-SIM-001 |
| `scenarios.EvenSpread` | The main Scenario: it provisions three Zones of three Nodes, deploys a nine-Pod Workload declaring even spread over Zones and over Nodes, and asserts three Pods in every Zone and one on every Node | `internal/scenarios/even_spread.go` | REQ-SIM-001 |
| `lab` command | Parses `--k8s-version`, detects the engine once, and prints the socket, the engine version and the Kubernetes version it mirrors. Provisions a Simulated Cluster for each Scenario in turn, runs it, removes that cluster, and prints the Placement per Zone and per Node. A Scenario that fails is named with what failed, and the run exits non-zero | `cmd/lab/main.go` | REQ-SIM-001 |
| Version-report test | Asserts a run naming no version reports the pinned default, that a run naming one mirrors and reports it, and that a version the lab cannot obtain stops the run before any Scenario | `cmd/lab/main_test.go` | REQ-SIM-001 |

A provisioned Node is created with its status already Ready and its declared capacity in place, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md). A Workload is realized as a Deployment whose Pod template carries one topology spread constraint per rule the Workload declares, so the control plane's own controllers produce the Placement. Each constraint is enforced rather than preferred, counts only the Nodes whose taints its Pods tolerate, selects those Pods by their Workload label, and declares no minimum domain count, per [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md). A Workload declaring no rule carries no constraint. Engine API responses are decoded into unexported types inside `internal/cluster`, and no engine type crosses that boundary. A Kubernetes version with no published cluster image fails at the pull, with an error naming the version and the full image reference.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates probed at startup (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then the candidate list, five paths | by a reviewed change to the constant | run-once, before every other engine call | Probed in order with one version request each, bounded by the probe timeout; the first socket that answers is kept for the run |
| Engine pull progress stream (missing image only) | one pull per Kubernetes version absent from the machine; one JSON object per progress message, the count unrecorded, treated as bounded | with the image's layer count, and with the versions a machine has run | run-once, written nowhere | Decoded as a stream, one object at a time and discarded; O(messages) time, constant memory |
| Nodes provisioned per run (`ApplyTopology`) | the Even spread Scenario's 9 Nodes, from 3 Zones of 3 | with the Topology a Scenario declares; above 9 unrecorded, treated as bounded, and capped at `maxZones` Zones | run-private, written once | One create call plus one status update per Node, in declaration order; each name derived from its Zone letter and its index |
| Probe Pods per provisioning (`ApplyTopology`) | one per declared Node, the Even spread Scenario's 9 | with the Nodes a Topology declares | run-private, created and removed within provisioning | One create per Node, then one label-selected list call per poll until each is Running and again until each is gone; O(nodes) per poll, within the start timeout |
| Pods of one Workload per observation (`Placement`) | the Even spread Scenario's 9 Pods | with the Pod count a Workload declares; above 9 unrecorded, treated as bounded | run-private, read-mostly | One label-selected list call, counted into maps keyed by Node and by Zone; O(pods + nodes) per observation |
| Spread rules carried onto one Pod template (the mapper) | the Even spread Scenario's 2 rules, one per domain | with the rules a Workload declares; at most one per domain | run-private, written once | Copied in declaration order, one topology spread constraint per rule; O(rules) |
| Settle polling (`AwaitSettled`) | one observation per poll interval, within the settle timeout | unchanged | run-private | Two consecutive observations compared by value; O(pods + nodes) per poll |
| Package graph read by the import-graph tests | unrecorded, treated as bounded; a few thousand packages once the Kubernetes client is required | with the module's dependency tree | test-only, read once per test | `go list -deps -json` decoded as a stream, one package object at a time; O(packages) time, constant memory |
| Module-owned Go files read by the call-rule test | tens of files; the module's own packages only, never a dependency | with the lab's own source | test-only, read once per test | Each file parsed once with `go/parser`, imports resolved to the file's own alias, then its calls scanned; O(files + nodes) |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency or throughput figure is a requirement (G-1). The first run of a Kubernetes version new to the machine also pays one image pull, within the pull timeout.
- **Limits:** one Simulated Cluster at a time, in one container, provisioned per Scenario and removed when that Scenario ends. A run of N Scenarios pays N container starts, sequentially. Every observation is bounded by the settle timeout and the poll interval (§ Constants). Provisioning is bounded by the start timeout, one candidate probe by the probe timeout, and a pull by the pull timeout. The API server's own limits are unrecorded, treated as bounded.

## Dependency Policy

Minimize external dependencies. Every dependency is an attack surface and a maintenance burden.

### Approved Sources

| Source | Examples | Rationale |
|--------|----------|-----------|
| Go standard library | `net`, `net/http`, `net/url`, `encoding/json`, `os`, `log/slog`, `slices`, `maps`, `testing` | The first choice for every need, including the engine's HTTP API over a unix socket and the test framework |
| Kubernetes project | `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` | The control-plane client every Kubernetes call in `internal/cluster` goes through |
| Google | `github.com/google/go-cmp` | Struct comparison and failure diffs in `internal/expect` |

### Adding a New Dependency

Before adding a dependency, verify:

1. **Necessity** — Can the standard library solve the problem? A dependency for one function is justified only when the function is more than a few lines of tested code and the library is established. The decision weighs writing and maintaining the code against the dependency and its transitive tree.
2. **Source** — Is it from an approved source above? If not, create an ADR.
3. **Audit** — Review transitive dependencies. Flag unknown modules.
4. **Verification** — Verify checksums and commit the lockfile.

### Prohibited

<!-- Dependency classes this project rejects, with the reason. -->

| Class | Examples | Reason |
|-------|----------|--------|
| Assertion libraries | Gomega, testify | Every assertion goes through `internal/expect`, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md) |
| Mock frameworks | gomock, mockery | Tests use real values and real I/O (`docs/testing-principles.md` § Mocking Policy) |
| Container-orchestration clients | testcontainers-go, the Docker SDK | The engine is driven through its HTTP API over a unix socket with the standard library, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A second Kubernetes client | controller-runtime, `k8s.io/kubectl` | One client library holds the Kubernetes boundary, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) |

This table is the source of truth for the class list; `make deps-check` carries its mechanical form (§ Contracts).

## Security Context

<!-- PROJECT: Describe this application's security profile: what it connects to, what it exposes, how it handles credentials, and how it runs (systemd, container, etc.). The security-reviewer reads this section before reviewing. -->

- **Inputs it processes:** the Scenario descriptions compiled into the binary, the `--k8s-version` flag, `DOCKER_HOST` when it is set, and the engine's JSON responses, including the published host port.
- **Outputs it produces:** the resolved socket, the engine version, the mirrored Kubernetes version and the Placement report on stdout, a warning when `DOCKER_HOST` does not resolve the engine, the process exit status, and Kubernetes resources in the Simulated Cluster the run provisioned.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port.
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable, and no service-account token. The client is built from the provisioned address alone, and no file under the home directory is read, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
- **Runtime:** a developer machine or a build agent, as the invoking user, with a container engine socket that answers.

## Threat Model

| Threat | Attack Vector | Mitigation |
|--------|--------------|------------|
| A run drives a real cluster (NG-1) | An ambient kubeconfig, a `KUBERNETES_*` variable, or an in-cluster service-account token discovered by the client | The lab's whole configuration surface is one flag and one variable. `--k8s-version` names a Kubernetes version, `DOCKER_HOST` names a local engine socket, and neither carries a cluster address or a credential. The client is constructed from the provisioned address with no credential, depguard denies `clientcmd` and the other discovery imports, and `forbidigo` denies the in-cluster configuration call, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). The client-target test and the call-rule test assert the client's address and the call's absence independently of the linter |
| A third party reaches the unauthenticated API server | The engine publishes the API port on every interface | The API port is published on the loopback address, on a host port the engine chooses, per the same ADR |
| A run drives an engine that is not on this machine | A `DOCKER_HOST` naming a `tcp://` or `ssh://` endpoint | Only a `unix://` path is ever dialed. The value is parsed as a URL, never interpolated into a command, and any other scheme is skipped without a request, with a warning naming the value and the reason; detection then probes the candidate list, per the same ADR |
| The engine socket grants control of every container on the machine | The lab holds an API handle the engine cannot scope down | The exported engine methods take a context and a `KubernetesVersion` alone. Inspect, stop and remove are methods on the handle this run created, which holds the container identifier unexported. The package declares no list call and no by-identifier call, per the same ADR. The engine-surface test asserts the surface from the package's own declarations |
| A run pulls or starts an image that is not a KWOK cluster image | The version input interpolated into an image reference | The repository is a constant no input reaches, and the tag is composed from the constant KWOK release and a validated version. `ParseKubernetesVersion` admits a leading `v` with digits and dots alone, so no reference, path or scheme character survives, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| A moved image tag changes what the Simulated Cluster runs | The pinned tag is re-pushed upstream | The Kubernetes version is pinned by tag, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md); digest pinning is not in force |
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario; the container name carries the Scenario and the invocation, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a suffix unique to the invocation, each port is the engine's own choice, and a run removes only the container its own handle names, per the same ADR |

## Implementation Order

| ID | Name | Depends On |
|----|------|------------|
| REQ-SIM-004 | Go build floor: the `make ci` gate, the lint rules, the Java and Gradle trees retired | — |
| REQ-SIM-001 | Topology provisioning, Workload deployment, and the Settled Placement report | REQ-SIM-004 |
| REQ-SIM-002 | One invocation runs every Scenario and names a missing container runtime | REQ-SIM-001 |
| REQ-SIM-003 | One Scenario runs on its own, named by the engineer | REQ-SIM-002 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | `Provision` starts the next Scenario's container and the readiness probe answers | cluster ready |
| cluster ready | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | `Deploy` creates the Workload's Deployment | Workload deployed |
| Workload deployed | two consecutive observations agree | Placement Settled |
| Placement Settled | the Scenario ends, whether its checks passed or failed | container removed |
| container removed | a Scenario is left to run | engine resolved |
| any state | close, or the process exits | container removed |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Running |
| Pending | no Node satisfies the Workload's rules or capacity | Pending, reported unplaceable (edge case 1) |
