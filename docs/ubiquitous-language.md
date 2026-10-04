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

**Workload**: One declared group of Pod replicas, deployed to the Simulated Cluster together with the rules that constrain where its Pods may land. Relationships: a Workload owns one or more Pods. Avoid: app, service, deployment.

**Topology**: The arrangement of Zones and Nodes a Simulated Cluster starts from. Relationships: a Topology names a Zone count, a Node count per Zone, and each Node's capacity. Avoid: layout, cluster shape, setup.

**Placement**: The mapping from each Pod of a Workload to the Node and Zone it occupies at one moment. Avoid: scheduling result, assignment, distribution.

**Settled**: The state a Placement reaches when it stops changing, so a later observation of the same Workload reports the same Pods on the same Nodes. Relationships: Required Capacity is judged once immediately after a Failure and again once the Placement is Settled. Avoid: stable, converged, quiesced, final.

**Skew**: The difference between the highest and the lowest Pod count across the Zones, or across the Nodes, of one Workload. Avoid: imbalance, spread delta.

**Failure**: A loss injected into the Simulated Cluster on purpose, scoped to one Pod, one Node, or one whole Zone. Relationships: a Scenario injects zero or more Failures. Avoid: chaos, fault, outage, incident.

**Required Capacity**: The minimum number of Running Pods a Workload needs to count as surviving a Failure. Relationships: judged twice per Failure — once immediately after it, once after Placement settles again. Avoid: quorum, minimum availability, headroom.

**Scenario**: A named sequence of steps run against a Simulated Cluster, ending in an asserted Placement outcome. Relationships: a Scenario provisions one Simulated Cluster with one Topology, deploys one or more Workloads, and injects zero or more Failures. Avoid: test case, experiment, simulation.

## Example Dialogue

**Developer:** The Scenario provisions three Zones with three Nodes each, then deploys a Workload of nine Pods. What does it assert?

**Domain expert:** That the Placement settles at nine Running Pods with a Skew of zero — three Pods in every Zone.

**Developer:** And after a Zone Failure?

**Domain expert:** The two surviving Zones hold all nine Pods, or the ones that cannot be placed stay Pending. Which of the two is correct depends on the Workload's rules, not on the Simulated Cluster.

**Developer:** So Required Capacity is what tells us whether that outcome is acceptable.

**Domain expert:** Yes. Judge it right after the Failure and again once Placement has settled.
