package scenario

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Change is what one entry of a Timeline records: one of the four changes the
// lab watched an object through, or a note the Timeline makes about itself, so a
// Timeline that is incomplete says so rather than reading complete.
type Change string

const (
	// ChangeAdded is an object the cluster did not hold before.
	ChangeAdded Change = "added"
	// ChangeModified is an object the control plane changed.
	ChangeModified Change = "modified"
	// ChangeRemoved is an object the cluster no longer holds.
	ChangeRemoved Change = "removed"
	// ChangeReported is the control plane reporting something about an object,
	// which is where the reason for a Placement comes from.
	ChangeReported Change = "reported"
	// ChangeNoted is the lab's own note about the record, naming no object.
	ChangeNoted Change = "noted"
)

// ObjectKind is which kind of object one entry names. A Timeline records the
// kinds a Placement is explained by, and no entry names a kind outside them.
type ObjectKind string

const (
	// NodeObject is one simulated machine of the Topology.
	NodeObject ObjectKind = "Node"
	// PodObject is one replica of the Workload, or one probe Pod of provisioning.
	PodObject ObjectKind = "Pod"
	// DeploymentObject is the Workload as the control plane holds it.
	DeploymentObject ObjectKind = "Deployment"
	// ReplicaSetObject is the Deployment's own replica set.
	ReplicaSetObject ObjectKind = "ReplicaSet"
)

// NodeReadiness is the Ready condition the control plane reports for one Node,
// which is the one thing about a Node a Placement depends on. A Timeline reads a
// Node change through it, the way it reads a Pod change through PodPhase.
type NodeReadiness string

const (
	// NodeReady is a Node the control plane reports Ready, so it can hold a Pod.
	NodeReady NodeReadiness = "Ready"
	// NodeNotReady is a Node the control plane does not report Ready.
	NodeNotReady NodeReadiness = "NotReady"
)

// Entry is one change in a Timeline: how long after the record started the lab
// received it, what changed about which object, and the reason the control plane
// gave where it gave one. A note names no object and carries its text as the
// detail.
//
// Every field is an observation the lab read rather than a value it decides, the
// way Pod's are, so the type carries no invariant of its own and is built with a
// keyed literal (docs/architecture-principles.md § Building Blocks).
type Entry struct {
	Elapsed time.Duration
	Change  Change
	Kind    ObjectKind
	Name    string
	// Reason is the control plane's own word for what happened, and Detail what
	// it said about it or what the lab observed. Either may be absent.
	Reason string
	Detail string
}

// Line is how this entry reads in a printed Timeline: the time since the record
// started, what changed, and the reason given for it.
func (e Entry) Line() string {
	line := elapsedAt(e.Elapsed) + "  " + e.changed()
	if reason := e.reasonGiven(); reason != "" {
		line += ", " + reason
	}

	return line
}

// observed reports whether this entry is a change to the object itself, whose
// Reason is the phase or the readiness the control plane held it in, rather than
// something the control plane said about it or a note about the record.
func (e Entry) observed() bool {
	return e.Change == ChangeAdded || e.Change == ChangeModified || e.Change == ChangeRemoved
}

// changed is the object this entry is about and what happened to it, which for a
// note is the note's own kind of change alone.
func (e Entry) changed() string {
	if e.Kind == "" {
		return string(e.Change)
	}

	return fmt.Sprintf("%s %s %s", e.Kind, e.Name, e.Change)
}

// reasonGiven is the reason and the detail this entry carries, either of which
// may be absent.
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

// Timeline is the ordered record of what changed in one Scenario's Simulated
// Cluster: its entries in the order the lab received them.
type Timeline struct {
	entries []Entry
}

// NewTimeline is the record of the entries given, in the order they were
// received.
func NewTimeline(entries []Entry) Timeline {
	return Timeline{entries: slices.Clone(entries)}
}

// Entries are the changes this Timeline holds, in receipt order.
func (t Timeline) Entries() []Entry {
	return slices.Clone(t.entries)
}

// Summarise is this Timeline read as the report a run prints: the Topology the
// lab set up, each Workload the Scenario deployed, each of that Workload's Pods
// with the path it took, and the Pods left Pending. It is a pure function of the
// entries the Timeline holds, so a Timeline read back from its file summarises
// the way the run that recorded it did
// (docs/adr/2026-10-05-timeline-summary-derived-from-the-record.md).
func (t Timeline) Summarise() TimelineSummary {
	reading := t.read()

	return TimelineSummary{
		entries:   len(t.entries),
		topology:  reading.topology(),
		workloads: reading.workloads(),
	}
}

// read is the one pass over the entries every part of the summary is derived
// from: each kind the summary groups by, grouped under the object it names
// (docs/system-design.md § Scale and Load).
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
			// A ReplicaSet entry names the Deployment's own replica set rather than
			// the Workload, which the Deployment entries already name.
		default:
			// A note names no object, so it reads in the entry count alone.
		}
	}

	return reading
}

// The labels the summary's lines read under, and what it says where there is
// nothing to name. The labels share one column width so a summary scans down.
const (
	topologyLabel = "Topology"
	workloadLabel = "Workload"
	pendingLabel  = "Pending"
	nothingNamed  = "none"
	labelWidth    = 8
)

// TimelineSummary is one Timeline read as the report a run prints: how many
// entries it holds, the Topology the lab set up, and each Workload with its
// Pods. It holds no file path, because where a Timeline was filed is the run's
// to say rather than the record's.
type TimelineSummary struct {
	entries   int
	topology  topologyReading
	workloads []workloadReading
}

// EntryCount is how many entries the summarised Timeline holds, which is what
// says how much of the record the summary reads down to.
func (s TimelineSummary) EntryCount() int {
	return s.entries
}

// Lines is how this summary reads, one line per group, with a Workload's Pods
// indented under it. The lines carry no indent of their own, so the caller
// prints them under its own and composes no wording.
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

// podsLeftPending names the Workload Pods the control plane never placed, which
// is what a reader of a Scenario that did not settle is looking for.
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

// topologyReading is the lab's own setup of the Topology collapsed to one line:
// how many Nodes the Timeline named, how many of them reported Ready, and when
// the last of those did.
type topologyReading struct {
	nodes   int
	ready   int
	readyAt time.Duration
}

// line is how the Topology reads: the Nodes that reported Ready out of those the
// Timeline named, and when the Topology was whole.
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

// workloadReading is one Workload the Scenario deployed: the readiness its last
// entry reported, when it first read that way, and its Pods in the order the
// Timeline first named them.
type workloadReading struct {
	name      string
	readiness string
	readyAt   time.Duration
	pods      []podReading
}

// lines are the Workload's own line and one line per Pod beneath it.
func (w workloadReading) lines() []string {
	lines := make([]string, 0, len(w.pods)+1)
	lines = append(lines, labelled(workloadLabel, w.reached()))
	for _, pod := range w.pods {
		lines = append(lines, podIndent+pod.line())
	}

	return lines
}

// reached is the Workload and the readiness it reached, in the control plane's
// own words. A Workload the control plane never reported on reads as its name.
func (w workloadReading) reached() string {
	if w.readiness == "" {
		return w.name
	}

	return w.name + ", " + w.readiness + " after " + elapsedAfter(w.readyAt)
}

// podsLeftPending names this Workload's Pods the control plane never placed.
func (w workloadReading) podsLeftPending() []string {
	var left []string
	for _, pod := range w.pods {
		if pod.leftPending() {
			left = append(left, pod.name)
		}
	}

	return left
}

// podIndent is how far a Pod's line sits under the Workload it belongs to.
const podIndent = "  "

// podReading is one Workload Pod: the path it took through the phases the
// control plane held it in, and the control plane's last word on it, which is
// the reason a Pod still Pending carries.
type podReading struct {
	name   string
	path   []podStep
	reason string
}

// line is how this Pod reads: the path it took and, where it was left Pending,
// the reason the control plane gave for that.
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

// pathTaken is the whole path this Pod took, step by step, so a Pod placed again
// after a Failure reads as more than its first Placement.
func (p podReading) pathTaken() string {
	taken := make([]string, 0, len(p.path))
	for _, step := range p.path {
		taken = append(taken, step.read())
	}

	return strings.Join(taken, " -> ")
}

// leftPending reports whether the control plane's last word left this Pod
// unplaced, which is when its own reason for that is worth printing. A Pod the
// lab never read a phase for counts as unplaced: it was never seen running.
func (p podReading) leftPending() bool {
	last, taken := p.lastStep()

	return !taken || last.phase == PodPending
}

// lastStep is the last step of the path this Pod took, and whether it took one.
func (p podReading) lastStep() (podStep, bool) {
	if len(p.path) == 0 {
		return podStep{}, false
	}

	return p.path[len(p.path)-1], true
}

// podStep is one step of a Pod's path: a phase the control plane put the Pod in,
// when it did, and the Node the Pod was on as it entered that phase, where it
// was on one.
type podStep struct {
	phase PodPhase
	node  string
	at    time.Duration
}

// read is how one step of a path reads: the phase, where it was placed, and when.
func (s podStep) read() string {
	if s.node == "" {
		return string(s.phase) + " " + elapsedAt(s.at)
	}

	return string(s.phase) + " on " + s.node + " " + elapsedAt(s.at)
}

// timelineReading is what one pass over a Timeline's entries leaves: the entries
// of each kind the summary groups by, under the object each names.
type timelineReading struct {
	nodes       namedEntries
	deployments namedEntries
	pods        namedEntries
}

// topology is how the Nodes the Timeline named read as one line: the Nodes that
// reported Ready, and when the last of them did.
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

// workloads are the Workloads the Timeline's Deployment entries named, in the
// order it first named them, each with its own Pods.
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

// podsOf are one Workload's Pods, in the order the Timeline first named them.
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

// workloadOf is the Workload a Pod belongs to: the one whose name the Pod's own
// begins with, followed by a hyphen, at the longest match where two Workload
// names nest. A Pod no Workload claims is the lab's own setup, and reads in the
// Topology line alone.
func (r timelineReading) workloadOf(pod string) string {
	claimed := ""
	for _, workload := range r.deployments.order {
		if strings.HasPrefix(pod, workload+"-") && len(workload) > len(claimed) {
			claimed = workload
		}
	}

	return claimed
}

// firstReadyAt is when a Node first reported Ready, read from the changes to the
// Node itself rather than from what the control plane said about it.
func firstReadyAt(changes []Entry) (time.Duration, bool) {
	for _, entry := range changes {
		if entry.observed() && entry.Reason == string(NodeReady) {
			return entry.Elapsed, true
		}
	}

	return 0, false
}

// readinessReached is the readiness a Workload's last entry reported and when
// the first entry reading the same was received, which is when the Workload
// reached it.
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

// podRead is one Pod read from the changes naming it: one step per phase the
// control plane put it in, and the control plane's last word on the Pod. A
// further change in the same phase is the scheduler binding the Pod to a Node
// on its way into the next one, which the step it is already on reads as.
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

// namedEntries groups entries under the object each names, keeping the order the
// Timeline first named each object in, which is the order a summary reads.
type namedEntries struct {
	order   []string
	grouped map[string][]Entry
}

func newNamedEntries() namedEntries {
	return namedEntries{grouped: map[string][]Entry{}}
}

// add files one entry under the object it names.
func (n *namedEntries) add(entry Entry) {
	if _, named := n.grouped[entry.Name]; !named {
		n.order = append(n.order, entry.Name)
	}

	n.grouped[entry.Name] = append(n.grouped[entry.Name], entry)
}

// of are the entries naming one object, in receipt order.
func (n namedEntries) of(name string) []Entry {
	return n.grouped[name]
}

// labelled is one line of a summary under its label, every label in one column.
func labelled(label, read string) string {
	return fmt.Sprintf("%-*s  %s", labelWidth, label, read)
}

// elapsedAfter is how long after the record started something was reached, and
// elapsedAt is when one change was received, as a printed Timeline reads it.
func elapsedAfter(elapsed time.Duration) string {
	return fmt.Sprintf("%.3fs", elapsed.Seconds())
}

func elapsedAt(elapsed time.Duration) string {
	return "+" + elapsedAfter(elapsed)
}
