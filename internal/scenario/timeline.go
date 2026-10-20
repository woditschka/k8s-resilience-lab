package scenario

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// A Change may be a note the Timeline makes about itself, so an incomplete
// Timeline says so rather than reading complete.
type Change string

const (
	ChangeAdded    Change = "added"
	ChangeModified Change = "modified"
	ChangeRemoved  Change = "removed"
	// ChangeReported is where the reason for a Placement comes from.
	ChangeReported Change = "reported"
	ChangeNoted    Change = "noted"
)

// ObjectKind names only the kinds a Placement is explained by, the ones a
// Timeline records.
type ObjectKind string

const (
	NodeObject ObjectKind = "Node"
	// PodObject covers a probe Pod of provisioning too.
	PodObject        ObjectKind = "Pod"
	DeploymentObject ObjectKind = "Deployment"
	ReplicaSetObject ObjectKind = "ReplicaSet"
)

// NodeReadiness is the one thing about a Node a Placement depends on.
type NodeReadiness string

const (
	NodeReady    NodeReadiness = "Ready"
	NodeNotReady NodeReadiness = "NotReady"
)

// Entry for a note names no object and carries its text as the detail. Every
// field is an observation rather than a value the lab decides, so Entry carries
// no invariant of its own and is built with a keyed literal.
type Entry struct {
	Elapsed time.Duration
	Change  Change
	Kind    ObjectKind
	Name    string
	// Either Reason or Detail may be absent.
	Reason string
	Detail string
}

func (e Entry) Line() string {
	line := elapsedAt(e.Elapsed) + "  " + e.changed()
	if reason := e.reasonGiven(); reason != "" {
		line += ", " + reason
	}

	return line
}

// An observed entry's Reason is the phase or readiness the object was held in,
// rather than something the control plane said about it or a note.
func (e Entry) observed() bool {
	return e.Change == ChangeAdded || e.Change == ChangeModified || e.Change == ChangeRemoved
}

func (e Entry) changed() string {
	if e.Kind == "" {
		return string(e.Change)
	}

	return fmt.Sprintf("%s %s %s", e.Kind, e.Name, e.Change)
}

func (e Entry) reasonGiven() string {
	switch {
	case e.Reason != "" && e.Detail != "":
		return e.Reason + ": " + e.Detail
	case e.Reason != "":
		return e.Reason
	default:
		return e.Detail
	}
}

// Timeline is the ordered record of what changed in a Scenario's Simulated
// Cluster while the Scenario ran, which explains its Placement.
type Timeline struct {
	entries []Entry
}

func NewTimeline(entries []Entry) Timeline {
	return Timeline{entries: slices.Clone(entries)}
}

func (t Timeline) Entries() []Entry {
	return slices.Clone(t.entries)
}

// Summarise is a pure function of the entries, so a Timeline read back from its
// file summarises the way the run that recorded it did.
func (t Timeline) Summarise() TimelineSummary {
	reading := t.read()

	return TimelineSummary{
		entries:   len(t.entries),
		topology:  reading.topology(),
		workloads: reading.workloads(),
	}
}

// Every part of the summary derives from this one pass over the entries.
func (t Timeline) read() timelineReading {
	reading := timelineReading{nodes: newNamedEntries(), deployments: newNamedEntries(), pods: newNamedEntries()}
	for _, entry := range t.entries {
		switch entry.Kind {
		case NodeObject:
			reading.nodes.add(entry)
		case DeploymentObject:
			reading.deployments.add(entry)
		case PodObject:
			reading.pods.add(entry)
		case ReplicaSetObject:
			// A ReplicaSet entry names the Deployment's own replica set, which
			// the Deployment entries already cover.
		default:
			// A note names no object, so it reads in the entry count alone.
		}
	}

	return reading
}

// The labels share one column width so a summary scans down.
const (
	topologyLabel = "Topology"
	workloadLabel = "Workload"
	pendingLabel  = "Pending"
	nothingNamed  = "none"
	labelWidth    = 8
)

// TimelineSummary holds no file path, because where a Timeline was filed is the
// run's to say rather than the record's.
type TimelineSummary struct {
	entries   int
	topology  topologyReading
	workloads []workloadReading
}

func (s TimelineSummary) EntryCount() int {
	return s.entries
}

// Lines gives lines carrying no indent of their own, so the caller prints them
// under its own and composes no wording.
func (s TimelineSummary) Lines() []string {
	lines := []string{s.topology.line()}
	if len(s.workloads) == 0 {
		lines = append(lines, labelled(workloadLabel, nothingNamed))
	}

	for _, workload := range s.workloads {
		lines = append(lines, workload.lines()...)
	}

	return append(lines, labelled(pendingLabel, s.podsLeftPending()))
}

func (s TimelineSummary) podsLeftPending() string {
	var left []string
	for _, workload := range s.workloads {
		left = append(left, workload.podsLeftPending()...)
	}

	if len(left) == 0 {
		return nothingNamed
	}

	return strings.Join(left, ", ")
}

type topologyReading struct {
	nodes   int
	ready   int
	readyAt time.Duration
}

func (t topologyReading) line() string {
	if t.nodes == 0 {
		return labelled(topologyLabel, "no Nodes")
	}

	counted := fmt.Sprintf("%d of %d Nodes %s", t.ready, t.nodes, NodeReady)
	if t.ready == 0 {
		return labelled(topologyLabel, counted)
	}

	return labelled(topologyLabel, counted+" after "+elapsedAfter(t.readyAt))
}

type workloadReading struct {
	name      string
	readiness string
	readyAt   time.Duration
	pods      []podReading
}

func (w workloadReading) lines() []string {
	lines := make([]string, 0, len(w.pods)+1)
	lines = append(lines, labelled(workloadLabel, w.reached()))
	for _, pod := range w.pods {
		lines = append(lines, podIndent+pod.line())
	}

	return lines
}

// A Workload the control plane never reported on reads as its name.
func (w workloadReading) reached() string {
	if w.readiness == "" {
		return w.name
	}

	return w.name + ", " + w.readiness + " after " + elapsedAfter(w.readyAt)
}

func (w workloadReading) podsLeftPending() []string {
	var left []string
	for _, pod := range w.pods {
		if pod.leftPending() {
			left = append(left, pod.name)
		}
	}

	return left
}

const podIndent = "  "

type podReading struct {
	name   string
	path   []podStep
	reason string
}

func (p podReading) line() string {
	line := p.name
	if taken := p.pathTaken(); taken != "" {
		line += "  " + taken
	}
	if p.leftPending() && p.reason != "" {
		line += ", " + p.reason
	}

	return line
}

// The whole path is printed, so a Pod placed again after a Failure reads as
// more than its first Placement.
func (p podReading) pathTaken() string {
	taken := make([]string, 0, len(p.path))
	for _, step := range p.path {
		taken = append(taken, step.read())
	}

	return strings.Join(taken, " -> ")
}

// A Pod the lab never read a phase for counts as unplaced: it was never seen
// running.
func (p podReading) leftPending() bool {
	last, taken := p.lastStep()

	return !taken || last.phase == PodPending
}

func (p podReading) lastStep() (podStep, bool) {
	if len(p.path) == 0 {
		return podStep{}, false
	}

	return p.path[len(p.path)-1], true
}

type podStep struct {
	phase PodPhase
	node  string
	at    time.Duration
}

func (s podStep) read() string {
	if s.node == "" {
		return string(s.phase) + " " + elapsedAt(s.at)
	}

	return string(s.phase) + " on " + s.node + " " + elapsedAt(s.at)
}

type timelineReading struct {
	nodes       namedEntries
	deployments namedEntries
	pods        namedEntries
}

func (r timelineReading) topology() topologyReading {
	reading := topologyReading{nodes: len(r.nodes.order)}
	for _, name := range r.nodes.order {
		readyAt, reported := firstReadyAt(r.nodes.of(name))
		if !reported {
			continue
		}

		reading.ready++
		reading.readyAt = max(reading.readyAt, readyAt)
	}

	return reading
}

func (r timelineReading) workloads() []workloadReading {
	readings := make([]workloadReading, 0, len(r.deployments.order))
	for _, name := range r.deployments.order {
		readiness, readyAt := readinessReached(r.deployments.of(name))
		readings = append(readings, workloadReading{
			name: name, readiness: readiness, readyAt: readyAt, pods: r.podsOf(name),
		})
	}

	return readings
}

func (r timelineReading) podsOf(workload string) []podReading {
	var readings []podReading
	for _, name := range r.pods.order {
		if r.workloadOf(name) != workload {
			continue
		}

		readings = append(readings, podRead(name, r.pods.of(name)))
	}

	return readings
}

// The longest match wins where two Workload names nest. A Pod no Workload
// claims is the lab's own setup and reads in the Topology line alone.
func (r timelineReading) workloadOf(pod string) string {
	claimed := ""
	for _, workload := range r.deployments.order {
		if strings.HasPrefix(pod, workload+"-") && len(workload) > len(claimed) {
			claimed = workload
		}
	}

	return claimed
}

// Readiness is read from changes to the Node itself rather than from what the
// control plane said about it.
func firstReadyAt(changes []Entry) (time.Duration, bool) {
	for _, entry := range changes {
		if entry.observed() && entry.Reason == string(NodeReady) {
			return entry.Elapsed, true
		}
	}

	return 0, false
}

func readinessReached(changes []Entry) (string, time.Duration) {
	reached := ""
	for _, entry := range changes {
		if entry.observed() {
			reached = entry.Detail
		}
	}

	for _, entry := range changes {
		if entry.observed() && entry.Detail == reached {
			return reached, entry.Elapsed
		}
	}

	return reached, 0
}

// A further change in the same phase is the scheduler binding the Pod to a
// Node, which the current step already reads as.
func podRead(name string, changes []Entry) podReading {
	reading := podReading{name: name}
	for _, entry := range changes {
		if !entry.observed() {
			reading.reason = entry.reasonGiven()

			continue
		}

		step := podStep{phase: PodPhase(entry.Reason), node: entry.Detail, at: entry.Elapsed}
		if last, taken := reading.lastStep(); taken && last.phase == step.phase {
			continue
		}

		reading.path = append(reading.path, step)
	}

	return reading
}

// namedEntries keeps the order each object was first named in, which is the
// order a summary reads.
type namedEntries struct {
	order   []string
	grouped map[string][]Entry
}

func newNamedEntries() namedEntries {
	return namedEntries{grouped: map[string][]Entry{}}
}

func (n *namedEntries) add(entry Entry) {
	if _, named := n.grouped[entry.Name]; !named {
		n.order = append(n.order, entry.Name)
	}

	n.grouped[entry.Name] = append(n.grouped[entry.Name], entry)
}

func (n namedEntries) of(name string) []Entry {
	return n.grouped[name]
}

func labelled(label, read string) string {
	return fmt.Sprintf("%-*s  %s", labelWidth, label, read)
}

func elapsedAfter(elapsed time.Duration) string {
	return fmt.Sprintf("%.3fs", elapsed.Seconds())
}

func elapsedAt(elapsed time.Duration) string {
	return "+" + elapsedAfter(elapsed)
}
