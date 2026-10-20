package cluster

import (
	"bytes"
	"log/slog"
	"net"
	"path/filepath"
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

func discardedWarnings() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func recordedWarnings() (*slog.Logger, *bytes.Buffer) {
	var written bytes.Buffer

	return slog.New(slog.NewTextHandler(&written, nil)), &written
}

func aSocketThatIsNotThere(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "absent-engine.sock")
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

// A declined value reaches a warning and the run's final error, neither of
// which may carry the password. A value is declined on two branches: one that
// parses and is redacted, and one whose url.Parse error quotes the raw value
// back.
func TestADeclinedDockerHost_ShouldBeReportedWithoutItsPassword(t *testing.T) {
	const password = "the-engine-password"
	const passwordURLParseRejects = "the-engine-pass word"

	for _, declined := range []struct {
		branch   string
		value    string
		password string
	}{
		{
			branch:   "a value that parses",
			value:    "ssh://operator:" + password + "@engine.example.invalid",
			password: password,
		},
		{
			branch:   "a value that does not parse",
			value:    "ssh://operator:" + passwordURLParseRejects + "@engine.example.invalid",
			password: passwordURLParseRejects,
		},
	} {
		t.Run(declined.branch, func(t *testing.T) {
			warnings, written := recordedWarnings()

			_, err := detectEngine(t.Context(), warnings, declined.value, []string{aSocketThatIsNotThere(t)})

			if err == nil {
				t.Fatal("neither the DOCKER_HOST value nor the candidate names an engine, but an endpoint was resolved")
			}
			if strings.Contains(written.String(), declined.password) {
				t.Errorf("the warning carries the DOCKER_HOST password, and it reads: %s", written)
			}
			if strings.Contains(err.Error(), declined.password) {
				t.Errorf("the run's error carries the DOCKER_HOST password, and it reads: %v", err)
			}
		})
	}
}

// These are the shapes whose parse error quotes part of the input back: the
// whole value in the *url.Error, three bytes at a bad percent escape, and the
// port at a bad port.
func TestAParseFailure_ShouldBeReportedWithoutTheValueThatFailed(t *testing.T) {
	fixedCause := []string{dockerHostVariable + " (it is not a URL: " + unparsableCause + ")"}

	for _, rejected := range []struct {
		shape    string
		value    string
		quotes   string
		fragment string
	}{
		{
			shape:    "a space in the password",
			value:    "ssh://operator:the-engine-pass word@engine.example.invalid",
			quotes:   "the whole value",
			fragment: "the-engine-pass",
		},
		{
			shape:    "a bad percent escape in the password",
			value:    "ssh://operator:the-engine-pass%zzword@engine.example.invalid",
			quotes:   "three bytes of it",
			fragment: "%zz",
		},
		{
			shape:    "a port that is not a number",
			value:    "tcp://engine.example.invalid:the-engine-port",
			quotes:   "the port it read",
			fragment: "the-engine-port",
		},
	} {
		t.Run(rejected.shape, func(t *testing.T) {
			warnings, written := recordedWarnings()

			_, tried, resolved := engineFromDockerHost(t.Context(), warnings, rejected.value)

			expect.Expect(t, resolved).To(expect.Equal(false))
			expect.Expect(t, tried).To(expect.Equal(fixedCause))
			if strings.Contains(written.String(), rejected.fragment) {
				t.Errorf("the parse error quotes %s, and the warning carries %q of the %s value: %s",
					rejected.quotes, rejected.fragment, dockerHostVariable, written)
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

func TestWhenNoSocketAnswers_ShouldFailNamingEveryPathTriedAndWhy(t *testing.T) {
	firstCandidate, secondCandidate := aSocketThatIsNotThere(t), aSocketThatIsNotThere(t)
	deadDockerHost := aSocketThatIsNotThere(t)

	_, err := detectEngine(t.Context(), discardedWarnings(), "unix://"+deadDockerHost, []string{firstCandidate, secondCandidate})

	if err == nil {
		t.Fatal("no socket answering is an error, but an endpoint was resolved")
	}
	for _, tried := range []string{deadDockerHost, firstCandidate, secondCandidate} {
		if !strings.Contains(err.Error(), tried) {
			t.Errorf("the error names every path tried, but %s is missing from: %v", tried, err)
		}
	}
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
