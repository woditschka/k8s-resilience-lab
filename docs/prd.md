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
Every Scenario runs in a Simulated Cluster of its own, provisioned with the Scenario's Topology before any Workload is deployed and removed when the Scenario ends `[REQ-SIM-001]`. A Scenario names its Topology, its Workload with that Workload's Pod specification, and the Placement it asserts `[REQ-SIM-001]`. The lab reports the Pods Running and Pending per Zone and per Node, each named by its position in the order the Topology declares it `[REQ-SIM-001]`. Every run reports the Kubernetes version it mirrors and touches nothing in the container runtime but its own Simulated Cluster `[REQ-SIM-001]`. Resilient placement is the simplest Scenario: nine Pods spread over three Zones of three Nodes `[REQ-SIM-001]`.

<a id="req-sim-002"></a>
One invocation with no arguments checks the lab's own rules, then runs every Scenario that needs a Simulated Cluster and reports each outcome afresh `[REQ-SIM-002]`.

<a id="req-sim-014"></a>
A run carries out its Scenarios at once, each on its own Simulated Cluster, so it lasts about as long as its longest Scenario `[REQ-SIM-014]`. Its report still reads as before: each Scenario's lines stand together, in the order the Scenarios are declared `[REQ-SIM-014]`. A Scenario that fails ends alone, and the others run to their own outcome `[REQ-SIM-014]`. The lab's own tests that need a Simulated Cluster also run at once, and those judging a full run's report share one run `[REQ-SIM-014]`.

<a id="req-sim-003"></a>
An engineer names one Scenario to re-run it alone, and a name matching no Scenario stops the run before a Simulated Cluster is provisioned `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
One command decides whether the lab itself is sound: its rules, its tests, its build, and that no part can reach a cluster the lab did not provision `[REQ-SIM-004]`. The repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The same command holds the lab to the dependencies it declared, free of known vulnerabilities `[REQ-SIM-007]`. Every connection the lab opens to a Simulated Cluster closes when its work ends `[REQ-SIM-007]`.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when Resilient placement runs, then the lab reports a Topology of three Zones, each holding the NodePool general of three Nodes, every Node Ready.
- `[REQ-SIM-001]` given Resilient placement's Topology, when its nine-Pod Workload is deployed, then the reported Placement is Settled at nine Running Pods, three in every Zone and one on every Node.
- `[REQ-SIM-001]` given an earlier run created Nodes and Pods, when a run starts, then the reported Placement names only the Nodes and Pods of the current run.
- `[REQ-SIM-001]` given the same Topology, when two runs are compared, then both report the same Zone and Node names.
- `[REQ-SIM-001]` given a Topology of three Zones, when a run reports its Placement, then each Zone and each Node is named by its position, counted from one.
- `[REQ-SIM-001]` given a run names no Kubernetes version, when it starts, then it reports the lab's pinned default version.
- `[REQ-SIM-001]` given a run names a Kubernetes version, when it starts, then its Simulated Cluster mirrors that version and the run reports it.
- `[REQ-SIM-001]` given other containers run on the machine, when a run starts and ends, then the lab's own Simulated Cluster is created and removed and no other container is touched.
- `[REQ-SIM-001]` given a Scenario that has done its work, when it ends, then its Simulated Cluster no longer exists on the machine.
- `[REQ-SIM-001]` given a second run is already under way on the machine, when a run starts and ends, then each run removes only the Simulated Cluster it created.
- `[REQ-SIM-002]` given a reachable container runtime, when the lab is invoked with no arguments, then every Scenario that needs a Simulated Cluster runs and reports its name and outcome.
- `[REQ-SIM-002]` given the lab's own rules fail, when the lab is invoked, then the run stops before a Simulated Cluster is provisioned.
- `[REQ-SIM-002]` given a run already passed and nothing changed since, when the lab is invoked again, then the Scenarios run again instead of repeating the earlier outcome.
- `[REQ-SIM-002]` given no container runtime is reachable, when the lab is invoked, then the run names the missing runtime and no Scenario starts.
- `[REQ-SIM-014]` given a run of every Scenario, when it starts, then each Scenario starts without waiting for another to end.
- `[REQ-SIM-014]` given a run of every Scenario, when it reports, then each Scenario's lines stand together, in declared order, whatever order the Scenarios finished in.
- `[REQ-SIM-014]` given Scenarios carried out at once, when each reports its Placement and its Timeline, then each names only its own Nodes and Pods.
- `[REQ-SIM-014]` given one Scenario fails part-way, when the run ends, then every other Scenario has reached its own outcome and no Simulated Cluster of the run remains.
- `[REQ-SIM-014]` given the lab's own tests, when the one command runs them, then the tests needing a Simulated Cluster run at once rather than one after another.
- `[REQ-SIM-014]` given the lab's own tests judging the lines of a run of every Scenario, when they run, then they share one such run instead of each starting its own.
- `[REQ-SIM-003]` given more than one Scenario exists, when the engineer names one of them, then only that Scenario runs and reports its outcome.
- `[REQ-SIM-003]` given a name that matches no Scenario, when the run starts, then it stops and names the Scenarios that exist.
- `[REQ-SIM-004]` given the declared toolchain is installed, when the one command runs, then the lab's own rules, its own tests and its build all run.
- `[REQ-SIM-004]` given any one of those checks fails, when the one command finishes, then it reports failure.
- `[REQ-SIM-004]` given a part of the lab could reach a cluster the lab did not provision itself, when the one command runs, then it fails and names that part.
- `[REQ-SIM-004]` given the repository, when an engineer looks for the lab, then exactly one implementation of it is present.
- `[REQ-SIM-007]` given a dependency differs from the one the lab declared, when the one command runs, then it fails and names that dependency.
- `[REQ-SIM-007]` given a dependency carries a known vulnerability, when the one command runs, then it fails and names that vulnerability.
- `[REQ-SIM-007]` given a part of the lab whose name resembles a permitted one, when the one command runs, then that part is reported rather than permitted.
- `[REQ-SIM-007]` given the lab's own tests have run, when the run ends, then no connection the lab opened to a Simulated Cluster is still open.

**Edge cases:**

1. A Scenario's Workload declares more Pods than its Topology can hold. The lab reports the unplaceable Pods as Pending, and a Scenario that asserts every Pod Running fails.
2. The machine's container runtime answers somewhere other than the conventional location. The run finds it, rather than reporting that none is reachable.
3. A stale runtime address is present alongside a live runtime. The run reaches the live one.
4. A run names a Kubernetes version the lab cannot obtain. The run stops and names that version before any Scenario starts.
5. A Scenario fails part-way through its steps. Its Simulated Cluster is removed anyway, and the run reports the failure.
6. A run names one Scenario. It runs that Scenario alone and reports it as a run of every Scenario reports it.
7. The container runtime cannot start one Scenario's Simulated Cluster while others start. That Scenario ends at its first Step and names why, and the others run on. A version the lab cannot obtain is no such case: edge case 4 holds.

**ADR:** [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md) · [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Workload's Pods

<a id="req-sim-009"></a>
A Scenario states the intent behind its Workload, not the settings that carry it out `[REQ-SIM-009]`. Its Workload declares one of the lab's predefined Pod specifications, one per intent `[REQ-SIM-009]`. Each holds only the settings that decide where Pods land and when they leave a Node `[REQ-SIM-009]`. A definition names each setting as Kubernetes does, the lab's own additions included, so one place holds all that decides a Placement `[REQ-SIM-009]`. Resilient placement spreads Pods evenly over Zones and Nodes `[REQ-SIM-009]`. Default placement, its deliberately weak counterpart, leaves the scheduler alone to decide `[REQ-SIM-009]`.

Customizations change a copy of the declared specification before deployment, in the order written, naming settings as Kubernetes names them `[REQ-SIM-009]`. The lab's own vocabulary names the settings that decide where Pods land `[REQ-SIM-009]`. Full access is the Customization reaching every other Kubernetes Pod setting `[REQ-SIM-009]`. Whatever it names, the Pods count as the Workload's and the lab's waiting time before Pods leave an unresponsive Node holds `[REQ-SIM-009]`. No Scenario uses full access yet `[REQ-SIM-009]`.

**Done when:**

- `[REQ-SIM-009]` given any Scenario of the lab, when its Workload line is read, then it declares one predefined Pod specification, with zero or more Customizations of it and nothing else.
- `[REQ-SIM-009]` given a Workload declaring a specification it customizes, when the settings its Pods were deployed with are read back, then the Customization's settings stand and the others stand unchanged.
- `[REQ-SIM-009]` given one Workload customized a specification, when a second Workload declares the same specification, then its Pods are deployed with the predefined settings.
- `[REQ-SIM-009]` given a predefined specification, when its definition is read, then every setting it holds stands there with the value it sets, under the name Kubernetes gives that setting.
- `[REQ-SIM-009]` given a Workload declaring a predefined specification, when the settings its Pods were deployed with are read back, then they are the ones that specification's definition names.
- `[REQ-SIM-009]` given the Resilient placement Scenario's Topology and its nine-Pod Workload declaring that specification, when the Placement settles, then three Pods are in every Zone and one on every Node.
- `[REQ-SIM-009]` given the same Topology and a nine-Pod Workload declaring Default placement, when the Placement settles, then every Pod is Running and no count per Zone is promised.
- `[REQ-SIM-009]` given two Workloads of one Scenario declaring different specifications, when both are deployed, then each is placed by the specification it declares.
- `[REQ-SIM-009]` given a Workload customized with full access, when its Pods' settings are read, then everything full access named stands, except the waiting time before Pods leave an unresponsive Node.
- `[REQ-SIM-009]` given full access written beside a Customization of the lab's own vocabulary, when both name one setting, then the one written later stands.
- `[REQ-SIM-009]` given full access that leaves out the waiting time before Pods leave an unresponsive Node, when the Workload is deployed, then that waiting time still holds.
- `[REQ-SIM-009]` given a Workload customized with full access, when its Placement is reported, then its Pods are counted as that Workload's.

**Edge cases:**

1. A Workload declaring Default placement on a Topology too small for its Pods. The Pods the scheduler cannot place stay Pending with its reason.
2. A Workload declaring Resilient placement on a Topology of one Zone. Its Pods still land one per Node, and that Zone holds them all.
3. Two Workloads of one Scenario declare the same specification. Each is spread on its own, without the other's Pods counting towards its Skew.
4. A specification declares no spread of its own. Its definition still names the settings the lab adds, so no specification is empty.
5. Two Customizations of one Workload name one setting. The specification deployed holds the value the later one sets.
6. A Customization names a setting its specification does not hold. The specification gains that setting, rather than the Customization being refused.
7. Full access leaves out a setting the lab depends on. That setting holds and the Workload is deployed, rather than the Scenario ending at its Step.
8. Full access names a setting the control plane refuses. The Scenario ends at the Step that deployed its Workload, as a refused value of any Customization does.

**ADR:** [ADR: A Workload declares one of the lab's predefined Pod specifications](adr/2026-10-05-workload-declares-a-predefined-pod-specification.md) · [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md)  ·  **Design:** [system-design.md](system-design.md)

### Fitting a Workload's Pods onto the Nodes

<a id="req-sim-011"></a>
A Node has room for only so much, and a Pod asks for a share of it `[REQ-SIM-011]`. A NodePool states its Nodes' Node capacity, or keeps the lab's own default, and every run reports it `[REQ-SIM-011]`. A Pod specification may hold a Request; one stating none asks for nothing, as every Scenario of the lab does `[REQ-SIM-011]`. A Pod no Node has room for stays Pending `[REQ-SIM-011]`. A Failure therefore leaves a Workload the room on the Nodes still responding, not those Nodes alone `[REQ-SIM-011]`. No room after a Zone stops responding demonstrates it: losing a Zone costs the spare Pods and not the Required Capacity `[REQ-SIM-011]`.

**Done when:**

- `[REQ-SIM-011]` given a NodePool stating its Nodes' cpu, memory and Pod count, when it is provisioned, then every Node of it has that capacity and the run reports it in that order.
- `[REQ-SIM-011]` given a NodePool stating the cpu and memory alone, when it is provisioned, then its Nodes hold the lab's own Pod count and the run reports it.
- `[REQ-SIM-011]` given a NodePool stating no Pod count, when its Topology's Given line is reported, then that line names its cpu and memory alone.
- `[REQ-SIM-011]` given a NodePool stating no Node capacity, when it is provisioned, then its Nodes have the lab's own default Pod count, cpu and memory.
- `[REQ-SIM-011]` given a Workload whose specification states a Request, when the settings its Pods were deployed with are read back, then that Request stands under the name Kubernetes gives it.
- `[REQ-SIM-011]` given a Workload whose specification states no Request, when its Pods are placed, then they ask for no cpu and no memory and the spread alone decides the Placement.
- `[REQ-SIM-011]` given a Workload requesting more cpu than its Topology has room for, when the Placement settles, then the Pods that fit are Running and the rest stay Pending.
- `[REQ-SIM-011]` given a Topology whose Nodes have room for one Pod each and a nine-Pod Workload, when the Placement settles, then nine Pods are Running, one on every Node.
- `[REQ-SIM-011]` given that Placement, when one Zone stops responding, then the Placement settles at six Pods Running and three Pending, and the Scenario asserts both.
- `[REQ-SIM-011]` given that Scenario, when its judgement of Required Capacity is reported, then it counts six Pods at the moment of the Failure and six once Settled, meeting the minimum.

**Edge cases:**

1. A NodePool states a Node capacity of no Pod, of no cpu, or of no memory. It is refused where it is stated, so a Node always has room for a Pod that asks for something. An omitted Pod count is no such statement, and keeps the lab's own count.
2. A Workload's Request is larger than any one Node of its Topology has room for. Every Pod of that Workload stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter of the two bounds decides, and the Pod count never comes into it.
4. A Customization written after a Request states another one. The later Request stands, as a later setting of any kind does.

**Design:** [system-design.md](system-design.md)

### Holding Nodes of different kinds in a Zone

<a id="req-sim-013"></a>
A real cluster seldom runs one kind of machine, so a Zone holds NodePools of identical Nodes, the same NodePools in every Zone `[REQ-SIM-013]`. A Topology states its Zone count and its NodePools, and nothing else `[REQ-SIM-013]`. Every NodePool states its name, its Node count and its Node capacity, the capacity defaulting as REQ-SIM-011 states `[REQ-SIM-013]`. Every Node belongs to exactly one NodePool and carries its name as a label `[REQ-SIM-013]`. Node names stay positional as REQ-SIM-001 states `[REQ-SIM-013]`. NodePools of different capacity decide where Pods land `[REQ-SIM-013]`.

**Done when:**

- `[REQ-SIM-013]` given three Zones each holding two Nodes of NodePool general and one larger Node of NodePool gpu, when provisioned, then every Zone holds three Ready Nodes with their NodePools' capacity.
- `[REQ-SIM-013]` given that Topology, when its Nodes are read from the cluster, then every Node carries a label naming its NodePool, general or gpu.
- `[REQ-SIM-013]` given that Topology, when a run reports its Placement, then each Zone's Nodes are numbered one to three, the two Nodes of general before the gpu Node.
- `[REQ-SIM-013]` given that Topology, when the run starts, then it reports each NodePool by its name, its Node count per Zone, and its cpu, memory and Pod count.
- `[REQ-SIM-013]` given that Topology and a Request only gpu Nodes have room for, when the Placement settles, then its Running Pods sit on gpu Nodes alone.
- `[REQ-SIM-013]` given every Scenario of the lab, each naming its one NodePool general, when it runs, then it reports that NodePool and reaches the outcome it asserted before.

**Edge cases:**

1. A NodePool of no Node. It is refused where it is stated.
2. Two NodePools of one Topology under the same name. They are refused where they are stated, since one NodePool's Nodes could not be told from the other's.
3. A NodePool name Kubernetes cannot hold as a label value. It is refused where it is stated.
4. A NodePool stating no Node capacity. Its Nodes take the lab's own default, as REQ-SIM-011 states.
5. A NodePool stating no name, or a Topology stating no NodePool. It is refused where it is stated.

**Design:** [system-design.md](system-design.md)

### Starting a Workload's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready, and a Workload replacing Pods after a Failure stays short of them meanwhile `[REQ-SIM-012]`. Every placed Pod therefore takes a Startup time, counted Pending on its Node until it is Ready `[REQ-SIM-012]`. A Pod replacing one lost in a Failure counts from its own placement `[REQ-SIM-012]`. The lab's own Startup time holds unless a Customization states another, in whole seconds `[REQ-SIM-012]`. It states one time, or a shortest and a longest between which each Pod takes its own, both included `[REQ-SIM-012]`. It is the one Customization that is no Kubernetes setting `[REQ-SIM-012]`.

**Done when:**

- `[REQ-SIM-012]` given a Workload whose specification states no Startup time, when its Pods are placed, then no Pod is Ready sooner than the lab's own Startup time after its placement.
- `[REQ-SIM-012]` given a Workload customized with one Startup time, when its Pods are placed, then no Pod is Ready sooner than that time after its placement.
- `[REQ-SIM-012]` given a Workload customized with a shortest and a longest Startup time, when its Pods are placed, then each Pod is Ready between the two after its placement.
- `[REQ-SIM-012]` given a placed Pod inside its Startup time, when the Placement is reported, then that Pod is counted Pending on its Node, and the Placement is not yet Settled.
- `[REQ-SIM-012]` given two Customizations of one Workload each stating a Startup time, when its Pods are placed, then they take the later one.
- `[REQ-SIM-012]` given one Workload customized its Startup time, when a second Workload declares the same specification, then its Pods take the lab's own Startup time.
- `[REQ-SIM-012]` given any run, when it starts, then it reports the lab's own Startup time.
- `[REQ-SIM-012]` given every Scenario of the lab, when it runs, then it reaches the outcome it asserted before Pods took a Startup time.

**Edge cases:**

1. A Startup time of zero. The Pod is Ready as soon as it is placed.
2. A shortest Startup time longer than its longest, or a negative time. It is refused where it is stated, and the Scenario ends at the Step declaring that Workload.
3. A shortest and a longest Startup time that are equal. Every Pod takes that one time.
4. A Startup time longer than the time a Wait for every Pod to be Ready is held to. The Scenario ends at that Wait, which names the Pods not Ready.
5. A Pod replacing one lost in a Failure. It counts as Pending on its new Node until its own Startup time has passed, so a Required Capacity judged meanwhile does not count it.

**Design:** [system-design.md](system-design.md)

### Surviving a Node or Zone Failure

<a id="req-sim-008"></a>
A Scenario injects a Failure and judges the Placement on both sides of it `[REQ-SIM-008]`. Node stops responding stops one Node of Resilient placement responding, as a hung or disconnected machine does `[REQ-SIM-008]`. Zone stops responding stops every Node of one Zone in one Step `[REQ-SIM-008]`. A stopped Node stays in the cluster, and the control plane moves its Pods away after a waiting time the lab shortens to a few seconds `[REQ-SIM-008]`. A Scenario says how many Nodes or Zones fail, and the lab picks them the same way on every run `[REQ-SIM-008]`. A surviving Node then holds two Pods, so Node stops responding asserts no Placement of one Pod per Node `[REQ-SIM-008]`. Losing a Zone costs the spread, so Zone stops responding asserts no count per Zone or Node; the lab accepts that while the Pod count holds `[REQ-SIM-008]`.

**Done when:**

- `[REQ-SIM-008]` given Resilient placement's Topology and Workload are Settled, when the Placement is judged before any Failure, then nine Pods are Running and three sit in every Zone.
- `[REQ-SIM-008]` given Resilient placement's Topology and Workload are Settled, when one Node stops responding, then that Node's Pod is moved away within a few seconds.
- `[REQ-SIM-008]` given a Node has stopped responding, when the Placement settles again, then nine Pods are Running, three in every Zone, and none on that Node.
- `[REQ-SIM-008]` given Node stops responding says one Node fails, when it runs twice, then the same Node fails in both runs and the run reports which.
- `[REQ-SIM-008]` given any run, when it starts, then it reports that Pods leave an unresponsive Node sooner than in a default cluster.
- `[REQ-SIM-008]` given Node stops responding, when its lines are read, then the When Step that injects the Failure carries no Node name and states no waiting time.
- `[REQ-SIM-008]` given Resilient placement's Topology and Workload are Settled, when every Node of one Zone stops responding, then those Nodes' Pods are moved away within a few seconds.
- `[REQ-SIM-008]` given one Zone has stopped responding, when the Placement settles again, then nine Pods are Running and none sits in that Zone.
- `[REQ-SIM-008]` given Zone stops responding says one Zone fails, when it runs twice, then the same Zone fails in both runs and the run reports which.
- `[REQ-SIM-008]` given Zone stops responding, when its lines are read, then the When Step that injects the Failure carries no Zone name.

**Edge cases:**

1. A Pod of a Node that stopped responding cannot be placed anywhere else. It stays Pending with the control plane's reason.
2. A When asks for more failing Nodes, or more failing Zones, than the Topology holds. The Scenario ends at that Step and names it.
3. Not every Pod is Ready again within the lab's waiting time after a Failure. The Scenario ends at that Wait, which names the Pods that were not Ready.

**ADR:** [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

### Judging a Workload's Failure tolerance

<a id="req-sim-010"></a>
A Workload declares its Required Capacity, the fewest Pods that must stay Running, so it runs spare Pods to survive a lost Zone `[REQ-SIM-010]`. One Step after a Failure judges that minimum at two moments `[REQ-SIM-010]`. The moment of the Failure says what it cost, and the Settled Placement says what the cluster restored `[REQ-SIM-010]`. A minimum missed at either moment is a Finding naming that moment `[REQ-SIM-010]`. Judging capacity and counting Pods by hand are both open to a Scenario, and every Failure Scenario declares six Pods of nine `[REQ-SIM-010]`.

**Done when:**

- `[REQ-SIM-010]` given a Workload declaring a Required Capacity, when its Given line is read, then that line names the fewest Pods the Workload must keep Running.
- `[REQ-SIM-010]` given a nine-Pod Workload declaring a Required Capacity of six, when one Node fails, then the judgement counts eight Pods at that moment and nine once Settled.
- `[REQ-SIM-010]` given the same Workload, when one whole Zone fails, then the judgement counts six Pods at that moment and nine once Settled, which meets the minimum.
- `[REQ-SIM-010]` given Pods on the Nodes a Failure stopped, when the capacity is judged at the moment of that Failure, then those Pods are not counted as Running.
- `[REQ-SIM-010]` given a minimum the Placement misses at one moment, when the Scenario runs, then the run reports a Finding naming that moment and the later Steps still run.
- `[REQ-SIM-010]` given any judgement of a Failure tolerance, when the run reports it, then it states the Pods counted at each moment and the minimum declared, met or not.
- `[REQ-SIM-010]` given every Failure Scenario, when its lines are read, then it declares its Workload's Required Capacity and judges it, beside the counts it states by hand.
- `[REQ-SIM-010]` given Pods left Pending for want of room, when the capacity is judged once the Placement is Settled, then they are not counted and the minimum is still met.

**Edge cases:**

1. A Workload declares a Required Capacity above the Pods it runs. The declaration is refused and the Scenario ends at that Given Step, which names it.
2. A Workload declares a Required Capacity of no Pod. It is refused at that same Step, because such a minimum judges nothing.
3. A Scenario judges a Failure tolerance with no Failure injected before it. The Scenario ends at that Then Step, which names it.
4. A Scenario judges the Failure tolerance of a Workload that declared no Required Capacity. The Scenario ends at that Then Step, which names it.
5. Every Pod of a Workload sits on the Nodes a Failure stopped. The count at the moment of the Failure is no Pod, and the judgement reports a Finding.

**Design:** [system-design.md](system-design.md)

### Explaining a Placement after the run

<a id="req-sim-005"></a>
A reported Placement says where Pods landed; a Timeline says how they got there `[REQ-SIM-005]`. The lab records in order every change to the Nodes, the Pods and the Workload's own objects, with the control plane's reason for each Placement `[REQ-SIM-005]`. When a Scenario ends, failed or not, the run prints a Timeline summary, or every entry on request `[REQ-SIM-005]`. The summary collapses the lab's own setup to one line and gives each Workload a line counting its Running Pods `[REQ-SIM-005]`. Each Pod's line gives its path to Running or its Pending reason, and the summary says plainly when none is Pending `[REQ-SIM-005]`. Every Scenario files its Timeline in one place under the working directory, and the lab removes none `[REQ-SIM-005]`.

**Done when:**

- `[REQ-SIM-005]` given a Scenario that has run, when it ends, then the summary opens with the Timeline's entry count and the file it was filed at.
- `[REQ-SIM-005]` given a Timeline whose entries are mostly the lab's own setup, when the summary is printed, then that setup is one line and every entry is still in the file.
- `[REQ-SIM-005]` given a Pod that reached Running, when the summary is read, then its line gives its path from Pending to Running and the Node it landed on.
- `[REQ-SIM-005]` given a Pod left Pending, when the summary is read, then its line gives the control plane's reason for it.
- `[REQ-SIM-005]` given an engineer asks for the whole Timeline, when a Scenario ends, then every entry is printed in the order it happened.
- `[REQ-SIM-005]` given a Scenario's Topology and Workload are in place, when its Timeline is read, then every change to the Nodes, the Pods and the Workload's own objects is in it.
- `[REQ-SIM-005]` given one Pod was placed and one stayed Pending, when the Timeline is read, then it carries the control plane's reason for each.
- `[REQ-SIM-005]` given a run of two Scenarios, when it ends, then each Scenario has left one Timeline file and neither file replaced the other.
- `[REQ-SIM-005]` given two runs at once on one machine, when both end, then each run has its own Timeline files.
- `[REQ-SIM-005]` given a Scenario's Simulated Cluster has been removed, when its Timeline file is opened, then the Timeline is still readable.

**Edge cases:**

1. A Scenario fails part-way through its Steps. The run prints its summary and files its Timeline anyway.
2. The place the Timeline files collect in does not exist when a run starts. The run creates it instead of failing.
3. A Timeline cannot be written. The run reports the Scenario's outcome and names the Timeline it could not write.
4. A Scenario ends before any Workload is deployed. The summary gives the Topology and names no Workload.
5. A Pod is placed, lost to a Failure, and placed again. Its line gives the whole path, not its first Placement alone.
6. A Scenario deploys more than one Workload. Each Workload gets its own counting line and its own Pod lines.

**Design:** [system-design.md](system-design.md)

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as Steps, one per line `[REQ-SIM-006]`. A Given declares, a When changes the cluster, a Wait waits for a cluster state, and a Then asserts the Placement `[REQ-SIM-006]`. A Scenario handles no error: a failing Step ends it, and a missed Then is a Finding `[REQ-SIM-006]`.

A Given declares, and the Wait after it applies what the Givens before it declared, so no Step waits silently `[REQ-SIM-006]`. Every Then and every Failure judges the Settled Placement a Wait reached `[REQ-SIM-006]`. A Wait's line names the time it took, so after a Failure it says how long the cluster took to recover `[REQ-SIM-006]`. A Wait may state a time to be reached in, bounded by the lab's own waiting time `[REQ-SIM-006]`. A Wait for a Settled Placement alone serves a Scenario expecting Pods to stay Pending `[REQ-SIM-006]`.

Every name a Scenario uses is one the lab offers, except Kubernetes' own settings under full access `[REQ-SIM-006]`. Every Scenario sits in one place, apart from the machinery, beside a README naming each `[REQ-SIM-006]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario of the lab, when its lines are read, then each line is one Given, When, Wait or Then Step and no line handles an error.
- `[REQ-SIM-006]` given any Scenario of the lab, when the names in its lines are read, then each is a name the lab offers, apart from the settings full access names.
- `[REQ-SIM-006]` given any Scenario of the lab, when it runs, then every Wait of it is a Step of its own and reports a Wait line of its own.
- `[REQ-SIM-006]` given a Scenario that declared a Workload, when its Wait runs, then every Pod of that Workload is Ready and its Wait line names the time it took.
- `[REQ-SIM-006]` given a Scenario that declared a Topology, when its Wait runs, then every Node of that Topology is Ready and its Wait line names the time it took.
- `[REQ-SIM-006]` given any Scenario of the lab, when its lines are read, then it Waits before its first Then and again after every Failure.
- `[REQ-SIM-006]` given a Wait written with its Given and one written on its own, when both run, then both report the same Wait line and Placement.
- `[REQ-SIM-006]` given a Scenario that has Waited, when the Thens after that Wait judge the Placement, then the Placement they judge has stopped changing.
- `[REQ-SIM-006]` given a Then with no Wait since the Given or the Failure before it, when the Scenario runs, then it ends at that Then.
- `[REQ-SIM-006]` given a Failure injected before the Scenario has Waited once, when the Scenario runs, then it ends at that When and names it.
- `[REQ-SIM-006]` given Pods not all Ready within the lab's waiting time, when the Wait runs, then the Scenario ends there and names the Pods not Ready.
- `[REQ-SIM-006]` given a Wait stating a time, when the cluster reaches what it waits for inside that time, then its line names that time and the time it took.
- `[REQ-SIM-006]` given a Wait stating a time, when that time passes first, then the Scenario ends at that Wait, whose line names that time alone.
- `[REQ-SIM-006]` given a Wait stating no time, when it runs, then the lab's own waiting time holds for it.
- `[REQ-SIM-006]` given a Wait stating a time above the lab's own waiting time, when the Scenario runs, then the time is refused and the refusal names that waiting time.
- `[REQ-SIM-006]` given a Wait stating a time written with its Given, and one written on its own, when both run, then each is held to the time it states.
- `[REQ-SIM-006]` given a Placement that stopped changing with Pods still Pending, when the Wait for a Settled Placement runs, then it is reached and the Thens after it judge that Placement.
- `[REQ-SIM-006]` given every Failure Scenario, when its lines are read, then it states a time on the Wait after its Failure and none on the Waits after its Givens.
- `[REQ-SIM-006]` given any run, when it starts, then it reports the lab's own waiting time.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when the Scenario runs, then it ends, names that Step, and runs no later Step.
- `[REQ-SIM-006]` given a Then the Placement does not meet, when the Scenario runs, then the run reports the Finding and the later Steps still run.
- `[REQ-SIM-006]` given Resilient placement written as Steps, when it runs, then it reports the Placement REQ-SIM-001 asserts.
- `[REQ-SIM-006]` given an engineer opening the repository, when they look for the Scenarios, then one place holds every Scenario and nothing of the machinery.
- `[REQ-SIM-006]` given that place, when an engineer reads its README, then the README names every Scenario and the outcome each asserts.

**Edge cases:**

1. A Scenario writes two Waits in a row. The second reports a wait of almost none, rather than being refused.
2. A Wait for every Pod stands before any Workload is declared. It waits for no Pod and reports that nothing was waited for.
3. A Wait states a time of no seconds. It is refused where it is stated, since no state of the cluster is reached in no time.
4. A Wait for a Settled Placement stands before any Workload is declared. It is reached at once, as a Wait with nothing to wait for is.

**ADR:** [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- (none yet)

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Does a Failure stay instantaneous, or does the lab also script a Failure over time — a Pod that dies after a delay, a Node that degrades before it is lost? The timed form is closer to a real outage and costs a second injection surface.
- What does the live view show, and what can an engineer do from it? Open options: a read-only view of the current Placement, or the same view with Failure injection and Scenario controls in it (G-2). Does either form show the Timeline as it grows?
- How long a Timeline file is kept, and whether the lab ever removes one.
- How an engineer reaches a recorded Timeline beyond opening its file, and whether a Scenario may assert on its own Timeline.
- Does the lab promise anything about a team's own production manifests, or only about the manifests it declares itself? A run mirrors a chosen Kubernetes version, so the stronger promise needs the Topology and the admission behavior to mirror a named environment as well.
- How a Workload declares the Failures its capacity has to survive, and whether a run says which of them the Scenario injected. A declared tolerance no Step exercises promises more than the run proves. Also open: whether such a declaration means those Failures at once or one after another.
- Whether the lab computes the best achievable spread after a Failure and judges the Placement against it, beside the counts a Scenario states by hand. Kubernetes applies spread rules when it places a Pod and never moves a Running one, so a computed judgement would report a Finding the cluster cannot avoid.
- Whether a Required Capacity judgement states a time of its own, beside the time the Wait before it is held to. A Wait's time covers every Pod becoming Ready, which is longer than the capacity alone taking until it is restored.
- Whether a Zone's Nodes also sit on racks, as a further failure domain between the Zone and the Node, identified by what a Node is labelled with.
- Which Scenario first points at a Node by its position in the Topology its Given declared, rather than saying how many Nodes fail. Both forms are settled vocabulary; only the count form exists today, and a position the Topology does not hold has no stated outcome. Also open: whether a Failure says where the failing Nodes sit, in one Zone or in different ones.
- What a Scenario asserts when every Zone of its Topology fails. The count form permits it, and no Node is then left to place a Pod on.
- What limits how many Zones a Topology may name. The lab refuses a count above a limit of its own, and a numbered Zone name puts no bound on the count.
- Which Failure the Scenario after Zone stops responding injects — a Pod, or a Node removed from the cluster rather than left unresponsive.
- Whether a Scenario of the lab demonstrates a Customization or full access, and which intent it would show. No Scenario names either today, and each arrives with the Scenario that needs it.
- Whether a Scenario changes a Workload's specification after that Workload is deployed. Today a Customization applies before deployment only, and a later change is a rollout the control plane carries out under its update strategy.
- Whether the lab says anything when full access leaves out a setting the lab depends on. Today that setting holds and the run stays silent about it, so a Scenario's own line is where an engineer reads what it asked for.
- Whether a run reports the specification each Workload was deployed with, and its settings after Customization.
- Whether a Then asserts which Pods stay Pending, or their count alone, which is what No room after a Zone stops responding asserts.
- Whether the lab's own vocabulary names a Pod's limits beside its Request, and what a Scenario would assert with them. Nothing runs in a Simulated Cluster, so a limit decides no Placement, and full access reaches one today.
- Whether a run also names one Startup time for every Pod it starts, beside the Startup time a Scenario states for its own Workload.
- Whether an engineer can ask a run to carry out its Scenarios one at a time, for a machine without room for every Simulated Cluster at once.
- Whether the lab also gives a Pod a time to stop once it is removed, as it gives one to start.
- Which further specifications the lab predefines, and which further settings one may hold and a Customization may name. Zone spread only and Soft spread each arrive with the Scenario that uses them, and so does affinity between a Workload's Pods. Also open: what a Scenario asserts when it demonstrates a deliberately weak specification.
