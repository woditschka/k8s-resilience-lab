<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for each Scenario in turn, whose Steps apply its Topology, deploy a Workload, Wait for them, and report the Placement reached. It records what the cluster did, prints a summary of that Timeline and files its entries. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no Pod image is pulled.

The binary is a modulith composed in `cmd/lab`: the top-level `scenarios` package holds the lab's own Scenarios, and six modules under `internal/` the machinery running them (§ Package Structure).

A Scenario is a Go function over a `lab.Scenario`, each line one Step, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). The control plane is eventually consistent, so every reported Placement comes from a Wait the Scenario wrote (§ Contracts), while a Timeline comes from the API's own watch streams, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md).

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

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). A Scenario source holds one exported Scenario function taking `*lab.Scenario` and nothing else, in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). Beside them stand the Workload names and the list `All`, importing `internal/lab` alone of the lab's own packages, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). Every name a Scenario uses is one `internal/lab` offers, and the names it re-exports stand in one file of its own, per [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, its only exception, per the same ADR. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file; the one home-relative path is the engine socket candidate `os.UserHomeDir` composes, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). Each rule is a `golangci-lint` rule, asserted again from the module's own sources (§ Contracts). Kubernetes types stay inside `internal/cluster`: only its mapper names both a Kubernetes and a `scenario` type, and it declares the function type a full access is written as. `internal/modulerules` holds the module-rule tests' shared helpers: it imports the standard library alone and no production package imports it, per [ADR: The module-rule tests share one test-support package](adr/2026-10-04-module-rule-test-support.md). `internal/timeline` imports `internal/scenario` and the standard library alone. A recording delivers entries to a one-method sink in `internal/cluster`, so the recording side names no file, per the Timeline ADR.

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository every pull and create names, reached by no input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors when `--k8s-version` is absent, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The engine socket paths probed in order when `DOCKER_HOST` resolves nothing, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits, a limit of the lab's own |
| `scenario.DefaultNodeCapacity` | `internal/scenario/topology.go` | The cpu, memory and Pod count a declared Node holds when a Scenario names no capacity, leaving room for more than one Pod of an ordinary Request. A capacity stating the cpu and memory alone takes its Pod count from here |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | How long a candidate socket has to answer, a pull's budget, and provisioning's |
| `scenario.WaitingTime` | `internal/scenario/wait.go` | The lab's own waiting time: the longest it waits for a state of the cluster, and the highest time a Wait may state |
| `pollInterval` | `internal/cluster/simulated_cluster.go` | The gap between one wait's observations |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The KWOK fake-Node annotation and label, the label carrying a Pod's Workload name, and the two Pod annotations KWOK's pod-ready Stage reads a Startup time from |
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The Zone and hostname labels a spread counts over, the Node-condition taint keys, and each setting's values |
| `scenario.DefaultStartupTime` | `internal/scenario/pod_specification.go` | The lab's own Startup time: one time every Pod takes whose specification states none (REQ-SIM-012) |
| The lab's Stage set | `internal/cluster/stages.go` | The Stages the KWOK controller applies: the image's own at the pinned KWOK release, its pod-ready Stage reading a Pod's Startup time, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on an unresponsive Node, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets, and the stock value it replaces, per the same ADR |
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The folder a run's Timelines collect in, under the working directory, and the suffix marking a filed one, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | What a created Timeline folder and file are readable and writable by: the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary. A constructor errors where a caller can get the value wrong, and not where the lab builds the only values. `SimulatedCluster` is the aggregate root of a run: a Topology, a Workload and an observation are reached through it alone. A Topology derives its names from the positions its Zones and Nodes hold, counted from one. Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. No package holds mutable process-wide state. Each Scenario runs on a Simulated Cluster provisioned for it alone and removed when it ends, failed or not, one Scenario after another, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md). One `DetectEngine` call and one flag parse per process serve the whole run, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) and [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given applies nothing: it constructs the immutable `internal/scenario` value and names itself when its constructor refuses it. A Step's mandatory value is a named argument and an optional setting a further call on the complete value, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Then the Placement does not meet records a Finding, and the later Steps still run.

A Scenario line is one of four Step kinds. A Wait is written `WaitFor()`, per [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](adr/2026-10-07-a-scenario-writes-its-own-wait.md). A Scenario writes it on the Scenario, or on the Given before it, whose `WaitFor()` offers the one Wait fitting what it declared. The lab's own Scenarios write each Wait as a Step of its own, per [the one-Step-per-line ADR](adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md). Either form reports the same line. A Wait that was reached names the time it took, measured from the Step before it; one that was not names the time it was held to. A stated time is marked as stated. Each Wait applies what the Givens before it declared. `AllNodesReady` holds until every declared Node reports Kubernetes' Ready condition, `AllPodsReady` until every declared Pod reports it. Each observes the Settled Placement beneath itself. `PlacementSettled` holds for that Settled Placement alone, so it is reached while Pods stay Pending, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) (REQ-SIM-011). Only the Scenario's own `WaitFor()` offers it, so it stands as a Step of its own. Every Then, and every Failure, reads the Placement the last Wait reached. One with no Wait since the Given or the Failure before it ends the Scenario at that Step. Two Waits in a row are admitted, and a Wait before any Workload waits for no Pod: a `PlacementSettled` there is reached at once. A Failure When waits for nothing and discards the observation it read. A Wait the lab's own waiting time outlasts ends the Scenario there, its line naming that time and the Step named with what was not Ready (REQ-SIM-008 edge case 3). A Wait may state in whole seconds the time it must be reached in, and the plain form of each Wait states none and is held to that waiting time. A time of no seconds, or above it, is refused at the Wait (REQ-SIM-006 edge case 3). The lab waits its own waiting time either way, judging the measured time against the time the Wait was held to and ending the Scenario at a Wait reached later.

A Workload's Given names the Pod specification its Pods are deployed with as a mandatory argument, with zero or more Customizations and nothing else, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Of its own vocabulary `internal/lab` offers the spread and the Request, the unresponsive-Node wait being no Customization, per REQ-SIM-008. Customizing returns a copy: one setting per field holds, the last written winning, in the place it was written. Full access reaches the Pod specification alone, so a Workload's label, selector and Deployment stay beyond it. The domain core holds no full access: the Step surface carries it to the mapper, which re-applies the specification's own tolerations after the last Customization, so that wait holds whatever full access left out. A value the control plane refuses ends the Scenario at the Wait deploying it, named with that Workload. The Step a Workload records names its specification, customized or not, and no report its settings. A Startup time is the one Customization naming no setting (REQ-SIM-012). Its value refuses a negative time or a shortest above its longest, ending the Scenario at that Workload's Given. The mapper writes it, or the lab's own, onto the Pod template's annotations, which full access does not reach, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md).

A stated Node capacity names the cpu and the memory, and the Pod count last or not at all, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). An omitted count is the lab's own and no statement of none (REQ-SIM-011 edge case 1). A capacity stating more than one Pod count ends the Scenario at that Given. Every line reporting a capacity names the cpu, then the memory, then the Pod count. A Given line whose Topology stated no count names the cpu and the memory alone. A Node reports the capacity its Topology declared as both its capacity and its allocatable room, and that Node's Timeline entry carries it. The scheduler alone decides which Pods fit, and the lab judges no Node's remaining room (REQ-SIM-011). A Pod no Node has room for stays Pending with the control plane's reason. A Request is transcribed onto the container the mapper owns. Every quantity crossing that boundary comes from the client library's own constructor rather than a parsed string.

A Workload Step returns the Workload it declared, and one further call on it declares that Workload's Required Capacity, per [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](adr/2026-10-06-required-capacity-judged-at-two-moments.md). A When keeps what its Failure cost, derived from the Placement it discards: the Pods found Running apart from the Nodes stopped, the latest Failure's count held. One Then judges the declared minimum at that moment and at the Settled Placement after it, its Step naming both counts. A minimum missed at either moment is a Finding naming it. Edge cases 1 to 4 are refused at that Given, or end the Scenario at that Then.

Every Scenario's run records a Timeline, one goroutine owning it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). An entry carries the time since recording started, and that receipt order is the Timeline's order: the API server's within one stream, the lab's own across them. A Timeline file's first line names the Scenario, its cluster and the Kubernetes version. One the run could not write is named, with the run exiting non-zero.

A run prints a Timeline summary rather than its entries, derived from the entries alone, per [ADR: A run prints a Timeline summary derived from the record](adr/2026-10-05-timeline-summary-derived-from-the-record.md). A Pod entry belongs to the Workload whose name its own begins with, at the longest match; every other object is the lab's own setup, collapsed to one Topology line. A Workload's readiness and a Pending Pod's reason come from the last entry reporting them, and Workloads and Pods read in the order the Timeline first names them. `--whole-timeline` prints every entry under the summary, in receipt order.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command: formatting, vet, lint, the dependency check, the tests and the build, failing on the first that fails. Its tests run under the race detector, and a missing C compiler is refused with how to install one, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md). The run states a per-package timeout of its own, above what a cluster-backed package takes for every Scenario the lab ships, per the same ADR | `Makefile` | REQ-SIM-004, REQ-SIM-011 |
| `make deps-check` | Fails and names the offender when a prohibited dependency class (§ Dependency Policy) appears in the module graph | `Makefile` | REQ-SIM-004 |
| depguard rules | Deny the imports § Package Structure states: `k8s.io` from `internal/scenario`, the machinery in a Scenario source, and the kubeconfig, home-directory and credential-plugin packages anywhere | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006 |
| `forbidigo` rule | Denies the in-cluster configuration call in any package, matched through the type checker | `.golangci.yml` | REQ-SIM-004 |
| Import-graph tests | Assert every depguard rule from the module's own package graph, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-004, REQ-SIM-006 |
| `modulerules` helpers | The package-graph, call and declaration scans the module-rule tests share, with its own test | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| Call-rule test | Asserts the in-cluster configuration call absent from the module's own files, independently of lint | `internal/cluster/callrule_test.go` | REQ-SIM-001 |
| `scenario.NodeCapacity` | The cpu, memory and Pod count one Node holds, so a Workload can exceed its Topology; a capacity of no Pod, of no cpu or of no memory is refused | `internal/scenario/topology.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.CPU`, `scenario.Memory` | The cpu and memory a Node capacity and a Request state: each the integer Kubernetes' own quantity carries exactly, millicores and mebibytes, rendering in Kubernetes' spelling | `internal/scenario/quantity.go` | REQ-SIM-011 |
| `scenario.Node` | One simulated machine, with the Zone it belongs to and its capacity | `internal/scenario/topology.go` | REQ-SIM-001 |
| `scenario.Topology` | The Zones, Nodes per Zone, and Node capacity a Scenario provisions; expands to its Nodes under the names derived for them | `internal/scenario/topology.go` | REQ-SIM-001 |
| Topology-naming test | Asserts the naming rule above, a Zone count above the limit refused, and each dimension of a capacity refused at none | `internal/scenario/topology_test.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.Workload` | A declared Pod count, the Pod specification its Pods are deployed with, and the Required Capacity it must keep Running where one was declared; a Pod count of none, and a minimum outside that count, are refused | `internal/scenario/workload.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-010 |
| `scenario.PodSpecification` | The settings one Workload's Pods are deployed with, under the intent they serve, and the Startup time it states where one is stated: immutable and equal by value | `internal/scenario/pod_specification.go` | REQ-SIM-009, REQ-SIM-012 |
| `scenario.StartupTime` | The time a placed Pod takes to become Ready: one time, or a shortest and a longest, in whole seconds. A negative time, or a shortest above its longest, is refused; equal bounds act as one time | `internal/scenario/pod_specification.go` | REQ-SIM-012 |
| `scenario.TopologySpread`, `scenario.Toleration` | The settings a specification may hold: where a Workload's Pods land and when they leave a Node, one spread per topology key and one toleration per taint, each field as Kubernetes names it, per [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009 |
| `scenario.Request` | The setting stating the cpu and memory one Pod asks a Node for, one per specification, each field as Kubernetes names it | `internal/scenario/pod_specification.go` | REQ-SIM-011 |
| Specification-definition tests | Assert the definition and Customization rules above, over REQ-SIM-009 edge cases 4 to 6 and REQ-SIM-012 edge cases 1 to 3, including equality, an empty specification refused, and a later Startup time winning | `internal/lab/pod_specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009, REQ-SIM-012 |
| Specification-mapping test | Asserts the transcription rule below field by field, over edge case 7, every Kubernetes name pinned to the client library's. A template carries the Startup time's two annotations, the lab's own where none is stated, whatever full access names | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-012 |
| Specification placement tests | Assert a deployed Pod carrying its specification's settings, customized or not, the Placement of edge cases 1 to 3, and the Pods of a Workload the Nodes have no room for staying Pending. No Pod is Ready before its Startup time, each within a stated range, and one still starting counts Pending on its Node | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012 |
| `scenario.Pod`, `scenario.PodPhase` | One observed replica, placed or not, the phase the control plane reports and whether it reports the Pod Ready | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Workload's Pods sit at one observation, against the Nodes that could hold them: the counts per Zone and per Node, the Skew, the Pods on a Node it was not read against, the placed Pods still Pending inside their Startup time, and the Pods it does not report Ready | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012 |
| `scenario.Placement.PodsRunningApartFrom` | The Pods one observation found Running on Nodes other than the ones named, which is what a Workload kept at the moment a Failure stopped those Nodes | `internal/scenario/placement.go` | REQ-SIM-010 |
| `scenario.NodeFailure`, `scenario.ZoneFailure`, their constructors | The Nodes one Failure stops responding, and the Zones a Zone Failure stops with every Node in them: resolved at construction to the first its Topology derives, refusing a count above them | `internal/scenario/failure.go` | REQ-SIM-008 |
| Failure test | Asserts the resolution rule above, and a count above the Topology refused | `internal/scenario/failure_test.go` | REQ-SIM-008 |
| `scenario.StatedWait` | The time a Wait is held to, and whether the Scenario stated it: whole seconds, refusing none and refusing above the lab's own waiting time, and judging a measured wait against itself. A Wait stating none is held to that waiting time, which this value is the one carrier of | `internal/scenario/wait.go` | REQ-SIM-006 |
| Stated-wait test | Asserts the refusals above, and a measured wait judged met or missed | `internal/scenario/wait_test.go` | REQ-SIM-006 |
| Provisioned-address client | The one construction path to the control plane: it carries the provisioned loopback address and no credential | `internal/cluster/client.go` | REQ-SIM-001 |
| Client-target test | Asserts the client rule above | `internal/cluster/client_test.go` | REQ-SIM-001 |
| `cluster.DetectEngine` | Resolves the container engine socket once, returning the endpoint and version that answered, or errors naming every path tried | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.Engine` | The restricted engine surface over one resolved unix socket: one method pulls, creates and starts a cluster container | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.ClusterContainer` | The handle to the container this run created: the identifier stays unexported, and every call acts on that container alone | `internal/cluster/runtime.go` | REQ-SIM-001 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors; the constructor admits a leading `v` with digits and dots alone | `internal/cluster/version.go` | REQ-SIM-001 |
| Engine-endpoint test | Asserts a `DOCKER_HOST` of another scheme skipped without a dial, a dead `unix://` value falling through, and a live one used without probing | `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| Engine-surface test | Asserts the engine-surface rule § Threat Model states, from the package's own declarations | `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.Provision` | Starts the Simulated Cluster on a resolved engine and version, waits for its API server, makes the served Stages the lab's own set, and returns the handle, or errors naming what it missed | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-012 |
| Cluster-removal test | Asserts a Scenario leaving no cluster behind, passed or failed, and one invocation removing only its own | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives: it applies a Topology, deploys a Workload, observes a Placement, and removes itself on close | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready with its capacity, then returns once the scheduler has placed and released one probe Pod on each, or errors naming the Nodes left without one, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go` | REQ-SIM-001 |
| `cluster.SimulatedCluster.AwaitSettled` | Polls one Workload's Placement until two agreeing observations account for every declared Pod, or the lab's waiting time expires. An unplaced Pod is accounted for, which is what the Wait for a Settled Placement holds for. A placed Pod still inside its Startup time keeps it unsettled | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012 |
| `cluster.SimulatedCluster.AwaitPodsReady`, `AwaitNodesReady` | What the two Waits observe, over the same polling loop under one further clause each: every declared Pod, and every declared Node, reporting Ready, erroring with those that did not | `internal/cluster/simulated_cluster.go` | REQ-SIM-006 |
| Readiness-wait tests | Assert each wait returning once the cluster reports every declared Pod, and every declared Node, Ready, and naming what was not Ready when the waiting time expires | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-006 |
| `cluster.SimulatedCluster.FailNode` | Stops one Node responding: it takes that Node off the KWOK controller and writes its Ready condition Unknown | `internal/cluster/simulated_cluster.go` | REQ-SIM-008 |
| Failure cluster tests | Assert a Node that stopped responding reading NotReady and staying in the cluster, its Pod leaving it, the Placement settling again off it, a whole Zone the same, and a Pod template tolerating both Node conditions at the unresponsive-Node wait | `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008 |
| Resilient placement test | Asserts every Node coming up Ready, the Placement settling at the even one REQ-SIM-001 states, and a run reporting its own Nodes and Pods alone | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001 |
| `scenario.Timeline`, `scenario.Entry` | The ordered record one Scenario's run left, and one change in it: when the lab received it, what changed, and the control plane's reason where it reported one. Immutable values; an entry renders its own report line | `internal/scenario/timeline.go` | REQ-SIM-005 |
| `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | Which of added, modified, removed, reported or noted an entry records, which kind of object it names, and the two words it reads a Node's Ready condition through | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-value test | Asserts an entry's printed line, a noted entry naming no object, and a Timeline cloning its entries at its boundary | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.TimelineSummary`, `scenario.Timeline.Summarise` | One Timeline read as the report a run prints: its entry count, the Topology, each Workload with its readiness, and each Pod with the path it took. A pure function of the entries | `internal/scenario/timeline.go` | REQ-SIM-005 |
| Timeline-summary test | Asserts the summary rule above over the entries alone, needing no cluster | `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.Record` | Opens this cluster's five watch streams and starts the one goroutine owning its Timeline, delivering each entry to the sink given. Re-opens a closed stream once, noting one it cannot | `internal/cluster/timeline.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.StopRecording` | Cancels the five watches, drains each stream, and returns the complete Timeline. A second call returns the same one, and `Close` calls it before removing the container | `internal/cluster/timeline.go` | REQ-SIM-005 |
| Entry mapper | The one place a watched Kubernetes object becomes a `scenario.Entry`: a Node's readiness and capacity, a Pod's phase and Node, ready replicas, and an Event's reason | `internal/cluster/resources.go` | REQ-SIM-005 |
| Timeline-recording test | Asserts a Scenario's Timeline holding an entry for every Node, probe Pod and Pod of its Settled Placement, in receipt order, with the control plane's reasons | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| Timeline-stream test | Asserts the re-open rule above, and a stream the lab cannot open stopping those already open. Its connection is a hand-written `watch.Interface`, the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.NewFolder` | Where a run's Timelines collect: the folder under a given root, created with the first file | `internal/timeline/timeline.go` | REQ-SIM-005 |
| `timeline.File` | One Scenario's filed Timeline, named for the Simulated Cluster it describes: it writes its header, then one JSON object per entry as it arrives, and keeps its first write error. One it could not open discards them | `internal/timeline/timeline.go` | REQ-SIM-005 |
| Timeline-file tests | Assert the folder and file rules above, and a written file reading back as recorded | `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.T`, `expect.Expect` | The assertion entry point, over the minimal interface `*testing.T` and a Then both satisfy | `internal/expect/expect.go` | REQ-SIM-001 |
| `expect.Equal`, `expect.HaveLen` | The matchers the lab asserts through; `Equal` reports a `go-cmp` diff | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against: `Given()` declares, `When()` injects a Failure, `WaitFor()` waits, `Then()` judges the Placement the last Wait reached, and the value records every Step | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Zones`, `lab.NodesPerZone`, `lab.Pods`, `lab.WorkloadName` | The named arguments a Step's mandatory values are passed as: defined types over a count or a name, so a swap does not compile | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.CPU`, `lab.MemoryGi` | The quantity spellings a Node capacity and a Request are written in, whole cores and whole Gi, each returning the integer `internal/scenario` holds | `internal/lab/pod_specification.go` | REQ-SIM-011 |
| `lab.DeclaredTopology`, `lab.DeclaredTopology.NodeCapacity` | The Topology one Given line declared, and the further call naming the capacity its Nodes hold. That call takes the cpu and the memory, and the Pod count last or not at all | `internal/lab/scenario.go` | REQ-SIM-001, REQ-SIM-011 |
| `lab.PodSpecification`, `lab.ResilientPlacement`, `lab.DefaultPlacement` | The specification a Workload Step declares, and where each predefined one is defined: a base naming it and carrying the lab's own settings, customized by the settings its intent sets. It carries the domain value rather than being one, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/lab/pod_specification.go` | REQ-SIM-009 |
| `lab.PodSpecification.Customize`, `lab.Customization` | What a Scenario changes a predefined specification with: zero or more Customizations in the order written. A Customization is one setting of the lab's own vocabulary, one Startup time, or one full access, and an unexported method keeps that set closed. Every predefined definition is written through the same call | `internal/lab/pod_specification.go` | REQ-SIM-009, REQ-SIM-012 |
| `lab.FullAccess`, `cluster.PodSpecAccess` | The Customization carrying a Scenario's own change to the Kubernetes Pod specification, and the function type the mapper declares for it, with the settings transcribed before it runs | `internal/lab/pod_specification.go`, `internal/cluster/resources.go` | REQ-SIM-009 |
| `lab.TopologySpread`, `lab.Request` | The two settings a Scenario may name, each taking the fields Kubernetes names that setting, as the type `internal/scenario` holds it in | `internal/lab/pod_specification.go` | REQ-SIM-009, REQ-SIM-011 |
| `lab.StartupSeconds`, `lab.StartupSecondsBetween` | The Startup time a Scenario may state beside a Request: one time, or a shortest and a longest, in whole seconds as a Wait's time is | `internal/lab/pod_specification.go` | REQ-SIM-012 |
| Scenario vocabulary | The Kubernetes label, taint and setting-value names `internal/lab` offers Scenarios, one line each | `internal/lab/vocabulary.go` | REQ-SIM-006, REQ-SIM-009 |
| `lab.When`, `lab.Nodes` | The When Step and its named arguments: `NodesFail` stops that many Nodes responding, and `ZonesFail` every Node of that many Zones, each naming in its Step what it stopped | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-008 |
| `lab.Wait` | The Wait Step a Scenario's own `WaitFor()` returns: `AllNodesReady`, `AllPodsReady` and `PlacementSettled`, written after the Givens or after a Failure. Each has a plain form stating no time and a `WithinSeconds` form stating one, so no Step states the lab's own waiting time | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-011 |
| `lab.TopologyWait`, `lab.WorkloadWait` | What a Given's own `WaitFor()` returns: the one Wait fitting what that Given declared, in both forms and nothing else, so every Wait is written through `WaitFor()` | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Then.NoPodsOnTheFailedNodes`, `lab.Then.NoPodsInTheFailedZones` | Judge the freshly Settled Placement: no Node, and no Zone, a When stopped holds a Pod of the Workload | `internal/lab/scenario.go` | REQ-SIM-008 |
| `lab.Then.PodsPending` | Judges how many Pods the Settled Placement leaves Pending, which is what no Node had room for | `internal/lab/scenario.go` | REQ-SIM-011 |
| `lab.DeclaredWorkload`, `lab.DeclaredWorkload.RequiredCapacity` | The Workload one Given line declared, and the further call naming the fewest Pods it must keep Running; the minimum it names renames that line | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Then.RequiredCapacityHeld` | Judges the declared minimum at the moment of the Failure and at the Settled Placement after it, its Step naming the minimum and both counts, and its matcher naming a moment missed | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Step`, `lab.StepKind` | One line of a Scenario as it ran: its kind and the name it reports. For a Wait: the time it took, the time it was held to, whether the Scenario stated that time, and whether the Wait was reached. Its line names the time it took where it was reached and the time it was held to where it was not, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Finding` | One Then outcome the Placement does not meet, naming its Step and the mismatch `expect` gave | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Outcome` | What one Scenario reported: its Steps in order, its Findings, its Settled Placement, any Step left undone, and its Timeline's path or the error that prevented filing | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| `lab.Runner`, `lab.NewRunner` | Runs one Scenario on the resolved engine: it derives the Scenario name from its function's file, provisions the Simulated Cluster, records the Timeline, hands its lines a `lab.Scenario`, removes the cluster, and returns the Outcome. A name it cannot derive errors | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| Step-surface tests | Assert the Step rules above: a Given declaring without applying, a refused Startup time ending the Scenario at its Workload's Given, a Wait applying and reporting how long it waited in either form, a stated time refused or missed ending the Scenario, a Step that cannot be carried out ending it, and a Then not met recording a Finding while later Steps run | `internal/lab/scenario_test.go` | REQ-SIM-006, REQ-SIM-012 |
| Cluster-backed Step-surface tests | Assert the Step surface against a real Simulated Cluster, which no stand-in reports. The Waits: one written with its Given and one on its own, one with nothing to wait for, and the Settled Placement reached with Pods Pending and with nothing declared. A setting the control plane refuses ends the Scenario at its Wait | `internal/lab/scenario_deployment_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011 |
| Scenario-name test | Asserts the naming rule above, and a function with no Scenario file an error naming it | `internal/lab/runner_test.go` | REQ-SIM-006 |
| Scenario-place tests | Assert the rules § Package Structure states for the Scenarios' place, off the Scenario sources themselves, and the Step rules § Contracts states: a Wait before the first Then, a Wait after every Failure, a time stated on that Wait alone, and every Wait a Step of its own | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-010 |
| Workload names | Where the lab's Scenarios take the names of the Workloads they deploy, one exported constant each | `scenarios/workloads.go` | REQ-SIM-006 |
| `scenarios.All` | The lab's Scenarios, one line each: the list a run iterates, so adding one touches the Scenarios' place alone | `scenarios/scenarios.go` | REQ-SIM-006 |
| The lab's Scenarios | Resilient placement is the simplest; Node stops responding and Zone stops responding run over its Topology and Workload. No room after a Zone stops responding runs over that Topology and Workload with every Node holding one of its Pods. A Scenario states above its Steps what it is for, in a sentence or two naming neither function nor Step, per [the one-Step-per-line ADR](adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md). Its Steps and outcome are read in its source, their place stating nothing above them | `scenarios/resilient_placement.go`, `scenarios/node_stops_responding.go`, `scenarios/zone_stops_responding.go`, `scenarios/no_room_after_a_zone_stops_responding.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-010, REQ-SIM-011 |
| Step-surface Failure tests | Assert the When rules § Contracts states, over edge case 2: the Failure injected and named, the Then after the next Wait judging the Placement reached, and a Then reading a Failure with no When ending it | `internal/lab/scenario_test.go` | REQ-SIM-008 |
| Failure-tolerance tests | Assert the Required Capacity rules above, over edge cases 1 to 4, including a Then with nothing to judge ending the Scenario | `internal/lab/scenario_test.go`, `internal/scenario/workload_test.go`, `internal/scenario/placement_test.go` | REQ-SIM-010 |
| Scenarios README | Explains what a Scenario is, how one works, and the vocabulary one may write. Names every Scenario once under its run name and its source file, with its intent, and nothing else | `scenarios/README.md` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-011 |
| `lab` command | Parses its two flags into one immutable value, detects the engine once, and prints what it resolved: the socket, both versions, the waits and eviction rate it sets against the defaults they replace, its own waiting time, and its own Startup time. Runs each Scenario `scenarios.All` holds through `lab.Runner` and prints each Outcome: its Steps, its Placement per Zone and per Node, and its Timeline summary under the path it was filed at. A Finding, a Step left undone, or an unwritten Timeline is named with its reason, and the run exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012 |
| Shipped-Scenarios test | Asserts every Scenario `scenarios.All` holds running with no Finding and no Step left undone, each Wait reporting its line and its duration within the time it states, and a run printing the waits, the eviction rate, the waiting time and the Startup time it sets. Every Scenario keeps its outcome under the lab's own Startup time | `cmd/lab/main_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-010, REQ-SIM-011, REQ-SIM-012 |
| Timeline-report test | Asserts each Scenario's summary printed, `--whole-timeline` printing every entry, two runs at once each leaving their own files, and a Scenario failing part-way printed and filed | `cmd/lab/main_test.go` | REQ-SIM-005 |
| Version-report test | Asserts a run naming no version reporting the pinned default, one naming a version mirroring it, and an unobtainable version stopping the run | `cmd/lab/main_test.go` | REQ-SIM-001 |

A Workload is realized as a Deployment whose Pod template transcribes the specification it declares: one spread constraint and one toleration per setting, each field as its setting names it, and no minimum domain count, per [the spread ADR](adr/2026-10-04-scenario-declared-pod-spread.md). The container, the Workload label and the selector are the mapper's own, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Every specification tolerates the not-ready and the unreachable Node-condition taint, NoExecute, for the waiting time the lab sets, per [the Failure ADR](adr/2026-10-05-node-failure-withholds-the-nodes-status.md). Provisioning composes the cluster container's arguments: the KWOK controller maintains marked Nodes alone, the control plane keeps Kubernetes' own heartbeat timings, and it taints a Zone's unresponsive Nodes at one monitor pass rather than its own per-Zone pace. A Failure takes its Node out of those managed Nodes, so the lab owns its status from then on, and that Node leaves the Ready Nodes a Placement is read against at once. The control plane then taints it unreachable and moves its Pods within seconds, per the same ADR. The container also enables KWOK's Stage CRD, and provisioning makes the served Stages the lab's own set before the first Node, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md). Its pod-ready Stage reads a Pod's Startup-time annotations, so a placed Pod reads Pending on its Node until its time passes. A Pod being deleted is in no Placement. Engine API responses are decoded into unexported types inside `internal/cluster`, and no engine type crosses that boundary. A Kubernetes version with no published cluster image fails at the pull, naming the version and the full image reference.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then five candidate paths | by a reviewed change to the constant | run-once, before every engine call | Probed in order within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent version; its messages unrecorded, treated as bounded | with the image's layers and the versions run | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes provisioned per run (`ApplyTopology`) | Resilient placement's 9, from 3 Zones of 3; above that unrecorded, treated as bounded | with the Topology declared, capped at `maxZones` Zones | run-private, written once | One create and one status update per Node, in declaration order |
| Probe Pods per provisioning (`ApplyTopology`) | one per declared Node, Resilient placement's 9 | with the Nodes declared | run-private, removed within provisioning | One create per Node, then one label-selected list per poll until each is Running and again until gone; O(nodes) per poll, within the start timeout |
| Pods of one Workload per observation (`Placement`) | Resilient placement's 9; above that unrecorded, treated as bounded | with the Pods declared | run-private, read-mostly | One label-selected list call, counted into maps by Node and by Zone; O(pods + nodes) per observation |
| Settings carried onto one Pod template (the mapper) | the specification's settings, Resilient placement's 4 and Default placement's 2, full accesses among them | one per field named and one per full access | run-private, written once | Transcribed in definition order, each full access called in its place; O(settings + accesses) |
| Predefined specifications, and the Customizations of one (`Customize`) | two specifications, one per intent; a line's Customizations unrecorded, treated as bounded | with the Customizations written | read once per Workload | Built per call and returned by copy; each setting matched against the fields held, each full access appended |
| Stages applied per provisioning (`Provision`) | the image's own set at the pinned KWOK release, under ten | by a reviewed change to the set or the release | run-private, written once before the first Node | One list, then one create or replace per Stage and one delete per Stage outside the set; O(stages) |
| Settle polling (`AwaitSettled`, `AwaitPodsReady`) | one observation per poll interval; one wait per Wait a Scenario writes, the Node Failure Scenario's 3 | with the Waits a Scenario writes | run-private | Two observations compared by value and scanned for unaccounted, for starting and for not-Ready Pods; O(pods + nodes) per poll, within the lab's waiting time |
| Node readiness polling (`AwaitNodesReady`) | the declared Nodes, Resilient placement's 9; one wait per Wait for every Node | with the Nodes declared | run-private | One list call per poll, the declared names matched against the Ready ones; O(nodes) per poll, within the lab's waiting time |
| Pods counted at the moment of a Failure (the When) | the Settled Placement's Pods, Resilient placement's 9, against 1 or 3 Nodes stopped | with the Pods declared and the Nodes stopped | run-private, derived once per When | One pass over the discarded observation against a set of the stopped Nodes; O(pods + nodes stopped) |
| Failure injected per When (`FailNode`) | one Node, or every Node of a Zone, the Zone Failure Scenario's 3 | with the Whens held and the Nodes per Zone declared | run-private, written once | One label and one status write per Node, retried on a conflict, in Topology order; O(nodes failed) |
| Packages and sources read by the module-rule and Scenario-place tests | a few thousand packages with the Kubernetes client, unrecorded and treated as bounded, and the module's own files, tens of them | with the dependency tree and the lab's own source | test-only, read once per test | `go list -deps -json` decoded one package at a time, each file parsed once with `go/parser`; O(packages + files + nodes) time |
| Steps recorded per Scenario (`lab.Scenario`) | Resilient placement's seven; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step, read once by the report | Appended to one slice in Step order; O(steps) |
| Timeline entries recorded per Scenario (`Record`) | Resilient placement's 150 or so over the five kinds; above that unrecorded, treated as bounded | with the Nodes and Pods declared, and the Scenario's length | run-private, appended by the recording goroutine | One slice appended in receipt order, each entry encoded to the open file as it arrives; O(1) per entry, O(entries) memory |
| Timeline entries read by the summary (`Summarise`) | the entries one Scenario recorded, 150 or so | with the entries and the Workloads | run-private, read once after the record is stopped | One pass in receipt order, grouped by Workload and by Pod name; O(entries × workloads) time, O(pods + nodes) memory |
| Watch streams open per Scenario (`Record`) | five, one per kind recorded | by a reviewed change to the kinds | run-private, open until the record is stopped | One `Watch` call per kind, merged by one `select`; a closed stream is re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, read by no run | One file, named for its Simulated Cluster |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1). A version new to the machine also pays one image pull.
- **Limits:** one Simulated Cluster at a time, provisioned per Scenario, so a run of N Scenarios pays N container starts in sequence. A test case running the lab pays the same. A cluster-backed package's run time therefore grows with the Scenarios shipped and the cases running them, which the gate's stated timeout stands above (§ Contracts). Every wait is bounded by a timeout of § Constants. One Timeline is held in memory and one file open at a time. The API server's own limits are unrecorded, treated as bounded.

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

- **Inputs it processes:** the Scenarios compiled into the binary, the `--k8s-version` and `--whole-timeline` flags, `DOCKER_HOST` when set, and the engine's and API server's responses.
- **Outputs it produces:** on stdout the settings it resolved and set, each Scenario's Placement report and Timeline summary, and a warning when `DOCKER_HOST` resolves nothing; the exit status; Kubernetes resources in the Simulated Cluster; one Timeline file per Scenario.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port.
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable, no service-account token and no home-directory file, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
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
| REQ-SIM-006 | A Scenario is a sequence of Given, When, Wait and Then Steps | REQ-SIM-001 |
| REQ-SIM-005 | A run prints and files a Timeline of what the cluster did | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-008 | A Scenario fails a Node, or a whole Zone | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-009 | A Workload declares one of the predefined Pod specifications | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-010 | A Workload declares the capacity a Failure must leave standing | REQ-SIM-008, REQ-SIM-009 |
| REQ-SIM-011 | A Node has a capacity, a Pod requests a share of it, and a Workload the Nodes still responding lack room for stays Pending | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-010 |
| REQ-SIM-012 | A placed Pod takes a Startup time before it is Ready | REQ-SIM-001, REQ-SIM-006, REQ-SIM-009 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | a Scenario's name cannot be derived | run stopped, that function and file named |
| engine resolved | `Provision` starts the next Scenario's container and the readiness probe answers | cluster ready |
| cluster ready | the Timeline file opens and `Record` opens the five watch streams | recording |
| cluster ready | the Timeline folder or file cannot be opened | recording, that path and reason kept |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | a Wait has `Deploy` create the declared Deployment | Workload deployed |
| Workload deployed | a Wait finds every declared Pod Ready and two agreeing observations accounting for them | Placement Settled |
| Workload deployed | a Wait for the Placement alone finds two agreeing observations accounting for every declared Pod | Placement Settled, Pods Pending or not |
| Placement Settled | a When stops its Nodes responding | Workload deployed, the observation discarded and the Failure's cost kept |
| Placement Settled | the Scenario ends, passed or failed | record stopped, the summary printed and the file closed |
| record stopped | the handle closes | container removed |
| container removed | a Scenario is left to run | engine resolved |
| any state | close, or the process exits | container removed |

A Scenario's Steps:

| From | Event | To |
|------|-------|----|
| declared | a Given constructs its Topology or its Workload | declared, that Step recorded |
| declared | the constructor refuses what a Given declares | ended, that Given named |
| declared | the cluster refuses what a Wait applies | ended, that Wait named with the declaration |
| declared | a Wait finds every declared Node, or every declared Pod, Ready, and the Placement has stopped changing | Placement observed, that Wait's line naming the time it took |
| declared | a Wait for the Placement alone finds it has stopped changing | Placement observed, Pods Pending or not |
| declared | the lab's own waiting time outlasts a Wait | ended, that Wait's line naming that time, the Step named with what was not Ready |
| declared | a Wait states a time of none, or above the lab's own waiting time | ended, that Wait named with that waiting time |
| declared | a Wait is reached later than the time it states | ended, that Wait's line naming that time and not the time it took |
| declared | a Then, or a Failure When, is reached with no Wait before it | ended, that Step named |
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
| Pending | the scheduler binds the Pod to a Node | Pending on that Node, inside its Startup time |
| Pending on that Node | its Startup time passes | Running and Ready |
| Pending | no Node satisfies the specification or the capacity | Pending, reported unplaceable (edge case 1) |
| Running | its Node stops responding and the waiting time passes | deleted by the control plane, replaced on a Node still responding |
