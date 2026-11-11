//go:build !integration

package cluster

import (
	"fmt"
	"go/build"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// The two kinds of test are selected by these tags alone, the ones the
// Makefile's test targets pass to go test.
var (
	theRunOfTheTestsNeedingNoEngine = []string{"unit"}
	theRunOfTheTestsNeedingOne      = []string{"integration"}
	theRunOfEveryTest               = []string(nil)
)

// A test reaches the engine through the first and starts a Simulated Cluster
// through the second.
var theWaysToTheEngine = []string{"DetectEngine", "Provision"}

// A test that skips would pass a run where the engine it needs is missing.
var theWaysToSkip = []string{"Skip", "Skipf", "SkipNow"}

func TestTheTestsNeedingNoContainerRuntime_ShouldPassWithNoRuntimeReachableAndStartNoSimulatedCluster(t *testing.T) {
	_, needingNone := theLabsTestsByKind(t)

	leftOut := testsLeftOutOf(t, needingNone, theRunOfTheTestsNeedingNoEngine)

	if len(leftOut) > 0 {
		t.Errorf("every test needing no container engine runs when only those are run, but these do not:\n%s", strings.Join(leftOut, "\n"))
	}
}

func TestATestNeedingAContainerRuntime_ShouldNotRunWhenOnlyTheTestsNeedingNoneAreRun(t *testing.T) {
	needingOne, _ := theLabsTestsByKind(t)

	compiled := testsCompiledIn(t, needingOne, theRunOfTheTestsNeedingNoEngine)

	if len(compiled) > 0 {
		t.Errorf("no test reaching the container engine runs among the tests needing none, but these do:\n%s", strings.Join(compiled, "\n"))
	}
}

func TestTheTestsNeedingAContainerRuntime_ShouldRunAloneWhenOnlyTheyAreRun(t *testing.T) {
	needingOne, needingNone := theLabsTestsByKind(t)

	leftOut := testsLeftOutOf(t, needingOne, theRunOfTheTestsNeedingOne)
	compiledBeside := testsCompiledIn(t, needingNone, theRunOfTheTestsNeedingOne)

	if len(leftOut) > 0 {
		t.Errorf("every test reaching the container engine runs when only those are run, but these do not:\n%s", strings.Join(leftOut, "\n"))
	}
	if len(compiledBeside) > 0 {
		t.Errorf("no test needing no container engine runs among those needing one, but these do:\n%s", strings.Join(compiledBeside, "\n"))
	}
}

// A test reaching for an engine that does not answer fails through its
// package's engine helper, which names the engine; a skip would pass instead.
func TestATestNeedingAContainerRuntimeWithNoneReachable_ShouldFailNamingTheRuntime(t *testing.T) {
	skipping := testFilesUsing(t, theWaysToSkip)

	if len(skipping) > 0 {
		t.Errorf("a test missing what it needs fails rather than skips, but these skip:\n%s", strings.Join(skipping, "\n"))
	}
}

func TestEveryTestOfTheLab_ShouldRunWhenNoKindIsChosen(t *testing.T) {
	needingOne, needingNone := theLabsTestsByKind(t)

	leftOut := testsLeftOutOf(t, slices.Concat(needingOne, needingNone), theRunOfEveryTest)

	if len(leftOut) > 0 {
		t.Errorf("the run choosing no kind runs every test, but these do not run in it:\n%s", strings.Join(leftOut, "\n"))
	}
}

type labTest struct {
	file string
	name string
}

func (l labTest) String() string {
	return fmt.Sprintf("  %s: %s", l.file, l.name)
}

// A test needs the engine when it, or a helper of its package's test files it
// calls however indirectly, reaches one of the ways to the engine. Only the
// test files are followed: a command whose arguments stop it before the engine
// is detected stays a test needing none.
func theLabsTestsByKind(t *testing.T) (needingOne, needingNone []labTest) {
	t.Helper()

	for _, inPackage := range byPackage(theLabsTestFiles(t)) {
		reaching := declarationsReaching(inPackage, theWaysToTheEngine)
		for _, file := range inPackage {
			for _, declared := range file.Declarations {
				if !isATest(declared) {
					continue
				}
				test := labTest{file: file.Path, name: declared.Name}
				if reaching[declared.Name] {
					needingOne = append(needingOne, test)

					continue
				}
				needingNone = append(needingNone, test)
			}
		}
	}

	// A scan finding no test that reaches the engine would let every rule
	// above hold over nothing.
	if len(needingOne) == 0 {
		t.Fatalf("no test of the module reaches the container engine through %v, so the kinds cannot be told apart", theWaysToTheEngine)
	}

	return needingOne, needingNone
}

func theLabsTestFiles(t *testing.T) []modulerules.GoFile {
	t.Helper()

	root := moduleRoot(t)
	files, err := modulerules.ScanGoFiles(root, modulerules.EveryGoFile)
	if err != nil {
		t.Fatalf("scan the module's own Go files: %v", err)
	}

	return slices.DeleteFunc(files, func(file modulerules.GoFile) bool {
		return !strings.HasSuffix(file.Path, "_test.go") || isIgnoredByGoTest(t, root, file.Path)
	})
}

// go test ./... passes over a directory whose name opens with a dot or an
// underscore, or is testdata, so a file there belongs to no run of either kind.
func isIgnoredByGoTest(t *testing.T, root, file string) bool {
	t.Helper()

	relative, err := filepath.Rel(root, filepath.Dir(file))
	if err != nil {
		t.Fatalf("place %s under the module root %s: %v", file, root, err)
	}

	return slices.ContainsFunc(strings.Split(relative, string(filepath.Separator)), func(directory string) bool {
		return directory != "." && (strings.HasPrefix(directory, ".") || strings.HasPrefix(directory, "_") || directory == "testdata")
	})
}

// A package is its directory and its name, since a directory holds a package
// and its external test package side by side.
func byPackage(files []modulerules.GoFile) map[string][]modulerules.GoFile {
	grouped := make(map[string][]modulerules.GoFile)
	for _, file := range files {
		key := filepath.Dir(file.Path) + " " + file.Package
		grouped[key] = append(grouped[key], file)
	}

	return grouped
}

// Declarations are matched by name alone, so a name two declarations share
// reaches when either does: the error leans towards a test needing the engine,
// which then runs among the tests needing one rather than without an engine.
func declarationsReaching(files []modulerules.GoFile, targets []string) map[string]bool {
	uses := make(map[string][]string)
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				uses[declared.Name] = append(uses[declared.Name], used.Name)
			}
		}
	}

	reaching := make(map[string]bool)
	for grew := true; grew; {
		grew = false
		for name, used := range uses {
			if reaching[name] {
				continue
			}
			if slices.ContainsFunc(used, func(u string) bool { return slices.Contains(targets, u) || reaching[u] }) {
				reaching[name], grew = true, true
			}
		}
	}

	return reaching
}

func isATest(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && strings.HasPrefix(declared.Name, "Test") && declared.Name != "TestMain"
}

func testsLeftOutOf(t *testing.T, tests []labTest, tags []string) []string {
	t.Helper()

	return reported(slices.DeleteFunc(slices.Clone(tests), func(test labTest) bool {
		return compiledIn(t, test.file, tags)
	}))
}

func testsCompiledIn(t *testing.T, tests []labTest, tags []string) []string {
	t.Helper()

	return reported(slices.DeleteFunc(slices.Clone(tests), func(test labTest) bool {
		return !compiledIn(t, test.file, tags)
	}))
}

// The file's build constraint is evaluated as go test evaluates it under the
// tags rather than read as text, so whatever constraint a file carries is
// judged by what it selects.
func compiledIn(t *testing.T, file string, tags []string) bool {
	t.Helper()

	run := build.Default
	run.BuildTags = tags
	matched, err := run.MatchFile(filepath.Dir(file), filepath.Base(file))
	if err != nil {
		t.Fatalf("evaluate the build constraint of %s under the tags %v: %v", file, tags, err)
	}

	return matched
}

func testFilesUsing(t *testing.T, names []string) []string {
	t.Helper()

	var using []string
	for _, file := range theLabsTestFiles(t) {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				if slices.Contains(names, used.Name) {
					using = append(using, fmt.Sprintf("  %s: %s uses %s", file.Path, declared.Name, used.Name))
				}
			}
		}
	}

	return using
}

func reported(tests []labTest) []string {
	lines := make([]string, 0, len(tests))
	for _, test := range tests {
		lines = append(lines, test.String())
	}

	return lines
}
