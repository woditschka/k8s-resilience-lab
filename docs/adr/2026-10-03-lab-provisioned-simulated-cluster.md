# The lab provisions the Simulated Cluster it drives

**Status:** Accepted, amended 2026-10-04: each Scenario provisions its own Simulated Cluster, option 7 (REQ-SIM-001); amended 2026-10-09: Scenarios run at once, option 9 (REQ-SIM-014); amended 2026-10-21: an Interactive run holds one Simulated Cluster, option 11 (REQ-SIM-022)

## Context

Two statements have to hold together. The lab reaches its cluster one way, so the unattended run and the live view share one code path. The lab also never runs against a real or pre-existing cluster, including one reached through a supplied address (NG-1).

A Kubernetes client is worse than neutral by default: `client-go` reads a kubeconfig or an in-cluster service-account token, so a misconfigured run could drive a production cluster while reporting success. A Scenario is a function in the binary rather than a test, so the cluster's supplier has to live in the binary too.

Starting the container means reaching a container engine. Each engine publishes its API on a unix socket at its own path, and an invocation with no arguments has to find one.

On 2026-10-04 the question became how many clusters a run holds. A Scenario's asserted Placement is only trustworthy when nothing from another Scenario can reach it: a leftover Node, a Pod still terminating, or a Scenario that failed halfway changes the next assertion. One named Scenario run on its own (REQ-SIM-003) has to produce the outcome the full run reports for it. Two invocations share one machine, a developer's run beside a build job, and neither may disturb the other.

On 2026-10-09 the Scenarios stopped running one after another. Each spends 7 to 17 seconds inside its cluster, yet about a minute of provisioning and removal surrounds each one. In sequence the shipped Scenarios cost a run several minutes, and the command tests about 15 minutes (REQ-SIM-014).

On 2026-10-21 the owner asked for an Interactive run whose cluster stays up to watch: "only the first setup or the full scenario then the rest stays interactive". A reset or another pick replaces the held cluster, answered "2 exactly": one cluster at a time, removed when the lab stops (REQ-SIM-022).

## Options Considered

1. **An address flag or environment variable** — one code path, and a value an operator can point at any cluster.
2. **The engine's command-line tool through `os/exec`** — endpoint resolution delegated to that tool. The cost is a child process per call, a text format to parse, and an argument vector carrying lab values.
3. **testcontainers-go or the Docker SDK inside the binary** — the same lifecycle, plus a container library and its client tree in the shipped program.
4. **The engine's HTTP API over its unix socket, with the standard library** — `net/http` on a socket dialer, JSON in and out, no child process and no dependency.

How many clusters a run holds, with option 4 in force:

5. **One Simulated Cluster per run, cleaned between Scenarios** — one container start per run, and cleanliness that depends on the cleanup code being complete.
6. **One long-lived Simulated Cluster reused across runs** — the fastest, and every outcome depends on what ran before.
7. **One Simulated Cluster per Scenario** — a container start per Scenario, and isolation that depends on nothing.

How a run schedules its Scenarios, with option 7 in force:

8. **One after another** — one container at a time, and a run as long as the sum of its Scenarios.
9. **All at once, each in its own goroutine** — one container per Scenario at once, and a run as long as its slowest Scenario.

How long an Interactive run keeps its cluster, with option 7 in force:

10. **Removed when its Steps end, as in a run** — nothing left to watch.
11. **Held until the engineer stops the lab, one at a time** — the same handle, closed later.

## Decision

Option 4. The container lifecycle lives in the binary, which is the one place where a single code path survives, and the engine's HTTP API needs nothing beyond the standard library.

One detection decides the endpoint at startup, and that endpoint serves the whole run. `DOCKER_HOST` is tried first, and a `unix://` path that answers is the endpoint. A value of any other scheme is skipped without a dial, and a `unix://` path that stays silent is skipped after its probe. Either skip warns, naming the value and the reason, and detection goes on to the fixed candidate list, probed in order until a socket answers. The list covers the conventional path, rootless Linux, and the sockets Docker Desktop, Rancher Desktop and Colima publish under the home directory.

A home-relative candidate is composed with `os.UserHomeDir` and dialed as a socket. No file under the home directory is read, so the denied Kubernetes home-directory helper and the kubeconfig discovery it begins stay out of the lab.

The run prints the socket it resolved and the engine version that answered. When nothing answers, the run stops and names every path it tried with why each failed, a skipped `DOCKER_HOST` among them.

`Provision` creates the container from the composed image reference and publishes the API port on the loopback address, with a host port the engine chooses. It pulls that reference only when creation reports the image missing. It waits for the API server's readiness probe, then returns a handle whose close removes the container. The container name carries the Scenario it serves and a random suffix drawn per provisioning. Two clusters of one run, of two concurrent tests, or of two invocations therefore never share a name or a port, and a leaked container is identifiable.

The engine surface is restricted by construction rather than by discipline. Its exported methods take a context and a validated Kubernetes version, so no caller supplies an image reference or a container identifier. The image repository is a constant and any version tag of it is allowed. A value that is not a version is refused before a request, per [ADR: Kubernetes version pin](2026-10-03-kubernetes-version-pin.md). Creating and starting live in one method, and pulling in one helper that `Provision` and the obtaining call both use; the obtaining call takes the version alone. Inspect, stop and remove are methods on the handle the run created, which holds the identifier unexported. The package declares no list call and no by-identifier call, and a surface test asserts that from its own declarations.

The client is a `rest.Config` carrying that address and no credential. No kubeconfig is read, no `KUBERNETES_*` variable is consulted, and no in-cluster token is discovered. A depguard rule denies `k8s.io/client-go/tools/clientcmd` and the other discovery imports across the module. In-cluster configuration is a call in the same package the provisioned-address client needs, so a `forbidigo` pattern denies it instead. Together the two rules keep the discovery paths from returning by accident. Neither the lab's one flag nor its one variable carries a cluster address.

Option 7, since 2026-10-04. A Scenario runs on a Simulated Cluster provisioned for it alone, removed when the Scenario ends, including when it fails or panics. The runner holds the handle and closes it, so a Scenario's own code carries no teardown.

Option 9, since 2026-10-09. No package holds mutable process-wide state: the resolved endpoint is an immutable value passed into each provisioning call, so the change is the runner's alone. The runner derives every Scenario's run name before it provisions any cluster, so a name it cannot derive stops the run before a cluster exists. It then obtains the version's cluster image once, before any Scenario starts. A version the lab cannot obtain stops the run once, naming it, and no Scenario starts (REQ-SIM-001 edge case 4). The runner joins every goroutine before returning and hands back the Outcomes in declared order, the order the report prints them in.

One Scenario's failure is its own. A cluster the engine cannot start, a Step left undone, a Finding or a panic ends that Scenario alone. The runner recovers a panic into that Scenario's own error, the lab's failure rather than a Step's Cause. A panic left to cross a goroutine would end the process before the other Scenarios' removals run, leaking their containers.

The run bounds nothing: it holds as many clusters as Scenarios are shipped. A machine short of room for them is an Open Question of the PRD. The lab's own tests bound how many run at once per package through the gate's `-parallel` flag.

Option 11, since 2026-10-21. An Interactive run carries out one Scenario on a cluster of its own, through the same provisioning, recording and Steps as a run. It stops the recording and files the Timeline when its Steps end, then holds the cluster. The handle closes when the engineer stops the lab, the Steps stopped first if still running. A run is unchanged: it removes each cluster when its Scenario ends.

## Consequences

NG-1 is enforced by absence: no cluster address to supply, no credential to use, and no code path that reads either. `DOCKER_HOST` selects which engine on this machine starts the container, and the Kubernetes client is built from that container's published port alone. The lab holds no secret, so none can leak into a log, an error, or a process argument.

Only a `unix://` path is ever dialed, so the engine is a socket file on this machine whatever `DOCKER_HOST` names, and no run reaches an engine across a network.

Endpoint resolution is the lab's own code. A machine whose engine answers off the conventional path is covered by the candidate list or by `DOCKER_HOST`, and by nothing else (the PRD's edge case 2). A stale or dead `DOCKER_HOST` beside a live engine costs one probe and falls through to the candidate list, which is the PRD's edge case 3. REQ-SIM-002 names the missing runtime when no socket answers.

The socket is full control of this machine's containers, and the engine offers no narrower grant. The lab's own surface is the narrowing: one image repository, any version tag of it, and the containers this run created. Another container on the machine is unreachable through the lab, because no call can name one. Two invocations on one machine therefore cannot collide.

An unauthenticated API server exists for each Scenario's lifetime, confined to the loopback interface and removed on close.

One named Scenario reports what the full run reports for it, because neither has a predecessor.

A run of N Scenarios pays N container starts at once, so it lasts about as long as its slowest Scenario. Isolation is worth that container count at the sizes a lab runs. A run's footprint is one container per Scenario shipped, which grows with the Scenarios; before 2026-10-09 it was one container at a time. Should a machine lack the room, the PRD's Open Question holds the remedy: a run one at a time.

The gate's footprint is each cluster-backed package's parallel tests times the clusters each holds, across the packages `go test` runs at once. Command tests judging a run of every Scenario share one run, which keeps that product small.

A missing image is pulled once per run, before the fan-out, within the pull timeout. The Scenarios of a run never pull at once, and a run's first Scenario starts only after that pull.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-014, REQ-SIM-022

**Files:** `internal/cluster/runtime.go`, `internal/cluster/simulated_cluster.go`, `internal/lab/runner.go`, `cmd/lab/main.go`, `cmd/lab/main_test.go`, `Makefile`

## References

- [system-design.md § Security Context](../system-design.md)
- [system-design.md § State Machine](../system-design.md)
- [ADR: Non-goal — real-cluster runs](2026-10-03-non-goal-real-cluster-runs.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
