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
	"sync/atomic"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

// A Scenario's function is named <Condition>_Should<Outcome>. The Go runtime
// writes a function's package path before its name, ending at a dot.
const (
	theOutcomeOpener     = "_Should"
	theWordSeparator     = "-"
	thePackagePathEndsAt = "."
	theUnderscore        = '_'
)

// Outcome is what a run reports for one Scenario: whether it held, reported a
// Finding or ended at a Step.
type Outcome struct {
	// Cluster is named so a reader can find it while the run is still going.
	Name      string
	Cluster   string
	Steps     []Step
	Findings  []Finding
	Placement scenario.Placement
	// JudgedDeployment names the Deployment the Placement is of, the newest the
	// Scenario declared; it is empty while none is declared.
	JudgedDeployment string
	// EndedAt and Cause are empty for a Scenario that carried out every Step.
	EndedAt Step
	Cause   error
	// A Timeline the lab could not write is the lab's own failure and leaves
	// the rest of this Outcome as reported.
	Timeline scenario.Timeline
	FiledAt  string
	NotFiled error
}

func (o Outcome) Ended() bool {
	return o.Cause != nil
}

// HoldsASettledPlacement is false after a Failure no Wait followed, since the
// Failure discarded the Placement, and before any Deployment's Pods were observed.
func (o Outcome) HoldsASettledPlacement() bool {
	return !o.Ended() && len(o.Placement.Pods()) > 0
}

func (o Outcome) Held() bool {
	return !o.Ended() && len(o.Findings) == 0
}

func (s *Scenario) outcome() Outcome {
	return Outcome{
		Steps:            slices.Clone(s.steps),
		Findings:         slices.Clone(s.findings),
		Placement:        s.observed,
		JudgedDeployment: s.deployment.Name(),
		EndedAt:          s.endedAt,
		Cause:            s.cause,
	}
}

// finish notes the line a further call could still have completed, stops the
// record and gives each Wait Step its narrative.
func (s *Scenario) finish() Outcome {
	s.noteTheOpenLine()
	recorded := s.provisioned.StopRecording()

	finished := s.outcome()
	finished.Timeline = recorded
	finished.Steps = givenTheirNarratives(finished.Steps, recorded.WaitNarratives())

	return finished
}

// The k-th narrative is the k-th Wait Step's, since each recorded Wait is
// noted once. A Wait Step past the narratives keeps none.
func givenTheirNarratives(steps []Step, narratives []scenario.WaitNarrative) []Step {
	narrated := slices.Clone(steps)
	waits := 0
	for at, step := range narrated {
		if step.Kind != WaitStep {
			continue
		}
		if waits < len(narratives) {
			narrated[at].Narrative = narratives[waits]
		}
		waits++
	}

	return narrated
}

// timelineCopy is written by the recording goroutine and read by a Then on the
// Scenario's, so both go through its lock.
type timelineCopy struct {
	guard  sync.Mutex
	copied []scenario.Entry
}

func (c *timelineCopy) Write(entry scenario.Entry) {
	c.guard.Lock()
	defer c.guard.Unlock()

	c.copied = append(c.copied, entry)
}

// A Scenario holding no copy reads an empty Timeline.
func (c *timelineCopy) entries() []scenario.Entry {
	if c == nil {
		return nil
	}

	c.guard.Lock()
	defer c.guard.Unlock()

	return slices.Clone(c.copied)
}

// recordingInto writes each entry into the Timeline file and the Scenario's copy.
type recordingInto struct {
	filed  cluster.EntrySink
	copied *timelineCopy
}

func (r recordingInto) Write(entry scenario.Entry) {
	r.filed.Write(entry)
	r.copied.Write(entry)
}

// Carried pairs a Scenario's Outcome with the lab's own failure while carrying
// it out: a cluster it could not provision, watch or remove, or a panic.
type Carried struct {
	Outcome Outcome
	Err     error
}

func (c Carried) Reported() bool {
	return c.Outcome.Cluster != ""
}

// Runner carries out each Scenario on a Simulated Cluster of its own and files
// its Timeline in the run's folder.
type Runner struct {
	engine    cluster.Endpoint
	version   cluster.KubernetesVersion
	timelines timeline.Folder
}

func NewRunner(engine cluster.Endpoint, version cluster.KubernetesVersion, timelines timeline.Folder) *Runner {
	return &Runner{engine: engine, version: version, timelines: timelines}
}

// Run removes the Scenario's cluster whether the Scenario held, reported a
// Finding or ended at a Step.
//
// The record starts before the first Step and stops after the last, so a
// Timeline holds the Topology coming up as well as the Placement settling. A
// Timeline file the lab could not write does not stop the Scenario and is
// reported on the Outcome.
//
// The returned error is the lab's own failure. A name it cannot derive stops
// the run before anything is provisioned, so no cluster is named after nothing.
func (r *Runner) Run(ctx context.Context, steps func(*Scenario)) (Outcome, error) {
	name, err := scenarioNameOfFunction(steps)
	if err != nil {
		return Outcome{}, err
	}

	return r.runNamed(ctx, name, steps)
}

// RunAtOnce derives every Scenario name and obtains the Kubernetes version
// before provisioning anything, so an underivable or duplicate name, or a
// version the lab cannot obtain, stops the run before any Scenario starts. Past
// that, one Scenario's failure ends that Scenario alone.
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

// Each goroutine writes only its own element, and the join makes those writes
// visible to the caller.
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

// recoverInto runs after Run's own deferred removal, so the Scenario's cluster
// is gone and the other Scenarios run on. A panic left to cross the goroutine
// would end the process before their removals ran, leaking their containers.
// The stack is kept, because a panic is a defect and the stack is where it is
// found.
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

// Observer is handed a Placement board of a held run's cluster once every
// interval, or the cause none was observed.
type Observer struct {
	Every    time.Duration
	Observed func(scenario.PlacementBoard, error)
}

// Hold carries out the Scenario up to its first When, or every Step where it
// has none, and hands its Outcome to reported. It then holds the cluster as the
// Steps left it until ctx ends, injecting each Failure injector is handed, and
// removes it. From provisioning until removal observer is handed the cluster's
// board.
func (r *Runner) Hold(ctx context.Context, steps func(*Scenario), observer Observer, injector *Injector,
	reported func(Outcome),
) (Outcome, error) {
	name, err := scenarioNameOfFunction(steps)
	if err != nil {
		return Outcome{}, err
	}

	return r.holdNamed(ctx, name, steps, observer, injector, reported)
}

func (r *Runner) holdNamed(ctx context.Context, name string, steps func(*Scenario), observer Observer,
	injector *Injector, reported func(Outcome),
) (Outcome, error) {
	return r.carryOut(ctx, name, steps, &holding{observer: observer, injector: injector, reported: reported})
}

// holding is what sets a held run apart from a run: the halt at the first
// When, the observer, the clicked Failures and the wait for the lab to stop.
type holding struct {
	observer Observer
	injector *Injector
	reported func(Outcome)
}

// Injector takes the clicks an engineer makes in the Live view into a held
// run's cluster, one at a time, from its Outcome being reported until the lab
// stops: each stops a Node or Zone responding, makes it respond again, or loses
// a Pod. Before and after, a click changes nothing.
//
// Each Failure is resolved against what the Steps declared and the latest
// board, and runs under the held run's context rather than a request's, so a
// closed browser tab leaves no Zone stopped part-way.
type Injector struct {
	mu sync.Mutex
	// held is set while the held run takes Failures, and nil otherwise.
	held *heldCluster
	// reportWhen is called under mu, so the When of one Failure prints whole
	// before the next is taken.
	reportWhen func(Step)
	// latest is written by the observer while a click reads it.
	latest atomic.Pointer[scenario.PlacementBoard]
}

// heldCluster is the cluster a held run's Failures reach and the Nodes stopped
// in it so far.
type heldCluster struct {
	ctx         context.Context
	provisioned *cluster.SimulatedCluster
	topology    scenario.Topology
	stopped     []string
}

// NewInjector reports each click it takes as a When to reportWhen.
func NewInjector(reportWhen func(Step)) *Injector {
	return &Injector{reportWhen: reportWhen}
}

// takeFailures reports the Outcome under the lock, so a click arriving as the
// Steps end is taken once its Outcome is on the terminal, never before.
func (i *Injector) takeFailures(held *heldCluster, report func()) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.held = held
	report()
}

// stopTakingFailures waits for a Failure under way to end, so none reaches a
// cluster being removed.
func (i *Injector) stopTakingFailures() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.held = nil
}

func (i *Injector) observed(board scenario.PlacementBoard) {
	i.latest.Store(&board)
}

// ClickNode stops the Node named responding, or makes it respond again where it
// stopped. A name the Topology does not derive changes nothing.
func (i *Injector) ClickNode(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.held == nil {
		return nil
	}
	failure, err := scenario.NewNodeFailureNaming(i.held.topology, name)
	if err != nil {
		return nil
	}

	if slices.Contains(i.held.stopped, name) {
		return i.restoring(failure.Names(), "Node "+name)
	}

	return i.stopping(failure.Names(), "Node "+name)
}

// ClickZone stops the Nodes of the Zone named that still respond, in the
// Topology's order, or makes every Node of it respond again where all of them
// stopped. A name the Topology does not derive changes nothing.
func (i *Injector) ClickZone(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.held == nil {
		return nil
	}
	failure, err := scenario.NewZoneFailureNaming(i.held.topology, name)
	if err != nil {
		return nil
	}

	stillResponding := failure.NodeNamesStillResponding(i.held.stopped)
	if len(stillResponding) == 0 {
		return i.restoring(failure.NodeNames(), "Zone "+name)
	}

	return i.stopping(stillResponding, "Zone "+name)
}

// LosePod loses the placed Pod named, its Deployment replacing it. A Pod the
// latest board holds unplaced or not at all, or one already gone, changes
// nothing.
func (i *Injector) LosePod(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	board := i.latest.Load()
	if i.held == nil || board == nil {
		return nil
	}
	failure, err := scenario.NewPodFailure(*board, name)
	if err != nil {
		return nil
	}

	lost, err := i.held.provisioned.LosePod(i.held.ctx, failure.Name())
	if err != nil {
		return fmt.Errorf("lose the Pod %s clicked: %w", name, err)
	}
	if lost {
		i.reportWhen(Step{Kind: WhenStep, Name: "Pod " + failure.BoardName() + " is lost"})
	}

	return nil
}

// stopping and restoring are called under mu, and clicked names the Node or
// Zone as its When does.
func (i *Injector) stopping(nodes []string, clicked string) error {
	if err := i.held.stop(nodes); err != nil {
		return err
	}
	i.reportWhen(Step{Kind: WhenStep, Name: clicked + " stops responding"})

	return nil
}

func (i *Injector) restoring(nodes []string, clicked string) error {
	if err := i.held.restore(nodes); err != nil {
		return err
	}
	i.reportWhen(Step{Kind: WhenStep, Name: clicked + " responds again"})

	return nil
}

// stop keeps every Node it stopped among the stopped, even where a later one
// fails, so a further click makes it respond again rather than stopping it.
func (h *heldCluster) stop(nodes []string) error {
	for _, node := range nodes {
		if err := h.provisioned.FailNode(h.ctx, node); err != nil {
			return fmt.Errorf("stop Node %s responding: %w", node, err)
		}
		h.stopped = append(h.stopped, node)
	}

	return nil
}

// restore takes every Node it made respond again out of the stopped, even
// where a later one fails, so a further click stops it rather than restoring it.
func (h *heldCluster) restore(nodes []string) error {
	for _, node := range nodes {
		if err := h.provisioned.RestoreNode(h.ctx, node); err != nil {
			return fmt.Errorf("make Node %s respond again: %w", node, err)
		}
		h.stopped = slices.DeleteFunc(h.stopped, func(stopped string) bool { return stopped == node })
	}

	return nil
}

func (r *Runner) runNamed(ctx context.Context, name string, steps func(*Scenario)) (Outcome, error) {
	return r.carryOut(ctx, name, steps, nil)
}

// Every Outcome carryOut returns names the Scenario, including one returned
// with the lab's own error before the Scenario reported.
func (r *Runner) carryOut(ctx context.Context, name string, steps func(*Scenario), held *holding) (reported Outcome, err error) {
	provisioned, err := cluster.Provision(ctx, r.engine, r.version, name)
	if err != nil {
		return Outcome{Name: name}, fmt.Errorf("the Scenario %s: %w", name, err)
	}

	filed := r.timelines.Open(timeline.Header{
		Scenario: name, Cluster: provisioned.Name(), Kubernetes: r.version.String(),
	})
	// Deferred before the removal below, so it runs after it: the removal stops
	// the recording, and a recording that outlives the file would write into a
	// closed one.
	defer filed.Close()

	// The removal is deferred here so a Scenario that ends part-way still loses
	// its cluster. The context is detached from ctx so that a cancelled run
	// still removes what it created.
	defer func() {
		removed := provisioned.Close(context.WithoutCancel(ctx))
		if removed != nil && err == nil {
			err = fmt.Errorf("remove the Simulated Cluster of the Scenario %s: %w", name, removed)
		}
	}()

	copied := &timelineCopy{}
	if err := provisioned.Record(ctx, recordingInto{filed: filed, copied: copied}); err != nil {
		return Outcome{Name: name}, err
	}

	running := newScenario(ctx, provisioned, copied)
	if held != nil {
		running.haltsAtTheFirstWhen = true
		// Deferred after the removal, so it runs before it: no observation
		// reaches a removed cluster.
		defer held.observer.observe(ctx, provisioned, &running.published, held.injector.observed)()
	}
	steps(running)

	reported = running.finish()
	filed.Close()

	reported.Name, reported.Cluster = name, provisioned.Name()
	reported.FiledAt, reported.NotFiled = filed.Path(), filed.Err()

	if held != nil {
		held.injector.takeFailures(
			&heldCluster{ctx: ctx, provisioned: provisioned, topology: running.topology, stopped: slices.Clone(running.failed)},
			func() { held.reported(reported) })
		<-ctx.Done()
		held.injector.stopTakingFailures()
	}

	return reported, nil
}

// observe starts one goroutine observing the cluster at the observer's interval
// and returns the function that stops it and waits for it to end. Before a
// Topology is declared nothing is handed over, and each board marks its
// newcomers against the one before.
func (o Observer) observe(ctx context.Context, provisioned *cluster.SimulatedCluster,
	declared *atomic.Pointer[scenario.Topology], latest func(scenario.PlacementBoard),
) (stop func()) {
	observing, stopObserving := context.WithCancel(ctx)
	var ended sync.WaitGroup
	ended.Go(func() {
		ticks := time.NewTicker(o.Every)
		defer ticks.Stop()

		var previous scenario.PlacementBoard
		for {
			select {
			case <-observing.Done():
				return
			case <-ticks.C:
			}

			topology := declared.Load()
			if topology == nil {
				continue
			}
			placed, err := observeOnce(observing, provisioned)
			if observing.Err() != nil {
				return
			}
			if err != nil {
				o.Observed(scenario.PlacementBoard{}, err)

				continue
			}
			previous = scenario.NewPlacementBoard(*topology, placed, previous)
			latest(previous)
			o.Observed(previous, nil)
		}
	})

	return func() {
		stopObserving()
		ended.Wait()
	}
}

// Each observation is held to the lab's waiting time, as a Wait's board is.
func observeOnce(ctx context.Context, provisioned *cluster.SimulatedCluster) (scenario.Placement, error) {
	observing, cancel := context.WithTimeout(ctx, scenario.WaitingTime)
	defer cancel()

	placed, err := provisioned.ObserveEveryDeployment(observing)
	if err != nil {
		return scenario.Placement{}, fmt.Errorf("observe the Placement board: %w", err)
	}

	return placed, nil
}

// ScenarioNamed refuses a name matching no Scenario with every Scenario name
// in the order given, so the engineer reads what can be asked for.
func ScenarioNamed(scenarios []func(*Scenario), name string) (func(*Scenario), error) {
	names, err := ScenarioNames(scenarios)
	if err != nil {
		return nil, err
	}

	at := slices.Index(names, name)
	if at < 0 {
		return nil, fmt.Errorf("no Scenario is named %q; the Scenarios are: %s", name, strings.Join(names, ", "))
	}

	return scenarios[at], nil
}

// ScenarioNames refuses an underivable or duplicate name as a run of every
// Scenario does.
func ScenarioNames(scenarios []func(*Scenario)) ([]string, error) {
	return scenarioNamesOfEach(scenarios)
}

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

// The name comes from the runtime's record of the function alone, so no run
// input reaches it.
func scenarioNameOfFunction(steps func(*Scenario)) (string, error) {
	function := functionNameOf(steps)
	if function == "" {
		return "", errors.New("the Go runtime holds no name for this Scenario's function, so it has no Scenario name")
	}

	return scenarioNameOf(function)
}

func functionNameOf(steps func(*Scenario)) string {
	defined := runtime.FuncForPC(reflect.ValueOf(steps).Pointer())
	if defined == nil {
		return ""
	}

	return defined.Name()
}

// scenarioNameOf turns ANodeThatStopsResponding_Should... into
// a-node-that-stops-responding. A closure, a method value, a name without
// _Should, or a condition holding anything but ASCII letters and digits yields
// no name rather than a guessed one, because the name reaches a container name
// and a Timeline path.
func scenarioNameOf(function string) (string, error) {
	identifier := function[strings.LastIndex(function, thePackagePathEndsAt)+1:]
	condition, _, stated := strings.Cut(identifier, theOutcomeOpener)
	if !stated || condition == "" || !isAnASCIIIdentifier(identifier) || strings.ContainsRune(condition, theUnderscore) {
		return "", fmt.Errorf("the function %s names no condition of ASCII letters and digits before %s, so it has no Scenario name",
			function, theOutcomeOpener)
	}

	return strings.Join(wordsOf(condition), theWordSeparator), nil
}

func isAnASCIIIdentifier(name string) bool {
	for at := range len(name) {
		if character := name[at]; !isASCIILetterOrDigit(character) && character != theUnderscore {
			return false
		}
	}

	return true
}

// A run of capitals is one word whose last capital starts the next where a
// lower-case letter follows it, so GPUNodes is gpu and nodes.
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
