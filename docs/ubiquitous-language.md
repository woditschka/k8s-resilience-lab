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

**Zone**: One failure domain of a Simulated Cluster, holding the Nodes that are lost together. Relationships: a Zone holds one or more Nodes. Avoid: AZ, availability zone, region, datacenter.

**Node**: One simulated machine that Pods are placed on. Relationships: a Node belongs to exactly one Zone and hosts zero or more Pods. Avoid: host, machine, worker, instance.

**Pod**: One replica of a Workload, either placed on a Node or left unplaced. Avoid: container, replica, task.

**Workload**: One declared group of Pod replicas, deployed to the Simulated Cluster under the one Pod specification it declares. Relationships: a Workload owns one or more Pods. Avoid: app, service, deployment.

**Pod specification**: The complete set of settings a Workload's Pods are deployed with, named for the intent it serves. Relationships: the lab predefines one per intent, a Workload declares one of them with zero or more Customizations, and a specification's definition names every setting it holds, the lab's own included. Resilient placement spreads a Workload's Pods evenly over the Zones and over the Nodes. Default placement leaves where its Pods land to the scheduler. Avoid: preset, profile, Pod template, PodSpec; Maximum availability and No spread rules, the earlier names of those two intents.

**Setting**: One value a Pod specification gives one Kubernetes configuration field, under the name Kubernetes gives that field. Relationships: the lab's own vocabulary names the settings that decide where a Workload's Pods land and when they leave a Node, and Full access reaches every other one. A Customization changes one or more Settings. Avoid: field, option, parameter, knob, property.

**Customization**: One change a Scenario applies to a Pod specification, yielding a changed copy and leaving the specification it was applied to as it was. Relationships: a Scenario applies zero or more Customizations in the order written, before the Workload is deployed, and a later one wins where two touch the same Setting. A Customization names each Setting it changes under the name Kubernetes gives it, either in the lab's own vocabulary or through Full access. Avoid: override, mutation, patch, modifier.

**Full access**: The Customization that reaches every Kubernetes Pod setting, for a Scenario needing one the lab's own vocabulary does not name. Relationships: a Scenario writes it among its Customizations and names each Setting as Kubernetes names it, so that Scenario reads partly in Kubernetes' own vocabulary. What the lab itself depends on holds whatever Full access leaves out. The lab's own waiting time stands over it, whatever it names. Avoid: escape hatch, raw spec, passthrough, override.

**Topology**: The arrangement of Zones and Nodes a Simulated Cluster starts from. Relationships: a Topology names a Zone count, a Node count per Zone, and each Node's capacity. Avoid: layout, cluster shape, setup.

**Placement**: The mapping from each Pod of a Workload to the Node and Zone it occupies at one moment. Avoid: scheduling result, assignment, distribution.

**Settled**: The state a Placement reaches when it stops changing, so a later observation of the same Workload reports the same Pods on the same Nodes. Relationships: Required Capacity is judged once immediately after a Failure and again once the Placement is Settled. Avoid: stable, converged, quiesced, final.

**Skew**: The difference between the highest and the lowest Pod count across the Zones, or across the Nodes, of one Workload. Avoid: imbalance, spread delta.

**Failure**: A loss injected into the Simulated Cluster on purpose, scoped to one Pod, one Node, or one whole Zone. Relationships: a Scenario injects zero or more Failures. A Node Failure leaves the Node in the cluster and unresponsive, so the control plane moves its Pods away after a waiting time. A Zone Failure stops every Node of one Zone responding at once, because a Zone holds the Nodes that are lost together. A Scenario states how many Nodes or how many Zones fail, or points at a Node by its position in the Topology. Avoid: chaos, fault, outage, incident.

**Required Capacity**: The minimum number of Running Pods a Workload needs to count as surviving a Failure. Relationships: a Workload declares it beside the Pods it runs, and it is judged twice per Failure — once at the moment of the Failure, once after Placement settles again. A Pod on a Node that stopped responding does not count as Running from the moment of the Failure. Avoid: quorum, minimum availability, headroom.

**Failure tolerance**: The property that a Workload goes on serving its Required Capacity through a Failure. Relationships: a Workload declares the Required Capacity its Failure tolerance is judged against, and a Then after the Failure judges both moments and states the Pods it counted at each. Losing a Zone or a Node costs the spread, and a Failure tolerance holds while the Pod count does. Avoid: resilience, availability, redundancy, high availability, SLO.

**Scenario**: A named sequence of Steps run against a Simulated Cluster, ending in an asserted Placement outcome. Relationships: a Scenario provisions one Simulated Cluster with one Topology, deploys one or more Workloads, and injects zero or more Failures. Avoid: test case, experiment, simulation.

**Step**: One line of a Scenario, either a Given that sets up, a When that injects a Failure, or a Then that asserts an outcome. Relationships: a Scenario is a sequence of Steps; a Given declares a Topology or a Workload, a When injects one Failure, a Then asserts one Placement outcome. Avoid: stage, phase, clause, instruction.

**Scenario name**: The short name an engineer gives one Scenario, which a run reports and which selects that Scenario to run on its own. Relationships: exactly one per Scenario, and not the name of the Scenario's function. A Scenario name may repeat the Pod specification its Workload declares, so Resilient placement names both the Scenario and that specification. Avoid: run name (a run is one invocation of the lab), test name, scenario id; Even spread, the earlier name of the Resilient placement Scenario.

**Timeline**: The ordered record of what changed in a Scenario's Simulated Cluster while the Scenario ran. Relationships: one Timeline per Scenario of a run, holding that Scenario's Node, Pod and Workload changes together with the control plane's reason for each Placement. A Timeline states what happened; a Finding judges it and a Failure causes it. Avoid: event log, audit log, history, trace, Event (a Kubernetes Event object is one entry of a Timeline).

**Entry**: One change a Timeline records, naming when the lab received it, what changed, and the control plane's reason where it reported one. Relationships: a Timeline is an ordered sequence of Entries, and a Kubernetes Event object is one Entry. Avoid: event, log line, record.

**Timeline summary**: The short report of one Scenario's Timeline that a run prints when the Scenario ends, scannable in one screen. Relationships: one per Timeline, grouping the Timeline's Entries by the Topology, each Workload and each Pod, while every Entry stays in the Timeline's file. Avoid: digest, overview, Timeline report.

**Finding**: An outcome a Then asserts that the Placement does not meet. Relationships: a Finding is a result of the Scenario, unlike a Step that cannot be carried out, which ends it. Avoid: failure (that word is the injected loss), error, bug, violation.

## Example Dialogue

**Developer:** The Scenario provisions three Zones with three Nodes each, then deploys a Workload of nine Pods. What does it assert?

**Domain expert:** That the Placement settles at nine Running Pods with a Skew of zero — three Pods in every Zone.

**Developer:** And after a Zone Failure?

**Domain expert:** The two surviving Zones hold all nine Pods, or the ones that cannot be placed stay Pending. Which of the two is correct depends on the Pod specification the Workload declares, not on the Simulated Cluster.

**Developer:** So Required Capacity is what tells us whether that outcome is acceptable.

**Domain expert:** Yes. Judge it right after the Failure and again once Placement has settled.
