// These tests drive a real server on a loopback port with a real client and
// hand it real Placement boards through the method the observer calls, so no
// stand-in for either side exists.
package live

import (
	"errors"
	"html"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Two Zones of one Node each tell a Zone's line from its Node's.
const (
	someNodePool   = "general"
	someNodePods   = 10
	someNodeCPU    = scenario.CPU(16000)
	someNodeMemory = scenario.Memory(65536)
	twoZones       = 2
	oneNodePerZone = 1

	aWorkload       = "checkout"
	aPodPlacedFirst = "checkout-5fc87ff7c8-7vd48"
	aPodPlacedLater = "checkout-5fc87ff7c8-q2x9z"
	aPodLeftPending = "checkout-5fc87ff7c8-p0nd1"

	whyTheObservationFailed = "list the Nodes of a-cluster: connection refused"

	aHostReboundToLoopback = "rebound.example"
	aPathNotServed         = "/elsewhere"
	theLoopbackAddress     = "http://127.0.0.1:"

	// The types a browser needs to render the page and run its script under
	// the nosniff header.
	anHTMLDocument    = "text/html; charset=utf-8"
	aJavaScriptSource = "text/javascript; charset=utf-8"

	// What stops an injected name from running or sending anything, and the
	// browser from caching or reinterpreting an answer.
	aPolicyDenyingEverythingButItself = "default-src 'none'"
	aPolicyDenyingFraming             = "frame-ancestors 'none'"
	aBanOnTypeSniffing                = "nosniff"
	aBanOnCaching                     = "no-store"

	aCauseCarryingMarkup = "refused by <script>alert(1)</script>"
	theCauseEscaped      = "refused by &lt;script&gt;alert(1)&lt;/script&gt;"
	theRefreshAttribute  = `data-refresh-ms="1000"`
)

func aTopologyOfTwoZones(t *testing.T) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(someNodePods, someNodeCPU, someNodeMemory)
	if err != nil {
		t.Fatalf("build a Node capacity: %v", err)
	}
	pool, err := scenario.NewNodePool(someNodePool, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build the NodePool %s: %v", someNodePool, err)
	}
	topology, err := scenario.NewTopology(twoZones, pool)
	if err != nil {
		t.Fatalf("build a Topology of two Zones: %v", err)
	}

	return topology
}

func aPodOn(node scenario.Node, name string) scenario.Pod {
	return scenario.Pod{Name: name, Workload: aWorkload, Node: node.Name, Phase: scenario.PodRunning}
}

func aPendingPod() scenario.Pod {
	return scenario.Pod{Name: aPodLeftPending, Workload: aWorkload, Phase: scenario.PodPending}
}

func aBoardOfOnePod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	topology := aTopologyOfTwoZones(t)

	return scenario.NewPlacementBoard(topology, []scenario.Pod{aPodOn(topology.Nodes()[0], aPodPlacedFirst)},
		scenario.PlacementBoard{})
}

// aServerShowing closes its server when the test ends, within the server's own
// bound.
func aServerShowing(t *testing.T, view *View) *Server {
	t.Helper()

	served, err := Listen(t.Context(), view)
	if err != nil {
		t.Fatalf("listen for the Live view: %v", err)
	}
	t.Cleanup(func() {
		if err := served.Close(); err != nil {
			t.Errorf("shut the Live view down: %v", err)
		}
	})

	return served
}

// aBrowser holds its own connections, so two of them stand for two browsers.
func aBrowser() *http.Client {
	return &http.Client{Transport: &http.Transport{}}
}

func aRequestTo(t *testing.T, served *Server, method, path string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, served.Address()+strings.TrimPrefix(path, "/"), nil)
	if err != nil {
		t.Fatalf("build a %s request for %s: %v", method, path, err)
	}

	return request
}

func answerTo(t *testing.T, browser *http.Client, request *http.Request) (*http.Response, string) {
	t.Helper()

	answered, err := browser.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.URL, err)
	}
	body, err := io.ReadAll(answered.Body)
	if err != nil {
		t.Fatalf("read the answer to %s %s: %v", request.Method, request.URL, err)
	}
	if err := answered.Body.Close(); err != nil {
		t.Fatalf("close the answer to %s %s: %v", request.Method, request.URL, err)
	}

	return answered, string(body)
}

// theBoardShownTo reads the text a browser shows, so the markup around it and
// the escaping inside it are left out.
func theBoardShownTo(t *testing.T, browser *http.Client, served *Server) string {
	t.Helper()

	answered, body := answerTo(t, browser, aRequestTo(t, served, http.MethodGet, boardPath))
	if answered.StatusCode != http.StatusOK {
		t.Fatalf("the Live view answers its board with %d: %s", answered.StatusCode, body)
	}

	return html.UnescapeString(theTextOf(body))
}

func theTextOf(markup string) string {
	var text strings.Builder
	inTag := false
	for _, character := range markup {
		switch {
		case character == '<':
			inTag = true
		case character == '>':
			inTag = false
		case !inTag:
			text.WriteRune(character)
		}
	}

	return strings.TrimSpace(text.String())
}

func linesOf(board scenario.PlacementBoard) string {
	return strings.Join(board.Lines(), "\n")
}

func TestTheLiveView_ShouldFollowEveryZoneNodeAndPodAsThePlacementChanges(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	first := scenario.NewPlacementBoard(topology, []scenario.Pod{aPodOn(topology.Nodes()[0], aPodPlacedFirst)},
		scenario.PlacementBoard{})
	second := scenario.NewPlacementBoard(topology, []scenario.Pod{
		aPodOn(topology.Nodes()[0], aPodPlacedFirst), aPodOn(topology.Nodes()[1], aPodPlacedLater), aPendingPod(),
	}, first)
	view := &View{}
	served := aServerShowing(t, view)
	browser := aBrowser()

	view.Observed(first, nil)
	shownFirst := theBoardShownTo(t, browser, served)
	view.Observed(second, nil)
	shownSecond := theBoardShownTo(t, browser, served)

	expect.Expect(t, []string{shownFirst, shownSecond}).To(expect.Equal([]string{linesOf(first), linesOf(second)}))
}

func TestTheLiveViewOpenedLateOrInTwoBrowsers_ShouldShowThePlacementAsItStands(t *testing.T) {
	board := aBoardOfOnePod(t)
	view := &View{}
	served := aServerShowing(t, view)

	view.Observed(board, nil)
	shownToOne := theBoardShownTo(t, aBrowser(), served)
	shownToAnother := theBoardShownTo(t, aBrowser(), served)

	expect.Expect(t, []string{shownToOne, shownToAnother}).To(expect.Equal([]string{linesOf(board), linesOf(board)}))
}

func TestAnObservationThatFailed_ShouldBeNamedInTheLiveViewUntilTheNextReplacesIt(t *testing.T) {
	board := aBoardOfOnePod(t)
	view := &View{}
	served := aServerShowing(t, view)
	browser := aBrowser()

	view.Observed(scenario.PlacementBoard{}, errors.New(whyTheObservationFailed))
	shownAfterTheFailure := theBoardShownTo(t, browser, served)
	view.Observed(board, nil)
	shownAfterTheNext := theBoardShownTo(t, browser, served)

	expect.Expect(t, strings.Contains(shownAfterTheFailure, whyTheObservationFailed)).To(expect.Equal(true))
	expect.Expect(t, shownAfterTheNext).To(expect.Equal(linesOf(board)))
}

func TestTheLiveView_ShouldListenOnTheLoopbackAddressAlone(t *testing.T) {
	served := aServerShowing(t, &View{})

	expect.Expect(t, strings.HasPrefix(served.Address(), theLoopbackAddress)).To(expect.Equal(true))
}

func TestTheLiveViewsPageScriptAndBoard_ShouldEachBeServedUnderTheirOwnType(t *testing.T) {
	typePerPath := map[string]string{
		pagePath:   anHTMLDocument,
		scriptPath: aJavaScriptSource,
		boardPath:  anHTMLDocument,
	}
	served := aServerShowing(t, &View{})

	for path, contentType := range typePerPath {
		t.Run(path, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, path))

			expect.Expect(t, []any{answered.StatusCode, answered.Header.Get("Content-Type")}).
				To(expect.Equal([]any{http.StatusOK, contentType}))
		})
	}
}

func TestEveryAnswerOfTheLiveView_ShouldCarryItsPolicyAndForbidSniffingAndCaching(t *testing.T) {
	served := aServerShowing(t, &View{})

	for _, path := range []string{pagePath, scriptPath, boardPath} {
		t.Run(path, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, path))
			policy := answered.Header.Get("Content-Security-Policy")

			expect.Expect(t, []any{
				strings.Contains(policy, aPolicyDenyingEverythingButItself),
				strings.Contains(policy, aPolicyDenyingFraming),
				answered.Header.Get("X-Content-Type-Options"),
				answered.Header.Get("Cache-Control"),
			}).To(expect.Equal([]any{true, true, aBanOnTypeSniffing, aBanOnCaching}))
		})
	}
}

func TestAnObservationFailureCarryingMarkup_ShouldBeServedEscapedNeverAsMarkup(t *testing.T) {
	view := &View{}
	served := aServerShowing(t, view)
	view.Observed(scenario.PlacementBoard{}, errors.New(aCauseCarryingMarkup))

	_, body := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, boardPath))

	expect.Expect(t, []bool{
		strings.Contains(body, theCauseEscaped),
		strings.Contains(body, "<script>"),
	}).To(expect.Equal([]bool{true, false}))
}

func TestTheLiveViewsPage_ShouldCarryTheRefreshIntervalTheScriptFollows(t *testing.T) {
	served := aServerShowing(t, &View{})

	_, page := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, pagePath))

	expect.Expect(t, strings.Contains(page, theRefreshAttribute)).To(expect.Equal(true))
}

func TestARequestTheLiveViewDoesNotServe_ShouldBeRefused(t *testing.T) {
	served := aServerShowing(t, &View{})
	refusals := map[string]struct {
		request *http.Request
		status  int
	}{
		"a method other than GET":  {request: aRequestTo(t, served, http.MethodPost, boardPath), status: http.StatusMethodNotAllowed},
		"a HEAD, which reads too":  {request: aRequestTo(t, served, http.MethodHead, boardPath), status: http.StatusMethodNotAllowed},
		"a path it does not serve": {request: aRequestTo(t, served, http.MethodGet, aPathNotServed), status: http.StatusNotFound},
		"a Host other than the address served": {
			request: withHost(aRequestTo(t, served, http.MethodGet, boardPath), aHostReboundToLoopback),
			status:  http.StatusMisdirectedRequest,
		},
	}

	for named, refused := range refusals {
		t.Run(named, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), refused.request)

			expect.Expect(t, answered.StatusCode).To(expect.Equal(refused.status))
		})
	}
}

func withHost(request *http.Request, host string) *http.Request {
	request.Host = host

	return request
}
