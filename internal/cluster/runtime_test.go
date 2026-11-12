//go:build !unit

package cluster

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// The engine is the real one on this machine's socket rather than a double.
// requireEngine fails only the calling test where none answers, so every test
// that needs no engine still runs.
func requireEngine(t *testing.T) Endpoint {
	t.Helper()

	endpoint, err := DetectEngine(t.Context(), discardedWarnings())
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	// Each test is the run that releases what it held open, as cmd/lab does
	// once per run.
	t.Cleanup(endpoint.Close)

	return endpoint
}

func TestADockerHostNamingAUnixSocketThatAnswers_ShouldBeUsedWithoutProbingACandidate(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)

	resolved, err := detectEngine(t.Context(), discardedWarnings(), "unix://"+live.Socket(), []string{aSocketThatIsNotThere(t)})
	if err != nil {
		t.Fatalf("resolve the engine from DOCKER_HOST: %v", err)
	}

	expect.Expect(t, resolved.Socket()).To(expect.Equal(live.Socket()))
}

func TestADockerHostNamingADeadUnixSocket_ShouldFallThroughToALiveCandidate(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)
	warnings, written := recordedWarnings()
	dead := aSocketThatIsNotThere(t)

	resolved, err := detectEngine(t.Context(), warnings, "unix://"+dead, []string{live.Socket()})
	if err != nil {
		t.Fatalf("fall through from a dead DOCKER_HOST to a live candidate: %v", err)
	}

	expect.Expect(t, resolved.Socket()).To(expect.Equal(live.Socket()))
	if !strings.Contains(written.String(), dead) {
		t.Errorf("the warning names the DOCKER_HOST that did not answer, but reads: %s", written)
	}
}

// The listener would answer a connection, so its count is the observation that
// none was made.
func TestADockerHostOfAnotherScheme_ShouldBeSkippedWithoutADialAndTheLiveCandidateUsed(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)
	elsewhere, dials := aListenerCountingItsDials(t)

	for scheme, dockerHost := range map[string]string{
		"tcp":   "tcp://" + elsewhere,
		"ssh":   "ssh://" + elsewhere,
		"npipe": `npipe:////./pipe/docker_engine`,
	} {
		t.Run(scheme, func(t *testing.T) {
			warnings, written := recordedWarnings()

			resolved, err := detectEngine(t.Context(), warnings, dockerHost, []string{live.Socket()})
			if err != nil {
				t.Fatalf("probe the candidates after skipping a %s DOCKER_HOST: %v", scheme, err)
			}

			expect.Expect(t, resolved.Socket()).To(expect.Equal(live.Socket()))
			expect.Expect(t, dials.Load()).To(expect.Equal(int64(0)))
			if !strings.Contains(written.String(), scheme) {
				t.Errorf("the warning names the scheme it skipped, but reads: %s", written)
			}
		})
	}
}

func TestADockerHostThatIsNotAURL_ShouldBeSkippedWithAWarningAndTheLiveCandidateUsed(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)
	// A non-hexadecimal percent escape is the one DOCKER_HOST shape that does
	// not parse as a URL at all.
	const notAURL = "unix://%zz"
	warnings, written := recordedWarnings()

	resolved, err := detectEngine(t.Context(), warnings, notAURL, []string{live.Socket()})
	if err != nil {
		t.Fatalf("probe the candidates after skipping a DOCKER_HOST that is not a URL: %v", err)
	}

	expect.Expect(t, resolved.Socket()).To(expect.Equal(live.Socket()))
	if !strings.Contains(written.String(), dockerHostVariable) {
		t.Errorf("the warning names the variable it skipped, but reads: %s", written)
	}
}

func TestAnEngineThatAnswers_ShouldReportTheVersionItRuns(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)

	if live.EngineVersion() == "" {
		t.Error("the resolved endpoint carries the engine version that answered, but it is empty")
	}
}

// anEngineAnsweringTheCreationLate forwards every request to the real engine,
// and the lab is stopped while a container's creation is under way: the
// creation's answer reaches the lab only after the stop.
func anEngineAnsweringTheCreationLate(live Endpoint, theLabStops context.CancelFunc) Endpoint {
	forwarded := live.client.Transport

	return anEngineStandIn(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/containers/create" {
			return forwarded.RoundTrip(request)
		}

		answer, err := forwarded.RoundTrip(request)
		theLabStops()

		return answer, err
	})
}

// startedLate is what a creation answered late leaves: whether the start was
// refused for the stop, and whether its container is still on the engine.
type startedLate struct {
	Refused          bool
	StillOnTheEngine bool
}

func TestASimulatedClusterWhoseCreationTheRuntimeAnswersLate_ShouldBeRemovedWhenItsScenarioEnds(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)
	if err := ObtainVersion(t.Context(), live, DefaultKubernetesVersion()); err != nil {
		t.Fatalf("obtain the pinned version %s: %v", DefaultKubernetesVersion(), err)
	}
	starting, theLabStops := context.WithCancel(t.Context())
	defer theLabStops()
	engine, err := newEngine(anEngineAnsweringTheCreationLate(live, theLabStops), resilientPlacementScenario)
	if err != nil {
		t.Fatalf("name the Simulated Cluster's container: %v", err)
	}
	t.Cleanup(func() {
		_ = removeContainer(context.WithoutCancel(t.Context()), live, engine.containerName, engine.containerName)
	})

	_, err = engine.StartCluster(starting, DefaultKubernetesVersion())

	expect.Expect(t, startedLate{
		Refused: errors.Is(err, context.Canceled), StillOnTheEngine: stillOnTheEngine(t, live, engine.containerName),
	}).To(expect.Equal(startedLate{Refused: true}))
}

// refusedCreation is what a refused creation leaves: whether the start failed on
// the engine's refusal, and whether the container already holding the name is
// still on the engine.
type refusedCreation struct {
	Refused          bool
	StillOnTheEngine bool
}

// The engine refuses a creation whose name a container already holds, so that
// container stands for another's that happens to carry the name.
func TestASimulatedClusterWhoseCreationTheRuntimeRefuses_ShouldLeaveTheContainerHoldingItsNameOnTheEngine(t *testing.T) {
	t.Parallel()
	live := requireEngine(t)
	if err := ObtainVersion(t.Context(), live, DefaultKubernetesVersion()); err != nil {
		t.Fatalf("obtain the pinned version %s: %v", DefaultKubernetesVersion(), err)
	}
	engine, err := newEngine(live, resilientPlacementScenario)
	if err != nil {
		t.Fatalf("name the Simulated Cluster's container: %v", err)
	}
	if _, err := engine.createContainer(t.Context(), clusterImage(DefaultKubernetesVersion())); err != nil {
		t.Fatalf("create the container holding the name %s: %v", engine.containerName, err)
	}
	t.Cleanup(func() {
		_ = removeContainer(context.WithoutCancel(t.Context()), live, engine.containerName, engine.containerName)
	})

	_, err = engine.StartCluster(t.Context(), DefaultKubernetesVersion())

	_, refused := errors.AsType[engineRefusal](err)
	expect.Expect(t, refusedCreation{
		Refused: refused, StillOnTheEngine: stillOnTheEngine(t, live, engine.containerName),
	}).To(expect.Equal(refusedCreation{Refused: true, StillOnTheEngine: true}))
}

// A machine that refuses the listener leaves the dial count unobservable, so
// the calling test fails here rather than skipping.
func aListenerCountingItsDials(t *testing.T) (string, *atomic.Int64) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("this machine does not let a test listen on a loopback port, so a dial count cannot be observed: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var dials atomic.Int64
	go func() {
		for {
			accepted, err := listener.Accept()
			if err != nil {
				return
			}
			dials.Add(1)
			_ = accepted.Close()
		}
	}()

	return listener.Addr().String(), &dials
}
