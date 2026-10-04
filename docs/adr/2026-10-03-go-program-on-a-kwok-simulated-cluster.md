# The lab is a Go program on a KWOK Simulated Cluster

**Status:** Accepted

## Context

The lab answers one question: does a declared Topology and Workload produce the Placement the manifests intend? Answering it needs genuine Kubernetes scheduling, a run that costs seconds, and no cluster per developer or per build (G-1). The same behavior also drives a live demonstration (G-2), so it cannot live only in a test suite.

The language is Go, asked for by name — "ideomatic go". Kubernetes is written in Go, so the lab exercises the configuration surface in the language that defines it.

## Options Considered

**What the lab schedules against:**

1. **A real cluster per run** — kind, k3d, or minikube. Real kubelet, real containers, minutes to start, gigabytes of image pulls, and application behavior the lab does not measure.
2. **A hand-written scheduler simulation** — no container runtime needed, and no Kubernetes behavior either. Every assertion would test the simulation.
3. **A KWOK Simulated Cluster** — a genuine control plane with fake Nodes and fake Pods; nothing runs inside a Pod.

**How the Go code is split:**

4. **A modulith of domain packages under `internal/`** — one binary whose parts are packages closed to outside importers rather than services.
5. **A tree of technical layers** — `models/`, `services/`, `adapters/`, which the architecture brief's module layout prohibits.

## Decision

Options 3 and 4. One Go module on Go 1.27, three packages under `internal/` — `scenario`, `cluster`, `expect` — and a composition root in `cmd/lab`. `scenario` holds the description with no I/O; `cluster` holds every Kubernetes call. A description is therefore assertable without a cluster, and the same packages serve the suite and the live view. One client library holds the Kubernetes boundary, `k8s.io/client-go`, and `github.com/google/go-cmp` supplies the failure diffs. Nothing else is added.

Three mechanisms enforce the split: the compiler rejects an import cycle, `internal/` closes a package to outside importers, and a `golangci-lint` depguard rule denies every `k8s.io/...` import from `internal/scenario`. Kubernetes is a boundary the project does not control, so anti-corruption at it is mandatory.

A provisioned fake Node is created with its status already Ready. KWOK's `node-initialize` stage fires only on a Node that is not yet Ready, and substitutes a capacity of its own, displacing the capacity a Scenario declared.

## Consequences

The control plane's scheduling behavior is the real one, so an assertion means what it says. A run costs a container start rather than a cluster build. Fidelity tracks whatever the Topology and the Kubernetes version mirror, and nothing more. No application-level behavior exists to observe, which the PRD declines as NG-2.

Deleting a Node is instant here, where a real cluster passes through NotReady and a toleration timeout. A later slice that needs that path injects a taint instead.

Node readiness is lab-written at provisioning and control-plane-owned after it, so the lab is coupled to KWOK's initialization behavior at that one point. A KWOK upgrade that changes the `node-initialize` stage is a change to this ADR.

`docs/architecture-principles.md` § Language Realization and § Go Idioms govern the Go forms, construction and naming included. Reviewers judge code against those rows, not the creator forms in § Naming.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-004

## References

- [system-design.md § Overview](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md)
- [ADR: Non-goal — application-level recovery](2026-10-03-non-goal-application-level-recovery.md)
