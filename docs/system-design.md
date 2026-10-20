<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for every Scenario at once, whose Steps apply its Topology, deploy a Workload, Wait for them, and report the Placement reached. It records what the cluster did as a Timeline, prints its summary and files it. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no Pod image is pulled.

The binary is a modulith composed in `cmd/lab`: the `scenarios` package holds the lab's own Scenarios, and the modules under `internal/` the machinery running them (§ Package Structure).

A Scenario is a Go function over a `lab.Scenario`, each line one Step, per [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md). The control plane is eventually consistent, so every reported Placement comes from a Wait the Scenario wrote (§ Contracts), while a Timeline comes from the API's own watch streams, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](adr/2026-10-05-timeline-from-the-watch-stream.md).

## Package Structure

<!-- The module map as it exists today. One line per module: name, responsibility. -->

```
.
├── cmd/
│   └── lab/            # composition root: runs the Scenarios, prints each Outcome
├── scenarios/          # the lab's Scenarios, every value they define, and their README
├── internal/
│   ├── cluster/        # Simulated Cluster: container lifecycle, Kubernetes calls, anti-corruption mapping
│   ├── expect/         # Expect(t, got).To(matcher) and its matchers
│   ├── lab/            # the Step surface: the Scenario value, its Steps, and the runner
│   │   ├── node/       # what a Node is, as a Scenario writes it: NodePool, capacity, labels, taints
│   │   └── pod/        # what a Pod carries, as a Scenario writes it: specification, settings, Startup time
│   ├── modulerules/    # the package-graph walk and call-rule scan the module-rule tests share
│   ├── scenario/       # Topology, Workload, Placement, Timeline values; no Kubernetes import
│   └── timeline/       # the Timeline folder, its file names and its JSON Lines format
├── timelines/          # every run's filed Timelines, git-ignored
├── .golangci.yml       # lint configuration carrying the module rules
├── Makefile            # the gate command and the targets it runs
├── go.mod              # the module path and the Go version
└── go.sum              # the module graph's committed checksums
```

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). `scenarios/scenarios.go` holds `All`, then every Scenario in `All`'s order, and no other source holds one, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Scenario is an exported function over `*lab.Scenario`, named in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). The `scenarios` package imports `internal/lab`, `internal/lab/pod` and `internal/lab/node` alone of the lab's own packages, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). Each name a Scenario writes sits in the package of its subject, `lab` for the Scenario, `pod` for a Pod and `node` for a Node, or is one the Scenarios define (REQ-SIM-016). `lab` imports `pod` and `node`, neither imports `lab` or the other, and the split stops there, per [ADR: Scenario code stays idiomatic Go, read by subject](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A setting of several values is a keyed literal of `pod` or `node` under Kubernetes' type and field names, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, per [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). The place declares the lab's Workload names, and one function per Named Node capacity, per predefined Pod specification and per Named setting, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) and [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Each takes nothing but `EvenSpreadOver`, which takes the topology key it spreads over, and each returns one value of `pod` or `node`. The place also declares the Node label keys the Scenarios state, one string constant each (REQ-SIM-017). Scenario code is idiomatic Go, imports no package by dot and passes the gate's format and lint checks (REQ-SIM-016), per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. Kubernetes types stay inside `internal/cluster`, whose mapper alone names both a Kubernetes and a `scenario` type and declares the full-access function type. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file; the one home-relative path is an engine socket candidate, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md). `internal/timeline` imports `internal/scenario` and the standard library alone, and a recording delivers entries to a one-method sink in `internal/cluster`, per the Timeline ADR. `internal/modulerules` imports the standard library alone and no production package imports it, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md). Each rule is a `golangci-lint` rule, asserted again from the module's own sources (§ Contracts).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository, reached by no input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors by default, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The engine socket paths probed in order when `DOCKER_HOST` resolves nothing, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits |
| The NodePool name rule | `internal/scenario/topology.go` | Kubernetes' label-value syntax a NodePool name must meet, pinned to the client library's, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) |
| The Named Node capacities | `scenarios/nodes.go` | Each Node capacity the Scenarios define once under a name, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) |
| The predefined Pod specifications | `scenarios/pods.go` | Each Pod specification the Scenarios define once under its intent, with its Startup time, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | The budgets for a socket probe, a pull and provisioning |
| `scenario.WaitingTime` | `internal/scenario/wait.go` | The longest the lab waits for a state of the cluster, and the highest time a Wait may state |
| `pollInterval` | `internal/cluster/simulated_cluster.go` | The gap between one wait's observations |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The keys the lab writes on Pods, the annotations it writes on Nodes, and the simulator's selection label, a literal here because the simulator's command is composed from this package's own literals, pinned by test to the domain's lab-own keys |
| The lab's own Node label keys and the label syntax | `internal/scenario/node_labels.go` | The keys a NodePool may not state, the NodePool label the mapper writes among them, and Kubernetes' label key and value syntax, pinned to the mapper and the client library, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) |
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The topology labels, taint keys and setting values a specification names |
| The lab's Stage set | `internal/cluster/stages.go` | The Stages the KWOK controller applies, the pod-ready one reading a Pod's Startup time, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on an unresponsive Node, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets, and the stock value it replaces, per the same ADR |
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The Timeline folder under the working directory, and the suffix marking a filed Timeline, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | A Timeline folder and file are readable and writable by the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary. A constructor errors where a caller can get the value wrong. `SimulatedCluster` is the aggregate root of a run: a Topology, a Workload and an observation are reached through it alone. A Topology derives its names from positions counted from one, a Zone's Nodes counted through its NodePools in declaration order (REQ-SIM-013). Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. No package holds mutable process-wide state. A Scenario name is the condition half of its function's name, lowercase ASCII words joined by hyphens, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A function with no condition, or two deriving one name, stops the run before any Simulated Cluster, naming each function (REQ-SIM-020). Every Scenario runs at once on a Simulated Cluster provisioned for it alone and removed when it ends, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). A Scenario's failure, a panic included, ends that Scenario alone (REQ-SIM-014). One engine detection and one flag parse serve the whole run, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) and [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given constructs its immutable value and applies nothing. A Step's mandatory value is a named argument and an optional setting a further call, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Then the Placement does not meet records a Finding, and the later Steps still run.

A Scenario line is one of four Step kinds. A Wait is written `WaitFor()` on the Scenario, or on the Given before it, which offers the one Wait fitting what it declared, per [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](adr/2026-10-07-a-scenario-writes-its-own-wait.md). The lab's own Scenarios write each Wait as a Step of its own, per the same ADR. A Wait applies what the Givens before it declared and observes the Settled Placement beneath its own condition: every Node Ready, every Pod Ready, or nothing more, as `PlacementSettled`, a Step of its own reached while Pods stay Pending (REQ-SIM-011). Every Then and every Failure reads the Placement the last Wait reached, and one with no Wait since the Given or Failure before it ends the Scenario. A Wait's time is held to the lab's own waiting time, stated or not; § State Machine carries where a Wait ends the Scenario (REQ-SIM-006, REQ-SIM-008).

A Workload's Given names its Pod specification as a mandatory argument, with zero or more Customizations, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Customizing returns a copy, the last setting per field winning. Full access reaches the Pod specification alone, never a Workload's label, selector or Deployment, and the domain core holds none. The mapper re-applies the specification's own tolerations after the last Customization, and writes its Startup time onto Pod template annotations full access does not reach, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) (REQ-SIM-012). A value the control plane refuses ends the Scenario at the Wait deploying it. The run prints every setting the domain holds for a Workload's Pods beneath its Given line, in the order held and as a manifest writes it, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Named setting prints as the settings it stands for, a full access as one line, and a refused specification as none (REQ-SIM-016).

The lab defines no Pod specification and holds no Startup time of its own, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A definition goes through `pod.Predefined` (REQ-SIM-015); one omitting its Startup time does not compile, and an empty name or the zero Startup time ends the Scenario at the Given declaring it. A definition names the Named setting `LeaveUnresponsiveNodes` and writes neither lab-taint toleration itself, per the same ADR. The Scenarios define that setting through `pod.Settings`, the one grouping means the lab offers, and the lab names no group of its own (REQ-SIM-009, REQ-SIM-016). They define the Even spread as `EvenSpreadOver`, one topology spread constraint over the key given, and Resilient placement names it over `node.Zone` and over `node.Hostname` (REQ-SIM-016).

Every field of a setting the lab offers is required, and its zero value is that field left out, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A Pod setting left out of a field ends the Scenario at the Given declaring its Workload, naming the setting and the field under its Kubernetes name (REQ-SIM-016). A Pod count reads by its subject: `lab.Pods` for a Workload's size and Required Capacity, the `Pods` field of `node.Capacity` for a Node's room (REQ-SIM-016). A toleration's required fields follow its operator and effect: Equal takes a value and Exists none, NoExecute takes a waiting time and NoSchedule none. A field stated against them is refused at the Given, as one left out is (REQ-SIM-019).

A later Workload Given gives the Scenario its next Workload, per [the taint ADR](adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md). The next Wait deploys it beside the earlier ones, which stay deployed, and every Wait, When and Then after it observes and judges the newest Workload alone. A Workload name the Scenario already declared ends the Scenario at that Given (REQ-SIM-019).

A NodePool's Node capacity is stated in every dimension, or named, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). The domain refuses a capacity of none in any dimension, one left out of `node.Capacity` included, at the Topology Given (REQ-SIM-011, REQ-SIM-016). The lab gives no capacity and defines no Named one; the Scenarios define each and hand it to the lab (REQ-SIM-015). A Node reports its declared capacity as both its capacity and its allocatable room, and the scheduler alone decides which Pods fit (REQ-SIM-011). Every quantity crossing into Kubernetes comes from the client library's own constructor, never a parsed string.

A Topology names its Zone count, then the NodePools every Zone holds, each stating its name, Node count per Zone and capacity, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). Every Node carries its NodePool's name under the lab's NodePool label (REQ-SIM-013).

A NodePool states Node labels of its own through a further call taking `node.Labels`, Kubernetes' label map, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). The lab's own keys are those it writes on every Node and any under its prefix. The domain refuses one of them, a key or value outside Kubernetes' label syntax, and one key stated twice, at the Topology Given (REQ-SIM-017). The mapper writes the lab's own labels after the stated ones. The simulator's selection label stays a literal of `internal/cluster`, which composes the simulator's command from its own literals alone, and a mapper test pins it to the domain's lab-own keys. The Given line reports a NodePool's stated labels after its capacity, as `key=value` in key order. An observation reads every label a Node carries beside the lab's own. A Then counting Pods per value of a key judges every value the NodePools state, a value holding no Pod as zero. A Pod on a Node without that key counts towards no value, and a key no NodePool states ends the Scenario at that Then (REQ-SIM-017). A When names one stated label and value as `node.Labels`, a keyed literal per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). It stops every observed Node carrying it, named in the Topology's order. The domain refuses a literal of other than one label, a key or value no NodePool states, and a value no observed Node carries, at that When (REQ-SIM-018).

A NodePool states taints through a further call taking `node.Taint` values, each a keyed literal of key, value and effect, per [ADR: A NodePool states NoSchedule taints, and a later Workload deploys beside the earlier, judged alone](adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md). The domain refuses, at the Topology Given, an effect other than NoSchedule, a field left out, a key or value outside Kubernetes' syntax. It also refuses one key stated twice and a key under the control plane's Node-condition prefix (REQ-SIM-019). The mapper writes them as the Node's taints, and each probe Pod tolerates its own Node's taints, so provisioning still ends on a tainted Node. The Given line reports them after the labels, as `tainted key=value:Effect` in key order. A Then counting Pods on one NodePool reads the observed Node's NodePool label, and a name the Topology lacks ends the Scenario at that Then.

A Workload's further call declares its Required Capacity, judged at the moment of the Failure and at the Settled Placement after it, a miss at either a Finding, per [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](adr/2026-10-06-required-capacity-judged-at-two-moments.md). A When keeps what its latest Failure cost: the Pods its discarded Placement found Running off the Nodes stopped (REQ-SIM-010).

Every Scenario's run records a Timeline under one owning goroutine, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). Receipt order is the Timeline's order: the API server's within one stream, the lab's own across them. A Timeline the run could not write is named, and the run exits non-zero.

A run prints a Timeline summary derived from the entries alone, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). A Pod entry belongs to the Workload whose name begins its own, at the longest match; every other object collapses to one Topology line. The last entry reporting a readiness or a Pending reason wins. `--whole-timeline` adds every entry in receipt order.

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command, failing on the first check that fails. Its tests run under the race detector, refusing a missing C compiler with install guidance, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md). Its per-package timeout stands above a cluster-backed package's run, and its `-parallel` flag bounds a package's parallel tests (§ Scale and Load) | `Makefile` | REQ-SIM-004, REQ-SIM-011, REQ-SIM-014 |
| `make deps-check` | Fails, naming the offender, when a prohibited dependency class (§ Dependency Policy) enters the module graph | `Makefile` | REQ-SIM-004 |
| depguard and `forbidigo` rules | Deny the imports and the in-cluster configuration call § Package Structure rules out | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006 |
| Import-graph and call-rule tests | Assert every lint rule again from the module's own package graph and files, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/cluster/callrule_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-001, REQ-SIM-004, REQ-SIM-006 |
| `modulerules` helpers | The package-graph, call and declaration scans the module-rule tests share | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.NodeCapacity`, `scenario.CPU`, `scenario.Memory` | The capacity one Node holds, refused at none in any dimension, so a Workload can exceed its Topology. Its quantities are integers Kubernetes' own quantity carries exactly | `internal/scenario/topology.go`, `internal/scenario/quantity.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.NamedNodeCapacity`, `scenario.NewNamedNodeCapacity` | A Node capacity under a name a Scenario gives it, refused at an empty name; the package defines none of its own, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-011, REQ-SIM-015 |
| `scenario.Node` | One simulated machine: its Zone, its NodePool's name, its capacity, its stated labels and taints, compared through its equality method | `internal/scenario/topology.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.NodeTaints`, `scenario.Taint` | The NoSchedule taints a NodePool states, immutable and equal by value, refused as § Contracts states | `internal/scenario/node_taints.go`, `internal/scenario/node_taints_test.go` | REQ-SIM-019 |
| `scenario.NodeLabels` | The labels a NodePool states, immutable and equal by value; a lab-own key, a key or value outside the label syntax, and a repeated key are refused | `internal/scenario/node_labels.go`, `internal/scenario/node_labels_test.go` | REQ-SIM-017 |
| `scenario.NodePool` | A name, a Node count per Zone, a capacity and any stated labels and taints; no Node, an empty name, a name outside the label-value syntax, and the zero capacity are refused | `internal/scenario/topology.go` | REQ-SIM-013, REQ-SIM-015, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Topology` | The Zones and their NodePools in declaration order, expanding to Nodes under derived names and their NodePool's labels, and naming the values its NodePools state for a key; no NodePool and a repeated name are refused. Its test asserts the naming rule, REQ-SIM-013 edge cases 1, 2, 3 and 5, REQ-SIM-017 edge cases 1 to 3 and REQ-SIM-019 edge cases 1 to 3 | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Label rule tests | Assert the domain's label and taint key and value rules agreeing with the client library's validation over the same names, and the domain's lab-own keys being those the mapper writes, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/cluster/resources_test.go` | REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Workload` | A declared Pod count, its Pod specification, and any Required Capacity; no Pod, and a minimum outside the count, are refused | `internal/scenario/workload.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-010, REQ-SIM-019 |
| `scenario.PodSpecification` | The settings one Workload's Pods are deployed with under their intent, and their Startup time: immutable and equal by value. An empty name is refused, and the lab's two tolerations hold at its waiting time | `internal/scenario/pod_specification.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015 |
| `scenario.StartupTime` | The time a placed Pod takes to become Ready, one time or a range, built from whole milliseconds; a negative time or an inverted range is refused | `internal/scenario/pod_specification.go` | REQ-SIM-012 |
| `scenario.TopologySpread`, `scenario.Toleration`, `scenario.Request` | The settings a specification may hold, each field as Kubernetes names it: one spread per topology key, one toleration per taint and one Request, per [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-011, REQ-SIM-019 |
| Specification-definition tests | Assert the definition, Customization and grouping rules on specifications and groups the tests define, over REQ-SIM-009 edge case 3, REQ-SIM-012 edge cases 1 to 3 and REQ-SIM-015 edge cases 1 and 2 | `internal/lab/pod/specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016 |
| Specification-mapping test | Asserts the transcription rule below over REQ-SIM-009 edge case 4, every Kubernetes name pinned to the client library's, the Startup-time annotations whatever full access names, the NodePool label, a Node's stated labels beside the lab's own, its taints, and a probe Pod tolerating them | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-012, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Specification placement tests | Assert a deployed Pod carrying its settings, REQ-SIM-009 edge cases 1 and 2, Pods no Node has room for staying Pending, REQ-SIM-019 edge case 5 with its Pods off the Nodes whose taints they do not tolerate, and no Pod Ready before its Startup time | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-019 |
| `scenario.Pod`, `scenario.PodPhase` | One observed replica, placed or not, with its reported phase and readiness | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Workload's Pods sit at one observation against the Nodes that could hold them, with the Skew, the Pods not yet Ready, the Pods per value of one label, and the Pods on one NodePool | `internal/scenario/placement.go`, `internal/scenario/placement_test.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Placement.PodsRunningApartFrom` | The Pods one observation found Running off the Nodes named: what a Workload kept through a Failure | `internal/scenario/placement.go` | REQ-SIM-010 |
| `scenario.NodeFailure`, `scenario.ZoneFailure` | The Nodes, or Zones, one Failure stops, resolved at construction to the first its Topology derives, compared through each Node's equality method; a count above them is refused | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-008, REQ-SIM-017 |
| `scenario.LabelFailure` | The Nodes one Failure by label stops: those of the observed Placement carrying the one label value named, in the Topology's order, refused as § Contracts states | `internal/scenario/failure.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-018 |
| `scenario.StatedWait` | The one carrier of the time a Wait is held to, stated or not, judging a measured wait against itself | `internal/scenario/wait.go`, `internal/scenario/wait_test.go` | REQ-SIM-006 |
| Provisioned-address client | The one construction path to the control plane: the provisioned loopback address and no credential | `internal/cluster/client.go`, `internal/cluster/client_test.go` | REQ-SIM-001 |
| `cluster.DetectEngine` | Resolves the engine socket once, returning the endpoint and version that answered, or errors naming every path tried | `internal/cluster/runtime.go`, `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| `cluster.Engine`, `cluster.ClusterContainer` | The restricted engine surface over one unix socket, and the handle to the one container this run created. Its test asserts the § Threat Model surface rule from the package's declarations | `internal/cluster/runtime.go`, `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors: a leading `v`, digits and dots alone | `internal/cluster/version.go` | REQ-SIM-001 |
| `cluster.Provision` | Starts the Simulated Cluster, waits for its API server, makes the served Stages the lab's own set, and returns the handle | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-012 |
| `cluster.ObtainVersion` | Makes the engine hold the cluster image of a version, pulling it only when absent, or errors naming that version | `internal/cluster/simulated_cluster.go`, `internal/cluster/runtime.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives, removing itself on close. Its parallel tests assert no cluster left behind and the even Placement of REQ-SIM-001 | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready under its labels and taints, returning once the scheduler has placed and released a probe Pod on each, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `cluster.SimulatedCluster.AwaitSettled` | Polls a Workload's Placement until two agreeing observations account for every declared Pod, unplaced ones included and starting ones not, or the waiting time expires | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012 |
| `cluster.SimulatedCluster.AwaitPodsReady`, `AwaitNodesReady` | The same loop, until every declared Pod, or Node, reports Ready, erroring with those that did not | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-006 |
| `cluster.SimulatedCluster.FailNode` | Stops one Node responding: off the KWOK controller, its Ready condition Unknown. Its tests assert the Node staying NotReady and its Pods moving | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008 |
| `scenario.Timeline`, `scenario.Entry`, `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | The ordered, immutable record one Scenario's run left, and one change in it, rendering its own report line | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.TimelineSummary`, `scenario.Timeline.Summarise` | The report a run prints of one Timeline, a pure function of its entries | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `cluster.SimulatedCluster.Record`, `StopRecording` | Open the watch streams under one goroutine delivering each entry to a sink, re-opening a closed stream once; stopping drains them and returns the Timeline, idempotently | `internal/cluster/timeline.go` | REQ-SIM-005 |
| Entry mapper | The one place a watched Kubernetes object becomes a `scenario.Entry` | `internal/cluster/resources.go` | REQ-SIM-005 |
| Timeline-recording tests | Assert every Node and Pod recorded in receipt order with their reasons, and the re-open rule. Its hand-written `watch.Interface` is the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.File` | Where a run's Timelines collect, and one Scenario's filed Timeline named for its cluster in JSON Lines, keeping its first write error | `internal/timeline/timeline.go`, `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.Expect`, `expect.Equal`, `expect.HaveLen` | The assertion entry point over the interface `*testing.T` and a Then share, and its matchers | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against, recording every Step and holding its newest Workload | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-019 |
| `lab.Zones`, `lab.Pods`, `lab.WorkloadName` | The named arguments of a Step's mandatory values: defined types, so a swap does not compile. `lab.Pods` counts a Workload's Pods and its Required Capacity | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-013, REQ-SIM-016 |
| `pod.Gi`, `node.Gi` | The gibibyte a memory amount is written in, beside a cpu amount in whole cores, as a manifest writes both | `internal/lab/pod/settings.go`, `internal/lab/node/node.go` | REQ-SIM-011, REQ-SIM-016 |
| `lab.DeclaredTopology`, `node.Pool`, `node.DeclaredPool`, `node.DeclaredPool.Labels`, `node.DeclaredPool.Taints`, `node.Labels`, `node.Taint`, `node.PoolName`, `node.PerZone` | The Topology one Given line declared, and one NodePool on it with its stated labels and taints, carrying the domain value or its first refusal, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/lab/scenario.go`, `internal/lab/node/node.go`, `internal/lab/node/node_test.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-019 |
| `node.Capacity`, `node.Named`, `node.CapacityName` | A NodePool's capacity as a keyed literal under Kubernetes' resource names, stated or named. The package defines no name of its own | `internal/lab/node/node.go` | REQ-SIM-011, REQ-SIM-015, REQ-SIM-016 |
| `pod.Specification`, `pod.Predefined`, `pod.SpecificationName` | The specification a Workload Step declares, and the definer a Scenario writes one through, carrying the domain value or its first refusal. The surface defines no specification of its own, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/lab/pod/specification.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016 |
| `pod.Specification.Customize`, `pod.Customization` | Zero or more Customizations in the order written, a closed set: one setting, one group of settings, one Startup time, or one full access | `internal/lab/pod/specification.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-016 |
| `pod.FullAccess`, `cluster.PodSpecAccess` | A Scenario's own change to the Kubernetes Pod specification, run after the settings are transcribed | `internal/lab/pod/specification.go`, `internal/cluster/resources.go` | REQ-SIM-009, REQ-SIM-016 |
| `pod.StartupTime`, `pod.StartupSeconds`, `pod.StartupSecondsBetween`, `pod.StartupMillis`, `pod.StartupMillisBetween` | The Startup time a Scenario may name; each form builds one `pod.StartupTime`, itself a Customization carrying the domain value or its refusal | `internal/lab/pod/specification.go` | REQ-SIM-012, REQ-SIM-015, REQ-SIM-016 |
| `pod.TopologySpreadConstraint`, `pod.Toleration`, `pod.Request` | The three settings a Scenario may write, each a keyed literal under Kubernetes' type and field names. A field left out records the specification's refusal, naming the setting and the field, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) | `internal/lab/pod/settings.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-016, REQ-SIM-019 |
| `pod.Settings`, `pod.Setting` | Groups settings under the name a Scenario-place function gives, applied in the order written. `pod.Setting` is sealed, so a group holds settings alone, never a Startup time, a full access or another group, per the Named-setting ADR | `internal/lab/pod/settings.go` | REQ-SIM-009, REQ-SIM-016 |
| `pod.DoNotSchedule`, `pod.Honor`, `pod.Exists`, `pod.Equal`, `pod.NoExecute`, `pod.NoSchedule` | The setting values a Scenario may name, in `internal/scenario`'s spelling | `internal/lab/pod/settings.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-016, REQ-SIM-019 |
| `node.Zone`, `node.Hostname`, `node.NotReady`, `node.Unreachable`, `node.UnresponsiveWait`, `node.NoSchedule` | The labels a spread counts over, the taints of a Node that stopped responding, the one effect a NodePool's taint takes, and the lab's waiting time in whole seconds, in `internal/scenario`'s spelling | `internal/lab/node/node.go` | REQ-SIM-008, REQ-SIM-009, REQ-SIM-016, REQ-SIM-019 |
| Setting-name tests | Assert each setting's field names against the client library's own type, every `pod` and `node` value equal to the domain's, and the two `Gi` one amount | `internal/lab/pod/settings_test.go`, `internal/lab/node/node_test.go` | REQ-SIM-016, REQ-SIM-019 |
| `lab.When`, `lab.Nodes`, `lab.When.NodesLabelledFail` | The When Step: a Failure of that many Nodes, or Zones, or of every Node carrying one stated label value, the Nodes stopped named in its Step | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-018 |
| `lab.Wait`, `lab.TopologyWait`, `lab.WorkloadWait` | The Waits `WaitFor()` offers on the Scenario and on a Given, each plain or with a stated time | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-011 |
| `lab.Then` judgements | Judge the Settled Placement: no Pod on a failed Node or Zone, the Pods left Pending, the Required Capacity at both moments, `PodsPerValueOf` a stated label, and `PodsOnNodePool` | `internal/lab/scenario.go` | REQ-SIM-008, REQ-SIM-010, REQ-SIM-011, REQ-SIM-017, REQ-SIM-019 |
| `lab.DeclaredWorkload` | The Workload one Given line declared, and the further call naming its Required Capacity | `internal/lab/scenario.go` | REQ-SIM-010 |
| `lab.Step`, `lab.Finding` | One line of a Scenario as it ran, a Wait's with its timing per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) and a Workload's Given with its settings; and one Then the Placement does not meet | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-016 |
| `lab.Outcome` | What one Scenario reported, its Timeline's path or filing error included | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006 |
| `lab.Runner`, `lab.NewRunner` | Runs one Scenario on its own Simulated Cluster, under the Scenario name its function's name gives, recording its Timeline. Over a list it obtains the version once before any Scenario starts, runs each in its own goroutine recovering a panic into that Scenario's error, and joins them before returning results in declared order, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). It derives every Scenario name before provisioning, refusing as § Contracts states | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-014, REQ-SIM-020 |
| Step-surface tests | Assert the Step, Failure and Required Capacity rules without a cluster, over REQ-SIM-008 edge case 2, REQ-SIM-010 edge cases 1 to 3, REQ-SIM-016 edge cases 1 to 3, REQ-SIM-017 edge cases 4 to 6, REQ-SIM-018 edge cases 1 to 4 and REQ-SIM-019 edge cases 4, 6 and 7, a Scenario's next Workload, and the labels and taints a Topology's Given line reports | `internal/lab/scenario_test.go`, `internal/scenario/workload_test.go`, `internal/scenario/placement_test.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-010, REQ-SIM-012, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019 |
| Cluster-backed Step-surface tests | Assert the Step surface on a real Simulated Cluster: NodePools with their capacity and label, every Wait form, a refused setting ending the Scenario at its Wait, a Failure by label stopping every Node carrying its value in one Step, and a later Workload deployed beside the earlier | `internal/lab/scenario_deployment_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-018, REQ-SIM-019 |
| Scenario-place tests | Assert off the Scenario sources the § Package Structure rules, every Scenario in `scenarios/scenarios.go` after `All` and in its order, every Wait a Step of its own before each Then and after each Failure, and the definition rules above. Each Named Node capacity, predefined Pod specification and Named setting is defined once in the place and nowhere else: one as exactly the two lab-taint tolerations, the Even spread as one spread constraint over its key. The label-key and taint files hold string constants alone. A Scenario of one NodePool names it general, and a Scenario of several names each after what its Nodes hold. Each name sits under the `lab`, `pod` or `node` qualifier that exports it | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-010, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020 |
| Workload names, Named Node capacities, predefined Pod specifications, Named settings, Node label keys, taints, `scenarios.All` | The lab's Workload names, Named Node capacities, predefined Pod specifications, Named settings, Node label keys and taint keys and values, and the list of Scenarios a run iterates | `scenarios/workloads.go`, `scenarios/nodes.go`, `scenarios/pods.go`, `scenarios/labels.go`, `scenarios/taints.go`, `scenarios/scenarios.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020 |
| The lab's Scenarios | Each states above its Steps what it is for, naming neither function nor Step, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) | `scenarios/scenarios.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-010, REQ-SIM-011, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020 |
| Scenarios README | What a Scenario is, its vocabulary, and every Scenario once under its Scenario name with its intent | `scenarios/README.md` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020 |
| `lab` command | Parses its flags and detects the engine once, prints what it resolved and set, runs `scenarios.All` and prints each Outcome in declared order. A Finding, a Step left undone, or an unwritten Timeline exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016 |
| Command tests | Assert every shipped Scenario running clean within its stated times, the settings, Timeline and version reports, and the Timeline files, over one shared run of every Scenario | `cmd/lab/main_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-010, REQ-SIM-011, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019 |

A Workload is realized as a Deployment whose Pod template transcribes its specification, one spread constraint and one toleration per setting and no minimum domain count, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). The container, the Workload label and the selector are the mapper's own, per the same ADR. Every specification tolerates the not-ready and unreachable Node-condition taints, NoExecute, for the lab's waiting time, whatever a definition or Customization writes for them (REQ-SIM-008), per [the Failure ADR](adr/2026-10-05-node-failure-withholds-the-nodes-status.md). KWOK's controller maintains marked Nodes alone, and the control plane keeps Kubernetes' heartbeat timings, tainting a Zone's unresponsive Nodes in one monitor pass. A Failure takes its Node out of the managed Nodes and the Ready Nodes a Placement reads against, and the control plane then taints it unreachable and moves its Pods, per the same ADR. Provisioning makes the served Stages the lab's own set before the first Node, so a placed Pod reads Pending until its Startup time passes, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md). A Pod being deleted is in no Placement. Engine API responses decode into unexported types inside `internal/cluster`. A Kubernetes version with no published cluster image fails at the pull, naming the version and image reference.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then five candidate paths | by a reviewed change to the constant | run-once, before every engine call | Probed in order within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent version per run; its messages unrecorded, treated as bounded | with the image's layers | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes and probe Pods per provisioning (`ApplyTopology`) | The a-workload-placed-for-resilience Scenario's 9 Nodes, one probe Pod each; above that unrecorded, treated as bounded | with the Zones and NodePools declared, capped at `maxZones` Zones | run-private, written once, probes removed within provisioning | One create and one status update per Node, then one probe each and one label-selected list per poll; O(nodes) per poll, within the start timeout |
| NodePools of one Topology (`scenario.Topology`) | REQ-SIM-013's 2; above that unrecorded, treated as bounded | with the NodePools a Topology line declares | run-private, built once per Given | One slice in declaration order, names checked against a set; O(pools) to build, O(nodes) to expand |
| Labels a NodePool states (`scenario.NodeLabels`) | the new Scenario's one per NodePool; above that unrecorded, treated as bounded | with the labels a NodePool line states | run-private, built once per Given, read by the mapper, the report and a Then | Simplest readable form: pairs held in key order, keys checked against a set; O(labels log labels) to build |
| Pods of one Workload per observation (`Placement`) | The a-workload-placed-for-resilience Scenario's 9; above that unrecorded, treated as bounded | with the Pods declared | run-private, read-mostly | One label-selected list of the newest Workload, counted into maps by Node, by Zone, by NodePool and by one label's value; O(pods + nodes) per observation |
| Taints a NodePool states (`scenario.NodeTaints`) and Workloads deployed per Scenario | the Scenario general-pods-beside-tainted-gpu-nodes: one taint on one NodePool, and two Workloads; above that unrecorded, treated as bounded | with the taints a NodePool line states and the Workload Givens written | run-private, built once per Given | Taints held in key order, keys checked against a set; earlier Workloads stay deployed and are never observed again; O(taints log taints) to build |
| Settings of one Pod template (the mapper) and its Given's report | Resilient placement's 4, Default placement's 2 | one per field named and per full access | run-private, written once, read once by the report | Transcribed and reported in definition order, each full access called in its place; O(settings + accesses) |
| Named Node capacities, predefined specifications, Named settings and one line's Customizations (`scenarios/`, `Customize`) | Standard alone; two specifications; two Named settings; a line's Customizations unrecorded, treated as bounded | by a reviewed change; with the Customizations written | built once per NodePool or Workload naming one | One function call per name, returned by copy; each setting matched against the fields held |
| Stages applied per provisioning (`Provision`) | the image's own set at the pinned KWOK release, under ten | by a reviewed change to the set or the release | run-private, written once before the first Node | One list, then one create, replace or delete per Stage; O(stages) |
| Wait polling (`AwaitSettled`, `AwaitPodsReady`, `AwaitNodesReady`) | one observation per poll interval, one wait per Wait, the Node Failure Scenario's 3 | with the Waits written and the Nodes declared | run-private | Two observations compared by value, or one list matched against the declared names; O(pods + nodes) per poll |
| Pods counted at the moment of a Failure (the When) | the Settled Placement's 9, against 1 or 3 Nodes stopped | with the Pods declared and the Nodes stopped | run-private, derived once per When | One pass against a set of the stopped Nodes; O(pods + nodes stopped) |
| Failure injected per When (`FailNode`) | one Node, a Zone's 3, or a label value's 3, one per Zone | with the Nodes per Zone declared | run-private, written once | One label and one status write per Node, retried on a conflict; O(nodes failed) |
| Packages and sources read by the module-rule and Scenario-place tests | a few thousand packages, unrecorded and treated as bounded, and tens of the module's own files | with the dependency tree and the lab's own source | test-only, read once per test | `go list -deps -json` decoded one package at a time, each file parsed once; O(packages + files + nodes) time |
| Scenario names derived per run (`lab.Runner`) | the Scenarios `All` names | with the Scenarios shipped | run-once, before any cluster | Simplest readable form: one derivation per function, a repeated name found through a set; O(scenarios × name length) |
| Steps recorded per Scenario (`lab.Scenario`) | The a-workload-placed-for-resilience Scenario's seven; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step, read once by the report | One slice in Step order; O(steps) |
| Timeline entries per Scenario (`Record`, `Summarise`) | The a-workload-placed-for-resilience Scenario's 150 or so; above that unrecorded, treated as bounded | with the Nodes and Pods declared and the Scenario's length | run-private, appended by the recording goroutine, read once by the summary | Appended in receipt order and encoded to the open file as each arrives, O(1) per entry; the summary one pass grouped by Workload and Pod, O(entries × workloads) |
| Watch streams open per Scenario (`Record`) | five, one per kind recorded | by a reviewed change to the kinds | run-private, open until the record is stopped | One `Watch` per kind merged by one `select`; a closed stream re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, read by no run | One file, named for its Simulated Cluster |
| Simulated Clusters at once (`lab.Runner`, the gate) | a run: one per Scenario. The gate: per cluster-backed package, the tests its `-parallel` flag admits times each test's clusters. The engine's ceiling unrecorded, treated as bounded | with the Scenarios shipped and the tests needing a cluster | run-private, one container and loopback port per cluster | One goroutine per Scenario, joined before the report. A cluster not started within the start timeout ends its own Scenario or test |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1). A version new to the machine also pays one image pull.
- **Limits:** one Simulated Cluster per Scenario, all at once, so a run lasts about as long as its slowest Scenario, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). The gate's timeout stands above a cluster-backed package's parallel batches (§ Contracts). Every wait is bounded by a timeout of § Constants. One Timeline is held in memory and one file open per running Scenario. The API server's own limits are unrecorded, treated as bounded.

## Dependency Policy

Minimize external dependencies.

### Approved Sources

| Source | Examples | Rationale |
|--------|----------|-----------|
| Go standard library | `net/http`, `encoding/json`, `log/slog`, `testing` | The first choice for every need, including the engine's HTTP API over a unix socket |
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
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a random suffix drawn per provisioning, each port is the engine's choice, and a run removes only the container its own handle names, per the same ADR |
| A Timeline is written outside its folder | A Scenario name interpolated into a path | A file name is the lab's constant prefix, a Scenario name of lowercase ASCII letters, digits and hyphens from a compiled-in function's name, and a random suffix, joined to the folder as one path element; `internal/timeline` refuses a separator or a parent reference in it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) and [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md) |
| A filed Timeline exposes what a run did to another user | The file outlives the run, in the working directory | A created Timeline folder and file are readable and writable by the invoking user alone, and a Timeline holds the simulated objects of a cluster carrying no credential and no real Workload, per the same ADR |

## Implementation Order

| ID | Name | Depends On |
|----|------|------------|
| REQ-SIM-004 | The gate and the lint rules | — |
| REQ-SIM-001 | Topology, Workload and the Settled Placement report | REQ-SIM-004 |
| REQ-SIM-002 | One invocation runs every Scenario | REQ-SIM-001 |
| REQ-SIM-003 | One Scenario runs on its own | REQ-SIM-002 |
| REQ-SIM-006 | Given, When, Wait and Then Steps | REQ-SIM-001 |
| REQ-SIM-005 | A Timeline printed and filed | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-008 | Node and Zone Failures | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-009 | Predefined Pod specifications | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-010 | Required Capacity | REQ-SIM-008, REQ-SIM-009 |
| REQ-SIM-011 | Node capacity, Requests and Pending Pods | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-010 |
| REQ-SIM-012 | Startup time | REQ-SIM-001, REQ-SIM-006, REQ-SIM-009 |
| REQ-SIM-013 | NodePools | REQ-SIM-001, REQ-SIM-011 |
| REQ-SIM-014 | Scenarios run at once, reported in declared order | REQ-SIM-002, REQ-SIM-003 |
| REQ-SIM-015 | Every Placement-deciding value on a Scenario's lines | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-013 |
| REQ-SIM-016 | Scenarios read by subject, every setting reported | REQ-SIM-008, REQ-SIM-009, REQ-SIM-015 |
| REQ-SIM-017 | Node labels a NodePool states, counted per value | REQ-SIM-013, REQ-SIM-016 |
| REQ-SIM-018 | A Failure of every Node carrying one label value | REQ-SIM-008, REQ-SIM-010, REQ-SIM-017 |
| REQ-SIM-019 | A general Workload stays off a NodePool its Pods do not tolerate | REQ-SIM-013, REQ-SIM-016, REQ-SIM-017 |
| REQ-SIM-020 | Every Scenario in one source, named by its condition | REQ-SIM-006 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket, from `DOCKER_HOST` or a candidate | engine resolved |
| engine resolved | any Scenario's function names no condition, or two derive one Scenario name | run stopped before any cluster, each function named |
| engine resolved | `ObtainVersion` finds the version's image held, or pulls it once | version obtained |
| engine resolved | the version's image is absent and cannot be pulled | run stopped before any Scenario, the version named |
| version obtained | `Provision` starts a Scenario's container and its readiness probe answers | cluster ready |
| version obtained | the engine cannot start a Scenario's container | that Scenario ended, naming why; the others run on |
| cluster ready | the Timeline file opens and `Record` opens the five watch streams | recording |
| cluster ready | the Timeline folder or file cannot be opened | recording, that path and reason kept |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | a Wait has `Deploy` create the declared Deployment | Workload deployed |
| Workload deployed | a Wait finds every declared Pod Ready in two agreeing observations | Placement Settled |
| Workload deployed | a Wait for the Placement alone finds two agreeing observations of every declared Pod | Placement Settled, Pods Pending or not |
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
| declared | a Wait finds every declared Node, or Pod, Ready and the Placement unchanged | Placement observed, that Wait's line naming the time it took |
| declared | a Wait for the Placement alone finds it has stopped changing | Placement observed, Pods Pending or not |
| declared | the lab's own waiting time outlasts a Wait | ended, that Wait's line naming that time, the Step named with what was not Ready |
| declared | a Wait states a time of none, or above the lab's own waiting time | ended, that Wait named with that waiting time |
| declared | a Wait is reached later than the time it states | ended, that Wait's line naming the stated time |
| declared | a Then, or a Failure When, is reached with no Wait before it | ended, that Step named |
| declared | a When asks for more Nodes, or more Zones, than the Topology holds | ended, that When named |
| declared | a When names a label, or value, no NodePool states, or one no observed Node carries | ended, that When named with the label or value |
| Placement observed | a When stops its own Nodes responding | declared, the observation discarded |
| Placement observed | a Then the Placement meets | Placement observed, that Step recorded |
| Placement observed | a Then the Placement does not meet | Placement observed, a Finding recorded |
| Placement observed | a Then reads a Failure and no When ran, judges a minimum the Workload never declared, or counts over a label no NodePool states | ended, that Then named |
| ended | a later Step is reached | ended, nothing carried out or reported |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Pending on that Node, inside its Startup time |
| Pending on that Node | its Startup time passes | Running and Ready |
| Pending | no Node satisfies the specification or the capacity | Pending, reported unplaceable (REQ-SIM-001 edge case 1) |
| Running | its Node stops responding and the waiting time passes | deleted by the control plane, replaced on a Node still responding |
