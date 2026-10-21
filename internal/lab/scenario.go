// Package lab holds the Step surface a Scenario's lines run against, and the
// runner that provisions a Simulated Cluster for each Scenario.
//
// It is the one package naming both a Step and the Simulated Cluster, so a
// Scenario source imports it, the pod package and the node package, and nothing
// else of the lab.
package lab

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const noPod = 0

const (
	// A Wait's line states the time it waited to the tenth of a second, the
	// precision a reader of a restore time acts on.
	noWait                = 0
	reportedWaitPrecision = 100 * time.Millisecond
	theWaitPrefix         = "Wait for"

	// A Wait's line names which of its two times follows, so a reader never
	// reads the time it took for the time it was held to.
	theTimeReached    = ", reached in "
	theTimeNotReached = ", not reached in "

	// The lab's own waiting time carries no mark, since a run reports it at its
	// start rather than on every line.
	theTimeHeldTo       = " of "
	theStatedTimeBefore = "the "
	theStatedTimeAfter  = " stated"
)

const (
	// allPodsReady is the condition kubectl rollout status waits on.
	allNodesReady = "all Nodes Ready"
	allPodsReady  = "all Pods Ready"

	// theSettledPlacement is the lab's own state rather than a Kubernetes
	// condition, because it holds for a Placement that has stopped changing
	// whether its Pods became Ready or stayed Pending.
	theSettledPlacement = "the Placement Settled"

	theWaitThatNeverStood = "no Wait stands before it"

	// The Step's own line states both times, so the cause does not repeat them.
	theWaitReachedTooLate = "this Wait states the time it must be reached in, and the cluster reached it later"
)

// Zones is a count rather than a list of names, so no Scenario names a Zone its
// Givens do not derive.
type Zones int

type Pods int

// Nodes is a count rather than a list of names, so no Scenario names a Node its
// Givens do not derive.
type Nodes int

// WorkloadName is also the label value the Workload's Pods carry.
type WorkloadName string

// The types above carry no invariant of their own: a swapped argument does not
// compile, and a value a Scenario may not declare is refused by the
// internal/scenario constructor the Step calls.

// StepKind is which of Given, When, Wait or Then a Step is, and so how its line
// opens.
type StepKind string

const (
	GivenStep StepKind = "Given"
	WhenStep  StepKind = "When"
	WaitStep  StepKind = scenario.WaitStepNamed
	ThenStep  StepKind = "Then"
)

func (k StepKind) opens() string {
	if k == WaitStep {
		return theWaitPrefix
	}

	return string(k)
}

// Step is one line of a Scenario as the run carried it out, kept for the
// report.
type Step struct {
	Kind StepKind
	Name string
	// Waited stays out of the name so a Step's name stays reproducible. It is
	// zero for a Wait the cluster never reached and for every other Step.
	Waited time.Duration
	// HeldTo is the time the line states, or the lab's own waiting time where
	// it states none.
	HeldTo     time.Duration
	TimeStated bool
	// Reached is recorded rather than derived from the two times, because a
	// Wait with nothing to wait for is reached in no measured time and a Wait
	// never reached measures none either.
	Reached bool
	// Settings stay out of the name, so a Finding and the Step a Scenario ended
	// at stay one short line.
	Settings []string
	// Narrative and Board are a Wait's alone. The Narrative is given once the
	// record has stopped; the Board is observed as the Wait ended, reached or
	// not, and BoardNotObserved says why a Wait has none.
	Narrative        scenario.WaitNarrative
	Board            scenario.PlacementBoard
	BoardNotObserved error
}

func (s Step) String() string {
	line := s.Kind.opens() + " " + s.Name
	if s.HeldTo == noWait {
		return line
	}
	if !s.Reached {
		return line + theTimeNotReached + s.timeHeldTo()
	}

	took := line + theTimeReached + s.Waited.Round(reportedWaitPrecision).String()
	if !s.TimeStated {
		return took
	}

	return took + theTimeHeldTo + s.timeHeldTo()
}

// A stated time is marked as stated, so a reader never takes the lab's own
// waiting time for a time a Scenario chose.
func (s Step) timeHeldTo() string {
	if !s.TimeStated {
		return s.HeldTo.String()
	}

	return theStatedTimeBefore + s.HeldTo.String() + theStatedTimeAfter
}

// Finding is a Then the Settled Placement did not meet: an observation about
// Kubernetes, not an error in the lab.
type Finding struct {
	Step     Step
	Mismatch string
}

// Scenario is the Step surface a Scenario's lines run against. No Step returns
// an error: the Scenario keeps the first refused declaration and every later
// Step then carries nothing out.
//
// A Scenario holds the context its cluster work runs under, because a
// Scenario's lines take nothing but the Scenario itself.
type Scenario struct {
	ctx         context.Context
	provisioned *cluster.SimulatedCluster

	topology scenario.Topology
	// accesses sit beside the Workload, because the domain value carries no
	// function value.
	workload scenario.Workload
	accesses []cluster.PodSpecAccessInPlace

	// undeployed are earlier Workloads a later Given superseded before a Wait
	// deployed them; the next Wait deploys them before the newest.
	undeployed        []declaredDeployment
	declaredWorkloads []WorkloadName

	steps    []Step
	findings []Finding
	// lineOpen is a Given or Then a further call may still complete, so its
	// note waits for the next Step or the Scenario's end.
	lineOpen bool

	failed      []string
	failedZones []string

	// runningAtTheFailure is derived when a When discards the Placement. A
	// later When overwrites it, so a Then judges the Failure standing before
	// it.
	runningAtTheFailure int

	// A Wait applies what the Givens above it declared, so its cause names a
	// declaration by the line that wrote it.
	topologyDeclaredAs string
	workloadDeclaredAs string

	observed scenario.Placement
	// board is the one the last Wait drew, which the next marks its newcomers
	// against.
	board scenario.PlacementBoard
	// settled is cleared by a When alone, so the next Wait applies nothing and
	// holds for the world the Failure left.
	topologyApplied  bool
	workloadDeployed bool
	settled          bool
	// changedTheCluster is set before the first change is attempted, so a
	// change that fails partway still has its board observed.
	changedTheCluster bool

	endedAt Step
	cause   error
}

type declaredDeployment struct {
	workload   scenario.Workload
	accesses   []cluster.PodSpecAccessInPlace
	declaredAs string
}

func newScenario(ctx context.Context, provisioned *cluster.SimulatedCluster) *Scenario {
	return &Scenario{ctx: ctx, provisioned: provisioned}
}

func (s *Scenario) Given() Given {
	return Given{scenario: s}
}

func (s *Scenario) When() When {
	return When{scenario: s}
}

func (s *Scenario) WaitFor() Wait {
	return Wait{scenario: s}
}

func (s *Scenario) Then() Then {
	return Then{scenario: s}
}

// record reports false once the Scenario has ended, recording nothing. A When
// is noted at once, before it stops its first Node, and a Wait as it ends.
func (s *Scenario) record(kind StepKind, name string) (Step, bool) {
	s.noteTheOpenLine()
	if s.cause != nil {
		return Step{}, false
	}

	step := Step{Kind: kind, Name: name}
	s.steps = append(s.steps, step)
	switch kind {
	case GivenStep, ThenStep:
		s.lineOpen = true
	case WhenStep:
		s.note(step)
	case WaitStep:
		// Noted as it ends, after its board.
	}

	return step, true
}

func (s *Scenario) noteTheOpenLine() {
	if !s.lineOpen {
		return
	}

	s.lineOpen = false
	s.note(s.steps[len(s.steps)-1])
}

func (s *Scenario) note(step Step) {
	s.provisioned.NoteStep(string(step.Kind), step.String())
}

// rename exists because an optional setting is a further call on the same line,
// not a line of its own.
func (s *Scenario) rename(name string) Step {
	last := len(s.steps) - 1
	s.steps[last].Name = name

	return s.steps[last]
}

func (s *Scenario) carrying(settings []string) Step {
	last := len(s.steps) - 1
	s.steps[last].Settings = settings

	return s.steps[last]
}

func (s *Scenario) waited(held scenario.StatedWait, took time.Duration, reached bool) Step {
	last := len(s.steps) - 1
	s.steps[last].HeldTo, s.steps[last].TimeStated = held.HeldTo(), held.Stated()
	s.steps[last].Waited, s.steps[last].Reached = took, reached

	return s.steps[last]
}

// The first stop is kept, so the Step named is the one a reader looks at.
func (s *Scenario) end(step Step, cause error) {
	if s.cause != nil {
		return
	}

	s.endedAt = step
	s.cause = cause
}

// A Scenario that has not waited since the Givens, or since the Failure that
// discarded the Placement, has none and ends at the Step that read for one.
func (s *Scenario) placement(at Step) (scenario.Placement, bool) {
	if !s.settled {
		s.end(at, errors.New("this Step reads the Placement a Wait reached, and "+theWaitThatNeverStood))

		return scenario.Placement{}, false
	}

	return s.observed, true
}

// await applies whatever the Givens above it declared and holds until the
// control plane reports what this Wait holds for.
//
// The lab waits its own waiting time whatever time the Wait was held to, so the
// run reports how long recovery really took.
func (s *Scenario) await(named string, held scenario.StatedWait, reaching func() (scenario.Placement, error)) {
	step, open := s.record(WaitStep, named)
	if !open {
		return
	}
	// Deferred, so a Wait that ends the Scenario still draws its board and is
	// noted.
	defer s.endWait()
	if !s.apply(step) {
		return
	}

	holding := time.Now()
	reached, err := reaching()
	if err != nil {
		s.end(s.notReached(held), err)

		return
	}
	s.observed, s.settled = reached, true
	s.reached(held, time.Since(holding))
}

// The stated time is resolved first, so a time the lab cannot hold a Wait to
// ends the Scenario before any cluster is reached.
func (s *Scenario) awaitStating(named string, seconds int, reaching func() (scenario.Placement, error)) {
	held, err := scenario.NewStatedWait(seconds)
	if err != nil {
		if step, open := s.record(WaitStep, named); open {
			s.end(step, err)
			s.endWait()
		}

		return
	}

	s.await(named, held, reaching)
}

// Every recorded Wait is noted exactly once, after its board, so the k-th
// Wait note is the k-th Wait Step's.
func (s *Scenario) endWait() {
	s.drawBoard()
	s.note(s.steps[len(s.steps)-1])
}

// drawBoard observes under a waiting time of its own rather than the Wait's,
// so a Wait that expired still shows where every Pod stood as it ended. Before
// the Scenario first changes its own cluster no Workload Pod can exist, so the
// declared Nodes alone are the board and nothing is observed.
func (s *Scenario) drawBoard() {
	last := len(s.steps) - 1
	var observed []scenario.Pod
	if s.changedTheCluster {
		placed, err := s.observeEveryWorkload()
		if err != nil {
			s.steps[last].BoardNotObserved = fmt.Errorf("observe the Placement board: %w", err)
			s.end(s.steps[last], s.steps[last].BoardNotObserved)

			return
		}
		observed = placed.Pods()
	}

	s.board = scenario.NewPlacementBoard(s.topology, observed, s.board)
	s.steps[last].Board = s.board
}

func (s *Scenario) observeEveryWorkload() (scenario.Placement, error) {
	observing, cancel := context.WithTimeout(s.ctx, scenario.WaitingTime)
	defer cancel()

	return s.provisioned.ObserveEveryWorkload(observing)
}

// A Wait reached later than it was held to ends the Scenario, since the Steps
// after it would judge a state the Scenario called too late. It takes the
// measured time rather than reading a clock, so the judgement runs over no
// cluster.
func (s *Scenario) reached(held scenario.StatedWait, took time.Duration) {
	waited := s.waited(held, took, held.Reached(took))
	if waited.Reached {
		return
	}

	s.end(waited, errors.New(theWaitReachedTooLate))
}

func (s *Scenario) notReached(held scenario.StatedWait) Step {
	return s.waited(held, noWait, false)
}

// These Waits belong to the Scenario rather than to either opening type,
// because a Scenario reaches them through a Given's WaitFor and through a Wait
// Step of its own.
func (s *Scenario) reachNodesReady() (scenario.Placement, error) {
	return s.provisioned.AwaitNodesReady(s.ctx, s.topology, s.workload)
}

func (s *Scenario) reachPodsReady() (scenario.Placement, error) {
	return s.provisioned.AwaitPodsReady(s.ctx, s.workload)
}

// reachSettledPlacement is reached by a Placement leaving Pods Pending, so a
// Scenario of a Workload the Nodes have no room for has a Wait its Thens can
// judge after.
func (s *Scenario) reachSettledPlacement() (scenario.Placement, error) {
	return s.provisioned.AwaitSettled(s.ctx, s.workload)
}

// The plain forms state no time rather than the lab's own waiting time, which
// keeps that waiting time's one home in internal/scenario.
func (s *Scenario) awaitNodesReady() {
	s.await(allNodesReady, scenario.NewWaitStatingNoTime(), s.reachNodesReady)
}

func (s *Scenario) awaitNodesReadyWithinSeconds(seconds int) {
	s.awaitStating(allNodesReady, seconds, s.reachNodesReady)
}

func (s *Scenario) awaitPodsReady() {
	s.await(allPodsReady, scenario.NewWaitStatingNoTime(), s.reachPodsReady)
}

func (s *Scenario) awaitPodsReadyWithinSeconds(seconds int) {
	s.awaitStating(allPodsReady, seconds, s.reachPodsReady)
}

func (s *Scenario) awaitSettledPlacement() {
	s.await(theSettledPlacement, scenario.NewWaitStatingNoTime(), s.reachSettledPlacement)
}

func (s *Scenario) awaitSettledPlacementWithinSeconds(seconds int) {
	s.awaitStating(theSettledPlacement, seconds, s.reachSettledPlacement)
}

// apply applies the Topology first, so the Workload's Pods have Nodes to be
// placed on. A Given reaches no cluster, so a refusal only the control plane
// can give is named here, and one the lab checks itself stays at the Given.
func (s *Scenario) apply(at Step) bool {
	if s.topologyDeclaredAs != "" && !s.topologyApplied {
		s.changedTheCluster = true
		if err := s.provisioned.ApplyTopology(s.ctx, s.topology); err != nil {
			s.end(at, fmt.Errorf("apply %s: %w", s.topologyDeclaredAs, err))

			return false
		}
		s.topologyApplied = true
	}
	pending := slices.Clone(s.undeployed)
	if s.workloadDeclaredAs != "" && !s.workloadDeployed {
		pending = append(pending, declaredDeployment{workload: s.workload, accesses: s.accesses, declaredAs: s.workloadDeclaredAs})
	}
	for _, deploying := range pending {
		s.changedTheCluster = true
		if err := s.provisioned.Deploy(s.ctx, deploying.workload, deploying.accesses...); err != nil {
			s.end(at, fmt.Errorf("deploy %s: %w", deploying.declaredAs, err))

			return false
		}
	}
	s.undeployed, s.workloadDeployed = nil, true

	return true
}

// The earlier Workloads stay deployed. The Placement observed before belongs to
// an earlier Workload, so it is discarded and the next Then judges the
// Placement a Wait reaches for this one.
func (s *Scenario) declareWorkload(workload scenario.Workload, accesses []cluster.PodSpecAccessInPlace, declaredAs string) {
	if s.workloadDeclaredAs != "" {
		if !s.workloadDeployed {
			s.undeployed = append(s.undeployed, declaredDeployment{workload: s.workload, accesses: s.accesses, declaredAs: s.workloadDeclaredAs})
		}
		s.observed, s.settled = scenario.Placement{}, false
	}
	s.workload, s.accesses, s.workloadDeclaredAs, s.workloadDeployed = workload, accesses, declaredAs, false
	s.declaredWorkloads = append(s.declaredWorkloads, WorkloadName(workload.Name()))
}

// Given declares one part of the world a Scenario runs in, and ends the
// Scenario at its own line when the declaration is refused.
type Given struct {
	scenario *Scenario
}

// Topology declares the Zones the Scenario provisions and the NodePools each of
// them holds. A line holding no NodePool is refused.
func (g Given) Topology(zones Zones, pools ...node.DeclaredPool) DeclaredTopology {
	declared := DeclaredTopology{scenario: g.scenario, zones: zones, pools: slices.Clone(pools)}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	return declared.declareNodePools(step)
}

// Workload declares the Pod replicas the Scenario deploys. A Workload declared
// after another is deployed beside it by the next Wait, every Step after it
// reads it alone, and a name already declared ends the Scenario.
//
// The full accesses are kept beside the Workload, because the domain holds a
// specification's settings alone.
func (g Given) Workload(name WorkloadName, pods Pods, declares pod.Specification) DeclaredWorkload {
	declared := DeclaredWorkload{scenario: g.scenario, name: name, pods: pods, declares: declares}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	specification, refused := declares.Declared()
	if refused != nil {
		g.scenario.end(step, refused)

		return declared
	}
	step = g.scenario.carrying(declares.Carries())

	if slices.Contains(g.scenario.declaredWorkloads, name) {
		g.scenario.end(step, fmt.Errorf("a Scenario declares each Workload once, "+
			"so the Workload %q declared again is refused", name))

		return declared
	}

	workload, err := scenario.NewWorkload(string(name), int(pods), specification)
	if err != nil {
		g.scenario.end(step, err)

		return declared
	}
	g.scenario.declareWorkload(workload, declares.Accesses(), step.Name)

	return declared
}

// DeclaredWorkload is complete as it stands, so no half-built Workload ever
// exists.
type DeclaredWorkload struct {
	scenario *Scenario
	name     WorkloadName
	pods     Pods
	declares pod.Specification
}

// RequiredCapacity declares the fewest Pods this Workload must keep Running
// through a Failure.
func (d DeclaredWorkload) RequiredCapacity(pods Pods) DeclaredWorkload {
	if d.scenario.cause != nil {
		return d
	}

	step := d.scenario.rename(d.named() + ", keeping at least " + counted(int(pods), "Pod", "Pods") + " Running")

	declared, err := d.scenario.workload.WithRequiredCapacity(int(pods))
	if err != nil {
		d.scenario.end(step, err)

		return d
	}
	d.scenario.workload, d.scenario.workloadDeclaredAs = declared, step.Name

	return d
}

func (d DeclaredWorkload) WaitFor() WorkloadWait {
	return WorkloadWait{scenario: d.scenario}
}

func (d DeclaredWorkload) named() string {
	return fmt.Sprintf("a Workload %s of %s declaring %s",
		d.name, counted(int(d.pods), "Pod", "Pods"), d.declares.Reads())
}

type DeclaredTopology struct {
	scenario *Scenario
	zones    Zones
	pools    []node.DeclaredPool
}

func (d DeclaredTopology) declareNodePools(step Step) DeclaredTopology {
	pools := make([]scenario.NodePool, 0, len(d.pools))
	for _, written := range d.pools {
		pool, refused := written.Declared()
		if refused != nil {
			d.scenario.end(step, refused)

			return d
		}
		pools = append(pools, pool)
	}

	topology, err := scenario.NewTopology(int(d.zones), pools...)

	return d.declare(step, topology, err)
}

func (d DeclaredTopology) declare(step Step, topology scenario.Topology, refused error) DeclaredTopology {
	if refused != nil {
		d.scenario.end(step, refused)

		return d
	}
	d.scenario.topology, d.scenario.topologyDeclaredAs = topology, step.Name

	return d
}

func (d DeclaredTopology) WaitFor() TopologyWait {
	return TopologyWait{scenario: d.scenario}
}

func (d DeclaredTopology) named() string {
	if len(d.pools) == 0 {
		return fmt.Sprintf("a Topology of %s of no NodePool", counted(int(d.zones), "Zone", "Zones"))
	}

	held := make([]string, 0, len(d.pools))
	for _, pool := range d.pools {
		held = append(held, pool.Reads())
	}

	return fmt.Sprintf("a Topology of %s of %s", counted(int(d.zones), "Zone", "Zones"), strings.Join(held, "; "))
}

// TopologyWait offers only the Wait that fits the line that opened it.
type TopologyWait struct {
	scenario *Scenario
}

type WorkloadWait struct {
	scenario *Scenario
}

// AllNodesReady is held to the lab's own waiting time.
func (t TopologyWait) AllNodesReady() {
	t.scenario.awaitNodesReady()
}

func (t TopologyWait) AllNodesReadyWithinSeconds(seconds int) {
	t.scenario.awaitNodesReadyWithinSeconds(seconds)
}

// AllPodsReady is held to the lab's own waiting time.
func (w WorkloadWait) AllPodsReady() {
	w.scenario.awaitPodsReady()
}

func (w WorkloadWait) AllPodsReadyWithinSeconds(seconds int) {
	w.scenario.awaitPodsReadyWithinSeconds(seconds)
}

// Wait offers all three Waits, because a Wait of its own stands wherever a
// Scenario needs one: before any Workload is declared, and after a Failure.
type Wait struct {
	scenario *Scenario
}

// AllNodesReady is held to the lab's own waiting time.
func (w Wait) AllNodesReady() {
	w.scenario.awaitNodesReady()
}

func (w Wait) AllNodesReadyWithinSeconds(seconds int) {
	w.scenario.awaitNodesReadyWithinSeconds(seconds)
}

// AllPodsReady is held to the lab's own waiting time.
func (w Wait) AllPodsReady() {
	w.scenario.awaitPodsReady()
}

func (w Wait) AllPodsReadyWithinSeconds(seconds int) {
	w.scenario.awaitPodsReadyWithinSeconds(seconds)
}

// PlacementSettled is reached while Pods stay Pending, which makes it the Wait
// to write where the Nodes still responding have no room for every Pod. It is
// held to the lab's own waiting time.
func (w Wait) PlacementSettled() {
	w.scenario.awaitSettledPlacement()
}

func (w Wait) PlacementSettledWithinSeconds(seconds int) {
	w.scenario.awaitSettledPlacementWithinSeconds(seconds)
}

// When injects a Failure and waits for nothing, so the Placement its Thens
// judge is the one the next Wait reaches.
type When struct {
	scenario *Scenario
}

// NodesFail stops the first Nodes the declared Topology derives, so the pick is
// the same on every run.
func (w When) NodesFail(nodes Nodes) {
	step, open := w.scenario.record(WhenStep, nodesFailing(int(nodes)))
	if !open {
		return
	}

	if _, observed := w.scenario.placement(step); !observed {
		return
	}

	failure, err := scenario.NewNodeFailure(w.scenario.topology, int(nodes))
	if err != nil {
		w.scenario.end(step, err)

		return
	}
	stopping := w.scenario.rename(nodesFailing(int(nodes)) + ", " + strings.Join(failure.Names(), ", "))

	w.scenario.stop(stopping, failure.Names())
}

// ZonesFail stops every Node of the first Zones the declared Topology derives,
// so the pick is the same on every run.
func (w When) ZonesFail(zones Zones) {
	step, open := w.scenario.record(WhenStep, zonesFailing(int(zones)))
	if !open {
		return
	}

	if _, observed := w.scenario.placement(step); !observed {
		return
	}

	failure, err := scenario.NewZoneFailure(w.scenario.topology, int(zones))
	if err != nil {
		w.scenario.end(step, err)

		return
	}
	stopping := w.scenario.rename(zonesFailing(int(zones)) + ", " + strings.Join(failure.Names(), ", "))

	if w.scenario.stop(stopping, failure.NodeNames()) {
		w.scenario.failedZones = append(w.scenario.failedZones, failure.Names()...)
	}
}

// NodesLabelledFail stops every Node still reporting the one label stated, in
// Topology order, so the pick is the same on every run. A label or value no
// NodePool states, or a literal of other than one label, ends the Scenario.
func (w When) NodesLabelledFail(labels node.Labels) {
	named := labelledNodesFailing(labels)
	step, open := w.scenario.record(WhenStep, named)
	if !open {
		return
	}

	observed, seen := w.scenario.placement(step)
	if !seen {
		return
	}

	failure, err := scenario.NewLabelFailure(w.scenario.topology, observed, labels)
	if err != nil {
		w.scenario.end(step, err)

		return
	}
	stopping := w.scenario.rename(named + ", " + strings.Join(failure.Names(), ", "))

	w.scenario.stop(stopping, failure.Names())
}

// stop discards the observation held until now, since it is the world before
// the Failure.
func (s *Scenario) stop(at Step, nodes []string) bool {
	for _, node := range nodes {
		s.changedTheCluster = true
		if err := s.provisioned.FailNode(s.ctx, node); err != nil {
			s.end(at, fmt.Errorf("stop Node %s responding: %w", node, err))

			return false
		}
		s.failed = append(s.failed, node)
	}
	s.runningAtTheFailure = s.podsKeptRunning(s.observed)

	s.observed, s.settled = scenario.Placement{}, false

	return true
}

// Both moments a Required Capacity is judged at count Pods this way.
func (s *Scenario) podsKeptRunning(observed scenario.Placement) int {
	return len(observed.PodsRunningApartFrom(s.failed))
}

func nodesFailing(count int) string {
	return counted(count, "Node", "Nodes") + failing(count)
}

func zonesFailing(count int) string {
	return counted(count, "Zone", "Zones") + failing(count)
}

func failing(count int) string {
	if count == 1 {
		return " fails"
	}

	return " fail"
}

func labelledNodesFailing(labels node.Labels) string {
	stated := make([]string, 0, len(labels))
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		stated = append(stated, key+"="+labels[key])
	}

	if len(stated) == 0 {
		return "Nodes labelled by no label fail"
	}

	return "Nodes labelled " + strings.Join(stated, ", ") + " fail"
}

// A Then the Placement does not meet records a Finding, and the Steps after it
// still run.
type Then struct {
	scenario *Scenario
}

func (t Then) AllPodsRunning() {
	t.judge("every Pod Running", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsInPhase(scenario.PodRunning)).
			To(expect.HaveLen[scenario.Pod](t.scenario.workload.Pods()))
	})
}

// PodsPerZone counts a Zone holding none as zero.
func (t Then) PodsPerZone(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" per Zone", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsPerZone()).
			To(expect.Equal(evenlyOver(t.scenario.topology.Zones(), pods)))
	})
}

// PodsPerValueOf counts a Pod under the value its observed Node carries, so a
// Pod on a Node stating no value counts toward none. A key no NodePool states
// ends the Scenario.
func (t Then) PodsPerValueOf(key string, pods int) {
	step, open := t.scenario.record(ThenStep, counted(pods, "Pod", "Pods")+" per value of "+key)
	if !open {
		return
	}

	values := t.scenario.topology.LabelValues(key)
	if len(values) == 0 {
		t.scenario.end(step, fmt.Errorf("this Then counts Pods per value of the label %q, "+
			"and no NodePool of the Topology states it", key))

		return
	}

	t.judgeAt(step, func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, podsIn(values, settled.PodsPerLabelValue(key))).To(expect.Equal(evenlyOver(values, pods)))
	})
}

// PodsOnNodePool counts a Pod Pending on no Node toward none. A NodePool the
// declared Topology lacks ends the Scenario.
func (t Then) PodsOnNodePool(pool node.PoolName, pods int) {
	step, open := t.scenario.record(ThenStep, counted(pods, "Pod", "Pods")+" on the NodePool "+string(pool))
	if !open {
		return
	}

	if !t.scenario.topology.HoldsNodePool(string(pool)) {
		t.scenario.end(step, fmt.Errorf("this Then counts Pods on the NodePool %q, "+
			"and the Topology holds no NodePool of that name", pool))

		return
	}

	named := []string{string(pool)}
	t.judgeAt(step, func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, podsIn(named, settled.PodsPerNodePool())).To(expect.Equal(evenlyOver(named, pods)))
	})
}

// PodsPerNode counts a Node holding none as zero.
func (t Then) PodsPerNode(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" per Node", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsPerNode()).
			To(expect.Equal(evenlyOver(nodesOf(t.scenario.topology), pods)))
	})
}

// PodsPending judges how many Pods are Pending; why each one is stands on the
// Timeline.
func (t Then) PodsPending(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" Pending", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsInPhase(scenario.PodPending)).To(expect.HaveLen[scenario.Pod](pods))
	})
}

// NoPodsOnTheFailedNodes counts a Pod still on a failed Node although that Node
// is in no observed Placement, because the report states what was observed.
func (t Then) NoPodsOnTheFailedNodes() {
	t.judgeWhatFailed(failedDomain{
		named:   "no Pod on the Nodes that failed",
		stopped: t.scenario.failed,
		counted: scenario.Placement.PodsPerNode,
		subject: "the Nodes a When stopped responding",
	})
}

// NoPodsInTheFailedZones counts a Pod in a failed Zone whose Nodes still
// respond under that Zone, which stays in the observation.
func (t Then) NoPodsInTheFailedZones() {
	t.judgeWhatFailed(failedDomain{
		named:   "no Pod in the Zones that failed",
		stopped: t.scenario.failedZones,
		counted: scenario.Placement.PodsPerZone,
		subject: "the Zones a When stopped whole",
	})
}

const (
	atTheMomentOfTheFailure    = "at the moment of the Failure"
	onceThePlacementHadSettled = "once the Placement had settled"
)

// RequiredCapacityHeld judges the declared minimum at the moment of the Failure
// and once the Placement has settled again. A Workload declaring no minimum, or
// a Scenario with no When before this Then, ends the Scenario.
func (t Then) RequiredCapacityHeld() {
	step, open := t.scenario.record(ThenStep, "the Required Capacity held")
	if !open {
		return
	}

	minimum, judgeable := t.minimumToJudge(step)
	if !judgeable {
		return
	}

	settled, observed := t.scenario.placement(step)
	if !observed {
		return
	}
	atTheFailure, onceSettled := t.scenario.runningAtTheFailure, t.scenario.podsKeptRunning(settled)

	judged := t.scenario.rename(fmt.Sprintf(
		"the Required Capacity held: a minimum of %s, %d at the moment of the Failure and %d once Settled",
		counted(minimum, "Pod", "Pods"), atTheFailure, onceSettled))

	t.judgeAt(judged, func(_ scenario.Placement, judging expect.T) {
		expect.Expect(judging, atTheFailure).To(keptAtLeast(minimum, atTheMomentOfTheFailure))
		expect.Expect(judging, onceSettled).To(keptAtLeast(minimum, onceThePlacementHadSettled))
	})
}

func (t Then) minimumToJudge(step Step) (minimum int, judgeable bool) {
	minimum, declared := t.scenario.workload.RequiredCapacity()
	switch {
	case !declared:
		t.scenario.end(step, errors.New("this Then judges the fewest Pods a Workload must keep Running, "+
			"and this Workload declares no Required Capacity"))

		return 0, false
	case len(t.scenario.failed) == 0:
		t.scenario.end(step, errors.New("this Then judges a Required Capacity through a Failure, "+
			"and no When ran before it"))

		return 0, false
	}

	return minimum, true
}

// keptAtLeast lives here rather than in internal/expect, which keeps its
// general matchers alone. Each moment missed is its own Finding.
func keptAtLeast(minimum int, moment string) expect.Matcher[int] {
	return func(kept int) (string, bool) {
		if kept >= minimum {
			return "", true
		}

		return fmt.Sprintf("%s the Workload kept %s Running, short of the %s its Required Capacity declares",
			moment, counted(kept, "Pod", "Pods"), counted(minimum, "Pod", "Pods")), false
	}
}

type failedDomain struct {
	named   string
	stopped []string
	counted func(scenario.Placement) map[string]int
	subject string
}

// The members are compared as one whole map rather than one count per member.
func (t Then) judgeWhatFailed(domain failedDomain) {
	step, open := t.scenario.record(ThenStep, domain.named)
	if !open {
		return
	}

	failed := slices.Clone(domain.stopped)
	if len(failed) == 0 {
		t.scenario.end(step, fmt.Errorf("this Then judges %s, and no When ran before it", domain.subject))

		return
	}

	t.judgeAt(step, func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, podsIn(failed, domain.counted(settled))).To(expect.Equal(evenlyOver(failed, noPod)))
	})
}

// A mismatch becomes a Finding rather than an error, so the Steps after it
// still run.
func (t Then) judge(name string, against func(scenario.Placement, expect.T)) {
	step, open := t.scenario.record(ThenStep, name)
	if !open {
		return
	}

	t.judgeAt(step, against)
}

func (t Then) judgeAt(step Step, against func(scenario.Placement, expect.T)) {
	settled, observed := t.scenario.placement(step)
	if !observed {
		return
	}

	against(settled, &findings{scenario: t.scenario, step: step})
}

// findings is the Then side of the interface *testing.T satisfies, so a Then
// judges through internal/expect outside a go test.
type findings struct {
	scenario *Scenario
	step     Step
}

func (*findings) Helper() {}

func (f *findings) Errorf(format string, args ...any) {
	f.scenario.findings = append(f.scenario.findings, Finding{
		Step:     f.step,
		Mismatch: fmt.Sprintf(format, args...),
	})
}

func evenlyOver(names []string, count int) map[string]int {
	expected := make(map[string]int, len(names))
	for _, name := range names {
		expected[name] = count
	}

	return expected
}

func podsIn(members []string, observed map[string]int) map[string]int {
	counts := make(map[string]int, len(members))
	for _, member := range members {
		counts[member] = observed[member]
	}

	return counts
}

func nodesOf(topology scenario.Topology) []string {
	nodes := topology.Nodes()
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}

func counted(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(count) + " " + plural
}
