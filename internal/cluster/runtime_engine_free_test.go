//go:build !integration

package cluster

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

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

// aPullCutShortBy lacks every image, and its pull's progress stream fails with
// the error cut returns once one message is read, as a download cut part-way
// leaves it.
func aPullCutShortBy(cut func(*http.Request) error) answeredBy {
	return func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			return anEngineAnswer(http.StatusNotFound, strings.NewReader(`{"message":"no such image"}`)), nil
		}

		return anEngineAnswer(http.StatusOK, io.MultiReader(strings.NewReader(`{"status":"Pulling fs layer"}`), cutBy{request, cut})), nil
	}
}

func anEngineAnswer(code int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: io.NopCloser(body)}
}

// cutBy fails the read it is reached at rather than when the answer is made,
// so the message before it is read first.
type cutBy struct {
	request *http.Request
	cut     func(*http.Request) error
}

func (c cutBy) Read([]byte) (int, error) { return 0, c.cut(c.request) }

func aReadError(*http.Request) error { return io.ErrUnexpectedEOF }

// The stream stalls until the obtain's own deadline ends it.
func theObtainTimeout(request *http.Request) error {
	<-request.Context().Done()

	return request.Context().Err()
}

// A stalled pull must end this test long before the pull's own timeout.
const anObtainTimeout = 100 * time.Millisecond

func TestAKubernetesVersionWhoseDownloadIsCutShort_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	version := DefaultKubernetesVersion()

	for cause, cut := range map[string]func(*http.Request) error{"a read error": aReadError, "the obtain timeout": theObtainTimeout} {
		t.Run(cause, func(t *testing.T) {
			obtaining, cancel := context.WithTimeout(t.Context(), anObtainTimeout)
			defer cancel()

			err := ObtainVersion(obtaining, anEngineStandIn(aPullCutShortBy(cut)), version)

			expect.Expect(t, strings.Contains(fmt.Sprint(err), version.String())).To(expect.Equal(true))
		})
	}
}

func TestADeschedulerWhoseDownloadIsCutShort_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	paired := theDeschedulerPairedByDefault(t)

	err := ObtainDescheduler(t.Context(), anEngineStandIn(aPullCutShortBy(aReadError)), paired)

	expect.Expect(t, strings.Contains(fmt.Sprint(err), paired.String())).To(expect.Equal(true))
}

// anEngineHoldingContainers stands in for an engine that creates, inspects and
// removes containers by the name the lab gives them, and answers a creation as
// creates does. It counts the removals it took, since a container removed by
// mistake is what a refused name must not cost.
type anEngineHoldingContainers struct {
	creates creation

	guarded sync.Mutex
	held    map[string]bool
	// createdOnceLookedFor are names whose creation the engine completes only
	// after the lab first looks for them, as a creation still under way does.
	createdOnceLookedFor map[string]bool
	removed              []string
}

// creation answers the request creating the container of the name given.
type creation func(engine *anEngineHoldingContainers, name string) (*http.Response, error)

func anEngineCreatingAs(creates creation) *anEngineHoldingContainers {
	return &anEngineHoldingContainers{creates: creates, held: map[string]bool{}, createdOnceLookedFor: map[string]bool{}}
}

func (a *anEngineHoldingContainers) endpoint() Endpoint {
	return anEngineStandIn(a.answer)
}

func (a *anEngineHoldingContainers) removals() []string {
	a.guarded.Lock()
	defer a.guarded.Unlock()

	return a.removed
}

func (a *anEngineHoldingContainers) hold(name string) {
	a.guarded.Lock()
	defer a.guarded.Unlock()

	a.held[name] = true
}

func (a *anEngineHoldingContainers) holdOnceLookedFor(name string) {
	a.guarded.Lock()
	defer a.guarded.Unlock()

	a.createdOnceLookedFor[name] = true
}

// The image is always present, so a creation makes no pull.
func (a *anEngineHoldingContainers) answer(request *http.Request) (*http.Response, error) {
	container, inspected := strings.CutSuffix(strings.TrimPrefix(request.URL.Path, containersPath), inspectSuffix)
	switch {
	case strings.HasPrefix(request.URL.Path, imagesPath):
		return anEngineAnswer(http.StatusOK, strings.NewReader("")), nil
	case request.URL.Path == containersPath+createPath:
		return a.creates(a, request.URL.Query().Get("name"))
	case request.Method == http.MethodGet && inspected:
		return a.inspect(container), nil
	case request.Method == http.MethodDelete:
		return a.remove(container), nil
	}

	return anEngineAnswer(http.StatusNoContent, strings.NewReader("")), nil
}

func (a *anEngineHoldingContainers) inspect(name string) *http.Response {
	a.guarded.Lock()
	defer a.guarded.Unlock()

	if a.createdOnceLookedFor[name] {
		delete(a.createdOnceLookedFor, name)
		a.held[name] = true

		return anEngineAnswer(http.StatusNotFound, strings.NewReader(noSuchContainer))
	}
	if !a.held[name] {
		return anEngineAnswer(http.StatusNotFound, strings.NewReader(noSuchContainer))
	}

	return anEngineAnswer(http.StatusOK, strings.NewReader(aContainerPublishingNoPort))
}

func (a *anEngineHoldingContainers) remove(name string) *http.Response {
	a.guarded.Lock()
	defer a.guarded.Unlock()

	if !a.held[name] {
		return anEngineAnswer(http.StatusNotFound, strings.NewReader(noSuchContainer))
	}
	delete(a.held, name)
	a.removed = append(a.removed, name)

	return anEngineAnswer(http.StatusNoContent, strings.NewReader(""))
}

const (
	imagesPath     = "/images/"
	containersPath = "/containers/"
	createPath     = "create"
	inspectSuffix  = "/json"

	noRemoval = 0

	noSuchContainer = `{"message":"no such container"}`
	// The control plane is not this stand-in's to serve, so no port is
	// published.
	aContainerPublishingNoPort = `{}`
)

// The container's identifier is its name, so the lab's removal by either
// reaches the same container.
func createdAtOnce(engine *anEngineHoldingContainers, name string) (*http.Response, error) {
	engine.hold(name)

	return anEngineAnswer(http.StatusCreated, strings.NewReader(fmt.Sprintf(`{"Id":%q}`, name))), nil
}

func refusedForAHeldName(*anEngineHoldingContainers, string) (*http.Response, error) {
	return anEngineAnswer(http.StatusConflict, strings.NewReader(`{"message":"the name is already in use"}`)), nil
}

func failedAfterCreating(engine *anEngineHoldingContainers, name string) (*http.Response, error) {
	engine.hold(name)

	return anEngineAnswer(http.StatusInternalServerError, strings.NewReader(`{"message":"the engine faulted"}`)), nil
}

// The request deadline cuts the creation while the engine is still under way.
func cutWhileUnderWay(engine *anEngineHoldingContainers, name string) (*http.Response, error) {
	engine.holdOnceLookedFor(name)

	return nil, context.DeadlineExceeded
}

func anEngineOf(t *testing.T, endpoint Endpoint) Engine {
	t.Helper()

	engine, err := newEngine(endpoint, resilientPlacementScenario)
	if err != nil {
		t.Fatalf("name the Simulated Cluster's container: %v", err)
	}

	return engine
}

func TestACreationTheRuntimeFailsWithAFaultOfItsOwn_ShouldRemoveTheClusterCreatedUnderThatName(t *testing.T) {
	t.Parallel()
	faulting := anEngineCreatingAs(failedAfterCreating)
	engine := anEngineOf(t, faulting.endpoint())

	_, _ = engine.StartCluster(t.Context(), DefaultKubernetesVersion())

	expect.Expect(t, faulting.removals()).To(expect.Equal([]string{engine.containerName}))
}

func TestACreationRefusedForANameAnotherContainerHolds_ShouldLeaveThatContainerUntouched(t *testing.T) {
	t.Parallel()
	refusing := anEngineCreatingAs(refusedForAHeldName)
	engine := anEngineOf(t, refusing.endpoint())

	_, _ = engine.StartCluster(t.Context(), DefaultKubernetesVersion())

	expect.Expect(t, refusing.removals()).To(expect.HaveLen[string](noRemoval))
}

func TestACreationStillUnderWayWhenTheLabStopsWaiting_ShouldRemoveTheClusterOnceCreated(t *testing.T) {
	t.Parallel()
	stillCreating := anEngineCreatingAs(cutWhileUnderWay)
	engine := anEngineOf(t, stillCreating.endpoint())

	_, _ = engine.StartCluster(t.Context(), DefaultKubernetesVersion())

	expect.Expect(t, stillCreating.removals()).To(expect.Equal([]string{engine.containerName}))
}
