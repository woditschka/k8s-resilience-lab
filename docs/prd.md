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
| G-3 | A Workload's survival of a Failure is judged against a stated threshold. | Each Scenario names the Required Capacity it expects and judges it immediately after the Failure and again after Placement settles. |
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
- `[REQ-SIM-009]` given a Workload customized with full access, when the settings its Pods carry are read back, then every setting full access named stands, apart from the lab's waiting time.
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

### Surviving a Node Failure

<a id="req-sim-008"></a>
A Scenario also injects a Failure and judges the Placement on both sides of it. Node loss starts from the Resilient placement Scenario's Topology and Workload, then stops one Node responding, as a machine does when it hangs or loses its network `[REQ-SIM-008]`. That Node stays in the cluster, and the control plane moves its Pods away after a waiting time `[REQ-SIM-008]`. The lab sets that waiting time to a few seconds rather than the Kubernetes default, and every run reports it `[REQ-SIM-008]`. A Scenario therefore states no waiting time of its own `[REQ-SIM-008]`.

Node loss says how many Nodes fail, and the lab picks them the same way on every run and reports which `[REQ-SIM-008]`. The Step that injects the Failure carries no Node name, so the failing Node is readable from the Scenario's own Steps `[REQ-SIM-008]`.

Node loss judges the Placement twice. Before the Failure it asserts nine Running Pods and three in every Zone `[REQ-SIM-008]`. Once the Placement settles again, it asserts those two outcomes and no Pod on the Node that stopped responding `[REQ-SIM-008]`. One surviving Node then holds two Pods, so Node loss asserts no Placement of one Pod per Node `[REQ-SIM-008]`.

**Done when:**

- `[REQ-SIM-008]` given Resilient placement's Topology and Workload are Settled, when the Placement is judged before any Failure, then nine Pods are Running and three sit in every Zone.
- `[REQ-SIM-008]` given Resilient placement's Topology and Workload are Settled, when one Node stops responding, then that Node's Pod is moved away within a few seconds.
- `[REQ-SIM-008]` given a Node has stopped responding, when the Placement settles again, then nine Pods are Running, three in every Zone, and none on that Node.
- `[REQ-SIM-008]` given Node loss says one Node fails, when it runs twice, then the same Node fails in both runs and the run reports which.
- `[REQ-SIM-008]` given any run, when it starts, then it reports that Pods leave an unresponsive Node sooner than in a default cluster.
- `[REQ-SIM-008]` given Node loss, when its lines are read, then one When Step injects the Failure, carries no Node name and states no waiting time.

**Edge cases:**

1. A Pod of the Node that stopped responding cannot be placed anywhere else. It stays Pending with the control plane's reason.
2. A When asks for more failing Nodes than the Topology holds. The Scenario ends at that Step and names it.
3. The Placement does not settle again within the lab's wait. The Scenario reports what it observed, rather than waiting longer.

**ADR:** [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](adr/2026-10-05-node-failure-withholds-the-nodes-status.md)  ·  **Design:** [system-design.md](system-design.md)

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
An engineer writes a Scenario as a short sequence of Steps, one per line `[REQ-SIM-006]`. A Given sets up the Topology and the Workload, a When injects a Failure, a Then asserts the Placement outcome `[REQ-SIM-006]`. A Scenario that injects no Failure has no When Step `[REQ-SIM-006]`. A Scenario handles no error of its own `[REQ-SIM-006]`. A Step that cannot be carried out ends the Scenario and names that Step `[REQ-SIM-006]`. A Then the Placement does not meet is reported as a Finding, and the Steps after it still run `[REQ-SIM-006]`.

Every name a Scenario uses is one the lab offers Scenarios, so one vocabulary carries every Step `[REQ-SIM-006]`. A Scenario reaching every Kubernetes Pod setting names Kubernetes' own settings there, which is the one exception `[REQ-SIM-006]`. An engineer opening the repository finds every Scenario in one place `[REQ-SIM-006]`. The machinery that runs them is kept apart from them `[REQ-SIM-006]`. A README beside the Scenarios names each one and the outcome it asserts `[REQ-SIM-006]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario of the lab, when its lines are read, then each line is one Given, When or Then Step and no line handles an error.
- `[REQ-SIM-006]` given any Scenario of the lab, when the names in its lines are read, then each is a name the lab offers, apart from the settings full access names.
- `[REQ-SIM-006]` given a Scenario that injects no Failure, when it runs, then it carries no When Step and still reports its Placement.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when the Scenario runs, then it ends, names that Step, and runs no later Step.
- `[REQ-SIM-006]` given a Then the Placement does not meet, when the Scenario runs, then the run reports the Finding and the later Steps still run.
- `[REQ-SIM-006]` given Resilient placement written as Steps, when it runs, then it reports the Placement REQ-SIM-001 asserts.
- `[REQ-SIM-006]` given an engineer opening the repository, when they look for the Scenarios, then one place holds every Scenario and nothing of the machinery.
- `[REQ-SIM-006]` given that place, when an engineer reads its README, then the README names every Scenario and the outcome each asserts.

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
- The Required Capacity Node loss states, and what counts as a Running Pod at the judging moment right after the Failure, while the unresponsive Node still holds one (G-3). Also open: how a missed minimum is reported, and what a minimum above the Workload's Pod count does. The two judging moments G-3 names are settled; no Scenario states a Required Capacity yet.
- Which Scenario first points at a Node by its position in the Topology its Given declared, rather than saying how many Nodes fail. Both forms are settled vocabulary; only the count form exists today, and a position the Topology does not hold has no stated outcome.
- What limits how many Zones a Topology may name. The lab refuses a count above a limit of its own, and a numbered Zone name puts no bound on the count.
- Which Failure the Scenario after Node loss injects — a Pod, a whole Zone, or a Node removed from the cluster rather than left unresponsive.
- Whether a Scenario of the lab demonstrates a Customization or full access, and which intent it would show. No Scenario names either today, and each arrives with the Scenario that needs it.
- Whether a Scenario changes a Workload's specification after that Workload is deployed. Today a Customization applies before deployment only, and a later change is a rollout the control plane carries out under its update strategy.
- Whether the lab says anything when full access leaves out a setting the lab depends on. Today that setting holds and the run stays silent about it, so a Scenario's own line is where an engineer reads what it asked for.
- Whether a run reports the specification each Workload was deployed with, and its settings after Customization.
- Which further specifications the lab predefines, and which further settings one may hold and a Customization may name. Zone spread only and Soft spread each arrive with the Scenario that uses them, and so does affinity between a Workload's Pods. Also open: what a Scenario asserts when it demonstrates a deliberately weak specification.
