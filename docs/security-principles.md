<!-- harness: 2026-09-30 -->
# Security Principles

This brief specializes the harness's non-negotiable security laws for this project. The four laws — security as an emergent property, defense in depth, least privilege, fail secure — are harness-owned: a project decides *how* it meets them, never *whether*. This document carries that "how": the project's trust-boundary map and the stack's state-of-the-art security defaults. The feature-implementer designs against it; the security-reviewer enforces it; the exhaustive item-by-item checklist lives in the `security-checks` skill.

## Trust Boundaries

A trust boundary is any point where data or control crosses from less-trusted to more-trusted. Every boundary the change introduces or crosses gets the same treatment.

| At the boundary | The rule |
|---|---|
| External input arrives (request, file, env, message) | Validate type, range, and shape before use; reject what the contract does not allow |
| A secret is read (token, key, password) | It never reaches a log, an error message, a URL, or a process argument |
| A secret is stored | It lives in the environment or a secret store, never in committed source |
| A privilege is exercised (file, network, process, query) | Grant the minimum scope; deny by default |
| An error crosses back out | The message carries debugging context inward, never internal detail outward |

Internal code, past the boundary, trusts its contracts — defensive checks belong at the boundary, not scattered through the core.

## Language Realization

The classes below derive from the dependency policy and threat model in `docs/system-design.md`. These defaults are the bar a change starts from, not a floor to argue down. The `security-checks` skill holds the exhaustive checklist.

| Vulnerability class | Principle at risk | Control |
|---|---|---|
| Ambient Kubernetes credential discovery | Least privilege, fail secure | The client is a `rest.Config` carrying the provisioned loopback address and no credential. No kubeconfig, `KUBERNETES_*` variable, or service-account token is read. There is no address flag and no address variable, so no run can be pointed elsewhere. A depguard rule denies the kubeconfig, home-directory and credential-plugin imports; a type-resolved `forbidigo` rule denies the in-cluster configuration call. No kubeconfig, cloud-credential or service-account provider is an approved dependency. |
| Unpinned or moved container image tag | Defense in depth | The image repository and the KWOK release are constants no input reaches. The Kubernetes version is the one variable part: a pinned default, or the `--k8s-version` value, validated before any request and printed by every run. Changing the default is a reviewed one-line change. |
| Unauthenticated plain-HTTP control plane reachable off-host | Least privilege | The Simulated Cluster publishes its API port on the loopback address, on a host port the engine chooses. It holds no real workload and no credential, and its container is removed when its Scenario's handle closes. |
| Control of every container through the engine socket | Least privilege | The lab starts no external process. It reaches the container engine over its HTTP API on a local unix socket, and a `DOCKER_HOST` with any other scheme is skipped without a dial. The engine client pulls, creates and starts a KWOK cluster container alone, and inspects, stops and removes only the container its own handle created. It declares no list call and no call taking an image reference or a container identifier. The engine-surface test asserts that surface. |
| One run or Scenario removing another's Simulated Cluster | Least privilege | Each Scenario provisions its own cluster container, named for the Scenario and the invocation, on a port the engine chooses. Removal acts on the handle's own container alone. |
| Untyped or unvalidated input at the external-input boundary | Validate at the boundary | The external input is the engine's responses, `DOCKER_HOST`, and the command line a run is invoked with. Each is parsed at the boundary into an immutable typed value, by a constructor returning an error on what the contract disallows. Engine responses are decoded into unexported types inside `internal/cluster`, and no engine type crosses that boundary. Scenario descriptions are compiled into the binary, so none is parsed at run time. |
| A discarded error leaving a run on an unusable value | Fail secure | Every error is returned wrapped with context, never discarded. Provisioning returns an error in place of a handle, so a failed start is never driven. `errorlint` is enabled in the lint configuration. |
| An untrusted or prohibited module entering the build | Defense in depth | The module graph holds the approved sources alone, and `go mod tidy -diff` fails the gate on an undeclared or stale requirement. `make deps-check` fails and names the offender for a prohibited class. Each dependency's checksum is committed in `go.sum` and verified by the toolchain on every build. |

## Standing Gaps

Checks the project does not run, and why. A reviewer states a listed gap in one clause and raises nothing for it. A gap not listed here is raised once, as a `clarify` finding to the owner, and then listed.

- **Image digest pinning** is not in force. The cluster image is pinned by tag only.
- **Dependency vulnerability scanning** does not run. `govulncheck` is absent from the agent runtime, and the gate command does not invoke it.
- **Container image scanning** does not run. The gate command invokes no image scanner.
- **Static security analysis** does not run. `gosec` is outside the enabled linter set.
