# Scenarios

The lab's experiments. A Scenario is one exported Go function over `*lab.Scenario` whose every line is one Step, and every one of them sits in `scenarios.go`. Nothing of the machinery that runs them lives here: the Step surface and the runner are `internal/lab`, with what a Pod carries in `internal/lab/pod` and what a Node is in `internal/lab/node`.

`scenarios.go` opens with `All`, the list of the Scenarios a run carries out, and holds every Scenario after it in the order `All` names them. These files sit beside it. `deployments.go` holds the names of the Deployments they deploy, one constant each. `labels.go` holds the keys of the Node labels their NodePools state, one string constant each. `taints.go` holds the keys and values of the taints their NodePools state, one string constant each. `nodes.go` holds the Named Node capacities they name, one function each, defined once with `node.Named`. `pods.go` holds the predefined Pod specifications they declare, one function each, defined once with `pod.Predefined`, and the Named settings they name: `LeaveUnresponsiveNodes`, `StandardPod` and `GPUPod`, each defined once with `pod.Settings`, and `EvenSpreadOver`, the Even spread. `descheduler.go` holds the Descheduler policies they declare, one function each returning one `descheduler.Policy` literal. The lab defines no Node capacity, no Pod specification, no Named setting and no Descheduler policy of its own.

Run them all with `make run`, or `go run ./cmd/lab`. The lab's flags pass through as `make run ARGS="--whole-timeline"`.

## How one works

A Scenario writes these kinds of Step:

- A **Given** declares the Topology or the Deployment, and reaches no cluster.
- A **When** injects the Failure.
- A **Wait** applies what the Givens above it declared, holds until the cluster reports the state it names, and reports the time it took where it was reached and the time it was held to where it was not. Reached or not, it is followed in the report by a Placement board of every Node and every Deployment's Pods, and a second separator stands between that board and the Thens judging it.
- A **Then** judges the Placement the last Wait reached, and records a Finding while the later Steps still run.

A Then and a Failure both read the Placement the last Wait reached, and a Failure discards it. So a Wait stands before the first Then, and another between a Failure and the Then after it. A Scenario missing either ends at that Step rather than judging a world that is still changing.

A Wait is written as a Step of its own, `s.WaitFor().AllPodsReady()`, or on the Given whose declaration it fits, `s.Given().Zones(…).WaitFor().AllNodesReady()`. Both forms are a Wait Step. The Scenarios here write the standalone one.

## The vocabulary a Scenario writes

Every name a Scenario writes reads by its subject. The Scenario's own is under `lab`, what a Pod carries under `pod`, and what a Node is under `node`. Each package stays the authority on what its names do, except the functions of `nodes.go` and `pods.go`, which the Scenarios define. A Scenario imports the three packages by name, never with a dot.

A setting of several values is a keyed literal under Kubernetes' names, such as `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}`. Every field the lab offers is required, except where Kubernetes ties a field to another. A toleration's `Value` follows its operator, which `pod.Equal` takes and `pod.Exists` does not. Its `TolerationSeconds` follows its effect, which `pod.NoExecute` takes and `pod.NoSchedule` does not. A field stated against them is refused as one left out is. A field left out holds its zero value, and the setting is refused at the Given declaring it: a Pod setting at the Deployment's Given, naming the setting and the field, and a Node capacity at the Topology's Given.

**Givens**

- `Given().Zones(n, node.Pool(node.PoolName, node.PerZone, capacity), …)`, the Topology of n Zones, each NodePool naming the label its Nodes carry and the room each of its Nodes has: `StandardNode()` or another function of `nodes.go` naming a Named Node capacity, or `node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}` stating cpu in whole cores, memory and the Pods a Node has room for, all three. Either way, the Topology's Given line reports the cpu, memory and Pod count, after the name where one is named. Zones holding Nodes of different kinds take one NodePool each. A Scenario of one NodePool names it general
- `.Labels(node.Labels{CapacityType: "spot"})` on a NodePool — the labels its Nodes carry beside the lab's own, written more than once if need be. The Given line reports them after the capacity, as `labelled capacity-type=spot`. A key the lab writes on every Node or owns under its prefix `k8s-resilience-lab/`, a key or value Kubernetes cannot hold as a label, and a key stated twice are refused at the Topology's Given
- `.Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule})` on a NodePool — the taints its Nodes carry apart from their labels, written more than once if need be. `node.NoSchedule` is the one effect a NodePool states. The Given line reports them after the labels, as `tainted nvidia.com/gpu=present:NoSchedule`. Another effect, a field left out, a key or value Kubernetes cannot hold, a key under `node.kubernetes.io/`, and a key stated twice are refused at the Topology's Given
- `Given().Deployment(name, n, request, specification)`, the request being `StandardPod()`, `GPUPod()` or a `pod.Request{…}` of the line's own, applied after the specification so it stands over any Request there. `pod.Settings()` in its place asks for no cpu and no memory. With `.SurvivesLosing(lab.Zones(n))` or `.SurvivesLosing(lab.Nodes(n))` for its Failure tolerance, the Zones or Nodes it survives losing to a Failure: zero or more, and fewer than the Topology declared above it holds, refused at its Given otherwise. It sets the Deployment's Floor, its replicas less the most Pods those Zones or Nodes hold under an even spread, and scales nothing. Its replicas and the `Pods` field of `node.Capacity`, a Node's room, are plain ints. A later Deployment Given is the Scenario's next Deployment: the next Wait deploys it beside the earlier ones, which stay deployed, and every Step after it reads it alone. A Deployment name the Scenario already declared is refused at its Given
- the Deployment constants of `deployments.go`, business capabilities of the domain the Scenarios model
- the label keys of `labels.go`, each an untyped string constant of the Scenarios' own: `CapacityType`, marking a Node on-demand or spot
- the taint keys and values of `taints.go`, each an untyped string constant of the Scenarios' own: `GPU`, marking a Node holding a gpu, and `GPUPresent`, its value
- the Named Node capacity functions of `nodes.go`, each `node.Named(name, node.Capacity{…})` under a name of the Scenarios' own; a new one is a function there, never a call in a Scenario
- `Given().Descheduler(EvenSpreadRestored())`, the Scenario's one Descheduler policy, run by the Descheduler paired with the run's Kubernetes version. Its line names the policy by its profile names and that Descheduler. Beneath stands one line per limit, profile, configuration entry and extension point under the policy file's keys, such as `pluginConfig: DefaultEvictor {nodeFit: true}` and `balance: [RemovePodsViolatingTopologySpreadConstraint]`. A policy of no profile, a profile unnamed or enabling no plugin, an entry holding another plugin's `Args`, and a second policy are refused at its Given
- `.EverySeconds(n)` on the Descheduler's Given — a pass under its policy on each interval of n whole seconds, from the Wait applying the Given until the Scenario ends, and the line names the interval, as `a pass every 5 seconds`. The next pass starts an interval after the last began, or as it exits where it outlasted the interval, so two never run at once. Each Wait's narrative names the Pods the passes before it evicted, as a When's pass does. An interval of no seconds or fewer is refused at its Given, and a pass that cannot run or does not end stops the interval and ends the Scenario at its next Step, or at this Given where none follows. An Interactive run keeps the passes running while it holds the cluster
- the Descheduler policy functions of `descheduler.go`, each returning one keyed `descheduler.Policy` literal written line for line like the Descheduler's `descheduler/v1alpha2` policy file: `MaxNoOfPodsToEvictPerNode`, `MaxNoOfPodsToEvictTotal`, `Profiles`, each `Profile`'s `Name`, `PluginConfig` entries with their plugin's typed `Args`, and `Plugins` with a `PluginSet{Enabled: …}` per extension point. A field left out stays out, so the Descheduler's own default applies, and the lab adds no entry and enables no plugin itself. `EvenSpreadRestored()` evicts at most one Pod per pass, which keeps a Floor while each replacement turns Ready within an interval; the Floor's Then remains the check, configures `DefaultEvictor` with `NodeFit` and `RemovePodsViolatingTopologySpreadConstraint` with the `DoNotSchedule` constraints, and enables the latter at `Balance`. A function returning one `descheduler.PluginConfig` literal names a group of the Scenarios' own, once an entry repeats

**Pod specifications**

- `ResilientPlacement()` of `pods.go` — its Pods spread as evenly over the Zones and over the Nodes as the control plane can place them, by `EvenSpreadOver(node.Zone)` and `EvenSpreadOver(node.Hostname)`, each starting in 500ms to 1s
- `SpreadOverOnDemandAndSpot()` of `pods.go` — its Pods spread as evenly over the Zones and over the values of `CapacityType` as the control plane can place them, by `EvenSpreadOver(node.Zone)` and `EvenSpreadOver(CapacityType)`, each starting in 500ms to 1s
- `DefaultPlacement()` of `pods.go` — no spread, so where its Pods land is the scheduler's own choice, each starting in 500ms to 1s
- `ToleratingGPUNodes()` of `pods.go` — its Pods tolerate the `GPU` taint, which permits them on a GPU Node, each starting in 500ms to 1s
- the specification functions of `pods.go`, each `pod.Predefined(name, startup, settings…)`: a name of the Scenarios' own, the Startup time its Pods take in one of the four forms below, then its settings. Each names `LeaveUnresponsiveNodes()` first and writes neither of the lab's two tolerations itself; the lab holds both at its waiting time whatever is written. No specification holds a Request, which the Deployment line names. A new specification is a function there, never a call in a Scenario
- `LeaveUnresponsiveNodes()` of `pods.go` — a Named setting, defined beside the specifications that name it, never by the lab: its Pods leave a Node that stopped responding after the lab's waiting time. It stands for exactly two tolerations under their Kubernetes names, `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}` and the same for `node.Unreachable`, grouped by `pod.Settings(…)`, the lab's only means to group settings under a name; it takes settings alone, so a Startup time, a full access or another group does not compile inside it
- `StandardPod()` and `GPUPod()` of `pods.go` — Named settings, each standing for exactly one `pod.Request` grouped by `pod.Settings(…)`: a quarter of a Standard Node, 4 cpu and 16Gi, and more than a Standard Node holds, 24 cpu and 96Gi, which only a GPU Node has room for
- `EvenSpreadOver(topologyKey)` of `pods.go` — the Even spread, a Named setting holding one `pod.TopologySpreadConstraint` over the label given, at a `MaxSkew` of one, `pod.DoNotSchedule` and `pod.Honor`. A Scenario wanting another skew, or a Pod placed past it, writes the literal itself
- the Deployment's Given line, which reports under it every setting its Pods carry, one line each as a manifest writes it, the line's Request last, a Named setting as the settings it stands for and a full access as one line saying it applies
- `pod.TopologySpreadConstraint{…}`, `pod.Request{CPU: 3, Memory: 12 * pod.Gi}` and `pod.Toleration{…}` — the settings a definition sets and a Customization changes, with the values `pod.DoNotSchedule`, `pod.Honor`, `pod.Exists`, `pod.Equal`, `pod.NoExecute` and `pod.NoSchedule`, the labels `node.Zone` and `node.Hostname`, the taints `node.NotReady` and `node.Unreachable`, and the lab's waiting time `node.UnresponsiveWait` in whole seconds
- `.Customize(…)` on a specification — that specification with the lab's own settings changed
- `pod.FullAccess(…)` — the Customization carrying a Scenario's own change to the Kubernetes Pod specification
- `pod.StartupSeconds(n)` — the Startup time of whole seconds each Pod takes to start, which a definition names and a Customization may state again; the Deployment's Given line reports it after the specification's name
- `pod.StartupSecondsBetween(shortest, longest)` — the Startup time stating that each Pod takes a time between the two, in whole seconds
- `pod.StartupMillis(n)` — the Startup time stating the whole milliseconds each of its Pods takes to start
- `pod.StartupMillisBetween(shortest, longest)` — the Startup time stating that each Pod takes a time between the two, in whole milliseconds

**Whens**

- `When().NodesFail(n)` — n Nodes stop responding
- `When().ZonesFail(n)` — every Node of n Zones stops responding at once
- `When().NodesLabelledFail(node.Labels{key: value})` — every Node carrying that one label a NodePool states stops responding at once; a key or value no NodePool states ends the Scenario at this When
- `When().NodesRespondAgain()` — every Node stopped so far responds again, whichever When stopped it, and the line names each Node; no Node stopped ends the Scenario at this When
- `When().ZonesRespondAgain()` — every Node stopped so far responds again, and the line names each Zone; no Node stopped, or a stopped Node sharing its Zone with a Node still responding, ends the Scenario at this When
- `When().PodsFail(deployment, n)` — n placed Pods of the named Deployment are lost in one Step, the first the last Wait's board lists, and the line names each; its ReplicaSet replaces them. A Deployment the Scenario has not declared, one declaring a Failure tolerance, a count below one or above its placed Pods, and a picked Pod already gone end the Scenario at this When

- `When().DeschedulerRunsOnce()` — one Descheduler pass under the policy the Scenario declared, and the line names it. The next Step starts once the pass has ended, and the next Wait's narrative names each Pod it evicted, `evicted from <Node> by the Descheduler`, apart from a Failure's. A Scenario declaring no policy, and a pass that cannot run or does not end within the lab's waiting time, end the Scenario at this When, the latter with the pass's last log lines. So does a Descheduler running on an interval

A Node that responds again stays among the Nodes and Zones that failed, so `NoPodsOnTheFailedNodes` and `NoPodsInTheFailedZones` still judge it.

**Waits**

- `WaitFor().AllNodesReady()` and `WaitFor().AllNodesReadyWithinSeconds(seconds)`
- `WaitFor().AllPodsReady()` and `WaitFor().AllPodsReadyWithinSeconds(seconds)`
- `WaitFor().PlacementSettled()` and `WaitFor().PlacementSettledWithinSeconds(seconds)` — the Placement alone, so it is reached while Pods stay Pending, whatever a Descheduler on an interval does
- `WaitFor().DeschedulerPassEvictingNothing()` — the first pass on the Descheduler's interval that began after this Wait did and evicted nothing, then the Settled Placement; a pass evicting nothing before the Wait does not end it. It states no time, and a Scenario whose Descheduler runs on no interval ends at it
- the first four on the Given that declares the Topology or the Deployment

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
- `Then().EnoughPodsStayedReady()` — every Deployment declaring a Failure tolerance kept its Floor of Ready Pods at every moment from the first Failure to this Then, read off the Timeline rather than the Settled Placement. A Pod counts while Ready, as the lab reads it from its Running entry, and one on a Node a When stopped is lost from that When until a When makes the Node respond again, though the control plane still reports it. One line beneath the Then per judged Deployment states whether it kept or fell below its Floor and its lowest Ready count, when and for how long. Each Deployment falling below records a Finding naming how low, from when and for how long, the Pods lost and the Nodes they were on. No Deployment declaring a Failure tolerance, or no When before it, ends the Scenario at this Then

## The Scenarios

- **a-deployment-placed-for-resilience** — the lab's simplest Scenario and its starting point. A Deployment declares Resilient placement on Nodes with room for more than one of its Pods, so the even Placement is the scheduler honouring that specification.
- **a-node-that-stops-responding** — the Resilient placement world, and then one Node stops responding: what the control plane restores, and the capacity held through it.
- **a-zone-that-stops-responding** — the same world, and then every Node of one Zone stops responding at once: the capacity surviving a lost Zone.
- **no-room-after-a-zone-stops-responding** — the lost Zone again, on Nodes with room for one Pod each: six of the nine Pods are Running, the three with nowhere to go stay Pending, and the Deployment keeps its Floor of six Ready Pods through the lost Zone.
- **a-deployment-spread-over-on-demand-and-spot** — three Zones, each holding an on-demand and a spot NodePool of one Standard Node, labelled by capacity type: six Pods spread evenly over both, two per Zone and three per capacity type.
- **spot-nodes-that-stop-responding** — three Zones, each holding two on-demand Nodes and one spot Node under Resilient placement, then every spot Node stops responding at once: the nine Pods are restored on the on-demand Nodes, three per Zone, and the Deployment keeps its Floor of six Ready Pods through the three spot Nodes it survives losing.
- **general-pods-beside-tainted-gpu-nodes** — three Zones, each holding two general Nodes and one larger gpu Node tainted NoSchedule: six Pods under Resilient placement all run on the general Nodes, then three Pods tolerating the taint run on the gpu Nodes, drawn there by GPU Pods, a Request only a gpu Node has room for.
- **a-node-that-responds-again** — the Resilient placement world, one Node stops responding and then responds again: the control plane never moves a running Pod, so that Node stays empty, every Zone still holds three Pods, and one Node holds two. It declares that it survives losing one Node, and the Floor of eight Ready Pods held throughout.
- **a-zone-that-responds-again** — the same world, every Node of one Zone stops responding and then responds again: that Zone stays empty while the nine Pods stay Running on the other Zones. It declares that it survives losing one Zone, and the Floor of six Ready Pods held throughout.
- **a-zone-that-responds-again-under-a-descheduler** — a-zone-that-responds-again's twin: the same world, declaring that it survives losing one Zone and `EvenSpreadRestored()` running a pass every five seconds before the first When. One Zone fails and responds again, the passes move Pods back one at a time until one evicts nothing, then nine Pods are Running, every Zone holds three, every Node one, and the Floor of six Ready Pods held throughout.
- **pods-lost-after-a-node-responds-again** — the same Nodes and placement as a-node-that-responds-again, declaring no Failure tolerance and judging no Floor, then all nine Pods are lost: the Deployment replaces them, and the Node that responded again takes a Pod like every other, so each Node holds one and every Zone three.
- **a-descheduler-pass-after-a-node-responds-again** — the same world as a-node-that-responds-again, declaring that it survives losing one Node and `EvenSpreadRestored()`, then one Descheduler pass: it evicts the one Pod standing past the spread on the Node holding two, the replacement takes the empty Node, so each Node holds one, every Zone three and all nine run, the end pods-lost-after-a-node-responds-again reaches by losing nine. The Floor of eight Ready Pods is judged.

A Scenario's Steps are read in `scenarios.go`.

## Writing one

The function is named for the condition it runs under and the outcome it asserts, `<Condition>_Should<Outcome>`.

```go
func ADeploymentPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Zones(3, node.Pool("general", node.PerZone(3), StandardNode()))
	s.WaitFor().AllNodesReady()
	s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement())
	s.WaitFor().AllPodsReady()
	s.Then().PodsPerZone(3)
}
```

Every line is one Step opened on `s`, so a reader scans the Step kinds in one column and a line is inserted, removed or reordered on its own. The Scenarios here are held to that layout by a rule test, per [the layout ADR](../docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md).

The Scenario runs under its Scenario name, the words of the condition before `_Should` in lowercase joined by hyphens, so the function above runs as `a-deployment-placed-for-resilience` and renaming the function renames the Scenario. A run of capitals is one word, so `GPUNodes` reads `gpu-nodes`. A condition holding anything but ASCII letters and digits, and two Scenarios naming one condition, are refused before any Simulated Cluster is provisioned. A new Scenario is a function in `scenarios.go`, after the last one, and one line at the end of `All`.

A Scenario's mandatory values are passed as named arguments — `node.PerZone`, `lab.DeploymentName`, `node.PoolName`, the NodePool's capacity and the specification — so a swapped or missing argument does not compile. Every count, a Deployment's replicas included, is a bare number under the Step naming its subject: `Zones(3, …)`, `ZonesFail(1)`, `NodesFail(1)`, `SurvivesLosing(lab.Zones(1))`, `PodsPerZone(3)`. A setting's values stand beside their Kubernetes field names, so none is read by its position. An optional setting is a further call on the complete declaration, `.SurvivesLosing` on the Deployment and `.Labels` on a NodePool.

A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its own Then Steps. The lab's Scenarios run in `cmd/lab`'s own suite, which fails when one reports a Finding or ends at a Step.
