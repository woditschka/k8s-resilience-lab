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

<a id="req-sim-003"></a>
An engineer names one Scenario to re-run it alone `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
One command decides whether the lab itself is sound, and the repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The same command holds the lab to its declared dependencies, free of known vulnerabilities, and to closing every connection it opens `[REQ-SIM-007]`.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when the Scenario a-deployment-placed-for-resilience runs, then three Zones each hold three Ready Nodes of the NodePool general. The Placement Settles at nine Running Pods, one on every Node.
- `[REQ-SIM-001]` given an earlier run created Nodes and Pods, when a run reports its Placement, then it names only its own, each Zone and Node by its position counted from one.
- `[REQ-SIM-001]` given a run naming a Kubernetes version or none, when it starts, then its Simulated Cluster mirrors that version or the lab's pinned default, and the run reports which.
- `[REQ-SIM-001]` given other containers and a second run on the machine, when a Scenario ends, then its own Simulated Cluster is gone and no other container was touched.
- `[REQ-SIM-002]` given a reachable container runtime, when the lab is invoked with no arguments, then every Scenario needing a Simulated Cluster runs afresh, even after a passed run. Each reports its name and outcome.
- `[REQ-SIM-002]` given the lab's own rules fail or no container runtime is reachable, when the lab is invoked, then the run names why and stops before any Simulated Cluster is provisioned.
- `[REQ-SIM-014]` given a run of every Scenario, when it reports, then no Scenario waited for another, and each Scenario's lines stand together in declared order.
- `[REQ-SIM-014]` given Scenarios carried out at once, when each reports its Placement and Timeline, then each names only its own Nodes and Pods.
- `[REQ-SIM-014]` given one Scenario fails part-way, when the run ends, then the run reports that failure, every other Scenario reached its own outcome, and no Simulated Cluster of the run remains.
- `[REQ-SIM-014]` given the lab's own tests, when the one command runs them, then those needing a Simulated Cluster run at once, and those judging a full run share one.
- `[REQ-SIM-003]` given several Scenarios, when the engineer names one, then only that Scenario runs and reports as a run of every Scenario does.
- `[REQ-SIM-003]` given a name matching no Scenario, when the run starts, then it stops before provisioning and names the Scenarios that exist.
- `[REQ-SIM-004]` given the declared toolchain, when the one command runs, then the lab's rules, tests and build all run. Any one failing, or a part able to reach a cluster the lab did not provision, fails the command and is named.
- `[REQ-SIM-004]` given the repository, when an engineer looks for the lab, then exactly one implementation is present.
- `[REQ-SIM-007]` given a dependency differing from the declared one, carrying a known vulnerability, or named like a permitted one, when the one command runs, then it fails and names it.
- `[REQ-SIM-007]` given the lab's own tests have run, when the run ends, then no connection the lab opened to a Simulated Cluster is still open.

**Edge cases:**

1. A Deployment declares more replicas than its Topology can hold. The unplaceable Pods are reported Pending, and a Scenario asserting every Pod Running fails.
2. The container runtime answers somewhere other than the conventional location. The run finds it.
3. A stale runtime address is present alongside a live runtime. The run reaches the live one.
4. A run names a Kubernetes version the lab cannot obtain. The run stops and names that version before any Scenario starts.
5. The container runtime cannot start one Scenario's Simulated Cluster while others start. That Scenario ends at its first Step and names why, and the others run on.

**ADR:** [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Deployment's Pods

<a id="req-sim-009"></a>
<a id="req-sim-025"></a>
A run names each Deployment by its Kubernetes kind `[REQ-SIM-025]`. A Scenario's Deployment declares one of the predefined Pod specifications the Scenarios define, one per intent `[REQ-SIM-009]`. Customizations change a copy before deployment, through the lab's vocabulary or full access to any other Pod setting, which no Scenario uses yet.

**Done when:**

- `[REQ-SIM-025]` given a Deployment, when its Given line is read, then it names the kind and replicas: 'a Deployment checkout of 9 replicas'.
- `[REQ-SIM-009]` given any Scenario, when its Deployment line is read, then it declares one predefined Pod specification, at most one Request and zero or more Customizations, nothing else.
- `[REQ-SIM-009]` given a predefined specification, when its definition is read, then every setting it holds stands there, under its Kubernetes name or through a Named setting.
- `[REQ-SIM-009]` given a Deployment declaring a specification, customized or not, when its Pods' settings are read back, then they are the definition's, with each Customization's settings standing over them.
- `[REQ-SIM-009]` given the Topology of the Scenario a-deployment-placed-for-resilience and nine Pods declaring Default placement, when the Placement settles, then every Pod is Running and no count per Zone is promised.
- `[REQ-SIM-009]` given two Deployments of one Scenario, when one customizes its specification or both declare different ones, then each is placed by its own, the other's Customizations never reaching it.
- `[REQ-SIM-009]` given two Customizations naming one setting, through the lab's vocabulary or full access, when both apply, then the later one stands.
- `[REQ-SIM-009]` given a Deployment customized with full access, when its Pods are read, then all it named stands except the waiting time before Pods leave an unresponsive Node. Its Pods count as that Deployment's.

**Edge cases:**

1. A Deployment declaring Resilient placement on a Topology of one Zone. Its Pods still land one per Node, all in that Zone.
2. Two Deployments of one Scenario declare the same specification. Each is spread on its own, the other's Pods not counting towards its Skew.
3. A Customization names a setting its specification does not hold. The specification gains it rather than refusing it.
4. Full access leaves out a setting the lab depends on. That setting holds and the Deployment is deployed.
5. Full access names a setting the control plane refuses. The Scenario ends at the Step deploying its Deployment, as for any refused Customization.

**ADR:** [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md)  ·  **Design:** [system-design.md](system-design.md)

### Stating what decides a Placement

<a id="req-sim-015"></a>
Every value deciding a Placement stands on a Scenario's own lines, stated or named there, and the lab gives none `[REQ-SIM-015]`. A name stands for values the Scenarios define once beside them, and each further Named Node capacity arrives with its first Scenario.

**Done when:**

- `[REQ-SIM-015]` given a NodePool stating no Node capacity, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-015]` given a NodePool naming a Named Node capacity, when provisioned, then its Nodes get that cpu, memory and Pod count, which the Given line reports by name.
- `[REQ-SIM-015]` given every predefined Pod specification, when its definition is read, then it names its own Startup time.
- `[REQ-SIM-015]` given the Named Node capacities and predefined Pod specifications the lab's Scenarios use, when their sources are read, then each is defined once beside them and the lab defines none.
- `[REQ-SIM-015]` given every Scenario of the lab, when it runs, then each NodePool states or names its Node capacity, and the outcome stays as asserted before.
- `[REQ-SIM-015]` given any run, when it starts, then it reports the lab's two waiting times, and no Node capacity or Startup time of its own.

**Edge cases:**

1. A predefined Pod specification defined with no Startup time. It is refused where it is defined.
2. A predefined Pod specification defined with no Request. Its Pods ask for no cpu and no memory.
3. Two definitions under one name, of a Named Node capacity or a Pod specification. They are reported where they are defined, so each name stands for one set of values.

**Design:** [system-design.md](system-design.md)

### Fitting a Deployment's Pods onto the Nodes

<a id="req-sim-011"></a>
A Node has room for only so much, and a Pod may Request a share of it `[REQ-SIM-011]`. A Pod no Node has room for stays Pending; after a Failure, only responding Nodes offer room. The Scenario no-room-after-a-zone-stops-responding shows a lost Zone costing the spare Pods, within the Failure tolerance.

<a id="req-sim-024"></a>
A Deployment line names Standard Pod, a quarter of the Standard capacity, or GPU Pod beside its replicas and the specification `[REQ-SIM-024]`.

**Done when:**

- `[REQ-SIM-011]` given a NodePool stating its Nodes' cpu, memory and Pod count, when it is provisioned, then every Node has that capacity and the run reports it in that order.
- `[REQ-SIM-011]` given a NodePool stating the cpu and memory alone, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-011]` given a specification stating a Request, or none, when its Pods are placed, then a stated Request stands under its Kubernetes name. Without one they ask for no cpu and no memory, and the spread alone decides.
- `[REQ-SIM-011]` given a Deployment requesting more cpu than its Topology has room for, when the Placement settles, then the Pods that fit are Running and the rest stay Pending.
- `[REQ-SIM-011]` given Nodes with room for one Pod each and a nine-Pod Deployment, when the Placement settles, then nine Pods are Running, one on every Node.
- `[REQ-SIM-011]` given that Placement, when one Zone stops responding, then it settles at six Pods Running and three Pending, and the Deployment never falls below its Floor of six.
- `[REQ-SIM-024]` given every Scenario's Deployment line, when read, then it names Standard Pod or GPU Pod or states its own Request, only one.
- `[REQ-SIM-024]` given every predefined Pod specification, when read, then it holds no Request, so a Deployment naming Tolerating GPU Nodes alone asks for nothing.
- `[REQ-SIM-024]` given a Deployment naming Standard Pod or GPU Pod, when it runs, then the lines beneath its Given line report that Request.

**Edge cases:**

1. A NodePool states a Node capacity of no Pod, no cpu, or no memory. It is refused where it is stated.
2. A Deployment's Request exceeds the room of every Node of its Topology. Every Pod of it stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter bound decides.

**Design:** [system-design.md](system-design.md)

### Holding Nodes of different kinds in a Zone

<a id="req-sim-013"></a>
A Zone holds NodePools of identical Nodes, the same in every Zone `[REQ-SIM-013]`.

**Done when:**

- `[REQ-SIM-013]` given three Zones of two general Nodes and one larger gpu Node each, when provisioned, then every Zone holds three Ready Nodes of their NodePool's capacity. Each Node carries a label naming its NodePool.
- `[REQ-SIM-013]` given that Topology, when a run reports its Placement, then each Zone's Nodes are numbered one to three, general's two before the gpu Node.
- `[REQ-SIM-013]` given that Topology, when the run starts, then it reports each NodePool's name, Node count per Zone, and cpu, memory and Pod count.
- `[REQ-SIM-013]` given that Topology and a Request only gpu Nodes have room for, when the Placement settles, then its Running Pods sit on gpu Nodes alone.
- `[REQ-SIM-013]` given every Scenario of the lab declaring one NodePool, when it runs, then that NodePool is named general and the Scenario reaches the outcome it asserted before.

**Edge cases:** each is refused where it is stated.

1. A NodePool of no Node.
2. Two NodePools of one Topology under one name, their Nodes being indistinguishable.
3. A NodePool name Kubernetes cannot hold as a label value.
4. A NodePool stating no Node capacity.
5. A NodePool stating no name, or a Topology stating no NodePool.

**Design:** [system-design.md](system-design.md)

### Marking a NodePool's Nodes with labels

<a id="req-sim-017"></a>
A NodePool may state Kubernetes labels for its Nodes, beside the lab's own Zone, hostname and NodePool labels `[REQ-SIM-017]`. A Then counts a Deployment's Pods per value of a stated label, and the Even spread spreads over one. The Scenario a-deployment-spread-over-on-demand-and-spot demonstrates it.

**Done when:**

- `[REQ-SIM-017]` given a NodePool stating labels, when provisioned, then each of its Nodes carries them beside the lab's own.
- `[REQ-SIM-017]` given that Topology, when the run starts, then its Given line reports each NodePool's stated labels after its capacity, under their Kubernetes keys.
- `[REQ-SIM-017]` given a NodePool stating a label under a key the lab sets, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-017]` given every earlier Scenario, whose NodePools state no label, when it runs, then it reaches the outcome it asserted before.
- `[REQ-SIM-017]` given a Then stating a label and a Pod count, when it judges the Placement, then it records a Finding unless every value the NodePools state holds that count.
- `[REQ-SIM-017]` given three Zones, each holding one-Node NodePools labelled on-demand and spot, when the Scenario a-deployment-spread-over-on-demand-and-spot places six Pods, then all six are Running. It asserts two per Zone and three per value of that label.

**Edge cases:**

1. A key or value Kubernetes cannot hold as a label. It is refused where it is stated.
2. One NodePool stating one key twice. It is refused where it is stated.
3. A NodePool stating no label. Its Nodes carry the lab's labels alone.
4. A Then stating a label no NodePool of the Topology states. The Scenario ends at that Then, naming the label.
5. A value of the label whose Nodes hold no Pod of the Deployment. It counts as none, so a Then asserting more records a Finding.
6. Pods on Nodes whose NodePool states no value of that label. They count towards no value.

**Design:** [system-design.md](system-design.md)

### Starting a Deployment's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready, so a Deployment replacing Pods after a Failure runs short meanwhile `[REQ-SIM-012]`.

**Done when:**

- `[REQ-SIM-012]` given a Deployment with a shortest and a longest Startup time, its specification's or customized, when its Pods are placed, then each is Ready between those bounds after its placement. Both bounds are included.
- `[REQ-SIM-012]` given a Deployment customized with one Startup time, in whole seconds or milliseconds, when its Pods are placed, then none is Ready sooner than that after its placement. Of two such Customizations, the later stands.
- `[REQ-SIM-012]` given a placed Pod inside its Startup time, when the Placement is reported, then that Pod counts Pending on its Node, and the Placement is not yet Settled.
- `[REQ-SIM-012]` given a Deployment declaring a predefined specification, when the run reports it, then it names that specification's shortest and longest Startup time.
- `[REQ-SIM-012]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before Pods took a Startup time.

**Edge cases:**

1. A Startup time of zero. The Pod is Ready as soon as it is placed.
2. A shortest Startup time longer than its longest, or a negative time. It is refused where it is stated, ending the Scenario at the Step declaring that Deployment.
3. A shortest and a longest Startup time that are equal. Every Pod takes that time.

**Design:** [system-design.md](system-design.md)

### Surviving a Node or Zone Failure

<a id="req-sim-008"></a>
A Scenario injects a Failure and judges the Placement on both sides of it `[REQ-SIM-008]`. A stopped Node stays in the cluster, as a hung machine does, until the control plane moves its Pods away after a waiting time the lab shortens. A Failure costs the spread, accepted while the Pod count holds, so neither Failure Scenario asserts a count per Node after it, nor a-zone-that-stops-responding one per Zone.

**Done when:**

- `[REQ-SIM-008]` given a Deployment declaring Resilient placement Settled at nine Pods, three per Zone, when one Node or Zone stops responding, then its Pods move away within a few seconds.
- `[REQ-SIM-008]` given one Node or one Zone stopped responding, when the Placement settles again, then nine Pods are Running, none on the stopped Nodes. After a Node Failure, every Zone still holds three.
- `[REQ-SIM-008]` given a-node-that-stops-responding or a-zone-that-stops-responding says one fails, when it runs twice, then the same Node or Zone fails in both runs and the run reports which.
- `[REQ-SIM-008]` given either Failure Scenario, when read, then the When injecting the Failure names no Node or Zone, and that of a-node-that-stops-responding states no waiting time.

**Edge cases:**

1. A Pod of a Node that stopped responding cannot be placed elsewhere. It stays Pending with the control plane's reason.
2. A When asks for more failing Nodes or Zones than the Topology holds. The Scenario ends at that Step, naming it.

**ADR:** [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

### Failing every Node of one label value

<a id="req-sim-018"></a>
A Failure may stop, in one Step, every Node carrying one value of a label a NodePool states `[REQ-SIM-018]`. The Scenario spot-nodes-that-stop-responding demonstrates it.

**Done when:**

- `[REQ-SIM-018]` given NodePools stating a label, when a When names that label and one of its values, then every Node carrying it stops responding in one Step. The run reports which, and their Pods are moved away within a few seconds.
- `[REQ-SIM-018]` given the Scenario spot-nodes-that-stop-responding, when read, then its When names the label and value alone, no Node, Zone, count or waiting time.
- `[REQ-SIM-018]` given three Zones, each holding two Nodes labelled on-demand and one labelled spot, when a Deployment declaring Resilient placement Settles at nine Pods, then every Node holds one.
- `[REQ-SIM-018]` given that Placement, when every spot Node stops responding, then the Deployment surviving the loss of three Nodes never falls below its Floor of six Ready Pods.
- `[REQ-SIM-018]` given that Failure, when the Placement settles again, then no Pod is on a stopped Node and every Zone still holds three.

**Edge cases:** each ends the Scenario at that When, naming what it named.

1. A When naming a label no NodePool of the Topology states, the lab's own labels included.
2. A When naming a value no NodePool states for that label.
3. A When naming a value whose every Node has already stopped responding.
4. A When naming no label, or more than one.

**Design:** [system-design.md](system-design.md)

### Keeping Pods off a dedicated NodePool

<a id="req-sim-019"></a>
A NodePool may state NoSchedule taints, keeping off its Nodes every Pod not tolerating them `[REQ-SIM-019]`. A toleration only permits a Pod onto a tainted Node, so a dedicated NodePool's Pods reach it by a Request only its Nodes have room for. A Scenario may declare a later Deployment, which the next Wait deploys beside the earlier; every Then counting Pods after it judges that Deployment alone. The Scenario general-pods-beside-tainted-gpu-nodes demonstrates it.

**Done when:**

- `[REQ-SIM-019]` given a NodePool stating a NoSchedule taint, when provisioned, then each of its Nodes carries it. The Given line reports it after the NodePool's labels, under its Kubernetes names.
- `[REQ-SIM-019]` given a NodePool stating a taint of another effect, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-019]` given a Pod specification tolerating a taint, when its Deployment's Given line is read, then the lines beneath report the toleration's key, operator, value and effect, and no waiting time.
- `[REQ-SIM-019]` given a Then stating a NodePool and a Pod count, when it judges the Placement, then it records a Finding unless that NodePool's Nodes hold that count.
- `[REQ-SIM-019]` given three Zones, each holding two general Nodes and one larger gpu Node tainted NoSchedule, when six Pods declaring Resilient placement are placed, then all six are Running on general Nodes.
- `[REQ-SIM-019]` given that Placement, when three Pods tolerating the taint, whose Request only gpu Nodes have room for, are placed, then all three are Running on gpu Nodes.
- `[REQ-SIM-019]` given every earlier Scenario, whose NodePools state no taint, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. A taint leaving out its key, value or effect, or stating a key or value Kubernetes cannot hold. It is refused where it is stated.
2. One NodePool stating one taint key twice. It is refused where it is stated.
3. A taint under a key the control plane sets on an unresponsive Node. It is refused where it is stated.
4. A toleration of a NoSchedule taint stating a waiting time. It is refused, ending the Scenario at the Step declaring that Deployment.
5. A Deployment not tolerating a taint finds no room left on the untainted Nodes. Its extra Pods stay Pending, none placed on a tainted Node.
6. A Then naming a NodePool the Topology lacks. The Scenario ends at that Then, naming it.
7. A Scenario declares a second Deployment under a name it already declared. It is refused, ending the Scenario at that Deployment's Given.

**Design:** [system-design.md](system-design.md)

### Judging a Deployment's Failure tolerance

<a id="req-sim-027"></a>
A Deployment declares the Zones or Nodes it survives losing `[REQ-SIM-027]`. Its Floor is its replicas less the most Pods those hold evenly spread. One Then judges that every declaring Deployment kept its Floor of Ready Pods since the first Failure.

**Done when:**

- `[REQ-SIM-027]` given any Deployment line, when read, then it declares Zones or Nodes it survives losing, never Pods.
- `[REQ-SIM-027]` given nine replicas over three Zones of three Nodes, when surviving one Zone or one Node, then the Floor is six or eight. Ten replicas surviving one Zone keep six.
- `[REQ-SIM-027]` given a Pod on a stopped Node or Pending, when judged, then it is lost, though the control plane may still report it Ready.
- `[REQ-SIM-027]` given any declaring Deployment below its Floor at any moment from the first Failure to the Then, when judged, then a Finding names it.
- `[REQ-SIM-027]` given a judgement, when reported, then each Deployment's line states kept or fell, its Floor, declared loss, and lowest Ready count with when and how long.
- `[REQ-SIM-027]` given a fall, when its Finding is read, then it states how low, when, how long, the Pods lost against the most declared, and their Nodes. An unended fall lasts 'for the rest of the Scenario'.
- `[REQ-SIM-027]` given every Failure Scenario, when run, then it declares what its When fails, every Floor holds, and its Placement outcome stands.

**Not yet in force:**

- `[REQ-SIM-027]` given a Then naming one Deployment, when it judges, then that Floor alone decides it.
- `[REQ-SIM-027]` given a Then asserting a Floor fell, when none fell, then it records a Finding. A Scenario pinning a Deployment to one lost Zone proves it.
- `[REQ-SIM-027]` given a Floor crossed, when the next Wait narrative is read, then it names each fall and return.

**Edge cases:**

1. A count below zero, or reaching the Topology's own. The Scenario ends at that Given, naming it.
2. A count of none. The Floor is every replica.
3. A Then with no Failure before it, or no declaring Deployment. The Scenario ends at that Then, naming it.

**Design:** [system-design.md](system-design.md)

### Explaining a Placement after the run

<a id="req-sim-005"></a>
A reported Placement says where Pods landed, and a Timeline says how they got there `[REQ-SIM-005]`. Every Scenario, failed or not, files its Timeline in one place under the working directory, and the lab removes none.

**Done when:**

- `[REQ-SIM-005]` given a Scenario that has run, when it ends, then one line names the Timeline's entry count and file, while the file keeps every Entry.
- `[REQ-SIM-005]` given an engineer asks for the whole Timeline, when a Scenario ends, then every entry is printed in the order it happened.
- `[REQ-SIM-005]` given a Scenario's Topology and Deployment in place, when its Timeline is read, then it holds every change to the Nodes, Pods and Deployment's own objects. It carries the control plane's reason for each Pod placed or left Pending.
- `[REQ-SIM-005]` given a run of two Scenarios, or two runs at once on one machine, when they end, then each Scenario left its own Timeline file, none replacing another.
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

- `[REQ-SIM-021]` given any Wait of a Scenario, reached or not, when its line is printed, then a separator line and a Placement board follow it. The separator is at least as long as the board's widest line.
- `[REQ-SIM-021]` given any Wait of a Scenario, when its board is printed, then a second separator line closes it before any Then or the next Step.
- `[REQ-SIM-021]` given a Then followed by a When, when the report is read, then no separator line stands between them.
- `[REQ-SIM-021]` given a Scenario of two Deployments, when a board is read, then every Node of the Topology has one line under its Zone, naming its NodePool. Each line lists every Pod of either Deployment on that Node by Deployment name and own suffix.
- `[REQ-SIM-021]` given a board after an earlier one, when read, then exactly the Pods new on each Node since the earlier board are marked. Pods left Pending are listed beneath the Nodes.
- `[REQ-SIM-021]` given Pods placed since the previous Wait, when that Wait's narrative is read, then it names each Pod's Node and when it reached Running, in the order these happened.
- `[REQ-SIM-021]` given a Failure before a Wait, when that Wait's narrative is read, then it names each Node turning NotReady, each Pod evicted and each replacement scheduled, in order. It claims no link between an eviction and a replacement.
- `[REQ-SIM-021]` given Pods left Pending when a Wait ends, when its narrative is read, then it names each with the scheduler's reason.
- `[REQ-SIM-021]` given a Wait after which nothing changed the Placement, when printed, then its line is followed directly by the separator and the board, with no narrative.
- `[REQ-SIM-021]` given a Scenario's Timeline, when its file or the whole Timeline is read, then each Step stands at its moment among the Entries.
- `[REQ-SIM-021]` given a Scenario that has run, when its report ends, then one Skew line for the judged Deployment follows the last board, then a separator line and the Timeline's line. No table of Pods per Zone or per Node and no summary of each Deployment's Pods appear.
- `[REQ-SIM-021]` given a Scenario whose Timeline was filed, when its Timeline's line is read, then the line ends at the file's name with no colon. The separator above it is at least as long as that line.
- `[REQ-SIM-021]` given a run of two Scenarios, when its report is read, then a heavier separator line stands between the run's opening lines and the first Scenario, and between the two Scenarios. It uses a character the separator line does not, and is at least as long as the widest line of the parts it divides.
- `[REQ-SIM-021]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. The board under the Wait for every Node Ready holds no Pod. Every Node still has its line, empty.
2. The first board holding a Deployment's Pods marks every one of them new.
3. A Pod is evicted by a Failure and a replacement is placed. The narrative names both in order, and the board shows the replacement alone, marked new.
4. A Scenario ends at a Wait it does not reach. That Wait still prints its narrative, both separators and its board, up to the moment the Scenario ended.
5. A Scenario ends before any Deployment is deployed. Its boards list the Nodes with no Pod, and no Skew line is given.
6. The Pods the lab places on its own while provisioning the Topology appear in neither a narrative nor a board.
7. A Wait has no Then judging its Placement, such as one before a When. Its board still closes with a second separator, and the next Step follows it.
8. A run holds one Scenario. One heavier separator line stands between the run's opening lines and that Scenario, and none follows it.

**Design:** [system-design.md](system-design.md)

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as Steps, one per line: Given declares, When changes the cluster, Wait waits for a cluster state, Then asserts the Placement `[REQ-SIM-006]`.

<a id="req-sim-020"></a>
An engineer reads every Scenario of the lab in one source, each run and reported under its Scenario name, the condition its function names `[REQ-SIM-020]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario of the lab, when read, then each line is one Given, When, Wait or Then Step, and none handles an error. Each name is one the lab offers, apart from those full access names.
- `[REQ-SIM-006]` given any Scenario of the lab, when it runs, then every Wait is a Step of its own reporting its own Wait line. One stands before its first Then and one after every Failure.
- `[REQ-SIM-006]` given a declared Topology or Deployment, when its Wait runs, then every declared Node or Pod is Ready and the Wait line names the time taken. Written with its Given or on its own, a Wait reports the same line and Placement.
- `[REQ-SIM-006]` given a Then with no Wait since the Given or Failure before it, when the Scenario runs, then it ends at that Then. A Failure before the first Wait ends it at that When, naming it.
- `[REQ-SIM-006]` given a Wait stating no time, when Pods are not all Ready within the lab's own waiting time, then the Scenario ends there, naming them.
- `[REQ-SIM-006]` given a Wait stating a time, when the cluster reaches its state inside it, then its line names that time and the time taken. When that time passes first, the Scenario ends at that Wait, whose line names that time alone.
- `[REQ-SIM-006]` given a Wait stating a time above the lab's own waiting time, when the Scenario runs, then the time is refused, naming that waiting time.
- `[REQ-SIM-006]` given a Scenario that has Waited, when the Thens after it judge the Placement, then that Placement has stopped changing, even with Pods still Pending.
- `[REQ-SIM-006]` given every Failure Scenario, when read, then it states a time on the Wait after its Failure and none on the Waits after its Givens.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when the Scenario runs, then it ends there, naming that Step. A Then the Placement does not meet only reports a Finding.
- `[REQ-SIM-006]` given the repository, when an engineer looks for the Scenarios, then one place holds them and no machinery, beside a README naming each and its asserted outcome.
- `[REQ-SIM-020]` given the repository, when an engineer looks for the Scenarios, then one source holds every Scenario of the lab and no other source holds one.
- `[REQ-SIM-020]` given any Scenario of the lab, when a run reports it, then it is named by the condition its function names, in lowercase words joined by hyphens, such as a-node-that-stops-responding.
- `[REQ-SIM-020]` given the README beside the Scenarios, when read, then it names each Scenario by its Scenario name.
- `[REQ-SIM-020]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. A Scenario writes two Waits in a row. The second reports a wait of almost none, rather than being refused.
2. A Wait for every Pod stands before any Deployment is declared. It waits for no Pod and reports that nothing was waited for.
3. A Wait states a time of no seconds. It is refused where it is stated.
4. A Wait for a Settled Placement stands before any Deployment is declared. It is reached at once.
5. Two Scenarios whose functions name the same condition. The lab's rules refuse them before any Simulated Cluster is provisioned, naming both `[REQ-SIM-020]`.
6. A Scenario whose function names no condition. The lab's rules refuse it before any Simulated Cluster is provisioned, naming the function `[REQ-SIM-020]`.

**ADR:** [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

### Reading a Scenario by subject

<a id="req-sim-016"></a>
<a id="req-sim-026"></a>
Each name a Scenario writes shows its subject, a Pod, a Node, or the Scenario with its Steps and Topology, unless the Scenarios define it `[REQ-SIM-016]`. Scenario code stays idiomatic Go, naming each value of a setting under its Kubernetes name, and each name the lab offers shows where it is defined `[REQ-SIM-026]`. The Scenarios define Named settings, and a Scenario states any spread but the Even spread itself.

**Done when:**

- `[REQ-SIM-016]` given any Scenario source of the lab, when read, then each name it writes shows its subject or is one the Scenarios define. A setting of several values names each value under its Kubernetes name, none only marking a position.
- `[REQ-SIM-026]` given any Scenario source of the lab, when read, then it is idiomatic Go and passes the lab's format and lint checks. Each name the lab offers shows where it is defined, inline Customizations included, and the Scenarios rename none.
- `[REQ-SIM-016]` given the Named setting for leaving an unresponsive Node, when its definition is read, then it holds the not-ready and unreachable tolerations at the lab's waiting time and nothing else. Every specification names it rather than writing either toleration.
- `[REQ-SIM-016]` given the Even spread, when its definition is read, then it holds one topology spread over the subject given and nothing else. Resilient placement names it over Zones and over Nodes rather than writing either spread.
- `[REQ-SIM-016]` given a Deployment declaring a predefined specification, customized or not, when its Given line is read, then it names the intent. The lines beneath report every setting its Pods carry, as customized, one per line under its Kubernetes name.
- `[REQ-SIM-016]` given a Deployment declaring Resilient placement, when the lines beneath its Given line are read, then they report both spreads in full.
- `[REQ-SIM-016]` given a setting of several values leaving out one of its values or stating it as zero, when it is stated, then it is refused where it is stated.
- `[REQ-SIM-016]` given any count a Scenario states, when read, then the Step naming it states its subject and the count reads bare, a Deployment's replicas included. A Node's room reads under the Node.
- `[REQ-SIM-016]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. A Deployment declares Default placement, which holds no spread. The lines beneath its Given line report the two tolerations alone.
2. A Customization writes another time for one of the two tolerations. The lab's waiting time holds, and the lines beneath the Given line report that time.
3. A setting of several values leaves out a value Kubernetes would default. It is refused all the same, since no value deciding a Placement falls to a default. A Scenario needing a zero sets it through full access.

**ADR:** [ADR: A Deployment declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) · [ADR: Scenario code stays idiomatic Go, read by subject](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)  ·  **Design:** [system-design.md](system-design.md)

### Watching a Scenario live

<a id="req-sim-022"></a>
An engineer starts an Interactive run naming one Scenario and opens its Live view in a browser `[REQ-SIM-022]`. Its Steps run up to its first When, reporting as in a run, and its Simulated Cluster is then held until the lab stops `[REQ-SIM-022]`. The Live view shows that cluster alone and changes nothing in it `[REQ-SIM-022]`.

The Live view draws each Zone as a box of its Nodes and each Node as a box of its Pods, Pending Pods apart `[REQ-SIM-022]`. A Node shows its NodePool, Node labels, capacity, requests and what is left, and a Pod its Request `[REQ-SIM-022]`. A Node's or Pod's box gives its name and whether it is Ready on its first line, and the page header names k8s-resilience-lab and the Scenario `[REQ-SIM-022]`. Every open Live view follows each change as the lab observes it `[REQ-SIM-022]`.

**Done when:**

- `[REQ-SIM-022]` given an Interactive run naming a Scenario, when it starts, then that Scenario's Simulated Cluster is provisioned and its Steps up to its first When run. They report to the terminal as in a run, which also names the Live view's address.
- `[REQ-SIM-022]` given the Live view open while the Steps run, when the Placement changes, then the Live view shows it as the lab observes it, with no reload.
- `[REQ-SIM-022]` given a held Simulated Cluster, when the Live view is read, then each Zone's box holds one box per Node. Each Node's box holds its Pods by Deployment name and own suffix, and the Pending Pods stand apart.
- `[REQ-SIM-022]` given a Node, when its box is read, then it names its NodePool and the Node labels that NodePool states. It gives its Pods, cpu and memory as capacity, requests and left: its Node capacity, what its Pods take by count and Request, and the difference.
- `[REQ-SIM-022]` given a placed or Pending Pod, when its box is read, then it gives its Request as cpu and memory, under the word requests. A Pod declaring no Request shows cpu 0 and memory 0.
- `[REQ-SIM-022]` given an Interactive run naming a Scenario, when the page header is read, then it names k8s-resilience-lab, a separator such as a hyphen, then the Scenario name.
- `[REQ-SIM-022]` given a Node or a placed Pod, when its box is read, then its first line gives its name and, at the top right, whether it is Ready by colour and word. A Pod's requests stand beneath, its box as wide as its Node's. A Pod new on its Node is marked as a Placement board marks it, and the mark fades.
- `[REQ-SIM-022]` given a narrow window, or a system set to light or dark, when the Live view is read, then it fits the window and follows that setting, loading nothing from another host.
- `[REQ-SIM-022]` given those Steps have run, when the engineer watches, then the cluster stays as they left it until the engineer stops the lab, removing it.
- `[REQ-SIM-022]` given every Scenario of the lab, when a run carries it out, then it reaches the outcome it asserted before.

<a id="req-sim-023"></a>
**Not yet in force.** Later slices add the following, first the Timeline growing beneath the boxes `[REQ-SIM-023]`. Whether Zones stack in a column, leaving a pane to their right on a laptop, is decided with it `[REQ-SIM-023]`. Until the controls ship, an Interactive run naming no Scenario is refused, as edge case 1 states.

- `[REQ-SIM-023]` given the Live view open, when the Timeline gains an Entry, then it appears beneath the boxes in order with no reload. Every Entry appears, each Step at its moment, each showing at a glance what changed.
- `[REQ-SIM-022]` given an Interactive run naming no Scenario, when the Live view opens, then it offers every Scenario by its Scenario name and provisions nothing until the engineer picks one.
- `[REQ-SIM-022]` given a held cluster, when the engineer picks another Scenario or resets, then it is removed before a fresh one is provisioned. The terminal reports each provisioning.
- `[REQ-SIM-022]` given the engineer chooses every Step, when a Scenario is picked or reset, then every Step runs and reports to the terminal, and the cluster is then held.
- `[REQ-SIM-022]` given Steps still running, when the engineer resets or picks another Scenario, then the Steps stop and the cluster is removed.

**Edge cases:**

1. An Interactive run names a Scenario matching none, or names no Scenario or two. It stops before provisioning and names the Scenarios that exist, as a run does.
2. A Scenario with no When. All its Steps run.
3. A Step that cannot be carried out. The terminal names it as a run does, and the cluster is held as that Step left it.
4. A Then records a Finding. The terminal reports it as a run does, and the later Steps still run.
5. The Live view opens only after the Steps ran, or in two browsers at once. Each shows the Placement as it stands.
6. Pods the lab places itself while provisioning the Topology appear nowhere in the Live view.
7. The engineer stops the lab while Steps still run. The Steps stop and the Simulated Cluster is removed.
8. No container runtime is reachable. The Interactive run names why and stops before offering the Live view.

**Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- <a id="req-sim-010"></a>`REQ-SIM-010`, a count of Pods lost judged at two moments. Superseded by `REQ-SIM-027`.

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Whether the lab scripts a Failure over time, such as a Pod dying after a delay, at the cost of a second injection surface.
- What an engineer's action on a Node or Pod in the Live view means, and whether a Failure injected there joins the Timeline as a When does.
- Whether the Live view's Timeline sets a Wait narrative's moments apart from other Entries.
- Whether the Live view is reachable from another machine.
- How long a Timeline file is kept, and whether the lab ever removes one.
- How an engineer reaches a recorded Timeline beyond its file, and whether a Scenario may assert on it.
- Whether the lab promises anything about a team's production manifests, which needs Topology and admission behavior mirroring a named environment.
- Whether a Deployment declares Failures it survives in sequence, and whether a run names those the Scenario injected.
- How another kind, such as a DaemonSet, declares its Failure tolerance.
- Which Nodes count towards the Floor of a Deployment confined to one NodePool.
- Whether the lab judges the Placement after a Failure against the best achievable spread, a Finding the cluster cannot avoid since spread rules apply only when placing a Pod.
- Whether Nodes also sit on racks, a failure domain between Zone and Node.
- Which Scenario first fails a Node by position, what a missing position yields, and whether a Failure says if its Nodes share a Zone.
- What a Scenario asserts when every Zone, or every Node of the value a Failure names, fails.
- Whether a Failure by label names several values, or stops one NodePool by name.
- Whether taints take PreferNoSchedule or NoExecute, the latter meeting the Failures' tolerations.
- Whether a Deployment selects a NodePool, beside a Request only its Nodes have room for.
- What limits a Topology's Zone count beyond the lab's own refusal.
- Which Failure follows a-zone-that-stops-responding: a Pod, or a Node removed rather than unresponsive.
- Whether a Scenario demonstrates a Customization or full access, and which intent.
- Whether a Scenario rolls out a change to a deployed Deployment's specification.
- Whether the lab says anything when full access leaves out a setting it depends on.
- Whether a Then asserts which Pods stay Pending, or their count alone.
- Whether the lab's vocabulary names a Pod's limits, which decide no Placement, and what a Scenario would assert with them.
- Whether a run names one Startup time for every Pod, beside each Deployment's own.
- Whether a run can carry out its Scenarios one at a time, for a machine without room for all at once.
- Whether the lab gives a removed Pod a time to stop, as it gives one to start.
- Which further specifications and settings the Scenarios define, and which a Customization may name. Zone spread only, Soft spread and affinity between a Deployment's Pods each arrive with their first Scenario. Also open: what a Scenario asserts demonstrating a weak specification.
