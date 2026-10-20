# Scenarios

The lab's experiments. A Scenario is one exported Go function over `*lab.Scenario` whose every line is one Step, and every one of them sits in `scenarios.go`. Nothing of the machinery that runs them lives here: the Step surface and the runner are `internal/lab`, with what a Pod carries in `internal/lab/pod` and what a Node is in `internal/lab/node`.

`scenarios.go` opens with `All`, the list of the Scenarios a run carries out, and holds every Scenario after it in the order `All` names them. These files sit beside it. `workloads.go` holds the names of the Workloads they deploy, one constant each. `labels.go` holds the keys of the Node labels their NodePools state, one string constant each. `taints.go` holds the keys and values of the taints their NodePools state, one string constant each. `nodes.go` holds the Named Node capacities they name, one function each, defined once with `node.Named`. `pods.go` holds the predefined Pod specifications they declare, one function each, defined once with `pod.Predefined`, and the two Named settings they name: `LeaveUnresponsiveNodes`, defined once with `pod.Settings`, and `EvenSpreadOver`, the Even spread. The lab defines no Node capacity, no Pod specification and no Named setting of its own.

Run them all with `make run`, or `go run ./cmd/lab`. The lab's flags pass through as `make run ARGS="--whole-timeline"`.

## How one works

A Scenario writes these kinds of Step:

- A **Given** declares the Topology or the Workload, and reaches no cluster.
- A **When** injects the Failure.
- A **Wait** applies what the Givens above it declared, holds until the cluster reports the state it names, and reports the time it took where it was reached and the time it was held to where it was not.
- A **Then** judges the Placement the last Wait reached, and records a Finding while the later Steps still run.

A Then and a Failure both read the Placement the last Wait reached, and a Failure discards it. So a Wait stands before the first Then, and another between a Failure and the Then after it. A Scenario missing either ends at that Step rather than judging a world that is still changing.

A Wait is written as a Step of its own, `s.WaitFor().AllPodsReady()`, or on the Given whose declaration it fits, `s.Given().Topology(…).WaitFor().AllNodesReady()`. Both forms are a Wait Step. The Scenarios here write the standalone one.

## The vocabulary a Scenario writes

Every name a Scenario writes reads by its subject. The Scenario's own is under `lab`, what a Pod carries under `pod`, and what a Node is under `node`. Each package stays the authority on what its names do, except the functions of `nodes.go` and `pods.go`, which the Scenarios define. A Scenario imports the three packages by name, never with a dot.

A setting of several values is a keyed literal under Kubernetes' names, such as `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}`. Every field the lab offers is required, except where Kubernetes ties a field to another. A toleration's `Value` follows its operator, which `pod.Equal` takes and `pod.Exists` does not. Its `TolerationSeconds` follows its effect, which `pod.NoExecute` takes and `pod.NoSchedule` does not. A field stated against them is refused as one left out is. A field left out holds its zero value, and the setting is refused at the Given declaring it: a Pod setting at the Workload's Given, naming the setting and the field, and a Node capacity at the Topology's Given.

**Givens**

- `Given().Topology(lab.Zones, node.Pool(node.PoolName, node.PerZone, capacity), …)`, each NodePool naming the label its Nodes carry and the room each of its Nodes has: `StandardNodes()` or another function of `nodes.go` naming a Named Node capacity, or `node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}` stating cpu in whole cores, memory and the Pods a Node has room for, all three. Either way, the Topology's Given line reports the cpu, memory and Pod count, after the name where one is named. Zones holding Nodes of different kinds take one NodePool each. A Scenario of one NodePool names it general
- `.Labels(node.Labels{CapacityType: "spot"})` on a NodePool — the labels its Nodes carry beside the lab's own, written more than once if need be. The Given line reports them after the capacity, as `labelled capacity-type=spot`. A key the lab writes on every Node or owns under its prefix `k8s-resilience-lab/`, a key or value Kubernetes cannot hold as a label, and a key stated twice are refused at the Topology's Given
- `.Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule})` on a NodePool — the taints its Nodes carry apart from their labels, written more than once if need be. `node.NoSchedule` is the one effect a NodePool states. The Given line reports them after the labels, as `tainted nvidia.com/gpu=present:NoSchedule`. Another effect, a field left out, a key or value Kubernetes cannot hold, a key under `node.kubernetes.io/`, and a key stated twice are refused at the Topology's Given
- `Given().Workload(name, lab.Pods, specification)`, with `.RequiredCapacity(lab.Pods)` for the fewest Pods it keeps Running through a Failure; `lab.Pods` counts a Workload's Pods, and the `Pods` field of `node.Capacity` a Node's room. A later Workload Given is the Scenario's next Workload: the next Wait deploys it beside the earlier ones, which stay deployed, and every Step after it reads it alone. A Workload name the Scenario already declared is refused at its Given
- the Workload constants of `workloads.go`, business capabilities of the domain the Scenarios model
- the label keys of `labels.go`, each an untyped string constant of the Scenarios' own: `CapacityType`, marking a Node on-demand or spot
- the taint keys and values of `taints.go`, each an untyped string constant of the Scenarios' own: `GPU`, marking a Node holding a gpu, and `GPUPresent`, its value
- the Named Node capacity functions of `nodes.go`, each `node.Named(name, node.Capacity{…})` under a name of the Scenarios' own; a new one is a function there, never a call in a Scenario

**Pod specifications**

- `ResilientPlacement()` of `pods.go` — its Pods spread as evenly over the Zones and over the Nodes as the control plane can place them, by `EvenSpreadOver(node.Zone)` and `EvenSpreadOver(node.Hostname)`, each starting in 500ms to 1s
- `SpreadOverOnDemandAndSpot()` of `pods.go` — its Pods spread as evenly over the Zones and over the values of `CapacityType` as the control plane can place them, by `EvenSpreadOver(node.Zone)` and `EvenSpreadOver(CapacityType)`, each starting in 500ms to 1s
- `DefaultPlacement()` of `pods.go` — no spread, so where its Pods land is the scheduler's own choice, each starting in 500ms to 1s
- `ToleratingGPUNodes()` of `pods.go` — its Pods tolerate the `GPU` taint and ask for more than a Standard Node holds, so only a GPU Node has room for one, each starting in 500ms to 1s
- the specification functions of `pods.go`, each `pod.Predefined(name, startup, settings…)`: a name of the Scenarios' own, the Startup time its Pods take in one of the four forms below, then its settings. Each names `LeaveUnresponsiveNodes()` first and writes neither of the lab's two tolerations itself; the lab holds both at its waiting time whatever is written. A new specification is a function there, never a call in a Scenario
- `LeaveUnresponsiveNodes()` of `pods.go` — a Named setting, defined beside the specifications that name it, never by the lab: its Pods leave a Node that stopped responding after the lab's waiting time. It stands for exactly two tolerations under their Kubernetes names, `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}` and the same for `node.Unreachable`, grouped by `pod.Settings(…)`, the lab's only means to group settings under a name; it takes settings alone, so a Startup time, a full access or another group does not compile inside it
- `EvenSpreadOver(topologyKey)` of `pods.go` — the Even spread, a Named setting holding one `pod.TopologySpreadConstraint` over the label given, at a `MaxSkew` of one, `pod.DoNotSchedule` and `pod.Honor`. A Scenario wanting another skew, or a Pod placed past it, writes the literal itself
- the Workload's Given line, which reports under it every setting its Pods carry, one line each as a manifest writes it, a Named setting as the settings it stands for and a full access as one line saying it applies
- `pod.TopologySpreadConstraint{…}`, `pod.Request{CPU: 3, Memory: 12 * pod.Gi}` and `pod.Toleration{…}` — the settings a definition sets and a Customization changes, with the values `pod.DoNotSchedule`, `pod.Honor`, `pod.Exists`, `pod.Equal`, `pod.NoExecute` and `pod.NoSchedule`, the labels `node.Zone` and `node.Hostname`, the taints `node.NotReady` and `node.Unreachable`, and the lab's waiting time `node.UnresponsiveWait` in whole seconds
- `.Customize(…)` on a specification — that specification with the lab's own settings changed
- `pod.FullAccess(…)` — the Customization carrying a Scenario's own change to the Kubernetes Pod specification
- `pod.StartupSeconds(n)` — the Startup time of whole seconds each Pod takes to start, which a definition names and a Customization may state again; the Workload's Given line reports it after the specification's name
- `pod.StartupSecondsBetween(shortest, longest)` — the Startup time stating that each Pod takes a time between the two, in whole seconds
- `pod.StartupMillis(n)` — the Startup time stating the whole milliseconds each of its Pods takes to start
- `pod.StartupMillisBetween(shortest, longest)` — the Startup time stating that each Pod takes a time between the two, in whole milliseconds

**Failures**

- `When().NodesFail(lab.Nodes)` — that many Nodes stop responding
- `When().ZonesFail(lab.Zones)` — every Node of that many Zones stops responding at once
- `When().NodesLabelledFail(node.Labels{key: value})` — every Node carrying that one label a NodePool states stops responding at once; a key or value no NodePool states ends the Scenario at this When

**Waits**

- `WaitFor().AllNodesReady()` and `WaitFor().AllNodesReadyWithinSeconds(seconds)`
- `WaitFor().AllPodsReady()` and `WaitFor().AllPodsReadyWithinSeconds(seconds)`
- `WaitFor().PlacementSettled()` and `WaitFor().PlacementSettledWithinSeconds(seconds)` — the Placement alone, so it is reached while Pods stay Pending
- the first four on the Given that declares the Topology or the Workload

A Wait stating no time is held to the lab's own waiting time, which every run reports at its start. A stated time is marked as stated on the line, so it is never read for the time the Wait took.

**Thens**

- `Then().AllPodsRunning()`
- `Then().PodsPerZone(pods)`
- `Then().PodsPerNode(pods)`
- `Then().PodsPerValueOf(key, pods)` — every value the NodePools state under that label holds that many Pods, a value holding none counted as zero; a Pod on a Node stating no value counts toward none, and a key no NodePool states ends the Scenario at this Then
- `Then().PodsOnNodePool(node.PoolName, pods)` — the Nodes of that NodePool hold that many Pods between them; a NodePool the Topology lacks ends the Scenario at this Then
- `Then().PodsPending(pods)` — how many Pods the Placement leaves Pending, which is what no Node had room for
- `Then().NoPodsOnTheFailedNodes()`
- `Then().NoPodsInTheFailedZones()`
- `Then().RequiredCapacityHeld()` — the declared Required Capacity, judged at the moment of the Failure and once the Placement has settled

## The Scenarios

- **a-workload-placed-for-resilience** — the lab's simplest Scenario and its starting point. A Workload declares Resilient placement on Nodes with room for more than one of its Pods, so the even Placement is the scheduler honouring that specification.
- **a-node-that-stops-responding** — the Resilient placement world, and then one Node stops responding: what the control plane restores, and the capacity held through it.
- **a-zone-that-stops-responding** — the same world, and then every Node of one Zone stops responding at once: the capacity surviving a lost Zone.
- **no-room-after-a-zone-stops-responding** — the lost Zone again, on Nodes with room for one Pod each: six of the nine Pods are Running, the three with nowhere to go stay Pending, and the declared Required Capacity of six is still held.
- **a-workload-spread-over-on-demand-and-spot** — three Zones, each holding an on-demand and a spot NodePool of one Standard Node, labelled by capacity type: six Pods spread evenly over both, two per Zone and three per capacity type.
- **spot-nodes-that-stop-responding** — three Zones, each holding two on-demand Nodes and one spot Node under Resilient placement, then every spot Node stops responding at once: the nine Pods are restored on the on-demand Nodes, three per Zone, and the Required Capacity of six is held.
- **general-pods-beside-tainted-gpu-nodes** — three Zones, each holding two general Nodes and one larger gpu Node tainted NoSchedule: six Pods under Resilient placement all run on the general Nodes, then three Pods tolerating the taint run on the gpu Nodes, drawn there by a Request only a gpu Node has room for.

A Scenario's Steps are read in `scenarios.go`.

## Writing one

The function is named for the condition it runs under and the outcome it asserts, `<Condition>_Should<Outcome>`.

```go
func AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNodes()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement())
	s.WaitFor().AllPodsReady()
	s.Then().PodsPerZone(3)
}
```

Every line is one Step opened on `s`, so a reader scans the Step kinds in one column and a line is inserted, removed or reordered on its own. The Scenarios here are held to that layout by a rule test, per [the layout ADR](../docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md).

The Scenario runs under its Scenario name, the words of the condition before `_Should` in lowercase joined by hyphens, so the function above runs as `a-workload-placed-for-resilience` and renaming the function renames the Scenario. A run of capitals is one word, so `GPUNodes` reads `gpu-nodes`. A condition holding anything but ASCII letters and digits, and two Scenarios naming one condition, are refused before any Simulated Cluster is provisioned. A new Scenario is a function in `scenarios.go`, after the last one, and one line at the end of `All`.

A Scenario's mandatory values are passed as named arguments — `lab.Zones`, `node.PerZone`, `lab.Pods`, `lab.WorkloadName`, `node.PoolName`, the NodePool's capacity and the specification — so a swapped or missing argument does not compile. A setting's values stand beside their Kubernetes field names, so none is read by its position. An optional setting is a further call on the complete declaration, `.RequiredCapacity` on the Workload and `.Labels` on a NodePool.

A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its own Then Steps. The lab's Scenarios run in `cmd/lab`'s own suite, which fails when one reports a Finding or ends at a Step.
