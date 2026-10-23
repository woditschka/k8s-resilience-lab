# The lab is a Go program on a KWOK Simulated Cluster

**Status:** Accepted

## Context

The lab answers one question: does a declared Topology and Deployment produce the Placement the manifests intend? Answering it needs genuine Kubernetes scheduling, a run that costs seconds, and no cluster per developer or per build (G-1). The same behavior also drives a live demonstration (G-2), so it cannot live only in a test suite.

The owner asked for the language by name: "ideomatic go". Kubernetes is written in Go, so the lab exercises the configuration surface in the language that defines it.

The module rules are asserted from the module's own package graph as well as by the linter, and more than one module's tests need the same graph walk.

## Options Considered

**What the lab schedules against:**

1. **A real cluster per run** — kind, k3d, or minikube. Minutes to start, gigabytes of image pulls, and application behavior the lab does not measure.
2. **A hand-written scheduler simulation** — no Kubernetes behavior, so every assertion would test the simulation.
3. **A KWOK Simulated Cluster** — a genuine control plane with fake Nodes and fake Pods; nothing runs inside a Pod.

**How the Go code is split:**

4. **A modulith of domain packages under `internal/`** — one binary whose parts are packages closed to outside importers.
5. **A tree of technical layers** — `models/`, `services/`, `adapters/`, which the architecture brief's module layout prohibits.

**How the module-rule tests share their graph walk:**

6. **A copy in each module's tests** — a rule changed in one copy and not the other is asserted for one module only.
7. **One package's test asserts every rule** — a module's rule asserted outside it survives that module being moved or split by accident.
8. **A test-support package under `internal/`** — one copy, imported by test files alone.

## Decision

Options 3, 4 and 8. One Go module, composed in `cmd/lab`, with the `scenarios` package beside it. Its modules under `internal/` are `scenario`, `cluster`, `expect`, `lab` with `lab/pod` and `lab/node`, `live`, `timeline` and `modulerules`. `scenario` holds the description with no I/O, and `cluster` holds every Kubernetes call. A description is therefore assertable without a cluster, and the same packages serve the run and the Live view.

One client library holds the Kubernetes boundary, `k8s.io/client-go`, and `github.com/google/go-cmp` supplies the failure diffs. The compiler rejects an import cycle, `internal/` closes a package to outside importers, and a depguard rule denies every `k8s.io/...` import from `internal/scenario`. Kubernetes is a boundary the project does not control, so anti-corruption at it is mandatory.

A provisioned fake Node is created with its status already Ready. KWOK's `node-initialize` stage fires only on a Node not yet Ready, and substitutes a capacity of its own for the declared one.

`internal/modulerules` holds the package-graph walk and the call-rule scan. The module-rule tests import it, no production package does, and the import-graph test asserts that absence. It imports the standard library alone.

## Consequences

The control plane's scheduling behavior is the real one, so an assertion means what it says. A run costs a container start rather than a cluster build. Fidelity tracks the Topology and the Kubernetes version, and nothing more. No application-level behavior exists to observe, which the PRD declines as NG-2.

Node readiness is lab-written at provisioning and control-plane-owned after it, so a KWOK upgrade that changes `node-initialize` is a change to this ADR.

`docs/architecture-principles.md` § Language Realization and § Go Idioms govern the Go forms. A module rule changes in one place, and every module asserting it asserts the changed rule.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-004

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md)
