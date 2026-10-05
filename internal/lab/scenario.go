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

// evenSkew is the maximum Skew that makes a spread even: the fullest and the
// emptiest domain differ by at most one Pod.
const evenSkew = 1

// noPod is what a Node that stopped responding holds once the control plane has
// moved its Pods away.
const noPod = 0

// Zones is how many Zones a Topology Step declares.
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

// The domains a Workload's Pods are spread over, named here so a Scenario source
// imports internal/lab alone.
const (
	// OverZones spreads a Workload's Pods over the Zones of its Topology.
	OverZones = scenario.SpreadOverZones
	// OverNodes spreads a Workload's Pods over the Nodes of its Topology.
	OverNodes = scenario.SpreadOverNodes
)

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

	steps    []Step
	findings []Finding

	// failed are the Nodes every When of this Scenario stopped responding, by
	// name and in the order they were stopped, which is what the Thens reading a
	// Failure judge against.
	failed []string

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
	if err := s.provisioned.Deploy(s.ctx, s.workload); err != nil {
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

// Workload declares the Pod replicas the Scenario deploys, under the name their
// Pods are labelled with. A further call spreads them.
func (g Given) Workload(name WorkloadName, pods Pods) DeclaredWorkload {
	declared := DeclaredWorkload{scenario: g.scenario, name: name, pods: pods}
	step, open := g.scenario.record(GivenStep, declared.named())
	if !open {
		return declared
	}

	return declared.declare(step)
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

// DeclaredWorkload is the Workload one Given line declared, complete as it
// stands and declaring no spread. A further call declares the same line again
// with the spread its Pods follow.
type DeclaredWorkload struct {
	scenario *Scenario
	name     WorkloadName
	pods     Pods
	spread   []scenario.SpreadDomain
}

// SpreadEvenly declares this line's Workload again, with its Pods spread as
// evenly over each domain named as the control plane can place them: the fullest
// and the emptiest domain differ by at most one Pod.
func (d DeclaredWorkload) SpreadEvenly(domains ...scenario.SpreadDomain) DeclaredWorkload {
	if d.scenario.cause != nil {
		return d
	}

	spread := DeclaredWorkload{scenario: d.scenario, name: d.name, pods: d.pods, spread: domains}

	return spread.declare(d.scenario.rename(spread.named()))
}

// declare constructs the Workload this line names, with one even spread rule per
// domain it declared, and ends the Scenario at step when a constructor refuses it.
func (d DeclaredWorkload) declare(step Step) DeclaredWorkload {
	rules := make([]scenario.SpreadRule, 0, len(d.spread))
	for _, domain := range d.spread {
		rule, err := scenario.NewSpreadRule(domain, evenSkew)
		if err != nil {
			d.scenario.end(step, err)

			return d
		}
		rules = append(rules, rule)
	}

	workload, err := scenario.NewWorkload(string(d.name), int(d.pods), rules...)
	if err != nil {
		d.scenario.end(step, err)

		return d
	}
	d.scenario.workload = workload

	return d
}

// named is how this line reads in a report.
func (d DeclaredWorkload) named() string {
	named := fmt.Sprintf("a Workload %s of %s", d.name, counted(int(d.pods), "Pod", "Pods"))
	if len(d.spread) == 0 {
		return named
	}

	over := make([]string, 0, len(d.spread))
	for _, domain := range d.spread {
		over = append(over, "over the "+string(domain))
	}

	return named + " spread evenly " + strings.Join(over, " and ")
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

// stop stops each of these Nodes responding and leaves this Scenario awaiting a
// fresh Settled Placement, ending it at the When that could not stop one. The
// observation held until now is the world before the Failure, so it is discarded
// rather than judged by the Thens after it.
func (s *Scenario) stop(at Step, nodes []string) {
	for _, node := range nodes {
		if err := s.provisioned.FailNode(s.ctx, node); err != nil {
			s.end(at, fmt.Errorf("stop Node %s responding: %w", node, err))

			return
		}
		s.failed = append(s.failed, node)
	}

	s.observed, s.settled = scenario.Placement{}, false
}

// nodesFailing is how a When's line reads before it names the Nodes it resolved.
func nodesFailing(count int) string {
	if count == 1 {
		return counted(count, "Node", "Nodes") + " fails"
	}

	return counted(count, "Node", "Nodes") + " fail"
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
			To(expect.Equal(evenlyOver(zonesOf(t.scenario.topology), pods)))
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
//
// It reads what a When left, so a Scenario with no When before it has nothing to
// judge and ends at this Step.
func (t Then) NoPodsOnTheFailedNodes() {
	step, open := t.scenario.record(ThenStep, "no Pod on the Nodes that failed")
	if !open {
		return
	}

	failed := slices.Clone(t.scenario.failed)
	if len(failed) == 0 {
		t.scenario.end(step, errors.New("this Then judges the Nodes a When stopped responding, and no When ran before it"))

		return
	}

	t.judgeAt(step, func(settled scenario.Placement, judged expect.T) {
		expect.Expect(judged, podsOn(failed, settled)).To(expect.Equal(evenlyOver(failed, noPod)))
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

// podsOn counts this observation's Pods on each of these Nodes, a Node holding
// none included as zero, so a Then over the failed Nodes compares one whole map
// rather than one count per Node.
func podsOn(nodes []string, settled scenario.Placement) map[string]int {
	observed := settled.PodsPerNode()
	counts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		counts[node] = observed[node]
	}

	return counts
}

// zonesOf are the Zone names of the Topology, each named once.
func zonesOf(topology scenario.Topology) []string {
	var zones []string
	seen := make(map[string]bool)
	for _, node := range topology.Nodes() {
		if !seen[node.Zone] {
			seen[node.Zone] = true
			zones = append(zones, node.Zone)
		}
	}

	return zones
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
