package lab

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

// A Scenario's function is named <Condition>_Should<Outcome>, and its Scenario
// name writes the words of the condition in lowercase with a hyphen between two.
// The Go runtime writes a function's package path before its name, ending at a
// dot.
const (
	theOutcomeOpener     = "_Should"
	theWordSeparator     = "-"
	thePackagePathEndsAt = "."
	theUnderscore        = '_'
)

// Outcome is what one Scenario reported: the name it ran under, the Steps it
// carried out in order, the Findings its Thens reported, the Placement it settled
// at, and the Step it could not carry out.
type Outcome struct {
	// Name is the Scenario name this Scenario ran under, the condition its
	// function's name states, and Cluster is the Simulated Cluster it ran on, named so a reader
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

// Carried is what carrying out one Scenario of a run left: the Outcome it
// reported, and Err, the lab's own failure while carrying it out. That failure
// is a cluster the lab could not provision, watch or remove, or a panic. A
// Scenario that ended before it reported leaves an Outcome naming it alone.
type Carried struct {
	Outcome Outcome
	Err     error
}

// Reported reports whether the Scenario ran on its Simulated Cluster far enough
// to report an Outcome of its own.
func (c Carried) Reported() bool {
	return c.Outcome.Cluster != ""
}

// Runner runs the lab's Scenarios, each on its own Simulated Cluster, on the
// engine a run resolved once and for the Kubernetes version that run mirrors.
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

// Run derives the Scenario name from the condition its function's name states,
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
// The error it returns is the lab's own: a Scenario name it could not derive, a
// cluster it could not provision, one whose changes it could not watch, or one it
// could not remove. A name it cannot derive stops the run before anything is
// provisioned, so no Simulated Cluster is ever named after nothing. What the
// Scenario found is the Outcome, which a caller reports whether or not the
// removal succeeded.
func (r *Runner) Run(ctx context.Context, steps func(*Scenario)) (Outcome, error) {
	name, err := scenarioNameOfFunction(steps)
	if err != nil {
		return Outcome{}, err
	}

	return r.runNamed(ctx, name, steps)
}

// RunAtOnce carries out every Scenario given at once, each through Run's own
// steps in a goroutine of its own, and hands back what each left in the order
// given once every one has ended, per
// docs/adr/2026-10-04-cluster-per-scenario.md.
//
// It derives every Scenario name before it provisions anything, so a name it
// cannot derive stops the run before any Simulated Cluster exists, as it does
// for Run, and so does a name two Scenarios derive, since their clusters and
// reports would share it. It then obtains the run's Kubernetes version once, so a version the lab cannot
// obtain stops the run, naming it, before any Scenario starts (docs/prd.md
// REQ-SIM-001 edge case 4). Past that, one Scenario's failure is its own: a
// cluster the engine cannot start, a Step left undone or a panic ends that
// Scenario alone.
func (r *Runner) RunAtOnce(ctx context.Context, scenarios []func(*Scenario)) ([]Carried, error) {
	names, err := scenarioNamesOfEach(scenarios)
	if err != nil {
		return nil, err
	}

	if err := cluster.ObtainVersion(ctx, r.engine, r.version); err != nil {
		return nil, err
	}

	return carryOutAtOnce(names, func(declared int) (Outcome, error) {
		return r.runNamed(ctx, names[declared], scenarios[declared])
	}), nil
}

// carryOutAtOnce carries out each Scenario named in a goroutine of its own and
// hands back what each left, in the order named, once every goroutine has
// ended. Each goroutine writes only its own element, and the join is what makes
// those writes visible to the caller, so the report is written after it from
// one goroutine alone.
func carryOutAtOnce(names []string, carryOut func(declared int) (Outcome, error)) []Carried {
	carried := make([]Carried, len(names))

	var each sync.WaitGroup
	for declared, name := range names {
		each.Go(func() {
			defer recoverInto(&carried[declared], name)

			outcome, err := carryOut(declared)
			carried[declared] = Carried{Outcome: outcome, Err: err}
		})
	}
	each.Wait()

	return carried
}

// recoverInto turns a panic of the Scenario named into what that Scenario left.
// It runs as the goroutine unwinds, after Run's own deferred removal, so the
// Scenario's cluster is gone and the other Scenarios run on. A panic left to
// cross the goroutine would end the process before their removals ran,
// leaking their containers. The stack is kept, because a panic is a defect of
// the lab or of a Scenario and the stack is where it is found.
func recoverInto(carried *Carried, name string) {
	recovered := recover()
	if recovered == nil {
		return
	}

	*carried = Carried{
		Outcome: Outcome{Name: name},
		Err:     fmt.Errorf("the Scenario %s panicked: %v\n%s", name, recovered, debug.Stack()),
	}
}

// runNamed is Run for a Scenario whose Scenario name is already derived. Every
// Outcome it returns names the Scenario, including one it hands back with the
// lab's own error before the Scenario reported.
func (r *Runner) runNamed(ctx context.Context, name string, steps func(*Scenario)) (reported Outcome, err error) {
	provisioned, err := cluster.Provision(ctx, r.engine, r.version, name)
	if err != nil {
		return Outcome{Name: name}, fmt.Errorf("the Scenario %s: %w", name, err)
	}

	filed := r.timelines.Open(timeline.Header{
		Scenario: name, Cluster: provisioned.Name(), Kubernetes: r.version.String(),
	})
	// Deferred before the removal below, so it runs after it: the removal stops the
	// recording, and a recording that outlives the file would write into a closed one.
	defer filed.Close()

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

	if err := provisioned.Record(ctx, filed); err != nil {
		return Outcome{Name: name}, err
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

// scenarioNamesOfEach is the Scenario name of each Scenario given, in the order
// given, refusing two that derive one name, naming both functions in that order.
func scenarioNamesOfEach(scenarios []func(*Scenario)) ([]string, error) {
	names := make([]string, 0, len(scenarios))
	derivedBy := make(map[string]string, len(scenarios))
	for _, steps := range scenarios {
		name, err := scenarioNameOfFunction(steps)
		if err != nil {
			return nil, err
		}

		function := functionNameOf(steps)
		if earlier, taken := derivedBy[name]; taken {
			return nil, fmt.Errorf("the Scenarios %s and %s both name the condition %s, so their Simulated Clusters and reports would share one name",
				earlier, function, name)
		}
		derivedBy[name] = function
		names = append(names, name)
	}

	return names, nil
}

// scenarioNameOfFunction is the Scenario name steps runs under. The Go
// runtime's own record of that function's name is what yields it, so a Scenario
// is named by its source alone and no run input reaches the name.
func scenarioNameOfFunction(steps func(*Scenario)) (string, error) {
	function := functionNameOf(steps)
	if function == "" {
		return "", errors.New("the Go runtime holds no name for this Scenario's function, so it has no Scenario name")
	}

	return scenarioNameOf(function)
}

// functionNameOf is the name the Go runtime records for steps, its package path
// included, and empty where it records none.
func functionNameOf(steps func(*Scenario)) string {
	defined := runtime.FuncForPC(reflect.ValueOf(steps).Pointer())
	if defined == nil {
		return ""
	}

	return defined.Name()
}

// scenarioNameOf is the Scenario name a function's runtime name gives: the
// words of the condition before its first _Should, lowercased and joined by
// hyphens, so ANodeThatStopsResponding_Should... runs as
// a-node-that-stops-responding. A closure, a method value, an identifier
// without _Should and a condition holding anything but ASCII letters and digits
// name no condition, and yield no name rather than a guessed one. The name then
// holds lowercase ASCII letters, digits and hyphens alone, because it reaches a
// container name and a Timeline path.
func scenarioNameOf(function string) (string, error) {
	identifier := function[strings.LastIndex(function, thePackagePathEndsAt)+1:]
	condition, _, stated := strings.Cut(identifier, theOutcomeOpener)
	if !stated || condition == "" || !isAnASCIIIdentifier(identifier) || strings.ContainsRune(condition, theUnderscore) {
		return "", fmt.Errorf("the function %s names no condition of ASCII letters and digits before %s, so it has no Scenario name",
			function, theOutcomeOpener)
	}

	return strings.Join(wordsOf(condition), theWordSeparator), nil
}

// isAnASCIIIdentifier reports whether name holds ASCII letters, digits and
// underscores alone, which a closure's or a method value's runtime name does
// not.
func isAnASCIIIdentifier(name string) bool {
	for at := range len(name) {
		if character := name[at]; !isASCIILetterOrDigit(character) && character != theUnderscore {
			return false
		}
	}

	return true
}

// wordsOf is each word of an ASCII condition, lowercased. A word starts at a
// capital after a lower-case letter or a digit, and a run of capitals is one
// word whose last capital starts the next where a lower-case letter follows it,
// so GPUNodes is gpu and nodes.
func wordsOf(condition string) []string {
	var words []string
	start := 0
	for at := 1; at < len(condition); at++ {
		if startsAWord(condition, at) {
			words = append(words, strings.ToLower(condition[start:at]))
			start = at
		}
	}

	return append(words, strings.ToLower(condition[start:]))
}

// startsAWord reports whether the capital at, if it is one, opens a new word of
// condition.
func startsAWord(condition string, at int) bool {
	if !isASCIIUpper(condition[at]) {
		return false
	}
	before := condition[at-1]
	followedByALowerCase := at+1 < len(condition) && isASCIILower(condition[at+1])

	return isASCIILower(before) || isASCIIDigit(before) || followedByALowerCase
}

func isASCIILetterOrDigit(character byte) bool {
	return isASCIIUpper(character) || isASCIILower(character) || isASCIIDigit(character)
}

func isASCIIUpper(character byte) bool { return 'A' <= character && character <= 'Z' }

func isASCIILower(character byte) bool { return 'a' <= character && character <= 'z' }

func isASCIIDigit(character byte) bool { return '0' <= character && character <= '9' }
