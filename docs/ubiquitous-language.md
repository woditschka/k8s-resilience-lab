<!-- harness: 2026-09-30 -->
# Ubiquitous Language

<!-- The canonical domain vocabulary for this project — the "ubiquitous language" in the Domain-Driven Design sense (Eric Evans, Chapter 2). The same terms are used by stakeholders, the PRD, the system design, and the source code.

  This file is durable memory. Agents and developers across sessions read it to keep the same vocabulary; drift here ripples into variable, function, and file names across the codebase.

  ENTRY FORMAT

    **TermName**: One-sentence definition. Define what it IS, not what it does. Relationships: (optional) one line naming the related concepts and the cardinality where obvious. Avoid: (optional) other words used in the wild for the same concept that this project rejects. Listing them heads off drift.

  Example:

    **Order**: A customer-placed request for one or more line items, accepted for fulfillment but not yet shipped. Relationships: An Order contains one or more LineItems and references one Customer. Avoid: Purchase, Transaction (those are billing-context terms).

  WHEN TO ADD A TERM

  - The moment a term resolves during a requirements interview or a design discussion. Do not batch. Do not wait for a second use.
  - When recurring domain terms in existing docs or source code aren't yet captured (adoption case — the system-design-expert may write here during a foundational triage to seed the initial vocabulary).

  WHAT NOT TO PUT HERE

  - Harness methodology vocabulary (slice, loop, triage verdict, etc.) — that is the team's method, not this project's domain. This file holds project-domain terms only.
  - Implementation details. Definitions describe domain concepts, not code shape.

  CONSUMERS

  - The product-requirements-expert resolves and writes PRD terms.
  - The system-design-expert resolves design terms; it writes here only during the foundational triage path on adoption.
  - The doc-reviewer lints cross-document term consistency.
  - The feature-implementer names new domain-facing code from the entries; the code-quality-reviewer checks those names against them.

  CADENCE

  Slow. The ubiquitous language changes less often than the PRD or system design. Treat updates here as load-bearing — they ripple into variable, function, and file names across the codebase. -->

## Domain Terms

**Simulated Cluster**: A Kubernetes cluster whose control plane is genuine and whose Nodes and Pods are fake, so scheduling decisions are real and no container ever runs. Relationships: a Simulated Cluster holds one Topology and zero or more Workloads. Avoid: mock cluster, fake cluster, test cluster.

**Zone**: One failure domain of a Simulated Cluster, holding the Nodes that are lost together. Relationships: a Zone holds one or more NodePools, and through them one or more Nodes. Avoid: AZ, availability zone, region, datacenter.

**Node**: One simulated machine that Pods are placed on. Relationships: a Node belongs to exactly one Zone and exactly one NodePool, and hosts zero or more Pods. Avoid: host, machine, worker, instance.

**Pod**: One replica of a Workload, either placed on a Node or left unplaced. Avoid: container, replica, task.

**Workload**: One declared group of Pod replicas, deployed to the Simulated Cluster under the one Pod specification it declares. Relationships: a Workload owns one or more Pods. Avoid: app, service, deployment.

**Pod specification**: The complete set of settings a Workload's Pods are deployed with, named for the intent it serves. Relationships: the Scenarios define one per intent, once, beside the Scenarios that use it, and the lab defines none and only carries out the definition. A Workload declares one of them with zero or more Customizations. A specification's definition names every setting it holds, the lab's own included, and its own Startup time. Resilient placement spreads a Workload's Pods evenly over the Zones and over the Nodes. Default placement leaves where its Pods land to the scheduler. Avoid: preset, profile, Pod template, PodSpec; Maximum availability and No spread rules, the earlier names of those two intents.

**Setting**: One value a Pod specification gives one Kubernetes configuration field, under the name Kubernetes gives that field. Relationships: the lab's own vocabulary names the settings that decide where a Workload's Pods land and when they leave a Node, and Full access reaches every other one. A Named setting stands for several Settings under one name. A Customization changes one or more Settings. Avoid: field, option, parameter, knob, property.

**Named setting**: A name the Scenarios define once for a fixed group of the lab's Settings, written under their Kubernetes names, so a Pod specification's definition names the group in one word. Relationships: the Scenarios define each one beside the Pod specifications that use it, as they define Resilient placement. The lab offers only the means to group Settings under a name, and keeps its own waiting time holding whatever a group states. The Scenarios define two today. One, for Pods leaving an unresponsive Node, stands for exactly the not-ready and unreachable tolerations at the lab's waiting time, and every predefined Pod specification names it. The other is the Even spread, which leaves one value open, the subject it spreads over. The lines beneath a Workload's Given line report each Setting a Named setting stands for, under its Kubernetes name. Avoid: preset, shortcut, alias, bundle, macro; Named Node capacity, which stands for a Node's room.

**Even spread**: The Named setting for one topology spread of a Workload's Pods over the subject it is given, Zones, Nodes, or a Node label a NodePool states, keeping the Skew at one at most. Relationships: it places no Pod that would exceed that Skew and counts only Nodes whose taints the Pods tolerate. Resilient placement names it once over Zones and once over Nodes. A Scenario wanting another spread states that spread under its Kubernetes names. Avoid: balanced spread, spread rule, anti-affinity.

**Request**: The cpu and memory one Pod of a Workload asks a Node for. Relationships: the Pod specification a Workload declares holds it as a Setting, under the name Kubernetes gives that setting, and the scheduler places a Pod by it. A Pod whose Request no Node has room for stays Pending. Avoid: limit, reservation, quota, demand, resource.

**Customization**: One change a Scenario applies to a Pod specification, yielding a changed copy and leaving the specification it was applied to as it was. Relationships: a Scenario applies zero or more Customizations in the order written, before the Workload is deployed, and a later one wins where two touch the same Setting. A Customization names each Setting it changes under the name Kubernetes gives it, either in the lab's own vocabulary or through Full access. A Startup time is the one thing a Customization changes that is no Setting. Avoid: override, mutation, patch, modifier.

**Full access**: The Customization that reaches every Kubernetes Pod setting, for a Scenario needing one the lab's own vocabulary does not name. Relationships: a Scenario writes it among its Customizations and names each Setting as Kubernetes names it, so that Scenario reads partly in Kubernetes' own vocabulary. What the lab itself depends on holds whatever Full access leaves out. The waiting time before Pods leave an unresponsive Node stands over it, whatever it names. Avoid: escape hatch, raw spec, passthrough, override.

**Topology**: The arrangement of Zones and Nodes a Simulated Cluster starts from. Relationships: a Topology names a Zone count and the NodePools every Zone holds. Avoid: layout, cluster shape, setup.

**NodePool**: A group of identical Nodes in a Zone sharing one Node capacity and one name, as the cloud providers' node pools do. Relationships: a Zone holds one or more NodePools, every Zone of a Topology holds the same ones, and every Node belongs to exactly one. Every NodePool states a name, and a Node carries its NodePool's name as a label. A NodePool may also state Node labels of its own beside its Node capacity, and Taints apart from them. Avoid: pool, node group, machine set, instance group.

**Node label**: One key and value a Node carries, under Kubernetes' names, which a Then counts Pods by, the Even spread spreads them over, and a Failure by label stops Nodes by. Relationships: the lab sets its Zone, its hostname and its NodePool's name on every Node, and they stay the lab's. A NodePool states any others its Nodes carry, beside its Node capacity, holding one value in every Zone. The Topology's Given line reports them. Avoid: tag, marker, annotation, attribute.

**Taint**: One key, value and effect a Node carries under Kubernetes' names, keeping off it every Pod without a Toleration of it. Relationships: a NodePool states any Taints its Nodes carry, apart from its Node labels, and the Topology's Given line reports them. The effect NoSchedule is the one a NodePool states so far. Avoid: repellent, block, lock; Node label, which keeps no Pod off.

**Toleration**: A Setting of a Pod specification that lets its Pods onto Nodes carrying one Taint, under Kubernetes' names. Relationships: a Toleration of a NoSchedule Taint names its key, the Equal operator, its value and effect, and no waiting time. It permits a Pod onto a tainted Node and never draws it there, while a Request only those Nodes have room for does. The Named setting for leaving an unresponsive Node stands for two Tolerations of Taints the control plane sets. Avoid: exemption, exception, allowance, permit.

**Node capacity**: The room one Node has: how much cpu and memory, and how many Pods it holds. Relationships: every NodePool states one for every Node it holds, either directly or through a Named Node capacity, and a NodePool stating none is refused. A capacity stated directly names the cpu, the memory and the Pod count, all three. A Pod is placed on a Node whose room covers that Pod's Request. Avoid: node size, machine type, instance size; Required Capacity, which is a Workload's own minimum and not a Node's room.

**Named Node capacity**: A Node capacity the Scenarios define once under a name, beside the Scenarios that use it, so a NodePool names it instead of stating its cpu, memory and Pod count. Relationships: the lab defines none and only carries out the definition a Scenario hands it. The lab's Scenarios define Standard today, and each further one arrives with the first Scenario that uses it. A Topology's Given line reports the cpu, memory and Pod count each name stands for. Avoid: preset, default, node size, machine type, instance type.

**Startup time**: The time one placed Pod of a Simulated Cluster takes to become Ready. Relationships: every predefined Pod specification names its own, a shortest and a longest both above zero, and a Customization of that specification may state another. A Customization states one time or a shortest and a longest, in whole seconds or whole milliseconds. Between a shortest and a longest, each Pod takes its own time. A Pod inside its Startup time stands on its Node, Pending and not yet Running. Avoid: boot time, warm-up, startup delay, readiness delay, latency.

**Placement**: The mapping from each Pod of a Workload to the Node and Zone it occupies at one moment. Avoid: scheduling result, assignment, distribution.

**Settled**: The state a Placement reaches when it stops changing, so a later observation of the same Workload reports the same Pods on the same Nodes. Relationships: the lab observes it beneath a Scenario's Wait, so the Thens after that Wait judge a Settled Placement, and a Scenario may also Wait for it by name. A Placement holding Pods that stay Pending is Settled once it stops changing. Required Capacity is judged once immediately after a Failure and again once the Placement is Settled. Avoid: stable, converged, quiesced, final.

**Ready**: The condition Kubernetes reports for a Pod able to serve, and for a Node able to take Pods. Relationships: a Scenario Waits for every Pod of a Workload it declared, or for every Node of its Topology, to be Ready. Avoid: live, healthy, up, started, available.

**Wait**: The Step of a Scenario where it waits for a state of the cluster before going on. Relationships: an engineer writes a Wait with the Given before it or as a Step of its own. Either way it reports a line of its own, naming the time it took and, where one was stated, the time it was held to. A Wait applies what the Givens before it declared, then waits. A Wait may state the time it must be reached in, and the lab judges the time it waited against that. A Wait stating none is held to the lab's own waiting time. A Scenario Waits for every Node of its Topology to be Ready, for every Pod of a Workload to be Ready, or for the Placement alone to be Settled. The last holds although Pods stay Pending. Every Then judges the Placement a Wait reached. Avoid: barrier, settle step, synchronization point; timeout or deadline for the time a Wait states.

**Skew**: The difference between the highest and the lowest Pod count across the Zones, the Nodes, or the values of one Node label, of one Workload. Avoid: imbalance, spread delta.

**Failure**: A loss injected into the Simulated Cluster on purpose, scoped to one Pod, one Node, one whole Zone, or every Node carrying one value of a Node label. Relationships: a Scenario injects zero or more Failures. A Node Failure leaves the Node in the cluster and unresponsive, so the control plane moves its Pods away after a waiting time. A Zone Failure stops every Node of one Zone responding at once, because a Zone holds the Nodes that are lost together. A Failure by label stops every Node carrying the value it names, in every Zone at once, as a cloud reclaims its spot Nodes. A Scenario states how many Nodes or how many Zones fail, names a Node label and one of its values, or points at a Node by its position in the Topology. Avoid: chaos, fault, outage, incident.

**Required Capacity**: The minimum number of Running Pods a Workload needs to count as surviving a Failure. Relationships: a Workload declares it beside the Pods it runs, and it is judged twice per Failure — once at the moment of the Failure, once after Placement settles again. A Pod on a Node that stopped responding does not count as Running from the moment of the Failure. Avoid: quorum, minimum availability, headroom.

**Failure tolerance**: The property that a Workload goes on serving its Required Capacity through a Failure. Relationships: a Workload declares the Required Capacity its Failure tolerance is judged against, and a Then after the Failure judges both moments and states the Pods it counted at each. Losing a Zone or a Node costs the spread, and a Failure tolerance holds while the Pod count does. Avoid: resilience, availability, redundancy, high availability, SLO.

**Scenario**: A named sequence of Steps run against a Simulated Cluster, ending in an asserted Placement outcome. Relationships: a Scenario provisions one Simulated Cluster with one Topology, deploys one or more Workloads, and injects zero or more Failures. Avoid: test case, experiment, simulation.

**Step**: One line of a Scenario, in one of four kinds: a Given that declares, a When that changes the cluster, a Wait, or a Then that asserts an outcome. Relationships: a Scenario is a sequence of Steps. A Given declares a Topology or a Workload and Waits only where the Scenario asks it to. A When injects one Failure. A Wait waits for a state of the cluster, and an engineer may write it with the Given before it. A Then asserts one Placement outcome. Avoid: stage, phase, clause, instruction.

**Scenario name**: The short name of one Scenario, read from the condition its function names, which a run reports and which selects that Scenario to run on its own. Relationships: exactly one per Scenario, the condition half of its function's name and never the outcome half. Avoid: run name (a run is one invocation of the lab), test name, scenario id; Even spread and resilient-placement, earlier names of the Scenario a-workload-placed-for-resilience.

**Timeline**: The ordered record of what changed in a Scenario's Simulated Cluster while the Scenario ran. Relationships: one Timeline per Scenario of a run, holding that Scenario's Steps at their moment and its Node, Pod and Workload changes, with the control plane's reason for each Placement. A Timeline states what happened; a Finding judges it and a Failure causes it. Avoid: event log, audit log, history, trace, Event (a Kubernetes Event object is one entry of a Timeline).

**Entry**: One change a Timeline records, naming when the lab received it, what changed, and the control plane's reason where it reported one. Relationships: a Timeline is an ordered sequence of Entries, and a Kubernetes Event object is one Entry. Avoid: event, log line, record.

**Wait narrative**: The moments that changed the Placement since the previous Wait, which a run prints in order under a Wait. Relationships: one per Wait, naming Nodes turning Ready or NotReady, Pods scheduled with their Node and when they reached Running, Pods evicted, and Pods left Pending with the scheduler's reason. A separator line follows it, then the Placement board. It states the order of an eviction and a replacement, never a link between them. Avoid: status, Timeline summary (the earlier report it replaces), changelog; Timeline, which is the whole record.

**Placement board**: The state of the whole Simulated Cluster that a run prints under a Wait, beneath its Wait narrative and a separator line. Relationships: one per Wait, across every Workload, giving one line per Node grouped by Zone with its NodePool. Each Node's line lists every Pod on it by Workload name and the Pod's own suffix, marking newcomers since the previous board. Pending Pods are listed beneath. A second separator line closes it, before the Thens judging that Wait's Placement and the next Step. Avoid: status, snapshot, Pods per Zone, Pods per Node, map, grid.

**Interactive run**: One invocation of the lab that holds the Simulated Cluster of one named Scenario for an engineer to watch in the Live view. Relationships: it carries out the Scenario's Steps up to its first When and reports them to the terminal as a run does. It removes the Simulated Cluster it holds when the engineer stops the lab. Avoid: session, demo, playground; run alone, which removes each Simulated Cluster when its Scenario ends.

**Live view**: The page a browser shows of an Interactive run's Simulated Cluster, following its Placement as it changes. Relationships: one per Interactive run, showing every Zone, each Zone's Nodes with their NodePool, the Pods on each Node by Workload name and own suffix, and the Pending Pods. Avoid: dashboard, UI, visualisation, web board; Placement board, which a run prints under a Wait.

**Finding**: An outcome a Then asserts that the Placement does not meet. Relationships: a Finding is a result of the Scenario, unlike a Step that cannot be carried out, which ends it. Avoid: failure (that word is the injected loss), error, bug, violation.

## Example Dialogue

**Developer:** The Scenario provisions three Zones with three Nodes each, then deploys a Workload of nine Pods. What does it assert?

**Domain expert:** That the Placement settles at nine Running Pods with a Skew of zero — three Pods in every Zone.

**Developer:** And after a Zone Failure?

**Domain expert:** The two surviving Zones hold all nine Pods, or the ones that cannot be placed stay Pending. Which of the two is correct depends on the Pod specification the Workload declares, not on the Simulated Cluster.

**Developer:** So Required Capacity is what tells us whether that outcome is acceptable.

**Domain expert:** Yes. Judge it right after the Failure and again once Placement has settled.
