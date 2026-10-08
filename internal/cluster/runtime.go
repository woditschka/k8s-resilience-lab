package cluster

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dockerHostVariable is the one environment variable the lab reads to find the
// engine. Only a unix path in it is ever dialed.
const dockerHostVariable = "DOCKER_HOST"

// unixScheme is the one DOCKER_HOST scheme the lab dials: an engine on this
// machine, reached over a socket file. A tcp:// or ssh:// value could name an
// engine on another machine, which the lab never drives
// (docs/system-design.md § Threat Model).
const unixScheme = "unix"

// homePrefix marks a candidate the invoking user's home directory is composed
// into. The home directory is used to build a socket path and nothing else: no
// file under it is read, per
// docs/adr/2026-10-03-lab-provisioned-simulated-cluster.md.
const homePrefix = "~/"

// socketCandidates are the engine sockets probed, in order, when DOCKER_HOST does
// not resolve the engine. The first that answers is the engine for the whole run.
var socketCandidates = []string{
	"/var/run/docker.sock",
	homePrefix + ".rd/docker.sock",
	homePrefix + ".docker/run/docker.sock",
	homePrefix + ".colima/default/docker.sock",
	"/var/run/podman/podman.sock",
}

// probeTimeout is how long one candidate socket has to answer before the next is
// tried, so an unresponsive socket cannot hold up a run.
const probeTimeout = 2 * time.Second

// engineHost is the host an engine request is addressed to. The socket decides
// which engine answers, so the host is a placeholder the dialer ignores.
const engineHost = "engine"

// engineTimeout is the budget one engine call runs within. Every call the lab
// makes is a round trip over container metadata on a socket on this machine, so a
// call still unanswered after this has wedged. Without a deadline of its own such
// a call holds up the run indefinitely, and the removal each Scenario defers
// never runs — which leaves an unauthenticated API server on the machine
// (docs/system-design.md § Threat Model).
const engineTimeout = 15 * time.Second

// idleConnectionTimeout is how long a connection to the engine is kept for a
// later call before it is closed. The polling loops make hundreds of calls over
// one socket, so they reuse one client; this is what stops a connection that is
// no longer wanted from being held for the rest of the run.
const idleConnectionTimeout = 30 * time.Second

// Endpoint is the engine this run drives: the unix socket that answered, the
// version it reported, and the client every call to it goes through. It is an
// immutable value resolved once and passed into every later engine call, so no
// package holds mutable process-wide state.
//
// The client belongs to the endpoint because the socket is fixed at detection
// time. One client per endpoint is what makes a polling loop cost one connection
// rather than one per poll, each with the goroutines that keep it alive.
type Endpoint struct {
	socket  string
	version string
	client  *http.Client
}

// Socket is the path of the unix socket this endpoint reaches the engine on.
func (e Endpoint) Socket() string {
	return e.socket
}

// EngineVersion is the version the engine on this socket reported.
func (e Endpoint) EngineVersion() string {
	return e.version
}

// Close releases what this endpoint holds open to the engine. The whole run
// shares one client per resolved socket, so the run is what closes it.
func (e Endpoint) Close() {
	if e.client != nil {
		e.client.CloseIdleConnections()
	}
}

// DetectEngine resolves the container engine once for the whole run: DOCKER_HOST
// first, then the candidate sockets in order. Warnings explain any value that was
// skipped or did not answer; the error names every path tried and why each failed.
func DetectEngine(ctx context.Context, warnings *slog.Logger) (Endpoint, error) {
	return detectEngine(ctx, warnings, os.Getenv(dockerHostVariable), socketCandidates)
}

// detectEngine is DetectEngine over a stated DOCKER_HOST value and candidate
// list, which is what lets a test drive detection without changing the process's
// environment.
func detectEngine(ctx context.Context, warnings *slog.Logger, dockerHost string, candidates []string) (Endpoint, error) {
	named, triedVariable, resolved := engineFromDockerHost(ctx, warnings, dockerHost)
	if resolved {
		return named, nil
	}

	probed, triedCandidates, resolved := engineFromCandidates(ctx, candidates)
	if resolved {
		return probed, nil
	}

	return Endpoint{}, fmt.Errorf("no container engine answered; tried %s", strings.Join(append(triedVariable, triedCandidates...), "; "))
}

// engineFromDockerHost resolves the engine from the DOCKER_HOST value. When it
// resolves none, it reports what was tried and why, so the run's final error can
// name it; an unset value was never a candidate and is reported as nothing tried.
//
// A value whose scheme is not unix is skipped with a warning and never dialed: it
// could name an engine on another machine, which the lab does not drive
// (docs/system-design.md § Threat Model).
//
// The value is parsed once here and reported in its redacted form, or by its
// variable name alone when it does not parse. The schemes the lab declines are the
// ones that carry a user and a password, and a skipped value reaches both a warning
// and the run's final error, which a reader pastes into an issue or a CI log
// (docs/security-principles.md § Trust Boundaries).
func engineFromDockerHost(ctx context.Context, warnings *slog.Logger, dockerHost string) (Endpoint, []string, bool) {
	if dockerHost == "" {
		return Endpoint{}, nil, false
	}

	parsed, unparsable := url.Parse(dockerHost)
	if unparsable != nil {
		// An unparsable value has no redacted form, so it is named by its variable
		// alone and its cause is the fixed sentence below.
		warnings.WarnContext(ctx, "the "+dockerHostVariable+" value is not a URL, so the socket candidates are probed instead",
			slog.String("reason", unparsableCause))

		return Endpoint{}, []string{fmt.Sprintf("%s (it is not a URL: %s)", dockerHostVariable, unparsableCause)}, false
	}

	socket, unusable := unixSocketIn(parsed)
	if unusable != nil {
		warnings.WarnContext(ctx, "the "+dockerHostVariable+" value does not name an engine on this machine, so the socket candidates are probed instead",
			slog.String("value", parsed.Redacted()), slog.String("reason", unusable.Error()))

		return Endpoint{}, []string{fmt.Sprintf("%s=%s (%v)", dockerHostVariable, parsed.Redacted(), unusable)}, false
	}

	endpoint, err := probe(ctx, socket)
	if err == nil {
		return endpoint, nil, true
	}
	warnings.WarnContext(ctx, "the socket named by "+dockerHostVariable+" did not answer, so the socket candidates are probed instead",
		slog.String("socket", socket), slog.String("reason", err.Error()))

	return Endpoint{}, []string{fmt.Sprintf("%s=%s (%v)", dockerHostVariable, socket, err)}, false
}

// unparsableCause is the cause reported for a DOCKER_HOST value that does not
// parse, and it is a fixed sentence because no part of such a value may be
// reported. url.Parse wraps the cause in a *url.Error whose message quotes the
// raw input, and net/url's own inner causes quote a fragment of it in their turn:
// a bad percent escape quotes three bytes of the value, an invalid host one byte,
// and a bad port the port it read. A DOCKER_HOST can carry a password, which
// never reaches a log or an error, so the cause is this sentence whatever the
// parser reports (docs/security-principles.md § Trust Boundaries).
const unparsableCause = "the value does not parse as a URL"

// engineFromCandidates probes the candidates in order and resolves the engine
// from the first that answers, or reports every path tried and why each failed.
func engineFromCandidates(ctx context.Context, candidates []string) (Endpoint, []string, bool) {
	tried := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		socket, unresolvable := resolveCandidate(candidate)
		if unresolvable != nil {
			tried = append(tried, fmt.Sprintf("%s (%v)", candidate, unresolvable))
			continue
		}

		endpoint, err := probe(ctx, socket)
		if err == nil {
			return endpoint, nil, true
		}
		tried = append(tried, fmt.Sprintf("%s (%v)", socket, err))
	}

	return Endpoint{}, tried, false
}

// unixSocketIn is the socket path a parsed DOCKER_HOST value names, or the reason
// the value names no engine this lab may drive. The value reaches here as a URL
// its caller parsed and is never interpolated into a command.
func unixSocketIn(parsed *url.URL) (string, error) {
	if parsed.Scheme != unixScheme {
		return "", fmt.Errorf("its scheme is %q and the lab dials a %q socket alone, which is an engine on this machine", parsed.Scheme, unixScheme)
	}
	if parsed.Path == "" {
		return "", errors.New("it names no socket path")
	}

	return parsed.Path, nil
}

// resolveCandidate is the socket path a candidate names, composing the invoking
// user's home directory into a home-relative one.
func resolveCandidate(candidate string) (string, error) {
	if !strings.HasPrefix(candidate, homePrefix) {
		return candidate, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate the home directory this candidate is relative to: %w", err)
	}

	return filepath.Join(home, strings.TrimPrefix(candidate, homePrefix)), nil
}

// probe resolves the engine on socket by asking it for its version, which is both
// the liveness check and the version a run reports. A socket that does not
// resolve leaves nothing open behind it.
func probe(ctx context.Context, socket string) (Endpoint, error) {
	candidate := Endpoint{socket: socket, client: engineClient(socket)}

	version, err := candidate.reportedVersion(ctx)
	if err != nil {
		candidate.Close()

		return Endpoint{}, err
	}
	candidate.version = version

	return candidate, nil
}

// reportedVersion is the version the engine on this endpoint names, within the
// probe timeout so an unresponsive socket cannot hold up a run.
func (e Endpoint) reportedVersion(ctx context.Context) (string, error) {
	probed, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var reported struct {
		Version string
	}
	if err := engineRequest(probed, e, http.MethodGet, "/version", nil, &reported); err != nil {
		return "", err
	}
	if reported.Version == "" {
		return "", errors.New("it answered without naming an engine version")
	}

	return reported.Version, nil
}

// engineRefusal is the engine's refusal of one request: the status code it
// answered with, and the sentence reporting it.
//
// The code is what tells one refusal from another. A container the engine no
// longer holds answers 404, which is what confirms a removal; every other code is
// an engine that is failing rather than a container that is gone, and reading
// the two alike would report a cluster removed while it still runs
// (docs/system-design.md § Threat Model).
type engineRefusal struct {
	code     int
	reported string
}

// Error is the sentence naming the call, the socket and what the engine said.
func (r engineRefusal) Error() string {
	return r.reported
}

// engineRequest makes one request to the engine this endpoint reaches and decodes
// its JSON answer into decoded, which may be nil when the answer carries nothing
// the lab reads. Responses are decoded into the lab's own unexported types, so no
// engine type crosses this package's boundary.
//
// Every call runs within the engine timeout, which is why no call site states one
// of its own; a caller needing a tighter budget sets it on the context it passes.
func engineRequest(ctx context.Context, endpoint Endpoint, method, path string, body io.Reader, decoded any) error {
	called, cancel := context.WithTimeout(ctx, engineTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(called, method, "http://"+engineHost+path, body)
	if err != nil {
		return fmt.Errorf("build the %s %s request: %w", method, path, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	answer, err := endpoint.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s over %s: %w", method, path, endpoint.socket, err)
	}
	defer func() { _ = answer.Body.Close() }()

	if answer.StatusCode >= http.StatusBadRequest {
		return engineRefusal{
			code: answer.StatusCode,
			reported: fmt.Sprintf("%s %s over %s: the engine answered %s: %s",
				method, path, endpoint.socket, answer.Status, engineMessageIn(answer.Body)),
		}
	}
	if decoded == nil {
		return nil
	}
	if err := json.NewDecoder(answer.Body).Decode(decoded); err != nil {
		return fmt.Errorf("decode the engine's answer to %s %s: %w", method, path, err)
	}

	return nil
}

// engineClient is a client that reaches the engine over one unix socket. The
// request's host is ignored by the dialer, so no network address is ever resolved.
//
// One of these serves a whole run. It is built here rather than per call because
// an abandoned transport's keep-alive connection is held open by goroutines of
// its own: a client per call would cost the polling loops one socket descriptor
// and its goroutines per poll.
func engineClient(socket string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			IdleConnTimeout: idleConnectionTimeout,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer

				return dialer.DialContext(ctx, unixScheme, socket)
			},
		},
	}
}

// engineMessageIn is the message the engine put in a failing answer, so an error
// carries what the engine itself said.
func engineMessageIn(body io.Reader) string {
	var answered struct {
		Message string
	}
	if err := json.NewDecoder(body).Decode(&answered); err != nil || answered.Message == "" {
		return "no message"
	}

	return answered.Message
}

// apiServerPort is the container port a KWOK control plane serves its API on. The
// engine publishes it on a host port of its own choosing, so two invocations on
// one machine cannot collide on a port
// (docs/adr/2026-10-04-cluster-per-scenario.md).
const apiServerPort = "8080/tcp"

// loopbackAddress is the only interface a Simulated Cluster's API port is
// published on. The API server is unauthenticated, so a published port on any
// other interface would offer it to the network
// (docs/system-design.md § Threat Model).
const loopbackAddress = "127.0.0.1"

// pullTimeout is the budget a pull of the pinned cluster image runs within. Only
// the first run of a Kubernetes version new to the machine pays it.
const pullTimeout = 10 * time.Minute

// startTimeout is how long provisioning waits for a Simulated Cluster's API
// server to answer its readiness probe.
const startTimeout = 2 * time.Minute

// removalTimeout is how long a removal has to take effect on the engine. The
// engine answers a removal as soon as it has accepted it, so the lab waits for
// the container to be gone rather than for the answer.
const removalTimeout = 30 * time.Second

// containerNamePrefix marks every container this lab creates, so a human reading
// the engine's own output can tell which containers came from a run.
const containerNamePrefix = "k8s-resilience-lab"

// invocationSuffixLength is how much of a random value a container name carries.
// It makes the name unique to this invocation, so two runs on one machine never
// name the same container (docs/adr/2026-10-04-cluster-per-scenario.md).
const invocationSuffixLength = 8

// NodeHeartbeatFactor is the factor the cluster's heartbeat timings run at under
// the lab, and DefaultNodeHeartbeatFactor the image's own factor that it
// replaces. They are the factors as the container's argument spells them, so the
// setting a run reports and the setting its container is created with cannot
// drift.
//
// The image scales every heartbeat duration by five — a Node lease of 200s, a
// monitor period of 25s and a grace period of 3m20s — which is longer than a
// Scenario waits for a Placement to settle. At one, the stock Kubernetes set is
// restored coherently, so a Node the lab reports unresponsive is noticed within
// one monitor period rather than after minutes.
const (
	NodeHeartbeatFactor        = "1"
	DefaultNodeHeartbeatFactor = "5"
)

// heartbeatFactorArgument puts the cluster's heartbeat timings back to the ones
// Kubernetes itself runs on.
const heartbeatFactorArgument = "--heartbeat-factor=" + NodeHeartbeatFactor

// NodeEvictionRate is how many unresponsive Nodes of one Zone the cluster taints
// per second under the lab, and DefaultNodeEvictionRate the rate Kubernetes
// itself runs at. They are the rates as the controller's argument spells them, so
// the setting a run reports and the setting its cluster is created with cannot
// drift.
//
// The node lifecycle controller paces this per Zone. At the stock rate a Zone's
// three Nodes were tainted ten seconds apart and the Placement settled 25s after
// the Failure, which is not the few seconds a Zone Failure is judged within; at
// ten per second the first Node was tainted within half a second and the other
// two at the next monitor pass, and the Placement settled 9s after the Failure
// (docs/system-design.md § Scale and Load). A single Node takes the limiter's
// burst token either way, so a Failure of one Node is unchanged.
const (
	NodeEvictionRate        = "10"
	DefaultNodeEvictionRate = "0.1"
)

// evictionRateArgument lifts the pace the control plane taints an unresponsive
// Zone's Nodes at, so what a Scenario waits for is the monitor period rather than
// a rate limiter.
const evictionRateArgument = "--extra-args=kube-controller-manager=node-eviction-rate=" + NodeEvictionRate

// manageAllNodesArgument and manageMarkedNodesArgument are what make the KWOK
// controller maintain the Nodes carrying the lab's marking alone. The image
// starts it managing every Node, which is why switching that off is not
// optional: managing all either beats a selector or is refused beside one, and
// an extra argument is appended after kwokctl's own, where a repeated boolean
// flag takes the last value.
//
// The selector is composed from the same two constants a Node is created with
// (resources.go toNode), so the marking cannot drift between creation and
// selection. Taking that label off a Node is then how a Failure leaves it
// unmaintained, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md.
const (
	manageAllNodesArgument    = "--extra-args=kwok-controller=manage-all-nodes=false"
	manageMarkedNodesArgument = "--extra-args=kwok-controller=manage-nodes-with-label-selector=" + fakeNodeLabel + "=" + fakeNodeLabelValue
)

// stageCRDArgument enables KWOK's Stage CRD, so the KWOK controller applies the
// Stages served through the API, the lab's own set (stages.go), rather than the
// ones the image's configuration file holds. Its pod-ready Stage is what reads a
// Pod's Startup time, per
// docs/adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md.
const stageCRDArgument = "--enable-crds=Stage"

// clusterArguments are the arguments every Simulated Cluster's container is
// created with. The image's entrypoint forwards them verbatim to the command
// that creates the cluster inside it.
//
// They are package constants and nothing else. No run input reaches the image,
// the container or its arguments, which is the same property the pinned
// repository and the composed container name carry
// (docs/system-design.md § Threat Model).
var clusterArguments = []string{
	heartbeatFactorArgument,
	evictionRateArgument,
	manageAllNodesArgument,
	manageMarkedNodesArgument,
	stageCRDArgument,
}

// Engine is the restricted engine surface over one resolved unix socket, bound to
// the one container it may create and to nothing else on the machine.
//
// The surface is restricted by construction: its exported method names a
// Kubernetes version, never an image and never a container, and the name of the
// container it creates is composed here rather than supplied. The engine socket
// grants control of every container on the machine, and this type is how the lab
// declines all but its own (docs/system-design.md § Threat Model).
type Engine struct {
	endpoint      Endpoint
	containerName string
}

// newEngine binds the resolved engine to the one container a Scenario's cluster
// runs in.
func newEngine(endpoint Endpoint, scenarioName string) (Engine, error) {
	name, err := containerNameFor(scenarioName)
	if err != nil {
		return Engine{}, err
	}

	return Engine{endpoint: endpoint, containerName: name}, nil
}

// containerNameFor is the name of the container serving scenarioName in this
// invocation: the lab's own prefix, the Scenario it serves, and a suffix unique
// to the invocation.
func containerNameFor(scenarioName string) (string, error) {
	if scenarioName == "" {
		return "", errors.New("a Simulated Cluster's container carries the name of the Scenario it serves, so an unnamed Scenario is refused")
	}

	return fmt.Sprintf("%s-%s-%s", containerNamePrefix, scenarioName, strings.ToLower(rand.Text()[:invocationSuffixLength])), nil
}

// StartCluster pulls the pinned cluster image for version when the machine does
// not already have it, then creates and starts this Scenario's cluster container
// and returns the handle to it.
func (e Engine) StartCluster(ctx context.Context, version KubernetesVersion) (*ClusterContainer, error) {
	image := clusterImage(version)

	if err := e.pullWhenAbsent(ctx, image); err != nil {
		return nil, err
	}

	identifier, err := e.createContainer(ctx, image)
	if err != nil {
		return nil, err
	}

	started := &ClusterContainer{endpoint: e.endpoint, identifier: identifier, name: e.containerName}
	if err := e.startContainer(ctx, identifier); err != nil {
		return nil, errors.Join(err, started.Remove(ctx))
	}

	return started, nil
}

// pullWhenAbsent pulls image unless the engine already holds it, so a repeated run
// of the same Kubernetes version makes no registry request.
func (e Engine) pullWhenAbsent(ctx context.Context, image string) error {
	if err := engineRequest(ctx, e.endpoint, http.MethodGet, "/images/"+url.PathEscape(image)+"/json", nil, nil); err == nil {
		return nil
	}

	pulling, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()

	repository, tag, _ := strings.Cut(image, ":")
	query := url.Values{"fromImage": {repository}, "tag": {tag}}
	progress, err := engineStream(pulling, e.endpoint, http.MethodPost, "/images/create?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("pull the cluster image %s: %w", image, err)
	}
	defer func() { _ = progress.Close() }()

	// The engine answers a pull as a stream of progress messages and reports a
	// failure inside it rather than in the status. Each message is decoded and
	// discarded in turn, so a pull costs constant memory
	// (docs/system-design.md § Scale and Load).
	messages := json.NewDecoder(progress)
	for messages.More() {
		var message struct {
			Error string
		}
		if err := messages.Decode(&message); err != nil {
			return fmt.Errorf("read the progress of pulling %s: %w", image, err)
		}
		if message.Error != "" {
			return fmt.Errorf("pull the cluster image %s: %s", image, message.Error)
		}
	}

	return nil
}

// createContainer creates this Scenario's container from image, publishing the
// API server's port on a loopback host port the engine chooses and passing the
// arguments the cluster inside it runs with.
func (e Engine) createContainer(ctx context.Context, image string) (string, error) {
	requested, err := json.Marshal(map[string]any{
		"Image":        image,
		"Cmd":          clusterArguments,
		"ExposedPorts": map[string]any{apiServerPort: map[string]any{}},
		"HostConfig": map[string]any{
			"PortBindings": map[string]any{
				apiServerPort: []map[string]string{{"HostIp": loopbackAddress, "HostPort": ""}},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("compose the request creating %s: %w", e.containerName, err)
	}

	var created struct {
		ID string `json:"Id"`
	}
	query := url.Values{"name": {e.containerName}}
	if err := engineRequest(ctx, e.endpoint, http.MethodPost, "/containers/create?"+query.Encode(), bytes.NewReader(requested), &created); err != nil {
		return "", fmt.Errorf("create the Simulated Cluster %s: %w", e.containerName, err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("create the Simulated Cluster %s: the engine named no container", e.containerName)
	}

	return created.ID, nil
}

// startContainer starts the container this engine created.
func (e Engine) startContainer(ctx context.Context, identifier string) error {
	if err := engineRequest(ctx, e.endpoint, http.MethodPost, "/containers/"+identifier+"/start", nil, nil); err != nil {
		return fmt.Errorf("start the Simulated Cluster %s: %w", e.containerName, err)
	}

	return nil
}

// ClusterContainer is the handle to the container one run created. It holds that
// container's identifier unexported, and inspect and remove act on that container
// alone: the type declares no call that takes an identifier from a caller, so the
// lab cannot reach another container on the machine
// (docs/system-design.md § Threat Model).
type ClusterContainer struct {
	endpoint   Endpoint
	identifier string
	name       string
}

// Name is the name this run gave its container, carrying the Scenario it serves
// and a suffix unique to the invocation.
func (c *ClusterContainer) Name() string {
	return c.name
}

// APIAddress is the loopback address the engine published this container's API
// server on, which is the one address the lab's control-plane client targets.
func (c *ClusterContainer) APIAddress(ctx context.Context) (string, error) {
	var inspected struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string
			}
		}
	}
	if err := engineRequest(ctx, c.endpoint, http.MethodGet, "/containers/"+c.identifier+"/json", nil, &inspected); err != nil {
		return "", fmt.Errorf("read the published API port of %s: %w", c.name, err)
	}

	published := inspected.NetworkSettings.Ports[apiServerPort]
	if len(published) == 0 || published[0].HostPort == "" {
		return "", fmt.Errorf("the Simulated Cluster %s publishes no %s port yet", c.name, apiServerPort)
	}

	return "http://" + net.JoinHostPort(loopbackAddress, published[0].HostPort), nil
}

// Remove removes the container this run created, with its anonymous volumes, and
// returns once the engine no longer holds it.
func (c *ClusterContainer) Remove(ctx context.Context) error {
	query := url.Values{"force": {"true"}, "v": {"true"}}
	if err := engineRequest(ctx, c.endpoint, http.MethodDelete, "/containers/"+c.identifier+"?"+query.Encode(), nil, nil); err != nil {
		return fmt.Errorf("remove the Simulated Cluster %s: %w", c.name, err)
	}

	return c.awaitRemoved(ctx)
}

// errStillHeld is what a removal poll found when the engine still answers for the
// container. It is not a failure of its own: it is the reason a removal that
// times out names.
var errStillHeld = errors.New("the engine still holds it")

// awaitRemoved polls until the engine answers that it no longer holds this
// container.
//
// A removal is accepted before it has taken effect, so a Close that returned on
// the engine's answer alone would leave the cluster on the machine for a moment
// after the Scenario that owned it ended. The engine answering that it does not
// hold the container is what confirms the removal, and only that: a transport
// failure, an engine restart or a 5xx is an engine the lab cannot read rather
// than a container that is gone. Reading those alike would report a removal
// complete while an unauthenticated API server still runs
// (docs/system-design.md § Threat Model).
func (c *ClusterContainer) awaitRemoved(ctx context.Context) error {
	removing, cancel := context.WithTimeout(ctx, removalTimeout)
	defer cancel()

	for {
		unconfirmed := errStillHeld
		if err := engineRequest(removing, c.endpoint, http.MethodGet, "/containers/"+c.identifier+"/json", nil, nil); err != nil {
			if isNoLongerHeld(err) {
				return nil
			}
			unconfirmed = err
		}

		select {
		case <-removing.Done():
			return fmt.Errorf("the removal of the Simulated Cluster %s was not confirmed within %s: %w", c.name, removalTimeout, unconfirmed)
		case <-time.After(pollInterval):
		}
	}
}

// isNoLongerHeld reports whether err is the engine's own answer that it does not
// hold what was asked for, which is the one answer that confirms a removal.
func isNoLongerHeld(err error) bool {
	var refused engineRefusal

	return errors.As(err, &refused) && refused.code == http.StatusNotFound
}

// engineStream makes one request whose answer the caller reads as a stream, which
// is how the engine reports the progress of a pull. The caller closes it.
func engineStream(ctx context.Context, endpoint Endpoint, method, path string, body io.Reader) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://"+engineHost+path, body)
	if err != nil {
		return nil, fmt.Errorf("build the %s %s request: %w", method, path, err)
	}

	answer, err := endpoint.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s %s over %s: %w", method, path, endpoint.socket, err)
	}
	if answer.StatusCode >= http.StatusBadRequest {
		message := engineMessageIn(answer.Body)
		_ = answer.Body.Close()

		return nil, fmt.Errorf("%s %s over %s: the engine answered %s: %s", method, path, endpoint.socket, answer.Status, message)
	}

	return answer.Body, nil
}
