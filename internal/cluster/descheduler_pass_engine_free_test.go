//go:build !integration

package cluster

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	theStdoutStream = 1
	theStderrStream = 2
)

// framed writes a line as the engine frames a log read without a terminal.
func framed(stream byte, line string) []byte {
	return append(aFrameHeaderStating(stream, uint32(len(line))), line...)
}

func aFrameHeaderStating(stream byte, length uint32) []byte {
	header := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[logFrameLengthAt:], length)

	return header
}

func TestAPassesLog_ShouldBeReadAsItsLastLinesOfBothStreamsInTheOrderWritten(t *testing.T) {
	const (
		anEarlierLine  = "I1109 the first line, beyond the lines kept"
		aStdoutLine    = "I1109 a line written to stdout"
		aStderrLine    = "E1109 a line written to stderr"
		linesKept      = 2
		aCutFramesHalf = "E1109 a frame the read limit cut"
	)
	written := slices.Concat(
		framed(theStdoutStream, anEarlierLine+"\n"),
		framed(theStdoutStream, aStdoutLine+"\n"),
		framed(theStderrStream, aStderrLine+"\n"),
		framed(theStderrStream, aCutFramesHalf)[:logFrameHeaderLength+1],
	)

	lines, err := lastLinesOf(bytes.NewReader(written), linesKept)

	if err != nil {
		t.Fatalf("read the frames of a pass's log: %v", err)
	}
	expect.Expect(t, lines).To(expect.Equal([]string{aStdoutLine, aStderrLine}))
}

// largestReadRecorded keeps the largest buffer a reader handed it to fill,
// which is the most the reader allocated for what it read. It stands in for the
// engine's log stream, unlimited, since io.LimitReader shortens the buffers it
// passes on and would hide the allocation.
type largestReadRecorded struct {
	unread  *bytes.Reader
	largest int
}

func (r *largestReadRecorded) Read(buffer []byte) (int, error) {
	r.largest = max(r.largest, len(buffer))

	return r.unread.Read(buffer)
}

// readWithinTheLimit is what reading a log leaves: the lines kept, and whether
// no buffer handed to the stream exceeded the read limit.
type readWithinTheLimit struct {
	Lines              []string
	WithinTheReadLimit bool
}

// Unframed log text read as a header states a length of hundreds of megabytes,
// so it falls under the same rule as a frame stating one. The length stated is
// far beyond the limit and still harmless to allocate.
func TestAPassesLogWhoseFrameStatesALengthBeyondTheReadLimit_ShouldBeReadWithinTheReadLimit(t *testing.T) {
	const (
		aLineBeforeIt         = "E1109 a line before the frame stating too much"
		aLineAfterIt          = "E1109 a line after the frame stating too much"
		anyLinesKept          = 20
		aLengthBeyondTheLimit = 1024 * logBytesReadAtMost
	)
	stream := &largestReadRecorded{unread: bytes.NewReader(slices.Concat(
		framed(theStdoutStream, aLineBeforeIt+"\n"),
		aFrameHeaderStating(theStderrStream, aLengthBeyondTheLimit),
		framed(theStderrStream, aLineAfterIt+"\n"),
	))}

	lines, err := lastLinesOf(stream, anyLinesKept)

	if err != nil {
		t.Fatalf("read the frames of a pass's log: %v", err)
	}
	expect.Expect(t, readWithinTheLimit{Lines: lines, WithinTheReadLimit: stream.largest <= logBytesReadAtMost}).
		To(expect.Equal(readWithinTheLimit{Lines: []string{aLineBeforeIt}, WithinTheReadLimit: true}))
}

// No test here turns on a Pod's name or UID beyond telling the Pods apart.
const (
	aPodsUID       = types.UID("a-pod")
	anotherPodsUID = types.UID("another-pod")
)

func aPodOf(uid types.UID) corev1.Pod {
	return corev1.Pod{UID: uid}
}

func withDeletionMarked(pod corev1.Pod) corev1.Pod {
	marked := *pod.DeepCopy()
	marked.DeletionTimestamp = markedForDeletion()

	return marked
}

func withDisruptionTargetMarked(pod corev1.Pod) corev1.Pod {
	marked := *pod.DeepCopy()
	marked.Status.Conditions = []corev1.PodCondition{{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue}}

	return marked
}

func TestAPassAfterWhichEveryPodItCouldEvictIsListedUnmarkedOrAPodAlreadyLeavingStillLeaves_ShouldHaveEvictedNothing(t *testing.T) {
	passes := map[string]struct{ before, after []corev1.Pod }{
		"every Pod listed again unmarked, a new one beside them": {
			before: []corev1.Pod{aPodOf(aPodsUID)},
			after:  []corev1.Pod{aPodOf(aPodsUID), aPodOf(anotherPodsUID)},
		},
		"a Pod marked for deletion before the pass gone after it": {
			before: []corev1.Pod{aPodOf(aPodsUID), withDeletionMarked(aPodOf(anotherPodsUID))},
			after:  []corev1.Pod{aPodOf(aPodsUID)},
		},
	}

	for named, pass := range passes {
		t.Run(named, func(t *testing.T) {
			expect.Expect(t, anyEvicted(pass.before, pass.after)).To(expect.Equal(false))
		})
	}
}

func TestAPassAfterWhichAPodItCouldEvictIsGoneMarkedForDeletionOrADisruptionTarget_ShouldHaveEvictedAPod(t *testing.T) {
	before := []corev1.Pod{aPodOf(aPodsUID), aPodOf(anotherPodsUID)}
	afters := map[string][]corev1.Pod{
		"a Pod listed no more":      {aPodOf(aPodsUID)},
		"a Pod marked for deletion": {aPodOf(aPodsUID), withDeletionMarked(aPodOf(anotherPodsUID))},
		"a Pod a disruption target": {aPodOf(aPodsUID), withDisruptionTargetMarked(aPodOf(anotherPodsUID))},
	}

	for named, after := range afters {
		t.Run(named, func(t *testing.T) {
			expect.Expect(t, anyEvicted(before, after)).To(expect.Equal(true))
		})
	}
}
