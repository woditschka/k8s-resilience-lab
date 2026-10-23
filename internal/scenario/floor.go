package scenario

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// WhenStepNamed names a When's note, the one a Floor is first judged at.
const WhenStepNamed = "When"

// FloorReading is one Deployment's Ready Pods read against its Floor from the
// first When's note to the last entry read.
type FloorReading struct {
	deployment Deployment
	failures   int

	lowest    int
	lowestAt  time.Duration
	lowestFor lasting

	// The fall is the one reaching the lowest count, and lost the Pods Ready
	// at the first When's note that were not at that lowest moment.
	fell    bool
	fallAt  time.Duration
	fallFor lasting
	lost    []string
}

// lasting is open while the reading it times has not ended by the last entry.
type lasting struct {
	took  time.Duration
	ended bool
}

func (l lasting) reads() string {
	if !l.ended {
		return "for the rest of the Scenario"
	}

	return "for " + elapsedAfter(l.took)
}

// Kept reports whether the Ready Pods never fell below the Floor.
func (r FloorReading) Kept() bool {
	return !r.fell
}

// Line states the reading whether the Floor held or not.
func (r FloorReading) Line() string {
	verdict := "kept its Floor"
	if r.fell {
		verdict = "fell below its Floor"
	}
	tolerance, _ := r.deployment.FailureTolerance()

	return fmt.Sprintf("%s %s of %s through the loss of %s: lowest %d of %d at %s %s",
		r.deployment.Name(), verdict, readyPods(r.deployment.Floor()), tolerance.Reads(),
		r.lowest, r.deployment.Replicas(), elapsedAt(r.lowestAt), r.lowestFor.reads())
}

// Fall is the Finding a reading below its Floor records, and empty for one
// that kept it.
func (r FloorReading) Fall() string {
	if !r.fell {
		return ""
	}

	replicas, floor := r.deployment.Replicas(), r.deployment.Floor()
	fall := fmt.Sprintf("%s fell to %d of %d Pods Ready, below its Floor of %d from %s %s: "+
		"%s lost to %s, where %d is the most it survives losing",
		r.deployment.Name(), r.lowest, replicas, floor, elapsedAt(r.fallAt), r.fallFor.reads(),
		podsCounted(replicas-r.lowest), r.theFailures(), replicas-floor)
	if len(r.lost) == 0 {
		return fall
	}

	return fall + "; " + strings.Join(r.lost, ", ")
}

func (r FloorReading) theFailures() string {
	if r.failures > 1 {
		return "the Failures"
	}

	return "the Failure"
}

// Floors reads each judged Deployment from the first When's note, pairing the
// k-th When note with the k-th set of Nodes stopped. A Pod counts while its
// latest reading is Running on a Node no When noted so far has stopped, so a Pod
// the control plane still reports Running on a stopped Node is lost from that
// note. A Pod belongs to the declared Deployment whose name begins its own, at
// the longest match. A Timeline holding no When's note judges nothing.
func (t Timeline) Floors(judged []Deployment, declared []string, stoppedPerWhen [][]string) ([]FloorReading, error) {
	replay := newFloorReplay(judged, declared)
	for _, entry := range t.entries {
		switch {
		case entry.notesAWhen():
			replay.when(stoppedPerWhen)
		case entry.Kind == PodObject && entry.observed():
			replay.readPod(entry)
		default:
			continue
		}
		replay.observe(entry.Elapsed)
	}
	if replay.whens == 0 {
		return nil, errors.New("the Timeline holds no When's note, so no Floor can be judged from the first Failure")
	}

	return replay.readings(), nil
}

func (e Entry) notesAWhen() bool {
	return e.Kind == StepObject && e.Name == WhenStepNamed
}

type floorReplay struct {
	declared []string
	pods     map[string]*podReplayed
	order    []string
	stopped  map[string]bool
	ready    map[string]int
	whens    int
	watches  []*floorWatch
}

type podReplayed struct {
	named      string
	deployment string
	node       string
	phase      PodPhase
	removed    bool
}

type floorWatch struct {
	reading       FloorReading
	readyAtFirst  []string
	observed      bool
	count         int
	falling       bool
	fallingFrom   time.Duration
	holdingLowest bool
}

func newFloorReplay(judged []Deployment, declared []string) *floorReplay {
	replay := &floorReplay{
		declared: declared, pods: map[string]*podReplayed{}, stopped: map[string]bool{}, ready: map[string]int{},
	}
	for _, deployment := range judged {
		replay.watches = append(replay.watches, &floorWatch{reading: FloorReading{deployment: deployment}})
	}

	return replay
}

func (r *floorReplay) readPod(entry Entry) {
	pod, claimed := r.podOf(entry.Name)
	if !claimed {
		return
	}

	wasReady := r.counts(pod)
	pod.phase = PodPhase(entry.Reason)
	pod.removed = pod.removed || entry.Change == ChangeRemoved
	if entry.Detail != "" {
		pod.node = entry.Detail
	}
	if isReady := r.counts(pod); isReady != wasReady {
		r.ready[pod.deployment] += readyChange(isReady)
	}
}

func readyChange(nowReady bool) int {
	if nowReady {
		return 1
	}

	return -1
}

func (r *floorReplay) podOf(name string) (*podReplayed, bool) {
	if pod, seen := r.pods[name]; seen {
		return pod, true
	}

	deployment := deploymentOf(name, r.declared)
	if deployment == "" {
		return nil, false
	}

	pod := &podReplayed{named: name, deployment: deployment}
	r.pods[name] = pod
	r.order = append(r.order, name)

	return pod, true
}

func (r *floorReplay) counts(pod *podReplayed) bool {
	return pod.phase == PodRunning && !pod.removed && pod.node != "" && !r.stopped[pod.node]
}

// when keeps the Pods Ready just before the first Failure stops any Node, and
// counts every Pod again once the Nodes stopped grow.
func (r *floorReplay) when(stoppedPerWhen [][]string) {
	if r.whens == 0 {
		for _, watch := range r.watches {
			watch.readyAtFirst = r.readyOf(watch.reading.deployment.Name())
		}
	}
	if r.whens < len(stoppedPerWhen) {
		for _, node := range stoppedPerWhen[r.whens] {
			r.stopped[node] = true
		}
	}
	r.whens++

	clear(r.ready)
	for _, pod := range r.pods {
		if r.counts(pod) {
			r.ready[pod.deployment]++
		}
	}
}

func (r *floorReplay) readyOf(deployment string) []string {
	var ready []string
	for _, name := range r.order {
		if pod := r.pods[name]; pod.deployment == deployment && r.counts(pod) {
			ready = append(ready, name)
		}
	}

	return ready
}

// observe judges nothing before the first When's note, since the Waits before
// it already hold the Pods Ready.
func (r *floorReplay) observe(at time.Duration) {
	if r.whens == 0 {
		return
	}

	for _, watch := range r.watches {
		watch.observe(at, r.ready[watch.reading.deployment.Name()], r.lostFrom)
	}
}

func (r *floorReplay) lostFrom(readyAtFirst []string) []string {
	var lost []string
	for _, name := range readyAtFirst {
		if pod := r.pods[name]; !r.counts(pod) {
			lost = append(lost, pod.named+" on "+pod.node)
		}
	}

	return lost
}

func (r *floorReplay) readings() []FloorReading {
	readings := make([]FloorReading, 0, len(r.watches))
	for _, watch := range r.watches {
		reading := watch.reading
		reading.failures = r.whens
		readings = append(readings, reading)
	}

	return readings
}

// observe follows two readings of the same count: the fall below the Floor
// (trackFall) and the lowest count (trackLowest). The fall reported is the one
// that reached the lowest count, so trackLowest records a fall only while
// trackFall has one open.
func (w *floorWatch) observe(at time.Duration, count int, lostFrom func([]string) []string) {
	if w.observed && count == w.count {
		return
	}
	first := !w.observed
	w.observed, w.count = true, count

	w.trackFall(at, count)
	w.trackLowest(at, count, first, lostFrom)
}

// trackFall opens a fall when the count drops below the Floor and closes it
// when the count stands at the Floor again. Closing the fall that reached the
// lowest count ends how long the reported fall lasted; closing a shallower
// fall leaves the reported one untouched.
func (w *floorWatch) trackFall(at time.Duration, count int) {
	floor := w.reading.deployment.Floor()
	switch {
	case count < floor && !w.falling:
		w.falling, w.fallingFrom, w.holdingLowest = true, at, false
	case count >= floor && w.falling:
		w.falling = false
		if w.holdingLowest {
			w.reading.fallFor, w.holdingLowest = lasting{took: at - w.reading.fallAt, ended: true}, false
		}
	}
}

// trackLowest records when the lowest count was first reached, and ends how
// long it lasted at the first rise after it. A new lowest reached during a
// fall makes that fall the reported one, holding it until trackFall closes it.
func (w *floorWatch) trackLowest(at time.Duration, count int, first bool, lostFrom func([]string) []string) {
	switch {
	case first || count < w.reading.lowest:
		w.reading.lowest, w.reading.lowestAt, w.reading.lowestFor = count, at, lasting{}
		if w.falling {
			w.reading.fell, w.reading.fallAt, w.reading.fallFor = true, w.fallingFrom, lasting{}
			w.reading.lost, w.holdingLowest = lostFrom(w.readyAtFirst), true
		}
	case !w.reading.lowestFor.ended:
		w.reading.lowestFor = lasting{took: at - w.reading.lowestAt, ended: true}
	}
}

func readyPods(count int) string {
	if count == 1 {
		return "1 Ready Pod"
	}

	return fmt.Sprintf("%d Ready Pods", count)
}

func podsCounted(count int) string {
	if count == 1 {
		return "1 Pod"
	}

	return fmt.Sprintf("%d Pods", count)
}
