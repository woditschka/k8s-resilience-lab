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
| G-4 | Writing and reading a Scenario takes no knowledge of the lab's machinery. | A Scenario is a sequence of Given, When and Then Steps and nothing else, and an engineer finds every Scenario in one place. |

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
The lab brings up a Simulated Cluster of its own for every Scenario and provisions that Scenario's Topology into it before any Workload is deployed `[REQ-SIM-001]`. Each Scenario names the Topology it provisions, the Workload it deploys with the rules that constrain where its Pods may land, and the Placement it asserts `[REQ-SIM-001]`. The lab reports that Placement per Zone and per Node, counting the Pods that are Running and the Pods that stay Pending `[REQ-SIM-001]`. Zone and Node names follow from the Topology alone, so every run of the same Topology reports the same names `[REQ-SIM-001]`. A Scenario's Simulated Cluster is removed when the Scenario ends, also when it fails `[REQ-SIM-001]`. No Node or Pod of an earlier Scenario can be mistaken for a current one `[REQ-SIM-001]`.

The lab's main Scenario is Even spread. It provisions three Zones with three Nodes each, which leaves two Nodes in a Zone once one of its Nodes is lost `[REQ-SIM-001]`. Its nine-Pod Workload declares even spread over Zones and over Nodes, and the Scenario asserts three Pods in every Zone and one Pod on every Node `[REQ-SIM-001]`.

Every run reports the Kubernetes version its Simulated Cluster mirrors `[REQ-SIM-001]`. An engineer names a version for a single run, and a run that names none mirrors the lab's pinned default `[REQ-SIM-001]`. A run manages only its own Simulated Cluster in the machine's container runtime: nothing else there is pulled, started, stopped or removed `[REQ-SIM-001]`. Two runs at once on one machine are no exception, so neither touches the other's Simulated Cluster `[REQ-SIM-001]`.

<a id="req-sim-002"></a>
Verifying the lab takes a single invocation with no arguments. That invocation checks the lab's own rules first, then runs every Scenario that needs a Simulated Cluster, one after another `[REQ-SIM-002]`. Each Scenario's name and outcome appears as the run proceeds, and no outcome is ever carried over from an earlier run `[REQ-SIM-002]`. A Scenario needs a container runtime, so a run that reaches none stops and names what is missing `[REQ-SIM-002]`.

<a id="req-sim-003"></a>
One Scenario also runs on its own, named by the engineer, so a single behavior is re-run without the others `[REQ-SIM-003]`. A name that matches no Scenario stops the run before a Simulated Cluster is provisioned `[REQ-SIM-003]`.

<a id="req-sim-004"></a>
A Scenario's outcome is trusted only when the lab itself is sound, and one command decides that. It checks the lab's own rules, runs the lab's own tests, and builds the lab `[REQ-SIM-004]`. The same command fails when any part of the lab could reach a cluster the lab did not provision itself `[REQ-SIM-004]`. The repository holds exactly one implementation of the lab `[REQ-SIM-004]`.

<a id="req-sim-007"></a>
The lab also accounts for what it depends on and for what it leaves open. The one command confirms every dependency is the one the lab declared `[REQ-SIM-007]`. It fails when a dependency carries a known vulnerability `[REQ-SIM-007]`. Its check for cluster access identifies each part of the lab exactly `[REQ-SIM-007]`. A part whose name resembles a permitted one is not mistaken for it `[REQ-SIM-007]`. Every connection the lab opens to a Simulated Cluster is closed when its work ends, the lab's own tests included `[REQ-SIM-007]`.

**Done when:**

- `[REQ-SIM-001]` given no Simulated Cluster is up, when Even spread runs, then the lab reports a Topology of three Zones with three Nodes each, every Node Ready.
- `[REQ-SIM-001]` given Even spread's Topology, when its nine-Pod Workload is deployed, then the reported Placement is Settled at nine Running Pods, three in every Zone and one on every Node.
- `[REQ-SIM-001]` given an earlier run created Nodes and Pods, when a run starts, then the reported Placement names only the Nodes and Pods of the current run.
- `[REQ-SIM-001]` given the same Topology, when two runs are compared, then both report the same Zone and Node names.
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

### Writing and finding a Scenario

<a id="req-sim-006"></a>
An engineer writes a Scenario as a short sequence of Steps, one per line `[REQ-SIM-006]`. A Given sets up the Topology and the Workload, a When injects a Failure, a Then asserts the Placement outcome `[REQ-SIM-006]`. A Scenario that injects no Failure has no When Step `[REQ-SIM-006]`. A Scenario handles no error of its own `[REQ-SIM-006]`. A Step that cannot be carried out ends the Scenario and names that Step `[REQ-SIM-006]`. A Then the Placement does not meet is reported as a Finding, and the Steps after it still run `[REQ-SIM-006]`.

An engineer opening the repository finds every Scenario in one place `[REQ-SIM-006]`. The machinery that runs them is kept apart from them `[REQ-SIM-006]`. A README beside the Scenarios names each one and the outcome it asserts `[REQ-SIM-006]`.

**Done when:**

- `[REQ-SIM-006]` given any Scenario of the lab, when its lines are read, then each line is one Given, When or Then Step and no line handles an error.
- `[REQ-SIM-006]` given a Scenario that injects no Failure, when it runs, then it carries no When Step and still reports its Placement.
- `[REQ-SIM-006]` given a Step that cannot be carried out, when the Scenario runs, then it ends, names that Step, and runs no later Step.
- `[REQ-SIM-006]` given a Then the Placement does not meet, when the Scenario runs, then the run reports the Finding and the later Steps still run.
- `[REQ-SIM-006]` given Even spread written as Steps, when it runs, then it reports the Placement REQ-SIM-001 asserts.
- `[REQ-SIM-006]` given an engineer opening the repository, when they look for the Scenarios, then one place holds every Scenario and nothing of the machinery.
- `[REQ-SIM-006]` given that place, when an engineer reads its README, then the README names every Scenario and the outcome each asserts.

**ADR:** [ADR: Scenarios are Go functions over immutable descriptions](adr/2026-10-03-scenarios-as-go-functions.md)  ·  **Design:** [system-design.md](system-design.md)

## Superseded

<!-- Retired requirements: each ID maps to its successor (or to the reason it was withdrawn) so existing links still resolve. Keep this a list, so every retired ID stays in a list item. -->

- (none yet)

## Open Questions

<!-- Unresolved product questions. Each resolves into a requirement, a non-goal, or an ADR. -->

- Does a Failure stay instantaneous, or does the lab also script a Failure over time — a Pod that dies after a delay, a Node that degrades before it is lost? The timed form is closer to a real outage and costs a second injection surface.
- What does the live view show, and what can an engineer do from it? Open options: a read-only view of the current Placement, or the same view with Failure injection and Scenario controls in it (G-2).
- Does the lab promise anything about a team's own production manifests, or only about the manifests it declares itself? A run mirrors a chosen Kubernetes version, so the stronger promise needs the Topology and the admission behavior to mirror a named environment as well.
- Which Failure the Scenario after Even spread injects — Pod, Node, or Zone — and the Required Capacity it asserts (G-3).
