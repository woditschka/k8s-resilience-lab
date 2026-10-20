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

// Only a unix path in DOCKER_HOST is ever dialed.
const dockerHostVariable = "DOCKER_HOST"

// A tcp:// or ssh:// value could name an engine on another machine, which the
// lab never drives.
const unixScheme = "unix"

// The home directory is used to build a socket path and nothing else: no file
// under it is read.
const homePrefix = "~/"

// The first candidate that answers is the engine for the whole run.
var socketCandidates = []string{
	"/var/run/docker.sock",
	homePrefix + ".rd/docker.sock",
	homePrefix + ".docker/run/docker.sock",
	homePrefix + ".colima/default/docker.sock",
	"/var/run/podman/podman.sock",
}

// probeTimeout keeps an unresponsive socket from holding up a run.
const probeTimeout = 2 * time.Second

// The socket decides which engine answers, so the host is a placeholder the
// dialer ignores.
const engineHost = "engine"

// Every engine call is a round trip on a local socket, so one still unanswered
// after this has wedged. Without a deadline it would hold up the run and the
// deferred removal would never run, leaving an unauthenticated API server on
// the machine.
const engineTimeout = 15 * time.Second

// The polling loops make hundreds of calls over one client; this closes a
// connection no longer wanted instead of holding it for the rest of the run.
const idleConnectionTimeout = 30 * time.Second

// Endpoint is resolved once and passed into every later engine call, so no
// package holds mutable process-wide state. It owns the client because the
// socket is fixed at detection time, which makes a polling loop cost one
// connection rather than one per poll.
type Endpoint struct {
	socket  string
	version string
	client  *http.Client
}

func (e Endpoint) Socket() string {
	return e.socket
}

func (e Endpoint) EngineVersion() string {
	return e.version
}

// Close is the run's to call, since the whole run shares one client per socket.
func (e Endpoint) Close() {
	if e.client != nil {
		e.client.CloseIdleConnections()
	}
}

// DetectEngine tries DOCKER_HOST first, then the candidate sockets in order.
// The error names every path tried and why each failed.
func DetectEngine(ctx context.Context, warnings *slog.Logger) (Endpoint, error) {
	return detectEngine(ctx, warnings, os.Getenv(dockerHostVariable), socketCandidates)
}

// detectEngine takes the DOCKER_HOST value as a parameter so a test can drive
// detection without changing the process's environment.
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

// An unset value was never a candidate and is reported as nothing tried. A
// value whose scheme is not unix is skipped and never dialed, since it could
// name an engine on another machine.
//
// The value is reported only in its redacted form, or by its variable name when
// it does not parse: the declined schemes can carry a user and a password, and
// a skipped value reaches a warning and the run's final error, which a reader
// pastes into an issue or a CI log.
func engineFromDockerHost(ctx context.Context, warnings *slog.Logger, dockerHost string) (Endpoint, []string, bool) {
	if dockerHost == "" {
		return Endpoint{}, nil, false
	}

	parsed, unparsable := url.Parse(dockerHost)
	if unparsable != nil {
		// An unparsable value has no redacted form, so it is named by its
		// variable alone.
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

// unparsableCause is a fixed sentence because no part of an unparsable value
// may be reported. url.Parse's errors quote the raw input or fragments of it,
// and a DOCKER_HOST can carry a password.
const unparsableCause = "the value does not parse as a URL"

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

// The value arrives as a parsed URL and is never interpolated into a command.
func unixSocketIn(parsed *url.URL) (string, error) {
	if parsed.Scheme != unixScheme {
		return "", fmt.Errorf("its scheme is %q and the lab dials a %q socket alone, which is an engine on this machine", parsed.Scheme, unixScheme)
	}
	if parsed.Path == "" {
		return "", errors.New("it names no socket path")
	}

	return parsed.Path, nil
}

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

// The version request doubles as the liveness check. A socket that does not
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

// The status code tells one refusal from another. Only 404 confirms a removal;
// every other code is a failing engine, and reading the two alike would report
// a cluster removed while it still runs.
type engineRefusal struct {
	code     int
	reported string
}

func (r engineRefusal) Error() string {
	return r.reported
}

// Responses are decoded into the lab's own unexported types, so no engine type
// crosses this package's boundary. Every call runs within the engine timeout; a
// caller needing a tighter budget sets it on its context.
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

// The dialer ignores the request's host, so no network address is ever
// resolved. One client serves a whole run, because an abandoned transport's
// keep-alive connection is held open by its own goroutines: a client per call
// would cost a socket descriptor and goroutines per poll.
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

func engineMessageIn(body io.Reader) string {
	var answered struct {
		Message string
	}
	if err := json.NewDecoder(body).Decode(&answered); err != nil || answered.Message == "" {
		return "no message"
	}

	return answered.Message
}

// The engine publishes the API port on a host port of its own choosing, so two
// Simulated Clusters on one machine cannot collide on a port.
const apiServerPort = "8080/tcp"

// The API server is unauthenticated, so a port published on any other interface
// would offer it to the network.
const loopbackAddress = "127.0.0.1"

// Only the first run of a Kubernetes version new to the machine pays
// pullTimeout.
const pullTimeout = 10 * time.Minute

const startTimeout = 2 * time.Minute

// The engine answers a removal as soon as it accepts it, so the lab waits for
// the container to be gone rather than for the answer.
const removalTimeout = 30 * time.Second

// The prefix lets a human reading the engine's own output tell which containers
// came from a run.
const containerNamePrefix = "k8s-resilience-lab"

// The random suffix keeps two clusters on one machine from naming the same
// container.
const containerSuffixLength = 8

// The image scales every heartbeat duration by five, a Node lease of 200s and a
// grace period of 3m20s, which is longer than a Scenario waits for a Placement
// to settle. A factor of one restores the stock Kubernetes timings, so an
// unresponsive Node is noticed within one monitor period. The values are
// spelled as the container's argument spells them, so the reported setting and
// the applied one cannot drift.
const (
	NodeHeartbeatFactor        = "1"
	DefaultNodeHeartbeatFactor = "5"
)

const heartbeatFactorArgument = "--heartbeat-factor=" + NodeHeartbeatFactor

// The node lifecycle controller paces tainting per Zone. At the stock rate a
// Zone's Nodes were tainted seconds apart, so a Zone Failure settled later than
// it is judged within. A single Node takes the limiter's burst token either
// way, so a Failure of one Node is unchanged. The values are spelled as the argument spells them, so the
// reported setting and the applied one cannot drift.
const (
	NodeEvictionRate        = "10"
	DefaultNodeEvictionRate = "0.1"
)

const evictionRateArgument = "--extra-args=kube-controller-manager=node-eviction-rate=" + NodeEvictionRate

// The image starts the KWOK controller managing every Node, so switching that
// off is required: managing all either beats a selector or is refused beside
// one. An extra argument is appended after kwokctl's own, where a repeated
// boolean flag takes the last value.
//
// The selector is composed from the same constants a Node is created with, so
// the marking cannot drift between creation and selection. Taking the label off
// a Node is how a Failure leaves it unmaintained.
const (
	manageAllNodesArgument    = "--extra-args=kwok-controller=manage-all-nodes=false"
	manageMarkedNodesArgument = "--extra-args=kwok-controller=manage-nodes-with-label-selector=" + fakeNodeLabel + "=" + fakeNodeLabelValue
)

// With the Stage CRD enabled the controller applies the lab's own Stages served
// through the API rather than the image's configuration file; the lab's
// pod-ready Stage is what reads a Pod's Startup time.
const stageCRDArgument = "--enable-crds=Stage"

// The image's entrypoint forwards these verbatim to the command that creates
// the cluster. They are constants alone, so no run input reaches the image, the
// container or its arguments.
var clusterArguments = []string{
	heartbeatFactorArgument,
	evictionRateArgument,
	manageAllNodesArgument,
	manageMarkedNodesArgument,
	stageCRDArgument,
}

// Engine is bound to the one container it may create. Its exported method names
// a Kubernetes version, never an image or a container, and the container name
// is composed here rather than supplied: the engine socket grants control of
// every container on the machine, and this type is how the lab declines all but
// its own.
type Engine struct {
	endpoint      Endpoint
	containerName string
}

func newEngine(endpoint Endpoint, scenarioName string) (Engine, error) {
	name, err := containerNameFor(scenarioName)
	if err != nil {
		return Engine{}, err
	}

	return Engine{endpoint: endpoint, containerName: name}, nil
}

func containerNameFor(scenarioName string) (string, error) {
	if scenarioName == "" {
		return "", errors.New("a Simulated Cluster's container carries the name of the Scenario it serves, so an unnamed Scenario is refused")
	}

	return fmt.Sprintf("%s-%s-%s", containerNamePrefix, scenarioName, strings.ToLower(rand.Text()[:containerSuffixLength])), nil
}

// StartCluster pulls the pinned image only when the machine lacks it.
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

// A repeated run of the same version makes no registry request.
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

	// The engine reports a pull failure inside the progress stream rather than
	// in the status. Each message is decoded and discarded in turn, so a pull
	// costs constant memory.
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

func (e Engine) startContainer(ctx context.Context, identifier string) error {
	if err := engineRequest(ctx, e.endpoint, http.MethodPost, "/containers/"+identifier+"/start", nil, nil); err != nil {
		return fmt.Errorf("start the Simulated Cluster %s: %w", e.containerName, err)
	}

	return nil
}

// ClusterContainer declares no call taking an identifier from a caller, so the
// lab cannot reach another container on the machine.
type ClusterContainer struct {
	endpoint   Endpoint
	identifier string
	name       string
}

func (c *ClusterContainer) Name() string {
	return c.name
}

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

// Remove returns only once the engine no longer holds the container.
func (c *ClusterContainer) Remove(ctx context.Context) error {
	query := url.Values{"force": {"true"}, "v": {"true"}}
	if err := engineRequest(ctx, c.endpoint, http.MethodDelete, "/containers/"+c.identifier+"?"+query.Encode(), nil, nil); err != nil {
		return fmt.Errorf("remove the Simulated Cluster %s: %w", c.name, err)
	}

	return c.awaitRemoved(ctx)
}

// errStillHeld is not a failure of its own: it is the reason a removal that
// times out names.
var errStillHeld = errors.New("the engine still holds it")

// A removal is accepted before it takes effect, so returning on the engine's
// answer would leave the cluster running for a moment after its Scenario ended.
// Only the engine's answer that it no longer holds the container confirms the
// removal: a transport failure, an engine restart or a 5xx is an engine the lab
// cannot read, and reading those alike would report a removal complete while an
// unauthenticated API server still runs.
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

func isNoLongerHeld(err error) bool {
	var refused engineRefusal

	return errors.As(err, &refused) && refused.code == http.StatusNotFound
}

// The caller closes the returned stream.
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
