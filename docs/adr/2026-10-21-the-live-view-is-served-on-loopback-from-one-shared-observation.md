# The Live view is served on loopback from one shared observation

**Status:** Accepted, amended 2026-10-21: the board is nested boxes pushed as Server-Sent Events, options 2, 9 and 11; amended 2026-10-22: a Node box shows its capacity, requests and room left, and a Pod box its Request

## Context

On 2026-10-21 the owner asked to "start a webserver and visuzlize it in a browser", to "see how zones, nodes and pods are distributes" (REQ-SIM-022). An Interactive run holds one Simulated Cluster, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md). Its Live view follows the Placement as it changes, without a reload, in one browser or several.

This is the lab's first inbound network surface. Until now the lab only dialed out: to the engine socket and to a loopback API server. The PRD leaves open whether the Live view is reachable from another machine. Later slices will inject a Failure from the page and may stream the Timeline to it.

The lab already reads the whole Placement in one observation for each Wait's Placement board. That board holds what the Live view shows: every Zone, each Node with its NodePool, each Pod by Workload name and suffix, and the Pending Pods.

The first slice chose option 1 below. Later on 2026-10-21 the owner asked for "zones, nodes and pods as boxes contained within eachoter with important infrmation e.g. capacity (total/left) labels,... clearly visible". They added that "the status should constantly update pushed from the server from the k8s cluster state". The decision moved from option 1 to option 2, and the board's drawing was decided.

On 2026-10-22 the owner said: "the cpu and memory requests of pods are important in my view to see how full the node is". Each Pod's Request stands on its Deployment line, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md).

## Options Considered

How the page learns of a change:

1. **The page fetches a server-rendered fragment at a fixed interval** — standard library alone, no connection held, every browser stateless to the server. The first slice's choice, set aside for option 2.
2. **Server-sent events pushing each changed board** — prompter, at the cost of one held connection and a broadcast per browser.
3. **A WebSocket** — needs a dependency outside the approved sources, or a hand-written protocol at a trust boundary.

Where the shown Placement comes from:

4. **One observer per Interactive run, polling the held cluster; every request reads its latest board** — API load independent of the browsers open.
5. **One observation per page request** — API load grows with the browsers and their tabs.
6. **The Timeline's watch streams** — no polling, but the record stops with the Steps and a board needs the declared Topology besides.

Where the server listens:

7. **The loopback address, on a port the operating system chooses** — the API server's own exposure; two invocations never collide.
8. **Every interface, or an address flag** — a decision the PRD keeps open.

How the board is drawn:

9. **A server-rendered, escaped fragment of nested boxes, tested in Go** — no framework, no browser automation, one render shared by every browser.
10. **Data sent to the page, drawn by its script or a framework** — the drawing logic lands where the Go tests cannot reach it, and a framework is an asset to vet or load.

How a newcomer's mark fades:

11. **The Live view counts each marked Pod's observations and steps the mark down** — asserted in Go, though the fade moves in steps of one observation.
12. **The page patches boxes by id and keeps a running animation** — a smooth fade, but hand-written diffing in the page, untested.

## Decision

Options 2, 4, 7, 9 and 11. The Live view is a `net/http` server in the binary, its page, script and stylesheet embedded. It serves reads alone, the board as a stream of Server-Sent Events. A request with another method or path, a Host other than the address served, or another Origin is refused. Server timeouts bound every connection and every event write. Streams beyond a fixed cap are refused, and stopping the lab ends every stream, then shuts the server down within a bound.

One observer goroutine per Interactive run observes the held cluster at a fixed interval, from provisioning until removal. Each observation builds the Placement board against the Scenario's declared Topology, so the Live view and the report share one model. The board gives each Node's readiness, its capacity, the observed Requests placed on it and the room left, by Pod count, cpu and memory, and each Pod's Request. Each observation renders one `html/template` fragment of nested boxes, pushed to every open stream when it differs from the last. A stream opens on the latest fragment, or the cause the last observation failed.

The terminal names the address once the server listens, before provisioning starts.

## Consequences

No dependency enters the module graph, and the Live view adds no Kubernetes import outside `internal/cluster`.

A change reaches the page within one observer interval, coarser than the watch streams. Each open browser holds one connection, bounded by the cap. A browser falling behind receives the latest fragment and skips the ones between. The fragment is well-formed markup, so Go tests decode it with `encoding/xml` rather than drive a browser.

A newcomer's mark fades in steps of one observation, never smoothly.

Another machine cannot reach the Live view. Widening that is a PRD decision, then a change to this ADR. The Host check keeps a page from another site, rebound to loopback, from reading the Live view. The Origin check keeps a page from another site from holding its streams open.

Reading is the server's whole surface. Injecting a Failure from the page adds the first state-changing request, needing its own guard against cross-site requests.

## Implementation

**Requirements:** REQ-SIM-022

**Files:** `internal/live/`, `internal/lab/runner.go`, `internal/scenario/placement_board.go`, `internal/cluster/simulated_cluster.go`, `cmd/lab/main.go`, `.golangci.yml`

## References

- [system-design.md § Contracts](../system-design.md)
- [system-design.md § Threat Model](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
