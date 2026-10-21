// Package live serves the Live view: the latest Placement board of an
// Interactive run's Simulated Cluster, read by a browser on loopback.
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
	"strings"
	"sync"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// RefreshInterval is how often the observer reads the held cluster and how
// often the page fetches the latest board.
const RefreshInterval = time.Second

// Every connection is bounded, so a slow or stalled client holds nothing open.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = time.Minute
	shutdownTimeout   = 5 * time.Second
)

// The port is the operating system's choice, so two invocations never collide.
const loopback = "127.0.0.1:0"

const (
	pagePath   = "/"
	scriptPath = "/live.js"
	boardPath  = "/board"
)

// The page runs only its own script, served from the binary, and fetches only
// from the lab, so an injected name could neither run nor send anything.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

//go:embed page.html live.js
var assets embed.FS

var (
	pageMarkup  = template.Must(template.ParseFS(assets, "page.html"))
	boardMarkup = template.Must(template.New("board").Parse(
		`{{if .NotObserved}}<p>no Placement board: {{.NotObserved}}</p>{{else}}<pre>{{.Lines}}</pre>{{end}}`))
)

// View holds the latest Placement board every browser reads, or the cause the
// last observation failed. Its zero value shows no board.
type View struct {
	mu          sync.RWMutex
	board       scenario.PlacementBoard
	notObserved error
}

// Observed replaces what the Live view shows; it is safe to call while
// browsers read.
func (v *View) Observed(board scenario.PlacementBoard, notObserved error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.board, v.notObserved = board, notObserved
}

type shown struct {
	Lines       string
	NotObserved error
}

func (v *View) shown() shown {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return shown{Lines: strings.Join(v.board.Lines(), "\n"), NotObserved: v.notObserved}
}

// Server is the Live view's server, listening on loopback alone.
type Server struct {
	http    *http.Server
	host    string
	stopped chan error
}

// Listen serves view until Close; the server reads every request but changes
// nothing.
func Listen(ctx context.Context, view *View) (*Server, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", loopback)
	if err != nil {
		return nil, fmt.Errorf("listen for the Live view on %s: %w", loopback, err)
	}

	host := listener.Addr().String()
	server := &Server{
		http: &http.Server{
			Handler:           routes{view: view, host: host},
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		host:    host,
		stopped: make(chan error, 1),
	}
	go func() { server.stopped <- server.http.Serve(listener) }()

	return server, nil
}

// Address is where a browser reaches the Live view.
func (s *Server) Address() string {
	return "http://" + s.host + "/"
}

// Close takes its bound from the server rather than from a caller, because the
// lab closes it after its own context has been cancelled.
func (s *Server) Close() error {
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

// routes answers a GET of the page, its script or the board, and refuses
// everything else before reading anything of the request.
type routes struct {
	view *View
	host string
}

// A Host other than the address served is a page from another site whose name
// was rebound to loopback.
func (r routes) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Host != r.host {
		http.Error(w, "the Live view answers at "+r.host+" alone", http.StatusMisdirectedRequest)

		return
	}
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "the Live view serves reads alone", http.StatusMethodNotAllowed)

		return
	}

	switch request.URL.Path {
	case pagePath:
		render(w, pageMarkup, RefreshInterval.Milliseconds())
	case scriptPath:
		script, err := assets.ReadFile("live.js")
		if err != nil {
			http.Error(w, "the Live view's script is missing from the binary", http.StatusInternalServerError)

			return
		}
		respond(w, "text/javascript; charset=utf-8", script)
	case boardPath:
		render(w, boardMarkup, r.view.shown())
	default:
		http.NotFound(w, request)
	}
}

// render writes nothing until the whole template has executed, so a failure is
// answered as one rather than as half a page.
func render(w http.ResponseWriter, markup *template.Template, data any) {
	var rendered bytes.Buffer
	if err := markup.Execute(&rendered, data); err != nil {
		http.Error(w, "the Live view could not render "+markup.Name(), http.StatusInternalServerError)

		return
	}

	respond(w, "text/html; charset=utf-8", rendered.Bytes())
}

func respond(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
