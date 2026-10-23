<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster for every Scenario at once, whose Steps apply its Topology, create a Deployment, Wait for them, and report the Placement reached on a Placement board under every Wait. It records what the cluster did and each Step as a Timeline, narrates from it above every board, and files it. The Simulated Cluster is a KWOK control plane in a container, so scheduling decisions are the real ones and no Pod image is pulled.

An Interactive run carries out one Scenario's Steps up to its first When, then holds its Simulated Cluster, shown in a Live view the lab serves on loopback (§ Contracts).

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
│   ├── live/           # the Live view: its loopback server, page, script and stylesheet, and the board it pushes
│   ├── modulerules/    # the package-graph walk and call-rule scan the module-rule tests share
│   ├── scenario/       # Topology, Deployment, Placement, Timeline values; no Kubernetes import
│   └── timeline/       # the Timeline folder, its file names and its JSON Lines format
├── timelines/          # every run's filed Timelines, git-ignored
├── .golangci.yml       # lint configuration carrying the module rules
├── Makefile            # the gate command and the targets it runs
├── go.mod              # the module path and the Go version
└── go.sum              # the module graph's committed checksums
```

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). `internal/timeline` imports `internal/scenario` and the standard library alone, and a recording delivers entries to a one-method sink in `internal/cluster`. `internal/live` imports `internal/scenario` and the standard library alone, and only `cmd/lab` imports it, per [ADR: The Live view is served on loopback from one shared observation](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). `internal/modulerules` imports the standard library alone, and no production package imports it, per [the KWOK ADR](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md). Kubernetes types stay inside `internal/cluster`, whose mapper alone names both a Kubernetes and a `scenario` type and declares the full-access function type. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file. The one home-relative path is an engine socket candidate, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md).

`scenarios/scenarios.go` holds `All`, then every Scenario in `All`'s order, and no other source holds one. A Scenario is an exported function over `*lab.Scenario`, named in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). The `scenarios` package imports `internal/lab`, `internal/lab/pod` and `internal/lab/node` alone of the lab's own packages, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Scenario line qualifies each name the lab offers, an inline Customization included, and writes bare each name the Scenarios define. The place holds no alias of a `lab`, `pod` or `node` name (REQ-SIM-026), per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). `lab` imports `pod` and `node`, neither imports `lab` or the other, and the split stops there, per the same ADR. Scenario code imports no package by dot and passes the gate's format and lint checks, per the same ADR. The place declares the lab's Deployment names, its Node label keys as string constants, and one function per Named Node capacity, predefined Pod specification and Named setting, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). Each takes nothing but `EvenSpreadOver`'s topology key and returns one `pod` or `node` value. Each import rule is a `golangci-lint` rule, and every rule is asserted from the module's own sources (§ Contracts).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository, reached by no input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors by default, per the same ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The engine socket paths probed in order when `DOCKER_HOST` resolves nothing, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits |
| The NodePool name rule | `internal/scenario/topology.go` | Kubernetes' label-value syntax a NodePool name must meet, pinned to the client library's, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | The budgets for a socket probe, a pull and provisioning |
| `scenario.WaitingTime` | `internal/scenario/wait.go` | The longest the lab waits for a state of the cluster, and the highest time a Wait may state |
| `pollInterval` | `internal/cluster/simulated_cluster.go` | The gap between one wait's observations |
| `controlPlaneTimeout` | `internal/cluster/client.go` | The deadline on every control-plane request, a watch's opening included; a watch's stream carries none, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The keys the lab writes on Pods and Nodes, and the simulator's selection label, a literal pinned by test to the domain's lab-own keys
| The lab's own Node label keys and the label syntax | `internal/scenario/node_labels.go` | The keys a NodePool may not state, the NodePool label among them, and Kubernetes' label syntax, pinned to the mapper and the client library, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The topology labels, taint keys and setting values a specification names |
| The lab's Stage set | `internal/cluster/stages.go` | The Stages the KWOK controller applies, the pod-ready one reading a Pod's Startup time, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on an unresponsive Node, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets, and the stock value it replaces, per the same ADR |
| The Live view's refresh interval, newcomer fade, open-stream cap and server timeouts | `internal/live/` | The observer's interval, the newcomer fade in observations, the open-stream cap, and the connection, event-write and shutdown bounds, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md)
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The Timeline folder under the working directory, and the suffix marking a filed Timeline, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | A Timeline folder and file are readable and writable by the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary. A constructor errors where a caller can get the value wrong. `SimulatedCluster` is the aggregate root of a run: a Topology, a Deployment and an observation are reached through it alone. A Topology derives Node names from positions counted from one, through its NodePools in declaration order (REQ-SIM-013). Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. No package holds mutable process-wide state. A Scenario name is the condition half of its function's name, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md), and a missing or repeated name stops the run before any cluster (REQ-SIM-020). Every Scenario runs at once on a Simulated Cluster of its own, removed when it ends, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). A Scenario's failure, a panic included, ends that Scenario alone (REQ-SIM-014). One engine detection and one flag parse serve the whole run, per the engine ADR and [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given constructs its immutable value and applies nothing. A Step's mandatory value is an argument and an optional setting a further call, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A Then the Placement does not meet records a Finding, and the later Steps still run.

A Wait is written `WaitFor()` on the Scenario, or on the Given before it, per [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](adr/2026-10-07-a-scenario-writes-its-own-wait.md). A Wait applies what the Givens before it declared and observes the Settled Placement beneath its condition (REQ-SIM-011). Every Then and Failure reads the Placement the last Wait reached. A Wait is held to the lab's own waiting time, stated or not (§ State Machine; REQ-SIM-006, REQ-SIM-008).

A Deployment's Given names a Customization for its Pods' Request, then its Pod specification with zero or more Customizations, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Customizing returns a copy, applying the line's Customization last, the last setting per field winning. Full access reaches the Pod specification alone, never the Deployment's label or selector, and the domain core holds none. The mapper re-applies the specification's own tolerations after the last Customization. It writes the Startup time onto Pod template annotations full access does not reach, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) (REQ-SIM-012). A value the control plane refuses ends the Scenario at the Wait deploying it. A Given line's settings print in the order the domain holds them, a full access as one line and a refused specification as none (REQ-SIM-016).

The lab defines no Pod specification, Startup time, Node capacity or setting group of its own. The Scenarios define each through `pod.Predefined`, `pod.Settings` and `node.Capacity` (REQ-SIM-015). A definition omitting its Startup time does not compile. A definition names the Named setting `LeaveUnresponsiveNodes` and writes neither lab-taint toleration itself, per the specification ADR. It holds no Request (REQ-SIM-024). Every field of a setting the lab offers is required, its zero value that field left out, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A field left out, or stated against a toleration's operator and effect, is refused at the Given (REQ-SIM-016, REQ-SIM-019).

A later Deployment Given gives the Scenario its next Deployment, which the next Wait creates beside the earlier ones, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md). The earlier stay deployed. Every later Wait, When and Then judges the newest alone, `EnoughPodsStayedReady` aside, and a board shows every Deployment (REQ-SIM-019, REQ-SIM-021).

A Topology names its Zone count, then the NodePools every Zone holds, each with its name, Node count per Zone and capacity, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). Every Node carries its NodePool's name under the lab's NodePool label (REQ-SIM-013). A Node capacity is stated in every dimension or named. A Node reports it as both its capacity and its allocatable room, and the scheduler alone decides which Pods fit (REQ-SIM-011). Every quantity crossing into Kubernetes comes from the client library's own constructor, never a parsed string.

A NodePool states Node labels as `node.Labels` and taints as `node.Taint` keyed literals, refused at the Topology Given as their `scenario` rows state (REQ-SIM-017, REQ-SIM-019). The mapper writes the lab's own labels after the stated ones, and the taints as the Node's taints. Each probe Pod tolerates its own Node's taints, so provisioning still ends on a tainted Node. An observation reads every label a Node carries. A Then counting Pods per label value or on one NodePool reads the observed Node's labels. A When by label stops every observed Node carrying it, in the Topology's order (REQ-SIM-017, REQ-SIM-018).

A Deployment's further call declares its Failure tolerance, `lab.Zones(n)` or `lab.Nodes(n)`, setting its Floor, per [ADR: A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline](adr/2026-10-06-a-deployment-declares-the-zones-or-nodes-it-survives-losing.md) (REQ-SIM-027). `EnoughPodsStayedReady` replays the Scenario's Timeline copy from the first When's note over every declaring Deployment. The preceding Wait held its Placement a poll interval, so the copy holds it. A Pod counts while Running, and is lost from the note of the When stopping its Node.

Every Scenario's run records a Timeline under one owning goroutine, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). Receipt order is the Timeline's order: the API server's within one stream, the lab's own across them. A Scenario notes each Step through the same goroutine. A Given or a Then is noted once its line is complete, a When before its first Node stops, and a Wait as it ends, after its board (REQ-SIM-021).

A Wait narrative holds the moments after the previous Wait's note up to its own, the k-th Wait note being the k-th Wait Step's. A Pod belongs to the Deployment whose name begins its own, at the longest match. A Pod no Deployment claims is the lab's own setup and in no narrative. An eviction is read from a Pod's first entry marked for deletion (REQ-SIM-021).

Every Wait, reached or not, ends with one observation of the whole Simulated Cluster, held to the lab's waiting time, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) (REQ-SIM-021). A Wait before the Scenario's first change to its cluster observes nothing, and a failed observation ends the Scenario at that Wait. `scenario.PlacementBoard` reads a Pod's Deployment from its Deployment label, and marks it new where the previous board did not hold it on that Node. The reports, written from the `lab.Outcome` values once every Scenario has ended, take the layout REQ-SIM-021 states.

An Interactive run is `--interactive` with one Scenario name, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). Any other argument shape stops the lab before the engine is detected, naming every Scenario name in `All`'s order (REQ-SIM-022). It detects the engine and obtains the version as a run does, then listens and names the Live view's address before it provisions (REQ-SIM-022 edge case 8).

It carries out its Scenario through the runner a run uses, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md), up to its first When; § State Machine gives its lifecycle (REQ-SIM-022). Stopping is an interrupt or a termination signal, cancelling the run's context. The cluster is then removed under a context detached from the cancelled one, and the server shuts down within its bound (REQ-SIM-022 edge case 7).

From provisioning until removal one observer goroutine calls `ObserveEveryDeployment` at the refresh interval, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) (REQ-SIM-022 edge case 6). It builds a `scenario.PlacementBoard` against the previous board and the declared Topology, which the Scenario publishes to it under synchronization. Before a Topology is declared the Live view shows none, and a failed observation leaves it naming its cause until the next. A run starts no observer.

`ObserveEveryDeployment` reports each Node's readiness beside the Node, never on `scenario.Node`, whose equality a Failure reads. It reports each Pod's Requests as its observed specification carries them, full access included. `scenario.PlacementBoard` gives each Pod its Request, cpu 0 and memory 0 where it declares none. It gives each Node's Pods, cpu and memory as capacity, requests and left: its Node capacity, what its placed Pods take by count and Request, and the difference.

The Live view serves reads alone: its page, script and stylesheet, and the board as a stream of Server-Sent Events, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). Each observation renders one well-formed `html/template` fragment, sent to every open stream when it differs from the last. A stream opens on the latest fragment, and a browser falling behind skips to it (REQ-SIM-022 edge case 5). The page is rendered once at listening from an embedded `html/template`. Its header names k8s-resilience-lab, a hyphen, then the held Scenario's name; its title is fixed (REQ-SIM-022). Another method, path, Host or Origin is refused, as is a stream beyond the open-stream cap. Each event write is bounded, closing the server ends every stream first, and the Live view changes nothing in the cluster.

The fragment nests a box per Zone, holding a box per Node, holding its Pods, with the unplaced Pods apart; what each box shows follows REQ-SIM-022. A Node's room is a table of capacity, requests and left, and a Pod's box gives its Request under the word requests. Each box carries a stable id derived from its name, and Ready is stated by class and by word. A Node's and a Pod's box give the name on the first line with that word at its right, a Pod's Request beneath it (REQ-SIM-022). A Node's box stacks its Pods one per row, each Pod's box spanning the Node's box, while the unplaced Pods wrap side by side (REQ-SIM-022). A newcomer's mark steps down over the newcomer fade, counted in observations. The stylesheet follows the system's light or dark setting, and nothing served names another host (REQ-SIM-022).

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command, failing on the first check that fails. Its tests run under the race detector, refusing a missing C compiler with install guidance, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md). Its per-package timeout stands above a cluster-backed package's run, and its `-parallel` flag bounds a package's parallel tests (§ Scale and Load) | `Makefile` | REQ-SIM-004, REQ-SIM-011, REQ-SIM-014 |
| `make deps-check` | Fails, naming the offender, when a prohibited dependency class (§ Dependency Policy) enters the module graph | `Makefile` | REQ-SIM-004 |
| depguard and `forbidigo` rules | Deny the imports and the in-cluster configuration call § Package Structure rules out | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006, REQ-SIM-022 |
| Import-graph and call-rule tests | Assert every lint rule again from the module's own package graph and files, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/cluster/callrule_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-001, REQ-SIM-004, REQ-SIM-006, REQ-SIM-022 |
| `modulerules` helpers | The package-graph, call and declaration scans the module-rule tests share | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.NodeCapacity`, `scenario.CPU`, `scenario.Memory` | The capacity one Node holds, refused at none in any dimension, so a Deployment can exceed its Topology. Its quantities are integers Kubernetes' own quantity carries exactly | `internal/scenario/topology.go`, `internal/scenario/quantity.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.NamedNodeCapacity`, `scenario.NewNamedNodeCapacity` | A Node capacity under a name a Scenario gives it, refused at an empty name; the package defines none of its own, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-011, REQ-SIM-015 |
| `scenario.Node` | One simulated machine: its Zone, its NodePool's name, its capacity, its stated labels and taints, compared through its equality method | `internal/scenario/topology.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.NodeTaints`, `scenario.Taint` | The NoSchedule taints a NodePool states, immutable and equal by value, refused at an effect other than NoSchedule, a field left out, a key or value outside Kubernetes' syntax, a repeated key, and a Node-condition-prefix key | `internal/scenario/node_taints.go`, `internal/scenario/node_taints_test.go` | REQ-SIM-019 |
| `scenario.NodeLabels` | The labels a NodePool states, immutable and equal by value; a lab-own key, a key or value outside the label syntax, and a repeated key are refused | `internal/scenario/node_labels.go`, `internal/scenario/node_labels_test.go` | REQ-SIM-017 |
| `scenario.NodePool` | A name, a Node count per Zone, a capacity and any stated labels and taints; no Node, an empty name, a name outside the label-value syntax, and the zero capacity are refused | `internal/scenario/topology.go` | REQ-SIM-013, REQ-SIM-015, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Topology` | The Zones and their NodePools in declaration order, expanding to Nodes under derived names and their NodePool's labels, and naming the values its NodePools state for a key; no NodePool and a repeated name are refused. Its test asserts the naming rule and the refusals' edge cases | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Label rule tests | Assert the domain's label and taint key and value rules agreeing with the client library's validation over the same names, and the domain's lab-own keys being those the mapper writes, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/cluster/resources_test.go` | REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Deployment`, `scenario.FailureTolerance` | A declared replica count, its Pod specification, and any Failure tolerance with its Floor; a count below one, and a tolerance below zero or reaching its Topology's Zones or Nodes, are refused | `internal/scenario/deployment.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-019, REQ-SIM-025, REQ-SIM-027 |
| `scenario.PodSpecification` | The settings one Deployment's Pods are deployed with under their intent, and their Startup time: immutable and equal by value. An empty name is refused, and the lab's two tolerations hold at its waiting time | `internal/scenario/pod_specification.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015 |
| `scenario.StartupTime` | The time a placed Pod takes to become Ready, one time or a range, built from whole milliseconds; a negative time or an inverted range is refused | `internal/scenario/pod_specification.go` | REQ-SIM-012 |
| `scenario.TopologySpread`, `scenario.Toleration`, `scenario.Request` | The settings a specification may hold, each field as Kubernetes names it: one spread per topology key, one toleration per taint and one Request, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-011, REQ-SIM-019 |
| Specification-definition tests | Assert the definition, Customization and grouping rules on specifications and groups the tests define, over the edge cases of the requirements they implement | `internal/lab/pod/specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016 |
| Specification-mapping test | Asserts the transcription rule below, every Kubernetes name pinned to the client library's, the Startup-time annotations whatever full access names, the NodePool label, a Node's stated labels and taints, and a probe Pod tolerating them | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-012, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Specification placement tests | Assert a deployed Pod carrying its settings, Pods no Node has room for staying Pending, Pods off the Nodes whose taints they do not tolerate, and no Pod Ready before its Startup time | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-019 |
| `scenario.Pod`, `scenario.PodPhase` | One observed Pod, placed or not, with its reported phase, readiness, Deployment name and Requests | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-021, REQ-SIM-022 |
| `scenario.PlacementBoard` | One observation's view of the whole Simulated Cluster: every Topology Node by Zone with its NodePool, labels, readiness and Pods, newcomers since the previous board marked and unplaced Pods beneath. It renders its own lines and their width. It gives its Zones, Nodes and Pods as immutable values, each Node with its capacity, requests and left, and each Pod with its Request | `internal/scenario/placement_board.go`, `internal/scenario/placement_board_test.go` | REQ-SIM-021, REQ-SIM-022 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Deployment's Pods sit at one observation against the Nodes that could hold them, with the Skew, the Pods not yet Ready, the Pods per value of one label, and the Pods on one NodePool | `internal/scenario/placement.go`, `internal/scenario/placement_test.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-017, REQ-SIM-019 |
| `scenario.NodeFailure`, `scenario.ZoneFailure` | The Nodes, or Zones, one Failure stops, resolved at construction to the first its Topology derives, compared through each Node's equality method; a count above them is refused | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-008, REQ-SIM-017 |
| `scenario.LabelFailure` | The Nodes one Failure by label stops: those of the observed Placement carrying the one label value named, in the Topology's order, refused at other than one label, a key or value no NodePool states, and a value no observed Node carries | `internal/scenario/failure.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-018 |
| `scenario.StatedWait` | The one carrier of the time a Wait is held to, stated or not, judging a measured wait against itself | `internal/scenario/wait.go`, `internal/scenario/wait_test.go` | REQ-SIM-006 |
| Provisioned-address client | The one construction path to the control plane: the provisioned loopback address, no credential, a request client under `controlPlaneTimeout` and a watch client with no deadline, both `k8s.io/client-go`, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) | `internal/cluster/client.go`, `internal/cluster/client_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-021 |
| `cluster.DetectEngine` | Resolves the engine socket once, returning the endpoint and version that answered, or errors naming every path tried | `internal/cluster/runtime.go`, `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| `cluster.Engine`, `cluster.ClusterContainer` | The restricted engine surface over one unix socket, and the handle to the one container this run created. Its test asserts the § Threat Model surface rule from the package's declarations | `internal/cluster/runtime.go`, `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors: a leading `v`, digits and dots alone | `internal/cluster/version.go` | REQ-SIM-001 |
| `cluster.Provision` | Starts the Simulated Cluster, waits for its API server, makes the served Stages the lab's own set, and returns the handle | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-012 |
| `cluster.ObtainVersion` | Makes the engine hold the cluster image of a version, pulling it only when absent, or errors naming that version | `internal/cluster/simulated_cluster.go`, `internal/cluster/runtime.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives, removing itself on close. Its parallel tests assert no cluster left behind and the even Placement of REQ-SIM-001 | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-014 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready under its labels and taints, returning once the scheduler has placed and released a probe Pod on each, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `cluster.SimulatedCluster.AwaitSettled` | Polls a Deployment's Placement until two agreeing observations account for every declared Pod, unplaced ones included and starting ones not, or the waiting time expires | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012 |
| `cluster.SimulatedCluster.AwaitPodsReady`, `AwaitNodesReady` | The same loop, until every declared Pod, or Node, reports Ready, erroring with those that did not | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-006 |
| `cluster.SimulatedCluster.ObserveEveryDeployment` | One observation of every Topology Node with its readiness, and every Pod carrying the lab's Deployment label with its Requests, so no probe Pod. Its cluster-backed test asserts two Deployments, a NotReady Node, observed Requests and no probe Pod | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-021, REQ-SIM-022 |
| `cluster.SimulatedCluster.FailNode` | Stops one Node responding: off the KWOK controller, its Ready condition Unknown. Its tests assert the Node staying NotReady and its Pods moving | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008 |
| `scenario.Timeline`, `scenario.Entry`, `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | The ordered, immutable record one Scenario's run left, and one change in it, rendering its own report line | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.Timeline`'s Floor reading | Each declaring Deployment's lowest Ready count from the first When's note, when and how long, and its deepest fall below its Floor with the Pods lost; a pure function of the entries | `internal/scenario/floor.go`, `internal/scenario/floor_test.go` | REQ-SIM-027 |
| `scenario.Timeline`'s Wait narratives, `scenario.StepObject` | The moments that changed the Placement before each Wait, one narrative per Wait note in note order, a pure function of the entries; and the kind a Step's note carries | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-021 |
| `cluster.SimulatedCluster.Record`, `StopRecording`, the Step note | Open the watch streams on the watch client under one goroutine delivering each entry to a sink, re-opening a closed stream once. Each open ends at `controlPlaneTimeout`, and each stream with the recording. A Step's note reaches the same goroutine, timed on receipt. Stopping drains them and returns the Timeline, idempotently | `internal/cluster/timeline.go`, `internal/cluster/timeline_test.go` | REQ-SIM-005, REQ-SIM-021 |
| Entry and observation mapper | The one place a watched Kubernetes object becomes a `scenario.Entry`, a Pod marked for deletion read as Terminating, and an observed Pod a `scenario.Pod` requesting what all its containers request, as the scheduler sums it | `internal/cluster/resources.go`, `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-021, REQ-SIM-022 |
| Timeline-recording tests | Assert every Node and Pod recorded in receipt order with their reasons, and the re-open rule. Its hand-written `watch.Interface` is the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005 |
| `timeline.Folder`, `timeline.File` | Where a run's Timelines collect, and one Scenario's filed Timeline named for its cluster in JSON Lines, keeping its first write error | `internal/timeline/timeline.go`, `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.Expect`, `expect.Equal`, `expect.HaveLen` | The assertion entry point over the interface `*testing.T` and a Then share, and its matchers | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against, recording every Step, holding its newest Deployment and every one declaring a Failure tolerance, and copying its Timeline's entries | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-019, REQ-SIM-027 |
| `lab.DeploymentName` | The Deployment Step's named argument: a defined type, so a swap does not compile. Every count, a Deployment's `replicas` among them, is an `int` under the Step naming its subject | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-013, REQ-SIM-016, REQ-SIM-025 |
| `lab.DeclaredTopology`, `lab.DeclaredDeployment`, `lab.FailureTolerance` | The Topology or Deployment one Given line declared, a Deployment's Request Customization taken before its specification, carrying the domain value or its first refusal, and the further call naming its Failure tolerance as `lab.Zones` or `lab.Nodes` | `internal/lab/scenario.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-024, REQ-SIM-025, REQ-SIM-027 |
| The `node` package | A NodePool as a Scenario writes it, with its stated labels and taints, carrying the domain value or its first refusal. Its capacity is a keyed literal under Kubernetes' resource names, memory in `node.Gi`, or a named one. Its label, taint and waiting-time values are in `internal/scenario`'s spelling | `internal/lab/node/node.go`, `internal/lab/node/node_test.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-019 |
| The `pod` package | A Pod specification as a Scenario writes it, defined through `pod.Predefined` and customized in the order written by one setting, one group, one Startup time or one full access. Each carries the domain value or its first refusal. Its three settings are keyed literals under Kubernetes' type and field names, memory in `pod.Gi`. A `pod.Settings` group holds settings alone, and its values are in `internal/scenario`'s spelling, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) and [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) | `internal/lab/pod/specification.go`, `internal/lab/pod/settings.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-019 |
| `pod.FullAccess`, `cluster.PodSpecAccess` | A Scenario's own change to the Kubernetes Pod specification, run after the settings are transcribed | `internal/lab/pod/specification.go`, `internal/cluster/resources.go` | REQ-SIM-009, REQ-SIM-016 |
| Setting-name tests | Assert each setting's field names against the client library's own type, every `pod` and `node` value equal to the domain's, and the two `Gi` one amount | `internal/lab/pod/settings_test.go`, `internal/lab/node/node_test.go` | REQ-SIM-016, REQ-SIM-019 |
| `lab.When`, `lab.When.NodesLabelledFail` | The When Step: a Failure of that many Nodes, or Zones, or of every Node carrying one stated label value, the Nodes stopped named in its Step | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-018, REQ-SIM-025 |
| `lab.Wait`, `lab.TopologyWait`, `lab.DeploymentWait` | The Waits `WaitFor()` offers on the Scenario and on a Given, each plain or with a stated time | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-011 |
| `lab.Then` judgements | Judge the Settled Placement: no Pod on a failed Node or Zone, the Pods left Pending, every declaring Deployment's Floor over the Timeline as `EnoughPodsStayedReady`, `PodsPerValueOf` a stated label, and `PodsOnNodePool` | `internal/lab/scenario.go` | REQ-SIM-008, REQ-SIM-011, REQ-SIM-017, REQ-SIM-019, REQ-SIM-025, REQ-SIM-027 |
| `lab.Step`, `lab.Finding` | One line of a Scenario as it ran, a Wait's with its timing per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md), its Wait narrative, and its Placement board or the cause it has none, and a Deployment's Given with its settings; and one Then the Placement does not meet | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-016, REQ-SIM-021 |
| `lab.Outcome` | What one Scenario reported, its Timeline's path or filing error included, and each Wait Step given its narrative once the record stopped | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-021 |
| `lab.Runner`, `lab.NewRunner` | Runs each Scenario on its own Simulated Cluster under its derived name, recording its Timeline into its file and the Scenario's copy. It obtains the version once, derives every name before provisioning, runs each Scenario in its own goroutine recovering a panic into its error, and returns results in declared order, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-014, REQ-SIM-020, REQ-SIM-022, REQ-SIM-027 |
| Step-surface tests | Assert the Step, Failure and Failure tolerance rules without a cluster, over the edge cases of the requirements they implement, a Scenario's next Deployment, and the labels and taints a Topology's Given line reports | `internal/lab/scenario_test.go`, `internal/scenario/deployment_test.go`, `internal/scenario/placement_test.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-024, REQ-SIM-025, REQ-SIM-027 |
| Cluster-backed Step-surface tests | Assert the Step surface on a real Simulated Cluster: NodePools with their capacity and label, every Wait form, a refused setting ending the Scenario at its Wait, a Failure by label in one Step, and a later Deployment beside the earlier | `internal/lab/scenario_deployment_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-018, REQ-SIM-019 |
| Scenario-place tests | Assert off the Scenario sources the § Package Structure and definition rules, every Scenario after `All` in its order, and every Wait a Step of its own before each Then and after each Failure. Each Named Node capacity, predefined specification and Named setting is defined once, in the place alone: one as exactly the two lab-taint tolerations, two as exactly one Request, the Even spread as one spread constraint over its key. A Scenario of one NodePool names it general, and one of several names each after what its Nodes hold | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-024, REQ-SIM-025, REQ-SIM-026, REQ-SIM-027 |
| The Scenarios' definitions, `scenarios.All` | The lab's Deployment names, Named Node capacities, predefined Pod specifications, Named settings, Node label keys, taint keys and values, and the list of Scenarios a run iterates | `scenarios/deployments.go`, `scenarios/nodes.go`, `scenarios/pods.go`, `scenarios/labels.go`, `scenarios/taints.go`, `scenarios/scenarios.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-024, REQ-SIM-025, REQ-SIM-026 |
| The lab's Scenarios | Each states above its Steps what it is for, naming neither function nor Step, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) | `scenarios/scenarios.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-011, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-026, REQ-SIM-027 |
| Scenarios README | What a Scenario is, its vocabulary, and every Scenario once under its Scenario name with its intent | `scenarios/README.md` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-026 |
| `lab` command | Parses its flags and detects the engine once, prints what it resolved and set, runs `scenarios.All` and prints each Outcome in declared order, a narrative and a board under every Wait and Floor lines under a Then. A Finding, a Step left undone, or an unwritten Timeline exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016, REQ-SIM-021, REQ-SIM-022, REQ-SIM-027 |
| Command tests | Assert every shipped Scenario running clean within its stated times, its settings, Timeline, version, narrative, board and Skew reports, and its Timeline files, over one shared run of every Scenario. An Interactive run's argument refusals are asserted before any engine is detected | `cmd/lab/main_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-021, REQ-SIM-022, REQ-SIM-024, REQ-SIM-027 |
| The Interactive run's held run | Carries out one Scenario up to its first When on its own Simulated Cluster and reports its Outcome. It runs the Live view's observer from provisioning until removal, and holds the cluster until its context ends | `internal/lab/runner.go`, `internal/lab/scenario.go`, `internal/lab/runner_cluster_test.go` | REQ-SIM-022 |
| Scenario lookup by name | The one Scenario whose derived name is the name given, or an error naming every Scenario name in `All`'s order | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-022 |
| The Live view | The loopback server, its page rendered once naming the held Scenario, its embedded script and stylesheet, and the latest board or its cause, rendered once per observation and pushed to every open stream, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) | `internal/live/` | REQ-SIM-022 |
| Live view tests | Assert over a real loopback server's stream and real boards, each fragment decoded with `encoding/xml`: what REQ-SIM-022 states the page shows, a placed Pod's box spanning its Node's as the served stylesheet lays it, and the fading mark. A change arrives without a reload, two browsers alike, and every refusal, the stream cap and no other host named are asserted | `internal/live/` | REQ-SIM-022 |

A Deployment's Pod template transcribes its specification: one spread constraint, with no minimum domain count, and one toleration per setting, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). The container, the Deployment label and the selector are the mapper's own. KWOK's controller maintains marked Nodes alone, and the control plane keeps Kubernetes' heartbeat timings, tainting a Zone's unresponsive Nodes in one pass. A Failure takes its Node out of the managed and the Ready Nodes, and the control plane taints it unreachable and moves its Pods. A Pod being deleted is in no Placement, and engine API responses decode into unexported types inside `internal/cluster`.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then five candidate paths | by a reviewed change to the constant | run-once, before every engine call | Probed in order within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent version per run; its messages unrecorded, treated as bounded | with the image's layers | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes and probe Pods per provisioning (`ApplyTopology`) | the a-deployment-placed-for-resilience Scenario's 9 Nodes, a probe Pod each; above that unrecorded, treated as bounded | with the Zones and NodePools, at most `maxZones` Zones | run-private, written once, probes removed within provisioning | One create and status update per Node, one probe each, one label-selected list per poll; O(nodes) per poll, within the start timeout |
| NodePools, labels and taints of one Topology (`scenario.Topology`, `NodeLabels`, `NodeTaints`) | REQ-SIM-013's 2 NodePools, one label per NodePool, one taint; above that unrecorded, treated as bounded | with the NodePool lines written | run-private, built once per Given, read by the mapper, the report and a Then | NodePools in declaration order, labels and taints in key order, names and keys checked against a set; O(pools) to build, O(nodes) to expand, O(n log n) per label or taint set |
| Pods of one Deployment per observation (`Placement`), and Deployments per Scenario | the a-deployment-placed-for-resilience Scenario's 9 Pods; two Deployments; above that unrecorded, treated as bounded | with the Pods and Deployment Givens declared | run-private, read-mostly | One label-selected list of the newest Deployment, counted into maps by Node, Zone, NodePool and one label's value; O(pods + nodes) per observation |
| Settings of one Pod template (the mapper) and its Given's report | Resilient placement's 4, Default placement's 2 | one per field named and per full access | run-private, written once, read once by the report | Transcribed and reported in definition order, each full access called in its place; O(settings + accesses) |
| Named Node capacities, predefined specifications, Named settings, one line's Customizations (`scenarios/`, `Customize`) | Standard and GPU, four specifications, four Named settings; Customizations unrecorded, treated as bounded | by a reviewed change; with the Customizations written | built once per NodePool or Deployment naming one | One call per name, returned by copy, each setting matched against the fields held |
| Stages applied per provisioning (`Provision`) | the lab's set, following KWOK's published set at the pinned release, under ten | by a reviewed change to the set or the release | run-private, written once before the first Node | One list, then one create, replace or delete per Stage; O(stages) |
| Wait polling (`AwaitSettled`, `AwaitPodsReady`, `AwaitNodesReady`) | one observation per poll interval, one wait per Wait, the Node Failure Scenario's 3 | with the Waits written and the Nodes declared | run-private | Two observations compared by value, or one list matched against the declared names; O(pods + nodes) per poll |
| Timeline entries replayed per `EnoughPodsStayedReady` | as Timeline entries per Scenario, two Deployments | with the Nodes, Pods and Steps | run-private, copied under a lock, replayed once per Then | One pass keeping each Pod's last reading in a map; O(entries × deployments) time, O(entries) space |
| Failure injected per When (`FailNode`) | one Node, a Zone's 3, or a label value's 3, one per Zone | with the Nodes per Zone declared | run-private, written once | One label and one status write per Node, retried on a conflict; O(nodes failed) |
| Packages and sources read by the module-rule and Scenario-place tests | a few thousand packages and tens of the module's files, unrecorded, treated as bounded | with the dependency tree and the lab's source | test-only, read once per test | `go list -deps -json` decoded one package at a time, each file parsed once; O(packages + files) |
| Scenario names derived per run (`lab.Runner`) | the Scenarios `All` names | with the Scenarios shipped | run-once, before any cluster | Simplest readable form: one derivation per function, a repeated name found through a set; O(scenarios × name length) |
| Pods and Nodes per Placement board (`ObserveEveryDeployment`, `scenario.PlacementBoard`) | two Deployments and 9 Nodes at most among the shipped Scenarios; above that unrecorded, treated as bounded | with the Deployments, Pods and Nodes declared | run-private, one per Wait, read once by the report | One Node list and one label-selected Pod list, sorted, each Node's requests summed over its Pods, compared to the previous board through a set; O((pods + nodes) log (pods + nodes)) per Wait |
| Steps recorded per Scenario (`lab.Scenario`) | The a-deployment-placed-for-resilience Scenario's seven; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step, read once by the report | One slice in Step order; O(steps) |
| Timeline entries per Scenario (`Record`, the Wait narratives) | about 150 at most among the shipped Scenarios, plus a note per Step; above that unrecorded, treated as bounded | with the Nodes, Pods and Steps | run-private, appended by the recording goroutine, read once by the narratives | Encoded to the open file on arrival, O(1) per entry. The narratives are one pass keeping each object's last reading in a map, cut at each Wait note; O(entries × deployments) time, O(objects) space |
| Watch streams open per Scenario (`Record`) | five, one per kind recorded | by a reviewed change to the kinds | run-private, open until the record is stopped | One `Watch` per kind merged by one `select`; a closed stream re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, read by no run | One file, named for its Simulated Cluster |
| Live view observations (the observer) | one per refresh interval per Interactive run, over the a-deployment-placed-for-resilience Scenario's 9 Nodes and Pods | with the Pods and Nodes declared | run-private, written by the observer alone, read by every stream | One `ObserveEveryDeployment`, one board, a fade count per Pod in a map and one render, swapped in under a lock; O((pods + nodes) log (pods + nodes)) per observation, independent of the browsers |
| Live view streams | an engineer's browsers, unrecorded, treated as bounded, held below the open-stream cap | with the browsers open | shared read-mostly | One held connection per stream, woken by a channel closed on each new fragment, writing the latest alone; O(fragment) per change, no render or observation of its own |
| Simulated Clusters at once (`lab.Runner`, the gate) | a run: one per Scenario, an Interactive run one. The gate: per cluster-backed package, the tests its `-parallel` flag admits times each test's clusters. The engine's ceiling unrecorded, treated as bounded | with the Scenarios and the cluster-backed tests | run-private, one container and loopback port per cluster | One goroutine per Scenario, joined before the report. A cluster not started within the start timeout ends its own Scenario or test |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1). A version new to the machine also pays one image pull.
- **Limits:** every wait is bounded by a timeout of § Constants. One Timeline is held in memory and one file open per running Scenario. The API server's limits are unrecorded, treated as bounded.

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

- **Inputs it processes:** the compiled-in Scenarios, the `--k8s-version`, `--whole-timeline` and `--interactive` flags and a Scenario name argument, and `DOCKER_HOST` when set. It also reads the engine's and API server's responses, and the Live view's HTTP requests on loopback.
- **Outputs it produces:** on stdout the settings it resolved and set, and each Scenario's report. An Interactive run also prints the Live view's address, serves its page and pushes its board. A warning names a `DOCKER_HOST` that resolves nothing. The lab also leaves an exit status, Kubernetes resources in the Simulated Cluster, and one Timeline file per Scenario.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port. An Interactive run also listens on a loopback port the operating system chooses, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md).
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable, no service-account token and no home-directory file, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
- **Runtime:** a developer machine or a build agent, as the invoking user, with a container engine socket that answers.

## Threat Model

| Threat | Attack Vector | Mitigation |
|--------|--------------|------------|
| A run drives a real cluster (NG-1) | An ambient kubeconfig, a `KUBERNETES_*` variable, or an in-cluster service-account token discovered by the client | No configuration input carries a cluster address or a credential. The client is constructed from the provisioned address alone, held by the lint rules and tests of `docs/security-principles.md` § Language Realization, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A third party reaches the unauthenticated API server | The engine publishes the API port on every interface | The API port is published on the loopback address, on a host port the engine chooses, per the same ADR |
| A run drives a remote engine | A `DOCKER_HOST` naming a `tcp://` or `ssh://` endpoint | Only a `unix://` path is ever dialed, parsed as a URL and never interpolated into a command. Another scheme is skipped with a warning naming it, per the same ADR
| The engine socket grants control of every container on the machine | The lab holds an API handle the engine cannot scope down | The exported engine methods take a context and a `KubernetesVersion` alone, never an image or a container. Inspect, stop and remove act on this run's handle, and the package declares no list and no by-identifier call, per the same ADR |
| A run pulls or starts an image other than KWOK's cluster image | The version input interpolated into an image reference | The repository is a constant no input reaches. The tag joins the constant KWOK release and a version validated to a leading `v` with digits and dots alone, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md). The container's command is constants alone |
| A moved image tag changes what the Simulated Cluster runs | The pinned tag is re-pushed upstream | The Kubernetes version is pinned by tag, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md); digest pinning is not in force |
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). A signal removes an Interactive run's held cluster; a forced kill leaves it for the engineer to remove |
| Another machine reaches the Live view | The server listens on every interface | It listens on the loopback address alone, on a port the operating system chooses, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) |
| A web page in the engineer's browser reads the Live view or holds its streams | A hostile site rebinds its name to the loopback address, or opens streams to it | A request whose Host is not the address served, or carrying another Origin, is refused. No response grants cross-origin access, per the same ADR |
| A request changes the Simulated Cluster, or ties up the server | A crafted method or path, a slow client, or many streams | Only reads of the page, its script, its stylesheet and the board's stream are served. Server timeouts bound every connection and event write, and a stream beyond the cap is refused, per the same ADR |
| Markup injected into the Live view | A Scenario, Deployment, NodePool, Pod name or label rendered as HTML | The page and the board are rendered through `html/template`, which escapes every name, and the page loads only its own embedded script and stylesheet, per the same ADR |
| A wedged control-plane request keeps a Simulated Cluster past its Scenario | The API server stops answering a request the run waits on | Every request, a watch's opening included, ends at `controlPlaneTimeout`. A watch's stream ends with its recording, which closing the cluster stops before removing it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a random suffix drawn per provisioning, each port is the engine's choice, and a run removes only the container its own handle names, per the same ADR |
| A Timeline is written outside its folder | A Scenario name interpolated into a path | A file name joins the lab's constant prefix, a Scenario name of lowercase ASCII letters, digits and hyphens from a compiled-in function's name, and a random suffix, as one path element. `internal/timeline` refuses a separator or a parent reference in it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) and [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md) |
| A filed Timeline exposes what a run did to another user | The file outlives the run, in the working directory | A created Timeline folder and file are readable and writable by the invoking user alone. A Timeline holds only the simulated objects of a cluster carrying no credential and no real Deployment, per the same ADR |

## Implementation Order

| ID | Name | Depends On |
|----|------|------------|
| REQ-SIM-004 | The gate and the lint rules | — |
| REQ-SIM-001 | Topology, Deployment and the Settled Placement report | REQ-SIM-004 |
| REQ-SIM-002 | One invocation runs every Scenario | REQ-SIM-001 |
| REQ-SIM-003 | One Scenario runs on its own | REQ-SIM-002 |
| REQ-SIM-006 | Given, When, Wait and Then Steps | REQ-SIM-001 |
| REQ-SIM-005 | A Timeline printed and filed | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-008 | Node and Zone Failures | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-009 | Predefined Pod specifications | REQ-SIM-001, REQ-SIM-006 |
| REQ-SIM-011 | Node capacity, Requests and Pending Pods | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009 |
| REQ-SIM-012 | Startup time | REQ-SIM-001, REQ-SIM-006, REQ-SIM-009 |
| REQ-SIM-013 | NodePools | REQ-SIM-001, REQ-SIM-011 |
| REQ-SIM-014 | Scenarios run at once, reported in declared order | REQ-SIM-002, REQ-SIM-003 |
| REQ-SIM-015 | Every Placement-deciding value on a Scenario's lines | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-013 |
| REQ-SIM-016 | Scenarios read by subject, every setting reported | REQ-SIM-008, REQ-SIM-009, REQ-SIM-015 |
| REQ-SIM-017 | Node labels a NodePool states, counted per value | REQ-SIM-013, REQ-SIM-016 |
| REQ-SIM-018 | A Failure of every Node carrying one label value | REQ-SIM-008, REQ-SIM-017 |
| REQ-SIM-019 | A general Deployment stays off a NodePool its Pods do not tolerate | REQ-SIM-013, REQ-SIM-016, REQ-SIM-017 |
| REQ-SIM-020 | Every Scenario in one source, named by its condition | REQ-SIM-006 |
| REQ-SIM-021 | A Wait narrative and a Placement board under every Wait | REQ-SIM-005, REQ-SIM-006, REQ-SIM-013, REQ-SIM-019 |
| REQ-SIM-022 | A Scenario watched live in an Interactive run | REQ-SIM-014, REQ-SIM-020, REQ-SIM-021 |
| REQ-SIM-024 | Standard Pod and GPU Pod Requests | REQ-SIM-009, REQ-SIM-011 |
| REQ-SIM-025 | A Deployment named by its kind | REQ-SIM-009, REQ-SIM-016 |
| REQ-SIM-026 | A Scenario line qualifies what the lab offers | REQ-SIM-016, REQ-SIM-025 |
| REQ-SIM-027 | A Deployment keeps its Floor of Ready Pods through a Failure | REQ-SIM-005, REQ-SIM-008, REQ-SIM-025 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not a `unix://` path, or does not answer | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket | engine resolved |
| engine resolved | a Scenario's function names no condition, or two derive one Scenario name | run stopped before any cluster, each function named |
| engine resolved | `ObtainVersion` finds the version's image held, or pulls it once | version obtained |
| engine resolved | the image is absent and cannot be pulled | run stopped before any Scenario, the version named |
| version obtained | `Provision` starts a Scenario's container and its readiness probe answers | cluster ready |
| version obtained | the engine cannot start a Scenario's container | that Scenario ended, naming why; the others run on |
| cluster ready | `Record` opens the five watch streams, and the Timeline file opens or its path and reason are kept | recording |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | a Wait has `Deploy` create the declared Deployment | Deployment created |
| Deployment created | a Wait finds two agreeing observations of every declared Pod, Ready unless it waits for the Placement alone | Placement Settled, Pods Pending or not |
| Placement Settled | a When stops its Nodes responding | Deployment created, the observation discarded |
| Placement Settled | the Scenario ends, passed or failed | record stopped, each Wait given its narrative and the file closed |
| record stopped | the handle closes | container removed |
| container removed | every Scenario of the run has ended | Outcomes reported in declared order |
| any state | close, or the process exits | container removed |

An Interactive run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | the arguments name no Scenario, an unknown one, or more than one | stopped before the engine, every Scenario name named |
| no cluster | the engine resolves and the version is obtained | the Live view listening, its address named |
| no cluster | no engine resolves, or the version cannot be obtained | stopped before the Live view, naming why |
| the Live view listening | the Scenario's cluster is provisioned and recording | Steps running, observed |
| Steps running, observed | its first When is reached, or its last Step ends, or a Step ends it | record stopped, the Outcome reported, the cluster held |
| the cluster held | the engineer stops the lab | container removed, server shut down |
| Steps running, observed | the engineer stops the lab | Steps stopped, container removed, server shut down |

A Scenario's Steps:

| From | Event | To |
|------|-------|----|
| declared | a Given constructs its Topology or its Deployment | declared, that Step recorded |
| declared | a constructor refuses what a Given declares, or the cluster refuses what a Wait applies | ended, that Step named with the declaration |
| declared | a Wait finds every declared Node, or Pod, Ready, or the Placement alone, unchanged | Placement observed, Pods Pending or not, the Wait's line naming its time |
| declared | the lab's own waiting time outlasts a Wait | ended, that Wait's line naming that time, the Step named with what was not Ready |
| declared | a Wait states a time of none or above the lab's own waiting time, or is reached later than the time it states | ended, that Wait named with the time |
| declared | a Then or a Failure When has no Wait before it, or a When asks for more Nodes or Zones than the Topology holds | ended, that Step named |
| declared | a When names a label or value no NodePool states, or one no observed Node carries | ended, that When named with the label or value |
| Placement observed | a When stops its own Nodes responding | declared, the observation discarded |
| Placement observed | a Then the Placement meets, or does not meet | Placement observed, that Step or a Finding recorded |
| Placement observed | a Then reads a Failure no When ran, judges a Floor no Deployment declares, or counts over a label no NodePool states | ended, that Then named |
| ended | a later Step is reached | ended, nothing carried out or reported |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Pending on that Node, inside its Startup time |
| Pending on that Node | its Startup time passes | Running and Ready |
| Pending | no Node satisfies the specification or the capacity | Pending, reported unplaceable (REQ-SIM-001 edge case 1) |
| Running | its Node stops responding and the waiting time passes | deleted by the control plane, replaced on a Node still responding |
