// Package lab holds the Step surface a Scenario's lines run against, and the
// runner that provisions a Simulated Cluster for one Scenario and reports what
// that Scenario found.
//
// It is the one package naming both a Step and the provisioned Simulated
// Cluster, so a Scenario source imports it alone of the lab's own packages, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md.
package lab

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// noPod is what a Node that stopped responding holds once the control plane has
// moved its Pods away.
const noPod = 0

// Zones is how many Zones a Topology Step declares, or how many a When Step stops
// responding. A Scenario states the count and the lab resolves which, so no
// Scenario names a Zone by a string its Givens do not derive.
type Zones int

// NodesPerZone is how many Nodes a Topology Step declares in each of its Zones.
type NodesPerZone int

// Pods is a count of Pods: what a Workload Step declares, or what one Node holds.
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

// StepKind is which of the three a Step is.
type StepKind string

const (
	// GivenStep sets up the world a Scenario runs in.
	GivenStep StepKind = "Given"
	// WhenStep injects the Failure a Scenario studies.
	WhenStep StepKind = "When"
	// ThenStep judges the Placement the Scenario settled at.
	ThenStep StepKind = "Then"
)

// Step is one line of a Scenario as it ran: which of the three kinds it is, and
// the name it reports.
type Step struct {
	Kind StepKind
	Name string
}

// String is how a Step reads in a report, the way its line reads in the source.
func (s Step) String() string {
	return string(s.Kind) + " " + s.Name
}

// Finding is one Then the Settled Placement did not meet, naming the Step that
// judged it and the mismatch internal/expect reported. A Finding is what the lab
// is for: it is an observation about Kubernetes, not an error in the lab.
type Finding struct {
	Step     Step
	Mismatch string
}

// Scenario is the Step surface a Scenario's lines run against. Each line is one
// Given, When or Then Step, and none of them names an error: the value carries
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

	observed scenario.Placement
	// deployed is whether the declaration has reached the Simulated Cluster, and
	// settled whether the Placement observed since the last Failure has settled. A
	// When clears the settled flag alone, so the next Step awaits a fresh Settled
	// Placement without the Topology being applied or the Workload deployed again.
	deployed bool
	settled  bool

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

// end stops the Scenario at the Step that could not be carried out. The first
// stop is the one kept, so the Step named is the one a reader looks at.
func (s *Scenario) end(step Step, cause error) {
	if s.cause != nil {
		return
	}

	s.endedAt = step
	s.cause = cause
}

// placement is the Settled Placement every Then judges: the one already observed,
// or a fresh one awaited at this Step. A Scenario awaits one before its first
// Then and one after every When, each within a budget of its own. A Scenario that
// cannot get that far ends at the Step that needed it, naming what it could not
// reach.
func (s *Scenario) placement(at Step) (scenario.Placement, bool) {
	if s.settled {
		return s.observed, true
	}
	if !s.deploy(at) {
		return scenario.Placement{}, false
	}

	settled, err := s.provisioned.AwaitSettled(s.ctx, s.workload)
	if err != nil {
		s.end(at, fmt.Errorf("await the Settled Placement: %w", err))

		return scenario.Placement{}, false
	}

	s.observed, s.settled = settled, true

	return s.observed, true
}

// deploy is where the declaration reaches the Simulated Cluster: the Topology is
// applied and the Workload deployed, once per Scenario, at the first Step that
// needs an observed Placement. A later Step awaits the Placement again without
// declaring the world again.
func (s *Scenario) deploy(at Step) bool {
	if s.deployed {
		return true
	}

	if err := s.provisioned.ApplyTopology(s.ctx, s.topology); err != nil {
		s.end(at, fmt.Errorf("apply the Topology: %w", err))

		return false
	}
	if err := s.provisioned.Deploy(s.ctx, s.workload, s.accesses...); err != nil {
		s.end(at, fmt.Errorf("deploy the Workload: %w", err))

		return false
	}
	s.deployed = true

	return true
}

// Given declares one part of the world a Scenario runs in. Each of its methods
// constructs the immutable internal/scenario value that part is, and ends the
// Scenario at its own line when that constructor refuses the declaration.
type Given struct {
	scenario *Scenario
}

// Topology declares the Zones the Scenario provisions and the Nodes in each of
// them, every Node holding scenario.DefaultNodeCapacity Pods. A further call
// names another capacity.
func (g Given) Topology(zones Zones, nodesPerZone NodesPerZone) DeclaredTopology {
	declared := DeclaredTopology{scenario: g.scenario, zones: zones, nodesPerZone: nodesPerZone}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	return declared.declare(step, scenario.DefaultNodeCapacity)
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
func (g Given) Workload(name WorkloadName, pods Pods, declares PodSpecification) DeclaredWorkload {
	declared := DeclaredWorkload{scenario: g.scenario, name: name, pods: pods, declares: declares}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	workload, err := scenario.NewWorkload(string(name), int(pods), declares.declared)
	if err != nil {
		g.scenario.end(step, err)

		return declared
	}
	g.scenario.workload, g.scenario.accesses = workload, declares.accesses

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
	declares PodSpecification
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
	d.scenario.workload = declared

	return d
}

// named is how this line reads in a report: its Pods, and the intent of the
// specification it declares.
func (d DeclaredWorkload) named() string {
	return fmt.Sprintf("a Workload %s of %s declaring %s",
		d.name, counted(int(d.pods), "Pod", "Pods"), d.declares.name())
}

// DeclaredTopology is the Topology one Given line declared. It is complete as it
// stands, and a further call declares the same line again at another capacity, so
// no half-built Topology ever exists (docs/architecture-principles.md
// § Construction and update).
type DeclaredTopology struct {
	scenario     *Scenario
	zones        Zones
	nodesPerZone NodesPerZone
}

// NodeCapacity declares this line's Topology again, with every Node holding pods
// Pods rather than the default.
func (d DeclaredTopology) NodeCapacity(pods Pods) DeclaredTopology {
	if d.scenario.cause != nil {
		return d
	}

	step := d.scenario.rename(d.named() + " holding " + counted(int(pods), "Pod", "Pods") + " each")

	return d.declare(step, int(pods))
}

// declare constructs the Topology this line names, at a capacity of pods per
// Node, and ends the Scenario at step when either constructor refuses it.
func (d DeclaredTopology) declare(step Step, pods int) DeclaredTopology {
	capacity, err := scenario.NewNodeCapacity(pods)
	if err != nil {
		d.scenario.end(step, err)

		return d
	}
	topology, err := scenario.NewTopology(int(d.zones), int(d.nodesPerZone), capacity)
	if err != nil {
		d.scenario.end(step, err)

		return d
	}
	d.scenario.topology = topology

	return d
}

// named is how this line reads in a report.
func (d DeclaredTopology) named() string {
	return fmt.Sprintf("a Topology of %s of %s",
		counted(int(d.zones), "Zone", "Zones"), counted(int(d.nodesPerZone), "Node", "Nodes"))
}

// When injects the Failure a Scenario studies. It waits for the world to have
// settled before it injects, so what the next Then judges is the Placement the
// control plane reached after the Failure and not the one it was still in.
type When struct {
	scenario *Scenario
}

// NodesFail stops the stated number of Nodes responding: the first ones the
// declared Topology derives, which makes the pick the same on every run. The Step
// is renamed to carry the Nodes resolved, so a run reports which of them failed,
// and the Scenario is left awaiting a fresh Settled Placement.
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
// and the Scenario is left awaiting a fresh Settled Placement.
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

// stop stops each of these Nodes responding and leaves this Scenario awaiting a
// fresh Settled Placement, ending it at the When that could not stop one, and
// reports whether every one of them stopped. The observation held until now is the
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
