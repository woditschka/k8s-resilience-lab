<!-- harness: 2026-09-30 -->
# Product Requirements Document: k8s-resilience-lab

<!-- AGENT: This PRD is narrative prose. State WHAT the system does and WHY it matters. Never HOW — mechanism (flags, exit codes, file layouts, algorithms) lives in system-design.md. Never the decision trail — alternatives and trade-offs live in adr/. -->
<!-- AGENT: Annotate each requirement inline with its [REQ-XX-NNN] tag where the prose expresses it, and give it one "Done when" acceptance bullet carrying the same tag. The prose is the intent; the tagged bullet is the bounded, testable contract. Drop an <a id="req-xx-nnn"></a> anchor at first mention so other docs deep-link to it. -->
<!-- AGENT: A requirement is active by being in the narrative — there is no per-requirement Status field. Retire one by moving it to the Superseded list; never renumber an ID. -->

## Context

A team writes Kubernetes manifests that declare where their Pods may land: spread rules across Zones, anti-affinity between Nodes, disruption budgets. Whether they produce the intended Placement, and whether it recovers after a Zone or Node is lost, is normally discovered in a real outage. Proving it earlier needs a costly cluster per developer and build.

k8s-resilience-lab is a learning and verification lab for that question. It starts a Simulated Cluster — a genuine Kubernetes control plane with fake Nodes and Pods — provisions a Topology, creates a Deployment, and reports the resulting Placement. Scheduling and controller behavior are real. Nothing runs inside a Pod, so no container image is pulled.

The lab serves two uses. Unattended, it runs each Scenario and asserts its Placement outcome, on a developer machine or in a build. Interactively, it shows the Simulated Cluster live and takes a Failure injected by hand.

## Goals

| ID | Goal | Success Metric |
|----|------|----------------|
| G-1 | Placement under a declared Topology is verified automatically, with no real cluster involved. | Every Scenario runs unattended on a developer machine and in a build, needing only a container engine. |
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
A run the engineer stops, or one its container engine cuts short, still removes every Simulated Cluster it started and names why it ended `[REQ-SIM-034]`.

<a id="req-sim-036"></a>
Wherever a first stop lands, the run names the stop and what it waited on, and none of its clusters remains `[REQ-SIM-036]`.

<a id="req-sim-003"></a>
An engineer names one Scenario to re-run it alone `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
One command decides whether the lab itself is sound, and the repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The same command holds the lab to its declared dependencies and to closing every connection it opens, and the build keeps it free of known vulnerabilities `[REQ-SIM-007]`.

<a id="req-sim-035"></a>
The lab's own tests run on their own by kind, needing a container engine or not, so those needing none wait for no Simulated Cluster `[REQ-SIM-035]`. The one command runs both.

**Done when:**

- `[REQ-SIM-001]` a-deployment-placed-for-resilience, run with no Simulated Cluster up, gets three Zones of three Ready general Nodes and nine Pods Settled Running, one per Node.
- `[REQ-SIM-001]` beside an earlier run's Nodes and Pods, a run reports only its own, each Zone and Node by position counted from one.
- `[REQ-SIM-001]` a run's cluster mirrors the Kubernetes version it names, or the pinned default, reporting which.
- `[REQ-SIM-001]` beside other containers and a second run, an ending Scenario removes its own Simulated Cluster alone.
- `[REQ-SIM-002]` with a reachable container engine, a no-argument invocation runs every Scenario needing a Simulated Cluster afresh, reporting name and outcome.
- `[REQ-SIM-002]` every Scenario of the lab reaches the outcome it asserts.
- `[REQ-SIM-002]` on failing lab rules or no reachable container engine, the lab names why and stops before provisioning.
- `[REQ-SIM-014]` in a full run, no Scenario waits for another, and each one's lines stand together in declared order.
- `[REQ-SIM-014]` Scenarios carried out at once each report a Placement and Timeline naming only their own Nodes and Pods.
- `[REQ-SIM-014]` a Scenario failing part-way is reported, the others reach their outcomes, and no Simulated Cluster remains.
- `[REQ-SIM-014]` under the one command, the lab's own tests needing a Simulated Cluster run at once, and those judging a full run share one.
- `[REQ-SIM-034]` a stopped run of every Scenario ends each Scenario still running as one failing part-way, leaving no Simulated Cluster or Descheduler of the run.
- `[REQ-SIM-034]` a Simulated Cluster the container engine created without its answer reaching the lab is removed when its Scenario ends.
- `[REQ-SIM-034]` a Kubernetes version or Descheduler obtained only in part is reported at run start as one the lab cannot obtain, named.
- `[REQ-SIM-036]` a stop while Simulated Clusters start ends each Scenario still starting naming the stop, never a wait that timed out.
- `[REQ-SIM-036]` a stop while a Scenario Waits for its Nodes to be Ready ends it naming the stop and the Nodes it waited on, no Deployment.
- `[REQ-SIM-036]` when the container engine fails a cluster's creation with a fault of its own, a cluster created under that name is removed when its Scenario ends; a name another container holds stays untouched.
- `[REQ-SIM-036]` a cluster the container engine still creates when the lab stops waiting on it is removed once created.
- `[REQ-SIM-036]` a second stop while a stopped run removes its clusters ends the run at once.
- `[REQ-SIM-003]` naming one Scenario runs only it, reporting as a full run does.
- `[REQ-SIM-003]` a name matching no Scenario stops the run before provisioning, naming the Scenarios that exist.
- `[REQ-SIM-004]` the one command runs the lab's rules, tests and build on the declared toolchain. Any failure, or a part able to reach a cluster the lab did not provision, fails it, named.
- `[REQ-SIM-004]` the repository holds exactly one implementation of the lab.
- `[REQ-SIM-007]` a dependency differing from its declaration, or named like a permitted one, fails the one command, named.
- `[REQ-SIM-007]` a known vulnerability in code the lab calls fails the build, named, on each push to main, each pull request and once a week. The one command checks for none.
- `[REQ-SIM-007]` when the lab's own tests end, no connection the lab opened to a Simulated Cluster stays open.
- `[REQ-SIM-035]` given no reachable container engine, the tests needing none each run and pass, and no Simulated Cluster starts.
- `[REQ-SIM-035]` with a reachable container engine, the tests needing one run alone.
- `[REQ-SIM-035]` with no reachable container engine, each test needing one fails, naming the container engine, under the one command or alone.
- `[REQ-SIM-035]` the one command runs every test of both kinds, on a developer machine as in a build.

**Edge cases:**

1. A Deployment declares more replicas than its Topology holds. Unplaceable Pods are reported Pending, failing a Scenario asserting every Pod Running.
2. The container engine answers at an unconventional location. The run finds it.
3. A stale container engine address sits beside a live one. The run reaches the live one.
4. A run names a Kubernetes version the lab cannot obtain. It stops, naming it, before any Scenario starts.
5. The container engine cannot start one Scenario's Simulated Cluster while others start. That Scenario ends at its first Step, naming why; the others run on.
6. The engineer stops a run while its Scenarios still provision. Every Simulated Cluster started so far is removed `[REQ-SIM-034]`.
7. A run is killed outright. Its Simulated Clusters stay for the engineer to remove `[REQ-SIM-034]`.
8. A second stop while the run removes its clusters. The run ends at once, and clusters not yet removed stay, as after a kill `[REQ-SIM-036]`.

**ADR:** [ADR: Kubernetes version pin for the Simulated Cluster](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Deployment's Pods

<a id="req-sim-009"></a>
<a id="req-sim-025"></a>
A run names each Deployment by its Kubernetes kind `[REQ-SIM-025]`. A Scenario's Deployment declares one of the predefined Pod specifications the Scenarios define, one per intent `[REQ-SIM-009]`. Customizations change a copy before deployment, through the lab's vocabulary or one full access to any other Pod setting, which no Scenario uses yet.

**Done when:**

- `[REQ-SIM-025]` a Deployment's Given line names its kind and replicas: 'a Deployment checkout of 9 replicas'.
- `[REQ-SIM-009]` every Scenario's Deployment line declares one predefined Pod specification, at most one Request and any Customizations, nothing else.
- `[REQ-SIM-009]` every setting a predefined specification holds stands in it, under its Kubernetes name or a Named setting.
- `[REQ-SIM-009]` a declared specification's Pods read back the definition's settings, each Customization's standing over them.
- `[REQ-SIM-009]` on a-deployment-placed-for-resilience's Topology, nine Pods declaring Default placement all settle Running, no count per Zone promised.
- `[REQ-SIM-009]` two Deployments of one Scenario are each placed by its own specification, the other's Customizations never reaching it.
- `[REQ-SIM-009]` of two Customizations naming one setting, through the lab's vocabulary or the one full access, the later stands.
- `[REQ-SIM-009]` a specification holding more than one full access is refused at its Deployment's Given, naming why, ending the Scenario there.
- `[REQ-SIM-009]` a Deployment's full access stands in its Pods but for the waiting time before Pods leave an unresponsive Node, and they count as that Deployment's.

**Edge cases:**

1. A Deployment declares Resilient placement on a Topology of one Zone. Its Pods still land one per Node.
2. Two Deployments of one Scenario declare the same specification. Each is spread on its own, the other's Pods not counting towards its Skew.
3. A Customization names a setting its specification does not hold. The specification gains it.
4. Full access leaves out a setting the lab depends on. That setting holds, and the Deployment deploys.
5. Full access names a setting the control plane refuses. The Scenario ends at the Step deploying it, as for any refused Customization.
6. Full access sets a setting a later Customization names, such as the Deployment line's Request. The later one stands.
7. A predefined specification holds a full access and the Deployment line adds a second. The specification is refused.

**ADR:** [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Stating what decides a Placement

<a id="req-sim-015"></a>
Every value deciding a Placement stands on a Scenario's own lines, stated or named there, and the lab gives none `[REQ-SIM-015]`. Each further Named Node capacity arrives with its first Scenario.

**Done when:**

- `[REQ-SIM-015]` a NodePool stating no Node capacity is refused where stated.
- `[REQ-SIM-015]` a NodePool naming a Named Node capacity provisions Nodes of that cpu, memory and Pod count, which the Given line reports by name.
- `[REQ-SIM-015]` every predefined Pod specification's definition names its own Startup time.
- `[REQ-SIM-015]` each Named Node capacity and Pod specification the Scenarios use is defined once beside them, and the lab defines none.
- `[REQ-SIM-015]` every run reports at its start the lab's two waiting times, and no Node capacity or Startup time of its own.

**Edge cases:**

1. A predefined Pod specification with no Startup time is refused where defined.
2. A predefined Pod specification with no Request. Its Pods ask for no cpu and no memory.
3. Two Named Node capacities or Pod specifications defined under one name are reported where defined, so each name stands for one set of values.

**Design:** [system-design.md](system-design.md)

### Fitting a Deployment's Pods onto the Nodes

<a id="req-sim-011"></a>
A Node has room for only so much, and a Pod may Request a share of it `[REQ-SIM-011]`. After a Failure, only responding Nodes offer room, as no-room-after-a-zone-stops-responding shows.

<a id="req-sim-024"></a>
A Deployment line names Standard Pod, a quarter of the Standard capacity, or GPU Pod beside its replicas and the specification `[REQ-SIM-024]`.

**Done when:**

- `[REQ-SIM-011]` a NodePool stating its Nodes' cpu, memory and Pod count gives every Node that capacity, reported in that order.
- `[REQ-SIM-011]` a NodePool stating cpu and memory alone is refused where stated.
- `[REQ-SIM-011]` a Request a specification states stands under its Kubernetes name; without one its Pods ask for nothing and the spread alone decides.
- `[REQ-SIM-011]` a Deployment requesting more cpu than its Topology has room for settles with the Pods that fit Running, the rest Pending.
- `[REQ-SIM-011]` given Nodes with room for one Pod each, a nine-Pod Deployment settles with nine Pods Running, one per Node.
- `[REQ-SIM-011]` from that Placement, when one Zone stops responding, it settles at six Pods Running and three Pending, never below its Floor of six.
- `[REQ-SIM-024]` every Scenario's Deployment line names Standard Pod or GPU Pod or states its own Request, only one.
- `[REQ-SIM-024]` no predefined Pod specification holds a Request, so a Deployment naming Tolerating GPU Nodes alone asks for nothing.
- `[REQ-SIM-024]` for a Deployment naming Standard Pod or GPU Pod, the lines beneath its Given line report that Request.

**Edge cases:**

1. A NodePool stating a Node capacity of no Pod, no cpu, or no memory is refused where stated.
2. A Deployment's Request exceeds every Node's room. Every Pod of it stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter bound decides.

**Design:** [system-design.md](system-design.md)

### Holding Nodes of different kinds in a Zone

<a id="req-sim-013"></a>
A Zone holds NodePools of identical Nodes, the same in every Zone `[REQ-SIM-013]`.

**Done when:**

- `[REQ-SIM-013]` three Zones of two general Nodes and one larger gpu Node each provision as three Ready Nodes per Zone, of their NodePool's capacity and label.
- `[REQ-SIM-013]` that Topology's Placement numbers each Zone's Nodes one to three, general's two first.
- `[REQ-SIM-013]` at run start, the report gives each NodePool's name, Node count per Zone, and cpu, memory and Pod count.
- `[REQ-SIM-013]` in that Topology, a Request only gpu Nodes have room for settles its Running Pods on gpu Nodes alone.
- `[REQ-SIM-013]` every Scenario declaring one NodePool names it general.

**Edge cases:** each is refused where stated.

1. A NodePool of no Node.
2. Two NodePools of one Topology under one name, their Nodes indistinguishable.
3. A NodePool name Kubernetes cannot hold as a label value.
4. A NodePool stating no Node capacity.
5. A NodePool stating no name, or a Topology stating no NodePool.
6. A second Topology in one Scenario, whether or not the first is provisioned. The refusal names the first, which the cluster keeps.

**Design:** [system-design.md](system-design.md)

### Marking a NodePool's Nodes with labels

<a id="req-sim-017"></a>
A NodePool may state Kubernetes labels for its Nodes, beside the lab's own Zone, hostname and NodePool labels `[REQ-SIM-017]`. A Then counts a Deployment's Pods per value of a stated label, and the Even spread spreads over one.

**Done when:**

- `[REQ-SIM-017]` each Node of a NodePool stating labels carries them beside the lab's own.
- `[REQ-SIM-017]` the Given line reports each NodePool's stated labels after its capacity, under their Kubernetes keys.
- `[REQ-SIM-017]` a NodePool stating a label under a key the lab sets is refused where stated.
- `[REQ-SIM-017]` a Then stating a label and a Pod count records a Finding unless each value the NodePools state holds that count.
- `[REQ-SIM-017]` over three Zones of one-Node NodePools labelled on-demand and spot, a-deployment-spread-over-on-demand-and-spot runs six Pods, two per Zone, three per label value.

**Edge cases:**

1. A key or value Kubernetes cannot hold as a label is refused where stated.
2. One NodePool stating one key twice is refused where stated.
3. A NodePool stating no label. Its Nodes carry the lab's labels alone.
4. A Then stating a label no NodePool states ends the Scenario at that Then, naming the label.
5. A label value whose Nodes hold no Pod of the Deployment counts as none, so a Then asserting more records a Finding.
6. Pods on Nodes whose NodePool states no value of that label count towards no value.

**Design:** [system-design.md](system-design.md)

### Starting a Deployment's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready, so a Deployment replacing Pods after a Failure runs short meanwhile `[REQ-SIM-012]`.

**Done when:**

- `[REQ-SIM-012]` each placed Pod is Ready between its shortest and longest Startup time, both included, specified or customized, after its placement.
- `[REQ-SIM-012]` a Deployment customized with one Startup time, in whole seconds or milliseconds, has no Pod Ready sooner; of two such Customizations, the later stands.
- `[REQ-SIM-012]` a reported Placement counts a placed Pod inside its Startup time Pending on its Node, and is not yet Settled.
- `[REQ-SIM-012]` a run names in its report a declared predefined specification's shortest and longest Startup time.

**Edge cases:**

1. A Startup time of zero. The Pod is Ready once placed.
2. A shortest Startup time longer than its longest, or a negative time, is refused where stated, ending the Scenario at the Step declaring that Deployment.
3. Equal shortest and longest Startup times. Every Pod takes that time.

**Design:** [system-design.md](system-design.md)

### Surviving a Node or Zone Failure

<a id="req-sim-008"></a>
A Scenario injects a Failure and judges the Placement on both sides of it `[REQ-SIM-008]`. A stopped Node stays in the cluster, as a hung machine does, until the control plane moves its Pods away after a shortened waiting time. A Failure costs the spread, accepted while the Pod count holds, so no count per Node is asserted after it, nor per Zone after a Zone Failure.

**Done when:**

- `[REQ-SIM-008]` with nine Pods declaring Resilient placement Settled, three per Zone, a Node or Zone stopping responding has its Pods move away within a few seconds.
- `[REQ-SIM-008]` resettled after one Node or Zone stopped, nine Pods are Running, none on the stopped Nodes; after a Node Failure every Zone still holds three.
- `[REQ-SIM-008]` a-node-that-stops-responding and a-zone-that-stops-responding each fail the same Node or Zone in two runs, reporting which.
- `[REQ-SIM-008]` in either Failure Scenario, the When injecting the Failure names no Node or Zone, and a-node-that-stops-responding's states no waiting time.

**Edge cases:**

1. A stopped Node's Pod cannot be placed elsewhere. It stays Pending with the control plane's reason.
2. A When failing more Nodes or Zones than the Topology holds ends the Scenario at that Step, naming it.
3. A Zone of many Nodes fails, by a When or a click. All its Nodes stop responding at once, the last before the control plane moves the first's Pods.

**ADR:** [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

### Failing every Node of one label value

<a id="req-sim-018"></a>
A Failure may stop, in one Step, every Node carrying one value of a label a NodePool states, as spot-nodes-that-stop-responding demonstrates `[REQ-SIM-018]`.

**Done when:**

- `[REQ-SIM-018]` a When naming a stated label and one of its values stops every Node carrying it in one Step, reported, and their Pods move away within seconds.
- `[REQ-SIM-018]` spot-nodes-that-stop-responding's When names the label and value alone, no Node, Zone, count or waiting time.
- `[REQ-SIM-018]` over three Zones of two on-demand Nodes and one spot Node each, nine Pods declaring Resilient placement Settle one per Node.
- `[REQ-SIM-018]` given that Placement, when every spot Node stops responding, the Deployment surviving the loss of three Nodes never falls below its Floor of six Ready Pods.
- `[REQ-SIM-018]` once that Failure settles, no Pod is on a stopped Node and every Zone still holds three.

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

- `[REQ-SIM-029]` a When makes every stopped Node, or every Node of every stopped Zone, respond again in one Step, reported as a Failure's When is.
- `[REQ-SIM-029]` in either Scenario, that When names no Node, Zone or count.
- `[REQ-SIM-029]` the next Wait's narrative names each Node that responded again turning Ready.
- `[REQ-SIM-029]` once a-node-that-responds-again Settles with its Node Ready again, that Node holds no Pod, every Zone three, and one Node two.
- `[REQ-SIM-029]` once a-zone-that-responds-again Settles with its Nodes Ready again, that Zone holds no Pod and the nine Pods stay Running elsewhere.

**Edge cases:** each ends the Scenario at that When, naming it.

1. A When making Nodes or Zones respond again while no Node of the Scenario is stopped.
2. A When making Zones respond again while a stopped Node shares its Zone with a Node still responding.

**Design:** [system-design.md](system-design.md)

### Losing Pods of a Deployment

<a id="req-sim-030"></a>
A When loses a stated number of one named Deployment's placed Pods in one Step, as a click on a Pod does in the Live view `[REQ-SIM-030]`. The control plane places each replacement among the Nodes then responding, a Node that responded again included.

**Done when:**

- `[REQ-SIM-030]` a When naming a Deployment with placed Pods and a number loses that many of them in one Step, the run reporting each.
- `[REQ-SIM-030]` the next Wait's narrative names each replacement scheduled for Pods a When lost, and its Node.
- `[REQ-SIM-030]` pods-lost-after-a-node-responds-again's When names the Deployment and the number alone, no Pod, Node or Zone.
- `[REQ-SIM-030]` from a-node-that-responds-again's Settled Placement, losing all nine Pods Settles with one per Node, nine Running, three per Zone.

**Edge cases:** each ends the Scenario at that When, naming it.

1. A When naming a Deployment the Scenario has not declared.
2. A When losing fewer than one Pod, or more than the Deployment has placed.
3. A When losing Pods of a Deployment declaring a Failure tolerance.

**Design:** [system-design.md](system-design.md)

### Moving Pods with the Descheduler

<a id="req-sim-031"></a>
A Scenario's Descheduler evicts running Pods its Descheduler policy names, which the control plane never moves, and their Deployments replace them `[REQ-SIM-031]`. A Given names the policy by its intent, defined beside the Scenarios, and the Descheduler mirrors the run's Kubernetes version. A dry run, leaving no Entry, is no part of the lab.

A When runs one pass, as a-descheduler-pass-after-a-node-responds-again demonstrates `[REQ-SIM-031]`. A Given may instead run a pass on each interval it states, with a Wait for a pass evicting nothing; Settled keeps its meaning.

**Done when:**

- `[REQ-SIM-031]` a policy's Given line names it and the Descheduler version, the lines beneath reporting the policy under Kubernetes' names.
- `[REQ-SIM-031]` a policy's definition mirrors the Descheduler's YAML line for line, the lab adding or inferring nothing.
- `[REQ-SIM-031]` a Scenario declaring a policy, under a Kubernetes version paired with no Descheduler, stops the run before provisioning, naming it; the default version has one.
- `[REQ-SIM-031]` a When runs one pass under the declared policy in one Step ending with the pass, its line naming the policy.
- `[REQ-SIM-031]` the next Wait's narrative names each Pod a pass evicted and each replacement with its Node, apart from a Failure's evictions.
- `[REQ-SIM-031]` from a-node-that-responds-again's Settled Placement, one pass evicts one Pod, Settling with one per Node, nine Running, three per Zone.
- `[REQ-SIM-031]` nothing of a Scenario's Descheduler outlives the Scenario, however it ends.
- `[REQ-SIM-031]` a policy declared with an interval runs a pass each interval until the Scenario ends, its Given line naming the interval.
- `[REQ-SIM-031]` given a Descheduler on an interval, a Wait for a pass evicting nothing ends at the first such pass begun after it.
- `[REQ-SIM-031]` a-zone-that-responds-again-under-a-descheduler, passing every five seconds, ends with nine Pods Running, three per Zone, one per Node, once its Zone fails, responds again and a pass evicts nothing; the Floor held and no When moved them.

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
A NodePool may state NoSchedule taints, keeping off its Nodes every Pod not tolerating them `[REQ-SIM-019]`. A toleration only permits a Pod onto a tainted Node, so a dedicated NodePool's Pods reach it by a Request only its Nodes have room for. A later Deployment, deployed by the next Wait, is the one every Then after it counts, as general-pods-beside-tainted-gpu-nodes demonstrates.

**Done when:**

- `[REQ-SIM-019]` a NodePool's NoSchedule taint is carried by its Nodes and reported on the Given line after the labels, under Kubernetes names.
- `[REQ-SIM-019]` a NodePool stating a taint of another effect is refused where stated.
- `[REQ-SIM-019]` a tolerating specification's Given line reports beneath it the toleration's key, operator, value and effect, and no waiting time.
- `[REQ-SIM-019]` a Then stating a NodePool and a Pod count records a Finding unless that NodePool's Nodes hold that count.
- `[REQ-SIM-019]` over three Zones of two general Nodes and one larger gpu Node tainted NoSchedule each, six Pods declaring Resilient placement all run on general Nodes.
- `[REQ-SIM-019]` from that Placement, three Pods tolerating the taint with a Request only gpu Nodes fit all run on gpu Nodes.

**Edge cases:**

1. A taint leaving out its key, value or effect, or stating a key or value Kubernetes cannot hold, is refused where stated.
2. One NodePool stating one taint key twice is refused where stated.
3. A taint under a key the control plane sets on an unresponsive Node is refused where stated.
4. A toleration of a NoSchedule taint stating a waiting time is refused, ending the Scenario at the Step declaring that Deployment.
5. A Deployment not tolerating a taint finds no room left on the untainted Nodes. Its extra Pods stay Pending, none on a tainted Node.
6. A Then naming a NodePool the Topology lacks ends the Scenario there, naming it.
7. A second Deployment under a name the Scenario already declared is refused, ending the Scenario at that Given.

**Design:** [system-design.md](system-design.md)

### Judging a Deployment's Failure tolerance

<a id="req-sim-027"></a>
A Deployment declares the Zones or Nodes it survives losing `[REQ-SIM-027]`. Its Floor is its replicas less the most Pods those hold evenly spread. One Then judges that every declaring Deployment kept its Floor of Ready Pods since the first Failure.

<a id="req-sim-038"></a>
A Scenario whose stopped Node or Zone responds again judges its Floor too, through the Failure and whatever moves Pods after it `[REQ-SIM-038]`. One losing Pods to a When declares none, since that When refuses a declaring Deployment.

**Done when:**

- `[REQ-SIM-027]` every Deployment line declares Zones or Nodes it survives losing, never Pods.
- `[REQ-SIM-027]` nine replicas over three Zones of three Nodes have a Floor of six surviving one Zone, eight surviving one Node; ten surviving one Zone keep six.
- `[REQ-SIM-027]` a Pod on a stopped Node, or Pending, is judged lost, though the control plane may still report it Ready.
- `[REQ-SIM-027]` a Finding names each declaring Deployment below its Floor at any moment from the first Failure to the Then.
- `[REQ-SIM-027]` each Deployment's judgement line states kept or fell, Floor, declared loss, and lowest Ready count, when and how long.
- `[REQ-SIM-027]` a fall's Finding states how low, when, how long, the Pods lost against the most declared, and their Nodes; an unended fall lasts 'for the rest of the Scenario'.
- `[REQ-SIM-027]` every Failure Scenario but one losing Pods to a When declares what its When fails, and every Floor holds.
- `[REQ-SIM-038]` a-node-that-responds-again declares surviving one Node and keeps its Floor of eight Ready Pods to its Then.
- `[REQ-SIM-038]` a-zone-that-responds-again declares surviving one Zone and keeps its Floor of six Ready Pods to its Then.
- `[REQ-SIM-038]` a-descheduler-pass-after-a-node-responds-again declares surviving one Node and keeps its Floor of eight through the Failure and the pass's eviction.
- `[REQ-SIM-038]` pods-lost-after-a-node-responds-again declares no Failure tolerance, and its report judges no Floor.
- `[REQ-SIM-038]` each of those four Scenarios still Settles at the Placement its other Thens assert.

**Not yet in force:**

- `[REQ-SIM-027]` a Then naming one Deployment is decided by that Floor alone.
- `[REQ-SIM-027]` a Then asserting a Floor fell records a Finding when none fell, which a Scenario pinning a Deployment to one lost Zone proves.
- `[REQ-SIM-027]` once a Floor is crossed, the next Wait narrative names each fall and return.

**Edge cases:**

1. A count below zero, or reaching the Topology's own, ends the Scenario at that Given, naming it.
2. A count of none. The Floor is every replica.
3. A Then with no Failure before it, or no declaring Deployment, ends the Scenario there, naming it.
4. An Interactive run reaches a Deployment declaring what it survives losing after the first When. That declaration is not carried out, and every Step reported before it stands.

**Design:** [system-design.md](system-design.md)

### Explaining a Placement after the run

<a id="req-sim-005"></a>
A reported Placement says where Pods landed, and a Timeline says how they got there `[REQ-SIM-005]`. Every Scenario, failed or not, files its Timeline in one place under the working directory, and the lab removes none.

**Done when:**

- `[REQ-SIM-005]` when a Scenario ends, one line names its Timeline's entry count and file, which keeps every Entry.
- `[REQ-SIM-005]` asked for the whole Timeline, a run prints every entry in the order it happened when a Scenario ends.
- `[REQ-SIM-005]` from its Topology and Deployment on, a Scenario's Timeline holds every change to its Nodes, Pods and Deployment objects, and the control plane's reason for each Pod placed or left Pending.
- `[REQ-SIM-005]` two Scenarios of a run, or two runs at once, each leave their own Timeline file, none replacing another.
- `[REQ-SIM-005]` a Scenario's Timeline file stays readable after its Simulated Cluster is removed.

**Edge cases:**

1. A Scenario failing part-way still files its Timeline and prints its line.
2. The place Timeline files collect in is missing at run start. The run creates it.
3. A Timeline cannot be written. The run reports the Scenario's outcome and names the Timeline it could not write.

**Design:** [system-design.md](system-design.md)

### Reading the Placement under every Wait

<a id="req-sim-021"></a>
Under every Wait, a run shows how the Placement changed and where every Pod stands `[REQ-SIM-021]`.

**Done when:**

- `[REQ-SIM-021]` every Wait, reached or not, prints after its line a separator, a Placement board and a second separator, before any Then or next Step, each separator spanning the board.
- `[REQ-SIM-021]` no separator line stands between a Then and the When after it.
- `[REQ-SIM-021]` a board gives every Node one line under its Zone, naming its NodePool and every Pod on it by Deployment name and own suffix, two Deployments included.
- `[REQ-SIM-021]` a board marks exactly the Pods new on each Node since the previous one, and lists Pods left Pending beneath the Nodes.
- `[REQ-SIM-021]` a Wait's narrative names each Pod placed since the previous Wait, its Node and when it reached Running, in order.
- `[REQ-SIM-021]` after a Failure, the next Wait's narrative names in order each Node turning NotReady, Pod evicted and replacement scheduled, no eviction linked to a replacement.
- `[REQ-SIM-021]` a Wait's narrative names each Pod left Pending when it ends, with the scheduler's reason.
- `[REQ-SIM-021]` when nothing changed the Placement since the previous Wait, the separator and board follow its line with no narrative.
- `[REQ-SIM-021]` in a Timeline's file, as in the whole Timeline, each Step stands at its moment among the Entries.
- `[REQ-SIM-021]` a report ends with the last board, one Skew line for the judged Deployment, a separator and the Timeline's line, no other summary.
- `[REQ-SIM-021]` a filed Timeline's line ends at the file's name, under a separator.
- `[REQ-SIM-021]` a heavier separator follows the run's opening lines and parts two Scenarios.

**Edge cases:**

1. The board under the Wait for every Node Ready holds no Pod. Every Node still has its empty line.
2. The first board holding a Deployment's Pods marks them all new.
3. A Failure evicts a Pod and a replacement is placed. The narrative names both in order; the board shows the replacement alone, marked new.
4. A Scenario ends at a Wait it does not reach. That Wait still prints its narrative, separators and board up to the end.
5. A Scenario ends before any Deployment is deployed. Its boards list Nodes with no Pod, and no Skew line follows.
6. Pods the lab places itself while provisioning the Topology appear in neither a narrative nor a board.
7. A Wait no Then judges, such as one before a When. Its board still closes with a second separator.
8. A run holds one Scenario. One heavier separator follows the run's opening lines, and none follows the Scenario.

**Design:** [system-design.md](system-design.md)

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as Steps, one per line: Given declares, When changes the cluster, Wait waits for a cluster state, Then asserts the Placement `[REQ-SIM-006]`.

<a id="req-sim-020"></a>
Every Scenario of the lab sits in one source, each run and reported under its Scenario name, the condition its function names `[REQ-SIM-020]`.

**Done when:**

- `[REQ-SIM-006]` each Scenario line is one Given, When, Wait or Then Step handling no error, each name one the lab offers or a full access name.
- `[REQ-SIM-006]` each Wait is a Step reporting its own line, one before the first Then and one after every Failure.
- `[REQ-SIM-006]` when a declared Topology's or Deployment's Wait runs, every declared Node or Pod is Ready and its line names the time taken, whether written with its Given or alone.
- `[REQ-SIM-006]` a Then with no Wait since the Given or Failure before it, or a Failure before the first Wait, ends the Scenario there, naming it.
- `[REQ-SIM-006]` a Wait stating no time whose Pods are not all Ready within the lab's waiting time ends the Scenario there, naming them.
- `[REQ-SIM-006]` a Wait stating no time that reaches its state names the time taken, no time it was held to, and the Scenario goes on.
- `[REQ-SIM-006]` a Wait stating a time names that time and the time taken when its state is reached inside it; otherwise the Scenario ends there, the line naming that time alone.
- `[REQ-SIM-006]` a Wait stating a time above the lab's waiting time is refused, naming that waiting time.
- `[REQ-SIM-006]` a Wait stating the least time a refusal names as admitted is reached inside it when the cluster is already in that state.
- `[REQ-SIM-006]` the Thens after a Wait judge a Placement that has stopped changing, even with Pods still Pending.
- `[REQ-SIM-006]` every Failure Scenario states a time on the Wait after its Failure, none on those after its Givens.
- `[REQ-SIM-006]` a Step that cannot be carried out ends the Scenario there, naming it; an unmet Then only reports a Finding.
- `[REQ-SIM-020]` one source holds every Scenario and no machinery, beside a README naming each by Scenario name with its asserted outcome.
- `[REQ-SIM-020]` a run reports each Scenario by the condition its function names, in lowercase words joined by hyphens, such as a-node-that-stops-responding.

**Edge cases:**

1. Two Waits in a row. The second reports a wait of almost none, not a refusal.
2. A Wait for every Pod before any Deployment is declared reports that nothing was waited for.
3. A Wait stating a time of no seconds is refused where stated.
4. A Wait for a Settled Placement before any Deployment is declared is reached at once.
5. Two Scenarios whose functions name one condition are refused by the lab's rules before provisioning, naming both `[REQ-SIM-020]`.
6. A Scenario whose function names no condition is refused by the lab's rules before provisioning, naming the function `[REQ-SIM-020]`.
7. A Wait stating no time reaches its state only after the lab's waiting time. A Wait for a pass evicting nothing can take that long, waiting for the pass, then for Settled. It is reported reached, naming the time taken.

**ADR:** [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

### Reading a Scenario by subject

<a id="req-sim-016"></a>
<a id="req-sim-026"></a>
Each name a Scenario writes shows its subject, a Pod, a Node, or the Scenario with its Steps and Topology, unless the Scenarios define it `[REQ-SIM-016]`. Scenario code stays idiomatic Go, and each name the lab offers shows where it is defined `[REQ-SIM-026]`. The Scenarios define Named settings, and a Scenario states any spread but the Even spread itself.

**Done when:**

- `[REQ-SIM-016]` each name a Scenario source writes shows its subject or is one the Scenarios define, and each setting's value stands under its Kubernetes name, never by position alone.
- `[REQ-SIM-026]` every Scenario source is idiomatic Go passing the lab's format and lint checks; each name the lab offers, inline Customizations included, shows where it is defined, and none is renamed.
- `[REQ-SIM-016]` the Named setting for leaving an unresponsive Node holds the not-ready and unreachable tolerations at the lab's waiting time alone, and every specification names it.
- `[REQ-SIM-016]` the Even spread holds one topology spread over the subject given alone, and Resilient placement names it over Zones and over Nodes.
- `[REQ-SIM-016]` a declared specification's Given line, customized or not, names the intent, every setting its Pods carry beneath, one per line under its Kubernetes name.
- `[REQ-SIM-016]` the lines beneath Resilient placement's Given line report both spreads in full.
- `[REQ-SIM-016]` a setting of several values leaving out a value or stating it as zero is refused where stated.
- `[REQ-SIM-016]` every count a Scenario states reads bare, replicas included, in a Step stating its subject; a Node's room reads under the Node.

**Edge cases:**

1. A Deployment declares Default placement, which holds no spread. The lines beneath its Given line report the two tolerations alone.
2. A Customization writes another time for either toleration. The lab's waiting time holds and is reported.
3. A setting of several values leaves out a value Kubernetes would default. It is refused all the same, since no value deciding a Placement falls to a default. A Scenario needing a zero sets it through full access.

**ADR:** [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) · [ADR: Scenario code stays idiomatic Go, read by subject](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)  ·  **Design:** [system-design.md](system-design.md)

### Watching a Scenario live

<a id="req-sim-022"></a>
An engineer starts an Interactive run naming one Scenario and opens its Live view in a browser `[REQ-SIM-022]`. Its Steps run up to its first When, and its Simulated Cluster is then held until the lab stops. The Live view draws that cluster alone as boxes, and only a Failure injected there changes it.

**Done when:**

- `[REQ-SIM-022]` an Interactive run naming a Scenario runs its Steps up to its first When, reporting as a run does, and names the Live view's address.
- `[REQ-SIM-022]` a Live view open while the Steps run shows each Placement change as the lab observes it, with no reload.
- `[REQ-SIM-022]` in the Live view, each Zone's box holds its Nodes' boxes, each holding its Pods by Deployment name and own suffix, Pending Pods apart.
- `[REQ-SIM-022]` a Node's box names its NodePool and stated labels, and gives Pods, cpu and memory as capacity, requests and left.
- `[REQ-SIM-022]` a Pod's box, placed or Pending, gives its Request as cpu and memory under the word requests, cpu 0 and memory 0 when none is declared.
- `[REQ-SIM-022]` the page header names k8s-resilience-lab, a separator such as a hyphen, then the Scenario name.
- `[REQ-SIM-022]` a Node's or placed Pod's box opens with its name and, top right, its Ready state by colour and word. A Pod's requests stand beneath, its box as wide as its Node's.
- `[REQ-SIM-022]` a Pod new on its Node has its box marked as a Placement board marks it, and the mark fades.
- `[REQ-SIM-022]` the Live view fits a narrow window and follows a light or dark setting, loading nothing from another host.
- `[REQ-SIM-022]` with the Steps run and no Failure injected, the cluster stays put until the engineer stops the lab, removing it.

<a id="req-sim-023"></a>
**Not yet in force.** Later slices add the following, first the Timeline beneath the boxes, deciding with it whether Zones stack in a column beside a pane on a laptop `[REQ-SIM-023]`. Edge case 1 holds until the controls ship.

- `[REQ-SIM-023]` each Entry joining the Timeline appears beneath the boxes in order, with no reload, each Step at its moment and each Entry showing at a glance what changed.
- `[REQ-SIM-022]` an Interactive run naming no Scenario offers every Scenario by name in the Live view, provisioning nothing until one is picked.
- `[REQ-SIM-022]` picking another Scenario or resetting, with a cluster held or Steps running, stops the Steps and removes the cluster before provisioning a fresh one, each reported.
- `[REQ-SIM-022]` when the engineer chooses every Step, picking or resetting a Scenario runs and reports them all, then holds the cluster.

**Edge cases:**

1. An Interactive run names no Scenario, two, or one matching none. It stops before provisioning, naming the Scenarios that exist.
2. A Scenario with no When. All its Steps run.
3. A Step that cannot be carried out. The terminal names it as a run does, holding the cluster as that Step left it.
4. A Then records a Finding. The terminal reports it as a run does, and later Steps still run.
5. The Live view opens after the Steps ran, or in two browsers at once. Each shows the Placement as it stands.
6. Pods the lab places itself while provisioning the Topology appear nowhere in the Live view.
7. A stop while Steps still run stops them and removes the Simulated Cluster.
8. No container engine is reachable. The Interactive run names why and stops before offering the Live view.

**Design:** [system-design.md](system-design.md)

### Injecting a Failure from the Live view

<a id="req-sim-028"></a>
An engineer clicks a Node's, Zone's or placed Pod's box in the Live view to inject a Failure `[REQ-SIM-028]`. A second click makes a stopped Node or Zone respond again; a lost Pod has no recovery.

<a id="req-sim-037"></a>
The Nodes of one Failure stop together, by a click or a Scenario's When, so a Zone of many Nodes fails as fast as one of two `[REQ-SIM-037]`.

**Done when:**

- `[REQ-SIM-028]` clicking a held cluster's Node box stops that Node as a Node Failure does, asking no confirmation.
- `[REQ-SIM-028]` clicking a held cluster's Zone box stops all its Nodes at once, as a Zone Failure does.
- `[REQ-SIM-028]` clicking a held cluster's placed Pod loses it, and its Deployment replaces it.
- `[REQ-SIM-028]` each click prints one When on the terminal, indented as a run's, naming the Node or Zone that stops responding or the Pod lost, a Pod as the Live view names it.
- `[REQ-SIM-028]` clicking a Pending Pod or any other part of the page changes nothing.
- `[REQ-SIM-028]` clicking a stopped Node, or a Zone whose every Node stopped, makes it respond again, its When naming the Node or Zone that responds again.
- `[REQ-SIM-028]` clicking one Node of a stopped Zone makes it alone respond again.
- `[REQ-SIM-037]` a Zone of many Nodes failing by a click or a When stops its last Node as soon after its first as a Zone of two does.
- `[REQ-SIM-037]` when a clicked Zone Failure, one of whose Nodes cannot be stopped, ends, each Node it did stop counts as stopped, and a click on one makes it respond again.
- `[REQ-SIM-037]` a click making a Node or Zone respond again that cannot be carried out is named not injected, and why, on the terminal and in the Timeline in place of its When.
- `[REQ-SIM-037]` a click making a Zone respond again only in part has its terminal line name each Node that did not.

**Edge cases:**

1. A click while the Steps still run changes nothing.
2. A Zone with only some Nodes stopped. A click stops the others.
3. A click loses a placed Pod on a stopped Node all the same.
4. A Node that responded again after a click. A further click stops it again.
5. A click whose Failure cannot be injected, wholly or in part. The terminal and the Timeline name that Failure as not injected, and why, in place of its When. A click making a Node or Zone respond again is named so too `[REQ-SIM-037]`.
6. A click whose Failure was injected in part. The Nodes it stopped stay stopped, and the run goes on holding the cluster.
7. A click making a Zone respond again only in part. The Nodes that responded again keep responding, and the run goes on holding the cluster `[REQ-SIM-037]`.

**Design:** [system-design.md](system-design.md)

### Opening the Live view in one command

<a id="req-sim-032"></a>
One command starts an Interactive run and opens its Live view in the machine's browser once the terminal names its address `[REQ-SIM-032]`. An Interactive run started on its own opens no browser.

**Done when:**

- `[REQ-SIM-032]` the command starts an Interactive run holding the Scenario it names, or a-zone-that-responds-again-under-a-descheduler, and the browser opens its Live view once the terminal names the address.
- `[REQ-SIM-032]` the command takes a requested Kubernetes version or the whole Timeline as an Interactive run started on its own does.
- `[REQ-SIM-032]` under the command, every terminal line stands as the Interactive run reports it, the address line holding its label and the address alone.
- `[REQ-SIM-032]` stopping the command at any point stops the lab and removes its Simulated Cluster.
- `[REQ-SIM-032]` a Finding or a Step left undone fails the command as it fails an Interactive run.

**Edge cases:**

1. With no way to open a browser, the address stays on the terminal and the run goes on.
2. The command names a Scenario matching none. It stops before provisioning, naming the Scenarios that exist, and opens no browser.
3. No container engine is reachable. The command names why and stops, opening no browser.

**Design:** [system-design.md](system-design.md)

### Following a held cluster in its Timeline and terminal

<a id="req-sim-033"></a>
An Interactive run's Timeline keeps recording while its cluster is held, each click joining it as a When `[REQ-SIM-033]`. The terminal names each moment that changes the Placement, as a Wait narrative does.

**Done when:**

- `[REQ-SIM-033]` a click on a held cluster puts in the Timeline, at its moment, the When the terminal prints, worded and indented as before.
- `[REQ-SIM-033]` a held cluster's Timeline holds each Node and Pod change after a click or a pass, with the control plane's reason, the Descheduler's evictions apart from a Failure's.
- `[REQ-SIM-033]` at the lab's stop, a held cluster's Timeline is filed with every Entry so far, and only then is its line printed.
- `[REQ-SIM-033]` while a cluster is held, the terminal names each Node turning Ready or NotReady and each Pod evicted or scheduled as a Wait narrative does: scheduled Pods with their Node and Running time, Descheduler evictions apart, none linked to a replacement.
- `[REQ-SIM-033]` no terminal line names a pass evicting nothing or an Entry changing no Placement.
- `[REQ-SIM-033]` each line naming a moment is indented as a Wait narrative's and opens with its elapsed time to the millisecond, the filed Timeline holding its Entry.
- `[REQ-SIM-033]` with a-zone-that-responds-again-under-a-descheduler held, clicking its Zone to stop and respond again, the terminal names its Nodes turning NotReady and Ready, and the Pods evicted and moved back, with their Nodes.
- `[REQ-SIM-033]` every Scenario's run report, Timeline and Live view stay as before.

**Edge cases:**

1. A stop while the Steps still run files the Timeline as for a Scenario that fails part-way.
2. A click changing nothing, on a Pending Pod or while the Steps run, adds no When to the Timeline.
3. A Timeline that cannot be written at the lab's stop is named on the terminal.
4. A pass while the Steps run. Only the next Wait narrative names its evictions.
5. No terminal line names a replacement no Node has room for.
6. With no Descheduler on an interval, the terminal names the clicks and the moments after them alone.
7. No terminal line names Pods the lab places while provisioning the Topology.
8. The terminal stops taking lines while the cluster is held. At the lab's stop, it still files the Timeline and removes the cluster, dropping unprinted lines.
9. A Descheduler on an interval stops while the cluster is held. The terminal names why as a run names a Step left undone, the cluster stays held, and the lab fails once stopped.

**Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- <a id="req-sim-010"></a>`REQ-SIM-010`, a count of Pods lost judged at two moments. Superseded by `REQ-SIM-027`.

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Whether more lab tests share one Simulated Cluster, once their run times are measured.
- Whether the lab scripts a Failure over time, such as a Pod dying after a delay, at the cost of a second injection surface.
- Whether a held cluster's terminal names Pending Pods with the scheduler's reason, or the time of day.
- Whether a Timeline growing while a cluster is held has a bound.
- Whether the Live view's Timeline sets a Wait narrative's moments apart, and another machine reaches the Live view.
- Whether a PodDisruptionBudget joins the lab as a Given.
- Whether a written Pod Failure paces itself to the Floor or leaves a fall for the Then, and repeats until the Placement is even.
- How long a Timeline file is kept, how it is reached beyond its file, and whether a Scenario asserts on it.
- Whether the lab promises anything about production manifests, needing a named environment's Topology and admission behavior.
- Whether a Deployment declares Failures it survives in sequence, and a run names those injected.
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
- Whether a Scenario demonstrates a Customization or full access, and the lab warns when full access drops a setting it needs.
- Whether a specification holds several full accesses, each applying in written order among its other Customizations.
- Whether a Scenario rolls out a change to a deployed specification.
- Whether a Then asserts which Pods stay Pending, or their count alone.
- Whether the lab's vocabulary names a Pod's limits.
- Whether a run names one Startup time for every Pod, or runs its Scenarios one at a time.
- Whether a removed Pod takes a time to stop.
- Which further specifications and settings arrive, such as Zone spread only, Soft spread and Pod affinity, and what demonstrating a weak one asserts.
