# The Live view is served on loopback from one shared observation

**Status:** Accepted

## Context

The owner asked to "start a webserver and visuzlize it in a browser", to "see how zones, nodes and pods are distributes" (REQ-SIM-022). An Interactive run holds one Simulated Cluster, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md). The owner asked for "zones, nodes and pods as boxes contained within eachoter with important infrmation e.g. capacity (total/left) labels,... clearly visible". The owner also asked that "the status should constantly update pushed from the server from the k8s cluster state". The owner added that "the cpu and memory requests of pods are important in my view to see how full the node is".

This is the lab's first inbound network surface; until now it only dialed out. The PRD leaves open whether another machine may reach it.

The owner then asked that "clicking on a pod should kill the pod, clicking on a node shoul take the node offline same for a zone" (REQ-SIM-028). The owner chose a "Plain single click". A lost Pod is gone, and its Deployment replaces it. Each click is the lab's first request that changes the cluster.

The lab already reads the whole Placement in one observation for each Wait's Placement board, which holds what the Live view shows.

## Options Considered

How the page learns of a change:

1. **The page fetches a server-rendered fragment at a fixed interval** — no connection held, and a change waits for the next fetch.
2. **Server-Sent Events pushing each changed board** — the push the owner asked for, at one held connection per browser.
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

How a click reaches the cluster:

13. **The page's script posts the clicked box's Failure; the write passes the Host and Origin checks and the standard library's cross-origin protection** — no state in the page, and the guard is a maintained one.
14. **The same, plus a per-run token embedded in the page** — a second guard, but a secret to render and compare, where every current browser sends Sec-Fetch-Site.
15. **A plain form post** — no script, but the page's policy forbids form targets, and a form reloads the page.

What a click on a Zone means, its Nodes filling its box:

16. **The innermost box carrying a Failure takes the click** — a Zone takes its name line and margin, a Node its box outside its Pods.
17. **A control of its own on each box** — an unambiguous target, but a second element per box the owner did not ask for.

How a Pod is lost:

18. **A delete with no grace period** — the Pod is gone at once, on a stopped Node too, and the ReplicaSet replaces it.
19. **A graceful delete** — no kubelet confirms a simulated Pod stopped, so on a stopped Node it stays Terminating.
20. **The eviction API** — honours the disruption budgets a Pod Failure ignores, and is refused where one forbids it.

## Decision

Options 2, 4, 7, 9, 11, 13, 16 and 18. The Live view is a `net/http` server in the binary, its page, script and stylesheet embedded. The page is rendered once at listening, naming the held Scenario. It serves reads, the board as a stream of Server-Sent Events, and one write: a POST naming a Node, a Zone or a Pod. The write reads no body and passes `http.CrossOriginProtection`. A request with another method or path, a Host other than the address served, or another Origin is refused. Server timeouts bound every connection and every event write. Streams beyond a fixed cap are refused, and stopping the lab ends every stream, then shuts the server down within a bound.

One observer goroutine per Interactive run observes the held cluster at a fixed interval, from provisioning until removal. Each observation builds the Placement board against the Scenario's declared Topology, so the Live view and the report share one model. The board gives each Node's readiness, its capacity, the observed Requests placed on it and the room left, by Pod count, cpu and memory, and each Pod's Request. Each observation renders one `html/template` fragment of nested boxes, pushed to every open stream when it differs from the last. A stream opens on the latest fragment, or the cause the last observation failed.

The terminal names the address once the server listens, before provisioning starts.

The held run takes each clicked Failure once its Steps up to the first When have reported, one at a time. It resolves the name against the declared Topology or the latest board, and prints the When naming what resolved. A click on a stopped Node, or on a Zone whose every Node stopped, makes it respond again, the held run alone knowing which Nodes it stopped. A Pod Failure deletes its Pod with no grace period.

## Consequences

No dependency enters the module graph, and the Live view adds no Kubernetes import outside `internal/cluster`.

A change reaches the page within one observer interval, coarser than the watch streams. Each open browser holds one connection, bounded by the cap. A browser falling behind receives the latest fragment and skips the ones between. The fragment is well-formed markup, so Go tests decode it with `encoding/xml` rather than drive a browser.

A newcomer's mark fades in steps of one observation, never smoothly.

Another machine cannot reach the Live view. Widening that is a PRD decision, then a change to this ADR. The Host check keeps a page from another site, rebound to loopback, from reading the Live view. The Origin check keeps a page from another site from holding its streams open.

The write is the one state-changing request. A cross-site browser request is refused by its Sec-Fetch-Site or Origin, and a rebound name by its Host. A process on the machine can still inject a Failure, as it can reach the unauthenticated API server. A name the lab does not hold changes nothing, and only a resolved name reaches the terminal.

A Pod Failure bypasses any disruption budget and any graceful stop, as a crashed machine would. A click on a Zone's Nodes never reaches the Zone, so the Zone's name line is its visible target.

## Implementation

**Requirements:** REQ-SIM-022, REQ-SIM-028

**Files:** `internal/live/`, `internal/lab/runner.go`, `internal/scenario/placement_board.go`, `internal/scenario/failure.go`, `internal/cluster/simulated_cluster.go`, `cmd/lab/main.go`, `.golangci.yml`

## References

- [system-design.md § Contracts](../system-design.md)
- [system-design.md § Threat Model](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
