// Package lab holds the Step surface a Scenario's lines run against, and the
// runner that provisions a Simulated Cluster for one Scenario and reports what
// that Scenario found.
//
// It is the one package naming both a Step and the provisioned Simulated
// Cluster. A Scenario source imports it beside the pod and node packages beneath
// it, which hold what a Pod carries and what a Node is, so every name a Scenario
// writes reads by its subject, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md and
// docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md.
package lab

import (
	"context"
	"errors"
	"fmt"
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

// noPod is what a Node that stopped responding holds once the control plane has
// moved its Pods away.
const noPod = 0

const (
	// noWait is what a Step that waited for nothing carries, reportedWaitPrecision
	// how finely a Wait's line states the time it waited — the tenth of a second a
	// reader of a restore time acts on — and theWaitPrefix the phrase that line
	// opens with, a Wait being the one kind whose line does not open with the
	// kind's own word.
	noWait                = 0
	reportedWaitPrecision = 100 * time.Millisecond
	theWaitPrefix         = "Wait for"

	// theTimeReached and theTimeNotReached are how such a line names which of the
	// two times follows it: the time the Wait took where it was reached, and the
	// time it was held to where it was not. A reader of a restore time therefore
	// never reads one of the two for the other.
	theTimeReached    = ", reached in "
	theTimeNotReached = ", not reached in "

	// theTimeHeldTo opens the time a reached Wait was held to, after the time it
	// took, and theStatedTimeBefore and theStatedTimeAfter mark a time the Scenario
	// stated on the Wait itself. The lab's own waiting time carries no mark, since
	// a run reports it at its start rather than on every line of every Scenario.
	theTimeHeldTo       = " of "
	theStatedTimeBefore = "the "
	theStatedTimeAfter  = " stated"
)

const (
	// allNodesReady and allPodsReady are how the two Waits name the Wait line each
	// of them reports, whichever way a Scenario wrote them: the Node condition and
	// the Pod condition the control plane reports, the latter being what kubectl
	// rollout status waits on.
	allNodesReady = "all Nodes Ready"
	allPodsReady  = "all Pods Ready"

	// theSettledPlacement is how the third Wait names its line: the lab's own state
	// rather than a condition of Kubernetes', because it holds for a Placement that
	// has stopped changing whether its Pods became Ready or stayed Pending.
	theSettledPlacement = "the Placement Settled"

	// theWaitThatNeverStood is what a Step reading a Placement names when no Wait
	// reached one, which is the line a reader has to add to the Scenario.
	theWaitThatNeverStood = "no Wait stands before it"

	// theWaitReachedTooLate is why a Scenario ends at a Wait the cluster reached
	// later than that Wait states. The Step's own line states both times, so the
	// cause names what ended the Scenario rather than repeating them.
	theWaitReachedTooLate = "this Wait states the time it must be reached in, and the cluster reached it later"
)

// Zones is how many Zones a Topology Step declares, or how many a When Step stops
// responding. A Scenario states the count and the lab resolves which, so no
// Scenario names a Zone by a string its Givens do not derive.
type Zones int

// Pods is a count of a Workload's Pods: how many a Workload Step declares, and
// how many its Required Capacity keeps Running. The room one Node has for Pods is
// the Pods field of node.Capacity, so each count reads by the subject it counts
// for.
type Pods int

// Nodes is how many Nodes a When Step stops responding. A Scenario states the
// count and the lab resolves which, so no Scenario names a Node by a string its
// Givens do not derive.
type Nodes int

// WorkloadName is what a Workload Step deploys under, and what its Pods are
// labelled with. A Scenario may name it outright; the lab's own Scenarios take
// theirs from the constants declared beside them, so the Workloads they deploy
// have one home.
type WorkloadName string

// The types above are names over a primitive and carry no invariant of their
// own: a swapped argument does not compile, and a value a Scenario may not
// declare is refused by the internal/scenario constructor the Step calls, which
// stays the one validation home (docs/architecture-principles.md § Go Idioms).

// StepKind is which of the four a Step is.
type StepKind string

const (
	// GivenStep sets up the world a Scenario runs in.
	GivenStep StepKind = "Given"
	// WhenStep injects the Failure a Scenario studies.
	WhenStep StepKind = "When"
	// WaitStep holds until the control plane reports a state of the cluster.
	WaitStep StepKind = "Wait"
	// ThenStep judges the Placement the Scenario settled at.
	ThenStep StepKind = "Then"
)

// opens is how a line of this kind opens: the kind's own word, and for a Wait the
// phrase reading into the state it held for.
func (k StepKind) opens() string {
	if k == WaitStep {
		return theWaitPrefix
	}

	return string(k)
}

// Step is one line of a Scenario as it ran: which of the four kinds it is, the
// name it reports, and the times it was held to and took where that line is a
// Wait.
type Step struct {
	Kind StepKind
	Name string
	// Waited is how long a Wait held before the cluster reached what it waited for,
	// and nothing at all for a Wait the cluster never reached and for every other
	// Step. It is a field of its own rather than part of the name, so a Step's name
	// stays reproducible, the rename a further call writes stays untouched, and a
	// later Scenario naming its own waiting time has a value to judge, per
	// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
	Waited time.Duration
	// HeldTo is the time that Wait had to be reached in — the time its line states,
	// or the lab's own waiting time where it states none — and TimeStated whether
	// the Scenario stated it. Every Wait carried out has a time, so a Step holding
	// none is a Step of another kind, or a Wait that was never carried out.
	HeldTo     time.Duration
	TimeStated bool
	// Reached is whether the cluster reached what the Wait held for inside that
	// time. It is recorded rather than read back off the two times above, because a
	// Wait with nothing to wait for is reached in no measured time and a Wait the
	// cluster never reached measures none either, per docs/prd.md REQ-SIM-006 edge
	// case 2.
	Reached bool
	// Settings are the settings a Workload's Given declared its Pods to carry, one
	// line each as a manifest writes it, in the order the domain holds them, and
	// nothing at all for a refused Pod specification and for every other Step. They
	// are a field of their own rather than part of the name, so a Finding and the
	// Step a Scenario ended at stay one short line, per
	// docs/adr/2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md.
	Settings []string
}

// String is how a Step reads in a report: how its kind opens a line and the name
// it reports, closing where it is a Wait with the time it took if it was reached
// and the time it was held to if it was not, each marked for which of the two it
// is, and a stated time marked as stated.
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

// timeHeldTo is how a line names the time its Wait was held to: marked as stated
// where the Scenario stated it on the Wait itself, and bare where it is the lab's
// own waiting time, so a reader never reads that bound for a time a Scenario
// chose.
func (s Step) timeHeldTo() string {
	if !s.TimeStated {
		return s.HeldTo.String()
	}

	return theStatedTimeBefore + s.HeldTo.String() + theStatedTimeAfter
}

// Finding is one Then the Settled Placement did not meet, naming the Step that
// judged it and the mismatch internal/expect reported. A Finding is what the lab
// is for: it is an observation about Kubernetes, not an error in the lab.
type Finding struct {
	Step     Step
	Mismatch string
}

// Scenario is the Step surface a Scenario's lines run against. Each line is one
// Given, When, Wait or Then Step, and none of them names an error: the value carries
// the first declaration that was refused and every later Step then carries
// nothing out, the way a run's report keeps its first write error.
//
// A Scenario holds the context its cluster work runs under rather than taking one
// per Step, because a Scenario's lines take nothing but the Scenario itself.
type Scenario struct {
	ctx         context.Context
	provisioned *cluster.SimulatedCluster

	topology scenario.Topology
	workload scenario.Workload
	// accesses are the full accesses the Workload line wrote, each in the place it
	// was written. The Step surface carries them beside the Workload, because the
	// domain value the Workload holds carries no function value.
	accesses []cluster.PodSpecAccessInPlace

	steps    []Step
	findings []Finding

	// failed are the Nodes every When of this Scenario stopped responding, and
	// failedZones the Zones it stopped whole, each by name and in the order they
	// were stopped, which is what the Thens reading a Failure judge against.
	failed      []string
	failedZones []string

	// runningAtTheFailure is what the Workload kept Running at the moment the
	// latest When injected its Failure, derived there from the Placement that When
	// discards. A later When overwrites it, so a Then judging it judges the Failure
	// standing before it.
	runningAtTheFailure int

	// topologyDeclaredAs and workloadDeclaredAs are how the Given that declared each
	// of them reads. A Wait applies what the Givens above it declared, so it is a
	// line below the one that declared it, and these are how its cause names what it
	// applied.
	topologyDeclaredAs string
	workloadDeclaredAs string

	observed scenario.Placement
	// topologyApplied is whether the declared Topology has reached the Simulated
	// Cluster and workloadDeployed whether the declared Workload has, so a second
	// Wait applies neither again. settled is whether a Wait has reached the
	// Placement observed since the last Failure: a When clears it alone, so the next
	// Wait applies nothing and holds for the world the Failure left.
	topologyApplied  bool
	workloadDeployed bool
	settled          bool

	endedAt Step
	cause   error
}

// newScenario is the one way a Scenario comes into being: the Runner's, over the
// Simulated Cluster provisioned for that Scenario alone.
func newScenario(ctx context.Context, provisioned *cluster.SimulatedCluster) *Scenario {
	return &Scenario{ctx: ctx, provisioned: provisioned}
}

// Given opens a Given Step, which declares the world the Scenario runs in.
func (s *Scenario) Given() Given {
	return Given{scenario: s}
}

// When opens a When Step, which injects the Failure the Scenario studies.
func (s *Scenario) When() When {
	return When{scenario: s}
}

// WaitFor opens a Wait Step, which holds until the control plane reports a state
// of the cluster.
func (s *Scenario) WaitFor() Wait {
	return Wait{scenario: s}
}

// Then opens a Then Step, which judges the Placement the Scenario settled at.
func (s *Scenario) Then() Then {
	return Then{scenario: s}
}

// record records a Step of this kind under this name, and reports whether it is
// to be carried out. A Scenario that has already ended records nothing.
func (s *Scenario) record(kind StepKind, name string) (Step, bool) {
	if s.cause != nil {
		return Step{}, false
	}

	step := Step{Kind: kind, Name: name}
	s.steps = append(s.steps, step)

	return step, true
}

// rename names the Step just recorded again, because an optional setting is a
// further call on the same Scenario line and not a line of its own.
func (s *Scenario) rename(name string) Step {
	last := len(s.steps) - 1
	s.steps[last].Name = name

	return s.steps[last]
}

// carrying records on the Step just recorded the settings its Workload's Pods
// carry, and reports that Step as it now reads.
func (s *Scenario) carrying(settings []string) Step {
	last := len(s.steps) - 1
	s.steps[last].Settings = settings

	return s.steps[last]
}

// waited records on the Step just recorded what its Wait was held to and how it
// came out: the time it was held to and whether the Scenario stated that time, the
// time the Wait took, and whether the cluster reached what it held for. Each is a
// field of that Step rather than part of its name, and the last is recorded rather
// than read back off the two times, because a Wait with nothing to wait for is
// reached in no measured time and one the cluster never reached measures none
// either. It reports that Step as it now reads.
func (s *Scenario) waited(held scenario.StatedWait, took time.Duration, reached bool) Step {
	last := len(s.steps) - 1
	s.steps[last].HeldTo, s.steps[last].TimeStated = held.HeldTo(), held.Stated()
	s.steps[last].Waited, s.steps[last].Reached = took, reached

	return s.steps[last]
}

// end stops the Scenario at the Step that could not be carried out. The first
// stop is the one kept, so the Step named is the one a reader looks at.
func (s *Scenario) end(step Step, cause error) {
	if s.cause != nil {
		return
	}

	s.endedAt = step
	s.cause = cause
}

// placement is the Placement the last Wait reached: what every Then judges, and
// what a Failure counts the Pods it keeps Running off. A Scenario that has not
// Waited since the Givens, or since the Failure that discarded the Placement
// before it, has none and ends at the Step that read for one, per
// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
func (s *Scenario) placement(at Step) (scenario.Placement, bool) {
	if !s.settled {
		s.end(at, errors.New("this Step reads the Placement a Wait reached, and "+theWaitThatNeverStood))

		return scenario.Placement{}, false
	}

	return s.observed, true
}

// await is the one Wait both ways of writing one run: it applies whatever the
// Givens above it declared, holds until the control plane reports what this Wait
// holds for, and records on its own Wait Step the time it was held to and how long
// it held. A Wait the lab's waiting time outlasts ends the Scenario there, its
// line naming the time it was held to and its cause naming what was not Ready.
//
// The lab waits its own waiting time whatever time the Wait was held to, so the
// run reports how long recovery really took.
func (s *Scenario) await(named string, held scenario.StatedWait, reaching func() (scenario.Placement, error)) {
	step, open := s.record(WaitStep, named)
	if !open {
		return
	}
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

// awaitStating is that Wait with a time stated on its line: the time is resolved
// where it is stated, so one the lab cannot hold a Wait to ends the Scenario at
// that Wait before anything is applied and before any cluster is reached.
func (s *Scenario) awaitStating(named string, seconds int, reaching func() (scenario.Placement, error)) {
	held, err := scenario.NewStatedWait(seconds)
	if err != nil {
		if step, open := s.record(WaitStep, named); open {
			s.end(step, err)
		}

		return
	}

	s.await(named, held, reaching)
}

// reached records the Wait just carried out as reached in the time it took, and
// ends the Scenario there where the cluster reached what it held for later than
// that Wait was held to: the Steps after it would judge a state the Scenario
// called too late. It takes the time measured rather than reading a clock, so the
// judgement runs over no cluster.
func (s *Scenario) reached(held scenario.StatedWait, took time.Duration) {
	waited := s.waited(held, took, held.Reached(took))
	if waited.Reached {
		return
	}

	s.end(waited, errors.New(theWaitReachedTooLate))
}

// notReached records the Wait the lab's own waiting time outlasted: it was held to
// a time and reached nothing, so its line names the time it was held to and no
// time it took, while the Scenario ends there naming what was not Ready.
func (s *Scenario) notReached(held scenario.StatedWait) Step {
	return s.waited(held, noWait, false)
}

// reachNodesReady and reachPodsReady are two of the three Waits themselves, each
// the one that fits what a Given declares: every Node of the declared Topology
// Ready, and every Pod of the declared Workload Ready. A Scenario reaches them
// through the WaitFor of a Given's own line or through a Wait Step of its own,
// which is why they are the Scenario's rather than either opening type's.
func (s *Scenario) reachNodesReady() (scenario.Placement, error) {
	return s.provisioned.AwaitNodesReady(s.ctx, s.topology, s.workload)
}

func (s *Scenario) reachPodsReady() (scenario.Placement, error) {
	return s.provisioned.AwaitPodsReady(s.ctx, s.workload)
}

// reachSettledPlacement is the third: it holds for the Placement of the declared
// Workload alone, which the two above hold for beneath their own condition. Every
// Pod the Workload declares is accounted for there, placed or not, so a Placement
// leaving Pods Pending reaches it and a Scenario of a Workload the Nodes have no
// room for has a Wait its Thens can judge after.
func (s *Scenario) reachSettledPlacement() (scenario.Placement, error) {
	return s.provisioned.AwaitSettled(s.ctx, s.workload)
}

// Each of the three is written in two forms, stating no time or stating one, and
// these six are the one place each form is paired with the Wait it carries out.
// The plain form states no time rather than stating the lab's own waiting time,
// which keeps that waiting time's one home in internal/scenario.
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

// apply is where a declaration reaches the Simulated Cluster: the Wait applies
// what the Givens above it declared and nothing else has applied yet — the
// Topology first, so the Workload's Pods have Nodes to be placed on — and the
// Scenario ends at that Wait where the control plane refuses one, its cause naming
// the declaration the Given above wrote. A Given declares alone and reaches no
// cluster, so a refusal only the control plane can give is named here and one the
// lab checks itself stays at the Given, per
// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
func (s *Scenario) apply(at Step) bool {
	if s.topologyDeclaredAs != "" && !s.topologyApplied {
		if err := s.provisioned.ApplyTopology(s.ctx, s.topology); err != nil {
			s.end(at, fmt.Errorf("apply %s: %w", s.topologyDeclaredAs, err))

			return false
		}
		s.topologyApplied = true
	}
	if s.workloadDeclaredAs != "" && !s.workloadDeployed {
		if err := s.provisioned.Deploy(s.ctx, s.workload, s.accesses...); err != nil {
			s.end(at, fmt.Errorf("deploy %s: %w", s.workloadDeclaredAs, err))

			return false
		}
		s.workloadDeployed = true
	}

	return true
}

// Given declares one part of the world a Scenario runs in. Each of its methods
// constructs the immutable internal/scenario value that part is, and ends the
// Scenario at its own line when that constructor refuses the declaration.
type Given struct {
	scenario *Scenario
}

// Topology declares the Zones the Scenario provisions and the NodePools every one
// of them holds, in the order they are written, per
// docs/adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md.
//
// A line holding no NodePool is refused here and ends the Scenario at it, as does
// any refusal a NodePool carries.
func (g Given) Topology(zones Zones, pools ...node.DeclaredPool) DeclaredTopology {
	declared := DeclaredTopology{scenario: g.scenario, zones: zones, pools: slices.Clone(pools)}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	return declared.declareNodePools(step)
}

// Workload declares the Pod replicas the Scenario deploys: how many, under the
// name their Pods are labelled with, and the predefined Pod specification they
// are deployed with, customized or not. The line carries no setting of its own
// besides the Required Capacity a further call declares, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md and
// docs/adr/2026-10-06-required-capacity-judged-at-two-moments.md.
//
// The full accesses that specification was customized with are kept beside the
// Workload, because what the domain holds of a specification is its settings
// alone.
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

	workload, err := scenario.NewWorkload(string(name), int(pods), specification)
	if err != nil {
		g.scenario.end(step, err)

		return declared
	}
	g.scenario.workload, g.scenario.accesses = workload, declares.Accesses()
	g.scenario.workloadDeclaredAs = step.Name

	return declared
}

// DeclaredWorkload is the Workload one Given line declared. It is complete as it
// stands, and a further call declares the fewest Pods it must keep Running, so no
// half-built Workload ever exists (docs/architecture-principles.md
// § Construction and update).
type DeclaredWorkload struct {
	scenario *Scenario
	name     WorkloadName
	pods     Pods
	declares pod.Specification
}

// RequiredCapacity declares the fewest Pods this Workload must keep Running
// through a Failure, renaming this line to the minimum it names. The Scenario ends
// here when the domain refuses that minimum.
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

// WaitFor opens the Wait that fits the Workload this line declared, so a Scenario
// can write the Wait on the line that declared what it waits for.
func (d DeclaredWorkload) WaitFor() WorkloadWait {
	return WorkloadWait{scenario: d.scenario}
}

// named is how this line reads in a report: its Pods, the intent of the
// specification it declares, and the Startup time that specification's Pods take.
func (d DeclaredWorkload) named() string {
	return fmt.Sprintf("a Workload %s of %s declaring %s",
		d.name, counted(int(d.pods), "Pod", "Pods"), d.declares.Reads())
}

// DeclaredTopology is the Topology one Given line declared. It is complete as it
// stands, so no half-built Topology ever exists (docs/architecture-principles.md
// § Construction and update).
type DeclaredTopology struct {
	scenario *Scenario
	zones    Zones
	pools    []node.DeclaredPool
}

// declareNodePools constructs the Topology of the NodePools this line names, and
// ends the Scenario at step on the first refusal: a NodePool the domain refused
// where it was written, or the Topology the domain refuses.
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

// declare makes topology the one this Scenario declared, or ends the Scenario at
// step with the refusal its constructor gave.
func (d DeclaredTopology) declare(step Step, topology scenario.Topology, refused error) DeclaredTopology {
	if refused != nil {
		d.scenario.end(step, refused)

		return d
	}
	d.scenario.topology, d.scenario.topologyDeclaredAs = topology, step.Name

	return d
}

// WaitFor opens the Wait that fits the Topology this line declared.
func (d DeclaredTopology) WaitFor() TopologyWait {
	return TopologyWait{scenario: d.scenario}
}

// named is how this line reads in a report: its Zones, then each NodePool in
// declaration order.
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

// TopologyWait is what the WaitFor of a Topology line opens, and WorkloadWait
// what the WaitFor of a Workload line opens: each offers the one Wait that fits
// what the line just declared, and nothing else.
type TopologyWait struct {
	scenario *Scenario
}

type WorkloadWait struct {
	scenario *Scenario
}

// AllNodesReady holds until the control plane reports every Node of the Topology
// this line declared Ready, within the lab's own waiting time.
func (t TopologyWait) AllNodesReady() {
	t.scenario.awaitNodesReady()
}

// AllNodesReadyWithinSeconds is that Wait held to the time this line states it
// must be reached in, in whole seconds.
func (t TopologyWait) AllNodesReadyWithinSeconds(seconds int) {
	t.scenario.awaitNodesReadyWithinSeconds(seconds)
}

// AllPodsReady holds until the control plane reports every Pod of the Workload
// this line declared Ready, within the lab's own waiting time.
func (w WorkloadWait) AllPodsReady() {
	w.scenario.awaitPodsReady()
}

// AllPodsReadyWithinSeconds is that Wait held to the time this line states it
// must be reached in, in whole seconds.
func (w WorkloadWait) AllPodsReadyWithinSeconds(seconds int) {
	w.scenario.awaitPodsReadyWithinSeconds(seconds)
}

// Wait is the state of the cluster a Scenario holds for the control plane to
// report, written as a Step of its own: all three Waits are offered here, because
// a Wait of its own stands wherever a Scenario needs one — before any Workload is
// declared, and after a Failure. A Wait applies what the Givens above it
// declared, which is why a Given reaches no cluster of its own.
type Wait struct {
	scenario *Scenario
}

// AllNodesReady is the Wait for every Node the declared Topology holds: the Wait a
// Scenario writes where Nodes are added or come back, and the same Wait the
// WaitFor of a Topology line opens. It is held to the lab's own waiting time.
func (w Wait) AllNodesReady() {
	w.scenario.awaitNodesReady()
}

// AllNodesReadyWithinSeconds is that Wait held to the time this line states it
// must be reached in, in whole seconds.
func (w Wait) AllNodesReadyWithinSeconds(seconds int) {
	w.scenario.awaitNodesReadyWithinSeconds(seconds)
}

// AllPodsReady is the Wait for every Pod the declared Workload holds: the Wait a
// Scenario writes after a Failure, and the same Wait the WaitFor of a Workload
// line opens. It is held to the lab's own waiting time.
func (w Wait) AllPodsReady() {
	w.scenario.awaitPodsReady()
}

// AllPodsReadyWithinSeconds is that Wait held to the time this line states it
// must be reached in, in whole seconds.
func (w Wait) AllPodsReadyWithinSeconds(seconds int) {
	w.scenario.awaitPodsReadyWithinSeconds(seconds)
}

// PlacementSettled is the Wait for the Placement of the declared Workload alone:
// the Wait a Scenario writes where the Nodes still responding have no room for
// every Pod, since it is reached while those Pods stay Pending and a Wait for
// every Pod Ready never would be. It is offered on a Wait Step of its own and on
// no Given's line, because what it holds for is the Scenario's own state and not
// the declaration of one line. It is held to the lab's own waiting time.
func (w Wait) PlacementSettled() {
	w.scenario.awaitSettledPlacement()
}

// PlacementSettledWithinSeconds is that Wait held to the time this line states it
// must be reached in, in whole seconds.
func (w Wait) PlacementSettledWithinSeconds(seconds int) {
	w.scenario.awaitSettledPlacementWithinSeconds(seconds)
}

// When is what a Scenario does to the cluster: the Failure it injects, and nothing
// else. A Failure acts on the world the Scenario has reached and waits for nothing
// beforehand, so the Placement its Thens judge is the one the Wait after it
// reaches.
type When struct {
	scenario *Scenario
}

// NodesFail stops the stated number of Nodes responding: the first ones the
// declared Topology derives, which makes the pick the same on every run. The Step
// is renamed to carry the Nodes resolved, so a run reports which of them failed,
// and the Scenario is left for the next Wait to reach the Placement the control
// plane settles at.
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

// ZonesFail stops every Node of the stated number of Zones responding: the first
// Zones the declared Topology derives, which makes the pick the same on every run.
// The Step is renamed to carry the Zones resolved — the Zones and not their Nodes,
// since a Zone is what this When fails — so a run reports which of them failed,
// and the Scenario is left for the next Wait to reach the Placement the control
// plane settles at.
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

// stop stops each of these Nodes responding and leaves this Scenario for the Wait
// after it to reach a fresh Placement, ending it at the When that could not stop
// one, and reports whether every one of them stopped. The observation held until now is the
// world before the Failure, so it is discarded rather than judged by the Thens
// after it.
func (s *Scenario) stop(at Step, nodes []string) bool {
	for _, node := range nodes {
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

// podsKeptRunning is what the Workload kept Running at one moment: the Pods this
// observation found Running apart from every Node a When stopped. Both moments a
// Required Capacity is judged at are counted this way — the Placement from before
// the Failure at the moment it was injected, and the Settled Placement after it,
// per docs/adr/2026-10-06-required-capacity-judged-at-two-moments.md.
func (s *Scenario) podsKeptRunning(observed scenario.Placement) int {
	return len(observed.PodsRunningApartFrom(s.failed))
}

// nodesFailing and zonesFailing are how a When's line reads before it names what
// it resolved.
func nodesFailing(count int) string {
	return counted(count, "Node", "Nodes") + failing(count)
}

func zonesFailing(count int) string {
	return counted(count, "Zone", "Zones") + failing(count)
}

// failing is the verb such a line carries, agreeing with the count before it.
func failing(count int) string {
	if count == 1 {
		return " fails"
	}

	return " fail"
}

// Then judges one part of the Placement a Scenario settled at. A Then the
// Placement does not meet records a Finding, and the Steps after it still run.
type Then struct {
	scenario *Scenario
}

// AllPodsRunning asserts every Pod the Workload declared is Running.
func (t Then) AllPodsRunning() {
	t.judge("every Pod Running", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsInPhase(scenario.PodRunning)).
			To(expect.HaveLen[scenario.Pod](t.scenario.workload.Pods()))
	})
}

// PodsPerZone asserts every Zone of the declared Topology holds pods Pods,
// including a Zone holding none.
func (t Then) PodsPerZone(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" per Zone", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsPerZone()).
			To(expect.Equal(evenlyOver(t.scenario.topology.Zones(), pods)))
	})
}

// PodsPerNode asserts every Node of the declared Topology holds pods Pods,
// including a Node holding none.
func (t Then) PodsPerNode(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" per Node", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsPerNode()).
			To(expect.Equal(evenlyOver(nodesOf(t.scenario.topology), pods)))
	})
}

// PodsPending asserts the Placement leaves pods Pods Pending, which is what no
// Node had room for. Why each of them is Pending is the control plane's own
// reason and is on the Timeline; what this Then judges is how many.
func (t Then) PodsPending(pods int) {
	t.judge(counted(pods, "Pod", "Pods")+" Pending", func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, settled.PodsInPhase(scenario.PodPending)).To(expect.HaveLen[scenario.Pod](pods))
	})
}

// NoPodsOnTheFailedNodes asserts no Node a When stopped holds a Pod of the
// Workload. A Node that stopped responding is no Ready Node, so it is in no
// Placement the lab observes; a Pod still on it is counted under its name all the
// same, because the report states what was observed.
func (t Then) NoPodsOnTheFailedNodes() {
	t.judgeWhatFailed(failedDomain{
		named:   "no Pod on the Nodes that failed",
		stopped: t.scenario.failed,
		counted: scenario.Placement.PodsPerNode,
		subject: "the Nodes a When stopped responding",
	})
}

// NoPodsInTheFailedZones asserts no Zone a When stopped whole holds a Pod of the
// Workload. Its Nodes leave the observation as they stop, so a Zone all of whose
// Nodes have left holds none; one still responding keeps its Zone in the
// observation, and a Pod there is counted under it.
func (t Then) NoPodsInTheFailedZones() {
	t.judgeWhatFailed(failedDomain{
		named:   "no Pod in the Zones that failed",
		stopped: t.scenario.failedZones,
		counted: scenario.Placement.PodsPerZone,
		subject: "the Zones a When stopped whole",
	})
}

// The two moments a Required Capacity is judged at, as a Finding names the one it
// was missed at.
const (
	atTheMomentOfTheFailure    = "at the moment of the Failure"
	onceThePlacementHadSettled = "once the Placement had settled"
)

// RequiredCapacityHeld asserts the Workload kept the fewest Pods it declared it
// must: at the moment the Failure was injected, and once the Placement had settled
// again. The Step is renamed to the minimum and both counts before either is
// judged, so a run states what it counted whether the minimum was met or not and a
// Finding carries those counts.
//
// It judges a declaration through a Failure, so a Workload declaring no minimum
// and a Scenario with no When before this Then each leave it nothing to judge, and
// it ends the Scenario at this Step.
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

// minimumToJudge is the Required Capacity this Then judges, and whether there is
// anything to judge at all: a Workload declaring no minimum and a Scenario no When
// ran in each leave this Then nothing, and the Scenario ends at the Step it
// recorded, which names no count because none was reached.
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

// keptAtLeast matches a count of Pods at or above the minimum declared, naming the
// moment it was counted at when it is below, so each moment missed is its own
// Finding under the one Step. It is declared here rather than in internal/expect,
// which keeps its general matchers alone.
func keptAtLeast(minimum int, moment string) expect.Matcher[int] {
	return func(kept int) (string, bool) {
		if kept >= minimum {
			return "", true
		}

		return fmt.Sprintf("%s the Workload kept %s Running, short of the %s its Required Capacity declares",
			moment, counted(kept, "Pod", "Pods"), counted(minimum, "Pod", "Pods")), false
	}
}

// failedDomain is what a Then reading a Failure judges: the domain members a When
// stopped, how a Placement counts Pods over them, how its Step reads, and what the
// refusal names. The two such Thens differ in nothing else.
type failedDomain struct {
	named   string
	stopped []string
	counted func(scenario.Placement) map[string]int
	subject string
}

// judgeWhatFailed records this Then's Step and judges the Settled Placement:
// nothing a When stopped holds a Pod of the Workload, compared as one whole map
// rather than one count per member.
//
// It reads what a When left, so a Scenario with no When before it has nothing to
// judge and ends at this Step.
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

// judge records a Then Step under this name and judges the Settled Placement
// against it. A mismatch becomes a Finding rather than an error, which is what
// lets a Scenario's lines name no error and the Steps after a Finding still run.
func (t Then) judge(name string, against func(scenario.Placement, expect.T)) {
	step, open := t.scenario.record(ThenStep, name)
	if !open {
		return
	}

	t.judgeAt(step, against)
}

// judgeAt judges the Settled Placement against a Then Step already recorded,
// which is what a Then that has something of its own to refuse records first.
func (t Then) judgeAt(step Step, against func(scenario.Placement, expect.T)) {
	settled, observed := t.scenario.placement(step)
	if !observed {
		return
	}

	against(settled, &findings{scenario: t.scenario, step: step})
}

// findings turns what a matcher reported into a Finding of this Scenario. It is
// the Then side of the interface *testing.T satisfies in a go test, so a Then
// judges through internal/expect wherever the Scenario runs.
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

// evenlyOver is the Placement a Then expects: each of names holding the same
// count.
func evenlyOver(names []string, count int) map[string]int {
	expected := make(map[string]int, len(names))
	for _, name := range names {
		expected[name] = count
	}

	return expected
}

// podsIn is what an observation counted over each of these domain members, one it
// counted nothing for included as zero.
func podsIn(members []string, observed map[string]int) map[string]int {
	counts := make(map[string]int, len(members))
	for _, member := range members {
		counts[member] = observed[member]
	}

	return counts
}

// nodesOf are the Node names of the Topology.
func nodesOf(topology scenario.Topology) []string {
	nodes := topology.Nodes()
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}

// counted is how a count reads in a Step's name: the number and its subject,
// singular at one.
func counted(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(count) + " " + plural
}
