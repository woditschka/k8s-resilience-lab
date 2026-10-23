# The lab provisions the Simulated Cluster it drives

**Status:** Accepted

## Context

The lab reaches its cluster one way, so a run and the Live view share one code path. It also never runs against a real or pre-existing cluster, including one reached through a supplied address (NG-1). `client-go` reads a kubeconfig or an in-cluster token by default, so a misconfigured run could drive a production cluster while reporting success.

Starting the container means reaching a container engine, and each engine publishes its API on a unix socket at its own path.

A Scenario's Placement is trustworthy only when nothing from another Scenario can reach it. One named Scenario run alone (REQ-SIM-003) must report what the full run reports for it. Each Scenario spends seconds inside its cluster and about a minute provisioning and removing it (REQ-SIM-014). The owner asked for an Interactive run whose cluster "stays interactive", one cluster at a time (REQ-SIM-022).

## Options Considered

How the lab reaches the engine:

1. **An address flag or environment variable** — one code path, and a value an operator can point at any cluster.
2. **The engine's command-line tool through `os/exec`** — a child process per call, a text format to parse, and lab values in an argument vector.
3. **testcontainers-go or the Docker SDK** — the same lifecycle, plus a container library and its client tree in the binary.
4. **The engine's HTTP API over its unix socket, with the standard library**.

How many clusters a run holds:

5. **One per run, cleaned between Scenarios** — cleanliness depends on the cleanup code being complete.
6. **One long-lived cluster reused across runs** — every outcome depends on what ran before.
7. **One per Scenario** — a container start per Scenario, and isolation that depends on nothing.

How a run schedules its Scenarios:

8. **One after another** — a run as long as the sum of its Scenarios.
9. **All at once, each in its own goroutine** — a run as long as its slowest Scenario.

How long an Interactive run keeps its cluster:

10. **Removed when its Steps end, as in a run** — nothing left to watch.
11. **Held until the engineer stops the lab, one at a time**.

## Decision

Options 4, 7, 9 and 11.

- **Endpoint.** One detection at startup serves the whole run. A `unix://` `DOCKER_HOST` that answers wins; any other scheme, or a silent path, is skipped with a warning. A fixed candidate list is then probed in order, a home-relative path composed with `os.UserHomeDir` and dialed, never read. The run prints the socket and engine version, or names every path tried.
- **Container.** `Provision` creates the container from the composed image reference, pulling only when the image is missing. It publishes the API port on loopback at an engine-chosen port and waits for readiness. Its name carries the Scenario and a random suffix.
- **Engine surface.** Exported methods take a context and a validated version alone; inspect, stop and remove act on this run's handle. The package declares no list and no by-identifier call, which a surface test asserts.
- **Client.** A `rest.Config` carries the provisioned address and no credential. Depguard denies `clientcmd` and the other discovery imports, and `forbidigo` denies in-cluster configuration.
- **Isolation.** Each Scenario runs on its own cluster, removed when it ends, a failure or panic included; the runner holds and closes the handle.
- **Concurrency.** No package holds mutable process-wide state. The runner derives every Scenario name, then obtains the image once, before any cluster. It recovers a panic into that Scenario's error, joins every goroutine and returns Outcomes in declared order.
- **Interactive run.** One Scenario runs through the same runner up to its first When, its Timeline filed there. The cluster is held until the engineer stops the lab.

Option 1 hands an operator a cluster address. Options 2 and 3 add a process or a dependency where the standard library suffices. Options 5 and 6 let one Scenario's leftovers reach another. Option 8 makes a run as long as every Scenario together. Option 10 leaves the Live view nothing to show.

## Consequences

NG-1 is enforced by absence: no cluster address to supply, no credential to use, and no code path reading either. Only a `unix://` path is dialed, so no run reaches an engine across a network. A stale `DOCKER_HOST` costs one probe (the PRD's edge case 3).

The socket is full control of this machine's containers. The lab's own surface is the narrowing: one image repository and the containers this run created, so two invocations cannot collide.

An unauthenticated API server exists for each Scenario's lifetime, confined to loopback. A run's footprint is one container per Scenario at once; a machine short of room is an Open Question of the PRD. The gate bounds its cluster-backed tests through `-parallel`.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-014, REQ-SIM-022

## References

- [system-design.md § Threat Model](../system-design.md#threat-model)
- [ADR: Non-goal — real-cluster runs](2026-10-03-non-goal-real-cluster-runs.md)
- [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md)
