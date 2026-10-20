# The lab is a Go program on a KWOK Simulated Cluster

**Status:** Accepted, amended 2026-10-04: the module-rule tests share one test-support package, `internal/modulerules`, option 8 (REQ-SIM-004)

## Context

The lab answers one question: does a declared Topology and Workload produce the Placement the manifests intend? Answering it needs genuine Kubernetes scheduling, a run that costs seconds, and no cluster per developer or per build (G-1). The same behavior also drives a live demonstration (G-2), so it cannot live only in a test suite.

The language is Go, asked for by name — "ideomatic go". Kubernetes is written in Go, so the lab exercises the configuration surface in the language that defines it.

On 2026-10-04 two packages, `internal/cluster` and `internal/scenario`, asserted the module rules from the module's own package graph, independently of the linter. Each carried the same graph walk, byte for byte, beside its call-rule scan. A helper shared as production source has no production caller, which the duplication was avoiding.

## Options Considered

**What the lab schedules against:**

1. **A real cluster per run** — kind, k3d, or minikube. Real kubelet, real containers, minutes to start, gigabytes of image pulls, and application behavior the lab does not measure.
2. **A hand-written scheduler simulation** — no container runtime needed, and no Kubernetes behavior either. Every assertion would test the simulation.
3. **A KWOK Simulated Cluster** — a genuine control plane with fake Nodes and fake Pods; nothing runs inside a Pod.

**How the Go code is split:**

4. **A modulith of domain packages under `internal/`** — one binary whose parts are packages closed to outside importers rather than services.
5. **A tree of technical layers** — `models/`, `services/`, `adapters/`, which the architecture brief's module layout prohibits.

**How the module-rule tests share their graph walk:**

6. **The duplication stays** — a comment in each copy naming the reason.
7. **One package's test asserts both rules** — one copy, and a module's rule asserted from outside that module.
8. **A test-support package under `internal/`** — one copy, imported by test files alone.

## Decision

Options 3 and 4. One Go module on Go 1.27, composed in `cmd/lab`, with the `scenarios` package beside it. Its modules under `internal/` are `scenario`, `cluster`, `expect`, `lab` with `lab/pod` and `lab/node`, `timeline` and `modulerules`. `scenario` holds the description with no I/O; `cluster` holds every Kubernetes call. A description is therefore assertable without a cluster, and the same packages serve the suite and the live view. One client library holds the Kubernetes boundary, `k8s.io/client-go`, and `github.com/google/go-cmp` supplies the failure diffs. Nothing else is added.

Three mechanisms enforce the split: the compiler rejects an import cycle, `internal/` closes a package to outside importers, and a `golangci-lint` depguard rule denies every `k8s.io/...` import from `internal/scenario`. Kubernetes is a boundary the project does not control, so anti-corruption at it is mandatory.

A provisioned fake Node is created with its status already Ready. KWOK's `node-initialize` stage fires only on a Node that is not yet Ready, and substitutes a capacity of its own, displacing the capacity a Scenario declared.

Option 8, since 2026-10-04. `internal/modulerules` holds the package-graph walk and the call-rule scan. The module-rule tests import it, no production package does, and the import-graph test asserts that absence. It imports the standard library alone, so reaching it leaves `internal/scenario`'s own rule intact. Option 6 is set aside: a rule changed in one copy and not the other is asserted for one module only. Option 7 is set aside: a module's rule asserted outside it survives that module being moved or split by accident.

## Consequences

The control plane's scheduling behavior is the real one, so an assertion means what it says. A run costs a container start rather than a cluster build. Fidelity tracks whatever the Topology and the Kubernetes version mirror, and nothing more. No application-level behavior exists to observe, which the PRD declines as NG-2.

Deleting a Node is instant here, where a real cluster passes through NotReady and a toleration timeout. A later slice that needs that path injects a taint instead.

Node readiness is lab-written at provisioning and control-plane-owned after it, so the lab is coupled to KWOK's initialization behavior at that one point. A KWOK upgrade that changes the `node-initialize` stage is a change to this ADR.

`docs/architecture-principles.md` § Language Realization and § Go Idioms govern the Go forms, construction and naming included. Reviewers judge code against those rows, not the creator forms in § Naming.

`internal/modulerules` is a module whose callers are test files alone, beside `internal/expect`, whose callers are tests and Scenarios. Its helpers are pure functions over a package graph and a file set, so it carries its own test and needs no container engine. A module rule changes in one place, and every module asserting it asserts the changed rule.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-004

## References

- [system-design.md § Overview](../system-design.md)
- [system-design.md § Package Structure](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md)
- [ADR: Non-goal — application-level recovery](2026-10-03-non-goal-application-level-recovery.md)
