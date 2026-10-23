// These tests drive a real server on a loopback port with a real client and
// hand it real Placement boards through the method the observer calls, so no
// stand-in for either side exists. A fragment is well-formed markup, so each is
// decoded with encoding/xml rather than read in a browser.
package live

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Two Zones of one Node each tell a Zone's box from its Node's.
const (
	someNodePool   = "general"
	someNodePods   = 10
	someNodeCPU    = scenario.CPU(16000)
	someNodeMemory = scenario.Memory(65536)
	twoZones       = 2
	oneNodePerZone = 1

	aLabelKey   = "example.com/capacity-type"
	aLabelValue = "spot"

	aWorkload       = "checkout"
	aPodPlacedFirst = "checkout-5fc87ff7c8-7vd48"
	aPodPlacedLater = "checkout-5fc87ff7c8-q2x9z"
	aPodLeftPending = "checkout-5fc87ff7c8-p0nd1"

	whyTheObservationFailed = "list the Nodes of a-cluster: connection refused"

	aHostReboundToLoopback = "rebound.example"
	anotherSitesOrigin     = "http://rebound.example"
	aPathNotServed         = "/elsewhere"
	theLoopbackAddress     = "http://127.0.0.1:"

	// The types a browser needs to render the page, apply its stylesheet, run
	// its script and follow its stream under the nosniff header.
	anHTMLDocument    = "text/html; charset=utf-8"
	aJavaScriptSource = "text/javascript; charset=utf-8"
	aStylesheet       = "text/css; charset=utf-8"
	anEventStream     = "text/event-stream"

	// What stops an injected name from running or sending anything, and the
	// browser from caching or reinterpreting an answer.
	aPolicyDenyingEverythingButItself = "default-src 'none'"
	aPolicyAdmittingItsOwnStylesheet  = "style-src 'self'"
	aPolicyDenyingFraming             = "frame-ancestors 'none'"
	aBanOnTypeSniffing                = "nosniff"
	aBanOnCaching                     = "no-store"

	aCauseCarryingMarkup = "refused by <script>alert(1)</script>"
	theFailureNamedAfter = "no Placement board: "
	aRequestedCPU        = scenario.CPU(4000)
	aRequestedMemory     = scenario.Memory(8192)
	theRequestsWord      = "requests"
	thePodCountsRow      = "Pods"
	theCPURow            = "cpu"
	theMemoryRow         = "memory"
	theOnePodPlacedOnIt  = 1

	// An absolute or scheme-relative address is how a page loads from another
	// host.
	anAbsoluteAddress       = "://"
	aSchemeRelativeAddress  = `"//`
	aSchemeRelativeResource = "url(//"

	theNewcomerMark = "+"
	readyWord       = "Ready"
	notReadyWord    = "NotReady"
	podNotReadyWord = "not Ready"
	readyClass      = "ready"
	notReadyClass   = "not-ready"
)

// aStreamReadBound keeps a test whose event never comes from waiting for the
// test binary's own timeout.
const aStreamReadBound = 10 * time.Second

func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(someNodePods, someNodeCPU, someNodeMemory)
	if err != nil {
		t.Fatalf("build a Node capacity: %v", err)
	}

	return capacity
}

func aTopologyOfTwoZones(t *testing.T) scenario.Topology {
	t.Helper()

	pool, err := scenario.NewNodePool(someNodePool, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build the NodePool %s: %v", someNodePool, err)
	}
	labels, err := scenario.NewNodeLabels(map[string]string{aLabelKey: aLabelValue})
	if err != nil {
		t.Fatalf("build the Node labels: %v", err)
	}
	labelled, err := pool.Labelled(labels)
	if err != nil {
		t.Fatalf("label the NodePool %s: %v", someNodePool, err)
	}
	topology, err := scenario.NewTopology(twoZones, labelled)
	if err != nil {
		t.Fatalf("build a Topology of two Zones: %v", err)
	}

	return topology
}

func aPodOn(node scenario.Node, name string) scenario.Pod {
	return scenario.Pod{Name: name, Workload: aWorkload, Node: node.Name, Phase: scenario.PodRunning, Ready: true}
}

func aPendingPod() scenario.Pod {
	return scenario.Pod{Name: aPodLeftPending, Workload: aWorkload, Phase: scenario.PodPending}
}

func aBoardOf(topology scenario.Topology, pods []scenario.Pod, previous scenario.PlacementBoard) scenario.PlacementBoard {
	return scenario.NewPlacementBoard(topology, scenario.NewPlacement(topology.Nodes(), pods), previous)
}

func aBoardOfOnePod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	topology := aTopologyOfTwoZones(t)

	return aBoardOf(topology, []scenario.Pod{aPodOn(topology.Nodes()[0], aPodPlacedFirst)}, scenario.PlacementBoard{})
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

func withHost(request *http.Request, host string) *http.Request {
	request.Host = host

	return request
}

func withOrigin(request *http.Request, origin string) *http.Request {
	request.Header.Set("Origin", origin)

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

// stream is one browser's open board stream, read event by event.
type stream struct {
	answered *http.Response
	events   *bufio.Reader
}

func aStreamOpenedBy(t *testing.T, browser *http.Client, served *Server) stream {
	t.Helper()

	reading, stop := context.WithTimeout(t.Context(), aStreamReadBound)
	t.Cleanup(stop)
	request := aRequestTo(t, served, http.MethodGet, boardPath).WithContext(reading)
	answered, err := browser.Do(request)
	if err != nil {
		t.Fatalf("open the board's stream: %v", err)
	}
	t.Cleanup(func() { _ = answered.Body.Close() })
	if answered.StatusCode != http.StatusOK {
		t.Fatalf("the Live view answers its board's stream with %d", answered.StatusCode)
	}

	return stream{answered: answered, events: bufio.NewReader(answered.Body)}
}

// nextFragment joins an event's data lines as a browser's EventSource does.
func (s stream) nextFragment(t *testing.T) string {
	t.Helper()

	var data []string
	for {
		line, err := s.events.ReadString('\n')
		if err != nil {
			t.Fatalf("read the next event of the board's stream: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" && len(data) > 0 {
			return strings.Join(data, "\n")
		}
		if field, found := strings.CutPrefix(line, "data: "); found {
			data = append(data, field)
		}
	}
}

func theFragmentShownTo(t *testing.T, browser *http.Client, served *Server) string {
	t.Helper()

	return aStreamOpenedBy(t, browser, served).nextFragment(t)
}

// element is any element of a fragment, kept whole so a test finds a box by
// its id and reads it by its classes.
type element struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []element  `xml:",any"`
}

func decoded(t *testing.T, fragment string) element {
	t.Helper()

	var root element
	if err := xml.Unmarshal([]byte(fragment), &root); err != nil {
		t.Fatalf("decode the fragment as well-formed markup: %v\n%s", err, fragment)
	}

	return root
}

func (e element) attr(name string) string {
	for _, attr := range e.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}

	return ""
}

func (e element) hasClass(class string) bool {
	return slices.Contains(strings.Fields(e.attr("class")), class)
}

// all lists, depth first, every element under e carrying class.
func (e element) all(class string) []element {
	var found []element
	for _, child := range e.Children {
		if child.hasClass(class) {
			found = append(found, child)
		}
		found = append(found, child.all(class)...)
	}

	return found
}

func (e element) first(t *testing.T, class string) element {
	t.Helper()

	found := e.all(class)
	if len(found) == 0 {
		t.Fatalf("no element of class %q under <%s id=%q>", class, e.XMLName.Local, e.attr("id"))
	}

	return found[0]
}

func (e element) byID(t *testing.T, id string) element {
	t.Helper()

	found, ok := e.find(id)
	if !ok {
		t.Fatalf("no element with id %q", id)
	}

	return found
}

func (e element) find(id string) (element, bool) {
	for _, child := range e.Children {
		if child.attr("id") == id {
			return child, true
		}
		if found, ok := child.find(id); ok {
			return found, true
		}
	}

	return element{}, false
}

// text is what a browser shows of e, its own text and its children's in order.
func (e element) text() string {
	var shown strings.Builder
	shown.WriteString(e.Text)
	for _, child := range e.Children {
		shown.WriteString(child.text())
	}

	return strings.TrimSpace(shown.String())
}

func idsOf(elements []element) []string {
	ids := make([]string, 0, len(elements))
	for _, one := range elements {
		ids = append(ids, one.attr("id"))
	}

	return ids
}

func textsOf(elements []element) []string {
	texts := make([]string, 0, len(elements))
	for _, one := range elements {
		texts = append(texts, one.text())
	}

	return texts
}

func podNamesOf(t *testing.T, pods []element) []string {
	t.Helper()

	var names []string
	for _, pod := range pods {
		names = append(names, pod.first(t, "pod-name").text())
	}

	return names
}

// nesting is what each box of a fragment holds, by id.
type nesting struct {
	Zones       []string
	NodesOfZone map[string][]string
	PodsOfNode  map[string][]string
	Pending     []string
}

func nestingOf(t *testing.T, fragment string) nesting {
	t.Helper()

	root := decoded(t, fragment)
	read := nesting{NodesOfZone: map[string][]string{}, PodsOfNode: map[string][]string{}}
	for _, zone := range root.all("zone") {
		read.Zones = append(read.Zones, zone.attr("id"))
		read.NodesOfZone[zone.attr("id")] = idsOf(zone.all("node"))
		for _, node := range zone.all("node") {
			read.PodsOfNode[node.attr("id")] = podNamesOf(t, node.all("pod"))
		}
	}
	for _, unplaced := range root.all("unplaced") {
		read.Pending = podNamesOf(t, unplaced.all("pod"))
	}

	return read
}

func boardNameOf(t *testing.T, board scenario.PlacementBoard, podName string) string {
	t.Helper()

	for _, pod := range board.Unplaced() {
		if pod.Name == podName {
			return pod.BoardName()
		}
	}
	for _, zone := range board.Zones() {
		for _, node := range zone.Nodes {
			for _, pod := range node.Pods {
				if pod.Name == podName {
					return pod.BoardName()
				}
			}
		}
	}
	t.Fatalf("no Pod %s on the board", podName)

	return ""
}

func TestTheLiveView_ShouldDrawEachZoneAsABoxHoldingItsNodesAndEachNodeItsPods(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	first, second := topology.Nodes()[0], topology.Nodes()[1]
	placed := []scenario.Pod{aPodOn(first, aPodPlacedFirst), aPodOn(second, aPodPlacedLater), aPendingPod()}
	settled := aBoardOf(topology, placed, aBoardOf(topology, placed, scenario.PlacementBoard{}))
	view := NewView()
	served := aServerShowing(t, view)

	view.Observed(settled, nil)
	fragment := theFragmentShownTo(t, aBrowser(), served)

	expect.Expect(t, nestingOf(t, fragment)).To(expect.Equal(nesting{
		Zones: []string{zoneID(first.Zone), zoneID(second.Zone)},
		NodesOfZone: map[string][]string{
			zoneID(first.Zone):  {nodeID(first.Name)},
			zoneID(second.Zone): {nodeID(second.Name)},
		},
		PodsOfNode: map[string][]string{
			nodeID(first.Name):  {boardNameOf(t, settled, aPodPlacedFirst)},
			nodeID(second.Name): {boardNameOf(t, settled, aPodPlacedLater)},
		},
		Pending: []string{boardNameOf(t, settled, aPodLeftPending)},
	}))
}

// room is one row of a Node's room: what it is, its capacity, what its Pods
// request and what is left.
type room struct {
	Of, Capacity, Requests, Left string
}

type nodeBox struct {
	NodePool string
	Labels   []string
	Room     []room
}

func nodeBoxOf(t *testing.T, fragment, id string) nodeBox {
	t.Helper()

	node := decoded(t, fragment).byID(t, id)
	var rows []room
	for _, row := range node.first(t, "room").all("room-row") {
		rows = append(rows, room{
			Of:       row.first(t, "room-of").text(),
			Capacity: row.first(t, "room-capacity").text(),
			Requests: row.first(t, "room-requests").text(),
			Left:     row.first(t, "room-left").text(),
		})
	}

	return nodeBox{
		NodePool: node.first(t, "node-pool").text(),
		Labels:   textsOf(node.first(t, "labels").all("label")),
		Room:     rows,
	}
}

func TestANodesBoxInTheLiveView_ShouldNameItsNodePoolLabelsAndCapacityRequestsAndLeft(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	node := topology.Nodes()[0]
	capacity := aCapacity(t)
	request := scenario.NewRequest(aRequestedCPU, aRequestedMemory)
	asking := aPodOn(node, aPodPlacedFirst)
	asking.Request = request
	view := NewView()
	served := aServerShowing(t, view)

	view.Observed(aBoardOf(topology, []scenario.Pod{asking}, scenario.PlacementBoard{}), nil)
	fragment := theFragmentShownTo(t, aBrowser(), served)

	expect.Expect(t, nodeBoxOf(t, fragment, nodeID(node.Name))).To(expect.Equal(nodeBox{
		NodePool: someNodePool,
		Labels:   []string{aLabelKey + "=" + aLabelValue},
		Room: []room{
			{
				Of: thePodCountsRow, Capacity: strconv.Itoa(capacity.Pods()),
				Requests: strconv.Itoa(theOnePodPlacedOnIt), Left: strconv.Itoa(capacity.Pods() - theOnePodPlacedOnIt),
			},
			{
				Of: theCPURow, Capacity: capacity.CPU().String(),
				Requests: request.CPU().String(), Left: (capacity.CPU() - request.CPU()).String(),
			},
			{
				Of: theMemoryRow, Capacity: capacity.Memory().String(),
				Requests: request.Memory().String(), Left: (capacity.Memory() - request.Memory()).String(),
			},
		},
	}))
}

func TestAPodsBoxInTheLiveView_ShouldGiveItsRequestAsCpuAndMemoryZeroWhenItDeclaresNone(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	node := topology.Nodes()[0]
	request := scenario.NewRequest(aRequestedCPU, aRequestedMemory)
	asking := aPodOn(node, aPodPlacedFirst)
	asking.Request = request
	declaringNone := aPodOn(node, aPodPlacedLater)
	pendingAsking := aPendingPod()
	pendingAsking.Request = request
	cases := map[string]struct {
		pods []scenario.Pod
		pod  string
		want string
	}{
		"a placed Pod with a Request": {
			pods: []scenario.Pod{asking}, pod: aPodPlacedFirst,
			want: theRequestsWord + " cpu " + request.CPU().String() + " memory " + request.Memory().String(),
		},
		"a placed Pod declaring none": {
			pods: []scenario.Pod{declaringNone}, pod: aPodPlacedLater,
			want: theRequestsWord + " cpu 0 memory 0",
		},
		"a Pending Pod with a Request": {
			pods: []scenario.Pod{pendingAsking}, pod: aPodLeftPending,
			want: theRequestsWord + " cpu " + request.CPU().String() + " memory " + request.Memory().String(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			view := NewView()
			served := aServerShowing(t, view)

			view.Observed(aBoardOf(topology, tc.pods, scenario.PlacementBoard{}), nil)
			fragment := theFragmentShownTo(t, aBrowser(), served)

			expect.Expect(t, decoded(t, fragment).byID(t, podID(tc.pod)).first(t, "pod-requests").text()).To(expect.Equal(tc.want))
		})
	}
}

// readiness is how a box states Ready: by the class its colour follows and by
// word.
type readiness struct {
	Class string
	Word  string
}

func readinessOf(t *testing.T, fragment, id string) readiness {
	t.Helper()

	box := decoded(t, fragment).byID(t, id)
	class := notReadyClass
	if box.hasClass(readyClass) {
		class = readyClass
	}

	return readiness{Class: class, Word: box.first(t, "readiness").text()}
}

func TestANodeOrPodInTheLiveView_ShouldStateWhetherItIsReadyByColourAndWord(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	responding, failed := topology.Nodes()[0], topology.Nodes()[1]
	starting := aPodOn(responding, aPodPlacedLater)
	starting.Ready = false
	observed := scenario.NewPlacement(topology.Nodes(), []scenario.Pod{aPodOn(responding, aPodPlacedFirst), starting}).
		WithNodesNotReady([]string{failed.Name})
	view := NewView()
	served := aServerShowing(t, view)

	view.Observed(scenario.NewPlacementBoard(topology, observed, scenario.PlacementBoard{}), nil)
	fragment := theFragmentShownTo(t, aBrowser(), served)

	expect.Expect(t, []readiness{
		readinessOf(t, fragment, nodeID(responding.Name)),
		readinessOf(t, fragment, nodeID(failed.Name)),
		readinessOf(t, fragment, podID(aPodPlacedFirst)),
		readinessOf(t, fragment, podID(aPodPlacedLater)),
	}).To(expect.Equal([]readiness{
		{Class: readyClass, Word: readyWord},
		{Class: notReadyClass, Word: notReadyWord},
		{Class: readyClass, Word: readyWord},
		{Class: notReadyClass, Word: podNotReadyWord},
	}))
}

// mark is how a Pod's box reads its newcomer mark: the board's mark, and the
// step of its fade the stylesheet colours.
type mark struct {
	Mark string
	Fade string
}

func markOf(t *testing.T, fragment string) mark {
	t.Helper()

	pod := decoded(t, fragment).byID(t, podID(aPodPlacedFirst))
	marked := ""
	if strings.HasPrefix(pod.first(t, "pod-name").text(), theNewcomerMark) {
		marked = theNewcomerMark
	}

	return mark{Mark: marked, Fade: pod.attr("data-fade")}
}

// marksOverTheFade hands the view the same Placement once more than the
// newcomer fade lasts, as the observer does for a cluster that has stopped
// changing, and reads the Pod's mark after each.
func marksOverTheFade(t *testing.T, view *View, browser stream, topology scenario.Topology, pods []scenario.Pod) []mark {
	t.Helper()

	var marks []mark
	var previous scenario.PlacementBoard
	for range newcomerFade + 1 {
		previous = aBoardOf(topology, pods, previous)
		view.Observed(previous, nil)
		marks = append(marks, markOf(t, browser.nextFragment(t)))
	}

	return marks
}

// steppingDown is the mark on the board that first holds a Pod and on each
// board after, one step lower per observation until it is gone.
func steppingDown() []mark {
	var marks []mark
	for left := newcomerFade; left > 0; left-- {
		marks = append(marks, mark{Mark: theNewcomerMark, Fade: strconv.Itoa(left)})
	}

	return append(marks, mark{})
}

func TestAPodNewOnItsNodeInTheLiveView_ShouldBeMarkedAsAPlacementBoardMarksIt(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	pods := []scenario.Pod{aPodOn(topology.Nodes()[0], aPodPlacedFirst)}
	view := NewView()
	served := aServerShowing(t, view)

	marks := marksOverTheFade(t, view, aStream(t, served), topology, pods)

	expect.Expect(t, marks).To(expect.Equal(steppingDown()))
}

// aStream opens a stream on a view not yet observed and reads past the empty
// board it opens on, so the next event is the first observation's.
func aStream(t *testing.T, served *Server) stream {
	t.Helper()

	opened := aStreamOpenedBy(t, aBrowser(), served)
	opened.nextFragment(t)

	return opened
}

func TestTheLiveView_ShouldShowEachChangeAsTheLabObservesItWithNoReload(t *testing.T) {
	topology := aTopologyOfTwoZones(t)
	firstPod := aPodOn(topology.Nodes()[0], aPodPlacedFirst)
	first := aBoardOf(topology, []scenario.Pod{firstPod}, scenario.PlacementBoard{})
	second := aBoardOf(topology, []scenario.Pod{firstPod, aPodOn(topology.Nodes()[1], aPodPlacedLater)}, first)
	view := NewView()
	served := aServerShowing(t, view)
	browser := aStream(t, served)

	view.Observed(first, nil)
	shownFirst := nestingOf(t, browser.nextFragment(t))
	view.Observed(second, nil)
	shownSecond := nestingOf(t, browser.nextFragment(t))

	expect.Expect(t, [][]string{
		shownFirst.PodsOfNode[nodeID(topology.Nodes()[1].Name)],
		shownSecond.PodsOfNode[nodeID(topology.Nodes()[1].Name)],
	}).To(expect.Equal([][]string{
		nil,
		{theNewcomerMark + boardNameOf(t, second, aPodPlacedLater)},
	}))
}

func TestTheLiveViewOpenedLateOrInTwoBrowsers_ShouldShowThePlacementAsItStands(t *testing.T) {
	view := NewView()
	served := aServerShowing(t, view)
	view.Observed(aBoardOfOnePod(t), nil)
	shownBeforeAnother := theFragmentShownTo(t, aBrowser(), served)

	shownToOne := theFragmentShownTo(t, aBrowser(), served)
	shownToAnother := theFragmentShownTo(t, aBrowser(), served)

	expect.Expect(t, []string{shownToOne, shownToAnother}).To(expect.Equal([]string{shownBeforeAnother, shownBeforeAnother}))
}

func TestAnObservationThatFailed_ShouldBeNamedInTheLiveViewUntilTheNextReplacesIt(t *testing.T) {
	board := aBoardOfOnePod(t)
	view := NewView()
	served := aServerShowing(t, view)
	browser := aStream(t, served)

	view.Observed(scenario.PlacementBoard{}, errors.New(whyTheObservationFailed))
	shownAfterTheFailure := decoded(t, browser.nextFragment(t)).text()
	view.Observed(board, nil)
	shownAfterTheNext := nestingOf(t, browser.nextFragment(t))

	expect.Expect(t, strings.Contains(shownAfterTheFailure, whyTheObservationFailed)).To(expect.Equal(true))
	expect.Expect(t, shownAfterTheNext.PodsOfNode[nodeID(aTopologyOfTwoZones(t).Nodes()[0].Name)]).
		To(expect.HaveLen[string](theOnePodPlacedOnIt))
}

func TestAnObservationFailureCarryingMarkup_ShouldBeServedEscapedNeverAsMarkup(t *testing.T) {
	view := NewView()
	served := aServerShowing(t, view)
	view.Observed(scenario.PlacementBoard{}, errors.New(aCauseCarryingMarkup))

	shown := decoded(t, theFragmentShownTo(t, aBrowser(), served))

	expect.Expect(t, []any{shown.text(), shown.all("script")}).To(expect.Equal([]any{
		theFailureNamedAfter + aCauseCarryingMarkup, []element(nil),
	}))
}

func TestTheLiveView_ShouldListenOnTheLoopbackAddressAlone(t *testing.T) {
	served := aServerShowing(t, NewView())

	expect.Expect(t, strings.HasPrefix(served.Address(), theLoopbackAddress)).To(expect.Equal(true))
}

func TestTheLiveViewsPageScriptStylesheetAndBoard_ShouldEachBeServedUnderTheirOwnType(t *testing.T) {
	typePerPath := map[string]string{
		pagePath:       anHTMLDocument,
		scriptPath:     aJavaScriptSource,
		stylesheetPath: aStylesheet,
	}
	served := aServerShowing(t, NewView())

	for path, contentType := range typePerPath {
		t.Run(path, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, path))

			expect.Expect(t, []any{answered.StatusCode, answered.Header.Get("Content-Type")}).
				To(expect.Equal([]any{http.StatusOK, contentType}))
		})
	}
	t.Run(boardPath, func(t *testing.T) {
		opened := aStreamOpenedBy(t, aBrowser(), served)

		expect.Expect(t, opened.answered.Header.Get("Content-Type")).To(expect.Equal(anEventStream))
	})
}

type policy struct {
	DeniesEverythingButItself bool
	AdmitsItsOwnStylesheet    bool
	DeniesFraming             bool
	Sniffing                  string
	Caching                   string
}

func policyOf(answered *http.Response) policy {
	stated := answered.Header.Get("Content-Security-Policy")

	return policy{
		DeniesEverythingButItself: strings.Contains(stated, aPolicyDenyingEverythingButItself),
		AdmitsItsOwnStylesheet:    strings.Contains(stated, aPolicyAdmittingItsOwnStylesheet),
		DeniesFraming:             strings.Contains(stated, aPolicyDenyingFraming),
		Sniffing:                  answered.Header.Get("X-Content-Type-Options"),
		Caching:                   answered.Header.Get("Cache-Control"),
	}
}

var theLiveViewsPolicy = policy{
	DeniesEverythingButItself: true, AdmitsItsOwnStylesheet: true, DeniesFraming: true,
	Sniffing: aBanOnTypeSniffing, Caching: aBanOnCaching,
}

func TestEveryAnswerOfTheLiveView_ShouldCarryItsPolicyAndForbidSniffingAndCaching(t *testing.T) {
	served := aServerShowing(t, NewView())

	for _, path := range []string{pagePath, scriptPath, stylesheetPath} {
		t.Run(path, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, path))

			expect.Expect(t, policyOf(answered)).To(expect.Equal(theLiveViewsPolicy))
		})
	}
	t.Run(boardPath, func(t *testing.T) {
		opened := aStreamOpenedBy(t, aBrowser(), served)

		expect.Expect(t, policyOf(opened.answered)).To(expect.Equal(theLiveViewsPolicy))
	})
}

func TestTheLiveViewsPage_ShouldLoadNothingFromAnotherHost(t *testing.T) {
	served := aServerShowing(t, NewView())

	for _, path := range []string{pagePath, scriptPath, stylesheetPath} {
		t.Run(path, func(t *testing.T) {
			_, body := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, path))

			expect.Expect(t, []bool{
				strings.Contains(body, anAbsoluteAddress),
				strings.Contains(body, aSchemeRelativeAddress),
				strings.Contains(body, aSchemeRelativeResource),
			}).To(expect.Equal([]bool{false, false, false}))
		})
	}
}

func TestARequestTheLiveViewDoesNotServe_ShouldBeRefused(t *testing.T) {
	served := aServerShowing(t, NewView())
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
		"an Origin other than the address served": {
			request: withOrigin(aRequestTo(t, served, http.MethodGet, boardPath), anotherSitesOrigin),
			status:  http.StatusForbidden,
		},
	}

	for named, refused := range refusals {
		t.Run(named, func(t *testing.T) {
			answered, _ := answerTo(t, aBrowser(), refused.request)

			expect.Expect(t, answered.StatusCode).To(expect.Equal(refused.status))
		})
	}
}

// The page's own requests carry the served origin, which is admitted.
func TestARequestFromTheLiveViewsOwnPage_ShouldBeServed(t *testing.T) {
	served := aServerShowing(t, NewView())
	ownOrigin := strings.TrimSuffix(served.Address(), "/")

	answered, _ := answerTo(t, aBrowser(), withOrigin(aRequestTo(t, served, http.MethodGet, scriptPath), ownOrigin))

	expect.Expect(t, answered.StatusCode).To(expect.Equal(http.StatusOK))
}

// streamsOpened holds every stream open until the test ends.
func streamsOpened(t *testing.T, served *Server, count int) {
	t.Helper()

	for range count {
		aStream(t, served)
	}
}

func TestAStreamBeyondTheOpenStreamCap_ShouldBeRefused(t *testing.T) {
	served := aServerShowing(t, NewView())
	streamsOpened(t, served, maxStreams)

	answered, _ := answerTo(t, aBrowser(), aRequestTo(t, served, http.MethodGet, boardPath))

	expect.Expect(t, answered.StatusCode).To(expect.Equal(http.StatusServiceUnavailable))
}

func TestClosingTheLiveView_ShouldEndEveryOpenStreamWithinItsBound(t *testing.T) {
	served, err := Listen(t.Context(), NewView())
	if err != nil {
		t.Fatalf("listen for the Live view: %v", err)
	}
	open := aStream(t, served)
	started := time.Now()

	closed := served.Close()
	took := time.Since(started)

	_, ended := open.events.ReadString('\n')
	expect.Expect(t, closed).To(expect.Equal[error](nil))
	expect.Expect(t, []bool{took < shutdownTimeout, errors.Is(ended, io.EOF)}).To(expect.Equal([]bool{true, true}))
}
