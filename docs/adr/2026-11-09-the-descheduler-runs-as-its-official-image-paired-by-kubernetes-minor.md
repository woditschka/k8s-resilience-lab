# The Descheduler runs as its official image, paired by Kubernetes minor, under a policy the Scenarios define

**Status:** Accepted

## Context

The control plane places Pods and never moves a running one, so a Placement stays where a Failure left it. The Kubernetes descheduler closes that gap by evicting Pods its policy names. The owner wants it in the lab through the same Step surface, at the Kubernetes version the run mirrors, under policies the Scenarios define and the lab never extends. Its reader is "Someone reading the Scenario source, who wants the intent in a glance and the Kubernetes meaning one jump away", the definition read like the descheduler's own policy file.

The descheduler publishes a multi-arch image per release at `registry.k8s.io/descheduler/descheduler`, and its releases track Kubernetes minors. It reads its policy through the Kubernetes universal decoder, which accepts JSON. A run has to know whether any Scenario declares a policy before it provisions, because a version with no paired descheduler must stop the run there.

## Options Considered

How the descheduler runs:

1. **Import it as a Go library** — couples the module graph to its `k8s.io` versions and drifts from the shipped behavior.
2. **`go install` its binary into a tools folder** — a second toolchain path, and a process on the host rather than beside the cluster.
3. **Its official image, a container beside the cluster's** — started through the engine the lab already drives.

How the descheduler version is chosen:

4. **A rule, the Kubernetes minor at patch zero** — breaks silently when a release skips or patches.
5. **A compiled-in table, one row per Kubernetes minor** — a new minor is a reviewed change.

How a Scenario writes a policy:

6. **Plugin names and arguments as strings in a map** — a misspelt argument surfaces only in the descheduler's log.
7. **One keyed literal per plugin, the lab deriving each extension point and configuration entry** — short, but the file the descheduler reads stays hidden, and the lab states what the definition does not.
8. **The policy file itself as a keyed literal, `descheduler.Policy`**, under its keys and Kubernetes' plugin and argument type names, in a `descheduler` package beside `pod` and `node`. Longer, and read line for line against the descheduler's documentation.

How an extension point's plugins are written:

9. **A helper, `descheduler.Enabled(…)`** — a lab name between the reader and the file's `enabled` key, a function existing only to label a position.
10. **The literal `descheduler.PluginSet{Enabled: …}`**, as the file writes it.

How a configuration entry repeated across policies is written:

11. **Repeated in each policy literal.**
12. **A Named group** — a function the Scenarios define, returning one `PluginConfig`, written inside the policy literal.

How a run learns a Scenario declares a policy before any cluster:

13. **Refuse at the Given** — the cluster is already provisioned.
14. **Stop every run at an unpaired version** — stops runs declaring no policy.
15. **A declaring pass** — each Scenario's function runs once over a Scenario holding no cluster, carrying out its Givens alone.

Where the container runner lives:

16. **A package of its own** — first needs the engine calls extracted from `internal/cluster`.
17. **`internal/cluster`, beside the engine calls.**

How a When learns its pass ended:

18. **The container's log stream, read for an end-of-pass line** — ties the lab to log wording a release may change.
19. **The engine's wait on the container's exit** — a zero interval ends the process after one pass. A pass erroring without exiting is held to the lab's waiting time.

How the kubeconfig and policy reach the container:

20. **A bind mount of host files** — a host path per pass, which a killed run leaves behind.
21. **An archive the engine unpacks into the created container before start.**

The descheduler's HTTPS health server on port 10258:

22. **Off, through a zero secure port** — unverified for the pinned release, and a refusal would stop every pass.
23. **On, unpublished** — no KWOK component uses the port, and the shared namespace publishes only the API port.

How the lab tells a Pod the descheduler evicted:

24. **The Event the descheduler reports** — a pass at a zero interval exits before its event sender flushes, so no such Event reaches the watch.
25. **The pass's log lines** — ties the lab to log wording a release may change, as option 18 does.
26. **Every Pod first marked for deletion between the pass's When note and a note at its exit** — misreads a Pod the control plane evicts from a stopped Node meanwhile, and races the watch's delivery against the engine's wait.
27. **The disruption-target condition the Eviction API sets on the Pod**, watched before the Pod is marked for deletion. The control plane deletes rather than evicts, and so does a Failure.

How the descheduler runs on an interval:

28. **One container at the descheduler's own interval, its passes learned from its log** — ties the lab to log wording, as option 18 does.
29. **One such container, its passes learned from its metrics** — served over its health server's HTTPS port, unpublished, from a scratch image with no client inside.
30. **One such container, its passes inferred from the interval's clock** — a pass's start is not observable, so a Wait could end on a pass that never ran.
31. **The lab starting one pass container on each interval**, each learned by its exit as a When's pass. The descheduler's own loop goes unused, and each pass reads the cluster afresh.

How a pass on an interval is judged to evict nothing:

32. **No disruption-target entry between the pass's start and exit** — races the watch's delivery against the engine's wait, as option 26 does.
33. **One listing of the Pods before the pass starts and one after it exits**, through the API server. Every Pod listed before stays listed, unmarked for deletion and no disruption target.

Where the interval is held:

34. **A `scenario.Descheduler` aggregate of the policy and the interval** — no invariant spans the two, so the root guards nothing.
35. **A `scenario.DeschedulerInterval` value beside the policy**, refusing an interval of zero or less as `scenario.StatedWait` refuses a time.

How a pass on an interval keeps a declared Floor, after the first pass past a Zone responding again evicted four of nine Pods on four Nodes against a Floor of six:

36. **The per-Node limit alone** — the spread plugin acts on the Zone and the hostname constraints together, so one pass evicts one Pod on each of several Nodes.
37. **A per-pass limit of one, the file's `maxNoOfPodsToEvictTotal`** — passes run one at a time, so a Floor below the replicas holds while each replacement turns Ready within an interval. Convergence takes a pass per Pod moved.
38. **A PodDisruptionBudget at the Floor**, honoured by the Eviction API — guards every voluntary eviction whatever its timing, but is a Given of its own the PRD leaves an open question.
39. **A looser Failure tolerance or Then on the Scenario** — hides the fall the Then exists to report.

## Decision

Options 3, 5, 8, 10, 12, 15, 17, 19, 21, 23, 27, 31, 33, 35 and 37.

- `descheduler.Policy` mirrors `descheduler/v1alpha2`: its per-Node and per-pass eviction limits, profiles, each profile's configuration entries with typed `Args`, and a `PluginSet` per extension point. Plugin and argument types carry Kubernetes' names.
- A field at its zero value stays out. The lab adds no entry, argument or default and derives no extension point, so the encoding transcribes the definition.
- A policy is named by its profile names in order. Its report prints the definition under the file's keys.
- The Scenarios may define a Named group, reported as the entry it returns. They define none until a configuration entry repeats.
- The definition becomes `scenario.DeschedulerPolicy`, immutable and free of I/O, as a Pod specification becomes `scenario.PodSpecification`.
- `internal/cluster` encodes the policy as `descheduler/v1alpha2` JSON with `encoding/json`, over its own types; no descheduler module is imported.
- `internal/cluster` holds the table pairing a Kubernetes minor with a descheduler tag. An unpaired minor is an error naming the version.
- The runner resolves the pairing once and runs the declaring pass before any cluster. A declared policy and an unpaired version stop the run.
- The descheduler container is started by `internal/cluster`, sharing the Simulated Cluster's network namespace, pinned by tag.
- A When runs one pass as such a container, at a zero interval, its command constants alone. Its kubeconfig names the loopback API port over plain HTTP with no credential.
- The When waits for the exit within the lab's waiting time, and removes the container whatever the outcome. Closing the cluster removes a pass container still held first.
- A Pod the descheduler evicts is read from the disruption-target condition the Eviction API sets on it, so no log is parsed for an eviction. The lab evicts no Pod itself.
- A further call on the Given states an interval in seconds. From the Wait applying the Given until its cluster is removed, the lab starts a pass on each interval, never two at once.
- A pass on an interval evicts nothing when the two listings agree. A Wait of its own ends at the first such pass begun after it, and Settled keeps its meaning.
- A pass on an interval failing stops the interval and ends the Scenario at its next Step or at its end.
- A policy the Scenarios run on an interval beside a declared Floor states a per-pass limit of one.

## Consequences

A definition reads as the policy file does, and an argument the lab does not offer fails to compile. The definition states every entry and extension point, so it is longer than a list of plugins. The pinned release refuses an enabled plugin with no configuration entry; the descheduler names it and the pass's When ends the Scenario. A limit of zero reads as left out. A plugin, limit or extension point no Scenario has used yet is a reviewed addition to the `descheduler` package.

A Scenario's function runs twice per run. That holds because a Given applies nothing and every other Step carries nothing out without a cluster.

The engine surface admits a second image and a container joining another's namespace. Its repository is a constant and its tag a table row, so no input reaches either.

A pass that errors without exiting costs its Scenario the lab's waiting time. On an interval, every pass creates, starts and removes a container, and its command stays constants alone. A Pod a Failure or the control plane removes during a pass reads as an eviction, so the Wait waits one interval more. A later Step evicting through the Eviction API would read as the descheduler's, so a Failure keeps deleting.

A per-pass limit of one keeps a Floor only while each replacement turns Ready within an interval, and the Floor's Then remains the check. A PodDisruptionBudget would make the guard independent of timing; it waits on the PRD's open question.

A new Kubernetes minor needs a table row before a Scenario declaring a policy runs on it. Pinning by tag carries the same standing gap as the cluster image, per [ADR: Kubernetes version pin](2026-10-03-kubernetes-version-pin.md).

## Implementation

**Requirements:** REQ-SIM-031

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
