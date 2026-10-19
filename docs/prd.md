<!-- harness: 2026-09-30 -->
# Product Requirements Document: k8s-resilience-lab

<!-- AGENT: This PRD is narrative prose. State WHAT the system does and WHY it matters. Never HOW — mechanism (flags, exit codes, file layouts, algorithms) lives in system-design.md. Never the decision trail — alternatives and trade-offs live in adr/. -->
<!-- AGENT: Annotate each requirement inline with its [REQ-XX-NNN] tag where the prose expresses it, and give it one "Done when" acceptance bullet carrying the same tag. The prose is the intent; the tagged bullet is the bounded, testable contract. Drop an <a id="req-xx-nnn"></a> anchor at first mention so other docs deep-link to it. -->
<!-- AGENT: A requirement is active by being in the narrative — there is no per-requirement Status field. Retire one by moving it to the Superseded list; never renumber an ID. -->

## Context

A team writes Kubernetes manifests that declare where their Pods may land: spread rules across Zones, anti-affinity between Nodes, disruption budgets. Whether those declarations produce the intended Placement, and whether the Placement recovers after a Zone or Node is lost, is normally discovered during a real outage. Proving it earlier needs a cluster, and a cluster per developer and per build is expensive.

k8s-resilience-lab is a learning and verification lab for that question. It starts a Simulated Cluster — a genuine Kubernetes control plane with fake Nodes and fake Pods — provisions a Topology, deploys a Workload, and reports the resulting Placement. Scheduling and controller behavior are the real ones, because the control plane is real. Nothing runs inside a Pod: no Workload image is ever pulled.

The lab serves two uses from one body of behavior. Unattended, it runs each Scenario and asserts the Placement outcome, on a developer machine or in a build. Interactively, it shows the Simulated Cluster live and takes a Failure injected by hand, to demonstrate what the assertion claims.

## Goals

| ID | Goal | Success Metric |
|----|------|----------------|
| G-1 | Placement under a declared Topology is verified automatically, with no real cluster involved. | Every Scenario runs unattended on a developer machine and in a build, needing only a container runtime. |
| G-2 | The behavior each Scenario asserts is also demonstrable live. | An engineer starts the lab, watches the Placement of a running Workload, and injects a Failure without editing code. |
| G-3 | A Workload's capacity is judged through a Failure, not only once the cluster has recovered. | Each Failure Scenario declares the Required Capacity its Workload must keep serving and judges it at the moment of the Failure and again once the Placement is Settled. |
| G-4 | A Scenario is written and read in one vocabulary. | Every name in a Scenario is one the lab offers Scenarios, each line is a Step, and every Scenario sits in one place. |
| G-5 | What happened inside a Scenario's Simulated Cluster is explainable after the run. | Every Scenario leaves a Timeline naming each Node, Pod and Workload change and the control plane's reason for each Placement. |

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
Every Scenario runs in a Simulated Cluster of its own, provisioned with its Topology before any Workload and removed when the Scenario ends `[REQ-SIM-001]`. The lab reports the Placement a Scenario asserts, its Pods Running and Pending per Zone and per Node, each named by its declared position `[REQ-SIM-001]`. A run reports the Kubernetes version it mirrors and touches nothing in the container runtime but its own Simulated Cluster `[REQ-SIM-001]`.

<a id="req-sim-002"></a>
One invocation with no arguments checks the lab's own rules, then runs every Scenario needing a Simulated Cluster and reports each outcome afresh `[REQ-SIM-002]`.

<a id="req-sim-014"></a>
A run carries out its Scenarios at once, so it lasts about as long as its longest Scenario, yet reports them in declared order `[REQ-SIM-014]`. A failing Scenario ends alone, and the lab's own tests needing a Simulated Cluster also run at once `[REQ-SIM-014]`.

<a id="req-sim-003"></a>
An engineer names one Scenario to re-run it alone `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
One command decides whether the lab itself is sound: its rules, its tests, its build, and that no part can reach a cluster the lab did not provision `[REQ-SIM-004]`. The repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The same command holds the lab to its declared dependencies, free of known vulnerabilities, and every connection the lab opens to a Simulated Cluster closes when its work ends `[REQ-SIM-007]`.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when Resilient placement runs, then three Zones each hold the NodePool general of three Ready Nodes. Its Placement Settles at nine Running Pods, one on every Node.
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

1. A Workload declares more Pods than its Topology can hold. The unplaceable Pods are reported Pending, and a Scenario asserting every Pod Running fails.
2. The container runtime answers somewhere other than the conventional location. The run finds it.
3. A stale runtime address is present alongside a live runtime. The run reaches the live one.
4. A run names a Kubernetes version the lab cannot obtain. The run stops and names that version before any Scenario starts.
5. The container runtime cannot start one Scenario's Simulated Cluster while others start. That Scenario ends at its first Step and names why, and the others run on.

**ADR:** [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md) · [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Workload's Pods

<a id="req-sim-009"></a>
A Scenario states the intent behind its Workload, not the settings carrying it out `[REQ-SIM-009]`. Its Workload declares one of the predefined Pod specifications the Scenarios define, one per intent `[REQ-SIM-009]`. Its definition names every setting deciding where Pods land and when they leave a Node, each under its Kubernetes name or through a Named setting `[REQ-SIM-009]`. Resilient placement spreads Pods evenly over Zones and Nodes, and Default placement, its deliberately weak counterpart, leaves the scheduler to decide `[REQ-SIM-009]`. Customizations change a copy of the specification before deployment, in the order written, through the lab's own vocabulary or through full access to every other Kubernetes Pod setting `[REQ-SIM-009]`. Either way the Pods count as the Workload's, and the lab's waiting time before Pods leave an unresponsive Node holds `[REQ-SIM-009]`. No Scenario uses full access yet `[REQ-SIM-009]`.

**Done when:**

- `[REQ-SIM-009]` given any Scenario of the lab, when its Workload line is read, then it declares one predefined Pod specification, with zero or more Customizations and nothing else.
- `[REQ-SIM-009]` given a predefined specification, when its definition is read, then every setting it holds stands there, under its Kubernetes name or through a Named setting.
- `[REQ-SIM-009]` given a Workload declaring a specification, customized or not, when its Pods' settings are read back, then they are the definition's, with each Customization's settings standing over them.
- `[REQ-SIM-009]` given Resilient placement's Topology and nine Pods declaring Default placement, when the Placement settles, then every Pod is Running and no count per Zone is promised.
- `[REQ-SIM-009]` given two Workloads of one Scenario, when one customizes its specification or both declare different ones, then each is placed by its own, the other's Customizations never reaching it.
- `[REQ-SIM-009]` given two Customizations naming one setting, through the lab's vocabulary or full access, when both apply, then the later one stands.
- `[REQ-SIM-009]` given a Workload customized with full access, when its Pods are read, then all it named stands except the waiting time before Pods leave an unresponsive Node. Its Pods count as that Workload's.

**Edge cases:**

1. A Workload declaring Resilient placement on a Topology of one Zone. Its Pods still land one per Node, all in that Zone.
2. Two Workloads of one Scenario declare the same specification. Each is spread on its own, the other's Pods not counting towards its Skew.
3. A Customization names a setting its specification does not hold. The specification gains it rather than refusing it.
4. Full access leaves out a setting the lab depends on. That setting holds and the Workload is deployed.
5. Full access names a setting the control plane refuses. The Scenario ends at the Step deploying its Workload, as for any refused Customization.

**ADR:** [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) · [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md) · [ADR: The Scenarios name leaving an unresponsive Node as one setting](adr/2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md)  ·  **Design:** [system-design.md](system-design.md)

### Stating what decides a Placement

<a id="req-sim-015"></a>
Every value that decides a Placement stands on a Scenario's own lines, stated there or named there, and the lab gives none of its own `[REQ-SIM-015]`. A name stands for values the Scenarios define once, beside the Scenarios using it, and the lab only carries out the definition `[REQ-SIM-015]`. So a NodePool states or names its Node capacity, and a predefined Pod specification names its own Request, if any, and its own Startup time `[REQ-SIM-015]`. Standard is the one Named Node capacity so far, and each further name arrives with its first Scenario `[REQ-SIM-015]`. The lab keeps two waiting times of its own, reported when a run starts: a Wait's when it states none, and the one before Pods leave an unresponsive Node `[REQ-SIM-015]`.

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

### Fitting a Workload's Pods onto the Nodes

<a id="req-sim-011"></a>
A Node has room for only so much, and a Pod asks for a share of it `[REQ-SIM-011]`. A Pod specification may hold a Request, and one stating none, as in every lab Scenario, asks for nothing `[REQ-SIM-011]`. A Pod no Node has room for stays Pending, so a Failure leaves a Workload only the room on the Nodes still responding `[REQ-SIM-011]`. No room after a Zone stops responding shows a lost Zone costing the spare Pods, not the Required Capacity `[REQ-SIM-011]`.

**Done when:**

- `[REQ-SIM-011]` given a NodePool stating its Nodes' cpu, memory and Pod count, when it is provisioned, then every Node has that capacity and the run reports it in that order.
- `[REQ-SIM-011]` given a NodePool stating the cpu and memory alone, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-011]` given a specification stating a Request, or none, when its Pods are placed, then that Request stands under its Kubernetes name. Without one they ask for no cpu and no memory, and the spread alone decides.
- `[REQ-SIM-011]` given a Workload requesting more cpu than its Topology has room for, when the Placement settles, then the Pods that fit are Running and the rest stay Pending.
- `[REQ-SIM-011]` given Nodes with room for one Pod each and a nine-Pod Workload, when the Placement settles, then nine Pods are Running, one on every Node.
- `[REQ-SIM-011]` given that Placement, when one Zone stops responding, then it settles at six Pods Running and three Pending. The Required Capacity judgement counts six at the moment of the Failure and six once Settled.

**Edge cases:**

1. A NodePool states a Node capacity of no Pod, no cpu, or no memory. It is refused where it is stated.
2. A Workload's Request exceeds the room of every Node of its Topology. Every Pod of it stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter bound decides.

**Design:** [system-design.md](system-design.md)

### Holding Nodes of different kinds in a Zone

<a id="req-sim-013"></a>
A real cluster seldom runs one kind of machine, so a Zone holds NodePools of identical Nodes, the same NodePools in every Zone `[REQ-SIM-013]`. A Topology states its Zone count and its NodePools alone, and each NodePool its name, Node count and Node capacity `[REQ-SIM-013]`. Every Node belongs to one NodePool, carries its name as a label, and keeps its positional name `[REQ-SIM-013]`. NodePools of different capacity decide where Pods land `[REQ-SIM-013]`.

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
A NodePool may state labels its Nodes carry, each under its Kubernetes key and value, beside the labels the lab sets: a Node's Zone, hostname and NodePool name `[REQ-SIM-017]`. A NodePool stating no label stays valid, so every Scenario keeps its asserted outcome `[REQ-SIM-017]`. A Then counts a Workload's Pods per value of a stated label, as another counts them per Zone, and the Even spread spreads over such a label too `[REQ-SIM-017]`. The Scenario Spread over on-demand and spot demonstrates it `[REQ-SIM-017]`.

**Done when:**

- `[REQ-SIM-017]` given a NodePool stating labels, when provisioned, then each of its Nodes carries them beside the lab's own.
- `[REQ-SIM-017]` given that Topology, when the run starts, then its Given line reports each NodePool's stated labels after its capacity, under their Kubernetes keys.
- `[REQ-SIM-017]` given a NodePool stating a label under a key the lab sets, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-017]` given every earlier Scenario, whose NodePools state no label, when it runs, then it reaches the outcome it asserted before.
- `[REQ-SIM-017]` given a Then stating a label and a Pod count, when it judges the Placement, then it records a Finding unless every value the NodePools state holds that count.
- `[REQ-SIM-017]` given three Zones, each holding one-Node NodePools labelled on-demand and spot, when Spread over on-demand and spot places six Pods, then all six are Running. It asserts two per Zone and three per value of that label.

**Edge cases:**

1. A key or value Kubernetes cannot hold as a label. It is refused where it is stated.
2. One NodePool stating one key twice. It is refused where it is stated.
3. A NodePool stating no label. Its Nodes carry the lab's labels alone.
4. A Then stating a label no NodePool of the Topology states. The Scenario ends at that Then, naming the label.
5. A value of the label whose Nodes hold no Pod of the Workload. It counts as none, so a Then asserting more records a Finding.
6. Pods on Nodes whose NodePool states no value of that label. They count towards no value.

**Design:** [system-design.md](system-design.md)

### Starting a Workload's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready, so a Workload replacing Pods after a Failure stays short of them meanwhile `[REQ-SIM-012]`. Every placed Pod counts Pending on its Node for its Startup time, a replacement counting from its own placement `[REQ-SIM-012]`. Every predefined Pod specification names a shortest and a longest Startup time, both above zero, so each Pod takes its own and none is Ready at placement `[REQ-SIM-012]`. A Customization may state one time, or a shortest and a longest, in whole seconds or milliseconds `[REQ-SIM-012]`. It is the one Customization that is no Kubernetes setting `[REQ-SIM-012]`.

**Done when:**

- `[REQ-SIM-012]` given a Workload with a shortest and a longest Startup time, its specification's or customized, when its Pods are placed, then each is Ready between those bounds after its placement. Both bounds are included.
- `[REQ-SIM-012]` given a Workload customized with one Startup time, in whole seconds or milliseconds, when its Pods are placed, then none is Ready sooner than that after its placement. Of two such Customizations, the later stands.
- `[REQ-SIM-012]` given a placed Pod inside its Startup time, when the Placement is reported, then that Pod counts Pending on its Node, and the Placement is not yet Settled.
- `[REQ-SIM-012]` given a Workload declaring a predefined specification, when the run reports it, then it names that specification's shortest and longest Startup time.
- `[REQ-SIM-012]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before Pods took a Startup time.

**Edge cases:**

1. A Startup time of zero. The Pod is Ready as soon as it is placed.
2. A shortest Startup time longer than its longest, or a negative time. It is refused where it is stated, ending the Scenario at the Step declaring that Workload.
3. A shortest and a longest Startup time that are equal. Every Pod takes that time.

**Design:** [system-design.md](system-design.md)

### Surviving a Node or Zone Failure

<a id="req-sim-008"></a>
A Scenario injects a Failure and judges the Placement on both sides of it `[REQ-SIM-008]`. Node stops responding stops one Node of Resilient placement, as a hung or disconnected machine does, and Zone stops responding stops every Node of one Zone in one Step `[REQ-SIM-008]`. A stopped Node stays in the cluster, and the control plane moves its Pods away after a waiting time the lab shortens to a few seconds `[REQ-SIM-008]`. Every run reports that shortening `[REQ-SIM-008]`. A Scenario says how many Nodes or Zones fail, and the lab picks the same ones on every run `[REQ-SIM-008]`. A Failure costs the spread, which the lab accepts while the Pod count holds `[REQ-SIM-008]`. So neither Scenario asserts a count per Node after it, and Zone stops responding asserts none per Zone `[REQ-SIM-008]`.

**Done when:**

- `[REQ-SIM-008]` given Resilient placement Settled at nine Pods, three in every Zone, when one Node or Zone stops responding, then its Pods are moved away within a few seconds.
- `[REQ-SIM-008]` given one Node or one Zone stopped responding, when the Placement settles again, then nine Pods are Running, none on the stopped Nodes. After a Node Failure, every Zone still holds three.
- `[REQ-SIM-008]` given Node or Zone stops responding says one fails, when it runs twice, then the same Node or Zone fails in both runs and the run reports which.
- `[REQ-SIM-008]` given either Failure Scenario, when read, then the When injecting the Failure names no Node or Zone, and Node stops responding's states no waiting time.

**Edge cases:**

1. A Pod of a Node that stopped responding cannot be placed elsewhere. It stays Pending with the control plane's reason.
2. A When asks for more failing Nodes or Zones than the Topology holds. The Scenario ends at that Step, naming it.

**ADR:** [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

### Failing every Node of one label value

<a id="req-sim-018"></a>
A Failure may stop every Node carrying one value of a label a NodePool states `[REQ-SIM-018]`. Its When names that label and value, and those Nodes stop responding in one Step, as in a Node Failure `[REQ-SIM-018]`. The Scenario Spot Nodes stop responding demonstrates it and judges the Required Capacity at both moments `[REQ-SIM-018]`.

**Done when:**

- `[REQ-SIM-018]` given NodePools stating a label, when a When names that label and one of its values, then every Node carrying it stops responding in one Step. The run reports which, and their Pods are moved away within a few seconds.
- `[REQ-SIM-018]` given Spot Nodes stop responding, when read, then its When names the label and value alone, no Node, Zone, count or waiting time.
- `[REQ-SIM-018]` given three Zones, each holding two Nodes labelled on-demand and one labelled spot, when Resilient placement Settles at nine Pods, then every Node holds one.
- `[REQ-SIM-018]` given that Placement, when every spot Node stops responding, then the Required Capacity judgement counts six Pods at that moment and nine once Settled. Both meet the six declared.
- `[REQ-SIM-018]` given that Failure, when the Placement settles again, then no Pod is on a stopped Node and every Zone still holds three.

**Edge cases:** each ends the Scenario at that When, naming what it named.

1. A When naming a label no NodePool of the Topology states, the lab's own labels included.
2. A When naming a value no NodePool states for that label.
3. A When naming a value whose every Node has already stopped responding.
4. A When naming no label, or more than one.

**Design:** [system-design.md](system-design.md)

### Keeping Pods off a dedicated NodePool

<a id="req-sim-019"></a>
A NodePool may state taints its Nodes carry, each under its Kubernetes key, value and effect, apart from its labels `[REQ-SIM-019]`. The one effect so far, NoSchedule, keeps every Pod not tolerating the taint off those Nodes `[REQ-SIM-019]`. A Pod specification may tolerate a taint, naming its key, the Equal operator, its value and effect, and no waiting time `[REQ-SIM-019]`. A toleration permits a Pod onto a tainted Node without drawing it there, so a dedicated NodePool's Pods reach it by a Request only its Nodes have room for `[REQ-SIM-019]`. A Then counts the Pods on one NodePool's Nodes, as another counts them per Zone `[REQ-SIM-019]`. A Scenario may declare a later Workload, which the next Wait deploys beside the earlier, and every Then after it judges that Workload alone `[REQ-SIM-019]`. A NodePool stating no taint stays valid, so every Scenario keeps its asserted outcome `[REQ-SIM-019]`. The Scenario General Pods stay off gpu Nodes demonstrates it `[REQ-SIM-019]`.

**Done when:**

- `[REQ-SIM-019]` given a NodePool stating a NoSchedule taint, when provisioned, then each of its Nodes carries it. The Given line reports it after the NodePool's labels, under its Kubernetes names.
- `[REQ-SIM-019]` given a NodePool stating a taint of another effect, when it is declared, then it is refused where it is stated.
- `[REQ-SIM-019]` given a Pod specification tolerating a taint, when its Workload's Given line is read, then the lines beneath report the toleration's key, operator, value and effect, and no waiting time.
- `[REQ-SIM-019]` given a Then stating a NodePool and a Pod count, when it judges the Placement, then it records a Finding unless that NodePool's Nodes hold that count.
- `[REQ-SIM-019]` given three Zones, each holding two general Nodes and one larger gpu Node tainted NoSchedule, when six Pods declaring Resilient placement are placed, then all six are Running on general Nodes, and none is on a gpu Node.
- `[REQ-SIM-019]` given that Placement, when three Pods tolerating the taint, whose Request only gpu Nodes have room for, are placed, then all three are Running on gpu Nodes, and none of them is on a general Node.
- `[REQ-SIM-019]` given every earlier Scenario, whose NodePools state no taint, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. A taint leaving out its key, value or effect, or stating a key or value Kubernetes cannot hold. It is refused where it is stated.
2. One NodePool stating one taint key twice. It is refused where it is stated.
3. A taint under a key the control plane sets on an unresponsive Node. It is refused where it is stated.
4. A toleration of a NoSchedule taint stating a waiting time. It is refused, ending the Scenario at the Step declaring that Workload.
5. A Workload not tolerating a taint finds no room left on the untainted Nodes. Its extra Pods stay Pending, none placed on a tainted Node.
6. A Then naming a NodePool the Topology lacks. The Scenario ends at that Then, naming it.
7. A Scenario declares a second Workload under a name it already declared. It is refused, ending the Scenario at that Workload's Given.

**Design:** [system-design.md](system-design.md)

### Judging a Workload's Failure tolerance

<a id="req-sim-010"></a>
A Workload declares its Required Capacity, the fewest Pods that must stay Running, so it runs spare Pods to survive a lost Zone `[REQ-SIM-010]`. One Step after a Failure judges that minimum at the moment of the Failure, for what it cost, and at the Settled Placement, for what the cluster restored `[REQ-SIM-010]`. A minimum missed at either moment is a Finding naming that moment `[REQ-SIM-010]`.

**Done when:**

- `[REQ-SIM-010]` given a Workload declaring a Required Capacity, when its Given line is read, then it names the fewest Pods the Workload must keep Running.
- `[REQ-SIM-010]` given nine Pods with a Required Capacity of six, when one Node or one Zone fails, then the judgement counts eight or six Pods at that moment, nine once Settled. Both meet the minimum.
- `[REQ-SIM-010]` given Pods on the Nodes a Failure stopped, or left Pending for want of room, when the capacity is judged, then they are not counted Running.
- `[REQ-SIM-010]` given a minimum missed at one moment, when the Scenario runs, then the run reports a Finding naming that moment and the later Steps still run.
- `[REQ-SIM-010]` given any judgement, when the run reports it, then it states the Pods counted at each moment and the minimum declared, met or not.
- `[REQ-SIM-010]` given every Failure Scenario, when read, then it declares six of nine Pods as its Workload's Required Capacity and judges it, beside the counts it states by hand.

**Edge cases:**

1. A Workload declares a Required Capacity above the Pods it runs. It is refused, ending the Scenario at that Given Step, which names it.
2. A Workload declares a Required Capacity of no Pod. It is refused at that same Step, since such a minimum judges nothing.
3. A Scenario judges a Failure tolerance with no Failure injected before it, or of a Workload declaring no Required Capacity. The Scenario ends at that Then Step, which names it.

**Design:** [system-design.md](system-design.md)

### Explaining a Placement after the run

<a id="req-sim-005"></a>
A reported Placement says where Pods landed, and a Timeline says how they got there `[REQ-SIM-005]`. The lab records in order every change to the Nodes, the Pods and the Workload's own objects, with the control plane's reason for each Placement `[REQ-SIM-005]`. When a Scenario ends, failed or not, the run prints a Timeline summary of each Workload's Pods, or every entry on request `[REQ-SIM-005]`. Every Scenario files its Timeline in one place under the working directory, and the lab removes none `[REQ-SIM-005]`.

**Done when:**

- `[REQ-SIM-005]` given a Scenario that has run, when it ends, then its summary names the Timeline's entry count and file. It gives the lab's own setup as one line, while the file keeps every entry.
- `[REQ-SIM-005]` given its Pods, when the summary is read, then a Pod that reached Running shows its path from Pending and its Node. A Pod left Pending shows the control plane's reason, and a summary with none Pending says so.
- `[REQ-SIM-005]` given an engineer asks for the whole Timeline, when a Scenario ends, then every entry is printed in the order it happened.
- `[REQ-SIM-005]` given a Scenario's Topology and Workload in place, when its Timeline is read, then it holds every change to the Nodes, Pods and Workload's own objects. It carries the control plane's reason for each Pod placed or left Pending.
- `[REQ-SIM-005]` given a run of two Scenarios, or two runs at once on one machine, when they end, then each Scenario left its own Timeline file, none replacing another.
- `[REQ-SIM-005]` given a Scenario's Simulated Cluster is removed, when its Timeline file is opened, then it is still readable.

**Edge cases:**

1. A Scenario fails part-way. The run prints its summary and files its Timeline anyway.
2. The place the Timeline files collect in does not exist when a run starts. The run creates it.
3. A Timeline cannot be written. The run reports the Scenario's outcome and names the Timeline it could not write.
4. A Scenario ends before any Workload is deployed. The summary gives the Topology and names no Workload.
5. A Pod is placed, lost to a Failure, and placed again. Its line gives the whole path.
6. A Scenario deploys more than one Workload. Each gets its own counting line and its own Pod lines.

**Design:** [system-design.md](system-design.md)

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as Steps, one per line `[REQ-SIM-006]`. A Given declares, a When changes the cluster, a Wait waits for a cluster state, and a Then asserts the Placement `[REQ-SIM-006]`. A Scenario handles no error: a failing Step ends it, and a missed Then is a Finding `[REQ-SIM-006]`. A Wait applies what the Givens before it declared, so no Step waits silently, and every Then and Failure judges the Settled Placement it reached `[REQ-SIM-006]`. A Wait's line names the time it took, so after a Failure it shows how long recovery took `[REQ-SIM-006]`. A Wait may state a time of its own within the lab's waiting time `[REQ-SIM-006]`. A Scenario uses only names the lab offers, save Kubernetes' settings under full access, and every Scenario sits in one place beside a README naming each `[REQ-SIM-006]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario of the lab, when read, then each line is one Given, When, Wait or Then Step, and none handles an error. Each name is one the lab offers, apart from those full access names.
- `[REQ-SIM-006]` given any Scenario of the lab, when it runs, then every Wait is a Step of its own reporting its own Wait line. One stands before its first Then and one after every Failure.
- `[REQ-SIM-006]` given a declared Topology or Workload, when its Wait runs, then every declared Node or Pod is Ready and the Wait line names the time taken. Written with its Given or on its own, a Wait reports the same line and Placement.
- `[REQ-SIM-006]` given a Then with no Wait since the Given or Failure before it, when the Scenario runs, then it ends at that Then. A Failure before the first Wait ends it at that When, naming it.
- `[REQ-SIM-006]` given a Wait stating no time, when Pods are not all Ready within the lab's own waiting time, then the Scenario ends there, naming them.
- `[REQ-SIM-006]` given a Wait stating a time, when the cluster reaches its state inside it, then its line names that time and the time taken. When that time passes first, the Scenario ends at that Wait, whose line names that time alone.
- `[REQ-SIM-006]` given a Wait stating a time above the lab's own waiting time, when the Scenario runs, then the time is refused, naming that waiting time.
- `[REQ-SIM-006]` given a Scenario that has Waited, when the Thens after it judge the Placement, then that Placement has stopped changing, even with Pods still Pending.
- `[REQ-SIM-006]` given every Failure Scenario, when read, then it states a time on the Wait after its Failure and none on the Waits after its Givens.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when the Scenario runs, then it ends there, naming that Step. A Then the Placement does not meet only reports a Finding.
- `[REQ-SIM-006]` given the repository, when an engineer looks for the Scenarios, then one place holds them and no machinery, beside a README naming each and its asserted outcome.

**Edge cases:**

1. A Scenario writes two Waits in a row. The second reports a wait of almost none, rather than being refused.
2. A Wait for every Pod stands before any Workload is declared. It waits for no Pod and reports that nothing was waited for.
3. A Wait states a time of no seconds. It is refused where it is stated.
4. A Wait for a Settled Placement stands before any Workload is declared. It is reached at once.

**ADR:** [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

### Reading a Scenario by subject

<a id="req-sim-016"></a>
Each name a Scenario writes shows the subject it belongs to, a Pod, a Node, or the Scenario with its Steps and Topology, unless the Scenarios define it themselves `[REQ-SIM-016]`. A setting of several values stands under its Kubernetes name with each value under its own `[REQ-SIM-016]`. A Scenario stays idiomatic Go, every name defined outside the Scenarios showing where it is defined `[REQ-SIM-016]`. The lab offers only the means to group settings under a name, and its waiting time holds whatever a group states `[REQ-SIM-016]`. The Scenarios define two Named settings `[REQ-SIM-016]`. Every predefined Pod specification names the one for leaving an unresponsive Node `[REQ-SIM-016]`. The Even spread keeps the Skew at one at most over the subject it is given `[REQ-SIM-016]`. Resilient placement names the Even spread over Zones and over Nodes, and a Scenario wanting another spread states it itself `[REQ-SIM-016]`. A Workload's Given line names its intent, and the lines beneath it report every setting its Pods carry, so what a Named setting stands for stays visible `[REQ-SIM-016]`. Every Scenario keeps its asserted outcome `[REQ-SIM-016]`.

**Done when:**

- `[REQ-SIM-016]` given any Scenario source of the lab, when read, then each name it writes shows its subject or is one the Scenarios define. A setting of several values names each value under its Kubernetes name, none only marking a position.
- `[REQ-SIM-016]` given any Scenario source of the lab, when read, then it is idiomatic Go, every name defined outside the Scenarios showing where it is defined. It passes the lab's format and lint checks.
- `[REQ-SIM-016]` given the Named setting for leaving an unresponsive Node, when its definition is read, then it holds the not-ready and unreachable tolerations at the lab's waiting time and nothing else. Every specification names it rather than writing either toleration.
- `[REQ-SIM-016]` given the Even spread, when its definition is read, then it holds one topology spread over the subject given and nothing else. Resilient placement names it over Zones and over Nodes rather than writing either spread.
- `[REQ-SIM-016]` given a Workload declaring a predefined specification, customized or not, when its Given line is read, then it names the intent. The lines beneath report every setting its Pods carry, as customized, one per line under its Kubernetes name.
- `[REQ-SIM-016]` given a Workload declaring Resilient placement, when the lines beneath its Given line are read, then they report both spreads in full.
- `[REQ-SIM-016]` given a setting of several values leaving out one of its values or stating it as zero, when it is stated, then it is refused where it is stated.
- `[REQ-SIM-016]` given a count of Pods stating a Workload's size, its Required Capacity or a Node's room, when read, then it reads by the subject it counts for. The Workload's counts read under the Scenario, the room under the Node.
- `[REQ-SIM-016]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before.

**Edge cases:**

1. A Workload declares Default placement, which holds no spread. The lines beneath its Given line report the two tolerations alone.
2. A Customization writes another time for one of the two tolerations. The lab's waiting time holds, and the lines beneath the Given line report that time.
3. A setting of several values leaves out a value Kubernetes would default. It is refused all the same, since no value deciding a Placement falls to a default. A Scenario needing a zero sets it through full access.

**ADR:** [ADR: Every predefined Pod specification is defined by the Scenarios](adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md) · [ADR: The Scenarios name leaving an unresponsive Node as one setting](adr/2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md) · [ADR: Scenario code stays idiomatic Go, read by subject](adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)  ·  **Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- (none yet)

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Does a Failure stay instantaneous, or does the lab also script one over time, such as a Pod dying after a delay? The timed form is closer to a real outage and costs a second injection surface.
- Is the live view of G-2 a read-only Placement view, or one with Failure injection and Scenario controls? Does either show the Timeline as it grows?
- How long a Timeline file is kept, and whether the lab ever removes one.
- How an engineer reaches a recorded Timeline beyond its file, and whether a Scenario may assert on its own Timeline.
- Does the lab promise anything about a team's own production manifests? That needs the Topology and admission behavior to mirror a named environment, not only a Kubernetes version.
- How a Workload declares the Failures its capacity must survive, whether a run names those the Scenario injected, and whether they mean at once or in sequence. A tolerance no Step exercises promises more than the run proves.
- Whether the lab judges the Placement after a Failure against the best achievable spread. Kubernetes applies spread rules only when placing a Pod, so such a Finding is one the cluster cannot avoid.
- Whether a Required Capacity judgement states a time of its own, beside its Wait's.
- Whether a Zone's Nodes also sit on racks, a failure domain between Zone and Node identified by a Node label. A label a NodePool states holds one value in every Zone, so racks wait for Zones that differ.
- Which Scenario first fails a Node by its position rather than by a count, and what a position the Topology lacks yields. Also open: whether a Failure says if its Nodes share a Zone.
- What a Scenario asserts when every Zone fails, or every Node carries the label value a Failure names, leaving no Node to place a Pod on.
- Whether a Failure by label names several values at once, or stops the Nodes of one NodePool by its name.
- Whether a NodePool's taints also take the PreferNoSchedule effect, which Pods avoid while other Nodes have room, or NoExecute, which moves placed Pods away and meets the Failures' tolerations.
- Whether a Workload is drawn onto a NodePool by selecting it, beside a Request only that NodePool's Nodes have room for.
- What limits a Topology's Zone count, beyond the lab's own refusal above a limit, since numbered Zone names set no bound.
- Which Failure the Scenario after Zone stops responding injects: a Pod, or a Node removed from the cluster rather than left unresponsive.
- Whether a Scenario demonstrates a Customization or full access, and which intent it shows.
- Whether a Scenario changes a Workload's specification after deployment, a rollout, where a Customization applies before deployment only.
- Whether the lab says anything when full access leaves out a setting the lab depends on, which today holds silently.
- Whether a Then asserts which Pods stay Pending, or their count alone, as No room after a Zone stops responding does.
- Whether the lab's vocabulary names a Pod's limits beside its Request, and what a Scenario would assert with them. A limit decides no Placement, and full access reaches one today.
- Whether a run also names one Startup time for every Pod, beside the one a Scenario states for its Workload.
- Whether an engineer can ask a run to carry out its Scenarios one at a time, for a machine without room for every Simulated Cluster at once.
- Whether the lab also gives a removed Pod a time to stop, as it gives one to start.
- Which further specifications the Scenarios define, and which further settings one may hold and a Customization may name. Zone spread only, Soft spread, and affinity between a Workload's Pods each arrive with their first Scenario. Also open: what a Scenario asserts when demonstrating a deliberately weak specification.
