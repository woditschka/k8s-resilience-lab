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
	// the rest of this Outcome as reported. The Outcome a held run reports at
	// its hold carries none of the three, since the record runs on until the
	// lab stops.
	Timeline scenario.Timeline
	FiledAt  string
	NotFiled error
}

func (o Outcome) Ended() bool {
	return o.Cause != nil
}

// StepLeftUndone names the Step the Scenario ended at and why, wrapping the
// cause, and is nil for a Scenario that carried out every Step. A run's exit
// and a held cluster's terminal name an ending in these words alike.
func (o Outcome) StepLeftUndone() error {
	if !o.Ended() {
		return nil
	}

	return fmt.Errorf("the Scenario %s ended at %s: %w", o.Name, o.EndedAt, o.Cause)
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

// finish stops the record and gives each Wait Step its narrative from it.
func (s *Scenario) finish() Outcome {
	s.endTheSteps()
	recorded := s.provisioned.StopRecording()

	finished := s.narrated(recorded)
	finished.Timeline = recorded

	return finished
}

// hold gives each Wait Step its narrative from the Timeline recorded so far and
// leaves the record running, so the changes while the cluster is held join it.
// It hands back that Timeline, the channel carrying each later entry, and the
// channel closed as the Descheduler's interval stops while held. That channel
// is nil where the Outcome already ends at the interval's stop, so the stop is
// named once.
func (s *Scenario) hold() (Outcome, scenario.Timeline, <-chan scenario.Entry, <-chan struct{}) {
	var stopsWhileHeld <-chan struct{}
	if !s.endTheSteps() {
		stopsWhileHeld = s.descheduling.aPassFailed()
	}
	soFar, later := s.provisioned.FollowTimeline()

	return s.narrated(soFar), soFar, later, stopsWhileHeld
}

// endTheSteps notes the line a further call could still have completed, and
// reports whether the Descheduler's interval had stopped by then.
func (s *Scenario) endTheSteps() (intervalStopped bool) {
	s.noteTheOpenLine()

	return s.endAtAStoppedInterval()
}

// endAtAStoppedInterval ends the Scenario at the Given that declared the
// Descheduler's interval where a pass on it failed after the last Step.
func (s *Scenario) endAtAStoppedInterval() (stopped bool) {
	failed := s.descheduling.failure()
	if failed == nil {
		return false
	}
	s.end(s.deschedulerDeclaredAt, failed)

	return true
}

// intervalStop is how the Descheduler's interval stopping ends the Scenario
// named, kept apart from the Scenario's own ending, which keeps its first stop.
func (s *Scenario) intervalStop(name string) Outcome {
	return Outcome{Name: name, EndedAt: s.deschedulerDeclaredAt, Cause: s.descheduling.failure()}
}

func (s *Scenario) narrated(recorded scenario.Timeline) Outcome {
	narrated := s.outcome()
	narrated.Steps = givenTheirNarratives(narrated.Steps, recorded.WaitNarratives())

	return narrated
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
// The returned error is the lab's own failure. A name it cannot derive, or a
// Descheduler policy it declares on a version the lab pairs no Descheduler
// with, stops the run before anything is provisioned, so no cluster is named
// after nothing. A Scenario declaring a policy has its Descheduler's image
// obtained before it is provisioned.
func (r *Runner) Run(ctx context.Context, steps func(*Scenario)) (Outcome, error) {
	name, err := scenarioNameOfFunction(steps)
	if err != nil {
		return Outcome{}, err
	}
	image, err := r.obtainTheDeschedulerOf(ctx, []func(*Scenario){steps})
	if err != nil {
		return Outcome{}, err
	}

	return r.runNamed(ctx, name, image, steps)
}

// RunAtOnce derives every Scenario name, pairs and obtains the Descheduler
// and obtains the Kubernetes version before provisioning anything, so an
// underivable or duplicate name, a Descheduler policy declared on a version
// the lab pairs no Descheduler with, or a version the lab cannot obtain, stops
// the run before any Scenario starts. Past that, one Scenario's failure ends
// that Scenario alone, and a Descheduler image the lab could not obtain ends
// each Scenario declaring a policy.
func (r *Runner) RunAtOnce(ctx context.Context, scenarios []func(*Scenario)) ([]Carried, error) {
	names, err := scenarioNamesOfEach(scenarios)
	if err != nil {
		return nil, err
	}
	image, err := r.obtainTheDeschedulerOf(ctx, scenarios)
	if err != nil {
		return nil, err
	}

	if err := cluster.ObtainVersion(ctx, r.engine, r.version); err != nil {
		return nil, err
	}

	return carryOutAtOnce(names, func(declared int) (Outcome, error) {
		return r.runNamed(ctx, names[declared], image, scenarios[declared])
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
// board. The Timeline records until ctx ends and is filed then, so the Outcome
// returned carries it and the Outcome reported does not.
//
// Each line scenario.Following reads from the entries recorded while held is
// handed to followed, after the Outcome and in order, from one goroutine. The
// lab's stop waits for the lines of the entries recorded before it, but no
// longer than followingStopWait. No line starts once Hold returns, but a line
// followed is still taking by then may finish after it, so a terminal that
// stopped taking lines keeps no cluster from being removed.
//
// A Scenario declaring a Descheduler policy has its image obtained, as for a
// run, before the cluster is provisioned.
//
// A Descheduler interval that stops while the cluster is held is named to
// followed as the Scenario ending at the Given that declared it, and the
// Outcome returned ends there. The cluster stays held until ctx ends.
func (r *Runner) Hold(ctx context.Context, steps func(*Scenario), observer Observer, injector *Injector,
	reported func(Outcome), followed func(string),
) (Outcome, error) {
	name, err := scenarioNameOfFunction(steps)
	if err != nil {
		return Outcome{}, err
	}
	image, err := r.obtainTheDeschedulerOf(ctx, []func(*Scenario){steps})
	if err != nil {
		return Outcome{}, err
	}

	return r.holdNamed(ctx, name, image, steps, observer, injector, reported, followed)
}

// deschedulerToObtain names the Descheduler a run obtains before any Scenario
// starts, which is none where no Scenario declares a policy, so a run declaring
// none keeps every Kubernetes version and fetches nothing for it. A declaring
// run on a version the lab pairs no Descheduler with is stopped.
func (r *Runner) deschedulerToObtain(scenarios []func(*Scenario)) (paired cluster.DeschedulerVersion, obtains bool, err error) {
	if !slices.ContainsFunc(scenarios, declaresADeschedulerPolicy) {
		return cluster.DeschedulerVersion{}, false, nil
	}

	paired, unpaired := r.version.PairedDescheduler()
	if unpaired != nil {
		return cluster.DeschedulerVersion{}, false, fmt.Errorf("a Scenario of this run declares a Descheduler policy: %w", unpaired)
	}

	return paired, true, nil
}

// obtainTheDeschedulerOf makes the run's one attempt at the image of the
// Descheduler deschedulerToObtain names, and makes none where it names none.
// The attempt's failure is handed to each Scenario rather than returned, so
// the Scenarios declaring no policy run on; the error returned is the stop.
func (r *Runner) obtainTheDeschedulerOf(ctx context.Context, scenarios []func(*Scenario)) (deschedulerImage, error) {
	paired, obtains, err := r.deschedulerToObtain(scenarios)
	if err != nil || !obtains {
		return deschedulerImage{}, err
	}

	return deschedulerImageAttempted(cluster.ObtainDescheduler(ctx, r.engine, paired)), nil
}

// declaresADeschedulerPolicy runs the Scenario's function over a Scenario
// holding no cluster, which carries out its Givens alone: a Given applies
// nothing, and every other Step records itself before it reaches a cluster.
func declaresADeschedulerPolicy(steps func(*Scenario)) bool {
	declaring := &Scenario{declaringOnly: true}
	steps(declaring)

	return declaring.declaresADeschedulerPolicy
}

func (r *Runner) holdNamed(ctx context.Context, name string, image deschedulerImage, steps func(*Scenario), observer Observer,
	injector *Injector, reported func(Outcome), followed func(string),
) (Outcome, error) {
	return r.carryOut(ctx, name, image, steps, &holding{observer: observer, injector: injector, reported: reported, followed: followed})
}

// holding is what sets a held run apart from a run: the halt at the first
// When, the observer, the clicked Failures, the lines followed and the wait for
// the lab to stop.
type holding struct {
	observer Observer
	injector *Injector
	reported func(Outcome)
	followed func(string)
}

// hold reports the Outcome at the hold, then follows the Timeline and takes the
// clicks until ctx ends. The record stops once no click is under way, so no
// click notes into a stopped record, and its stop ends the following.
//
// The Descheduler's interval is stopped and joined before its failure is read,
// so a pass failing as the lab stops still ends the Scenario at the Given that
// declared the interval.
func (h *holding) hold(ctx context.Context, running *Scenario, name string) Outcome {
	held, soFar, later, stopsWhileHeld := running.hold()
	held.Name, held.Cluster = name, running.provisioned.Name()

	h.injector.takeFailures(
		&heldCluster{ctx: ctx, provisioned: running.provisioned, topology: running.topology, stopped: slices.Clone(running.failed)},
		func() { h.reported(held) })
	stopTheFollowing := h.follow(soFar, later, stopsWhileHeld, func() string {
		return running.intervalStop(name).StepLeftUndone().Error()
	})
	<-ctx.Done()
	h.injector.stopTakingFailures()
	running.stopDescheduling()
	running.endAtAStoppedInterval()
	held.EndedAt, held.Cause = running.endedAt, running.cause

	held.Timeline = running.provisioned.StopRecording()
	stopTheFollowing()

	return held
}

// follow starts once the Outcome is reported, so no line comes before it, and
// returns the function that ends it once the record has stopped. The record
// waits on this reader until the lab stops, then hands it nothing more, and
// the function returned waits in turn for the lines of what it was handed, as
// linesFollowed bounds it. The interval's stop is named through the same
// reader, so it takes its place among the lines and holds up the lab's stop on
// a stalled terminal no longer than they do.
func (h *holding) follow(soFar scenario.Timeline, later <-chan scenario.Entry,
	intervalStopped <-chan struct{}, intervalStop func() string,
) (stop func()) {
	lines := &linesFollowed{read: make(chan struct{})}
	go func() {
		defer close(lines.read)
		following := scenario.NewFollowing(soFar)
		for {
			var handed []string
			select {
			case entry, recording := <-later:
				if !recording {
					return
				}
				handed = following.Read(entry)
			case <-intervalStopped:
				// Named once: a nil channel is never ready again.
				intervalStopped = nil
				handed = []string{intervalStop()}
			}
			if !lines.hand(handed, h.followed) {
				return
			}
		}
	}()

	return lines.stop
}

// followingStopWait bounds how long the lab's stop waits for the lines of the
// entries handed before it, so a terminal that stopped taking lines delays the
// cluster's removal by no more.
const followingStopWait = 2 * time.Second

// linesFollowed lets the stop wait for every line of the entries the reader
// was handed before it, and never longer than followingStopWait for a
// terminal that stopped taking lines: then no line starts, and one already
// printing is left to finish on its own.
type linesFollowed struct {
	stopped atomic.Bool
	// read is closed once the reader has nothing more to hand.
	read chan struct{}
}

// hand reports false once the stop gave up waiting before a line, and nothing
// more is to be handed.
func (l *linesFollowed) hand(lines []string, followed func(string)) bool {
	for _, line := range lines {
		if l.stopped.Load() {
			return false
		}
		followed(line)
	}

	return true
}

func (l *linesFollowed) stop() {
	waited := time.NewTimer(followingStopWait)
	defer waited.Stop()

	select {
	case <-l.read:
	case <-waited.C:
		l.stopped.Store(true)
	}
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

// NewInjector notes each click it takes as a When into the held cluster's
// Timeline, which is where the terminal reads it from, or as its Failure not
// injected where it could not be, wholly or in part.
func NewInjector() *Injector {
	return &Injector{}
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

// ClickZone stops the Nodes of the Zone named that still respond, or makes
// every Node of it respond again where all of them stopped. A name the
// Topology does not derive changes nothing.
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

	// A Pod gone between the board and the deletion is lost already, so its When
	// is noted all the same and the terminal and the Timeline name the click
	// alike.
	return i.held.inject(Step{Kind: WhenStep, Name: "Pod " + failure.BoardName() + " is lost"}, func() error {
		if _, err := i.held.provisioned.LosePod(i.held.ctx, failure.Name()); err != nil {
			return fmt.Errorf("lose the Pod %s clicked: %w", name, err)
		}

		return nil
	})
}

// stopping and restoring are called under mu, and clicked names the Node or
// Zone as its When does.
func (i *Injector) stopping(nodes []string, clicked string) error {
	return i.held.inject(Step{Kind: WhenStep, Name: clicked + " stops responding"}, func() error {
		return i.held.stop(nodes)
	})
}

func (i *Injector) restoring(nodes []string, clicked string) error {
	return i.held.inject(Step{Kind: WhenStep, Name: clicked + " responds again"}, func() error {
		return i.held.restore(nodes)
	})
}

// errInjectionUnfinished is why a Failure whose injection never returned, one
// that panicked and the HTTP server recovered from, is noted not injected.
var errInjectionUnfinished = errors.New("its injection did not return")

// inject holds the When's note open from before the Failure's first change
// until the injection returns, so the changes it made stand after it. The note
// is then given as the When, or as the Failure named not injected and why in its
// place. It is given on every return, a panic included, since a note never
// given holds every later entry back from the terminal.
func (h *heldCluster) inject(when Step, failure func() error) error {
	held := h.provisioned.HoldStepNote()
	injected := errInjectionUnfinished
	defer func() { held.Note(noteOf(when, injected)) }()

	injected = failure()

	return injected
}

// noteOf words the note as the terminal prints it: the When, or the Failure
// it names with why it was not injected.
func noteOf(when Step, injected error) (step, line string) {
	if injected == nil {
		return string(when.Kind), when.String()
	}

	return scenario.NotInjectedNamed, scenario.NotInjectedNamed + ": " + when.Name + ", because " + injected.Error()
}

// stop keeps every Node it stopped among the stopped, even where another
// fails, so a further click makes it respond again rather than stopping it.
func (h *heldCluster) stop(nodes []string) error {
	stopped, err := h.provisioned.FailNodes(h.ctx, nodes)
	h.stopped = append(h.stopped, stopped...)

	return err
}

// restore takes every Node it made respond again out of the stopped, even
// where another fails, so a further click stops it rather than restoring it.
func (h *heldCluster) restore(nodes []string) error {
	restored, err := h.provisioned.RestoreNodes(h.ctx, nodes)
	h.stopped = slices.DeleteFunc(h.stopped, func(stopped string) bool { return slices.Contains(restored, stopped) })

	return err
}

func (r *Runner) runNamed(ctx context.Context, name string, image deschedulerImage, steps func(*Scenario)) (Outcome, error) {
	return r.carryOut(ctx, name, image, steps, nil)
}

// Every Outcome carryOut returns names the Scenario, including one returned
// with the lab's own error before the Scenario reported.
func (r *Runner) carryOut(ctx context.Context, name string, image deschedulerImage, steps func(*Scenario), held *holding,
) (reported Outcome, err error) {
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
	running.kubernetes, running.deschedulerImage = r.version, image
	// Deferred after the removal, so it runs before it: no pass starts on a
	// removed cluster, and Close finds no pass but one already held.
	defer running.stopDescheduling()
	if held != nil {
		running.haltsAtTheFirstWhen = true
		// Deferred after the removal, so it runs before it: no observation
		// reaches a removed cluster.
		defer held.observer.observe(ctx, provisioned, &running.published, held.injector.observed)()
	}
	steps(running)

	if held != nil {
		reported = held.hold(ctx, running, name)
	} else {
		reported = running.finish()
	}
	filed.Close()

	reported.Name, reported.Cluster = name, provisioned.Name()
	reported.FiledAt, reported.NotFiled = filed.Path(), filed.Err()

	return reported, nil
}

// deschedulerImage is what the run's one attempt at the paired Descheduler's
// image, made before any Scenario starts, left. Its zero value is no attempt at
// all, which a Scenario declaring a policy refuses as it refuses a failed one,
// so a run that skips the attempt fails on every machine, one already holding
// the image included.
type deschedulerImage struct {
	attempted bool
	failed    error
}

func deschedulerImageAttempted(failed error) deschedulerImage {
	return deschedulerImage{attempted: true, failed: failed}
}

// unobtained is nil once the image is held, and why it is not otherwise.
func (i deschedulerImage) unobtained() error {
	if !i.attempted {
		return errors.New("the run obtained no Descheduler image before its Scenarios started")
	}

	return i.failed
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
