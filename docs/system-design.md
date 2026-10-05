<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for each Scenario in turn, provisions that Scenario's Topology into it, deploys a Workload, and reports the Placement once it is Settled. It records what that cluster did while the Scenario ran, prints a summary of that Timeline, and leaves every entry in a file of its own. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no container image is ever pulled for a Pod.

The binary is a modulith composed in `cmd/lab`. The top-level `scenarios` package holds the lab's own Scenarios, one file each, their list, and the README naming them. Six modules under `internal/` hold the machinery that runs them. The `lab` module holds the Step surface a Scenario's lines run against, and the runner. The `scenario` module holds a run's description and its observed outcome as immutable values, and imports no Kubernetes package. The `cluster` module owns the container lifecycle, every Kubernetes call, and the mapping between the two vocabularies. The `timeline` module owns where a Timeline is kept: its folder, its file name and its line format. The `expect` module holds the assertion helper a Then Step and a `go test` suite share. The `modulerules` module holds the walk and scan the module-rule tests share.

A Scenario is an exported Go function over a `lab.Scenario`, each line one Given, When or Then Step, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). It is named for the condition and the outcome it asserts, and it runs under a Scenario name derived from its file. The control plane is eventually consistent, so every reported Placement comes from `AwaitSettled`, which polls until two consecutive observations agree. A Timeline comes from the API's own watch streams instead, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md). Section details follow in Contracts and Scale and Load.

## Package Structure

<!-- The module map as it exists today. One line per module: name, responsibility. -->

```
.
├── cmd/
│   └── lab/            # composition root: resolves the version and the engine, runs the Scenarios the list holds, prints each Outcome
├── scenarios/          # the lab's Scenarios, one file each, their list, their Workload names, and the README naming them
├── internal/
│   ├── cluster/        # Simulated Cluster: container lifecycle, Kubernetes calls, anti-corruption mapping
│   ├── expect/         # Expect(t, got).To(matcher) and its matchers
│   ├── lab/            # the Step surface: the Scenario value, its Steps, and the runner
│   ├── modulerules/    # the package-graph walk and call-rule scan the module-rule tests share
│   ├── scenario/       # Topology, Workload, Placement, Timeline values; no Kubernetes import
│   └── timeline/       # the Timeline folder, its file names and its JSON Lines format
├── timelines/          # the filed Timelines of every run, git-ignored, removed by no run
├── .golangci.yml       # lint configuration carrying the module rules
├── Makefile            # the gate command and the targets it runs
├── go.mod              # the module path and the Go version
└── go.sum              # the committed checksum of every module in the graph
```

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, and a `golangci-lint` depguard rule denies it, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md). That rule names those two packages exactly; the top-level `scenarios` package reaches the Simulated Cluster through `internal/lab`. A Scenario source holds one exported Scenario function taking `*lab.Scenario` and no other declaration. That function is named in the `<Condition>_Should<Outcome>` shape a test name carries without its `Test` prefix (`docs/testing-principles.md` § Test Naming). Beside the Scenarios, `workloads.go` declares the Workload names they deploy as exported `lab.WorkloadName` constants, and `scenarios.go` the one exported function `All`, the list of the lab's Scenarios. Neither holds anything else. All of them import `internal/lab` alone of the lab's own packages, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). A depguard rule denies the other four packages and `k8s.io` under `scenarios/`, and the import-graph test asserts the same over the module's production packages. The place holds no test file, and the Scenario-place test reports one. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. No package imports `k8s.io/client-go/tools/clientcmd`, the home-directory helper, or a credential plugin, and a second depguard rule denies them, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). No package calls in-cluster configuration, and a type-resolved `forbidigo` pattern denies that call, per the same ADR. Both depguard rules are asserted a second time by the import-graph tests. A call-rule test over the module's own files asserts the `forbidigo` call rule a second time, per the same ADR. A home-relative engine socket candidate is composed with `os.UserHomeDir` and dialed, and no package reads a file under the home directory, per the same ADR. Kubernetes types stay inside `internal/cluster`, and only its mapper names both a Kubernetes type and a `scenario` type. The module-rule tests of `internal/cluster` and `internal/scenario` share their helpers through `internal/modulerules`, which imports the standard library alone. No production package imports it, and the import-graph test asserts that, per [ADR: The module-rule tests share one test-support package](adr/2026-10-04-module-rule-test-support.md). `internal/timeline` imports `internal/scenario` and the standard library alone. The sink a recording delivers its entries to is a one-method interface declared in `internal/cluster`, so the recording side names no file and `internal/cluster` imports no Timeline file, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository every pull and create names; no run input reaches it, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors when `--k8s-version` is absent, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The one home of the ordered engine socket paths probed when `DOCKER_HOST` does not resolve the engine, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits, one per letter of the derived Zone name |
| `scenario.DefaultNodeCapacity` | `internal/scenario/topology.go` | What a declared Node holds when a Scenario names no capacity; it leaves room for more than one Pod per Node |
| `probeTimeout` | `internal/cluster/runtime.go` | How long one candidate socket has to answer before the next is tried |
| `pullTimeout` | `internal/cluster/runtime.go` | The budget a pull of the pinned image runs within |
| `startTimeout` | `internal/cluster/runtime.go` | How long provisioning waits for the API server to answer its readiness probe |
| `settleTimeout`, `pollInterval` | `internal/cluster/simulated_cluster.go` | The budget `AwaitSettled` polls within, and the gap between polls |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The KWOK fake-Node annotation, the Zone and hostname labels, and the label carrying a Pod's Workload name |
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The one folder under the working directory a run's Timelines collect in, and the suffix marking a filed Timeline, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | What a created Timeline folder and a created Timeline file are readable and writable by: the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable. Methods take value receivers, a type with invariants hides its fields behind one constructor returning an error, and slices are cloned at every boundary. `SimulatedCluster` is the aggregate root of a run — a Topology, a Workload, and an observation are reached through it alone. A Topology derives its own names: Zones are lettered from `zone-a`, and a Node is `<zone>-node-<n>`, numbered from one within its Zone. A Zone count above `maxZones` is refused at construction, naming the limit. Pod names come from the control plane, and an observed Pod's Zone is read from its Node's Zone label rather than from any name. One `DetectEngine` call per process resolves the engine, and every later engine call uses that one endpoint, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). That endpoint is an immutable value passed into each provisioning call, and no package holds mutable process-wide state. Each Scenario runs on a Simulated Cluster provisioned for it alone and removed when the Scenario ends, including when it fails; Scenarios run one after another, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md). One flag parse resolves the Kubernetes version, which the whole run then mirrors, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md). The engine surface is restricted by construction: a caller names a Kubernetes version, never an image and never a container, per the lab-provisioned ADR.

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given declares alone: it constructs the immutable `internal/scenario` value and names itself when that constructor refuses it. A Step's mandatory values are named arguments, each a defined count or name type in `internal/lab`. An optional setting is a further call on the complete value, taking the same named arguments, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). A Topology Step takes `lab.Zones` and `lab.NodesPerZone`, at `DefaultNodeCapacity`, and a further call names another capacity as `lab.Pods`. A Workload Step takes a `lab.WorkloadName` and `lab.Pods`; the lab's own Scenarios name the constants declared beside them. A count type carries no invariant; the `internal/scenario` constructor refuses an invalid count. The declaration reaches the Simulated Cluster at the first Step needing an observed Placement, which applies the Topology, deploys the Workload and awaits the Settled Placement. A Step that cannot be carried out is the last Step recorded, and every later Step carries nothing out and reports nothing. A Then the Placement does not meet records a Finding, and the later Steps still run. Every Then judges through `internal/expect`, per the same ADR. The Step kinds are Given, When and Then; the Failure a When injects is a later slice. A Scenario's Scenario name is the stem of the file its function is defined in, with hyphens for underscores. A Scenario name the lab cannot derive is an error naming the function and the file, and no Simulated Cluster is provisioned, per the same ADR. `cmd/lab`'s own suite is where the lab's Scenarios run on a Simulated Cluster, and it fails when one reports a Finding or ends at a Step.

Every Scenario's run records a Timeline. Five watch streams feed it: the cluster's Nodes, its Pods, its Deployments, its ReplicaSets, and the Events of the Workload's namespace. One goroutine owns the record. It selects over the five streams, maps each change into an immutable `scenario.Entry`, and delivers it to the sink it was given, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md). A stream the API server closes is re-opened once from the last version seen, and a re-open that fails is a noted entry. A stop cancels the five watches and drains each stream to its close. An entry carries the time since recording started, and that receipt order is the Timeline's order. Within one stream it is the API server's order; across streams it is the order the lab received them. A Timeline file is named for the Simulated Cluster it describes and carries one JSON object per line. Its first line names the Scenario, its cluster and the Kubernetes version; each later line is one entry, written as that entry arrives. The folder is created when a run files its first Timeline. A file keeps its first write error and discards every later entry. The run reports every Scenario's outcome, names each Timeline it could not write, and exits non-zero, per the same ADR. The lab removes no Timeline file.

A run prints a Timeline summary rather than its entries, derived from the entries alone, per [ADR: A run prints a Timeline summary derived from the record](adr/2026-10-05-timeline-summary-derived-from-the-record.md). The run prints it under the line naming the entry count and the file it was filed at. A Deployment entry names a Workload; a Pod entry belongs to the Workload whose name its own begins with, followed by a hyphen, at the longest match. Every other object is the lab's own setup, collapsed to one Topology line counting the Nodes that reported Ready and when the last of them did. A Workload carries the readiness its last Deployment entry reported, timed at the first entry reading the same. Under it is one line per Pod, giving that Pod's whole path and the Node it landed on. A Pod still Pending carries the control plane's reason from the last entry reporting it, and the summary names the Pods left Pending or says there are none. Workloads and Pods read in the order the Timeline first names them. A run given `--whole-timeline` prints every entry under the summary, in receipt order, per the same ADR.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command: formatting, vet, lint, the dependency check, the tests and the build, failing on the first check that fails. Its test check runs the race detector on every machine, asserting no toolchain prerequisite of its own. Where the detector needs cgo and a C compiler, the Go toolchain's refusal fails the gate, which names how to install one, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md) | `Makefile` | REQ-SIM-004 |
| `make deps-check` | Fails and names the offender when a prohibited dependency class (§ Dependency Policy) appears in the module graph | `Makefile` | REQ-SIM-004 |
| depguard rules | Deny `k8s.io` imports from `internal/scenario`, the machinery imports in a Scenario source, and the kubeconfig, home-directory and credential-plugin imports in any package | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006 |
| `forbidigo` rule | Denies the in-cluster configuration call in any package, matched through the type checker rather than the source text | `.golangci.yml` | REQ-SIM-004 |
| Import-graph tests | Assert every depguard rule from the module's own package graph, independently of the linter, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-004, REQ-SIM-006 |
| `modulerules` helpers | The one package-graph walk, call-rule scan and declaration scan the module-rule tests share, with its own test over a package graph and a file set. A scanned declaration carries the type a const or var writes | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
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
| `scenario.Timeline`, `scenario.Entry` | The ordered record one Scenario's run left, and one change in it: when the lab received it, what changed, and the control plane's reason where it reported one. Immutable values, and an entry renders the line a report prints | `internal/scenario/timeline.go` | REQ-SIM-005 |
| `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | Which of added, modified, removed, reported or noted an entry records, which kind of object it names, and the two words a Node entry reads its Ready condition through | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-value test | Asserts an entry's printed line reads the same for the same entry, that a noted entry names no object, and that a Timeline clones its entries at its boundary | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.TimelineSummary`, `scenario.Timeline.Summarise` | One Timeline read as the report a run prints: its entry count, the Topology the lab set up, each Workload with its readiness, each Workload Pod with the whole path it took and the Node it landed on, and the Pods left Pending. A pure function of the entries, holding no path and rendering its own lines | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-summary test | Asserts a summary counts the Timeline's entries, collapses the lab's own Nodes and probe Pods to one Topology line, gives a placed Pod its path and its Node and a Pending Pod the control plane's reason, names no Workload where none was deployed, gives a re-placed Pod its whole path, and counts two Workloads apart. Asserts over entries the test builds, needing no cluster | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.Record` | Opens this cluster's five watch streams and starts the one goroutine owning its Timeline, delivering each entry to the sink given. Re-opens a closed stream once and notes a re-open it cannot make | `internal/cluster/timeline.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.StopRecording` | Cancels the five watches, drains each stream to its close, and returns the complete Timeline. A second call returns the same Timeline, and `Close` calls it before removing the container | `internal/cluster/timeline.go` | REQ-SIM-005 |
| Entry mapper | The one place a watched Kubernetes object becomes a `scenario.Entry`: a Node's Ready condition and capacity, a Pod's phase and the Node it is bound to, a Deployment's and a ReplicaSet's ready replicas, and an Event's reason and message | `internal/cluster/resources.go` | REQ-SIM-005 |
| Timeline-recording test | Asserts a Scenario's Timeline holds an entry for every Node of its Topology, every probe Pod and every Pod of its Settled Placement, in receipt order, and the control plane's reason both for a placed Pod and for one left Pending | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| Timeline-stream test | Asserts a closed stream is re-opened once from the last version seen, and that a failed re-open and a second close are noted entries. Asserts a stream the lab cannot open stops those already open, and that a cluster already recording refuses a second record. The stream connection is a hand-written `watch.Interface`, the lab's one stand-in for a system boundary, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md) | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.NewFolder` | Where a run's Timelines collect: the folder under a given root, created when the run opens its first file in it | `internal/timeline/timeline.go` | REQ-SIM-005 |
| `timeline.File` | One Scenario's filed Timeline, named for the Simulated Cluster it describes: it writes its header, then one JSON object per entry as that entry arrives, and keeps its first write error. One that could not be opened reports that error and discards every entry | `internal/timeline/timeline.go` | REQ-SIM-005 |
| Timeline-file tests | Assert a missing folder is created rather than failing the run, that a folder or file the lab cannot write leaves its path and its reason and swallows no entry elsewhere, and that a written file reads back as the entries recorded | `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.T`, `expect.Expect` | The assertion entry point, over the minimal interface `*testing.T` and the Scenario runner both satisfy | `internal/expect/expect.go` | REQ-SIM-001 |
| `expect.Equal`, `expect.HaveLen` | The matchers this slice asserts through; `Equal` reports a `go-cmp` diff on failure | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against: `Given()` declares the Topology and the Workload, `Then()` judges the Settled Placement, and the value records every Step | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Zones`, `lab.NodesPerZone`, `lab.Pods`, `lab.WorkloadName` | The named arguments a Step's mandatory values are passed as: defined types over a count or a name, holding no invariant, so a swapped argument does not compile | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Step`, `lab.StepKind` | One line of a Scenario as it ran: its kind, Given, When or Then, and the name it reports | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Finding` | One Then outcome the Placement does not meet, naming its Step and the mismatch `expect` reported | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Outcome` | What one Scenario reported: its Scenario name, its Steps in order, its Findings, its Settled Placement, the Simulated Cluster it ran on, the Step that could not be carried out, its Timeline, the path that Timeline was filed at, and the error that prevented filing it | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| `lab.Runner`, `lab.NewRunner` | Runs one Scenario on the resolved engine, filing its Timeline in the folder the runner was built with: it derives that Scenario's Scenario name from the file its function is defined in, provisions its Simulated Cluster, opens that cluster's Timeline file and records into it, hands its lines a `lab.Scenario`, stops the record, removes the cluster, and returns the Outcome. A name it cannot derive is an error naming the function and the file | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| Step-surface tests | Assert a Given declares the Topology and the Workload, and a Then judges the Placement outcome. Assert a Step that cannot be carried out ends the Scenario naming itself, and a Then the Placement does not meet records a Finding while the later Steps run. Assert a Scenario without a Failure carries no When Step | `internal/lab/scenario_test.go` | REQ-SIM-006 |
| Scenario-name test | Asserts a Scenario's Scenario name is its file's stem with hyphens for underscores, and that a function the lab can locate no Scenario file for is an error naming it | `internal/lab/runner_test.go` | REQ-SIM-006 |
| Scenario-place tests | Assert every Scenario source holds one exported Scenario function and no other declaration, named in the Scenario-name shape, and that the two files beside them declare the Workload names and the list alone. Assert the list names every Scenario of the place exactly once, and that every Scenario file yields a Scenario name. Assert the place holds every Scenario, a README naming them and no test file, and that no Scenario source imports the machinery | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006 |
| Workload names | Where the lab's own Scenarios take the names of the Workloads they deploy from, one exported constant per Workload | `scenarios/workloads.go` | REQ-SIM-006 |
| `scenarios.All` | The lab's Scenarios, one line each: the one list a run iterates, so adding a Scenario touches the Scenarios' place alone | `scenarios/scenarios.go` | REQ-SIM-006 |
| `scenarios.AnEvenlySpreadWorkload_ShouldPlaceThreePodsPerZoneAndOnePerNode` | The main Scenario, running as `even-spread`: Given Steps declaring three Zones of three Nodes and the nine-Pod `Checkout` Workload spread evenly over Zones and over Nodes, and Then Steps asserting three Pods in every Zone and one on every Node | `scenarios/even_spread.go` | REQ-SIM-001, REQ-SIM-006 |
| Scenarios README | Names every Scenario of the lab and the outcome each asserts, and nothing else | `scenarios/README.md` | REQ-SIM-006 |
| `lab` command | Parses `--k8s-version` and `--whole-timeline` in the one parse, into one immutable value it passes by value, detects the engine once, and prints the socket, the engine version and the Kubernetes version it mirrors. Resolves the Timeline folder under the working directory and builds the runner with it. Runs each Scenario `scenarios.All` holds through `lab.Runner`, under the Scenario name derived for it, and prints its Steps, its Placement per Zone and per Node, and its Timeline summary under the path it was filed at, with every entry beneath that summary when the run was given `--whole-timeline`. A Scenario with a Finding, or one that ended at a Step, is named with that outcome and the run exits non-zero. A Timeline the run could not write is named with its reason, leaving every Scenario's outcome as reported | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006 |
| Shipped-Scenarios test | Asserts a run reports no Finding and no Step that could not be carried out, for every Scenario `scenarios.All` holds. Asserts Even spread written as Steps settles at three Pods in every Zone and one on every Node | `cmd/lab/main_test.go` | REQ-SIM-006 |
| Timeline-report test | Asserts a run prints each Scenario's Timeline summary, that a run given `--whole-timeline` prints every entry in the order the changes happened, that a run of two Scenarios leaves one file per Scenario with neither replacing the other, that two runs at once leave each run's files of its own, that a Scenario failing part-way is still printed and filed, and that a filed Timeline reads back after its Simulated Cluster is removed. Each run files into a folder of the test's own | `cmd/lab/main_test.go` | REQ-SIM-005 |
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
| Steps recorded per Scenario (`lab.Scenario`) | Even spread's five Steps; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step, read once by the report | Appended to one slice in Step order; O(steps) |
| Scenario sources parsed by the Scenario-place tests | the place's files: one Scenario today, their list and their Workload names | with the Scenarios the lab holds | test-only, read once per test | Each file parsed once with `go/parser`, then its declarations and imports scanned; O(files + nodes) |
| Timeline entries recorded per Scenario (`Record`) | the Even spread Scenario's 150 or so: its 9 Nodes, its 9 probe Pods, its 9 Workload Pods, their Deployment and ReplicaSet, and the control plane's Events about them; above that unrecorded, treated as bounded | with the Nodes and Pods a Scenario declares, and with how long that Scenario runs | run-private, appended once by the recording goroutine, read once by the report | One slice appended in receipt order, each entry also encoded to the open file as it arrives; O(1) per entry, O(entries) memory |
| Timeline entries read by the summary (`Summarise`) | the entries one Scenario recorded, the Even spread Scenario's 150 or so | with the entries recorded, and with the Workloads a Scenario deploys | run-private, read once after the record is stopped | One pass over the entries in receipt order, grouped into maps keyed by Workload and by Pod name; O(entries × workloads) time with one Workload today, O(pods + nodes) memory |
| Watch streams open per Scenario (`Record`) | five, one per kind recorded | by a reviewed change to the kinds recorded | run-private, open from provisioning until the record is stopped | One `Watch` call per kind, merged by one `select` in the goroutine owning the record; a stream the server closes is re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, then listed and read by no run | One file per Scenario, named for its Simulated Cluster; a run opens its own files and reads none |
| Module-owned Go files read by the call-rule test | tens of files; the module's own packages only, never a dependency | with the lab's own source | test-only, read once per test | Each file parsed once with `go/parser`, imports resolved to the file's own alias, then its calls scanned; O(files + nodes) |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency or throughput figure is a requirement (G-1). The first run of a Kubernetes version new to the machine also pays one image pull, within the pull timeout.
- **Limits:** one Simulated Cluster at a time, in one container, provisioned per Scenario and removed when that Scenario ends. A run of N Scenarios pays N container starts, sequentially. Every observation is bounded by the settle timeout and the poll interval (§ Constants). Provisioning is bounded by the start timeout, one candidate probe by the probe timeout, and a pull by the pull timeout. One Timeline is held in memory at a time, and one Timeline file is open at a time. A summary is derived from the Timeline already in memory. The API server's own limits are unrecorded, treated as bounded.

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

- **Inputs it processes:** the Scenario descriptions compiled into the binary, the `--k8s-version` and `--whole-timeline` flags, `DOCKER_HOST` when it is set, the engine's JSON responses including the published host port, and the API server's watch responses for the kinds a Timeline records.
- **Outputs it produces:** the resolved socket, the engine version, the mirrored Kubernetes version, the Placement report and each Scenario's Timeline summary on stdout, every entry of that Timeline when the run was given `--whole-timeline`, a warning when `DOCKER_HOST` does not resolve the engine, the process exit status, Kubernetes resources in the Simulated Cluster the run provisioned, and one Timeline file per Scenario in the folder under the working directory.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port.
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable, and no service-account token. The client is built from the provisioned address alone, and no file under the home directory is read, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
- **Runtime:** a developer machine or a build agent, as the invoking user, with a container engine socket that answers.

## Threat Model

| Threat | Attack Vector | Mitigation |
|--------|--------------|------------|
| A run drives a real cluster (NG-1) | An ambient kubeconfig, a `KUBERNETES_*` variable, or an in-cluster service-account token discovered by the client | The lab's whole configuration surface is two flags and one variable. `--k8s-version` names a Kubernetes version, `--whole-timeline` names no value, `DOCKER_HOST` names a local engine socket, and none of them carries a cluster address or a credential. The client is constructed from the provisioned address with no credential, depguard denies `clientcmd` and the other discovery imports, and `forbidigo` denies the in-cluster configuration call, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). The client-target test and the call-rule test assert the client's address and the call's absence independently of the linter |
| A third party reaches the unauthenticated API server | The engine publishes the API port on every interface | The API port is published on the loopback address, on a host port the engine chooses, per the same ADR |
| A run drives an engine that is not on this machine | A `DOCKER_HOST` naming a `tcp://` or `ssh://` endpoint | Only a `unix://` path is ever dialed. The value is parsed as a URL, never interpolated into a command, and any other scheme is skipped without a request, with a warning naming the value and the reason; detection then probes the candidate list, per the same ADR |
| The engine socket grants control of every container on the machine | The lab holds an API handle the engine cannot scope down | The exported engine methods take a context and a `KubernetesVersion` alone. Inspect, stop and remove are methods on the handle this run created, which holds the container identifier unexported. The package declares no list call and no by-identifier call, per the same ADR. The engine-surface test asserts the surface from the package's own declarations |
| A run pulls or starts an image that is not a KWOK cluster image | The version input interpolated into an image reference | The repository is a constant no input reaches, and the tag is composed from the constant KWOK release and a validated version. `ParseKubernetesVersion` admits a leading `v` with digits and dots alone, so no reference, path or scheme character survives, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| A moved image tag changes what the Simulated Cluster runs | The pinned tag is re-pushed upstream | The Kubernetes version is pinned by tag, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md); digest pinning is not in force |
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario; the container name carries the Scenario and the invocation, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a suffix unique to the invocation, each port is the engine's own choice, and a run removes only the container its own handle names, per the same ADR |
| A Timeline is written outside its folder | A Scenario name interpolated into a path | A Timeline file is named for the Simulated Cluster it describes: the lab's constant prefix, a Scenario name that is a compiled-in source file's stem, and a random suffix. The name is joined to the folder as one path element, and `internal/timeline` refuses a name holding a separator or a parent reference, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| A filed Timeline exposes what a run did to another user | The file outlives the run, in the working directory | A created Timeline folder and file are readable and writable by the invoking user alone. A Timeline holds the simulated objects of a cluster the lab provisioned and the control plane's messages about them, and that cluster holds no credential and no real Workload, per the same ADR |

## Implementation Order

| ID | Name | Depends On |
|----|------|------------|
| REQ-SIM-004 | Go build floor: the `make ci` gate, the lint rules, the Java and Gradle trees retired | — |
| REQ-SIM-001 | Topology provisioning, Workload deployment, and the Settled Placement report | REQ-SIM-004 |
| REQ-SIM-002 | One invocation runs every Scenario and names a missing container runtime | REQ-SIM-001 |
| REQ-SIM-003 | One Scenario runs on its own, named by the engineer | REQ-SIM-002 |
| REQ-SIM-006 | A Scenario is a sequence of Given, When and Then Steps, in one place with its README | REQ-SIM-001 |
| REQ-SIM-005 | A Scenario's run prints and files a Timeline of what its Simulated Cluster did | REQ-SIM-001, REQ-SIM-006 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | a Scenario's Scenario name cannot be derived from its file | run stopped, that function and file named |
| engine resolved | `Provision` starts the next Scenario's container and the readiness probe answers | cluster ready |
| cluster ready | the Timeline file opens and `Record` opens the five watch streams | recording |
| cluster ready | the Timeline folder or file cannot be opened | recording, that path and its reason kept |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | `Deploy` creates the Workload's Deployment | Workload deployed |
| Workload deployed | two consecutive observations agree | Placement Settled |
| Placement Settled | the Scenario ends, whether its checks passed or failed | record stopped, the summary printed and the file closed |
| record stopped | the handle closes | container removed |
| container removed | a Scenario is left to run | engine resolved |
| any state | close, or the process exits | container removed |

A Scenario's Steps:

| From | Event | To |
|------|-------|----|
| declared | a Given constructs its Topology or its Workload | declared, that Step recorded |
| declared | a Given's constructor refuses the declaration | ended, that Given named |
| declared | the first Step needing a Placement applies the Topology, deploys the Workload, and the Placement settles | Placement observed |
| declared | the Topology, the Workload or the Settled Placement cannot be reached | ended, that Step named |
| Placement observed | a Then the Placement meets | Placement observed, that Step recorded |
| Placement observed | a Then the Placement does not meet | Placement observed, a Finding recorded |
| ended | a later Step is reached | ended, nothing carried out and nothing reported |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Running |
| Pending | no Node satisfies the Workload's rules or capacity | Pending, reported unplaceable (edge case 1) |
