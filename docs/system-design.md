<!-- harness: 2026-09-30 -->
# System Design Document: k8s-resilience-lab

<!-- AGENT: Current state only. The path to each decision lives in adr/. -->
<!-- AGENT: Source code is authoritative for types, interfaces, parameters, and constant values. Name each contract once, say what it guarantees and which requirement it implements, and point at the source file. Do not transcribe fields, parameters, or constant literals — in a table OR in prose. They rot when the code changes and add no design information. -->
<!-- AGENT: Cross-reference prd.md for requirements, adr/ for decisions. -->
<!-- AGENT: An ADR is cited in full at its first mention in this document; every later mention links a short name for the same ADR. -->

## Overview

k8s-resilience-lab is one Go binary. A run provisions a Simulated Cluster per Scenario, all at once. Its Steps apply its Topology, create a Deployment, Wait for them, and report the Placement reached on a Placement board under every Wait. It records each cluster's changes and Steps as a Timeline, narrates from it above every board, and files it. The Simulated Cluster is a KWOK control plane in a container: scheduling decisions are the real ones, and no Pod image is pulled.

An Interactive run carries out one Scenario's Steps up to its first When, then holds its Simulated Cluster, shown in a Live view the lab serves on loopback. A click there injects a Failure into the held cluster (§ Contracts).

The binary is a modulith composed in `cmd/lab` (§ Package Structure).

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
│   │   ├── descheduler/ # a Descheduler policy as a Scenario writes it
│   │   ├── node/       # a NodePool as a Scenario writes it
│   │   └── pod/        # a Pod specification as a Scenario writes it
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

`internal/scenario` and `internal/timeline` import no `k8s.io/...` package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md) and [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). `internal/timeline` imports `internal/scenario` and the standard library alone; a recording delivers entries to a one-method sink in `internal/cluster`. `internal/live` imports `internal/scenario` and the standard library alone, and only `cmd/lab` imports it, per [ADR: The Live view is served on loopback from one shared observation](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). `internal/modulerules` imports the standard library alone, per the KWOK ADR. Kubernetes types stay inside `internal/cluster`, whose mapper alone names both a Kubernetes and a `scenario` type. `internal/lab` is the one package naming both a Step and the provisioned Simulated Cluster. No package imports a kubeconfig, home-directory or credential-plugin package, calls in-cluster configuration, or reads a home-directory file. The one home-relative path is an engine socket candidate, per [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md).

`scenarios/scenarios.go` holds `All`, then every Scenario in `All`'s order, and no other source holds one. A Scenario is an exported function over `*lab.Scenario`, named in the `<Condition>_Should<Outcome>` shape (`docs/testing-principles.md` § Test Naming). Of the lab's own packages, `scenarios` imports `internal/lab` and its `pod`, `node` and `descheduler` packages alone, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md). A Scenario writing full access also imports the one Kubernetes package naming the Pod specification, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). A Scenario line qualifies each name the lab offers and writes bare each name the Scenarios define, with no alias and no dot import (REQ-SIM-026), per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). `lab` imports `pod`, `node` and `descheduler`, and none of these imports `lab` or another, per the same ADR. The place declares every value the Scenarios define, per [ADR: A Topology declares the NodePools every Zone holds](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). Each import rule is a `golangci-lint` rule, asserted again from the module's own sources (§ Contracts).

## Constants

<!-- Name each constant and cite the source file that owns its value; do not copy the value (source is authoritative). -->

| Name | Source | Description |
|------|--------|-------------|
| `clusterImageRepository` | `internal/cluster/simulated_cluster.go` | The one KWOK cluster image repository, reached by no input, per [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) |
| `kwokRelease`, `defaultKubernetesVersion` | `internal/cluster/simulated_cluster.go` | The KWOK release the image tag carries, and the Kubernetes version a run mirrors by default, per the same ADR |
| The Descheduler pairing | `internal/cluster/descheduler_version.go` | The Descheduler tag paired with each Kubernetes minor, per [ADR: The Descheduler runs as its official image, paired by Kubernetes minor, under a policy the Scenarios define](adr/2026-11-09-the-descheduler-runs-as-its-official-image-paired-by-kubernetes-minor.md) |
| `deschedulerImageRepository`, the pass's command and kubeconfig | `internal/cluster/descheduler_pass.go` | The one Descheduler image repository, reached by no input, and the pass container's arguments and kubeconfig, per the Descheduler ADR |
| `socketCandidates` | `internal/cluster/runtime.go` | The engine socket paths probed in order when `DOCKER_HOST` resolves nothing, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| `maxZones` | `internal/scenario/topology.go` | The highest Zone count a Topology admits |
| The NodePool name rule | `internal/scenario/topology.go` | Kubernetes' label-value syntax a NodePool name must meet, pinned to the client library's, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) |
| `probeTimeout`, `pullTimeout`, `startTimeout` | `internal/cluster/runtime.go` | The budgets for a socket probe, a pull and provisioning |
| `scenario.WaitingTime` | `internal/scenario/wait.go` | The longest the lab waits for a state of the cluster, and the highest time a Wait may state |
| `pollInterval` | `internal/cluster/simulated_cluster.go` | The gap between one wait's observations |
| `controlPlaneTimeout` | `internal/cluster/client.go` | The deadline on every control-plane request, a watch's opening included; a watch's stream carries none, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| Kubernetes label and annotation keys | `internal/cluster/resources.go` | The keys the lab writes on Pods and Nodes, and the simulator's selection label, pinned by test to the domain's lab-own keys
| The lab's own Node label keys and the label syntax | `internal/scenario/node_labels.go` | The keys a NodePool may not state and Kubernetes' label syntax, pinned to the mapper and the client library, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
| The Kubernetes names the lab spells itself | `internal/scenario/pod_specification.go` | The topology labels, taint keys and setting values a specification names |
| The lab's Stage set | `internal/cluster/stages.go` | The Stages the KWOK controller applies, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) |
| `scenario.UnresponsiveNodeWait`, `scenario.DefaultUnresponsiveNodeWait` | `internal/scenario/pod_specification.go` | How long a Pod stays on an unresponsive Node, and the default it replaces, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) |
| `NodeEvictionRate`, `DefaultNodeEvictionRate` | `internal/cluster/runtime.go` | The per-Zone eviction rate a cluster argument sets, and the stock value it replaces, per the same ADR |
| The Live view's refresh interval, newcomer fade, open-stream cap and server timeouts | `internal/live/` | Their values, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md)
| `folderName`, `fileSuffix` | `internal/timeline/timeline.go` | The Timeline folder under the working directory, and the suffix marking a filed Timeline, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| `folderPermissions`, `filePermissions` | `internal/timeline/timeline.go` | Readable and writable by the invoking user alone |

## Contracts

<!-- One row per public type, interface, or function. Purpose in one line; the source file owns the signature; Implements names the requirement(s). No field or parameter lists — those live in source. Add a short prose note above the table only for an invariant a row cannot carry. -->

Every type in `internal/scenario` is immutable: value receivers, fields behind one constructor, slices cloned at every boundary, and a constructor erroring where a caller can get the value wrong. `SimulatedCluster` is the aggregate root of a run, through which alone a Topology, a Deployment and an observation are reached. A Topology derives Node names from positions counted from one, through its NodePools in declaration order (REQ-SIM-013). No package holds mutable process-wide state. A Scenario name is the condition half of its function's name, per [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md); a missing or repeated name stops the run before any cluster (REQ-SIM-020). Every Scenario runs at once on a Simulated Cluster of its own, removed when it ends, and its failure, a panic included, ends it alone, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) (REQ-SIM-014). Pod names come from the control plane, and an observed Pod's Zone from its Node's Zone label. One engine detection and one flag parse serve the whole run, per the engine ADR and [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md).

A Scenario's lines name no error; the `lab.Scenario` value carries each one. A Given constructs its immutable value and applies nothing. A Step's mandatory value is an argument and an optional setting a further call, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A Then the Placement does not meet records a Finding, and the later Steps still run.

A Wait is written `WaitFor()` on the Scenario, or on the Given before it, per [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](adr/2026-10-07-a-scenario-writes-its-own-wait.md). A Wait applies what the Givens before it declared and observes the Settled Placement beneath its condition (REQ-SIM-011). Every Then and Failure reads the Placement the last Wait reached. A Wait is held to the lab's own waiting time, stated or not (§ State Machine; REQ-SIM-006, REQ-SIM-008).

A Deployment's Given names a Customization for its Pods' Request, then its Pod specification with zero or more Customizations, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). Each returns a copy, the last setting per field winning. Full access reaches the Pod specification alone, never the Deployment's label or selector. The mapper re-applies the specification's own tolerations last and writes the Startup time onto Pod template annotations, per [the Stage ADR](adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) (REQ-SIM-012). A value the control plane refuses ends the Scenario at the Wait deploying it. A Given line's settings print in the domain's order (REQ-SIM-016).

The lab defines no Pod specification, Startup time, Node capacity or setting group of its own: the Scenarios define each through `pod.Predefined`, `pod.Settings` and `node.Capacity` (REQ-SIM-015). A definition omitting its Startup time does not compile; it holds no Request (REQ-SIM-024), and names the Named setting `LeaveUnresponsiveNodes` rather than either lab-taint toleration, per the specification ADR. Every field of a setting the lab offers is required, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md). A field left out, or stated against a toleration's operator and effect, is refused at the Given (REQ-SIM-016, REQ-SIM-019).

A Given declares a Scenario's one Descheduler policy, a `descheduler.Policy` keyed literal the Scenarios define, mirroring `descheduler/v1alpha2` key for key, per [the Descheduler ADR](adr/2026-11-09-the-descheduler-runs-as-its-official-image-paired-by-kubernetes-minor.md) (REQ-SIM-031). The lab adds no entry and derives no extension point. A policy is named by its profile names in order, and its report names the paired Descheduler and each entry under the policy's own keys. A second policy, and what `scenario.DeschedulerPolicy` refuses, are refused at the Given. Before any cluster, a run or held run carries out every Scenario's Givens. A declared policy stops it at an unpaired version, naming the version, and otherwise has `ObtainDescheduler` obtain the paired image once. An image not obtained ends each declaring Scenario at its Descheduler Given, naming why, with no retry.

A When runs one Descheduler pass under the declared policy, a container of the paired image beside its cluster's (§ Threat Model), per [the Descheduler ADR](adr/2026-11-09-the-descheduler-runs-as-its-official-image-paired-by-kubernetes-minor.md) (REQ-SIM-031). The pass is held to `scenario.WaitingTime`. Its container is removed by its name before the pass and on every path after it, absence counting as removed. A pass failing or out of time ends the Scenario at that When, naming the cause and the container's last log lines. The lab evicts no Pod itself. The mapper reads a Pod as descheduled once, at the first change carrying the Eviction API's disruption-target mark and no deletion mark; its removal reads as removed. The Wait narrative names that eviction apart from a Failure's.

`EverySeconds`, a further call on the Descheduler Given, runs its passes one at a time on a `scenario.DeschedulerInterval`, per the Descheduler ADR (REQ-SIM-031). The interval runs from the Wait applying the Given until the runner stops and joins it, before the cluster closes. A held run keeps it running. The next pass starts at the later of one interval after the last's start and the last's exit. A pass evicted nothing when every Pod listed before it is listed after it, unmarked and no disruption target. A failed pass stops the interval and ends the Scenario at its next Step or its end. `WaitFor().DeschedulerPassEvictingNothing()` ends at the first pass begun after it that evicted nothing, then reaches the Settled Placement. It ends the Scenario with no interval running, as a When's pass does while one runs. Beside a declared Floor, a policy on an interval evicts one Pod per pass at most, per the Descheduler ADR.

A later Deployment Given gives the Scenario its next Deployment, which the next Wait creates beside the earlier ones, still deployed, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md). Every later Wait, When and Then judges the newest alone, `EnoughPodsStayedReady` and `PodsFail` aside, and a board shows every Deployment (REQ-SIM-019, REQ-SIM-021, REQ-SIM-030).

A Topology names its Zone count, then the NodePools every Zone holds, each with its name, Node count per Zone and capacity, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md). Every Node carries its NodePool's name under the lab's NodePool label (REQ-SIM-013). A Node capacity, stated in every dimension or named, is both the Node's capacity and its allocatable room, and the scheduler alone decides which Pods fit (REQ-SIM-011). Every quantity crossing into Kubernetes comes from the client library's own constructor, never a parsed string.

A NodePool states Node labels as `node.Labels` and taints as `node.Taint` keyed literals, refused at the Topology Given as their `scenario` rows state (REQ-SIM-017, REQ-SIM-019). Each probe Pod tolerates its own Node's taints, so provisioning ends on a tainted Node too. A Then counting Pods per label value or on one NodePool reads the observed Node's labels. A When by label stops every observed Node carrying it, in the Topology's order (REQ-SIM-018).

A Deployment's further call declares its Failure tolerance, `lab.Zones(n)` or `lab.Nodes(n)`, setting its Floor, per [ADR: A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline](adr/2026-10-06-a-deployment-declares-the-zones-or-nodes-it-survives-losing.md) (REQ-SIM-027). `EnoughPodsStayedReady` replays the Scenario's Timeline copy from the first When's note. A Pod counts while Running, lost from the note of the When stopping its Node until a When makes it respond again (REQ-SIM-029). The k-th When's Nodes pair with the k-th When note; a When losing Pods or running a Descheduler pass pairs with none (REQ-SIM-030, REQ-SIM-031).

A When makes every Node stopped so far respond again, `NodesRespondAgain` naming each Node and `ZonesRespondAgain` each Zone, in the Topology's order (REQ-SIM-029). A Scenario keeps the Nodes stopped now apart from the Nodes and Zones that failed, which the Thens read, and a later Failure may pick a Node that responded again.

A When `PodsFail` loses that many placed Pods of a declared Deployment in one Step, each a Pod Failure as a click injects (REQ-SIM-030). It picks the first in the last Wait's board's order. It refuses as `scenario.PodsFailure` does, and a picked Pod the cluster no longer holds ends the Scenario. The When discards the observation.

Every Scenario's run records a Timeline under one owning goroutine, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md). Receipt order is the Timeline's order. A Scenario notes each Step through the same goroutine. A Given or a Then is noted once its line is complete, a When before its first change, and a Wait as it ends, after its board (REQ-SIM-021).

A Wait narrative holds the moments after the previous Wait's note up to its own, the k-th Wait note being the k-th Wait Step's. A Pod belongs to the Deployment whose name begins its own, at the longest match. A Pod no Deployment claims is the lab's own setup and in no narrative. An eviction is read from a Pod's first entry marked for deletion, or a descheduled Pod's removal when none came first (REQ-SIM-021, REQ-SIM-031).

Every Wait, reached or not, ends with one observation of the whole Simulated Cluster, held to the lab's waiting time, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) (REQ-SIM-021). A Wait before the Scenario's first change observes nothing, and a failed observation ends the Scenario at that Wait. A board marks a Pod new where the previous board did not hold it on that Node. The reports are written from the `lab.Outcome` values once every Scenario has ended, in the layout REQ-SIM-021 states.

An Interactive run is `--interactive` with one Scenario name, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). Any other argument shape stops the lab before the engine is detected, naming every Scenario name in `All`'s order (REQ-SIM-022). It detects the engine and obtains the version, then listens and names the Live view's address before provisioning (REQ-SIM-022 edge case 8).

It carries out its Scenario through a run's runner up to its first When (§ State Machine; REQ-SIM-022). An interrupt or a termination signal cancels its context; the cluster is then removed under a detached context, and the server shuts down within its bound (REQ-SIM-022 edge case 7).

From provisioning until removal one observer goroutine builds a `scenario.PlacementBoard` from `ObserveEveryDeployment` at the refresh interval, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) (REQ-SIM-022 edge case 6). Before a Topology is declared the Live view shows none, and a failed observation shows its cause until the next. `ObserveEveryDeployment` reports each Node's readiness beside the Node, never on `scenario.Node`, whose equality a Failure reads. A run starts no observer.

The Live view serves its page, script and stylesheet, and the board as Server-Sent Events, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md). Each observation renders one `html/template` fragment, pushed to every open stream when it changed. A stream opens on the latest, and a lagging browser skips to it (REQ-SIM-022 edge case 5). The page is rendered once at listening, its header naming the held Scenario (REQ-SIM-022). Its one write is a POST naming a Node, a Zone or a Pod, handing that click to the held run (REQ-SIM-028). It reads no body, and its refusals and bounds are § Threat Model's. Closing the server ends every stream first.

The fragment lays out Zones, Nodes and Pods as REQ-SIM-022 states, each box under a stable id derived from its name. A Zone's, a Node's and a placed Pod's box carry the Failure a click on it injects, and the script posts the innermost box's Failure (REQ-SIM-028). Nothing served names another host (REQ-SIM-022).

Once its Outcome is reported, the held run takes the Live view's clicks one at a time until its context ends; earlier a click changes nothing (REQ-SIM-028). A Node or Zone name resolves against the declared Topology, a Pod name against the latest board's placed Pods; one resolving to nothing changes nothing. A stopped Node, or a wholly stopped Zone, responds again; another Zone stops its Nodes still responding. Each click runs under the held run's context, never the request's (REQ-SIM-033).

A held run's Timeline keeps recording until the lab stops, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) (REQ-SIM-033). A click that resolves notes its When before its first change. At the hold, the owning goroutine hands over the Timeline so far, notes included, then each later entry in order. The Outcome reported at the hold carries no Timeline line. That line prints once the context ends, no click is under way and the file has closed. `scenario.Following` reads the later entries into terminal lines by the Wait narratives' reading. A When's note prints as its Step, every other line as a narrative's moment once complete, so lines may stand out of elapsed order. A moment read before the hold prints nothing. One goroutine of the held run prints these lines. When the Descheduler interval stops while held, that goroutine also prints the stop once, worded as for a Step left undone. It prints no such line when the Outcome reported at the hold already named the stop. The Outcome returned at the lab's stop then ends at the Given that declared the interval, unless it already ended at an earlier Step. Either way the lab exits non-zero, as for a Step left undone. The goroutine prints no line once the lab stops; the hold waits for it unless a line is printing. A terminal that stops taking lines holds up neither filing nor removal.

`make demo` starts an Interactive run and opens its Live view in the machine's browser (REQ-SIM-032). It and `make interactive` hand `ARGS` to the lab ahead of `--interactive`. The Makefile recipe opens it, never the lab, which starts no external process (`docs/security-principles.md`). The recipe passes the lab's output on unchanged, lets the lab's shutdown lines through an interrupt, and exits with the lab's status (§ Threat Model).

| Contract | Purpose | Source | Implements |
|----------|---------|--------|------------|
| `make ci` | The gate command, failing on the first check that fails. Its tests run under the race detector, per [ADR: The gate runs the tests under the race detector](adr/2026-10-04-race-detector-in-the-gate.md). Its timeout and `-parallel` flag follow § Scale and Load | `Makefile` | REQ-SIM-004, REQ-SIM-011, REQ-SIM-014 |
| `make demo` | Starts an Interactive run and opens its Live view through the machine's opener | `Makefile` | REQ-SIM-032 |
| `make deps-check` | Fails, naming the offender, when a prohibited dependency class (§ Dependency Policy) enters the module graph | `Makefile` | REQ-SIM-004 |
| depguard and `forbidigo` rules | Deny the imports and the in-cluster configuration call § Package Structure rules out | `.golangci.yml` | REQ-SIM-004, REQ-SIM-006, REQ-SIM-022 |
| Import-graph and call-rule tests | Assert every lint rule again from the module's own package graph and files, and that no production package reaches the test-support package | `internal/scenario/imports_test.go`, `internal/cluster/imports_test.go`, `internal/cluster/callrule_test.go`, `internal/lab/scenarios_rules_test.go` | REQ-SIM-001, REQ-SIM-004, REQ-SIM-006, REQ-SIM-022 |
| `modulerules` helpers | The package-graph, call and declaration scans the module-rule tests share | `internal/modulerules/` | REQ-SIM-001, REQ-SIM-006 |
| `scenario.NodeCapacity`, `scenario.CPU`, `scenario.Memory` | The capacity one Node holds, refused at none in any dimension, in integers Kubernetes' own quantity carries exactly | `internal/scenario/topology.go`, `internal/scenario/quantity.go` | REQ-SIM-001, REQ-SIM-011 |
| `scenario.NamedNodeCapacity`, `scenario.NewNamedNodeCapacity` | A Node capacity under a name a Scenario gives it, refused at an empty name, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-011, REQ-SIM-015 |
| `scenario.Node` | One simulated machine: its Zone, NodePool, capacity, labels and taints, compared through its equality method | `internal/scenario/topology.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.NodeTaints`, `scenario.Taint` | The NoSchedule taints a NodePool states, immutable and equal by value, refusing what its test asserts | `internal/scenario/node_taints.go`, `internal/scenario/node_taints_test.go` | REQ-SIM-019 |
| `scenario.NodeLabels` | The labels a NodePool states, immutable and equal by value, refusing what its test asserts | `internal/scenario/node_labels.go`, `internal/scenario/node_labels_test.go` | REQ-SIM-017 |
| `scenario.NodePool` | A name, a Node count per Zone, a capacity and any stated labels and taints, refusing what its test asserts | `internal/scenario/topology.go` | REQ-SIM-013, REQ-SIM-015, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Topology` | The Zones and their NodePools in declaration order, expanding to Nodes under derived names, refusing what its test asserts | `internal/scenario/topology.go`, `internal/scenario/topology_test.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Label rule tests | Assert the domain's label and taint rules agreeing with the client library's validation, and its lab-own keys being those the mapper writes, per [the NodePool ADR](adr/2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | `internal/cluster/resources_test.go` | REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `scenario.Deployment`, `scenario.FailureTolerance` | A declared replica count, its Pod specification, and any Failure tolerance with its Floor, refusing what its test asserts | `internal/scenario/deployment.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-019, REQ-SIM-025, REQ-SIM-027 |
| `scenario.PodSpecification` | The settings one Deployment's Pods are deployed with under their intent, and their Startup time: immutable and equal by value | `internal/scenario/pod_specification.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015 |
| `scenario.StartupTime` | The time a placed Pod takes to become Ready, one time or a range of whole milliseconds, refusing what its test asserts | `internal/scenario/pod_specification.go` | REQ-SIM-012 |
| `scenario.DeschedulerPolicy` | A Descheduler policy as the domain holds it, its eviction limits and its profiles in order: immutable and equal by value, refusing what its test asserts | `internal/scenario/descheduler_policy.go`, `internal/scenario/descheduler_policy_test.go` | REQ-SIM-031 |
| `scenario.DeschedulerInterval` | The interval a Descheduler's passes run on, in whole seconds: immutable and equal by value, refusing what its test asserts | `internal/scenario/descheduler_policy.go`, `internal/scenario/descheduler_policy_test.go` | REQ-SIM-031 |
| `scenario.TopologySpread`, `scenario.Toleration`, `scenario.Request` | The settings a specification may hold, each field as Kubernetes names it, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) | `internal/scenario/pod_specification.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-011, REQ-SIM-019 |
| Specification-definition tests | Assert the definition, Customization and grouping rules | `internal/lab/pod/specification_test.go`, `internal/scenario/pod_specification_test.go` | REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016 |
| Specification-mapping test | Asserts the transcription rule below, every Kubernetes name pinned to the client library's, and the annotations, labels and taints the mapper writes | `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-009, REQ-SIM-012, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| Specification placement tests | Assert a deployed Pod carrying its settings, Pods staying Pending where no Node has room or tolerance, and no Pod Ready before its Startup time | `internal/cluster/simulated_cluster_test.go` | REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-019 |
| `scenario.Pod`, `scenario.PodPhase` | One observed Pod, placed or not, with its reported phase, readiness, Deployment name and Requests | `internal/scenario/placement.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-021, REQ-SIM-022 |
| `scenario.PlacementBoard` | One observation's view of the whole Simulated Cluster, every Node by Zone with its Pods and room and newcomers marked, rendering its own lines | `internal/scenario/placement_board.go`, `internal/scenario/placement_board_test.go` | REQ-SIM-021, REQ-SIM-022 |
| `scenario.Placement`, `scenario.SpreadDomain` | Where a Deployment's Pods sit at one observation against the Nodes that could hold them, with the Skew and the counts a Then reads | `internal/scenario/placement.go`, `internal/scenario/placement_test.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-017, REQ-SIM-019 |
| `scenario.NodeFailure`, `scenario.ZoneFailure` | The Nodes, or Zones, one Failure stops, resolved at construction to the first its Topology derives or to the one named, refusing what its test asserts | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-008, REQ-SIM-017, REQ-SIM-028 |
| `scenario.NodesRespondingAgain`, `scenario.ZonesRespondingAgain` | The Nodes, or Zones, one When makes respond again: every Node stopped so far, in the Topology's order, refusing what its test asserts | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-029 |
| `scenario.PodFailure` | The one placed Pod a Failure loses, resolved against a Placement board, refusing what its test asserts | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-028 |
| `scenario.PodsFailure` | The first placed Pods of one Deployment in a Placement board's order, each a `scenario.PodFailure`, refusing what its test asserts | `internal/scenario/failure.go`, `internal/scenario/failure_test.go` | REQ-SIM-030 |
| `scenario.LabelFailure` | The observed Nodes carrying the one label value a Failure names, in the Topology's order, refusing what its test asserts | `internal/scenario/failure.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-018 |
| `scenario.StatedWait` | The one carrier of the time a Wait is held to, stated or not, judging a measured wait against itself | `internal/scenario/wait.go`, `internal/scenario/wait_test.go` | REQ-SIM-006 |
| Provisioned-address client | The one construction path to the control plane: the provisioned loopback address and no credential, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) | `internal/cluster/client.go`, `internal/cluster/client_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-021 |
| `cluster.DetectEngine` | Resolves the engine socket once, returning the endpoint and version that answered, or errors naming every path tried | `internal/cluster/runtime.go`, `internal/cluster/runtime_test.go` | REQ-SIM-001 |
| `cluster.Engine`, `cluster.ClusterContainer` | The restricted engine surface over one unix socket, and the handles to this Scenario's cluster container and one pass container at a time, asserted against § Threat Model | `internal/cluster/runtime.go`, `internal/cluster/engine_surface_test.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-022, REQ-SIM-031, REQ-SIM-032 |
| `cluster.KubernetesVersion`, `cluster.ParseKubernetesVersion` | The validated version a run mirrors: a leading `v`, digits and dots alone | `internal/cluster/version.go` | REQ-SIM-001 |
| `cluster.DeschedulerVersion` and its pairing | The Descheduler paired with a Kubernetes version's minor, or an error naming that version | `internal/cluster/descheduler_version.go`, `internal/cluster/descheduler_version_test.go` | REQ-SIM-031 |
| The policy encoding | A Descheduler policy as `descheduler/v1alpha2` JSON through `encoding/json`, stating what the policy states and nothing else | `internal/cluster/descheduler_policy.go`, `internal/cluster/descheduler_policy_test.go` | REQ-SIM-031 |
| `cluster.Provision` | Starts the Simulated Cluster, waits for its API server, makes the served Stages the lab's own set, and returns the handle. A cluster it could not finish, a stop included, is removed under a detached context | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-012, REQ-SIM-022, REQ-SIM-032 |
| `cluster.ObtainVersion`, `ObtainDescheduler` | Makes the engine hold a version's cluster image, or a paired Descheduler's, pulling it only when absent, or errors naming that version | `internal/cluster/simulated_cluster.go`, `internal/cluster/runtime.go` | REQ-SIM-001, REQ-SIM-014, REQ-SIM-031 |
| `cluster.SimulatedCluster` | The provisioned cluster a run drives, removing any pass container by its name, then itself, on close | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-014, REQ-SIM-031 |
| `cluster.SimulatedCluster.ApplyTopology` | Creates every declared Node Ready, returning once the scheduler has placed and released a probe Pod on each, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](adr/2026-10-04-scheduler-observable-provisioning.md) | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-001, REQ-SIM-013, REQ-SIM-017, REQ-SIM-019 |
| `cluster.SimulatedCluster.AwaitSettled` | Polls a Deployment's Placement until two agreeing observations account for every declared Pod, or the waiting time expires | `internal/cluster/simulated_cluster.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012 |
| `cluster.SimulatedCluster.AwaitPodsReady`, `AwaitNodesReady` | The same loop, until every declared Pod, or Node, reports Ready, erroring with those that did not | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-006 |
| `cluster.SimulatedCluster.ObserveEveryDeployment` | One observation of every Topology Node with its readiness, and every Pod carrying the lab's Deployment label with its Requests, so no probe Pod | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-021, REQ-SIM-022 |
| `cluster.SimulatedCluster.FailNode`, `RestoreNode` | Stops one Node responding, off the KWOK controller with its Ready condition Unknown; `RestoreNode` puts it back Ready, per [the Node Failure ADR](adr/2026-10-05-node-failure-withholds-the-nodes-status.md) | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go`, `internal/cluster/resources_test.go` | REQ-SIM-008, REQ-SIM-028, REQ-SIM-029 |
| `cluster.SimulatedCluster.LosePod` | Deletes one Pod carrying the lab's Deployment label with no grace period, its ReplicaSet replacing it | `internal/cluster/simulated_cluster.go`, `internal/cluster/simulated_cluster_test.go` | REQ-SIM-028, REQ-SIM-030 |
| `cluster.SimulatedCluster.RunDeschedulerPass` | Runs one Descheduler pass of the paired image, fetching none, under a policy, returning whether it evicted a Pod or an error naming why, its container removed | `internal/cluster/descheduler_pass.go`, `internal/cluster/descheduler_pass_test.go` | REQ-SIM-031 |
| `scenario.Timeline`, `scenario.Entry`, `scenario.Change`, `scenario.ObjectKind`, `scenario.NodeReadiness` | The ordered, immutable record one Scenario's run left, and one change in it, rendering its own report line | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-005 |
| `scenario.Timeline`'s Floor reading | Each declaring Deployment's lowest Ready count from the first When's note and its deepest fall below its Floor; a pure function of the entries | `internal/scenario/floor.go`, `internal/scenario/floor_test.go` | REQ-SIM-027, REQ-SIM-029 |
| `scenario.Timeline`'s Wait narratives, `scenario.StepObject` | The moments that changed the Placement before each Wait, one narrative per Wait note, and the kind a Step's note carries; a pure function of the entries | `internal/scenario/timeline.go`, `internal/scenario/timeline_test.go` | REQ-SIM-021, REQ-SIM-031 |
| `scenario.Following` | A held cluster's terminal lines, read one entry at a time after the Timeline so far by the Wait narratives' own reading, asserted without a cluster | `internal/scenario/following.go`, `internal/scenario/timeline.go`, `internal/scenario/following_test.go` | REQ-SIM-033 |
| `cluster.SimulatedCluster.Record`, `StopRecording`, the Step note | The recording goroutine over the watch streams, per the Timeline ADR. It delivers entries to a sink, times Step notes on receipt, and stops idempotently. A reader gets the Timeline so far, then each later entry until `Record`'s context ends, and none after. A Step note returns once handed on or with no reader | `internal/cluster/timeline.go`, `internal/cluster/timeline_test.go` | REQ-SIM-005, REQ-SIM-021, REQ-SIM-031, REQ-SIM-033 |
| Entry and observation mapper | The one place a watched Kubernetes object becomes a `scenario.Entry`, and an observed Pod a `scenario.Pod` requesting what the scheduler sums over its containers | `internal/cluster/resources.go`, `internal/cluster/resources_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-021, REQ-SIM-022, REQ-SIM-031 |
| Timeline-recording tests | Assert recording in receipt order, the re-open rule and the hand-over to a reader. Its hand-written `watch.Interface` is the lab's one stand-in for a boundary, per the Timeline ADR | `internal/cluster/timeline_test.go` | REQ-SIM-005, REQ-SIM-033 |
| `timeline.Folder`, `timeline.File` | Where a run's Timelines collect, and one Scenario's filed Timeline named for its cluster in JSON Lines, keeping its first write error | `internal/timeline/timeline.go`, `internal/timeline/timeline_test.go` | REQ-SIM-005 |
| `expect.Expect`, `expect.Equal`, `expect.HaveLen` | The assertion entry point over the interface `*testing.T` and a Then share, and its matchers | `internal/expect/expect.go` | REQ-SIM-001 |
| `lab.Scenario` | The Step surface a Scenario's lines run against, recording every Step and holding its Deployments, its stopped and failed Nodes, and its Timeline's copy | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-019, REQ-SIM-027, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| `lab.DeploymentName` | The Deployment Step's named argument, a defined type so a swap does not compile; every count is an `int` under the Step naming its subject | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-013, REQ-SIM-016, REQ-SIM-025 |
| `lab.DeclaredTopology`, `lab.DeclaredDeployment`, `lab.FailureTolerance` | The Topology or Deployment one Given line declared, carrying the domain value or its first refusal, and the further call naming a Deployment's Failure tolerance | `internal/lab/scenario.go` | REQ-SIM-001, REQ-SIM-011, REQ-SIM-013, REQ-SIM-024, REQ-SIM-025, REQ-SIM-027 |
| `lab.Given.Descheduler`, `lab.DeclaredDescheduler.EverySeconds` | The Given declaring a Scenario's `descheduler.Policy`, carrying the domain value or its first refusal, and its further call stating the interval | `internal/lab/scenario.go` | REQ-SIM-031 |
| The Descheduler interval | One goroutine starting a pass each interval until stopped, publishing each pass's outcome to the Scenario under a lock | `internal/lab/descheduler_interval.go`, `internal/lab/runner.go`, `internal/lab/runner_cluster_test.go` | REQ-SIM-031, REQ-SIM-033 |
| The `node` package | A NodePool as a Scenario writes it, under Kubernetes' resource names, carrying the domain value or its first refusal | `internal/lab/node/node.go`, `internal/lab/node/node_test.go` | REQ-SIM-001, REQ-SIM-008, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-019 |
| The `pod` package | A Pod specification as a Scenario writes it, defined through `pod.Predefined` and customized in the order written, under Kubernetes' type and field names, per [the read-by-subject ADR](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) | `internal/lab/pod/specification.go`, `internal/lab/pod/settings.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-019 |
| The `descheduler` package | A Descheduler policy as a Scenario writes it, `descheduler.Policy`, keyed as the YAML the Descheduler itself reads, carrying the domain value or its first refusal | `internal/lab/descheduler/policy.go` | REQ-SIM-031 |
| `pod.FullAccess`, `cluster.PodSpecAccess` | A Scenario's own change to the Kubernetes Pod specification, run after the settings are transcribed | `internal/lab/pod/specification.go`, `internal/cluster/resources.go` | REQ-SIM-009, REQ-SIM-016 |
| Setting-name tests | Assert each setting's field names against the client library's own type, and every `pod` and `node` value equal to the domain's | `internal/lab/pod/settings_test.go`, `internal/lab/node/node_test.go` | REQ-SIM-016, REQ-SIM-019 |
| `lab.When`, `lab.When.NodesLabelledFail`, `NodesRespondAgain`, `ZonesRespondAgain`, `PodsFail`, `DeschedulerRunsOnce` | The When Step: a Failure of Nodes, Zones or a label value's Nodes, the stopped Nodes responding again, Pods of one Deployment lost, or one Descheduler pass. Its Step names what it acted on | `internal/lab/scenario.go`, `internal/lab/runner_cluster_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-018, REQ-SIM-025, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| `lab.Wait`, `lab.TopologyWait`, `lab.DeploymentWait` | The Waits `WaitFor()` offers on the Scenario and on a Given, each plain or with a stated time. `DeschedulerPassEvictingNothing` is plain alone | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-011, REQ-SIM-031 |
| `lab.Then` judgements | Judge the Settled Placement: no Pod on a failed Node or Zone, the Pods left Pending, each declaring Deployment's Floor as `EnoughPodsStayedReady`, `PodsPerValueOf` a label, and `PodsOnNodePool` | `internal/lab/scenario.go` | REQ-SIM-008, REQ-SIM-011, REQ-SIM-017, REQ-SIM-019, REQ-SIM-025, REQ-SIM-027, REQ-SIM-029 |
| `lab.Step`, `lab.Finding` | One line of a Scenario as it ran, a Wait's with its timing, narrative and board, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md); and one Then the Placement does not meet | `internal/lab/scenario.go` | REQ-SIM-006, REQ-SIM-016, REQ-SIM-021 |
| `lab.Outcome` | What one Scenario reported, its Timeline's path or filing error included, each Wait Step with its narrative | `internal/lab/runner.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-021, REQ-SIM-033 |
| `lab.Runner`, `lab.NewRunner` | Runs each Scenario at once on its own Simulated Cluster under its derived name, recording its Timeline and recovering a panic into its error. It returns results in declared order, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-005, REQ-SIM-006, REQ-SIM-014, REQ-SIM-020, REQ-SIM-022, REQ-SIM-027, REQ-SIM-031 |
| Step-surface tests | Assert the Step, Failure and Failure tolerance rules without a cluster | `internal/lab/scenario_test.go`, `internal/scenario/deployment_test.go`, `internal/scenario/placement_test.go`, `internal/scenario/label_failure_test.go` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-024, REQ-SIM-025, REQ-SIM-027, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| Cluster-backed Step-surface tests | Assert the Step surface on a real Simulated Cluster, every Wait form included | `internal/lab/scenario_deployment_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-018, REQ-SIM-019 |
| Scenario-place tests | Assert off the Scenario sources the § Package Structure and definition rules, every Scenario after `All` in its order, and a Wait Step before each Then and after each When | `internal/lab/scenarios_rules_test.go` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-024, REQ-SIM-025, REQ-SIM-026, REQ-SIM-027, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| The Scenarios' definitions, `scenarios.All` | Every value the Scenarios define, and the list of Scenarios a run iterates | `scenarios/` | REQ-SIM-006, REQ-SIM-009, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-024, REQ-SIM-025, REQ-SIM-026, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| The lab's Scenarios | Each states above its Steps what it is for, naming neither function nor Step, per [the Wait ADR](adr/2026-10-07-a-scenario-writes-its-own-wait.md) | `scenarios/scenarios.go` | REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-009, REQ-SIM-011, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-026, REQ-SIM-027, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| Scenarios README | What a Scenario is, its vocabulary, and every Scenario once under its Scenario name with its intent | `scenarios/README.md` | REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-020, REQ-SIM-026, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031 |
| `lab` command | Parses its flags, detects the engine once, runs `scenarios.All` and prints each Outcome in declared order, and an Interactive run's held lines and Timeline line. A Finding, a Step left undone, or an unwritten Timeline exits non-zero | `cmd/lab/main.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016, REQ-SIM-021, REQ-SIM-022, REQ-SIM-027, REQ-SIM-028, REQ-SIM-033 |
| Command tests | Assert every shipped Scenario running clean within its stated times and every report it prints over one shared run, and an Interactive run's argument refusals and Live view line | `cmd/lab/main_test.go` | REQ-SIM-001, REQ-SIM-005, REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-012, REQ-SIM-014, REQ-SIM-015, REQ-SIM-016, REQ-SIM-017, REQ-SIM-018, REQ-SIM-019, REQ-SIM-021, REQ-SIM-022, REQ-SIM-024, REQ-SIM-027, REQ-SIM-029, REQ-SIM-030, REQ-SIM-031, REQ-SIM-032 |
| The Interactive run's held run | Carries out one Scenario up to its first When, reports its Outcome and holds the cluster until its context ends | `internal/lab/runner.go`, `internal/lab/scenario.go`, `internal/lab/descheduler_interval.go`, `internal/lab/runner_cluster_test.go` | REQ-SIM-022, REQ-SIM-028, REQ-SIM-033 |
| Scenario lookup by name | The one Scenario whose derived name is the name given, or an error naming every Scenario name in `All`'s order | `internal/lab/runner.go`, `internal/lab/runner_test.go` | REQ-SIM-022 |
| The Live view | The loopback server of the page and the latest board, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) | `internal/live/` | REQ-SIM-022, REQ-SIM-028 |
| Live view tests | Assert over a real loopback server and real boards what REQ-SIM-022 states the page shows, and a change reaching two browsers without a reload | `internal/live/` | REQ-SIM-022, REQ-SIM-028 |

A Deployment's Pod template transcribes its specification: one spread constraint, with no minimum domain count, and one toleration per setting, per [the specification ADR](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md). The container, the Deployment label and the selector are the mapper's own. KWOK's controller maintains marked Nodes alone, and the control plane keeps Kubernetes' heartbeat timings. A Failure takes its Node out of the managed and the Ready Nodes, and the control plane taints it unreachable and moves its Pods. A Pod being deleted is in no Placement.

## Scale and Load

<!-- PROJECT: The workload the code is sized for. The design owner writes each row at triage; the implementer selects against it and the code-quality reviewer judges against the same row. A figure nobody knows is written as "unrecorded, treated as bounded" and corrected here when the real figure arrives. "Bounded" is a complete row. -->

| Data set or path | Realistic size | Growth | Access pattern | Form |
|------------------|----------------|--------|----------------|------|
| Engine socket candidates (`DetectEngine`) | a `unix://` `DOCKER_HOST` when set, then `socketCandidates` | by a reviewed change to the constant | run-once, before every engine call | Probed in order within the probe timeout; the first to answer is kept |
| Engine pull progress stream (missing image only) | one pull per absent image per run; its messages unrecorded, treated as bounded | with the image's layers | run-once, written nowhere | Decoded one object at a time and discarded; O(messages) time, constant memory |
| Nodes and probe Pods per provisioning (`ApplyTopology`) | 9 Nodes in the shipped Scenarios, a probe Pod each; above that unrecorded, treated as bounded | with the Zones and NodePools, at most `maxZones` Zones | run-private, written once | One create and status update per Node, one probe each, one list per poll; O(nodes) per poll, within the start timeout |
| NodePools, labels and taints of one Topology (`scenario.Topology`, `NodeLabels`, `NodeTaints`) | REQ-SIM-013's 2 NodePools, one label per NodePool, one taint; above that unrecorded, treated as bounded | with the NodePool lines written | run-private, built once per Given | NodePools in declaration order, labels and taints in key order; O(pools) to build, O(nodes) to expand, O(n log n) per label or taint set |
| Pods of one Deployment per observation (`Placement`), and Deployments per Scenario | 9 Pods and two Deployments in the shipped Scenarios; above that unrecorded, treated as bounded | with the Pods and Deployment Givens declared | run-private, read-mostly | One label-selected list of the newest Deployment, counted by Node, Zone, NodePool and label value; O(pods + nodes) per observation |
| Settings of one Pod template (the mapper) and its Given's report | Resilient placement's 4, Default placement's 2 | one per field named and per full access | run-private, written once | Transcribed and reported in definition order; O(settings + accesses) |
| Named Node capacities, predefined specifications, Named settings, one line's Customizations (`scenarios/`, `Customize`) | Standard and GPU, four specifications, four Named settings; Customizations unrecorded, treated as bounded | by a reviewed change; with the Customizations written | built once per NodePool or Deployment naming one | One call per name, returned by copy |
| Stages applied per provisioning (`Provision`) | the lab's set, following KWOK's published set at the pinned release, under ten | by a reviewed change to the set or the release | run-private, written once | One list, then one create, replace or delete per Stage; O(stages) |
| Wait polling (`AwaitSettled`, `AwaitPodsReady`, `AwaitNodesReady`) | one observation per poll interval, one wait per Wait, the Node Failure Scenario's 3 | with the Waits written and the Nodes declared | run-private | Two observations compared by value, or one list matched against the declared names; O(pods + nodes) per poll |
| Timeline entries replayed per `EnoughPodsStayedReady` | as Timeline entries per Scenario, two Deployments | with the Nodes, Pods and Steps | run-private, replayed once per Then | One pass keeping each Pod's last reading in a map; O(entries × deployments) time, O(entries) space |
| Failures and restorations per When or click (`FailNode`, `RestoreNode`, `LosePod`, `scenario.PodsFailure`, the responding-again values) | one Node, a Zone's or a label value's 3, or one Deployment's 9 placed Pods in the shipped Scenarios; clicks unrecorded, treated as bounded | with the Nodes, replicas and clicks | run-private, once per When; a click's under the held run's lock, one at a time | Simplest readable form: one resolution against the Topology or the latest board, then a label and a status write per Node, retried on a conflict, or one delete per Pod; O(nodes stopped × nodes) or O(pods) |
| Packages and sources read by the module-rule and Scenario-place tests | a few thousand packages and tens of the module's files, unrecorded, treated as bounded | with the dependency tree and the lab's source | test-only, read once per test | `go list -deps -json` decoded one package at a time, each file parsed once; O(packages + files) |
| Scenario names derived, and the declaring pass, per run (`lab.Runner`) | the Scenarios `All` names | with the Scenarios shipped | run-once, before any cluster | Simplest readable form: one derivation per function, repeats found through a set, then each function once over no cluster; O(scenarios × (name length + steps)) |
| A Descheduler policy and the pairing table (`scenario.DeschedulerPolicy`, its encoding) | one profile of two entries shipped; one pairing row per minor; unrecorded, treated as bounded | with the definitions and the minors paired | run-private, built once per Given | Simplest readable form: definition order, the pairing a lookup by minor; O(policy size) |
| Descheduler passes per Scenario (`RunDeschedulerPass`) | one pass per When, or one per interval for the cluster's life, unbounded in count while an Interactive run holds it | with the Whens and intervals written | run-private, one container at a time | Simplest readable form: one container per pass, its engine and API calls constant in count; the log tail read on failure alone, capped in lines, and in bytes for the stream and each frame |
| Pods and Nodes per Placement board (`ObserveEveryDeployment`, `scenario.PlacementBoard`) | two Deployments and 9 Nodes at most among the shipped Scenarios; above that unrecorded, treated as bounded | with the Deployments, Pods and Nodes declared | run-private, one per Wait | One Node list and one label-selected Pod list, sorted, compared to the previous board through a set; O((pods + nodes) log (pods + nodes)) per Wait |
| Steps recorded per Scenario (`lab.Scenario`) | seven in the largest shipped Scenario; above that unrecorded, treated as bounded | with the lines a Scenario holds | run-private, one append per Step | One slice in Step order; O(steps) |
| Timeline entries per Scenario (`Record`, the Wait narratives) | about 150 at most among the shipped Scenarios, plus a note per Step; above that unrecorded, treated as bounded | with the Nodes, Pods and Steps | run-private, one writer | Encoded to the open file on arrival, the Pods read descheduled kept in a set; O(1) average per entry. The narratives are one pass over a map of each object's last reading; O(entries × deployments) time, O(objects) space |
| Watch streams open per Scenario (`Record`) | one per kind recorded | by a reviewed change to the kinds | run-private | One `Watch` per kind merged by one `select`; a closed stream re-opened once from the last version seen |
| Timeline files in the folder | one per Scenario per run, removed by no run; unrecorded, treated as bounded | with the runs a machine has done | written once, read by no run | One file, named for its Simulated Cluster |
| Timeline entries while a cluster is held (`Record`, the held run) | the Steps' entries, then each click's and each pass's; unbounded while held, unrecorded, treated as bounded | with the clicks, the passes and the time held | run-private, one writer | Simplest readable form: as Timeline entries per Scenario; O(1) per entry, O(entries) space |
| Terminal lines while a cluster is held (`scenario.Following`, the held run) | one per click and per moment; unrecorded, treated as bounded | with the clicks, the passes and the time held | run-private, one reader | Simplest readable form: the narratives' reading, a map of each object's last reading; O(1) average per entry, O(objects) space |
| Live view observations (the observer) | one per refresh interval per Interactive run, over 9 Nodes and their Pods | with the Pods and Nodes declared | run-private, one writer, every stream reading | One observation, board and render per interval, a fade count per Pod, swapped in under a lock; O((pods + nodes) log (pods + nodes)), independent of the browsers |
| Live view streams | an engineer's browsers, unrecorded, treated as bounded, held below the open-stream cap | with the browsers open | shared read-mostly | One held connection per stream, woken on each new fragment, writing the latest alone; O(fragment) per change |
| Simulated Clusters at once (`lab.Runner`, the gate) | a run: one per Scenario, an Interactive run one. The gate: one package at a time, the tests its `-parallel` flag admits times each test's clusters. The engine's ceiling unrecorded, treated as bounded | with the Scenarios and the cluster-backed tests | run-private, one container and loopback port per cluster | One goroutine per Scenario, joined before the report. A cluster not started within the start timeout ends its own Scenario or test |
<!-- One row per collection or path that scales with use: rows, requests per second, payload size. Access pattern: request-private, shared read-mostly, shared read-write. Form: the structure or algorithm chosen. On a hot path it carries its time and space bound with its kind (worst-case, average, amortized). A hand-written one, or a measured claim, links its ADR. -->

- **Hot paths:** none. A run's cost is the container start, measured in seconds, and no latency figure is a requirement (G-1).
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

- **Inputs it processes:** the compiled-in Scenarios, the lab's flags, a Scenario name argument and `DOCKER_HOST` when set. It also reads the engine's and API server's responses, and the Live view's HTTP requests on loopback.
- **Outputs it produces:** on stdout the settings it resolved and each Scenario's report, and an Interactive run's Live view. A warning names a `DOCKER_HOST` that resolves nothing. The lab also leaves an exit status, Kubernetes resources in the Simulated Cluster, and one Timeline file per Scenario.
- **External services it connects to:** the local container engine through its HTTP API on a unix socket, and the KWOK API server on a loopback host port. An Interactive run also listens on a loopback port the operating system chooses, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md).
- **Credential handling:** none. The lab reads no kubeconfig, no `KUBERNETES_*` variable, no service-account token and no home-directory file, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md).
- **Runtime:** a developer machine or a build agent, as the invoking user, with a container engine socket that answers.

## Threat Model

| Threat | Attack Vector | Mitigation |
|--------|--------------|------------|
| A run drives a real cluster (NG-1) | An ambient kubeconfig, a `KUBERNETES_*` variable, or an in-cluster service-account token discovered by the client | No configuration input carries a cluster address or a credential. The client is built from the provisioned address alone, held by the lint rules and tests of `docs/security-principles.md` § Language Realization, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md) |
| A third party reaches the unauthenticated API server | The engine publishes the API port on every interface | The API port is published on the loopback address, on a host port the engine chooses, per the same ADR |
| A run drives a remote engine | A `DOCKER_HOST` naming a `tcp://` or `ssh://` endpoint | Only a `unix://` path is ever dialed, parsed as a URL and never interpolated into a command; another scheme is skipped with a warning, per the same ADR
| The engine socket grants control of every container on the machine | The lab holds an API handle the engine cannot scope down | The exported engine methods take a context and a `KubernetesVersion` or `DeschedulerVersion` alone, never an image or a container. Inspect, wait, log, archive and remove act on this Scenario's own handles, and the package declares no list and no by-identifier call, per the same ADR |
| A run pulls or starts an image other than KWOK's cluster image or the paired Descheduler image | The version input interpolated into an image reference | Both repositories are constants no input reaches. The cluster tag joins the constant KWOK release and a version validated to a leading `v` with digits and dots alone, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md). The Descheduler tag is a row of the compiled-in pairing, per [the Descheduler ADR](adr/2026-11-09-the-descheduler-runs-as-its-official-image-paired-by-kubernetes-minor.md). Each container's command is constants alone |
| A moved image tag changes what the Simulated Cluster or the Descheduler runs | The pinned tag is re-pushed upstream | Both images are pinned by tag, per [the version-pin ADR](adr/2026-10-03-kubernetes-version-pin.md); digest pinning is not in force |
| A Simulated Cluster outlives its Scenario | The Scenario fails, or the run exits, before the container is removed | `Provision` returns a handle whose close removes the container, deferred by the runner around each Scenario, per [the engine ADR](adr/2026-10-03-lab-provisioned-simulated-cluster.md). A pass container is removed by its name as its pass ends and again before its cluster, its creation answered or not. A signal removes an Interactive run's held cluster; a forced kill leaves it for the engineer |
| The Descheduler reaches beyond its own Simulated Cluster | Its container joins another network or carries a credential | It shares its cluster's network namespace alone, publishes no port, mounts nothing, drops every capability and gains no privileges. Its kubeconfig names the loopback API port over plain HTTP with no credential, and its health server listens inside that namespace alone, per the Descheduler ADR |
| Another machine reaches the Live view | The server listens on every interface | It listens on the loopback address alone, on a port the operating system chooses, per [the Live view ADR](adr/2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) |
| A web page in the engineer's browser reads the Live view, holds its streams or injects a Failure | A hostile site rebinds its name to the loopback address, opens streams to it, or posts to its write | A request whose Host is not the address served, or carrying another Origin, is refused, and the write passes `http.CrossOriginProtection`. No response grants cross-origin access, per the same ADR |
| A request changes the Simulated Cluster beyond one Failure, or ties up the server | A crafted method, path or name, a slow client, or many streams | Only the reads and the one write are served. The write's name resolves against the Topology or the latest board, so another name changes nothing, and the terminal prints the resolved name alone. A local process reaches the write as it reaches the API server. Server timeouts bound every connection and event write, and a stream beyond the cap is refused, per the same ADR |
| The browser opener is handed something other than the Live view's address | A line of the lab's output carrying a Scenario, Deployment or Pod name read as the address | `make demo` hands on only the text after the Live view label, on a line starting with it and naming `http://127.0.0.1:` and a digit port. Any other line is printed alone. The command tests assert the line's shape (§ Contracts) |
| Markup injected into the Live view | A Scenario, Deployment, NodePool, Pod name or label rendered as HTML | The page and the board are rendered through `html/template`, which escapes every name, and the page loads only its own embedded script and stylesheet, per the same ADR |
| A wedged control-plane request keeps a Simulated Cluster past its Scenario | The API server stops answering a request the run waits on | Every request, a watch's opening included, ends at `controlPlaneTimeout`. A watch's stream ends with its recording, stopped before the cluster is removed, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) |
| A concurrent invocation's cluster is removed | Two runs on one machine naming the same container | Each name carries a random suffix drawn per provisioning, each port is the engine's choice, and a run removes only the container its own handle names, per the same ADR |
| A Timeline is written outside its folder | A Scenario name interpolated into a path | A file name joins the lab's constant prefix, a Scenario name of lowercase ASCII letters, digits and hyphens from a compiled-in function's name, and a random suffix, as one path element. `internal/timeline` refuses a separator or a parent reference in it, per [the Timeline ADR](adr/2026-10-05-timeline-from-the-watch-stream.md) and [the Scenario ADR](adr/2026-10-03-scenarios-as-go-functions.md) |
| A filed Timeline exposes what a run did to another user | The file outlives the run, in the working directory | A created Timeline folder and file are readable and writable by the invoking user alone, and hold only simulated objects of a cluster with no credential, per the same ADR |

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
| REQ-SIM-028 | A Failure injected from the Live view | REQ-SIM-008, REQ-SIM-022 |
| REQ-SIM-029 | A stopped Node or Zone responding again | REQ-SIM-008, REQ-SIM-018, REQ-SIM-021, REQ-SIM-027, REQ-SIM-028 |
| REQ-SIM-030 | Losing Pods of a Deployment | REQ-SIM-021, REQ-SIM-027, REQ-SIM-028, REQ-SIM-029 |
| REQ-SIM-031 | A Descheduler declared and run from a Scenario | REQ-SIM-016, REQ-SIM-021, REQ-SIM-029 |
| REQ-SIM-032 | The Live view opened in one command | REQ-SIM-022, REQ-SIM-031 |
| REQ-SIM-033 | A held cluster followed in its Timeline | REQ-SIM-005, REQ-SIM-028, REQ-SIM-031 |

## State Machine

A run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | `--k8s-version` is parsed, or the default version applies | version resolved |
| version resolved | `DOCKER_HOST` is not an answering `unix://` path | warned and skipped, candidates probed |
| version resolved | `DetectEngine` resolves the engine socket | engine resolved |
| engine resolved | a Scenario's function names no condition, or two derive one Scenario name | run stopped before any cluster, each function named |
| engine resolved | `ObtainVersion` finds the version's image held, or pulls it once, and `ObtainDescheduler` the paired one for a declared policy | version obtained |
| engine resolved | the version's image is absent and cannot be pulled | run stopped before any Scenario, the version named |
| engine resolved | the paired Descheduler image cannot be obtained | each Scenario declaring a policy ended at its Descheduler Given, naming why; the others run on |
| version obtained | `Provision` starts a Scenario's container and its readiness probe answers | cluster ready |
| version obtained | the engine cannot start a Scenario's container | that Scenario ended, naming why; the others run on |
| cluster ready | `Record` opens its watch streams, and the Timeline file opens or its path and reason are kept | recording |
| recording | `ApplyTopology` creates every Node with a Ready status | Nodes Ready |
| Nodes Ready | the scheduler places one probe Pod on every Node, and the probe is removed | Topology provisioned |
| Nodes Ready | a Node holds no probe Pod within the start timeout | provisioning failed, that Node named |
| Topology provisioned | a Wait has `Deploy` create the declared Deployment | Deployment created |
| Deployment created | a Wait finds two agreeing observations of every declared Pod, Ready unless it waits for the Placement alone | Placement Settled |
| Placement Settled | a When stops Nodes, makes them respond again, loses Pods, or runs a Descheduler pass that exits zero | Deployment created, the observation discarded |
| Placement Settled | a When's Descheduler pass fails or outlives `scenario.WaitingTime` | that Scenario ended, its pass container removed |
| Topology provisioned | a Wait applies a Descheduler Given stating an interval | its interval running, one pass at a time, beside every later state |
| its interval running | a pass on it fails or outlives `scenario.WaitingTime` | the interval stopped, its Scenario ended at its next Step or its end |
| Placement Settled | the Scenario ends, passed or failed | record stopped, each Wait given its narrative and the file closed |
| record stopped | the handle closes | the interval joined, any pass container removed by its name, then the cluster's |
| container removed | every Scenario of the run has ended | Outcomes reported in declared order |
| any state | close, or the process exits | container removed |

An Interactive run's lifecycle:

| From | Event | To |
|------|-------|----|
| no cluster | the arguments name no Scenario, an unknown one, or more than one | stopped before the engine, every Scenario name named |
| no cluster | the engine resolves and the version is obtained | the Live view listening, its address named |
| no cluster | no engine resolves, or the version cannot be obtained | stopped before the Live view, naming why |
| the Live view listening | the Scenario's cluster is provisioned and recording | Steps running, observed |
| Steps running, observed | its first When is reached, or its Steps end | the Outcome reported with no Timeline line, the cluster held and recording |
| Steps running, observed | a box is clicked | Steps running, observed, nothing changed |
| the cluster held | a Node's, a Zone's or a placed Pod's box is clicked | the cluster held, the click's When noted and on the terminal, its Failure injected or its Nodes responding again |
| the cluster held | the cluster changes after a click or a Descheduler pass | the cluster held, each change recorded and each complete moment on the terminal |
| the cluster held | the engineer stops the lab | record stopped, the Timeline's line printed, container removed, server shut down |
| Steps running, observed | the engineer stops the lab | the Outcome and the Timeline's line printed, container removed, server shut down |

A Scenario's Steps:

| From | Event | To |
|------|-------|----|
| declared | a Given constructs its Topology or its Deployment | declared, that Step recorded |
| declared | a constructor refuses a Given, or the cluster refuses what a Wait applies | ended, that Step named with the declaration |
| declared | a Wait finds its condition met | Placement observed, Pods Pending or not, the Wait's line naming its time |
| declared | the lab's own waiting time outlasts a Wait | ended, that Wait named with that time and what was not Ready |
| declared | a Wait states a time of none or above the lab's waiting time, or is reached after it | ended, that Wait named with the time |
| declared, or Placement observed | a Then or a Failure with no Wait before it, or a When or a Then over Nodes, Zones, labels, Pods or a Floor the Scenario does not hold | ended, that Step named |
| Placement observed | a When stops its own Nodes responding, makes the stopped Nodes respond again, or loses Pods of a Deployment | declared, the observation discarded |
| Placement observed | a Then the Placement meets, or does not meet | Placement observed, that Step or a Finding recorded |
| ended | a later Step is reached | ended, nothing carried out or reported |

A Pod's phase, as observed:

| From | Event | To |
|------|-------|----|
| absent | the Deployment's replicas are created | Pending |
| Pending | the scheduler binds the Pod to a Node | Pending on that Node, inside its Startup time |
| Pending on that Node | its Startup time passes | Running and Ready |
| Pending | no Node satisfies the specification or the capacity | Pending, reported unplaceable (REQ-SIM-001 edge case 1) |
| Running | its Node stops responding and the waiting time passes | deleted by the control plane, replaced on a Node still responding |
| Pending on a Node, or Running | a Pod Failure, clicked or a When's, loses it | deleted at once, replaced by its ReplicaSet |
