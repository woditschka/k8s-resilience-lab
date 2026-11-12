//go:build !integration

package cluster

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	theScanTool       = "golang.org/x/vuln/cmd/govulncheck"
	theScanToolModule = "golang.org/x/vuln"
	theScanTarget     = "vuln-check"
	theToolsFile      = "tools.mod"
	theLabsModuleFile = "go.mod"
	theWorkflowFile   = ".github/workflows/checks.yml"
	theOneCommand     = "ci"
	theScanRecipe     = "go tool -modfile=tools.mod govulncheck"
	someFixedVersion  = `^v\d+\.\d+\.\d+$`
	// The workflow's bare `on:` key reads as the boolean true in YAML 1.1.
	theWorkflowTriggersKey = "true"
)

type moduleFile struct {
	Tool    []struct{ Path string }
	Require []struct{ Path, Version string }
}

type workflow struct {
	Triggers map[string]json.RawMessage `json:"true"`
	Jobs     map[string]struct {
		Steps []struct{ Run string }
	}
}

func TestTheVulnerabilityScanTool_ShouldBePinnedToOneVersionApartFromTheLabsModule(t *testing.T) {
	root := moduleRoot(t)

	tools := readModuleFile(t, root, theToolsFile)
	labs := readModuleFile(t, root, theLabsModuleFile)

	expect.Expect(t, toolPaths(tools)).To(expect.Equal([]string{theScanTool}))
	expect.Expect(t, versionsRequiring(tools, theScanToolModule)).To(expect.HaveLen[string](1))
	expect.Expect(t, regexp.MustCompile(someFixedVersion).MatchString(versionsRequiring(tools, theScanToolModule)[0])).To(expect.Equal(true))
	expect.Expect(t, toolPaths(labs)).To(expect.Equal([]string(nil)))
	expect.Expect(t, versionsRequiring(labs, theScanToolModule)).To(expect.Equal([]string(nil)))
}

func TestTheOneCommand_ShouldNotRunTheVulnerabilityScan(t *testing.T) {
	root := moduleRoot(t)

	theOneCommandsRun := dryRun(t, root, theOneCommand)
	theScanTargetsRun := dryRun(t, root, theScanTarget)

	expect.Expect(t, strings.Contains(theOneCommandsRun, "govulncheck")).To(expect.Equal(false))
	expect.Expect(t, strings.Contains(theOneCommandsRun, theScanTarget)).To(expect.Equal(false))
	expect.Expect(t, strings.Contains(theScanTargetsRun, theScanRecipe)).To(expect.Equal(true))
}

func TestTheBuild_ShouldScanForKnownVulnerabilitiesOnEveryPushToMainEveryPullRequestAndWeekly(t *testing.T) {
	root := moduleRoot(t)

	declared := readWorkflow(t, root)

	expect.Expect(t, slices.Sorted(maps.Keys(declared.Triggers))).To(expect.Equal([]string{"pull_request", "push", "schedule"}))
	expect.Expect(t, pushedBranches(t, declared)).To(expect.Equal([]string{"main"}))
	expect.Expect(t, jobsRunning(declared, "make "+theScanTarget)).To(expect.Equal([]string{theScanTarget}))
}

func readModuleFile(t *testing.T, root, name string) moduleFile {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "mod", "edit", "-json", name)
	command.Dir = root
	command.Stderr = os.Stderr
	out, err := command.Output()
	if err != nil {
		t.Fatalf("read %s as JSON: %v", name, err)
	}

	var read moduleFile
	if err := json.Unmarshal(out, &read); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}

	return read
}

func toolPaths(file moduleFile) []string {
	var paths []string
	for _, tool := range file.Tool {
		paths = append(paths, tool.Path)
	}

	return paths
}

func versionsRequiring(file moduleFile, module string) []string {
	var versions []string
	for _, required := range file.Require {
		if required.Path == module {
			versions = append(versions, required.Version)
		}
	}

	return versions
}

func dryRun(t *testing.T, root, target string) string {
	t.Helper()

	command := exec.CommandContext(t.Context(), "make", "-n", target)
	command.Dir = root
	command.Stderr = os.Stderr
	out, err := command.Output()
	if err != nil {
		t.Fatalf("print what make %s would run: %v", target, err)
	}

	return string(out)
}

func readWorkflow(t *testing.T, root string) workflow {
	t.Helper()

	file, err := os.Open(filepath.Join(root, theWorkflowFile))
	if err != nil {
		t.Fatalf("open the workflow: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	var read workflow
	if err := yaml.NewYAMLOrJSONDecoder(file, 4096).Decode(&read); err != nil {
		t.Fatalf("decode the workflow: %v", err)
	}
	if read.Triggers == nil {
		t.Fatalf("the workflow declares no triggers under the key %q", theWorkflowTriggersKey)
	}

	return read
}

func pushedBranches(t *testing.T, declared workflow) []string {
	t.Helper()

	var push struct{ Branches []string }
	if err := json.Unmarshal(declared.Triggers["push"], &push); err != nil {
		t.Fatalf("decode the push trigger: %v", err)
	}

	return push.Branches
}

func jobsRunning(declared workflow, command string) []string {
	var names []string
	for name, job := range declared.Jobs {
		for _, step := range job.Steps {
			if strings.TrimSpace(step.Run) == command {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)

	return names
}
