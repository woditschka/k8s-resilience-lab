// Package live serves the Live view: the latest Placement board of an
// Interactive run's Simulated Cluster, pushed to a browser on loopback.
package live

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// RefreshInterval is how often the observer reads the held cluster.
const RefreshInterval = time.Second

// newcomerFade is how many observations a newcomer's mark lasts, longer than
// one interval so an engineer glancing away still sees it.
const newcomerFade = 3

// maxStreams bounds the connections a page from another site could hold open
// were it to pass every other check.
const maxStreams = 8

// Every connection is bounded, so a slow or stalled client holds nothing open.
// A stream outlives the write timeout, so each event bounds its own write.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	eventWriteTimeout = 5 * time.Second
	idleTimeout       = time.Minute
	shutdownTimeout   = 5 * time.Second
)

// The port is the operating system's choice, so two invocations never collide.
const loopback = "127.0.0.1:0"

const (
	pagePath       = "/"
	scriptPath     = "/live.js"
	stylesheetPath = "/live.css"
	boardPath      = "/board"
)

// The page runs only its own script and stylesheet, served from the binary,
// and connects only to the lab, so an injected name could neither run nor send
// anything.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

//go:embed page.html live.js live.css board.html
var assets embed.FS

var (
	boardMarkup = template.Must(template.ParseFS(assets, "board.html"))
	pageMarkup  = template.Must(template.ParseFS(assets, "page.html"))
)

// View holds the fragment every open stream shows: the latest Placement board
// rendered, or the cause the last observation failed.
type View struct {
	mu       sync.RWMutex
	fragment []byte
	// changed is closed and replaced each time the fragment changes, waking
	// every stream at once.
	changed chan struct{}
	// fading is written by Observed alone.
	fading map[podOnNode]int
}

type podOnNode struct {
	pod  string
	node string
}

// NewView shows a board of no Zone until the first observation.
func NewView() *View {
	return &View{changed: make(chan struct{}), fragment: rendered(scenario.PlacementBoard{}, nil, nil)}
}

// Observed renders what the Live view shows once per observation, however many
// browsers read it, and wakes every stream when it differs from the last. A
// failed observation keeps every newcomer's fade where it stood.
func (v *View) Observed(board scenario.PlacementBoard, notObserved error) {
	if notObserved == nil {
		v.fading = fadedAgainst(board, v.fading)
	}
	fragment := rendered(board, v.fading, notObserved)

	v.mu.Lock()
	defer v.mu.Unlock()

	if bytes.Equal(fragment, v.fragment) {
		return
	}
	v.fragment = fragment
	close(v.changed)
	v.changed = make(chan struct{})
}

// fadedAgainst gives a newcomer the whole fade and steps every other Pod's down
// by one, dropping a Pod whose mark is gone or that left its Node.
func fadedAgainst(board scenario.PlacementBoard, before map[podOnNode]int) map[podOnNode]int {
	fading := make(map[podOnNode]int)
	step := func(pod scenario.BoardPod, node string) {
		key := podOnNode{pod: pod.Name, node: node}
		if pod.Newcomer {
			fading[key] = newcomerFade
		} else if left := before[key] - 1; left > 0 {
			fading[key] = left
		}
	}
	for _, zone := range board.Zones() {
		for _, node := range zone.Nodes {
			for _, pod := range node.Pods {
				step(pod, node.Name)
			}
		}
	}
	for _, pod := range board.Unplaced() {
		step(pod, "")
	}

	return fading
}

// rendered falls back to naming the failure, so a template fault still shows
// rather than leaving the last board standing as if current.
func rendered(board scenario.PlacementBoard, fading map[podOnNode]int, notObserved error) []byte {
	var fragment bytes.Buffer
	if err := boardMarkup.Execute(&fragment, toShown(board, fading, notObserved)); err != nil {
		fragment.Reset()
		fragment.WriteString(`<p class="not-observed">`)
		template.HTMLEscape(&fragment, []byte("the Live view could not render the board: "+err.Error()))
		fragment.WriteString(`</p>`)
	}

	return fragment.Bytes()
}

// shownBoard is the board as the fragment names it: every word and class a
// box carries decided here, so the template only places them.
type shownBoard struct {
	NotObserved error
	Zones       []shownZone
	Unplaced    []shownPod
}

type shownZone struct {
	ID    string
	Name  string
	Nodes []shownNode
}

type shownNode struct {
	ID        string
	Name      string
	NodePool  string
	Labels    []string
	Readiness shownReadiness
	Room      []shownRoom
	Pods      []shownPod
}

type shownReadiness struct {
	Class string
	Word  string
}

type shownRoom struct {
	Of       string
	Capacity string
	Requests string
	Left     string
}

type shownPod struct {
	ID        string
	Name      string
	Readiness shownReadiness
	CPU       string
	Memory    string
	// Fade is the step of a newcomer's mark, none once it has faded.
	Fade int
}

var (
	ready        = shownReadiness{Class: "ready", Word: string(scenario.NodeReady)}
	nodeNotReady = shownReadiness{Class: "not-ready", Word: string(scenario.NodeNotReady)}
	podNotReady  = shownReadiness{Class: "not-ready", Word: "not Ready"}
	pending      = shownReadiness{Class: "pending", Word: string(scenario.PodPending)}
)

// newcomerMark is the mark a Placement board gives a newcomer.
const newcomerMark = "+"

func toShown(board scenario.PlacementBoard, fading map[podOnNode]int, notObserved error) shownBoard {
	if notObserved != nil {
		return shownBoard{NotObserved: notObserved}
	}

	var shown shownBoard
	for _, zone := range board.Zones() {
		nodes := make([]shownNode, 0, len(zone.Nodes))
		for _, node := range zone.Nodes {
			nodes = append(nodes, toShownNode(node, fading))
		}
		shown.Zones = append(shown.Zones, shownZone{ID: zoneID(zone.Name), Name: zone.Name, Nodes: nodes})
	}
	for _, pod := range board.Unplaced() {
		shown.Unplaced = append(shown.Unplaced, toShownPod(pod, pending, fading[podOnNode{pod: pod.Name}]))
	}

	return shown
}

func toShownNode(node scenario.BoardNode, fading map[podOnNode]int) shownNode {
	readiness := nodeNotReady
	if node.Readiness == scenario.NodeReady {
		readiness = ready
	}
	labels := make([]string, 0, len(node.Labels.Pairs()))
	for _, label := range node.Labels.Pairs() {
		labels = append(labels, label.Key+"="+label.Value)
	}
	pods := make([]shownPod, 0, len(node.Pods))
	for _, pod := range node.Pods {
		podReadiness := podNotReady
		if pod.Ready {
			podReadiness = ready
		}
		pods = append(pods, toShownPod(pod, podReadiness, fading[podOnNode{pod: pod.Name, node: node.Name}]))
	}

	return shownNode{
		ID: nodeID(node.Name), Name: node.Name, NodePool: node.NodePool, Labels: labels, Readiness: readiness,
		// The Pod count comes first, since it is the room a Node runs out of first in
		// most Scenarios.
		Room: []shownRoom{
			{
				Of: "Pods", Capacity: strconv.Itoa(node.Capacity.Pods()),
				Requests: strconv.Itoa(node.Requests.Pods), Left: strconv.Itoa(node.Left.Pods),
			},
			{
				Of: "cpu", Capacity: node.Capacity.CPU().String(),
				Requests: node.Requests.CPU.String(), Left: node.Left.CPU.String(),
			},
			{
				Of: "memory", Capacity: node.Capacity.Memory().String(),
				Requests: node.Requests.Memory.String(), Left: node.Left.Memory.String(),
			},
		},
		Pods: pods,
	}
}

func toShownPod(pod scenario.BoardPod, readiness shownReadiness, fade int) shownPod {
	name := pod.BoardName()
	if fade > 0 {
		name = newcomerMark + name
	}

	return shownPod{
		ID: podID(pod.Name), Name: name, Readiness: readiness,
		CPU: pod.Request.CPU().String(), Memory: pod.Request.Memory().String(), Fade: fade,
	}
}

// An id is derived from the name the control plane gave, which is unique per
// kind, so a box keeps its id from one fragment to the next.
func zoneID(name string) string { return "zone-" + name }

func nodeID(name string) string { return "node-" + name }

func podID(name string) string { return "pod-" + name }

func (v *View) latest() ([]byte, <-chan struct{}) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.fragment, v.changed
}

// Server is the Live view's server, listening on loopback alone.
type Server struct {
	http    *http.Server
	host    string
	closing chan struct{}
	stopped chan error
}

// Listen serves view until Close, its page headed by the name of the Scenario
// held; the server reads every request but changes nothing.
func Listen(ctx context.Context, view *View, scenarioName string) (*Server, error) {
	var page bytes.Buffer
	if err := pageMarkup.Execute(&page, struct{ Scenario string }{Scenario: scenarioName}); err != nil {
		return nil, fmt.Errorf("render the Live view's page for Scenario %s: %w", scenarioName, err)
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", loopback)
	if err != nil {
		return nil, fmt.Errorf("listen for the Live view on %s: %w", loopback, err)
	}

	host := listener.Addr().String()
	closing := make(chan struct{})
	server := &Server{
		http: &http.Server{
			Handler: routes{
				view: view, page: page.Bytes(), host: host, origin: "http://" + host,
				streams: make(chan struct{}, maxStreams), closing: closing,
			},
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		host:    host,
		closing: closing,
		stopped: make(chan error, 1),
	}
	go func() { server.stopped <- server.http.Serve(listener) }()

	return server, nil
}

// Address is where a browser reaches the Live view.
func (s *Server) Address() string {
	return "http://" + s.host + "/"
}

// Close ends every stream first, since a stream never goes idle and shutting
// down waits for every connection that is not. It takes its bound from the
// server rather than from a caller, because the lab closes it after its own
// context has been cancelled.
func (s *Server) Close() error {
	close(s.closing)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shut := s.http.Shutdown(ctx)
	if shut != nil {
		shut = errors.Join(fmt.Errorf("shut the Live view at %s down: %w", s.host, shut), s.http.Close())
	}

	stopped := <-s.stopped
	if errors.Is(stopped, http.ErrServerClosed) {
		stopped = nil
	}

	return errors.Join(shut, stopped)
}

// routes answers a GET of the page, its script, its stylesheet or the board's
// stream, and refuses everything else before reading anything of the request.
type routes struct {
	view *View
	// page is rendered once, since the Scenario held never changes.
	page   []byte
	host   string
	origin string
	// streams holds one token per open stream.
	streams chan struct{}
	closing <-chan struct{}
}

// A Host other than the address served is a page from another site whose name
// was rebound to loopback; an Origin other than it is a page from another site
// reaching for the stream.
func (r routes) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Host != r.host {
		http.Error(w, "the Live view answers at "+r.host+" alone", http.StatusMisdirectedRequest)

		return
	}
	if origin := request.Header.Get("Origin"); origin != "" && origin != r.origin {
		http.Error(w, "the Live view answers its own page alone", http.StatusForbidden)

		return
	}
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "the Live view serves reads alone", http.StatusMethodNotAllowed)

		return
	}

	switch request.URL.Path {
	case pagePath:
		secured(w, "text/html; charset=utf-8")
		_, _ = w.Write(r.page)
	case scriptPath:
		serveAsset(w, "live.js", "text/javascript; charset=utf-8")
	case stylesheetPath:
		serveAsset(w, "live.css", "text/css; charset=utf-8")
	case boardPath:
		r.stream(w, request)
	default:
		http.NotFound(w, request)
	}
}

func serveAsset(w http.ResponseWriter, name, contentType string) {
	asset, err := assets.ReadFile(name)
	if err != nil {
		http.Error(w, "the Live view's "+name+" is missing from the binary", http.StatusInternalServerError)

		return
	}
	secured(w, contentType)
	_, _ = w.Write(asset)
}

// stream sends the latest fragment on opening and after each change, so a
// browser falling behind skips to the latest rather than queueing.
func (r routes) stream(w http.ResponseWriter, request *http.Request) {
	select {
	case r.streams <- struct{}{}:
		defer func() { <-r.streams }()
	default:
		http.Error(w, "the Live view holds as many streams as it serves", http.StatusServiceUnavailable)

		return
	}

	secured(w, "text/event-stream")
	events := http.NewResponseController(w)
	for {
		fragment, changed := r.view.latest()
		if err := sendEvent(w, events, fragment); err != nil {
			return
		}

		select {
		case <-changed:
		case <-request.Context().Done():
			return
		case <-r.closing:
			return
		}
	}
}

// sendEvent writes one data line per line of the fragment, as the event-stream
// format requires, under its own deadline.
func sendEvent(w http.ResponseWriter, events *http.ResponseController, fragment []byte) error {
	if err := events.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
		return fmt.Errorf("bound the event's write: %w", err)
	}

	var event strings.Builder
	for line := range strings.SplitSeq(string(fragment), "\n") {
		event.WriteString("data: " + line + "\n")
	}
	event.WriteString("\n")
	if _, err := w.Write([]byte(event.String())); err != nil {
		return fmt.Errorf("send the board: %w", err)
	}

	if err := events.Flush(); err != nil {
		return fmt.Errorf("flush the board: %w", err)
	}

	return nil
}

func secured(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
}
