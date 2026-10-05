package lab

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

// A Scenario source is one Go file and no test file, and a run name writes a
// hyphen between two words where that file's stem writes an underscore.
const (
	goFileSuffix        = ".go"
	testFileStem        = "_test"
	theWordSeparator    = "_"
	theRunNameSeparator = "-"
)

// Outcome is what one Scenario reported: the name it ran under, the Steps it
// carried out in order, the Findings its Thens reported, the Placement it settled
// at, and the Step it could not carry out.
type Outcome struct {
	// Name is what this Scenario ran under, derived from the file its function is
	// defined in, and Cluster is the Simulated Cluster it ran on, named so a reader
	// can find it while the run is still going.
	Name      string
	Cluster   string
	Steps     []Step
	Findings  []Finding
	Placement scenario.Placement
	// EndedAt is the Step that could not be carried out, and Cause is why. Both
	// are empty for a Scenario that carried out every Step it holds.
	EndedAt Step
	Cause   error
	// Timeline is what this Scenario's Simulated Cluster did while it ran, FiledAt
	// where that Timeline was written, and NotFiled what prevented writing it. A
	// Timeline the lab could not write is the lab's own failure and leaves the rest
	// of this Outcome as reported, per
	// docs/adr/2026-10-05-timeline-from-the-watch-stream.md.
	Timeline scenario.Timeline
	FiledAt  string
	NotFiled error
}

// Ended reports whether a Step could not be carried out, which is what stopped
// the Steps after it.
func (o Outcome) Ended() bool {
	return o.Cause != nil
}

// Held reports whether every Step was carried out and the Placement met each
// Then.
func (o Outcome) Held() bool {
	return !o.Ended() && len(o.Findings) == 0
}

// outcome is what this Scenario reported, as one value for the Runner to hand on.
func (s *Scenario) outcome() Outcome {
	return Outcome{
		Steps:     slices.Clone(s.steps),
		Findings:  slices.Clone(s.findings),
		Placement: s.observed,
		EndedAt:   s.endedAt,
		Cause:     s.cause,
	}
}

// Runner runs the lab's Scenarios one at a time, on the engine a run resolved
// once and for the Kubernetes version that run mirrors.
type Runner struct {
	engine    cluster.Endpoint
	version   cluster.KubernetesVersion
	timelines timeline.Folder
}

// NewRunner builds the runner of a run: the resolved engine every Scenario's
// Simulated Cluster is provisioned on, the Kubernetes version each mirrors, and
// the folder every Scenario's Timeline is filed in.
func NewRunner(engine cluster.Endpoint, version cluster.KubernetesVersion, timelines timeline.Folder) *Runner {
	return &Runner{engine: engine, version: version, timelines: timelines}
}

// Run derives the Scenario's run name from the file its function is defined in,
// provisions a Simulated Cluster under that name, records the Timeline of what
// that cluster does, hands the Scenario's lines a Scenario to record their Steps
// into, and removes that cluster whether the Scenario held, reported a Finding or
// ended at a Step.
//
// The record is started before the first Step and stopped after the last one, so
// a Timeline holds the Topology coming up as well as the Placement settling, and
// the cluster is still there while it is being recorded. Opening the Timeline
// file cannot stop the Scenario: a file the lab could not write is reported on
// the Outcome, per docs/adr/2026-10-05-timeline-from-the-watch-stream.md.
//
// The error it returns is the lab's own: a run name it could not derive, a
// cluster it could not provision, one whose changes it could not watch, or one it
// could not remove. A name it cannot derive stops the run before anything is
// provisioned, so no Simulated Cluster is ever named after nothing. What the
// Scenario found is the Outcome, which a caller reports whether or not the
// removal succeeded.
func (r *Runner) Run(ctx context.Context, steps func(*Scenario)) (reported Outcome, err error) {
	name, err := runNameOfScenario(steps)
	if err != nil {
		return Outcome{}, err
	}

	provisioned, err := cluster.Provision(ctx, r.engine, r.version, name)
	if err != nil {
		return Outcome{}, fmt.Errorf("the Scenario %s: %w", name, err)
	}

	// The removal is deferred here rather than left to the Scenario, so a Scenario
	// that ends part-way through its Steps still loses its cluster, per
	// docs/adr/2026-10-04-cluster-per-scenario.md. The context is detached from ctx
	// so that a cancelled run still removes what it created.
	defer func() {
		removed := provisioned.Close(context.WithoutCancel(ctx))
		if removed != nil && err == nil {
			err = fmt.Errorf("remove the Simulated Cluster of the Scenario %s: %w", name, removed)
		}
	}()

	filed := r.timelines.Open(timeline.Header{
		Scenario: name, Cluster: provisioned.Name(), Kubernetes: r.version.String(),
	})
	defer filed.Close()

	if err := provisioned.Record(ctx, filed); err != nil {
		return Outcome{}, err
	}

	running := newScenario(ctx, provisioned)
	steps(running)

	recorded := provisioned.StopRecording()
	filed.Close()

	reported = running.outcome()
	reported.Name, reported.Cluster = name, provisioned.Name()
	reported.Timeline, reported.FiledAt, reported.NotFiled = recorded, filed.Path(), filed.Err()

	return reported, nil
}

// runNameOfScenario is the name the Scenario steps runs under. The Go runtime's
// own record of where that function is defined is what yields it, so a Scenario
// is named by its source alone and no run input reaches the name.
func runNameOfScenario(steps func(*Scenario)) (string, error) {
	definedAt := reflect.ValueOf(steps).Pointer()
	defined := runtime.FuncForPC(definedAt)
	if defined == nil {
		return "", errors.New("the Go runtime holds no position for this Scenario's function, so it has no run name")
	}

	file, _ := defined.FileLine(definedAt)
	name, err := runNameOf(file)
	if err != nil {
		return "", fmt.Errorf("the Scenario %s: %w", defined.Name(), err)
	}

	return name, nil
}

// runNameOf is the run name a Scenario's file gives: the stem of that file, with
// hyphens for underscores, so even_spread.go runs as even-spread. A path that is
// no Scenario source yields no name rather than a guessed or empty one.
func runNameOf(file string) (string, error) {
	stem, isGoFile := strings.CutSuffix(filepath.Base(file), goFileSuffix)
	if !isGoFile || stem == "" || strings.HasSuffix(stem, testFileStem) {
		return "", fmt.Errorf("%s is no Scenario source, so no run name follows from it", file)
	}

	return strings.ReplaceAll(stem, theWordSeparator, theRunNameSeparator), nil
}
