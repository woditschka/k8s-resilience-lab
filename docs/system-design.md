<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for every Scenario at once, whose Steps apply its Topology, deploy a Workload, Wait for them, and report the Placement reached. It records what the cluster did, prints a summary of that Timeline and files its entries. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no Pod image is pulled.

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

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). A Scenario source holds one exported Scenario function taking `*lab.Scenario` and nothing else, in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). The `scenarios` package imports `internal/lab` alone of the lab's own packages, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). Every name a Scenario uses is one `internal/lab` offers, re-exported in one file, per [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, its only exception, per the same ADR. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. Kubernetes types stay inside `internal/cluster`, whose mapper alone names both a Kubernetes and a `scenario` type and declares the full-access function type. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file. The one home-relative path is an engine socket candidate, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). `internal/timeline` imports `internal/scenario` and the standard library alone; a recording delivers entries to a one-method sink in `internal/cluster`, per the Timeline ADR. `internal/modulerules` imports the standard library alone and no production package imports it, per [ADR: The module-rule tests share one test-support package](adr/2026-10-04-module-rule-test-support.md). Each rule is a `golangci-lint` rule, asserted again from the module's own sources (§ Contracts).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository, reached by no input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors by default, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The engine socket paths probed in order when `DOCKER_HOST` resolves nothing, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits |
| The NodePool name rule | `internal/scenario/topology.go` | Kubernetes' label-value syntax a NodePool name must meet, pinned to the client library's, per [ADR: Every NodePool is named where it is declared](adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md) |
| `scenario.DefaultNodeCapacity` | `internal/scenario/topology.go` | The capacity a Node holds when a Scenario names none, room for more than one ordinary Pod; also the Pod count of a capacity stating none |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | The budgets for a socket probe, a pull and provisioning |
| `scenario.WaitingTime` | `internal/scenario/wait.go` | The lab's own waiting time: the longest it waits for a state of the cluster, and the highest time a Wait may state |
| `pollInterval` | `internal/cluster/simulated_cluster.go` | The gap between one wait's observations |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The keys the lab writes on Nodes and Pods: KWOK's fake-Node marks, the Workload and NodePool labels, and the Startup-time annotations |
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The topology labels, taint keys and setting values a specification names |
| `scenario.DefaultStartupTime` | `internal/scenario/pod_specification.go` | The Startup time of every Pod whose specification states none (REQ-SIM-012) |
| The lab's Stage set | `internal/cluster/stages.go` | The Stages the KWOK controller applies: the image's own, its pod-ready Stage reading a Pod's Startup time, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on an unresponsive Node, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets, and the stock value it replaces, per the same ADR |
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The Timeline folder under the working directory, and the suffix marking a filed Timeline, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | A Timeline folder and file are readable and writable by the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary. A constructor errors where a caller can get the value wrong. `SimulatedCluster` is the aggregate root of a run: a Topology, a Workload and an observation are reached through it alone. A Topology derives its names from the positions its Zones and Nodes hold, counted from one, a Zone's Nodes counted through its NodePools in declaration order (REQ-SIM-013). Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. No package holds mutable process-wide state. Every Scenario runs at once, each on a Simulated Cluster provisioned for it alone and removed when it ends, per [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md). A Scenario's failure, a cluster the engine cannot start or a panic included, ends that Scenario alone (REQ-SIM-014). One engine detection and one flag parse serve the whole run, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) and [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given applies nothing: it constructs the immutable `internal/scenario` value and names itself when its constructor refuses it. A Step's mandatory value is a named argument and an optional setting a further call on the complete value, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Then the Placement does not meet records a Finding, and the later Steps still run.

A Scenario line is one of four Step kinds. A Wait is written `WaitFor()`, on the Scenario or on the Given before it. A Given's `WaitFor()` offers the one Wait fitting what it declared, per [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](adr/2026-10-07-a-scenario-writes-its-own-wait.md). The lab's own Scenarios write each Wait as a Step of its own, per [ADR: The lab's Scenarios write one Step per line](adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md). Each Wait applies what the Givens before it declared and observes the Settled Placement beneath its own condition. `AllNodesReady` adds every declared Node Ready, `AllPodsReady` every declared Pod Ready, and `PlacementSettled` nothing, so it is reached while Pods stay Pending (REQ-SIM-011). Only the Scenario's own `WaitFor()` offers `PlacementSettled`, so it stands as a Step of its own. Every Then and every Failure reads the Placement the last Wait reached; one with no Wait since the Given or Failure before it ends the Scenario there. Two Waits in a row are admitted, and a Wait before any Workload waits for no Pod. A Wait may state a time in whole seconds. A time of no seconds, or above the lab's own waiting time, is refused at the Wait (REQ-SIM-006 edge case 3). The plain form is held to that waiting time. A Wait the waiting time outlasts, or reached later than its stated time, ends the Scenario there, naming what was not Ready (REQ-SIM-008 edge case 3). A reached Wait's line names the time it took since the Step before it, an unreached one the time it was held to.

A Workload's Given names its Pod specification as a mandatory argument, with zero or more Customizations, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Customizing returns a copy; one setting per field holds, the last written winning. Full access reaches the Pod specification alone, never a Workload's label, selector or Deployment, and the domain core holds none. The mapper re-applies the specification's own tolerations after the last Customization. A value the control plane refuses ends the Scenario at the Wait deploying it. A Startup time is the one Customization naming no setting (REQ-SIM-012). The mapper writes it, or the lab's own, onto Pod template annotations full access does not reach, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md). A Step names a Workload's specification, and no report its settings.

A stated Node capacity names the cpu and the memory, and the Pod count last or not at all, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). An omitted count is the lab's own (REQ-SIM-011 edge case 1), and more than one ends the Scenario at that Given. A reported capacity reads cpu, memory, then Pod count, omitting a count the NodePool did not state. A Node reports its declared capacity as both its capacity and its allocatable room. The scheduler alone decides which Pods fit; the lab judges no Node's remaining room (REQ-SIM-011). Every quantity crossing into Kubernetes comes from the client library's own constructor, never a parsed string.

A Topology names its Zone count, then the NodePools every Zone holds and nothing else, per [the NodePool ADR](adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md). A NodePool states its name, then its Node count per Zone, and optionally a capacity, else the lab's own. Its Topology line names each NodePool in declaration order. Edge cases 1, 2, 3 and 5 end the Scenario at the Topology line. Every Node carries its NodePool's name under the lab's NodePool label (REQ-SIM-013).

A Workload's further call declares its Required Capacity, per [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](adr/2026-10-06-required-capacity-judged-at-two-moments.md). A When keeps what its Failure cost: the Pods its discarded Placement found Running apart from the Nodes stopped, the latest Failure's count held. One Then judges the minimum at that moment and at the Settled Placement after it, a miss at either a Finding. Edge cases 1 to 4 are refused at that Given, or end the Scenario at that Then.

Every Scenario's run records a Timeline, one goroutine owning it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). Receipt order is the Timeline's order: the API server's within one stream, the lab's own across them. A Timeline the run could not write is named, and the run exits non-zero.

A run prints a Timeline summary derived from the entries alone, per [ADR: A run prints a Timeline summary derived from the record](adr/2026-10-05-timeline-summary-derived-from-the-record.md). A Pod entry belongs to the Workload whose name its own begins with, at the longest match; every other object is the lab's setup, collapsed to one Topology line. The last entry reporting a readiness or a Pending reason wins. `--whole-timeline` prints every entry under the summary, in receipt order.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command, failing on the first check that fails. Its tests run under the race detector, refusing a missing C compiler with how to install one, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md). Its per-package timeout stands above what a cluster-backed package takes for every shipped Scenario. Its `-parallel` flag bounds a package's parallel tests, per § Scale and Load | `Makefile` | REQ-SIM-004, REQ-SIM-011, REQ-SIM-014 |
| `make deps-check` | Fails, naming the offender, when a prohibited dependency class (§ Dependency Policy) enters the module graph | `Makefile` | REQ-SIM-004 |
| depguard and `forbidigo` rules | Deny the imports and the in-cluster configuration call § Package Structure rules out | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006 |
| Import-graph and call-rule tests | Assert every lint rule again from the module's own package graph and files, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/cluster/callrule_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-001, REQ-SIM-004, REQ-SIM-006 |
| `modulerules` helpers | The package-graph, call and declaration scans the module-rule tests share | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.NodeCapacity`, `scenario.CPU`, `scenario.Memory` | The capacity one Node holds, refused at none in any dimension, so a Workload can exceed its Topology. Its quantities are integers Kubernetes' own quantity carries exactly, rendered in Kubernetes' spelling | `internal/scenario/topology.go`, `internal/scenario/quantity.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.Node` | One simulated machine: its Zone, its NodePool's name, and its capacity | `internal/scenario/topology.go` | REQ-SIM-001, REQ-SIM-013 |
| `scenario.NodePool` | A name, a Node count per Zone and a capacity; no Node, an empty name, and a name outside the label-value syntax are refused | `internal/scenario/topology.go` | REQ-SIM-013 |
| `scenario.Topology` | The Zones and their NodePools in declaration order, expanding to Nodes under derived names. Its one constructor takes NodePools; no NodePool and a repeated name are refused. Its test asserts the naming rule and REQ-SIM-013 edge cases 1, 2, 3 and 5 | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013 |
| NodePool-name rule test | Asserts the domain's label-value rule agreeing with the client library's validation over the same names, per [the NodePool ADR](adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md) | `internal/cluster/resources_test.go` | REQ-SIM-013 |
| `scenario.Workload` | A declared Pod count, its Pod specification, and any Required Capacity; no Pod, and a minimum outside the count, are refused | `internal/scenario/workload.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-010 |
| `scenario.PodSpecification` | The settings one Workload's Pods are deployed with, under their intent, and any Startup time: immutable and equal by value | `internal/scenario/pod_specification.go` | REQ-SIM-009, REQ-SIM-012 |
| `scenario.StartupTime` | The time a placed Pod takes to become Ready, one time or a range in whole seconds; a negative time or an inverted range is refused | `internal/scenario/pod_specification.go` | REQ-SIM-012 |
| `scenario.TopologySpread`, `scenario.Toleration`, `scenario.Request` | The settings a specification may hold, each field as Kubernetes names it. It holds one spread per topology key, one toleration per taint and one Request, per [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-011 |
| Specification-definition tests | Assert the definition and Customization rules, over REQ-SIM-009 edge cases 4 to 6 and REQ-SIM-012 edge cases 1 to 3 | `internal/lab/pod_specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009, REQ-SIM-012 |
| Specification-mapping test | Asserts the transcription rule below over edge case 7, every Kubernetes name pinned to the client library's, the Startup-time annotations whatever full access names, and the NodePool label | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-012, REQ-SIM-013 |
| Specification placement tests | Assert a deployed Pod carrying its settings, edge cases 1 to 3, Pods no Node has room for staying Pending, and no Pod Ready before its Startup time | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012 |
| `scenario.Pod`, `scenario.PodPhase` | One observed replica, placed or not, with its reported phase and readiness | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Workload's Pods sit at one observation against the Nodes that could hold them, with the Skew and the Pods not Ready or still starting | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012 |
| `scenario.Placement.PodsRunningApartFrom` | The Pods one observation found Running off the Nodes named: what a Workload kept when a Failure stopped them | `internal/scenario/placement.go` | REQ-SIM-010 |
| `scenario.NodeFailure`, `scenario.ZoneFailure` | The Nodes, or Zones, one Failure stops, resolved at construction to the first its Topology derives; a count above them is refused | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-008 |
| `scenario.StatedWait` | The one carrier of the time a Wait is held to and whether it was stated, judging a measured wait against itself | `internal/scenario/wait.go`, `internal/scenario/wait_test.go` | REQ-SIM-006 |
| Provisioned-address client | The one construction path to the control plane: the provisioned loopback address and no credential | `internal/cluster/client.go`, `internal/cluster/client_test.go` | REQ-SIM-001 |
| `cluster.DetectEngine` | Resolves the engine socket once, returning the endpoint and version that answered, or errors naming every path tried. Its test covers a `DOCKER_HOST` of another scheme, a dead one and a live one | `internal/cluster/runtime.go`, `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| `cluster.Engine`, `cluster.ClusterContainer` | The restricted engine surface over one unix socket, and the handle to the one container this run created. Its test asserts the § Threat Model surface rule from the package's declarations | `internal/cluster/runtime.go`, `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors: a leading `v`, digits and dots alone | `internal/cluster/version.go` | REQ-SIM-001 |
| `cluster.Provision` | Starts the Simulated Cluster, waits for its API server, makes the served Stages the lab's own set, and returns the handle | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-012 |
| `cluster.ObtainVersion` | Makes the engine hold the cluster image of a version, pulling it only when absent, or errors naming that version. It takes a context, the `Endpoint` and a `KubernetesVersion` alone | `internal/cluster/simulated_cluster.go`, `internal/cluster/runtime.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives, removing itself on close. Its tests, each parallel to the others, assert no cluster left behind, passed or failed, and the even Placement of REQ-SIM-001 | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready. It returns once the scheduler has placed and released a probe Pod on each, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-013 |
| `cluster.SimulatedCluster.AwaitSettled` | Polls a Workload's Placement until two agreeing observations account for every declared Pod, unplaced ones included and starting ones not, or the waiting time expires | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012 |
| `cluster.SimulatedCluster.AwaitPodsReady`, `AwaitNodesReady` | The same loop, until every declared Pod, or Node, reports Ready, erroring with those that did not | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-006 |
| `cluster.SimulatedCluster.FailNode` | Stops one Node responding: off the KWOK controller, its Ready condition Unknown. Its tests assert the Node staying NotReady and its Pods moving, for a Node and a Zone | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008 |
| `scenario.Timeline`, `scenario.Entry`, `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | The ordered, immutable record one Scenario's run left, and one change in it, rendering its own report line | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.TimelineSummary`, `scenario.Timeline.Summarise` | The report a run prints of one Timeline, a pure function of its entries | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.Record`, `StopRecording` | Open the cluster's watch streams under one goroutine delivering each entry to a sink, re-opening a closed stream once; stopping drains them and returns the complete Timeline, idempotently | `internal/cluster/timeline.go` | REQ-SIM-005 |
| Entry mapper | The one place a watched Kubernetes object becomes a `scenario.Entry` | `internal/cluster/resources.go` | REQ-SIM-005 |
| Timeline-recording tests | Assert every Node, probe Pod and Pod of the Settled Placement recorded in receipt order with their reasons, and the re-open rule. Its hand-written `watch.Interface` is the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.File` | Where a run's Timelines collect, and one Scenario's filed Timeline named for its cluster: a header, then one JSON object per entry, keeping its first write error | `internal/timeline/timeline.go`, `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.Expect`, `expect.Equal`, `expect.HaveLen` | The assertion entry point over the interface `*testing.T` and a Then share, and its matchers; `Equal` reports a `go-cmp` diff | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against, recording every Step | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Zones`, `lab.NodesPerZone`, `lab.Pods`, `lab.WorkloadName`, `lab.NodePoolName` | The named arguments of a Step's mandatory values: defined types, so a swap does not compile | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-013 |
| `lab.CPU`, `lab.MemoryGi` | The whole-core and whole-Gi spellings a capacity and a Request are written in | `internal/lab/pod_specification.go` | REQ-SIM-011 |
| `lab.DeclaredTopology`, `lab.NodePool` | The Topology one Given line declared, and one NodePool on it. `lab.NodePool` takes its name, then its Node count per Zone, and names its capacity in a further call; the Topology line takes NodePools alone, per [the NodePool ADR](adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md) | `internal/lab/scenario.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013 |
| `lab.PodSpecification`, `lab.ResilientPlacement`, `lab.DefaultPlacement` | The specification a Workload Step declares and the predefined ones, each a base customized by its intent's settings; it carries the domain value, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/lab/pod_specification.go` | REQ-SIM-009 |
| `lab.PodSpecification.Customize`, `lab.Customization` | Zero or more Customizations in the order written, a closed set: one setting, one Startup time, or one full access | `internal/lab/pod_specification.go` | REQ-SIM-009, REQ-SIM-012 |
| `lab.FullAccess`, `cluster.PodSpecAccess` | A Scenario's own change to the Kubernetes Pod specification, run after the settings are transcribed | `internal/lab/pod_specification.go`, `internal/cluster/resources.go` | REQ-SIM-009 |
| `lab.TopologySpread`, `lab.Request`, `lab.StartupSeconds`, `lab.StartupSecondsBetween` | The settings and the Startup time a Scenario may name | `internal/lab/pod_specification.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012 |
| Scenario vocabulary | The Kubernetes names `internal/lab` offers Scenarios | `internal/lab/vocabulary.go` | REQ-SIM-006, REQ-SIM-009 |
| `lab.When`, `lab.Nodes` | The When Step: a Failure of that many Nodes, or Zones, named in its Step | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-008 |
| `lab.Wait`, `lab.TopologyWait`, `lab.WorkloadWait` | The Waits `WaitFor()` offers on the Scenario and on a Given, each with a plain form and a `WithinSeconds` form | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-011 |
| `lab.Then` judgements | Judge the Settled Placement: no Pod on a failed Node or Zone, the Pods left Pending, and the Required Capacity at both moments | `internal/lab/scenario.go` | REQ-SIM-008, REQ-SIM-010, REQ-SIM-011 |
| `lab.DeclaredWorkload` | The Workload one Given line declared, and the further call naming its Required Capacity | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Step`, `lab.Finding` | One line of a Scenario as it ran, a Wait's with its timing per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md); and one Then the Placement does not meet | `internal/lab/scenario.go` | REQ-SIM-006 |
| `lab.Outcome` | What one Scenario reported: its Steps, Findings, Settled Placement, any Step left undone, and its Timeline's path or filing error | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| `lab.Runner`, `lab.NewRunner` | Runs one Scenario on its own Simulated Cluster, named for its function's file, recording its Timeline and returning the Outcome. Its method over a list derives every run name, then obtains the version once, returning the run's error naming it before any Scenario starts. It runs each Scenario in a goroutine of its own, recovering a panic into that Scenario's own error, never into a Step's Cause. It joins every goroutine before returning the results in declared order, per [the per-Scenario cluster ADR](adr/2026-10-04-cluster-per-scenario.md) | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-014 |
| Step-surface tests | Assert the Step, Failure and Required Capacity rules above without a cluster, including REQ-SIM-008 edge case 2 and REQ-SIM-010 edge cases 1 to 4 | `internal/lab/scenario_test.go`, `internal/scenario/workload_test.go`, `internal/scenario/placement_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-010, REQ-SIM-012, REQ-SIM-013 |
| Cluster-backed Step-surface tests | Assert the Step surface against a real Simulated Cluster: NodePools provisioned with their capacity and label, every Wait form, and a refused setting ending the Scenario at its Wait | `internal/lab/scenario_deployment_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013 |
| Scenario-place tests | Assert the § Package Structure rules off the Scenario sources, and every Wait a Step of its own before each Then and after each Failure | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-010 |
| Workload names, `scenarios.All` | The lab's Workload names, and the list of Scenarios a run iterates | `scenarios/workloads.go`, `scenarios/scenarios.go` | REQ-SIM-006 |
| The lab's Scenarios | Resilient placement, Node and Zone stops responding, and No room after a Zone stops responding. Each states above its Steps what it is for, naming neither function nor Step, per [the one-Step-per-line ADR](adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md) | `scenarios/resilient_placement.go`, `scenarios/node_stops_responding.go`, `scenarios/zone_stops_responding.go`, `scenarios/no_room_after_a_zone_stops_responding.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-010, REQ-SIM-011 |
| Scenarios README | What a Scenario is, how one works, its vocabulary, and every Scenario once with its intent | `scenarios/README.md` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-013 |
| `lab` command | Parses its flags once, detects the engine once, prints what it resolved and set against the defaults, then runs `scenarios.All` at once and prints each Outcome in declared order, every Scenario reaching its own outcome. A Finding, a Step left undone, or an unwritten Timeline exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-014 |
| Command tests | Assert every shipped Scenario running clean within its stated times under the lab's own Startup time, the settings printed, the Timeline report and files, and the version report. The tests judging the lines of a run of every Scenario share one run, held by a package-level once-computed fixture whose Timeline folder the package's `TestMain` removes | `cmd/lab/main_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-010, REQ-SIM-011, REQ-SIM-012, REQ-SIM-014 |

A Workload is realized as a Deployment whose Pod template transcribes its specification, one spread constraint and one toleration per setting and no minimum domain count, per [the spread ADR](adr/2026-10-04-scenario-declared-pod-spread.md). The container, the Workload label and the selector are the mapper's own, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Every specification tolerates the not-ready and unreachable Node-condition taints, NoExecute, for the waiting time the lab sets, per [the Failure ADR](adr/2026-10-05-node-failure-withholds-the-nodes-status.md). The cluster container's KWOK controller maintains marked Nodes alone; the control plane keeps Kubernetes' heartbeat timings and taints a Zone's unresponsive Nodes at one monitor pass. A Failure takes its Node out of the managed Nodes and out of the Ready Nodes a Placement reads against. The control plane then taints it unreachable and moves its Pods, per the same ADR. Provisioning makes the served Stages the lab's own set before the first Node, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md). A placed Pod therefore reads Pending until its Startup time passes. A Pod being deleted is in no Placement. Engine API responses decode into unexported types inside `internal/cluster`. A Kubernetes version with no published cluster image fails at the pull, naming the version and the image reference.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then five candidate paths | by a reviewed change to the constant | run-once, before every engine call | Probed in order within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent version, once per run before any Scenario starts; its messages unrecorded, treated as bounded | with the image's layers and the versions run | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes provisioned per run (`ApplyTopology`) | Resilient placement's 9, from 3 Zones of 3; above that unrecorded, treated as bounded | with the Zones and each NodePool's Node count declared, capped at `maxZones` Zones | run-private, written once | One create and one status update per Node, in declaration order |
| NodePools of one Topology (`scenario.Topology`) | REQ-SIM-013's 2; above that unrecorded, treated as bounded | with the NodePools a Topology line declares | run-private, built once per Given | Held as one slice in declaration order, names checked against a set at construction; O(pools) to build, O(zones × Nodes per Zone) to expand |
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
| Simulated Clusters at once (`lab.Runner`, the gate) | a run: one per Scenario shipped. The gate: per cluster-backed package, the parallel tests the gate's `-parallel` flag in the Makefile admits, times each test's clusters. The engine's own ceiling unrecorded, treated as bounded | with the Scenarios shipped and the tests needing a cluster | run-private, one container and loopback port per cluster | One goroutine per Scenario, joined before the report; the gate's `-parallel` flag. A cluster that does not start within the start timeout ends its own Scenario or test |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1). A version new to the machine also pays one image pull.
- **Limits:** one Simulated Cluster per Scenario, all at once, so a run lasts about as long as its slowest Scenario, per [the per-Scenario cluster ADR](adr/2026-10-04-cluster-per-scenario.md). The command tests judging a run of every Scenario share one run. A cluster-backed package's run time grows with its parallel batches, which the gate's stated timeout stands above (§ Contracts). Every wait is bounded by a timeout of § Constants. One Timeline is held in memory and one file open per running Scenario. The API server's own limits are unrecorded, treated as bounded.

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
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario; its name carries the Scenario and a random suffix drawn per provisioning, per [the cluster-per-Scenario ADR](adr/2026-10-04-cluster-per-scenario.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a random suffix drawn per provisioning, each port is the engine's choice, and a run removes only the container its own handle names, per the same ADR |
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
| REQ-SIM-013 | A Zone holds NodePools of Nodes with different capacities | REQ-SIM-001, REQ-SIM-011 |
| REQ-SIM-014 | A run carries out its Scenarios at once and reports them in declared order | REQ-SIM-002, REQ-SIM-003 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | any Scenario's name cannot be derived | run stopped before any cluster, that function and file named |
| engine resolved | `ObtainVersion` finds the version's image held, or pulls it once | version obtained |
| engine resolved | the version's image is absent and cannot be pulled | run stopped before any Scenario, the version named |
| version obtained | `Provision` starts each Scenario's container, every Scenario at once, and its readiness probe answers | cluster ready |
| version obtained | the engine cannot start a Scenario's container | that Scenario ended, naming why; the others run on |
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
| container removed | every Scenario of the run has ended | Outcomes reported in declared order |
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
