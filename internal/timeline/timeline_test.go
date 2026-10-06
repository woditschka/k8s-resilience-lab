package timeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The Scenario the Timelines below record, the Simulated Clusters they ran on,
// and the Kubernetes version those clusters mirror. Two Simulated Clusters of
// the same Scenario are what two invocations of the lab produce, and two
// Scenarios of one run are what the lab's own list holds.
const (
	theScenarioName        = "resilient-placement"
	itsCluster             = "k8s-resilience-lab-resilient-placement-7f3a9c"
	itsClusterInAnotherRun = "k8s-resilience-lab-resilient-placement-b1d20e"
	anotherScenarioName    = "a-zone-failure"
	itsOwnCluster          = "k8s-resilience-lab-a-zone-failure-4c81fa"
	someKubernetesVersion  = "v1.36.1"
)

// The two changes the Timelines below hold: a Node the Topology added, and the
// Placement the control plane reported for a Pod.
const (
	whenTheNodeWasAdded    = 120 * time.Millisecond
	theNodeAddedAsFiled    = "120ms"
	aNodeName              = "zone-1-node-1"
	theReadyCondition      = "Ready"
	theNodeCapacity        = "10 Pods"
	whenThePodWasScheduled = 3470 * time.Millisecond
	thePodScheduledAsFiled = "3.47s"
	aPodName               = "web-5d8f9b7c6-2xk4m"
	theScheduledReason     = "Scheduled"
	theScheduledMessage    = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-1-node-1"
)

// aHeader is the first line of the Timeline of the Scenario above, and
// anotherScenariosHeader of a second Scenario of the same run.
func aHeader() Header {
	return Header{Scenario: theScenarioName, Cluster: itsCluster, Kubernetes: someKubernetesVersion}
}

func anotherScenariosHeader() Header {
	return Header{Scenario: anotherScenarioName, Cluster: itsOwnCluster, Kubernetes: someKubernetesVersion}
}

// aNodeAdded and aPodReported are the entries the tests below file.
func aNodeAdded() scenario.Entry {
	return scenario.Entry{
		Elapsed: whenTheNodeWasAdded, Change: scenario.ChangeAdded, Kind: scenario.NodeObject,
		Name: aNodeName, Reason: theReadyCondition, Detail: theNodeCapacity,
	}
}

func aPodReported() scenario.Entry {
	return scenario.Entry{
		Elapsed: whenThePodWasScheduled, Change: scenario.ChangeReported, Kind: scenario.PodObject,
		Name: aPodName, Reason: theScheduledReason, Detail: theScheduledMessage,
	}
}

func TestAFiledTimeline_ShouldHoldItsHeaderAndOneJSONObjectPerEntry(t *testing.T) {
	folder := NewFolder(t.TempDir())

	filed := folder.Open(aHeader())
	filed.Write(aNodeAdded())
	filed.Write(aPodReported())
	filed.Close()

	expectFiled(t, filed)
	expect.Expect(t, linesOf(t, filed.Path())).To(expect.Equal([]string{
		`{"record":"timeline","scenario":"` + theScenarioName + `","cluster":"` + itsCluster +
			`","kubernetes":"` + someKubernetesVersion + `"}`,
		`{"record":"entry","elapsed":"` + theNodeAddedAsFiled + `","change":"added","kind":"Node","name":"` +
			aNodeName + `","reason":"` + theReadyCondition + `","detail":"` + theNodeCapacity + `"}`,
		`{"record":"entry","elapsed":"` + thePodScheduledAsFiled + `","change":"reported","kind":"Pod","name":"` +
			aPodName + `","reason":"` + theScheduledReason + `","detail":"` + theScheduledMessage + `"}`,
	}))
}

func TestAMissingTimelineFolder_ShouldBeCreatedRatherThanFailTheRun(t *testing.T) {
	root := t.TempDir()
	folder := NewFolder(root)

	filed := folder.Open(aHeader())
	filed.Write(aNodeAdded())
	filed.Close()

	expectFiled(t, filed)
	expect.Expect(t, folder.Path()).To(expect.Equal(filepath.Join(root, folderName)))
	expect.Expect(t, filesIn(t, folder.Path())).To(expect.Equal([]string{itsCluster + fileSuffix}))
}

func TestARunOfTwoScenarios_ShouldLeaveOneTimelineFilePerScenario(t *testing.T) {
	folder := NewFolder(t.TempDir())

	oneScenario, theOther := folder.Open(aHeader()), folder.Open(anotherScenariosHeader())
	oneScenario.Write(aNodeAdded())
	theOther.Write(aPodReported())
	oneScenario.Close()
	theOther.Close()

	expectFiled(t, oneScenario)
	expectFiled(t, theOther)
	expect.Expect(t, filesIn(t, folder.Path())).To(expect.Equal([]string{
		itsOwnCluster + fileSuffix, itsCluster + fileSuffix,
	}))
	expect.Expect(t, entriesFiledIn(t, oneScenario.Path())).To(expect.Equal([]scenario.Entry{aNodeAdded()}))
	expect.Expect(t, entriesFiledIn(t, theOther.Path())).To(expect.Equal([]scenario.Entry{aPodReported()}))
}

func TestTwoRunsAtOnce_ShouldLeaveEachRunsTimelinesInFilesOfItsOwn(t *testing.T) {
	folder := NewFolder(t.TempDir())
	anotherRunOfTheSameScenario := Header{
		Scenario: theScenarioName, Cluster: itsClusterInAnotherRun, Kubernetes: someKubernetesVersion,
	}

	oneRun, theOtherRun := folder.Open(aHeader()), folder.Open(anotherRunOfTheSameScenario)
	oneRun.Write(aNodeAdded())
	theOtherRun.Write(aPodReported())
	oneRun.Close()
	theOtherRun.Close()

	expectFiled(t, oneRun)
	expectFiled(t, theOtherRun)
	expect.Expect(t, entriesFiledIn(t, oneRun.Path())).To(expect.Equal([]scenario.Entry{aNodeAdded()}))
	expect.Expect(t, entriesFiledIn(t, theOtherRun.Path())).To(expect.Equal([]scenario.Entry{aPodReported()}))
}

func TestAFolderTheLabCannotWrite_ShouldLeaveItsPathAndItsReasonAndFileNoEntry(t *testing.T) {
	root := aRootTheLabCannotWrite(t)
	folder := NewFolder(root)

	filed := folder.Open(aHeader())
	filed.Write(aNodeAdded())
	filed.Close()

	expect.Expect(t, filed.Path()).To(expect.Equal(filepath.Join(folder.Path(), itsCluster+fileSuffix)))
	expectNamedInTheFilingError(t, filed, folder.Path())
	expect.Expect(t, exists(folder.Path())).To(expect.Equal(false))
}

func TestAClusterNameThatIsNoPathElement_ShouldBeRefusedRatherThanFiledElsewhere(t *testing.T) {
	names := map[string]string{
		"a name holding a path separator":   "resilient-placement/../../etc/passwd",
		"a name that is a parent reference": "..",
		"no name at all":                    "",
	}

	for shape, cluster := range names {
		t.Run(shape, func(t *testing.T) {
			folder := NewFolder(t.TempDir())

			filed := folder.Open(Header{
				Scenario: theScenarioName, Cluster: cluster, Kubernetes: someKubernetesVersion,
			})
			filed.Write(aNodeAdded())
			filed.Close()

			expectNamedInTheFilingError(t, filed, cluster)
			expect.Expect(t, filed.Path()).To(expect.Equal(""))
			expect.Expect(t, exists(folder.Path())).To(expect.Equal(false))
		})
	}
}

// aRootTheLabCannotWrite is a working directory the invoking user may read and
// not write, which is what a Timeline folder that cannot be created comes down
// to. It is made writable again when the test ends, so the framework can remove
// it.
func aRootTheLabCannotWrite(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	const readableNotWritable = 0o500
	if err := os.Chmod(root, readableNotWritable); err != nil {
		t.Fatalf("make the working directory %s unwritable: %v", root, err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Errorf("make the working directory %s writable again: %v", root, err)
		}
	})

	return root
}

// exists reports whether the path is there, which is how a folder the lab did
// not create is read.
func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// expectNamedInTheFilingError asserts the Timeline reports what prevented it
// from being filed, naming what a reader needs to find it.
func expectNamedInTheFilingError(t *testing.T, filed *File, named string) {
	t.Helper()

	err := filed.Err()
	if err == nil {
		t.Fatalf("a Timeline that could not be written reports the reason, but %s reported none", filed.Path())
	}
	if !strings.Contains(err.Error(), named) {
		t.Errorf("the reason names %q, but reads: %v", named, err)
	}
}

// expectFiled reports a Timeline the lab could not write, naming the error it
// kept, because every test above files one that should have been written.
func expectFiled(t *testing.T, filed *File) {
	t.Helper()

	if err := filed.Err(); err != nil {
		t.Fatalf("file the Timeline at %s: %v", filed.Path(), err)
	}
}

// linesOf are the lines of a filed Timeline, which is what the published format
// is read by.
func linesOf(t *testing.T, path string) []string {
	t.Helper()

	filed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the filed Timeline %s: %v", path, err)
	}

	return strings.Split(strings.TrimSuffix(string(filed), "\n"), "\n")
}

// filesIn are the names of the files the Timeline folder holds, in name order.
func filesIn(t *testing.T, folder string) []string {
	t.Helper()

	listed, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("read the Timeline folder %s: %v", folder, err)
	}

	names := make([]string, 0, len(listed))
	for _, file := range listed {
		names = append(names, file.Name())
	}

	return names
}

// filedEntry is one entry line as a reader of the published format sees it, so
// these tests read a filed Timeline back the way the later query path will
// rather than through the writer's own type.
type filedEntry struct {
	Record  string `json:"record"`
	Elapsed string `json:"elapsed"`
	Change  string `json:"change"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail"`
}

// entriesFiledIn are the entries a filed Timeline holds, read back from its
// lines with the standard library alone.
func entriesFiledIn(t *testing.T, path string) []scenario.Entry {
	t.Helper()

	lines := linesOf(t, path)
	read := make([]scenario.Entry, 0, len(lines))
	for _, line := range lines[1:] {
		var filed filedEntry
		if err := json.Unmarshal([]byte(line), &filed); err != nil {
			t.Fatalf("read the entry line %s of %s: %v", line, path, err)
		}

		elapsed, err := time.ParseDuration(filed.Elapsed)
		if err != nil {
			t.Fatalf("read the elapsed time %q of %s: %v", filed.Elapsed, path, err)
		}
		read = append(read, scenario.Entry{
			Elapsed: elapsed, Change: scenario.Change(filed.Change), Kind: scenario.ObjectKind(filed.Kind),
			Name: filed.Name, Reason: filed.Reason, Detail: filed.Detail,
		})
	}

	return read
}
