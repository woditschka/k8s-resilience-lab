package scenario

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// pendingLabel heads the Pods a board holds on no Node.
const pendingLabel = "Pending"

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
	// StepObject is a Step's note, named by which of Given, When, Wait or Then
	// the Step is and carrying the Step's line.
	StepObject ObjectKind = "Step"
)

// WaitStepNamed names a Wait's note, the one each Wait narrative ends at.
const WaitStepNamed = "Wait"

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

// WaitNarratives gives one narrative per Wait note, in note order. It is a pure
// function of the entries, so a Timeline read back from its file narrates the
// way the run that recorded it did. A change after the last Wait's note stands
// under no Wait.
func (t Timeline) WaitNarratives() []WaitNarrative {
	narrating := newNarration(t.deploymentsNamed())
	var narratives []WaitNarrative
	for _, entry := range t.entries {
		if entry.notesAWait() {
			narratives = append(narratives, narrating.cut())

			continue
		}
		narrating.read(entry)
	}

	return narratives
}

func (t Timeline) deploymentsNamed() []string {
	var named []string
	for _, entry := range t.entries {
		if entry.Kind == DeploymentObject && !slices.Contains(named, entry.Name) {
			named = append(named, entry.Name)
		}
	}

	return named
}

func (e Entry) notesAWait() bool {
	return e.Kind == StepObject && e.Name == WaitStepNamed
}

// WaitNarrative is the moments that changed the Placement before one Wait, in
// the order they happened, and the Pods left Pending as it ended.
type WaitNarrative struct {
	lines []string
}

// Lines carry no indent of their own, so the caller prints them under its own.
func (n WaitNarrative) Lines() []string {
	return slices.Clone(n.lines)
}

// Width is what a separator below the narrative spans at least.
func (n WaitNarrative) Width() int {
	widest := 0
	for _, line := range n.lines {
		widest = max(widest, utf8.RuneCountInString(line))
	}

	return widest
}

// Equal is what go-cmp compares a narrative through, since its field is
// unexported.
func (n WaitNarrative) Equal(other WaitNarrative) bool {
	return slices.Equal(n.lines, other.lines)
}

// narration keeps each object's last reading across the whole Timeline and the
// moments since the last cut.
type narration struct {
	deployments []string
	nodes       map[string]NodeReadiness
	pods        map[string]*podNarrated
	podOrder    []string
	moments     []moment
}

type podNarrated struct {
	named     string
	node      string
	phase     PodPhase
	reason    string
	ran       bool
	evicted   bool
	removed   bool
	scheduled int
}

// noMoment marks a Pod whose scheduling stands under an earlier Wait, so its
// Running reads as a moment of its own.
const noMoment = -1

type moment struct {
	at      time.Duration
	says    string
	running string
}

func (m moment) line() string {
	line := elapsedAt(m.at) + "  " + m.says
	if m.running != "" {
		line += ", Running at " + m.running
	}

	return line
}

func newNarration(deployments []string) *narration {
	return &narration{deployments: deployments, nodes: map[string]NodeReadiness{}, pods: map[string]*podNarrated{}}
}

func (n *narration) read(entry Entry) {
	switch entry.Kind {
	case NodeObject:
		n.readNode(entry)
	case PodObject:
		n.readPod(entry)
	default:
		// A Deployment, a ReplicaSet, a Step or a note about the record changes
		// no Placement.
	}
}

// A Node first seen counts as NotReady, so its coming up reads as turning
// Ready and a Node first seen NotReady reads as nothing.
func (n *narration) readNode(entry Entry) {
	if !entry.observed() || entry.Change == ChangeRemoved {
		return
	}

	readiness := NodeReadiness(entry.Reason)
	last, seen := n.nodes[entry.Name]
	if !seen {
		last = NodeNotReady
	}
	n.nodes[entry.Name] = readiness
	if readiness != last {
		n.moments = append(n.moments, moment{at: entry.Elapsed, says: "Node " + entry.Name + " " + string(readiness)})
	}
}

// A Pod no Deployment claims is the lab's own setup and in no narrative.
func (n *narration) readPod(entry Entry) {
	pod, claimed := n.podOf(entry.Name)
	if !claimed {
		return
	}
	if !entry.observed() {
		pod.reason = entry.reasonGiven()

		return
	}

	if entry.Change == ChangeRemoved {
		pod.removed = true
	}
	phase := PodPhase(entry.Reason)
	if phase == PodTerminating {
		n.evict(pod, entry)

		return
	}
	if pod.node == "" && entry.Detail != "" {
		pod.node = entry.Detail
		pod.scheduled = len(n.moments)
		n.moments = append(n.moments, moment{at: entry.Elapsed, says: "Pod " + pod.named + " scheduled on " + pod.node})
	}
	pod.phase = phase
	if phase == PodRunning && !pod.ran {
		n.run(pod, entry.Elapsed)
	}
}

// The lab deletes no Deployment Pod, so the first entry marking one for deletion
// is its eviction.
func (n *narration) evict(pod *podNarrated, entry Entry) {
	pod.phase = PodTerminating
	if pod.evicted {
		return
	}

	pod.evicted = true
	n.moments = append(n.moments, moment{at: entry.Elapsed, says: "Pod " + pod.named + " evicted from " + pod.node})
}

func (n *narration) run(pod *podNarrated, at time.Duration) {
	pod.ran = true
	if pod.scheduled != noMoment {
		n.moments[pod.scheduled].running = elapsedAt(at)

		return
	}

	n.moments = append(n.moments, moment{at: at, says: "Pod " + pod.named + " Running on " + pod.node})
}

func (n *narration) podOf(name string) (*podNarrated, bool) {
	if pod, seen := n.pods[name]; seen {
		return pod, true
	}

	deployment := deploymentOf(name, n.deployments)
	if deployment == "" {
		return nil, false
	}

	pod := &podNarrated{named: boardNameOf(deployment, name), scheduled: noMoment}
	n.pods[name] = pod
	n.podOrder = append(n.podOrder, name)

	return pod, true
}

// cut ends one Wait's narrative with the Pods left Pending as it ended, and
// starts the next.
func (n *narration) cut() WaitNarrative {
	lines := make([]string, 0, len(n.moments))
	for _, happened := range n.moments {
		lines = append(lines, happened.line())
	}
	for _, name := range n.podOrder {
		if pod := n.pods[name]; pod.leftPending() {
			lines = append(lines, pod.pendingLine())
		}
	}

	n.moments = nil
	for _, pod := range n.pods {
		pod.scheduled = noMoment
	}

	return WaitNarrative{lines: lines}
}

func (p *podNarrated) leftPending() bool {
	return p.node == "" && p.phase == PodPending && !p.removed
}

func (p *podNarrated) pendingLine() string {
	line := "Pod " + p.named + " left Pending"
	if p.reason != "" {
		line += ", " + p.reason
	}

	return line
}

// The longest match wins where two Deployment names nest.
func deploymentOf(pod string, deployments []string) string {
	claimed := ""
	for _, deployment := range deployments {
		if strings.HasPrefix(pod, deployment+"-") && len(deployment) > len(claimed) {
			claimed = deployment
		}
	}

	return claimed
}

func elapsedAfter(elapsed time.Duration) string {
	return fmt.Sprintf("%.3fs", elapsed.Seconds())
}

func elapsedAt(elapsed time.Duration) string {
	return "+" + elapsedAfter(elapsed)
}
