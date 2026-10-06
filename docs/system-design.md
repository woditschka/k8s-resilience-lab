<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for each Scenario in turn, applies its Topology, deploys a Workload, and reports the Placement once it is Settled. It records what the cluster did, prints a summary of that Timeline and files its entries. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no Pod image is pulled.

The binary is a modulith composed in `cmd/lab`: the top-level `scenarios` package holds the lab's own Scenarios, and six modules under `internal/` the machinery running them (§ Package Structure).

A Scenario is a Go function over a `lab.Scenario`, each line one Step, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). The control plane is eventually consistent, so every reported Placement comes from `AwaitSettled` (§ Contracts), while a Timeline comes from the API's own watch streams, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md).

## Package Structure

<!-- The module map as it exists today. One line per module: name, responsibility. -->

```
.
├── cmd/
│   └── lab/            # composition root: resolves the version and engine, runs the Scenarios, prints each Outcome
├── scenarios/          # the lab's Scenarios, their list, their Workload names and their README
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

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). A Scenario source holds one exported Scenario function taking `*lab.Scenario` and no other declaration, in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). Beside them stand the Workload names and the list `All`. All of them import `internal/lab` alone of the lab's own packages, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). Every name a Scenario uses is one `internal/lab` offers, and the names it re-exports stand in one file of its own, per [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, its only exception, per the same ADR. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file; the one home-relative path is the engine socket candidate `os.UserHomeDir` composes, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). Each rule is a `golangci-lint` rule, asserted again from the module's own packages and files (§ Contracts). Kubernetes types stay inside `internal/cluster`: only its mapper names both a Kubernetes type and a `scenario` type, and it declares the one function type a full access is written as. `internal/modulerules` holds the module-rule tests' shared helpers: it imports the standard library alone and no production package imports it, per [ADR: The module-rule tests share one test-support package](adr/2026-10-04-module-rule-test-support.md). `internal/timeline` imports `internal/scenario` and the standard library alone. The sink a recording delivers entries to is a one-method interface in `internal/cluster`, so the recording side names no file, per the Timeline ADR.

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository every pull and create names, reached by no run input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors when `--k8s-version` is absent, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The ordered engine socket paths probed when `DOCKER_HOST` does not resolve the engine, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits, a limit of the lab's own |
| `scenario.DefaultNodeCapacity` | `internal/scenario/topology.go` | What a declared Node holds when a Scenario names no capacity, leaving room for more than one Pod |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | How long one candidate socket has to answer, a pull's budget, and how long provisioning waits for the API server |
| `settleTimeout`, `pollInterval` | `internal/cluster/simulated_cluster.go` | The budget `AwaitSettled` polls within, and the gap between polls |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The KWOK fake-Node annotation and label, and the label carrying a Pod's Workload name |
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The Zone and hostname labels a spread counts over, the Node-condition taint keys, and each setting's values |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on a Node that stopped responding, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets the control plane to, and the stock value it replaces, per the same ADR |
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The folder under the working directory a run's Timelines collect in, and the suffix marking a filed one, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | What a created Timeline folder and file are readable and writable by: the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary. A constructor returns an error where a caller can get the value wrong, and none where the lab builds the only values. `SimulatedCluster` is the aggregate root of a run: a Topology, a Workload and an observation are reached through it alone. A Topology derives its names from the positions its Zones and Nodes hold, counted from one. Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. One `DetectEngine` call per process resolves the engine, and every later call uses that endpoint, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). No package holds mutable process-wide state. Each Scenario runs on a Simulated Cluster provisioned for it alone and removed when it ends, failed or not; Scenarios run one after another, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md). One flag parse resolves the Kubernetes version the whole run mirrors, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given declares alone: it constructs the immutable `internal/scenario` value and names itself when that constructor refuses it. A Step's mandatory values are named arguments, each a defined count or name type in `internal/lab`; an optional setting is a further call on the complete value, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Then the Placement does not meet records a Finding, judged through `internal/expect`, and the later Steps still run. A Scenario settles once, and once more after every When. A When awaits the Settled Placement, stops the first Nodes, or the Nodes of the first Zones, its Topology derives, and discards that observation. A count above the Nodes or the Zones held ends the Scenario at that When (edge case 2). A wait that expires ends it at the Step that needed it, naming what the last observation held (edge case 3).

A Workload's Given names the Pod specification its Pods are deployed with as a mandatory argument, with zero or more Customizations and nothing else, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Of its own vocabulary `internal/lab` offers the spread setting alone, the lab's waiting time being no Customization, per REQ-SIM-008. Customizing returns a copy: one setting per field holds, the last written winning, and every Customization applies in the place it was written. Full access reaches the Pod specification alone, so a Workload's label, selector and Deployment stay beyond it. The domain core holds no full access: the Step surface carries it to the mapper, which re-applies the specification's own tolerations after the last Customization, so the lab's waiting time holds whatever full access left out. A value the control plane refuses ends the Scenario at the Step that deployed the Workload. The Step a Workload records names its specification, customized or not, and no report its settings.

A Workload Step returns the Workload it declared, and one further call on it declares that Workload's Required Capacity, per [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](adr/2026-10-06-required-capacity-judged-at-two-moments.md). A When keeps what its Failure cost, derived from the Placement it discards: the Pods found Running on Nodes other than the ones stopped, the latest Failure's count held. One Then judges the declared minimum at that moment and at the Settled Placement after it, its Step naming both counts. A minimum missed at either moment is a Finding naming it. Edge cases 1 to 4 are refused at that Given, or end the Scenario at that Then.

Every Scenario's run records a Timeline, one goroutine owning it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). An entry carries the time since recording started, and that receipt order is the Timeline's order: the API server's within one stream, the lab's own across them. A Timeline file's first line names the Scenario, its cluster and the Kubernetes version. A Timeline the run could not write is named, with the run exiting non-zero, per the same ADR.

A run prints a Timeline summary rather than its entries, derived from the entries alone, per [ADR: A run prints a Timeline summary derived from the record](adr/2026-10-05-timeline-summary-derived-from-the-record.md). A Pod entry belongs to the Workload whose name its own begins with, at the longest match; every other object is the lab's own setup, collapsed to one Topology line. A Workload's readiness is its last Deployment entry's, and a Pending Pod's reason the control plane's from the last entry reporting it. Workloads and Pods read in the order the Timeline first names them. `--whole-timeline` prints every entry under the summary, in receipt order, per the same ADR.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command: formatting, vet, lint, the dependency check, the tests and the build, failing on the first that fails. Its tests run under the race detector, and where that needs a C compiler the refusal names how to install one, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md) | `Makefile` | REQ-SIM-004 |
| `make deps-check` | Fails and names the offender when a prohibited dependency class (§ Dependency Policy) appears in the module graph | `Makefile` | REQ-SIM-004 |
| depguard rules | Deny `k8s.io` imports from `internal/scenario`, the machinery imports in a Scenario source apart from the one Kubernetes package full access names, and the kubeconfig, home-directory and credential-plugin imports in any package | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006 |
| `forbidigo` rule | Denies the in-cluster configuration call in any package, matched through the type checker rather than the source text | `.golangci.yml` | REQ-SIM-004 |
| Import-graph tests | Assert every depguard rule from the module's own package graph, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-004, REQ-SIM-006 |
| `modulerules` helpers | The package-graph walk, call-rule scan and declaration scan the module-rule tests share, with its own test | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| Call-rule test | Asserts the in-cluster configuration call is absent from the module's own files, independently of lint | `internal/cluster/callrule_test.go` | REQ-SIM-001 |
| `scenario.NodeCapacity` | What one Node can hold, so a Workload can exceed its Topology | `internal/scenario/topology.go` | REQ-SIM-001 |
| `scenario.Node` | One simulated machine, with the Zone it belongs to and its capacity | `internal/scenario/topology.go` | REQ-SIM-001 |
| `scenario.Topology` | The Zones, Nodes per Zone, and Node capacity a Scenario provisions; expands to its Nodes under the names derived for them | `internal/scenario/topology.go` | REQ-SIM-001 |
| Topology-naming test | Asserts a Topology expands to the same Zone and Node names, and a Zone count above the limit refused | `internal/scenario/topology_test.go` | REQ-SIM-001 |
| `scenario.Workload` | A declared Pod count, the Pod specification its Pods are deployed with, and the Required Capacity it must keep Running where one was declared; a Pod count of none, and a minimum above that count or below one Pod, are refused | `internal/scenario/workload.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-010 |
| `scenario.PodSpecification` | The settings one Workload's Pods are deployed with, under the intent they serve: immutable and equal by value | `internal/scenario/pod_specification.go` | REQ-SIM-009 |
| `scenario.TopologySpread`, `scenario.Toleration` | The settings a specification may hold: where a Workload's Pods land and when they leave a Node, one spread per topology key and one toleration per taint, each field as Kubernetes names it, per [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009 |
| Specification-definition tests | Assert the definition and Customization rules above, over edge cases 4 to 6: each intent's settings, a field replaced or added, the specification left as it was, a full access in its place, equality, and an empty one refused | `internal/lab/pod_specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009 |
| Specification-mapping test | Asserts the transcription rule below field by field, over edge case 7: one constraint and one toleration per setting, every Kubernetes name pinned to the client library's, the container, label and selector the mapper's, and a full access standing where it was written | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009 |
| Specification placement tests | Assert a deployed Pod carries its specification's settings, customized or not, and the Placement of edge cases 1 to 3: two specifications and two Workloads each placed by their own, unplaceable Pods Pending with the reason, and one Zone still one Pod per Node | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009 |
| `scenario.Pod`, `scenario.PodPhase` | One observed replica, placed or not, and the phase the control plane reports | `internal/scenario/placement.go` | REQ-SIM-001 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Workload's Pods sit at one observation, against the Nodes that could hold them: a count per Zone and per Node, an empty one counted as zero, the Skew between the fullest and the emptiest domain, and the Pods on a Node it was not read against | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-008 |
| `scenario.Placement.PodsRunningApartFrom` | The Pods one observation found Running on Nodes other than the ones named, which is what a Workload kept at the moment a Failure stopped those Nodes | `internal/scenario/placement.go` | REQ-SIM-010 |
| `scenario.NodeFailure`, `scenario.ZoneFailure`, their constructors | The Nodes one Failure stops responding, and the Zones a Zone Failure stops with every Node in them: resolved at construction to the first its Topology derives, refusing a count above them | `internal/scenario/failure.go` | REQ-SIM-008 |
| Failure test | Asserts a count resolves to the same first Nodes, or Zones with their Nodes, and one above them refused | `internal/scenario/failure_test.go` | REQ-SIM-008 |
| Provisioned-address client | The one construction path to the control plane: it carries the provisioned loopback address and no credential | `internal/cluster/client.go` | REQ-SIM-001 |
| Client-target test | Asserts the client carries the provisioned address and no credential or kubeconfig | `internal/cluster/client_test.go` | REQ-SIM-001 |
| `cluster.DetectEngine` | Resolves the container engine socket once, returning the endpoint and engine version that answered, or errors naming every path it tried | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.Engine` | The restricted engine surface over one resolved unix socket: its exported methods take a context and a `KubernetesVersion`, and one pulls, creates and starts a cluster container | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.ClusterContainer` | The handle to the container this run created: the identifier stays unexported, and inspect, stop and remove act on that container alone | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors; the constructor admits a leading `v` with digits and dots alone, refusing every other value before a request | `internal/cluster/version.go` | REQ-SIM-001 |
| Engine-endpoint test | Asserts a `DOCKER_HOST` of any scheme but `unix` skipped without a dial, a dead `unix://` value falling through to a live candidate, and a live one used without probing | `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| Engine-surface test | Asserts from the package's own declarations the engine-surface rule § Threat Model states: no exported method taking an image or a container, and a container command of constants alone | `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.Provision` | Starts the Simulated Cluster on a resolved engine and version, waits for its API server and returns the handle, or errors naming what it could not reach | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| Cluster-removal test | Asserts a Scenario leaves no cluster behind, passed or failed, and a second invocation alongside the first removing only its own | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives: it applies a Topology, deploys a Workload, observes a Placement, and removes itself on close | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready with its capacity, then returns once the scheduler has placed and released one probe Pod on each, or errors naming the Nodes it placed none on, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.AwaitSettled` | The one wait a Scenario or a test calls: polls one Workload's Placement until two agreeing observations account for every declared Pod, or the settle timeout expires | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.SimulatedCluster.FailNode` | Stops one Node responding: it takes that Node off the KWOK controller and writes its Ready condition Unknown, or errors naming it | `internal/cluster/simulated_cluster.go` | REQ-SIM-008 |
| Failure cluster tests | Assert a Node that stopped responding reads NotReady and stays in the cluster, its Pod leaving it, the Placement settling again with none on it, a whole Zone stopping settling at every Pod Running on the Zones still responding, and the two Node-condition tolerations on a Pod template at the lab's waiting time | `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008 |
| Resilient placement test | Asserts every Node of the Topology coming up Ready, the Placement settling at nine Running Pods, three per Zone and one per Node, and a run reporting its own Nodes and Pods alone | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `scenario.Timeline`, `scenario.Entry` | The ordered record one Scenario's run left, and one change in it: when the lab received it, what changed, and the control plane's reason where it reported one. Immutable values; an entry renders its own report line | `internal/scenario/timeline.go` | REQ-SIM-005 |
| `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | Which of added, modified, removed, reported or noted an entry records, which kind of object it names, and the two words a Node entry reads its Ready condition through | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-value test | Asserts an entry's printed line reads the same for the same entry, a noted entry naming no object, and a Timeline cloning its entries at its boundary | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.TimelineSummary`, `scenario.Timeline.Summarise` | One Timeline read as the report a run prints: its entry count, the Topology, each Workload with its readiness, and each Pod with the path it took. A pure function of the entries, rendering its own lines | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-summary test | Asserts the summary rule above: the entry count, the lab's own setup collapsed to one Topology line, every Pod with its Node or the control plane's reason, and two Workloads counted apart. Needs no cluster | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.Record` | Opens this cluster's five watch streams and starts the one goroutine owning its Timeline, delivering each entry to the sink given. Re-opens a closed stream once and notes a re-open it cannot make | `internal/cluster/timeline.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.StopRecording` | Cancels the five watches, drains each stream to its close, and returns the complete Timeline. A second call returns the same Timeline, and `Close` calls it before removing the container | `internal/cluster/timeline.go` | REQ-SIM-005 |
| Entry mapper | The one place a watched Kubernetes object becomes a `scenario.Entry`, one kind at a time: a Node's readiness and capacity, a Pod's phase and Node, ready replicas, and an Event's reason | `internal/cluster/resources.go` | REQ-SIM-005 |
| Timeline-recording test | Asserts a Scenario's Timeline holds an entry for every Node of its Topology, every probe Pod and every Pod of its Settled Placement, in receipt order, with each reason the control plane gave | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| Timeline-stream test | Asserts a closed stream re-opened once from the last version seen, a failed re-open and a second close as noted entries, and a stream the lab cannot open stopping those already open. Its connection is a hand-written `watch.Interface`, the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.NewFolder` | Where a run's Timelines collect: the folder under a given root, created when the run opens its first file | `internal/timeline/timeline.go` | REQ-SIM-005 |
| `timeline.File` | One Scenario's filed Timeline, named for the Simulated Cluster it describes: it writes its header, then one JSON object per entry as it arrives, and keeps its first write error. One it could not open discards every entry | `internal/timeline/timeline.go` | REQ-SIM-005 |
| Timeline-file tests | Assert a missing folder created rather than failing the run, a folder or file the lab cannot write leaving its path and reason, and a written file reading back as recorded | `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.T`, `expect.Expect` | The assertion entry point, over the minimal interface `*testing.T` and the Scenario runner both satisfy | `internal/expect/expect.go` | REQ-SIM-001 |
| `expect.Equal`, `expect.HaveLen` | The matchers the lab asserts through; `Equal` reports a `go-cmp` diff | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against: `Given()` declares the Topology and the Workload, `Then()` judges the Settled Placement, and the value records every Step | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Zones`, `lab.NodesPerZone`, `lab.Pods`, `lab.WorkloadName` | The named arguments a Step's mandatory values are passed as: defined types over a count or a name, so a swapped argument does not compile | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.PodSpecification`, `lab.ResilientPlacement`, `lab.DefaultPlacement` | The specification a Workload Step declares, and where each predefined one is defined: a base naming it and carrying the lab's own settings, customized by the settings its intent sets. It carries the domain value rather than being one, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/lab/pod_specification.go` | REQ-SIM-009 |
| `lab.PodSpecification.Customize`, `lab.Customization` | What a Scenario changes a predefined specification with: zero or more Customizations in the order written. A Customization is one setting of the lab's own vocabulary or one full access, and an unexported method keeps that set closed. Every predefined definition is written through the same call | `internal/lab/pod_specification.go` | REQ-SIM-009 |
| `lab.FullAccess`, `cluster.PodSpecAccess` | The Customization carrying a Scenario's own change to the Kubernetes Pod specification, and the function type the mapper declares for it, with the settings transcribed before it runs | `internal/lab/pod_specification.go`, `internal/cluster/resources.go` | REQ-SIM-009 |
| `lab.TopologySpread` | The one setting a Scenario may name, taking the fields Kubernetes names that setting, each as the type `internal/scenario` holds it in | `internal/lab/pod_specification.go` | REQ-SIM-009 |
| Scenario vocabulary | The Kubernetes label, taint and setting-value names `internal/lab` offers Scenarios, one line each | `internal/lab/vocabulary.go` | REQ-SIM-006, REQ-SIM-009 |
| `lab.When`, `lab.Nodes` | The When Step and its named arguments: `NodesFail` awaits the Settled Placement, stops that many Nodes responding, names them in its Step, and discards that observation; `ZonesFail` does the same over every Node of that many Zones, naming the Zones | `internal/lab/scenario.go` | REQ-SIM-008 |
| `lab.Then.NoPodsOnTheFailedNodes`, `lab.Then.NoPodsInTheFailedZones` | Judge the freshly Settled Placement: no Node, and no Zone, a When stopped holds a Pod of the Workload | `internal/lab/scenario.go` | REQ-SIM-008 |
| `lab.DeclaredWorkload`, `lab.DeclaredWorkload.RequiredCapacity` | The Workload one Given line declared, and the further call naming the fewest Pods it must keep Running; the minimum it names renames that line | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Then.RequiredCapacityHeld` | Judges the declared minimum at the moment of the Failure and at the Settled Placement after it, its Step naming the minimum and both counts, and its matcher naming a moment missed | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Step`, `lab.StepKind` | One line of a Scenario as it ran: its kind, Given, When or Then, and the name it reports | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Finding` | One Then outcome the Placement does not meet, naming its Step and the mismatch `expect` reported | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Outcome` | What one Scenario reported: its name, its Steps in order, its Findings, its Settled Placement, its cluster, the Step that could not be carried out, and its Timeline with its path or the error that prevented filing | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| `lab.Runner`, `lab.NewRunner` | Runs one Scenario on the resolved engine: it derives the Scenario name from its function's file, provisions the Simulated Cluster, records the Timeline into the runner's folder, hands its lines a `lab.Scenario`, removes the cluster, and returns the Outcome. A name it cannot derive is an error naming both | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| Step-surface tests | Assert the Step rules above: a Given declaring, a Then judging, a Step that cannot be carried out ending the Scenario, a Then not met recording a Finding while later Steps run, and no When without a Failure | `internal/lab/scenario_test.go` | REQ-SIM-006 |
| Scenario-name test | Asserts a Scenario's name is its file's stem with hyphens for underscores, and that a function with no Scenario file is an error naming it | `internal/lab/runner_test.go` | REQ-SIM-006 |
| Scenario-place tests | Assert the rules § Package Structure states for the Scenarios' place: one Scenario per source and no other declaration, a Workload line of one predefined specification with its Customizations and at most one further Workload call, every name one `internal/lab` offers but the settings full access names, the two files beside it, the list, the README, and no test file or import past the one exception | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-010 |
| Workload names | Where the lab's Scenarios take the names of the Workloads they deploy, one exported constant each | `scenarios/workloads.go` | REQ-SIM-006 |
| `scenarios.All` | The lab's Scenarios, one line each: the list a run iterates, so adding one touches the Scenarios' place alone | `scenarios/scenarios.go` | REQ-SIM-006 |
| `scenarios.AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode` | The simplest Scenario, a common starting point, running as `resilient-placement`: Givens declaring three Zones of three Nodes and the nine-Pod `Checkout` Workload under Resilient placement, and Thens asserting three Pods in every Zone and one on every Node | `scenarios/resilient_placement.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-009 |
| `scenarios.ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding` | Node loss, running as `node-loss`: Resilient placement's Givens at a Required Capacity of six Pods, Thens asserting nine Running Pods and three per Zone, a When failing one Node, then both again, none on that Node, and the minimum held at eight Pods and nine | `scenarios/node_loss.go` | REQ-SIM-008, REQ-SIM-009, REQ-SIM-010 |
| `scenarios.AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding` | Zone loss, running as `zone-loss`: the same Givens and minimum, Thens asserting nine Running Pods and three per Zone, a When failing one Zone, then nine again, none in that Zone, and the minimum held at six Pods and nine | `scenarios/zone_loss.go` | REQ-SIM-008, REQ-SIM-009, REQ-SIM-010 |
| Step-surface Failure tests | Assert the When rules § Contracts states: the await, the Failure injected, the Nodes or Zones named, the next Then judging a Placement settled after it, a count above the Topology ending the Scenario, and a Then reading a Failure with no When ending it | `internal/lab/scenario_test.go` | REQ-SIM-008 |
| Failure-tolerance tests | Assert the Required Capacity rules above, over edge cases 1 to 4: a minimum declared and one refused, no Pod on a stopped Node counted at that moment, both counts stated whether met or not, a Finding naming the moment missed, and a Then with nothing to judge ending the Scenario | `internal/lab/scenario_test.go`, `internal/scenario/workload_test.go`, `internal/scenario/placement_test.go` | REQ-SIM-010 |
| Scenarios README | Names every Scenario of the lab and the outcome each asserts, and nothing else | `scenarios/README.md` | REQ-SIM-006 |
| `lab` command | Parses its two flags into one immutable value, detects the engine once, and prints what it resolved: the socket, both versions, and the waits and eviction rate it sets against the defaults they replace. Runs each Scenario `scenarios.All` holds through `lab.Runner` and prints each Outcome: its Steps, its Placement per Zone and per Node, and its Timeline summary under the path it was filed at. A Finding, a Step left undone, or a Timeline it could not write is named with its reason, and the run exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008 |
| Shipped-Scenarios test | Asserts every Scenario `scenarios.All` holds running with no Finding and no Step left undone, each settling at the Placement its Thens assert, each Failure Scenario's Step stating its counts, and a run printing the waits and the eviction rate it sets | `cmd/lab/main_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-010 |
| Timeline-report test | Asserts each Scenario's summary printed, `--whole-timeline` printing every entry in receipt order, two Scenarios and two runs at once each leaving their own files, and a Scenario failing part-way printed and filed, outliving its cluster | `cmd/lab/main_test.go` | REQ-SIM-005 |
| Version-report test | Asserts a run naming no version reporting the pinned default, one naming a version mirroring it, and a version the lab cannot obtain stopping the run before any Scenario | `cmd/lab/main_test.go` | REQ-SIM-001 |

A Workload is realized as a Deployment whose Pod template transcribes the specification it declares: one spread constraint and one toleration per setting, each field as its setting names it, and no minimum domain count, per [the spread ADR](adr/2026-10-04-scenario-declared-pod-spread.md). The container, the Workload label and the selector are the mapper's own, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Every specification tolerates the not-ready and the unreachable Node-condition taint, NoExecute, for the waiting time the lab sets, per [the Failure ADR](adr/2026-10-05-node-failure-withholds-the-nodes-status.md). Provisioning composes the cluster container's arguments: the KWOK controller maintains marked Nodes alone, the control plane keeps Kubernetes' own heartbeat timings, and it taints a Zone's unresponsive Nodes at one monitor pass rather than its own per-Zone pace. A Failure takes its Node out of those managed Nodes, so the lab owns its status from then on, and that Node leaves the Ready Nodes a Placement is read against at once. The control plane then taints it unreachable and moves its Pods within about four seconds, a whole Zone's within about seven, per the same ADR. A Pod being deleted is in no Placement. Engine API responses are decoded into unexported types inside `internal/cluster`, and no engine type crosses that boundary. A Kubernetes version with no published cluster image fails at the pull, naming the version and the full image reference.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then five candidate paths | by a reviewed change to the constant | run-once, before every engine call | Probed in order, one version request each within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent version; its messages unrecorded, treated as bounded | with the image's layers and the versions run | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes provisioned per run (`ApplyTopology`) | Resilient placement's 9, from 3 Zones of 3; above that unrecorded, treated as bounded | with the Topology declared, capped at `maxZones` Zones | run-private, written once | One create and one status update per Node, in declaration order |
| Probe Pods per provisioning (`ApplyTopology`) | one per declared Node, Resilient placement's 9 | with the Nodes declared | run-private, created and removed within provisioning | One create per Node, then one label-selected list per poll until each is Running and again until gone; O(nodes) per poll, within the start timeout |
| Pods of one Workload per observation (`Placement`) | Resilient placement's 9; above that unrecorded, treated as bounded | with the Pods declared | run-private, read-mostly | One label-selected list call, counted into maps by Node and by Zone; O(pods + nodes) per observation |
| Settings carried onto one Pod template (the mapper) | the specification's settings, Resilient placement's 4 and Default placement's 2, full accesses among them | one per field named and one per full access | run-private, written once | Transcribed in definition order, each full access called in its place; O(settings + accesses) |
| Predefined specifications, and the Customizations of one (`Customize`) | two specifications, one per intent; a line's Customizations unrecorded, treated as bounded | by a reviewed change, and with the Customizations written | read once per Workload | Built per call and returned by copy; each setting matched against the fields held, each full access appended |
| Settle polling (`AwaitSettled`) | one observation per poll interval; one wait per Scenario and one after every When, Node loss's 2 | with the Whens a Scenario holds | run-private | Two observations compared by value and scanned for unaccounted Pods; O(pods + nodes) per poll, within the settle timeout |
| Pods counted at the moment of a Failure (the When) | the Settled Placement's Pods, Resilient placement's 9, against 1 or 3 Nodes stopped | with the Pods declared and the Nodes stopped | run-private, derived once per When | One pass over the discarded observation against a set of the stopped Nodes; O(pods + nodes stopped) |
| Failure injected per When (`FailNode`) | one Node, or every Node of a Zone, Zone loss's 3 | with the Whens held and the Nodes per Zone declared | run-private, written once | One label and one status write per Node, retried on a conflict, in Topology order; O(nodes failed) |
| Package graph read by the import-graph tests | a few thousand packages with the Kubernetes client; unrecorded, treated as bounded | with the dependency tree | test-only, read once per test | `go list -deps -json` decoded one package at a time; O(packages) time, constant memory |
| Go sources read by the module-rule and Scenario-place tests | the module's own files, tens of them | with the lab's own source | test-only, read once per test | Each file parsed once with `go/parser`, then its declarations, imports and calls scanned; O(files + nodes) |
| Steps recorded per Scenario (`lab.Scenario`) | Resilient placement's five; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step, read once by the report | Appended to one slice in Step order; O(steps) |
| Timeline entries recorded per Scenario (`Record`) | Resilient placement's 150 or so over the five kinds; above that unrecorded, treated as bounded | with the Nodes and Pods declared, and the Scenario's length | run-private, appended by the recording goroutine | One slice appended in receipt order, each entry encoded to the open file as it arrives; O(1) per entry, O(entries) memory |
| Timeline entries read by the summary (`Summarise`) | the entries one Scenario recorded, 150 or so | with the entries recorded and the Workloads deployed | run-private, read once after the record is stopped | One pass in receipt order, grouped by Workload and by Pod name; O(entries × workloads) time, O(pods + nodes) memory |
| Watch streams open per Scenario (`Record`) | five, one per kind recorded | by a reviewed change to the kinds | run-private, open from provisioning until the record is stopped | One `Watch` call per kind, merged by one `select`; a closed stream is re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, read by no run | One file per Scenario, named for its Simulated Cluster |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1). A version new to the machine also pays one image pull.
- **Limits:** one Simulated Cluster at a time, provisioned per Scenario and removed when it ends, so a run of N Scenarios pays N container starts in sequence. Every wait is bounded by a timeout of § Constants. One Timeline is held in memory and one file open at a time, and a summary derives from that Timeline. The API server's own limits are unrecorded, treated as bounded.

## Dependency Policy

Minimize external dependencies.

### Approved Sources

| Source | Examples | Rationale |
|--------|----------|-----------|
| Go standard library | `net`, `net/http`, `net/url`, `encoding/json`, `os`, `log/slog`, `slices`, `maps`, `testing` | The first choice for every need, including the engine's HTTP API over a unix socket |
| Kubernetes project | `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` | The control-plane client every Kubernetes call in `internal/cluster` goes through |
| Google | `github.com/google/go-cmp` | Struct comparison and failure diffs in `internal/expect` |

### Adding a New Dependency

Before adding a dependency, verify that the standard library cannot solve it and the library is established, that its source is approved above or an ADR records the exception, that its transitive dependencies are reviewed with any unknown module flagged, and that its checksum is verified and the lockfile committed.

### Prohibited

<!-- Dependency classes this project rejects, with the reason. -->

| Class | Examples | Reason |
|-------|----------|--------|
| Assertion libraries | Gomega, testify | Every assertion goes through `internal/expect`, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md) |
| Mock frameworks | gomock, mockery | Tests use real values and real I/O (`docs/testing-principles.md` § Mocking Policy) |
| Container-orchestration clients | testcontainers-go, the Docker SDK | The engine is driven through its HTTP API over a unix socket with the standard library, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A second Kubernetes client | controller-runtime, `k8s.io/kubectl` | One client library holds the Kubernetes boundary, per [the KWOK ADR](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) |

`make deps-check` carries this table's mechanical form (§ Contracts).

## Security Context

<!-- PROJECT: Describe this application's security profile: what it connects to, what it exposes, how it handles credentials, and how it runs (systemd, container, etc.). The security-reviewer reads this section before reviewing. -->

- **Inputs it processes:** the Scenario descriptions compiled into the binary, the `--k8s-version` and `--whole-timeline` flags, `DOCKER_HOST` when set, the engine's JSON responses, and the API server's watch responses.
- **Outputs it produces:** on stdout the resolved socket, both versions, the waiting time it sets, each Scenario's Placement report and Timeline summary, and a warning when `DOCKER_HOST` does not resolve the engine; the process exit status; Kubernetes resources in the provisioned Simulated Cluster; one Timeline file per Scenario.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port.
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable and no service-account token, and no file under the home directory, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
- **Runtime:** a developer machine or a build agent, as the invoking user, with a container engine socket that answers.

## Threat Model

| Threat | Attack Vector | Mitigation |
|--------|--------------|------------|
| A run drives a real cluster (NG-1) | An ambient kubeconfig, a `KUBERNETES_*` variable, or an in-cluster service-account token discovered by the client | The configuration surface is `--k8s-version`, `--whole-timeline` and `DOCKER_HOST`, none carrying a cluster address or a credential. The client is constructed from the provisioned address alone, held by the lint rules and tests of `docs/security-principles.md` § Language Realization, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A third party reaches the unauthenticated API server | The engine publishes the API port on every interface | The API port is published on the loopback address, on a host port the engine chooses, per the same ADR |
| A run drives an engine that is not on this machine | A `DOCKER_HOST` naming a `tcp://` or `ssh://` endpoint | Only a `unix://` path is ever dialed. The value is parsed as a URL, never interpolated into a command, and another scheme is skipped with a warning naming it, per the same ADR |
| The engine socket grants control of every container on the machine | The lab holds an API handle the engine cannot scope down | The exported engine methods take a context and a `KubernetesVersion` alone, never an image and never a container. Inspect, stop and remove act on the handle this run created, and the package declares no list and no by-identifier call, per the same ADR |
| A run pulls or starts an image that is not a KWOK cluster image | The version input interpolated into an image reference | The repository is a constant no input reaches, and the tag comes from the constant KWOK release and a validated version admitting a leading `v` with digits and dots alone, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md). The container's command is constants alone |
| A moved image tag changes what the Simulated Cluster runs | The pinned tag is re-pushed upstream | The Kubernetes version is pinned by tag, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md); digest pinning is not in force |
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario; its name carries the Scenario and the invocation, per [the cluster-per-Scenario ADR](adr/2026-10-04-cluster-per-scenario.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a suffix unique to the invocation, each port is the engine's choice, and a run removes only the container its own handle names, per the same ADR |
| A Timeline is written outside its folder | A Scenario name interpolated into a path | A file name is the lab's constant prefix, a compiled-in source file's stem, and a random suffix, joined to the folder as one path element; `internal/timeline` refuses a separator or a parent reference in it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| A filed Timeline exposes what a run did to another user | The file outlives the run, in the working directory | A created Timeline folder and file are readable and writable by the invoking user alone, and a Timeline holds the simulated objects of a cluster carrying no credential and no real Workload, per the same ADR |

## Implementation Order

| ID | Name | Depends On |
|----|------|------------|
| REQ-SIM-004 | Go build floor: the `make ci` gate and the lint rules | — |
| REQ-SIM-001 | Topology provisioning, Workload deployment, the Settled Placement report | REQ-SIM-004 |
| REQ-SIM-002 | One invocation runs every Scenario | REQ-SIM-001 |
| REQ-SIM-003 | One Scenario runs on its own | REQ-SIM-002 |
| REQ-SIM-006 | A Scenario is a sequence of Given, When and Then Steps | REQ-SIM-001 |
| REQ-SIM-005 | A run prints and files a Timeline of what the cluster did | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-008 | A Scenario fails a Node, or a whole Zone | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-009 | A Workload declares one of the predefined Pod specifications | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-010 | A Workload declares the capacity a Failure must leave standing | REQ-SIM-008, REQ-SIM-009 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | a Scenario's name cannot be derived from its file | run stopped, that function and file named |
| engine resolved | `Provision` starts the next Scenario's container and the readiness probe answers | cluster ready |
| cluster ready | the Timeline file opens and `Record` opens the five watch streams | recording |
| cluster ready | the Timeline folder or file cannot be opened | recording, that path and its reason kept |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | `Deploy` creates the Workload's Deployment | Workload deployed |
| Workload deployed | two agreeing observations account for every declared Pod | Placement Settled |
| Placement Settled | a When stops its Nodes responding | Workload deployed, the observation discarded and what the Failure cost kept |
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
| declared | a When asks for more Nodes, or more Zones, than the Topology holds | ended, that When named |
| Placement observed | a When stops its own Nodes responding | declared, the observation discarded |
| Placement observed | a Then the Placement meets | Placement observed, that Step recorded |
| Placement observed | a Then the Placement does not meet | Placement observed, a Finding recorded |
| Placement observed | a Then reads a Failure and no When ran, or judges a minimum the Workload never declared | ended, that Then named |
| ended | a later Step is reached | ended, nothing carried out and nothing reported |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Running |
| Pending | no Node satisfies the declared specification or capacity | Pending, reported unplaceable (edge case 1) |
| Running | its Node stops responding and the waiting time passes | deleted by the control plane, replaced on a Node still responding |
