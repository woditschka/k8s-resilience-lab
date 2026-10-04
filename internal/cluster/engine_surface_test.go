package cluster

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// admittedVersions are the shapes a run may name its Kubernetes version in, and
// refusedValues the shapes that are not a version at all. Both tables seed the
// fuzz corpus below, so a value either suite names is a value the fuzzer starts
// from.
var (
	admittedVersions = map[string]string{
		"with a leading v":   "v1.36.1",
		"without one":        "1.36.1",
		"of two components":  "1.36",
		"of one component":   "1",
		"of four components": "1.36.1.2",
	}

	refusedValues = map[string]string{
		"empty":                    "",
		"a bare v":                 "v",
		"another repository":       "evil.example.com/image:tag",
		"a tag separator":          "1.36.1:latest",
		"a digest":                 "1.36.1@sha256:0000",
		"a path separator":         "../1.36.1",
		"a shell metacharacter":    "1.36.1;id",
		"a space":                  "1.36.1 ",
		"a letter":                 "1.36.1-rc1",
		"an alternative image":     "latest",
		"an underscore":            "1_36_1",
		"a leading dot":            ".1.36.1",
		"two leading v characters": "vv1.36.1",
		"a doubled separator":      "1..36.1",
		"a trailing separator":     "1.36.1.",
	}
)

func TestAKubernetesVersion_ShouldBeAdmittedWithOrWithoutItsLeadingV(t *testing.T) {
	for shape, named := range admittedVersions {
		t.Run(shape, func(t *testing.T) {
			version, err := ParseKubernetesVersion(named)
			if err != nil {
				t.Fatalf("a version named %q is admitted: %v", named, err)
			}

			expect.Expect(t, version.String()[:1]).To(expect.Equal("v"))
		})
	}
}

func TestAValueThatIsNotAVersion_ShouldBeRefusedBeforeAnyRequest(t *testing.T) {
	for shape, named := range refusedValues {
		t.Run(shape, func(t *testing.T) {
			if _, err := ParseKubernetesVersion(named); err == nil {
				t.Fatalf("a value of %q is refused, but it was admitted as a version", named)
			}
		})
	}
}

// FuzzAnAdmittedVersion_ShouldReachTheImageTagAsDigitsAndDotsAlone is the
// adversarial half of the two tables above: the tables name the values a reader
// should see refused, and the fuzzer searches for a value neither table
// anticipated that the version pattern admits anyway.
//
// The property is the one the version type exists for: whatever a run names, the
// image a Simulated Cluster pulls is the pinned repository's KWOK image, and the
// only part of its tag that comes from the run is digits and dots
// (docs/adr/2026-10-03-kubernetes-version-pin.md). Asserting the composed
// reference rather than the pattern keeps the test independent of how the
// pattern is written.
func FuzzAnAdmittedVersion_ShouldReachTheImageTagAsDigitsAndDotsAlone(f *testing.F) {
	for _, named := range admittedVersions {
		f.Add(named)
	}
	for _, named := range refusedValues {
		f.Add(named)
	}

	labComposed := fmt.Sprintf("%s:%s-k8s.v", clusterImageRepository, kwokRelease)

	f.Fuzz(func(t *testing.T, named string) {
		version, err := ParseKubernetesVersion(named)
		if err != nil {
			return
		}

		fromTheRun, composed := strings.CutPrefix(clusterImage(version), labComposed)
		if !composed {
			t.Fatalf("a version named %q is admitted, but it composes the image %q rather than one of %q",
				named, clusterImage(version), labComposed)
		}
		if strings.ContainsFunc(fromTheRun, isNeitherDigitNorDot) {
			t.Errorf("a version named %q is admitted, and it carries %q into the image tag",
				named, fromTheRun)
		}
		if reparsed, err := ParseKubernetesVersion(version.String()); err != nil || !reparsed.Equal(version) {
			t.Errorf("a version named %q normalises to %q, which is admitted again unchanged, but: %v",
				named, version, err)
		}
	})
}

// isNeitherDigitNorDot reports whether a rune is something other than the digits
// and dots a version is made of.
func isNeitherDigitNorDot(character rune) bool {
	return !unicode.IsDigit(character) && character != '.'
}

// admittedParameters are the only types an exported method of the engine surface
// may take. An image reference or a container identifier would widen the surface
// from "the cluster this run created" to "anything on the machine", which the
// engine socket cannot scope down for us
// (docs/system-design.md § Threat Model, the engine-socket row).
var admittedParameters = []string{"context.Context", "KubernetesVersion"}

// restrictedSurface are the types whose exported methods carry that restriction.
var restrictedSurface = []string{"Engine", "ClusterContainer"}

func TestNoExportedEngineMethod_ShouldTakeAnImageOrAContainerIdentifier(t *testing.T) {
	var offenders []string

	for _, method := range exportedMethodsOf(t, restrictedSurface) {
		for _, parameter := range method.parameters {
			if !slices.Contains(admittedParameters, parameter) {
				offenders = append(offenders, fmt.Sprintf("  %s.%s takes a %s", method.receiver, method.name, parameter))
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("an exported method of the engine surface takes a %s alone, but:\n%s",
			strings.Join(admittedParameters, " or a "), strings.Join(offenders, "\n"))
	}
}

// listingEndpoints are the engine API paths that answer about containers or
// images the lab did not create. The package names none of them, so it cannot
// reach another container even by mistake.
var listingEndpoints = []string{"/containers/json", "/images/json", "/containers/prune", "/images/prune"}

func TestTheEngineSurface_ShouldNameNoEndpointThatListsOrPrunesTheMachine(t *testing.T) {
	var offenders []string

	for _, literal := range stringLiteralsOf(t) {
		for _, listing := range listingEndpoints {
			if strings.Contains(literal.value, listing) {
				offenders = append(offenders, fmt.Sprintf("  %s names %q", literal.position, literal.value))
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("the package names no engine endpoint that lists or prunes the machine, but:\n%s", strings.Join(offenders, "\n"))
	}
}

// exportedMethod is one exported method of the engine surface, as the package's
// own source declares it.
type exportedMethod struct {
	receiver   string
	name       string
	parameters []string
}

// exportedMethodsOf is every exported method declared on one of receivers, read
// from the package's own source rather than from reflection, so the assertion
// covers the declaration a reviewer reads.
func exportedMethodsOf(t *testing.T, receivers []string) []exportedMethod {
	t.Helper()

	var methods []exportedMethod
	for _, parsed := range packageFiles(t) {
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Recv == nil || !function.Name.IsExported() {
				continue
			}

			receiver := typeNameOf(function.Recv.List[0].Type)
			if !slices.Contains(receivers, receiver) {
				continue
			}
			methods = append(methods, exportedMethod{
				receiver:   receiver,
				name:       function.Name.Name,
				parameters: parameterTypesOf(function),
			})
		}
	}

	return methods
}

// parameterTypesOf is the type of every parameter of function, as the source
// spells it.
func parameterTypesOf(function *ast.FuncDecl) []string {
	var types []string
	for _, parameter := range function.Type.Params.List {
		named := max(len(parameter.Names), 1)
		for range named {
			types = append(types, typeNameOf(parameter.Type))
		}
	}

	return types
}

// typeNameOf is the type an expression names, with the pointer, slice and
// variadic markers stripped so that *ClusterContainer reads as ClusterContainer.
func typeNameOf(expression ast.Expr) string {
	switch named := expression.(type) {
	case *ast.StarExpr:
		return typeNameOf(named.X)
	case *ast.Ellipsis:
		return typeNameOf(named.Elt)
	case *ast.ArrayType:
		return typeNameOf(named.Elt)
	case *ast.SelectorExpr:
		return typeNameOf(named.X) + "." + named.Sel.Name
	case *ast.Ident:
		return named.Name
	default:
		return fmt.Sprintf("%T", expression)
	}
}

// sourceLiteral is one string literal of the package, with where it is written.
type sourceLiteral struct {
	position string
	value    string
}

// stringLiteralsOf is every string literal in the package's own source files.
func stringLiteralsOf(t *testing.T) []sourceLiteral {
	t.Helper()

	files := token.NewFileSet()

	var literals []sourceLiteral
	for _, parsed := range parsedWith(t, files) {
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, isLiteral := node.(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				return true
			}

			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			literals = append(literals, sourceLiteral{
				position: files.Position(literal.Pos()).String(),
				value:    value,
			})

			return true
		})
	}

	return literals
}

// packageFiles are this package's own parsed source files.
func packageFiles(t *testing.T) []*ast.File {
	t.Helper()

	return parsedWith(t, token.NewFileSet())
}

// parsedWith parses this package's own production source files into files. Test
// files are left out: the rules asserted here govern what the lab's own shipped
// code declares, and a test naming a forbidden path is how the test states it.
func parsedWith(t *testing.T, files *token.FileSet) []*ast.File {
	t.Helper()

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list the package's own source files: %v", err)
	}

	parsed := make([]*ast.File, 0, len(sources))
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(files, source, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", source, err)
		}
		parsed = append(parsed, file)
	}

	return parsed
}

// The engine socket grants control of every container on the machine, so the
// property that matters is observed against the machine as it is: every container
// that was already there is left exactly as it was, and the only container the
// run adds and removes is its own.
//
// What is asserted is that no container the lab did not create goes away while the
// run goes on, not the whole inventory and not each container's state: another
// invocation of the lab, or any other work on the machine, may create, start, stop
// and — for a Simulated Cluster of its own — remove containers while this run goes
// on, and that is the concurrent-invocation case the lab supports rather than a
// breach (docs/adr/2026-10-04-cluster-per-scenario.md). assertUntouched below
// states where that tolerance stops.
// That the lab *cannot* reach a container it did not create is asserted from the
// package's own declarations by the two tests above; this one is the observable
// outcome of a run on a real machine.
//
// The inventory is read and never written. The only create, start and remove here
// go through the lab's own code, and only for the cluster it created
// (docs/system-design.md § Threat Model, the engine-socket row).
func TestARun_ShouldTouchNoContainerBesidesItsOwnSimulatedCluster(t *testing.T) {
	engine := requireEngine(t)
	standingBy := containersOnTheEngine(t, engine)

	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), evenSpreadScenario)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster beside whatever else runs here: %v", err)
	}
	own := provisioned.Name()

	duringTheRun := containersOnTheEngine(t, engine)
	if _, created := duringTheRun[own]; !created {
		t.Errorf("the run creates its own Simulated Cluster %s, but the engine does not hold it", own)
	}
	assertUntouched(t, standingBy, duringTheRun)

	if err := provisioned.Close(t.Context()); err != nil {
		t.Fatalf("remove the Simulated Cluster %s: %v", own, err)
	}

	afterTheRun := containersOnTheEngine(t, engine)
	if _, left := afterTheRun[own]; left {
		t.Errorf("the run removes its own Simulated Cluster %s, but the engine still holds it", own)
	}
	assertUntouched(t, standingBy, afterTheRun)
}

// assertUntouched reports any container that was on the engine before the run and
// has since gone away, other than a Simulated Cluster of the lab's own.
//
// A lab-named container standing by is another invocation's Simulated Cluster, and
// that invocation removes it when its own Scenario ends — inside this run's window
// when the two overlap, which is the supported concurrent-invocation case and not
// this run's doing. Tolerating it here is what keeps the assertion as wide as its
// subject: every container the lab did not create, and so every container whose
// removal would be a breach. That an invocation removes only the Simulated Cluster
// it created is asserted where both identities are known, by
// TestConcurrentRuns_ShouldEachRemoveOnlyTheClusterTheyCreated.
func assertUntouched(t *testing.T, before, now map[string]string) {
	t.Helper()

	for name := range before {
		if _, present := now[name]; present {
			continue
		}
		if strings.HasPrefix(name, containerNamePrefix) {
			continue
		}
		t.Errorf("the run removes no container but its own, yet %s is no longer on the engine", name)
	}
}

// containersOnTheEngine is the state of every container the engine holds, running
// or not, by name. It is a read of the machine's own state, which is why it lives
// in a test: the lab itself declares no call that could list or reach a container
// it did not create.
func containersOnTheEngine(t *testing.T, endpoint Endpoint) map[string]string {
	t.Helper()

	var listed []struct {
		Names []string
		State string
	}
	if err := engineRequest(t.Context(), endpoint, http.MethodGet, "/containers/json?all=true", nil, &listed); err != nil {
		t.Fatalf("read which containers the engine holds: %v", err)
	}

	// The engine reports a container's name with a leading slash; the lab names
	// its own container without one.
	held := make(map[string]string, len(listed))
	for _, container := range listed {
		for _, name := range container.Names {
			held[strings.TrimPrefix(name, "/")] = container.State
		}
	}

	return held
}
