<!-- harness: 2026-09-30 -->
# Product Requirements Document: k8s-resilience-lab

<!-- AGENT: This PRD is narrative prose. State WHAT the system does and WHY it matters. Never HOW — mechanism (flags, exit codes, file layouts, algorithms) lives in system-design.md. Never the decision trail — alternatives and trade-offs live in adr/. -->
<!-- AGENT: Annotate each requirement inline with its [REQ-XX-NNN] tag where the prose expresses it, and give it one "Done when" acceptance bullet carrying the same tag. The prose is the intent; the tagged bullet is the bounded, testable contract. Drop an <a id="req-xx-nnn"></a> anchor at first mention so other docs deep-link to it. -->
<!-- AGENT: A requirement is active by being in the narrative — there is no per-requirement Status field. Retire one by moving it to the Superseded list; never renumber an ID. -->

## Context

A team writes Kubernetes manifests that declare where their Pods may land: spread rules across Zones, anti-affinity between Nodes, disruption budgets. Whether those declarations produce the intended Placement, and whether the Placement recovers after a Zone or Node is lost, is normally discovered during a real outage. Proving it earlier needs a costly cluster per developer and per build.

k8s-resilience-lab is a learning and verification lab for that question. It starts a Simulated Cluster — a genuine Kubernetes control plane with fake Nodes and Pods — provisions a Topology, creates a Deployment, and reports the resulting Placement. Scheduling and controller behavior are real. Nothing runs inside a Pod: no container image is ever pulled.

The lab serves two uses from one body of behavior. Unattended, it runs each Scenario and asserts the Placement outcome, on a developer machine or in a build. Interactively, it shows the Simulated Cluster live and takes a Failure injected by hand, to demonstrate what the assertion claims.

## Goals

| ID | Goal | Success Metric |
|----|------|----------------|
| G-1 | Placement under a declared Topology is verified automatically, with no real cluster involved. | Every Scenario runs unattended on a developer machine and in a build, needing only a container runtime. |
| G-2 | The behavior each Scenario asserts is also demonstrable live. | An engineer starts the lab, watches the Placement of a running Deployment, and injects a Failure without editing code. |
| G-3 | A Deployment's survival is judged through a Failure, not only once the cluster has recovered. | Each Failure Scenario declares the Zones or Nodes its Deployment survives losing, and judges every moment from the Failure to its Then. |
| G-4 | A Scenario is written and read in one vocabulary. | Every name in a Scenario is one the lab offers Scenarios, each line is a Step, and every Scenario sits in one place. |
| G-5 | What happened inside a Scenario's Simulated Cluster is explainable after the run. | Every Scenario leaves a Timeline naming each Node, Pod and Deployment change and the control plane's reason for each Placement. |

## Non-Goals

<!-- Declined scope with the reason it was declined; never silently dropped. A non-goal ADR records the path to each decision. The first REQ id on a row is the declined one; a later id names a successor, which stays open. -->

| ID | Non-Goal | Rationale |
|----|----------|-----------|
| NG-1 | Running a Scenario against a real or pre-existing Kubernetes cluster, including one reached through a supplied cluster address. | The lab provisions and destroys the Simulated Cluster it uses, so a run never depends on cluster credentials or on another team's environment. See [ADR: Non-goal — real-cluster runs](adr/2026-10-03-non-goal-real-cluster-runs.md). |
| NG-2 | Verifying application-level recovery: readiness and liveness behavior, graceful shutdown, networking, storage, resource pressure, and out-of-memory kills. | No container runs in a Simulated Cluster, so none of this behavior exists to observe. See [ADR: Non-goal — application-level recovery](adr/2026-10-03-non-goal-application-level-recovery.md). |

## Requirements

<!-- Narrative prose, grouped by capability area. Weave each requirement into the prose and tag it inline with [REQ-XX-NNN]. After each group, a "Done when:" list gives every requirement in that group its bounded acceptance bullet, tagged with the same ID. A capability with edge cases adds a numbered "Edge cases:" list — stable numbering that tests and reviews cite. Mechanism stays out — link to system-design.md for the how. -->

### Simulated cluster lab

<a id="req-sim-001"></a>
Every Scenario runs in a Simulated Cluster of its own, provisioned with its Topology and removed when the Scenario ends `[REQ-SIM-001]`.

<a id="req-sim-002"></a>
One invocation with no arguments checks the lab's own rules, then runs every Scenario needing a Simulated Cluster `[REQ-SIM-002]`.

<a id="req-sim-014"></a>
A run carries out its Scenarios at once yet reports them in declared order, and a failing Scenario ends alone `[REQ-SIM-014]`.

<a id="req-sim-034"></a>
A run the engineer stops, or one its container runtime cuts short, still removes every Simulated Cluster it started and names why it ended `[REQ-SIM-034]`.

<a id="req-sim-036"></a>
A stop lands wherever the run then is: while its clusters start, during a Wait, or while the container runtime still creates a cluster `[REQ-SIM-036]`. Wherever it lands, the run names the stop and what it waited on, and none of its clusters remains. A second stop ends the run at once.

<a id="req-sim-003"></a>
An engineer names one Scenario to re-run it alone `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
One command decides whether the lab itself is sound, and the repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The same command holds the lab to its declared dependencies, free of known vulnerabilities, and to closing every connection it opens `[REQ-SIM-007]`.

<a id="req-sim-035"></a>
An engineer runs the lab's own tests needing no container runtime on their own, without waiting for any Simulated Cluster `[REQ-SIM-035]`. The tests needing one also run on their own, and the one command still runs both.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when a-deployment-placed-for-resilience runs, then three Zones each hold three Ready general Nodes. Nine Pods Settle Running, one per Node.
- `[REQ-SIM-001]` given an earlier run's Nodes and Pods, when a run reports, then it names only its own, each Zone and Node by position counted from one.
- `[REQ-SIM-001]` given a Kubernetes version named or none, when a run starts, then its cluster mirrors it or the pinned default, reporting which.
- `[REQ-SIM-001]` given other containers and a second run, when a Scenario ends, then its own Simulated Cluster is gone and no other container was touched.
- `[REQ-SIM-002]` given a reachable container runtime, when invoked with no arguments, then every Scenario needing a Simulated Cluster runs afresh, reporting name and outcome.
- `[REQ-SIM-002]` given every Scenario of the lab, when a run carries it out, then it reaches the outcome it asserts.
- `[REQ-SIM-002]` given failing lab rules or no reachable container runtime, when the lab is invoked, then it names why and stops before provisioning.
- `[REQ-SIM-014]` given a run of every Scenario, when it reports, then none waited for another, and each Scenario's lines stand together in declared order.
- `[REQ-SIM-014]` given Scenarios carried out at once, when each reports, then its Placement and Timeline name only its own Nodes and Pods.
- `[REQ-SIM-014]` given one Scenario failing part-way, when the run ends, then it is reported, the others reached their outcomes, and no Simulated Cluster remains.
- `[REQ-SIM-014]` given the lab's own tests, when the one command runs them, then those needing a Simulated Cluster run at once, and those judging a full run share one.
- `[REQ-SIM-034]` given a run of every Scenario, when the engineer stops it, then each Scenario still running ends as one failing part-way does. No Simulated Cluster or Descheduler of the run remains.
- `[REQ-SIM-034]` given a Simulated Cluster the container runtime created without its answer reaching the lab, when its Scenario ends, then that cluster is removed all the same.
- `[REQ-SIM-034]` given a Kubernetes version or Descheduler the lab obtains only in part, when the run starts, then the lab reports it as one it cannot obtain, naming it.
- `[REQ-SIM-036]` given a run whose Simulated Clusters still start, when the engineer stops it, then each Scenario still starting ends naming the stop, never a wait that ran out of time.
- `[REQ-SIM-036]` given a Scenario Waiting for its Nodes to be Ready, when the engineer stops the run, then the Scenario ends naming the stop and the Nodes it waited on, no Deployment.
- `[REQ-SIM-036]` given a container runtime failing a cluster's creation with a fault of its own, when the Scenario ends, then a cluster created under that name is removed. A name another container holds stays untouched.
- `[REQ-SIM-036]` given a cluster the container runtime still creates when the lab stops waiting on it, when its Scenario ends, then that cluster is removed once created.
- `[REQ-SIM-036]` given a stopped run still removing its clusters, when the engineer stops it again, then the run ends at once.
- `[REQ-SIM-003]` given several Scenarios, when the engineer names one, then only it runs, reporting as a full run does.
- `[REQ-SIM-003]` given a name matching no Scenario, when the run starts, then it stops before provisioning, naming the Scenarios that exist.
- `[REQ-SIM-004]` given the declared toolchain, when the one command runs, then the lab's rules, tests and build run. A failure, or a part able to reach a cluster the lab did not provision, fails it, named.
- `[REQ-SIM-004]` given the repository, when an engineer looks for the lab, then exactly one implementation is present.
- `[REQ-SIM-007]` given a dependency differing from its declaration, vulnerable, or named like a permitted one, when the one command runs, then it fails, naming it.
- `[REQ-SIM-007]` given the lab's own tests, when they end, then no connection the lab opened to a Simulated Cluster stays open.
- `[REQ-SIM-035]` given no reachable container runtime, when the engineer runs the tests needing no container runtime, then each of them runs and passes, and no Simulated Cluster starts.
- `[REQ-SIM-035]` given a reachable container runtime, when the engineer runs the tests needing one, then only those run.
- `[REQ-SIM-035]` given no reachable container runtime, when the one command or the tests needing one run, then each test needing one fails, naming the runtime.
- `[REQ-SIM-035]` given the one command, when it runs the lab's own tests, then every test of both kinds runs, on a developer machine as in a build.

**Edge cases:**

1. A Deployment declares more replicas than its Topology can hold. The unplaceable Pods are reported Pending, and a Scenario asserting every Pod Running fails.
2. The container runtime answers somewhere other than the conventional location. The run finds it.
3. A stale runtime address is present alongside a live runtime. The run reaches the live one.
4. A run names a Kubernetes version the lab cannot obtain. The run stops, naming it, before any Scenario starts.
5. The runtime cannot start one Scenario's Simulated Cluster while others start. That Scenario ends at its first Step, naming why, and the others run on.
6. The engineer stops a run while its Scenarios still provision. Every Simulated Cluster started so far is removed `[REQ-SIM-034]`.
7. A run is killed outright, with no chance to stop. Its Simulated Clusters stay for the engineer to remove `[REQ-SIM-034]`.
8. The engineer stops a run a second time while it removes its clusters. The run ends at once, and the clusters not yet removed stay, as after a kill `[REQ-SIM-036]`.

**ADR:** [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: Lab-provisioned Simulated Cluster](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: Predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Deployment's Pods

<a id="req-sim-009"></a>
<a id="req-sim-025"></a>
A run names each Deployment by its Kubernetes kind `[REQ-SIM-025]`. A Scenario's Deployment declares one of the predefined Pod specifications the Scenarios define, one per intent `[REQ-SIM-009]`. Customizations change a copy before deployment, through the lab's vocabulary or full access to any other Pod setting, which no Scenario uses yet. A specification holds one full access at most, and one holding more is refused `[REQ-SIM-009]`.

**Done when:**

- `[REQ-SIM-025]` given a Deployment, when its Given line is read, then it names the kind and replicas: 'a Deployment checkout of 9 replicas'.
- `[REQ-SIM-009]` given any Scenario, when its Deployment line is read, then it declares one predefined Pod specification, at most one Request and any Customizations, nothing else.
- `[REQ-SIM-009]` given a predefined specification, when read, then every setting it holds stands there, under its Kubernetes name or a Named setting.
- `[REQ-SIM-009]` given a declared specification, customized or not, when its Pods' settings are read back, then they are the definition's, each Customization's standing over them.
- `[REQ-SIM-009]` given a-deployment-placed-for-resilience's Topology and nine Pods declaring Default placement, when settled, then every Pod is Running, no count per Zone promised.
- `[REQ-SIM-009]` given two Deployments of one Scenario, when placed, then each is placed by its own specification, the other's Customizations never reaching it.
- `[REQ-SIM-009]` given two Customizations naming one setting, through the lab's vocabulary or the one full access, when both apply, then the later stands.
- `[REQ-SIM-009]` given a specification holding more than one full access, when its Deployment is declared, then it is refused, naming why, and the Scenario ends at that Given.
- `[REQ-SIM-009]` given a Deployment customized with full access, when its Pods are read, then all it named stands but the waiting time before Pods leave an unresponsive Node. Its Pods count as that Deployment's.

**Edge cases:**

1. A Deployment declaring Resilient placement on a Topology of one Zone. Its Pods still land one per Node, all in that Zone.
2. Two Deployments of one Scenario declare the same specification. Each is spread on its own, the other's Pods not counting towards its Skew.
3. A Customization names a setting its specification does not hold. The specification gains it.
4. Full access leaves out a setting the lab depends on. That setting holds and the Deployment is deployed.
5. Full access names a setting the control plane refuses. The Scenario ends at the Step deploying it, as for any refused Customization.
6. A specification's full access sets a setting that a later Customization names, such as the Request on the Deployment line. The later Customization stands.
7. A predefined specification holds a full access, and the Deployment line adds a second. The specification is refused all the same.

**ADR:** [ADR: Predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Stating what decides a Placement

<a id="req-sim-015"></a>
Every value deciding a Placement stands on a Scenario's own lines, stated or named there, and the lab gives none `[REQ-SIM-015]`. A name stands for values the Scenarios define once beside them, and each further Named Node capacity arrives with its first Scenario.

**Done when:**

- `[REQ-SIM-015]` given a NodePool stating no Node capacity, when declared, then it is refused where stated.
- `[REQ-SIM-015]` given a NodePool naming a Named Node capacity, when provisioned, then its Nodes get that cpu, memory and Pod count, which the Given line reports by name.
- `[REQ-SIM-015]` given every predefined Pod specification, when its definition is read, then it names its own Startup time.
- `[REQ-SIM-015]` given the Named Node capacities and Pod specifications the Scenarios use, when their sources are read, then each is defined once beside them and the lab defines none.
- `[REQ-SIM-015]` given any run, when it starts, then it reports the lab's two waiting times, and no Node capacity or Startup time of its own.

**Edge cases:**

1. A predefined Pod specification defined with no Startup time. It is refused where defined.
2. A predefined Pod specification defined with no Request. Its Pods ask for no cpu and no memory.
3. Two definitions under one name, of a Named Node capacity or a Pod specification. They are reported where defined, so each name stands for one set of values.

**Design:** [system-design.md](system-design.md)

### Fitting a Deployment's Pods onto the Nodes

<a id="req-sim-011"></a>
A Node has room for only so much, and a Pod may Request a share of it `[REQ-SIM-011]`. A Pod no Node has room for stays Pending; after a Failure, only responding Nodes offer room, as no-room-after-a-zone-stops-responding shows.

<a id="req-sim-024"></a>
A Deployment line names Standard Pod, a quarter of the Standard capacity, or GPU Pod beside its replicas and the specification `[REQ-SIM-024]`.

**Done when:**

- `[REQ-SIM-011]` given a NodePool stating its Nodes' cpu, memory and Pod count, when provisioned, then every Node has that capacity, reported in that order.
- `[REQ-SIM-011]` given a NodePool stating the cpu and memory alone, when declared, then it is refused where stated.
- `[REQ-SIM-011]` given a specification stating a Request or none, when its Pods are placed, then a stated Request stands under its Kubernetes name. Without one they ask for nothing and the spread alone decides.
- `[REQ-SIM-011]` given a Deployment requesting more cpu than its Topology has room for, when it settles, then the Pods that fit are Running and the rest stay Pending.
- `[REQ-SIM-011]` given Nodes with room for one Pod each and a nine-Pod Deployment, when it settles, then nine Pods are Running, one per Node.
- `[REQ-SIM-011]` given that Placement, when one Zone stops responding, then it settles at six Pods Running and three Pending, never below its Floor of six.
- `[REQ-SIM-024]` given every Scenario's Deployment line, when read, then it names Standard Pod or GPU Pod or states its own Request, only one.
- `[REQ-SIM-024]` given every predefined Pod specification, when read, then it holds no Request, so a Deployment naming Tolerating GPU Nodes alone asks for nothing.
- `[REQ-SIM-024]` given a Deployment naming Standard Pod or GPU Pod, when it runs, then the lines beneath its Given line report that Request.

**Edge cases:**

1. A NodePool states a Node capacity of no Pod, no cpu, or no memory. It is refused where stated.
2. A Deployment's Request exceeds the room of every Node of its Topology. Every Pod of it stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter bound decides.

**Design:** [system-design.md](system-design.md)

### Holding Nodes of different kinds in a Zone

<a id="req-sim-013"></a>
A Zone holds NodePools of identical Nodes, the same in every Zone `[REQ-SIM-013]`.

**Done when:**

- `[REQ-SIM-013]` given three Zones of two general Nodes and one larger gpu Node each, when provisioned, then every Zone holds three Ready Nodes, each of its NodePool's capacity and label.
- `[REQ-SIM-013]` given that Topology, when a run reports its Placement, then each Zone's Nodes are numbered one to three, general's two first.
- `[REQ-SIM-013]` given that Topology, when the run starts, then it reports each NodePool's name, Node count per Zone, and cpu, memory and Pod count.
- `[REQ-SIM-013]` given that Topology and a Request only gpu Nodes have room for, when it settles, then its Running Pods sit on gpu Nodes alone.
- `[REQ-SIM-013]` given every Scenario declaring one NodePool, when it runs, then that NodePool is named general.

**Edge cases:** each is refused where stated.

1. A NodePool of no Node.
2. Two NodePools of one Topology under one name, their Nodes being indistinguishable.
3. A NodePool name Kubernetes cannot hold as a label value.
4. A NodePool stating no Node capacity.
5. A NodePool stating no name, or a Topology stating no NodePool.
6. A second Topology in one Scenario, whether or not the first is provisioned. The refusal names the first, which the cluster keeps.

**Design:** [system-design.md](system-design.md)

### Marking a NodePool's Nodes with labels

<a id="req-sim-017"></a>
A NodePool may state Kubernetes labels for its Nodes, beside the lab's own Zone, hostname and NodePool labels `[REQ-SIM-017]`. A Then counts a Deployment's Pods per value of a stated label, and the Even spread spreads over one.

**Done when:**

- `[REQ-SIM-017]` given a NodePool stating labels, when provisioned, then each of its Nodes carries them beside the lab's own.
- `[REQ-SIM-017]` given that Topology, when the run starts, then its Given line reports each NodePool's stated labels after its capacity, under their Kubernetes keys.
- `[REQ-SIM-017]` given a NodePool stating a label under a key the lab sets, when declared, then it is refused where stated.
- `[REQ-SIM-017]` given a Then stating a label and a Pod count, when it judges, then it records a Finding unless every value the NodePools state holds that count.
- `[REQ-SIM-017]` given three Zones of one-Node NodePools labelled on-demand and spot, when a-deployment-spread-over-on-demand-and-spot places six Pods, then all six run, two per Zone and three per label value.

**Edge cases:**

1. A key or value Kubernetes cannot hold as a label. It is refused where stated.
2. One NodePool stating one key twice. It is refused where stated.
3. A NodePool stating no label. Its Nodes carry the lab's labels alone.
4. A Then stating a label no NodePool of the Topology states. The Scenario ends at that Then, naming the label.
5. A label value whose Nodes hold no Pod of the Deployment. It counts as none, so a Then asserting more records a Finding.
6. Pods on Nodes whose NodePool states no value of that label. They count towards no value.

**Design:** [system-design.md](system-design.md)

### Starting a Deployment's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready, so a Deployment replacing Pods after a Failure runs short meanwhile `[REQ-SIM-012]`.

**Done when:**

- `[REQ-SIM-012]` given a shortest and a longest Startup time, specified or customized, when Pods are placed, then each is Ready between those bounds, both included, after its placement.
- `[REQ-SIM-012]` given a Deployment customized with one Startup time, in whole seconds or milliseconds, when its Pods are placed, then none is Ready sooner. Of two such Customizations, the later stands.
- `[REQ-SIM-012]` given a placed Pod inside its Startup time, when the Placement is reported, then it counts Pending on its Node, and the Placement is not yet Settled.
- `[REQ-SIM-012]` given a Deployment declaring a predefined specification, when the run reports it, then it names that specification's shortest and longest Startup time.

**Edge cases:**

1. A Startup time of zero. The Pod is Ready as soon as it is placed.
2. A shortest Startup time longer than its longest, or a negative time. It is refused where stated, ending the Scenario at the Step declaring that Deployment.
3. A shortest and a longest Startup time that are equal. Every Pod takes that time.

**Design:** [system-design.md](system-design.md)

### Surviving a Node or Zone Failure

<a id="req-sim-008"></a>
A Scenario injects a Failure and judges the Placement on both sides of it `[REQ-SIM-008]`. A stopped Node stays in the cluster, as a hung machine does, until the control plane moves its Pods away after a waiting time the lab shortens. A Failure costs the spread, accepted while the Pod count holds, so no count per Node is asserted after it, nor per Zone after a Zone Failure.

**Done when:**

- `[REQ-SIM-008]` given nine Pods declaring Resilient placement Settled, three per Zone, when one Node or Zone stops responding, then its Pods move away within a few seconds.
- `[REQ-SIM-008]` given one Node or Zone stopped, when the Placement settles again, then nine Pods are Running, none on the stopped Nodes. After a Node Failure, every Zone still holds three.
- `[REQ-SIM-008]` given a-node-that-stops-responding or a-zone-that-stops-responding, when it runs twice, then the same Node or Zone fails in both runs and the run reports which.
- `[REQ-SIM-008]` given either Failure Scenario, when read, then the When injecting the Failure names no Node or Zone, and that of a-node-that-stops-responding states no waiting time.

**Edge cases:**

1. A Pod of a Node that stopped responding cannot be placed elsewhere. It stays Pending with the control plane's reason.
2. A When asks for more failing Nodes or Zones than the Topology holds. The Scenario ends at that Step, naming it.
3. A Zone holding many Nodes fails, by a When or a click. All its Nodes stop responding at once, the last before the control plane moves the Pods of the first.

**ADR:** [ADR: Node Failure](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

### Failing every Node of one label value

<a id="req-sim-018"></a>
A Failure may stop, in one Step, every Node carrying one value of a label a NodePool states `[REQ-SIM-018]`. The Scenario spot-nodes-that-stop-responding demonstrates it.

**Done when:**

- `[REQ-SIM-018]` given a When naming a stated label and one of its values, when run, then every Node carrying it stops responding in one Step, reported, and their Pods move away within seconds.
- `[REQ-SIM-018]` given spot-nodes-that-stop-responding, when read, then its When names the label and value alone, no Node, Zone, count or waiting time.
- `[REQ-SIM-018]` given three Zones of two on-demand Nodes and one spot Node each, when nine Pods declaring Resilient placement Settle, then every Node holds one.
- `[REQ-SIM-018]` given that Placement, when every spot Node stops responding, then the Deployment surviving the loss of three Nodes never falls below its Floor of six Ready Pods.
- `[REQ-SIM-018]` given that Failure, when the Placement settles again, then no Pod is on a stopped Node and every Zone still holds three.

**Edge cases:** each ends the Scenario at that When, naming what it named.

1. A When naming a label no NodePool of the Topology states, the lab's own labels included.
2. A When naming a value no NodePool states for that label.
3. A When naming a value whose every Node has already stopped responding.
4. A When naming no label, or more than one.

**Design:** [system-design.md](system-design.md)

### A stopped Node or Zone responding again

<a id="req-sim-029"></a>
A When makes every Node a Failure stopped respond again, by Node or by Zone, as a click does in the Live view `[REQ-SIM-029]`. The control plane places a Pod only when it is created, so a Node responding again stays empty.

**Done when:**

- `[REQ-SIM-029]` given stopped Nodes or Zones, when a When makes them respond again, then every stopped Node, or every Node of every stopped Zone, responds again in one Step. The run reports which, as a Failure's When does.
- `[REQ-SIM-029]` given either Scenario, when read, then that When names no Node, Zone or count.
- `[REQ-SIM-029]` given Nodes that responded again, when the next Wait's narrative is read, then it names each turning Ready.
- `[REQ-SIM-029]` given a-node-that-responds-again Settled after its Node is Ready again, then that Node holds no Pod, every Zone holds three, and one Node holds two.
- `[REQ-SIM-029]` given a-zone-that-responds-again Settled after its Nodes are Ready again, then that Zone holds no Pod and the nine Pods stay Running on the others.

**Edge cases:** each ends the Scenario at that When, naming it.

1. A When making Nodes or Zones respond again while no Node of the Scenario is stopped.
2. A When making Zones respond again while a stopped Node shares its Zone with a Node still responding.

**Design:** [system-design.md](system-design.md)

### Losing Pods of a Deployment

<a id="req-sim-030"></a>
A When loses a stated number of one named Deployment's placed Pods in one Step, as a click on a Pod's box does in the Live view `[REQ-SIM-030]`. The control plane places each replacement among the Nodes then responding, a Node that responded again included.

**Done when:**

- `[REQ-SIM-030]` given a Deployment with placed Pods, when a When names it and a number, then that many of its placed Pods are lost in one Step, the run reporting each.
- `[REQ-SIM-030]` given Pods lost to a When, when the next Wait's narrative is read, then it names each replacement scheduled and its Node.
- `[REQ-SIM-030]` given pods-lost-after-a-node-responds-again, when read, then that When names the Deployment and the number alone, no Pod, Node or Zone.
- `[REQ-SIM-030]` given a-node-that-responds-again's Settled Placement, when all nine Pods are lost and it Settles, then every Node holds one Pod, nine are Running and every Zone holds three.

**Edge cases:** each ends the Scenario at that When, naming it.

1. A When naming a Deployment the Scenario has not declared.
2. A When losing fewer than one Pod, or more than the Deployment has placed.
3. A When losing Pods of a Deployment declaring a Failure tolerance.

**Design:** [system-design.md](system-design.md)

### Moving Pods with the Descheduler

<a id="req-sim-031"></a>
A Scenario's Descheduler evicts running Pods its Descheduler policy names, which the control plane never moves, and their Deployments replace them `[REQ-SIM-031]`. A Given names the policy by its intent, defined beside the Scenarios. The Descheduler mirrors the run's Kubernetes version. A dry run, leaving no Entry, is no part of the lab.

A When runs one pass, and the next Step starts once it ends `[REQ-SIM-031]`. A Given may instead run a pass on each interval it states, with a Wait for a pass evicting nothing; Settled keeps its meaning. The Scenario a-descheduler-pass-after-a-node-responds-again demonstrates one pass.

**Done when:**

- `[REQ-SIM-031]` given a declared policy, when its Given line is read, then it names the policy and Descheduler version, the lines beneath reporting the policy under Kubernetes' names.
- `[REQ-SIM-031]` given a policy, when its definition is read, then it mirrors the Descheduler's YAML line for line, the lab adding or inferring nothing.
- `[REQ-SIM-031]` given a Kubernetes version paired with no Descheduler, when a run carries a Scenario declaring a policy, then it stops before provisioning, naming it. The default version has one.
- `[REQ-SIM-031]` given a declared policy, when a When runs one pass, then it runs under that policy in one Step ending with the pass, the When's line naming the policy.
- `[REQ-SIM-031]` given a pass, when the next Wait's narrative is read, then it names each Pod evicted and each replacement with its Node, apart from a Failure's evictions.
- `[REQ-SIM-031]` given a-node-that-responds-again's Settled Placement, when one pass runs and it Settles, then the pass evicted one Pod. Every Node holds one, nine are Running and every Zone holds three.
- `[REQ-SIM-031]` given a Scenario that ran a pass, when it ends however it ends, then nothing of its Descheduler remains.
- `[REQ-SIM-031]` given a policy declared with an interval, when the Scenario runs, then a pass runs on each interval until it ends, and the Given line names the interval.
- `[REQ-SIM-031]` given a Descheduler on an interval, when a Scenario Waits for a pass evicting nothing, then the Wait ends at the first such pass begun after it.
- `[REQ-SIM-031]` given a-zone-that-responds-again-under-a-descheduler, passing every five seconds, when its Zone fails, responds again, and a pass evicts nothing, then nine Pods are Running, three per Zone, one per Node. The Floor held and no When moved them.

**Edge cases:** "refused" means refused where stated; "ends" means the Scenario ends at that Step, naming it.

1. A policy naming no plugin: refused.
2. Two policies defined under one name: reported where defined.
3. A Scenario declaring a second policy: refused.
4. A When running a pass in a Scenario declaring no policy: ends.
5. A pass that cannot run or does not end: ends, naming why.
6. A pass evicting nothing leaves the Placement as it stands.
7. An interval of zero or less: refused.
8. A Wait for a pass evicting nothing while no interval runs: ends.
9. A When running one pass while the Descheduler runs on an interval: ends.
10. A Descheduler on an interval that cannot start or stops: the Scenario ends, naming why.

**Design:** [system-design.md](system-design.md)

### Keeping Pods off a dedicated NodePool

<a id="req-sim-019"></a>
A NodePool may state NoSchedule taints, keeping off its Nodes every Pod not tolerating them `[REQ-SIM-019]`. A toleration only permits a Pod onto a tainted Node, so a dedicated NodePool's Pods reach it by a Request only its Nodes have room for. A later Deployment, deployed by the next Wait, is the one every Then after it counts. The Scenario general-pods-beside-tainted-gpu-nodes demonstrates it.

**Done when:**

- `[REQ-SIM-019]` given a NodePool stating a NoSchedule taint, when provisioned, then its Nodes carry it, the Given line reporting it after the labels, under Kubernetes names.
- `[REQ-SIM-019]` given a NodePool stating a taint of another effect, when declared, then it is refused where stated.
- `[REQ-SIM-019]` given a specification tolerating a taint, when its Given line is read, then the lines beneath report the toleration's key, operator, value and effect, and no waiting time.
- `[REQ-SIM-019]` given a Then stating a NodePool and a Pod count, when it judges, then it records a Finding unless that NodePool's Nodes hold that count.
- `[REQ-SIM-019]` given three Zones, each of two general Nodes and one larger gpu Node tainted NoSchedule, when six Pods declaring Resilient placement are placed, then all six are Running on general Nodes.
- `[REQ-SIM-019]` given that Placement, when three Pods tolerating the taint with a Request only gpu Nodes fit are placed, then all three run on gpu Nodes.

**Edge cases:**

1. A taint leaving out its key, value or effect, or stating a key or value Kubernetes cannot hold. It is refused where stated.
2. One NodePool stating one taint key twice. It is refused where stated.
3. A taint under a key the control plane sets on an unresponsive Node. It is refused where stated.
4. A toleration of a NoSchedule taint stating a waiting time. It is refused, ending the Scenario at the Step declaring that Deployment.
5. A Deployment not tolerating a taint finds no room left on the untainted Nodes. Its extra Pods stay Pending, none on a tainted Node.
6. A Then naming a NodePool the Topology lacks. The Scenario ends at that Then, naming it.
7. A Scenario declares a second Deployment under a name it already declared. It is refused, ending the Scenario at that Given.

**Design:** [system-design.md](system-design.md)

### Judging a Deployment's Failure tolerance

<a id="req-sim-027"></a>
A Deployment declares the Zones or Nodes it survives losing `[REQ-SIM-027]`. Its Floor is its replicas less the most Pods those hold evenly spread. One Then judges that every declaring Deployment kept its Floor of Ready Pods since the first Failure.

**Done when:**

- `[REQ-SIM-027]` given any Deployment line, when read, then it declares Zones or Nodes it survives losing, never Pods.
- `[REQ-SIM-027]` given nine replicas over three Zones of three Nodes, when surviving one Zone or one Node, then the Floor is six or eight. Ten replicas surviving one Zone keep six.
- `[REQ-SIM-027]` given a Pod on a stopped Node or Pending, when judged, then it is lost, though the control plane may still report it Ready.
- `[REQ-SIM-027]` given a declaring Deployment below its Floor at any moment from the first Failure to the Then, when judged, then a Finding names it.
- `[REQ-SIM-027]` given a judgement, when reported, then each Deployment's line states kept or fell, Floor, declared loss, and lowest Ready count, when and how long.
- `[REQ-SIM-027]` given a fall, when its Finding is read, then it states how low, when, how long, the Pods lost against the most declared, and their Nodes. An unended fall lasts 'for the rest of the Scenario'.
- `[REQ-SIM-027]` given every Failure Scenario, when run, then it declares what its When fails, and every Floor holds.

**Not yet in force:**

- `[REQ-SIM-027]` given a Then naming one Deployment, when it judges, then that Floor alone decides it.
- `[REQ-SIM-027]` given a Then asserting a Floor fell, when none fell, then it records a Finding, which a Scenario pinning a Deployment to one lost Zone proves.
- `[REQ-SIM-027]` given a Floor crossed, when the next Wait narrative is read, then it names each fall and return.

**Edge cases:**

1. A count below zero, or reaching the Topology's own. The Scenario ends at that Given, naming it.
2. A count of none. The Floor is every replica.
3. A Then with no Failure before it, or no declaring Deployment. The Scenario ends at that Then, naming it.
4. An Interactive run reaches a Deployment declaring what it survives losing after the first When. That declaration is not carried out, and every Step reported before it stands as reported.

**Design:** [system-design.md](system-design.md)

### Explaining a Placement after the run

<a id="req-sim-005"></a>
A reported Placement says where Pods landed, and a Timeline says how they got there `[REQ-SIM-005]`. Every Scenario, failed or not, files its Timeline in one place under the working directory, and the lab removes none.

**Done when:**

- `[REQ-SIM-005]` given a Scenario that has run, when it ends, then one line names the Timeline's entry count and file, which keeps every Entry.
- `[REQ-SIM-005]` given an engineer asks for the whole Timeline, when a Scenario ends, then every entry is printed in the order it happened.
- `[REQ-SIM-005]` given a Scenario's Topology and Deployment in place, when its Timeline is read, then it holds every change to its Nodes, Pods and Deployment objects, with the control plane's reason for each Pod placed or left Pending.
- `[REQ-SIM-005]` given two Scenarios of a run, or two runs at once, when they end, then each Scenario left its own Timeline file, none replacing another.
- `[REQ-SIM-005]` given a Scenario's Simulated Cluster is removed, when its Timeline file is opened, then it is still readable.

**Edge cases:**

1. A Scenario fails part-way. The run prints its Timeline's line and files its Timeline anyway.
2. The place the Timeline files collect in does not exist when a run starts. The run creates it.
3. A Timeline cannot be written. The run reports the Scenario's outcome and names the Timeline it could not write.

**Design:** [system-design.md](system-design.md)

### Reading the Placement under every Wait

<a id="req-sim-021"></a>
Under every Wait, a run shows how the Placement changed and where every Pod stands `[REQ-SIM-021]`.

**Done when:**

- `[REQ-SIM-021]` given any Wait, reached or not, when printed, then a separator line, a Placement board and a second separator follow its line, before any Then or next Step. Each separator is as long as the board's widest line.
- `[REQ-SIM-021]` given a Then followed by a When, when read, then no separator line stands between them.
- `[REQ-SIM-021]` given two Deployments, when a board is read, then every Node has one line under its Zone, naming its NodePool and listing every Pod on it by Deployment name and own suffix.
- `[REQ-SIM-021]` given a board after an earlier one, when read, then exactly the Pods new on each Node since are marked. Pods left Pending are listed beneath the Nodes.
- `[REQ-SIM-021]` given Pods placed since the previous Wait, when its narrative is read, then it names each Pod's Node and when it reached Running, in order.
- `[REQ-SIM-021]` given a Failure before a Wait, when its narrative is read, then it names in order each Node turning NotReady, Pod evicted and replacement scheduled, linking no eviction to a replacement.
- `[REQ-SIM-021]` given Pods left Pending when a Wait ends, when its narrative is read, then it names each with the scheduler's reason.
- `[REQ-SIM-021]` given nothing changed the Placement since the previous Wait, when printed, then the separator and board follow its line directly, with no narrative.
- `[REQ-SIM-021]` given a Timeline, when its file or the whole Timeline is read, then each Step stands at its moment among the Entries.
- `[REQ-SIM-021]` given a report's end, when read, then the last board is followed by one Skew line for the judged Deployment, a separator and the Timeline's line, and no other summary.
- `[REQ-SIM-021]` given a filed Timeline, when its line is read, then it ends at the file's name with no colon, under a separator at least as long.
- `[REQ-SIM-021]` given two Scenarios, when read, then a heavier separator of another character follows the run's opening lines and parts the Scenarios, as long as the widest line beside it.

**Edge cases:**

1. The board under the Wait for every Node Ready holds no Pod. Every Node still has its line, empty.
2. The first board holding a Deployment's Pods marks every one of them new.
3. A Pod is evicted by a Failure and a replacement is placed. The narrative names both in order, and the board shows the replacement alone, marked new.
4. A Scenario ends at a Wait it does not reach. That Wait still prints its narrative, separators and board, up to the end.
5. A Scenario ends before any Deployment is deployed. Its boards list the Nodes with no Pod, and no Skew line is given.
6. The Pods the lab places on its own while provisioning the Topology appear in neither a narrative nor a board.
7. A Wait with no Then judging it, such as one before a When. Its board still closes with a second separator before the next Step.
8. A run holds one Scenario. One heavier separator follows the run's opening lines, and none follows the Scenario.

**Design:** [system-design.md](system-design.md)

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as Steps, one per line: Given declares, When changes the cluster, Wait waits for a cluster state, Then asserts the Placement `[REQ-SIM-006]`.

<a id="req-sim-020"></a>
An engineer reads every Scenario of the lab in one source, each run and reported under its Scenario name, the condition its function names `[REQ-SIM-020]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario, when read, then each line is one Given, When, Wait or Then Step, none handling an error, and each name one the lab offers or a full access name.
- `[REQ-SIM-006]` given any Scenario, when it runs, then each Wait is a Step reporting its own line, one before the first Then and one after every Failure.
- `[REQ-SIM-006]` given a declared Topology or Deployment, when its Wait runs, then every declared Node or Pod is Ready and the line names the time taken, whether written with its Given or alone.
- `[REQ-SIM-006]` given a Then with no Wait since the Given or Failure before it, or a Failure before the first Wait, when run, then the Scenario ends there, naming it.
- `[REQ-SIM-006]` given a Wait stating no time, when Pods are not all Ready within the lab's waiting time, then the Scenario ends there, naming them.
- `[REQ-SIM-006]` given a Wait stating no time, when its state is reached, then its line names the time taken and no time it was held to. The Scenario goes on.
- `[REQ-SIM-006]` given a Wait stating a time, when the state is reached inside it, then its line names that time and the time taken. Otherwise the Scenario ends there, the line naming that time alone.
- `[REQ-SIM-006]` given a Wait stating a time above the lab's waiting time, when run, then it is refused, naming that waiting time.
- `[REQ-SIM-006]` given a Wait stating the least time a refusal names as admitted, when the cluster is already in that state, then the Wait is reached inside that time.
- `[REQ-SIM-006]` given a Scenario that has Waited, when the Thens after it judge, then the Placement has stopped changing, even with Pods still Pending.
- `[REQ-SIM-006]` given every Failure Scenario, when read, then it states a time on the Wait after its Failure and none on the Waits after its Givens.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when run, then the Scenario ends there, naming it. An unmet Then only reports a Finding.
- `[REQ-SIM-020]` given the repository, when an engineer looks for the Scenarios, then one source holds every one and no machinery, beside a README naming each by Scenario name with its asserted outcome.
- `[REQ-SIM-020]` given any Scenario, when a run reports it, then it is named by the condition its function names, in lowercase words joined by hyphens, such as a-node-that-stops-responding.

**Edge cases:**

1. A Scenario writes two Waits in a row. The second reports a wait of almost none, rather than being refused.
2. A Wait for every Pod stands before any Deployment is declared. It reports that nothing was waited for.
3. A Wait states a time of no seconds. It is refused where stated.
4. A Wait for a Settled Placement stands before any Deployment is declared. It is reached at once.
5. Two Scenarios whose functions name the same condition. The lab's rules refuse them before provisioning, naming both `[REQ-SIM-020]`.
6. A Scenario whose function names no condition. The lab's rules refuse it before provisioning, naming the function `[REQ-SIM-020]`.
7. A Wait stating no time reaches its state only after the lab's waiting time has passed. A Wait for a pass evicting nothing can take that long, waiting for the pass and then for Settled. It is reported reached, naming the time taken.

**ADR:** [ADR: Scenarios as Go functions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

### Reading a Scenario by subject

<a id="req-sim-016"></a>
<a id="req-sim-026"></a>
Each name a Scenario writes shows its subject, a Pod, a Node, or the Scenario with its Steps and Topology, unless the Scenarios define it `[REQ-SIM-016]`. Scenario code stays idiomatic Go, naming each value of a setting under its Kubernetes name, and each name the lab offers shows where it is defined `[REQ-SIM-026]`. The Scenarios define Named settings, and a Scenario states any spread but the Even spread itself.

**Done when:**

- `[REQ-SIM-016]` given any Scenario source, when read, then each name it writes shows its subject or is one the Scenarios define. Each value of a setting stands under its Kubernetes name, never by position alone.
- `[REQ-SIM-026]` given any Scenario source, when read, then it is idiomatic Go passing the lab's format and lint checks. Each name the lab offers, inline Customizations included, shows where it is defined, and none is renamed.
- `[REQ-SIM-016]` given the Named setting for leaving an unresponsive Node, when read, then it holds the not-ready and unreachable tolerations at the lab's waiting time alone, and every specification names it.
- `[REQ-SIM-016]` given the Even spread, when read, then it holds one topology spread over the subject given alone, and Resilient placement names it over Zones and over Nodes.
- `[REQ-SIM-016]` given a declared specification, customized or not, when its Given line is read, then it names the intent, with every setting its Pods carry beneath, one per line under its Kubernetes name.
- `[REQ-SIM-016]` given Resilient placement, when the lines beneath its Given line are read, then they report both spreads in full.
- `[REQ-SIM-016]` given a setting of several values leaving out a value or stating it as zero, when stated, then it is refused where stated.
- `[REQ-SIM-016]` given any count a Scenario states, when read, then the Step naming it states its subject and the count reads bare, replicas included. A Node's room reads under the Node.

**Edge cases:**

1. A Deployment declares Default placement, which holds no spread. The lines beneath its Given line report the two tolerations alone.
2. A Customization writes another time for either toleration. The lab's waiting time holds and is reported.
3. A setting of several values leaves out a value Kubernetes would default. It is refused all the same, since no value deciding a Placement falls to a default. A Scenario needing a zero sets it through full access.

**ADR:** [ADR: Predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) · [ADR: Idiomatic Scenario code](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)  ·  **Design:** [system-design.md](system-design.md)

### Watching a Scenario live

<a id="req-sim-022"></a>
An engineer starts an Interactive run naming one Scenario and opens its Live view in a browser `[REQ-SIM-022]`. Its Steps run up to its first When, and its Simulated Cluster is then held until the lab stops. The Live view draws that cluster alone as boxes, and only a Failure injected there changes it.

**Done when:**

- `[REQ-SIM-022]` given an Interactive run naming a Scenario, when it starts, then its Steps up to its first When run, reporting as in a run. The terminal also names the Live view's address.
- `[REQ-SIM-022]` given the Live view open while the Steps run, when the Placement changes, then it shows the change as the lab observes it, with no reload.
- `[REQ-SIM-022]` given a held cluster, when the Live view is read, then each Zone's box holds its Nodes' boxes, each holding its Pods by Deployment name and own suffix, with Pending Pods apart.
- `[REQ-SIM-022]` given a Node's box, when read, then it names its NodePool and stated labels, and gives Pods, cpu and memory as capacity, requests and left.
- `[REQ-SIM-022]` given a Pod's box, placed or Pending, when read, then it gives its Request as cpu and memory under the word requests, cpu 0 and memory 0 when none is declared.
- `[REQ-SIM-022]` given the page header, when read, then it names k8s-resilience-lab, a separator such as a hyphen, then the Scenario name.
- `[REQ-SIM-022]` given a Node's or placed Pod's box, when read, then its first line gives its name and, top right, its Ready state by colour and word. A Pod's requests stand beneath, its box as wide as its Node's.
- `[REQ-SIM-022]` given a Pod new on its Node, when its box is read, then it is marked as a Placement board marks it, and the mark fades.
- `[REQ-SIM-022]` given a narrow window or a light or dark setting, when read, then the Live view fits and follows it, loading nothing from another host.
- `[REQ-SIM-022]` given the Steps have run and no Failure injected, when watched, then the cluster stays put until the engineer stops the lab, removing it.

<a id="req-sim-023"></a>
**Not yet in force.** Later slices add the following, first the Timeline beneath the boxes `[REQ-SIM-023]`, deciding with it whether Zones stack in a column beside a pane on a laptop `[REQ-SIM-023]`. Until the controls ship, an Interactive run naming no Scenario is refused, per edge case 1.

- `[REQ-SIM-023]` given the Live view open, when the Timeline gains an Entry, then it appears beneath the boxes in order with no reload, each Step at its moment and each Entry showing at a glance what changed.
- `[REQ-SIM-022]` given an Interactive run naming no Scenario, when the Live view opens, then it offers every Scenario by name, provisioning nothing until one is picked.
- `[REQ-SIM-022]` given a held cluster, or Steps still running, when the engineer picks another Scenario or resets, then the Steps stop and it is removed before a fresh one is provisioned, each reported.
- `[REQ-SIM-022]` given the engineer chooses every Step, when a Scenario is picked or reset, then every Step runs and reports, and the cluster is then held.

**Edge cases:**

1. An Interactive run names no Scenario, two, or one matching none. It stops before provisioning, naming the Scenarios that exist.
2. A Scenario with no When. All its Steps run.
3. A Step that cannot be carried out. The terminal names it as a run does, and the cluster is held as that Step left it.
4. A Then records a Finding. The terminal reports it as a run does, and the later Steps still run.
5. The Live view opens only after the Steps ran, or in two browsers at once. Each shows the Placement as it stands.
6. Pods the lab places itself while provisioning the Topology appear nowhere in the Live view.
7. The engineer stops the lab while Steps still run. The Steps stop and the Simulated Cluster is removed.
8. No container runtime is reachable. The Interactive run names why and stops before offering the Live view.

**Design:** [system-design.md](system-design.md)

### Injecting a Failure from the Live view

<a id="req-sim-028"></a>
An engineer clicks a Node's, Zone's or placed Pod's box in the Live view to inject a Failure, and the terminal reports it as a When `[REQ-SIM-028]`. A second click makes a stopped Node or Zone respond again; a lost Pod has no recovery.

<a id="req-sim-037"></a>
The Nodes of one Failure stop together, by a click or a Scenario's When, so a Zone of many Nodes fails as fast as one of two `[REQ-SIM-037]`. A click making a Node or Zone respond again that cannot be carried out is named as a click's Failure is.

**Done when:**

- `[REQ-SIM-028]` given a held cluster, when a Node's box is clicked, then that Node stops responding as in a Node Failure, with no confirmation asked.
- `[REQ-SIM-028]` given a held cluster, when a Zone's box is clicked, then all its Nodes stop responding at once, as in a Zone Failure.
- `[REQ-SIM-028]` given a held cluster, when a placed Pod's box is clicked, then that Pod is gone and its Deployment replaces it.
- `[REQ-SIM-028]` given a click, when the terminal is read, then one When, indented as a run's, names what changed, a Pod as the Live view does: "When Node worker-2 stops responding", "When Zone zone-a stops responding", "When Pod checkout-abc12 is lost".
- `[REQ-SIM-028]` given a held cluster, when a Pending Pod or any other part of the page is clicked, then nothing changes.
- `[REQ-SIM-028]` given a stopped Node, or a Zone whose every Node stopped, when its box is clicked, then it responds again: "When Node worker-2 responds again", "When Zone zone-a responds again".
- `[REQ-SIM-028]` given one Node of a stopped Zone, when its box is clicked, then it alone responds again.
- `[REQ-SIM-037]` given a Zone of many Nodes, when it fails by a click or a Scenario's When, then its last Node stops as soon after its first as in a Zone of two.
- `[REQ-SIM-037]` given a clicked Zone Failure one of whose Nodes cannot be stopped, when it ends, then each Node it did stop counts as stopped, and a click on one makes it respond again.
- `[REQ-SIM-037]` given a click making a Node or Zone respond again that cannot be carried out, when the terminal and the Timeline are read, then they name it not injected, and why, in place of its When: "Not injected: Node worker-2 responds again, because …".
- `[REQ-SIM-037]` given a click making a Zone respond again only in part, when the terminal is read, then its line names each Node that did not respond again.

**Edge cases:**

1. A click while the Steps still run changes nothing.
2. A Zone with only some Nodes stopped. A click stops the others.
3. A placed Pod on a stopped Node. A click loses it all the same.
4. A Node that responds again after a click. A further click stops it once more.
5. A click whose Failure cannot be injected, wholly or in part. The terminal and the Timeline name that Failure as not injected, and why, in place of its When. A click making a Node or Zone respond again is named so too `[REQ-SIM-037]`.
6. A click whose Failure was injected only in part. The Nodes it stopped stay stopped, and the run goes on holding the cluster.
7. A click making a Zone respond again only in part. The Nodes that responded again keep responding, and the run goes on holding the cluster `[REQ-SIM-037]`.

**Design:** [system-design.md](system-design.md)

### Opening the Live view in one command

<a id="req-sim-032"></a>
One command starts an Interactive run and opens its Live view in the machine's browser once the terminal names its address `[REQ-SIM-032]`. An Interactive run started on its own opens no browser.

**Done when:**

- `[REQ-SIM-032]` given the command naming a Scenario or none, when it starts, then the Interactive run holds that Scenario or a-zone-that-responds-again-under-a-descheduler. The browser opens its Live view once the terminal names the address.
- `[REQ-SIM-032]` given the command asking for a Kubernetes version or the whole Timeline, when it starts, then the Interactive run takes each as one started on its own does.
- `[REQ-SIM-032]` given the command running, when the terminal is read, then every line stands as the Interactive run reports it, the address line holding its label and the address alone.
- `[REQ-SIM-032]` given the command running, when the engineer stops it at any point, then the lab stops and its Simulated Cluster is removed.
- `[REQ-SIM-032]` given a Finding or a Step left undone, when the command ends, then it fails as an Interactive run does.

**Edge cases:**

1. The machine offers no way to open a browser. The address stays on the terminal and the run goes on.
2. The command names a Scenario matching none. It stops before provisioning, names the Scenarios that exist, and opens no browser.
3. No container runtime is reachable. The command names why and stops, opening no browser.

**Design:** [system-design.md](system-design.md)

### Following a held cluster in its Timeline and terminal

<a id="req-sim-033"></a>
An Interactive run's Timeline keeps recording while its cluster is held, each click joining it as a When `[REQ-SIM-033]`. The terminal names each moment that changes the Placement, as a Wait narrative does, opening with its elapsed time.

**Done when:**

- `[REQ-SIM-033]` given a click on a held cluster, when the Timeline is read, then it holds at its moment the When the terminal prints, worded and indented as before.
- `[REQ-SIM-033]` given a held cluster, when Nodes or Pods change after a click or a pass, then its Timeline holds each change and the control plane's reason, the Descheduler's evictions apart from a Failure's.
- `[REQ-SIM-033]` given a held cluster, when the lab stops, then the Timeline is filed with every Entry up to then, and only then is its line printed.
- `[REQ-SIM-033]` given a held cluster, when Nodes turn Ready or NotReady or Pods are evicted or scheduled, then the terminal names each as a Wait narrative does. Scheduled Pods name their Node and Running time; Descheduler evictions stand apart, none linked to a replacement.
- `[REQ-SIM-033]` given a pass evicting nothing, or an Entry changing no Placement, when the terminal is read, then no line names it.
- `[REQ-SIM-033]` given a line naming a moment, when read, then it is indented as a Wait narrative's line and opens with its elapsed time to the millisecond, the filed Timeline holding its Entry.
- `[REQ-SIM-033]` given a-zone-that-responds-again-under-a-descheduler held, when its Zone is clicked to stop, then to respond again, then the terminal names its Nodes turning NotReady and Ready, the Pods evicted and moved back, and their Nodes.
- `[REQ-SIM-033]` given any Scenario, when a run carries it out, then its report, Timeline and Live view stay as before.

**Edge cases:**

1. The engineer stops the lab while the Steps still run. The Timeline is filed as for a Scenario that fails part-way.
2. A click changing nothing, on a Pending Pod or while the Steps run. The Timeline gains no When.
3. The Timeline cannot be written when the lab stops. The terminal names the Timeline it could not write.
4. A pass while the Steps still run. Only the next Wait narrative names its evictions.
5. A replacement no Node has room for. No terminal line names it.
6. No Descheduler on an interval. The terminal names the clicks and the moments after them alone.
7. Pods the lab places while provisioning the Topology. No terminal line names them.
8. The terminal stops taking lines while the cluster is held. When the lab stops, it still files the Timeline and removes the cluster, dropping lines not yet printed.
9. A Descheduler on an interval stops while the cluster is held. The terminal names why as a run names a Step left undone, the cluster stays held, and the lab fails once stopped.

**Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- <a id="req-sim-010"></a>`REQ-SIM-010`, a count of Pods lost judged at two moments. Superseded by `REQ-SIM-027`.

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Whether more of the lab's own tests share one Simulated Cluster, once their run times are measured.
- Whether the lab scripts a Failure over time, such as a Pod dying after a delay, at the cost of a second injection surface.
- Whether a held cluster's terminal names Pending Pods with the scheduler's reason, or the time of day.
- Whether a Timeline growing while a cluster is held has a bound.
- Whether the Live view's Timeline sets a Wait narrative's moments apart, and whether another machine reaches the Live view.
- Whether a PodDisruptionBudget joins the lab as a Given.
- Whether a written Pod Failure paces itself to the Floor or leaves a fall for the Then, and whether it repeats until the Placement is even.
- How long a Timeline file is kept, how it is reached beyond its file, and whether a Scenario asserts on it.
- Whether the lab promises anything about production manifests, needing a named environment's Topology and admission behavior.
- Whether a Deployment declares Failures it survives in sequence, and whether a run names those injected.
- How a DaemonSet or another kind declares its Failure tolerance.
- Which Nodes count towards the Floor of a Deployment confined to one NodePool.
- Whether the lab judges a Placement after a Failure against the best achievable spread.
- Whether Nodes also sit on racks, between Zone and Node.
- Which Scenario first fails a Node by position, what a missing position yields, and whether a Failure says if its Nodes share a Zone.
- What a Scenario asserts when every Zone, or every Node of a label value, fails.
- Whether a Failure by label names several values, or one NodePool.
- Whether taints take PreferNoSchedule or NoExecute.
- Whether a Deployment selects a NodePool beyond a Request only its Nodes fit.
- What limits a Topology's Zone count.
- Which Failure follows a-zone-that-stops-responding: a Pod, or a removed Node.
- Whether a Scenario demonstrates a Customization or full access, and whether the lab warns when full access drops a setting it depends on.
- Whether a specification holds several full accesses, each applying in the order written among its other Customizations.
- Whether a Scenario rolls out a change to a deployed specification.
- Whether a Then asserts which Pods stay Pending, or their count alone.
- Whether the lab's vocabulary names a Pod's limits.
- Whether a run names one Startup time for every Pod, or runs its Scenarios one at a time.
- Whether a removed Pod takes a time to stop.
- Which further specifications and settings arrive, such as Zone spread only, Soft spread and Pod affinity, and what demonstrating a weak one asserts.
