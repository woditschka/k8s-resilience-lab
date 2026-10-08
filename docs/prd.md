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
The lab brings up a Simulated Cluster of its own for every Scenario and provisions that Scenario's Topology into it before any Workload is deployed `[REQ-SIM-001]`. Each Scenario names the Topology it provisions, the Workload it deploys with the Pod specification that Workload declares, and the Placement it asserts `[REQ-SIM-001]`. The lab reports that Placement per Zone and per Node, counting the Pods that are Running and the Pods that stay Pending `[REQ-SIM-001]`. Zone and Node names follow from the Topology alone, so every run of the same Topology reports the same names `[REQ-SIM-001]`. Each Zone is numbered from one in the order the Topology declares it, and each Node from one within its Zone `[REQ-SIM-001]`. A reported name therefore states the position its Zone or Node holds `[REQ-SIM-001]`. A Scenario's Simulated Cluster is removed when the Scenario ends, also when it fails `[REQ-SIM-001]`. No Node or Pod of an earlier Scenario can be mistaken for a current one `[REQ-SIM-001]`.

The lab's simplest Scenario is Resilient placement, named for the intent its Workload declares. It provisions three Zones with three Nodes each, which leaves two Nodes in a Zone once one of its Nodes is lost `[REQ-SIM-001]`. Its nine-Pod Workload declares the Resilient placement Pod specification, and the Scenario asserts three Pods in every Zone and one Pod on every Node `[REQ-SIM-001]`.

Every run reports the Kubernetes version its Simulated Cluster mirrors `[REQ-SIM-001]`. An engineer names a version for a single run, and a run that names none mirrors the lab's pinned default `[REQ-SIM-001]`. A run manages only its own Simulated Cluster in the machine's container runtime: nothing else there is pulled, started, stopped or removed `[REQ-SIM-001]`. Two runs at once on one machine are no exception, so neither touches the other's Simulated Cluster `[REQ-SIM-001]`.

<a id="req-sim-002"></a>
Verifying the lab takes a single invocation with no arguments. That invocation checks the lab's own rules first, then runs every Scenario that needs a Simulated Cluster, one after another `[REQ-SIM-002]`. Each Scenario's name and outcome appears as the run proceeds, and no outcome is ever carried over from an earlier run `[REQ-SIM-002]`. A Scenario needs a container runtime, so a run that reaches none stops and names what is missing `[REQ-SIM-002]`.

<a id="req-sim-003"></a>
One Scenario also runs on its own when the engineer names it, so a single behavior is re-run without the others `[REQ-SIM-003]`. A Scenario name that matches no Scenario stops the run before a Simulated Cluster is provisioned `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
A Scenario's outcome is trusted only when the lab itself is sound, and one command decides that. It checks the lab's own rules, runs the lab's own tests, and builds the lab `[REQ-SIM-004]`. The same command fails when any part of the lab could reach a cluster the lab did not provision itself `[REQ-SIM-004]`. The repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The lab also accounts for what it depends on and for what it leaves open. The one command confirms every dependency is the one the lab declared `[REQ-SIM-007]`. It fails when a dependency carries a known vulnerability `[REQ-SIM-007]`. Its check for cluster access identifies each part of the lab exactly `[REQ-SIM-007]`. A part whose name resembles a permitted one is not mistaken for it `[REQ-SIM-007]`. Every connection the lab opens to a Simulated Cluster is closed when its work ends, the lab's own tests included `[REQ-SIM-007]`.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when Resilient placement runs, then the lab reports a Topology of three Zones with three Nodes each, every Node Ready.
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

**ADR:** [ADR: Kubernetes version pin](adr/2026-10-03-kubernetes-version-pin.md) · [ADR: The lab provisions the Simulated Cluster it drives](adr/2026-10-03-lab-provisioned-simulated-cluster.md) · [ADR: Each Scenario provisions its own Simulated Cluster](adr/2026-10-04-cluster-per-scenario.md) · [ADR: A Scenario's Workload declares the spread of its own Pods](adr/2026-10-04-scenario-declared-pod-spread.md)  ·  **Design:** [system-design.md](system-design.md)

### Declaring a Workload's Pods

<a id="req-sim-009"></a>
A Scenario states the intent behind its Workload rather than the settings that carry it out. It declares one of the lab's predefined Pod specifications, and that specification holds every setting the Workload's Pods are deployed with `[REQ-SIM-009]`. A Workload is configured that one way and no other `[REQ-SIM-009]`. The lab predefines one specification per intent, so a reader sees what a Scenario demonstrates without reading settings `[REQ-SIM-009]`.

A predefined specification holds the settings that decide where a Workload's Pods land and when they leave a Node, and no other setting `[REQ-SIM-009]`. An engineer who opens a predefined specification reads which setting it sets and to which value `[REQ-SIM-009]`. Each setting carries the name Kubernetes gives it, so the definition reads as the configuration a manifest would state `[REQ-SIM-009]`. The settings the lab adds itself stand in that definition too, so one place holds the whole of what decides a Placement `[REQ-SIM-009]`.

Resilient placement spreads a Workload's Pods evenly over the Zones and over the Nodes, and both of the lab's Scenarios declare it `[REQ-SIM-009]`. Default placement declares nothing about where Pods may land, so the scheduler alone decides `[REQ-SIM-009]`. Where those Pods land is not predictable before the run, and a Scenario declaring it asserts only what holds for every Placement `[REQ-SIM-009]`. The lab offers it as the deliberately weak counterpart to Resilient placement `[REQ-SIM-009]`.

A Scenario also changes a predefined specification before its Workload is deployed. It names zero or more Customizations with the specification it declares, and each changes the settings it names `[REQ-SIM-009]`. A Customization leaves the specification it was applied to as it was, so no other Workload declaring that specification is affected `[REQ-SIM-009]`. The Customizations apply in the order written, and a later one wins where two name one setting `[REQ-SIM-009]`. A Customization names a setting the way a predefined definition names it, under the name Kubernetes gives it `[REQ-SIM-009]`. The lab's own vocabulary names the settings that decide where a Scenario's Pods land, and the waiting time before Pods leave an unresponsive Node stays the lab's own `[REQ-SIM-009]`.

A Scenario that needs a setting the lab's own vocabulary does not name reaches every Kubernetes Pod setting instead, as one more Customization of the specification it declares `[REQ-SIM-009]`. Full access is a Customization like any other: it yields a changed copy and applies in its turn among the others `[REQ-SIM-009]`. It names each setting as Kubernetes names it, so a Scenario using it reads partly in Kubernetes' own vocabulary `[REQ-SIM-009]`. What the lab itself depends on holds whatever full access leaves out `[REQ-SIM-009]`. A Workload's Pods are still reported as that Workload's, and the waiting time before they leave an unresponsive Node is still the lab's, whatever full access names `[REQ-SIM-009]`. No Scenario of the lab uses full access, which arrives with the Scenario needing a setting the vocabulary lacks `[REQ-SIM-009]`.

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
A Node has room for only so much, and a Pod asks for a share of it. A Topology states the Node capacity its Nodes have: the cpu and memory of one Node, and last the Pods it holds `[REQ-SIM-011]`. A Topology stating no capacity gets the lab's own default, so a Scenario about Placement alone names none `[REQ-SIM-011]`. A stated capacity may leave the Pod count out, which keeps the lab's own count `[REQ-SIM-011]`. Its Given line then names the cpu and memory alone `[REQ-SIM-011]`. Every run reports the whole capacity the Nodes it provisioned have, the Pod count included `[REQ-SIM-011]`. Every line reporting a capacity names the cpu, then the memory, then the Pod count `[REQ-SIM-011]`.

A Workload's Pods state what they ask a Node for. The Pod specification that Workload declares holds that Request, naming the cpu and the memory one Pod needs `[REQ-SIM-011]`. A Request is a setting like any other, under the name Kubernetes gives it `[REQ-SIM-011]`. A specification stating none leaves its Pods asking for nothing, which is how every Scenario of the lab places its Pods `[REQ-SIM-011]`.

The scheduler places a Pod by what that Pod requests, so a Pod no Node has room for stays Pending with the control plane's reason `[REQ-SIM-011]`. What a Failure leaves a Workload is the room on the Nodes still responding, and not those Nodes alone `[REQ-SIM-011]`.

No room after a Zone stops responding demonstrates that. Its Topology gives every Node room for one of the Workload's Pods, so its nine-Pod Workload lands one Pod per Node `[REQ-SIM-011]`. One Zone then stops responding, and the six Nodes still responding have room for six of the nine Pods `[REQ-SIM-011]`. The Scenario therefore waits for the Placement to be Settled rather than for every Pod to be Ready `[REQ-SIM-011]`. It then asserts no Pod in the Zone that stopped responding and three Pods left Pending `[REQ-SIM-011]`. Its Workload declares the same Required Capacity of six, which the six Pods still Running meet `[REQ-SIM-011]`. Losing a Zone here costs the spare Pods and not the capacity `[REQ-SIM-011]`.

**Done when:**

- `[REQ-SIM-011]` given a Topology stating its Nodes' cpu, memory and Pod count, when it is provisioned, then every Node has that capacity and the run reports it in that order.
- `[REQ-SIM-011]` given a Topology stating the cpu and memory alone, when it is provisioned, then its Nodes hold the lab's own Pod count and the run reports it.
- `[REQ-SIM-011]` given a Topology stating no Pod count, when its Given line is reported, then that line names the cpu and the memory alone.
- `[REQ-SIM-011]` given a Topology stating no Node capacity, when it is provisioned, then its Nodes have the lab's own default Pod count, cpu and memory.
- `[REQ-SIM-011]` given a Workload whose specification states a Request, when the settings its Pods were deployed with are read back, then that Request stands under the name Kubernetes gives it.
- `[REQ-SIM-011]` given a Workload whose specification states no Request, when its Pods are placed, then they ask for no cpu and no memory and the spread alone decides the Placement.
- `[REQ-SIM-011]` given a Workload requesting more cpu than its Topology has room for, when the Placement settles, then the Pods that fit are Running and the rest stay Pending.
- `[REQ-SIM-011]` given a Topology whose Nodes have room for one Pod each and a nine-Pod Workload, when the Placement settles, then nine Pods are Running, one on every Node.
- `[REQ-SIM-011]` given that Placement, when one Zone stops responding, then the Placement settles at six Pods Running and three Pending, and the Scenario asserts both.
- `[REQ-SIM-011]` given that Scenario, when its judgement of Required Capacity is reported, then it counts six Pods at the moment of the Failure and six once Settled, meeting the minimum.

**Edge cases:**

1. A Topology states a Node capacity of no Pod, of no cpu, or of no memory. It is refused where it is stated, so a Node always has room for a Pod that asks for something. An omitted Pod count is no such statement, and keeps the lab's own count.
2. A Workload's Request is larger than any one Node of its Topology has room for. Every Pod of that Workload stays Pending.
3. A Request leaves a Node room for fewer Pods than its Node capacity names. The tighter of the two bounds decides, and the Pod count never comes into it.
4. A Customization written after a Request states another one. The later Request stands, as a later setting of any kind does.

**Design:** [system-design.md](system-design.md)

### Starting a Workload's Pods

<a id="req-sim-012"></a>
A real Pod takes time to become Ready once it is placed, and a Workload replacing Pods after a Failure stays short of them for that time. Every placed Pod of a Simulated Cluster therefore takes a Startup time before it is Ready `[REQ-SIM-012]`. A Pod inside its Startup time stands on the Node it was placed on, counted as Pending and not as Running `[REQ-SIM-012]`. A Placement holding such a Pod is not yet Settled `[REQ-SIM-012]`. A Pod replacing one lost in a Failure takes a Startup time of its own, counted from its own placement `[REQ-SIM-012]`.

Every Pod takes the lab's own Startup time, one time the same for every Pod, unless its Pod specification states another `[REQ-SIM-012]`. Every run reports the lab's own Startup time `[REQ-SIM-012]`. No predefined specification states one, so every Scenario of the lab takes the lab's own and keeps the outcome it asserts `[REQ-SIM-012]`.

A Scenario states another Startup time as one more Customization of the specification its Workload declares, written beside its Request `[REQ-SIM-012]`. It states either one time, which every Pod of that Workload takes, or a shortest and a longest time `[REQ-SIM-012]`. Given a shortest and a longest, each Pod takes its own time between the two, both included `[REQ-SIM-012]`. A Startup time is stated in whole seconds, as a Wait's time is `[REQ-SIM-012]`. A Startup time is the one thing a Customization changes that is no Kubernetes setting, and otherwise behaves as any Customization does `[REQ-SIM-012]`. It leaves the specification it was applied to as it was, and a later Startup time wins over an earlier one `[REQ-SIM-012]`.

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
A Scenario also injects a Failure and judges the Placement on both sides of it. Node stops responding starts from Resilient placement's Topology and Workload, then stops one Node responding, as a machine does when it hangs or loses its network `[REQ-SIM-008]`. That Node stays in the cluster, and the control plane moves its Pods away after a waiting time `[REQ-SIM-008]`. The lab sets that waiting time to a few seconds rather than the Kubernetes default, and every run reports it `[REQ-SIM-008]`. A Scenario therefore states no waiting time for Pods to leave such a Node `[REQ-SIM-008]`.

A Scenario says how many Nodes fail, or how many Zones, and the lab picks them the same way on every run and reports which `[REQ-SIM-008]`. The Step that injects the Failure carries no Node and no Zone name, so what failed is readable from the Scenario's own Steps `[REQ-SIM-008]`.

Node stops responding judges the Placement twice. Before the Failure it waits for every Pod to be Ready and asserts three Pods in every Zone `[REQ-SIM-008]`. After the Failure it waits again, then asserts three Pods in every Zone and no Pod on the Node that stopped responding `[REQ-SIM-008]`. One surviving Node then holds two Pods, so Node stops responding asserts no Placement of one Pod per Node `[REQ-SIM-008]`.

Zone stops responding starts from the same Topology and Workload, then stops every Node of one Zone responding at once `[REQ-SIM-008]`. A Zone holds the Nodes that are lost together, so one Step takes all of them `[REQ-SIM-008]`. Before the Failure, Zone stops responding waits for every Pod to be Ready and asserts three Pods in every Zone `[REQ-SIM-008]`. After the Failure it waits again, then asserts no Pod in the Zone that stopped responding `[REQ-SIM-008]`. The capacity the Workload serves is therefore restored by the Zones still standing `[REQ-SIM-008]`. Two Zones then carry what three carried, so Zone stops responding asserts no count of Pods per Zone and none per Node `[REQ-SIM-008]`. Losing a Zone costs the spread, and the lab accepts that cost while the Pod count holds `[REQ-SIM-008]`.

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
A Workload states the capacity it has to serve, and the lab judges whether a Failure leaves that capacity standing. The Workload declares its Required Capacity beside the Pods it runs, as the fewest Pods that must stay Running `[REQ-SIM-010]`. A Workload therefore runs spare Pods, so that a lost Zone still leaves the minimum behind `[REQ-SIM-010]`. A minimum above the Pods the Workload runs, or of no Pod at all, is refused where it is declared `[REQ-SIM-010]`.

A Failure tolerance is judged at two moments, which one Step after the Failure judges together `[REQ-SIM-010]`. The first is the moment the Failure was injected, where a Pod on a Node that stopped responding no longer counts as Running `[REQ-SIM-010]`. The second is the moment the Placement is Settled again, where the Pods the control plane placed elsewhere count `[REQ-SIM-010]`. The first moment says what the Failure cost, the second what the cluster restored `[REQ-SIM-010]`.

Every judgement states the Pods it counted at each moment and the minimum it judged them against, met or not `[REQ-SIM-010]`. An engineer therefore checks the lab's count by hand against the Placement the same run reports `[REQ-SIM-010]`. A minimum missed at either moment is reported as a Finding naming that moment `[REQ-SIM-010]`.

Judging Failure tolerance is one way to judge a Failure and counting Pods by hand is the other, and a Scenario is held to neither `[REQ-SIM-010]`. An engineer writing one takes either way or both `[REQ-SIM-010]`. Every Failure Scenario declares six Pods of nine and judges that minimum, beside the counts it already asserts `[REQ-SIM-010]`. One lost Node leaves eight Pods at the moment of the Failure and one lost Zone leaves six `[REQ-SIM-010]`. Nine are Running again once the Placement is Settled, except where the Nodes still responding have no room, which leaves six `[REQ-SIM-010]`.

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
A reported Placement says where the Pods landed; a Timeline says how they got there. The lab records what changes in a Scenario's Simulated Cluster while the Scenario runs, in the order it happened `[REQ-SIM-005]`. A Timeline holds every change to the Nodes, the Pods and the Workload's own objects `[REQ-SIM-005]`. It also holds the control plane's reason for placing each Pod or for leaving it Pending `[REQ-SIM-005]`.

When a Scenario ends, the run prints a Timeline summary, also when the Scenario fails `[REQ-SIM-005]`. The summary opens with how many entries the Timeline holds and where its file is `[REQ-SIM-005]`. It gives the lab's own setup of the Topology one line, and each Workload a line counting its Pods now Running `[REQ-SIM-005]`. Every Pod of a Workload then gets a line giving its path from Pending to Running and the Node it landed on `[REQ-SIM-005]`. A Pod still Pending carries the control plane's reason for it instead, and the summary says plainly when no Pod is Pending `[REQ-SIM-005]`. A collapsed line changes neither the recording nor the Timeline's file `[REQ-SIM-005]`. An engineer who asks for the whole Timeline gets every entry printed in the order it happened `[REQ-SIM-005]`.

Every Scenario of a run also leaves its Timeline in a file of its own, without an engineer asking for it `[REQ-SIM-005]`. The files collect in one place under the working directory, and the lab removes none of them `[REQ-SIM-005]`. A Timeline therefore outlives both the run and the Simulated Cluster it describes `[REQ-SIM-005]`. Two runs at once leave two sets of files, neither replacing the other's `[REQ-SIM-005]`.

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
An engineer writes a Scenario as a short sequence of Steps, one per line `[REQ-SIM-006]`. A Given declares the Topology and the Workload, and a When changes the cluster `[REQ-SIM-006]`. A Wait waits for a state of the cluster, and a Then asserts the Placement outcome `[REQ-SIM-006]`. A Scenario handles no error of its own `[REQ-SIM-006]`. A Step that cannot be carried out ends the Scenario and names that Step `[REQ-SIM-006]`. A Then the Placement does not meet is reported as a Finding, and the Steps after it still run `[REQ-SIM-006]`.

A Scenario writes every Wait it needs, so no Step waits silently `[REQ-SIM-006]`. An engineer writes a Wait with the Given before it, or as a Step of its own, and both forms are the same Step kind `[REQ-SIM-006]`. A Wait written with a Given therefore reports its own line after that Given's `[REQ-SIM-006]`. Every Scenario of the lab writes each Wait as a Step of its own, so each of its lines is one Step `[REQ-SIM-006]`. Every Scenario carries a Wait, whether or not it injects a Failure `[REQ-SIM-006]`. A Given declares, and the Wait after it applies what the Givens before it declared, so a Given Waits only where the Scenario asks it to `[REQ-SIM-006]`. A Wait on a Topology waits until every Node it declares is Ready, and a Wait on a Workload until every Pod it declares is Ready `[REQ-SIM-006]`. Beneath a Wait the lab observes that the Placement has stopped changing, so the Thens after it judge a Settled Placement `[REQ-SIM-006]`. A Wait's line names the time it took, which after a Failure is how long the cluster took to place the Pods again, read from the Scenario's own lines `[REQ-SIM-006]`.

Every Then judges the Placement a Wait reached, and so does every Failure `[REQ-SIM-006]`. A Then with no Wait between it and the Given or the Failure before it ends the Scenario at that Then `[REQ-SIM-006]`. A Failure injected before the Scenario has Waited once ends it at that When `[REQ-SIM-006]`. A Wait the lab's own waiting time outlasts ends the Scenario there, naming what was not Ready `[REQ-SIM-006]`.

A Wait may also state the time it must be reached in, in whole seconds, beside what it waits for `[REQ-SIM-006]`. A Wait stating none is held to the lab's own waiting time, so every Wait has a time `[REQ-SIM-006]`. A Wait reached inside the time it states names that time beside the time it took `[REQ-SIM-006]`. A Wait not reached in that time ends the Scenario at that Step, naming the time it states and not the time it took `[REQ-SIM-006]`. A time above the lab's own waiting time is refused where it is stated, naming that waiting time `[REQ-SIM-006]`. Every run reports the lab's own waiting time at its start, so an engineer reads the bound a stated time is held to `[REQ-SIM-006]`.

A Wait also waits for the Placement alone to be Settled, whether or not every Pod is Ready `[REQ-SIM-006]`. A Scenario expecting Pods to stay Pending needs it, since waiting for every Pod to be Ready would end such a Scenario at that Wait `[REQ-SIM-006]`. The Thens after it judge a Placement that has stopped changing with Pods still Pending `[REQ-SIM-006]`. It states the time it must be reached in as any other Wait does `[REQ-SIM-006]`.

Every Failure Scenario states a time on the Wait after its Failure, which is the time the cluster is given to restore the Placement `[REQ-SIM-006]`. The Waits after its Givens state none, and the simplest Scenario states none at all `[REQ-SIM-006]`.

Every name a Scenario uses is one the lab offers Scenarios, so one vocabulary carries every Step `[REQ-SIM-006]`. A Scenario reaching every Kubernetes Pod setting names Kubernetes' own settings there, which is the one exception `[REQ-SIM-006]`. An engineer opening the repository finds every Scenario in one place `[REQ-SIM-006]`. The machinery that runs them is kept apart from them `[REQ-SIM-006]`. A README beside the Scenarios names each one and the outcome it asserts `[REQ-SIM-006]`.

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
- Whether the lab also gives a Pod a time to stop once it is removed, as it gives one to start.
- Which further specifications the lab predefines, and which further settings one may hold and a Customization may name. Zone spread only and Soft spread each arrive with the Scenario that uses them, and so does affinity between a Workload's Pods. Also open: what a Scenario asserts when it demonstrates a deliberately weak specification.
