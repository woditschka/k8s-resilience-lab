# The Live view is served on loopback from one shared observation

**Status:** Accepted

## Context

On 2026-10-21 the owner asked to "start a webserver and visuzlize it in a browser", to "see how zones, nodes and pods are distributes" (REQ-SIM-022). An Interactive run holds one Simulated Cluster, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md). Its Live view follows the Placement as it changes, without a reload, in one browser or several.

This is the lab's first inbound network surface. Until now the lab only dialed out: to the engine socket and to a loopback API server. The PRD leaves open whether the Live view is reachable from another machine. Later slices will inject a Failure from the page and may stream the Timeline to it.

The lab already reads the whole Placement in one observation for each Wait's Placement board. That board holds what the Live view shows: every Zone, each Node with its NodePool, each Pod by Workload name and suffix, and the Pending Pods.

## Options Considered

How the page learns of a change:

1. **The page fetches a server-rendered fragment at a fixed interval** — standard library alone, no connection held, every browser stateless to the server.
2. **Server-sent events pushing each changed board** — prompter, at the cost of one held connection and a broadcast per browser.
3. **A WebSocket** — needs a dependency outside the approved sources, or a hand-written protocol at a trust boundary.

Where the shown Placement comes from:

4. **One observer per Interactive run, polling the held cluster; every request reads its latest board** — API load independent of the browsers open.
5. **One observation per page request** — API load grows with the browsers and their tabs.
6. **The Timeline's watch streams** — no polling, but the record stops with the Steps and a board needs the declared Topology besides.

Where the server listens:

7. **The loopback address, on a port the operating system chooses** — the API server's own exposure; two invocations never collide.
8. **Every interface, or an address flag** — a decision the PRD keeps open.

## Decision

Options 1, 4 and 7. The Live view is a `net/http` server in the binary, rendering the Placement board with `html/template`, its page and script embedded. It serves reads alone. A request with another method, another path, or a Host other than the address served is refused. Server timeouts bound every connection, and stopping the lab shuts the server down within a bound.

One observer goroutine per Interactive run observes the held cluster at a fixed interval, from provisioning until removal. Each observation builds the Placement board against the Scenario's declared Topology, so the Live view and the report share one model. Every request reads the latest board, or the cause the last observation failed.

The terminal names the address once the server listens, before provisioning starts.

## Consequences

No dependency enters the module graph, and the Live view adds no Kubernetes import outside `internal/cluster`.

A change reaches the page within one observer interval plus one fetch interval, coarser than the watch streams. A later slice streaming the Timeline may revisit option 2 then.

Another machine cannot reach the Live view. Widening that is a PRD decision, then a change to this ADR. The Host check keeps a page from another site, rebound to loopback, from reading the Live view.

Reading is the server's whole surface. Injecting a Failure from the page adds the first state-changing request, needing its own guard against cross-site requests.

## Implementation

**Requirements:** REQ-SIM-022

**Files:** `internal/live/`, `internal/lab/runner.go`, `cmd/lab/main.go`, `.golangci.yml`

## References

- [system-design.md § Contracts](../system-design.md)
- [system-design.md § Threat Model](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
