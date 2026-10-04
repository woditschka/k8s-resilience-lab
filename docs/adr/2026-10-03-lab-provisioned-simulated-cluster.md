# The lab provisions the Simulated Cluster it drives

**Status:** Accepted

## Context

Two statements have to hold together. The lab reaches its cluster one way, so the unattended run and the live view share one code path. The lab also never runs against a real or pre-existing cluster, including one reached through a supplied address (NG-1).

A Kubernetes client is worse than neutral by default: `client-go` reads a kubeconfig or an in-cluster service-account token, so a misconfigured run could drive a production cluster while reporting success. A Scenario is a function in the binary rather than a test, so the cluster's supplier has to live in the binary too.

Starting the container means reaching a container engine. Each engine publishes its API on a unix socket at its own path, and an invocation with no arguments has to find one.

## Options Considered

1. **An address flag or environment variable** — one code path, and a value an operator can point at any cluster.
2. **The engine's command-line tool through `os/exec`** — endpoint resolution delegated to that tool. The cost is a child process per call, a text format to parse, and an argument vector carrying lab values.
3. **testcontainers-go or the Docker SDK inside the binary** — the same lifecycle, plus a container library and its client tree in the shipped program.
4. **The engine's HTTP API over its unix socket, with the standard library** — `net/http` on a socket dialer, JSON in and out, no child process and no dependency.

## Decision

Option 4. The container lifecycle lives in the binary, which is the one place where a single code path survives, and the engine's HTTP API needs nothing beyond the standard library.

One detection decides the endpoint at startup, and that endpoint serves the whole run. `DOCKER_HOST` is tried first, and a `unix://` path that answers is the endpoint. A value of any other scheme is skipped without a dial, and a `unix://` path that stays silent is skipped after its probe. Either skip warns, naming the value and the reason, and detection goes on to the fixed candidate list, probed in order until a socket answers. The list covers the conventional path, rootless Linux, and the sockets Docker Desktop, Rancher Desktop and Colima publish under the home directory.

A home-relative candidate is composed with `os.UserHomeDir` and dialed as a socket. No file under the home directory is read, so the denied Kubernetes home-directory helper and the kubeconfig discovery it begins stay out of the lab.

The run prints the socket it resolved and the engine version that answered. When nothing answers, the run stops and names every path it tried with why each failed, a skipped `DOCKER_HOST` among them.

`Provision` creates the container from the composed image reference and publishes the API port on the loopback address, with a host port the engine chooses. It pulls that reference only when creation reports the image missing. It waits for the API server's readiness probe, then returns a handle whose close removes the container. The container name names the Scenario it serves and the invocation that created it, which makes a leaked container identifiable, per [ADR: Each Scenario provisions its own Simulated Cluster](2026-10-04-cluster-per-scenario.md).

The engine surface is restricted by construction rather than by discipline. Its exported methods take a context and a validated Kubernetes version, so no caller supplies an image reference or a container identifier. The image repository is a constant and any version tag of it is allowed. A value that is not a version is refused before a request, per [ADR: Kubernetes version pin](2026-10-03-kubernetes-version-pin.md). Pulling, creating and starting live in one method. Inspect, stop and remove are methods on the handle the run created, which holds the identifier unexported. The package declares no list call and no by-identifier call, and a surface test asserts that from its own declarations.

The client is a `rest.Config` carrying that address and no credential. No kubeconfig is read, no `KUBERNETES_*` variable is consulted, and no in-cluster token is discovered. A depguard rule denies `k8s.io/client-go/tools/clientcmd` and the other discovery imports across the module. In-cluster configuration is a call in the same package the provisioned-address client needs, so a `forbidigo` pattern denies it instead. Together the two rules keep the discovery paths from returning by accident. Neither the lab's one flag nor its one variable carries a cluster address.

## Consequences

NG-1 is enforced by absence: no cluster address to supply, no credential to use, and no code path that reads either. `DOCKER_HOST` selects which engine on this machine starts the container, and the Kubernetes client is built from that container's published port alone. The lab holds no secret, so none can leak into a log, an error, or a process argument.

Only a `unix://` path is ever dialed, so the engine is a socket file on this machine whatever `DOCKER_HOST` names, and no run reaches an engine across a network.

Endpoint resolution is the lab's own code. A machine whose engine answers off the conventional path is covered by the candidate list or by `DOCKER_HOST`, and by nothing else (the PRD's edge case 2). A stale or dead `DOCKER_HOST` beside a live engine costs one probe and falls through to the candidate list, which is the PRD's edge case 3. REQ-SIM-002 names the missing runtime when no socket answers.

The socket is full control of this machine's containers, and the engine offers no narrower grant. The lab's own surface is the narrowing: one image repository, any version tag of it, and the one container this run created. Another container on the machine is unreachable through the lab, because no call can name one.

An unauthenticated API server exists for the lifetime of a run, confined to the loopback interface and removed on close.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § Security Context](../system-design.md)
- [ADR: Non-goal — real-cluster runs](2026-10-03-non-goal-real-cluster-runs.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
