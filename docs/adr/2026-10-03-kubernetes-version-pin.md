# Kubernetes version pin for the Simulated Cluster

**Status:** Accepted

## Context

Every assertion the lab makes is relative to a Kubernetes version. Topology-spread evaluation, eviction behavior, and scheduler defaults change between minors. The version the Simulated Cluster runs is therefore the lab's fidelity promise, and it has to be a recorded decision rather than whatever a tag resolves to today.

The owner mirrors no named production cluster and wants to compare versions against each other, so a run has to be able to name the version it mirrors. The PRD leaves open whether the lab ever promises anything about a team's own manifests.

## Options Considered

1. **Mirror a named environment** — the strongest promise, and it binds the lab to a cluster estate the owner does not have.
2. **Pin the newest available minor** — the behavior a team upgrades into, and it moves whenever the lab is deliberately bumped.
3. **A floating tag** — no pin. Assertions change meaning without a commit.

## Decision

Option 2. The image reference is composed from two package-level constants and one run input: the KWOK cluster image repository, the KWOK v0.8.0 release, and the Kubernetes version. The default version is the newest available, v1.36.1. Moving the default is a reviewed one-line change to that constant.

`--k8s-version` names the version for one run, which is how a Kubernetes upgrade is exercised before it is committed. Resolution happens once at startup and the run prints the version it mirrors, so an outcome names the version that produced it. The default is the pinned constant, and no version is looked up on a registry.

The flag selects a version and nothing else. No input reaches the repository constant, and a value that is not a version is refused before any request. Both hold by construction, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md). The lab's other input selects which local engine starts the container, never a cluster address, per the same ADR.

Option 1 becomes available without reversing this record: the default constant changes to the named environment's minor, and the PRD's fidelity question resolves at the same time.

## Consequences

An assertion's meaning is traceable to the printed version. Two versions are compared by running the same Scenario twice and reading which outcomes differ, which is a use the lab was built for.

A version with no published cluster image is a failed pull naming the version, which is the price of letting a run name one.

The tag resolves. `registry.k8s.io/kwok/cluster:v0.8.0-k8s.v1.36.1` answers with digest `sha256:ca4e2441f16d95ebb8b8afe0f85c4fc0916c9e8323a394f2078bbcf6552ba4da`, published for `linux/amd64` and `linux/arm64`.

Pinning by tag leaves a moved tag undetectable. Digest pinning is the stronger control and is not yet in force; `security-principles.md` carries it as a standing gap. It would pin the digest above for the default version and need one digest per further version.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § Constants](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
