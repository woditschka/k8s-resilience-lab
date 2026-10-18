package modulerules_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

const (
	// everyPackageInTheModule is the pattern a module-wide rule is asserted over.
	everyPackageInTheModule = "./..."

	// probedModulePath is the module path of the module each test below builds.
	probedModulePath = "probed"

	// importedPath is the path the built module imports, and unimportedPath a path
	// no file in it names.
	importedPath   = "encoding/json"
	unimportedPath = "k8s.io/client-go/rest"

	// scannedCall is the function the call-rule scan looks for, and callAlias the
	// name the calling file imports its package under, so the scan has to resolve
	// the alias rather than match the text of the call.
	scannedCall = "InClusterConfig"
	callAlias   = "rename"

	// callAtLine and callAtColumn are where the call sits in callingSource.
	callAtLine   = 5
	callAtColumn = 18

	// declaredType is the named type the scanned sources write — on the one constant
	// of scannedSource that names a type, which is what a rule over a declaration of
	// a named type reads, and on the parameter chainedSource chains its call off.
	declaredType = "Name"
)

// importingSource imports a denied path; callingSource calls a denied function
// through a renamed import, and cleanSource names neither.
const (
	importingSource = `package probed

import "encoding/json"

var Decoder = json.NewDecoder
`

	callingSource = `package probed

import rename "k8s.io/client-go/rest"

var Reached, _ = rename.InClusterConfig()
`

	cleanSource = `package probed

var Nothing = "nothing denied here"
`

	importingTestSource = `package probed

import "encoding/json"

var TestDecoder = json.NewDecoder
`

	// scannedSource holds one of each top-level declaration, so a scan of it
	// reports every kind, the type a declaration names where it names one, the
	// parameters a function takes and the lines of its body.
	scannedSource = `package probed

import "encoding/json"

type Decoded struct{}

type Name string

const limit = 1

const Labelled Name = "labelled"

var Decoder = json.NewDecoder

func Scan(into *Decoded, from string) {
	Decoder(nil)
	_ = from
}
`

	// chainedSource holds a method chained onto a call, the shape a Scenario writes
	// its steps in, so a scan of it reports the expression each name is selected
	// from where that expression is a call rather than a plain name.
	chainedSource = `package probed

func Chain(step Name) {
	step.Given().Workload(limit)
}
`

	// literalSource passes a function literal to a call, the shape a Scenario
	// writes full access in, so a scan of it reports the names that literal uses
	// and not the parameter it declares for itself.
	literalSource = `package probed

func Reach(step Name) {
	step.Customize(func(settings *Decoded) {
		settings.Field = limit
	})
}
`

	// literalResultSource passes a function literal that names its own result as
	// well, which is the other list a function type declares names in. No Scenario
	// writes one today; the rules this package serves read every function literal of
	// the surface they scan, so both lists are read the same way. The body writes
	// that result nowhere, so the list the literal declares it in is the one place
	// the scan can read it from.
	literalResultSource = `package probed

func Report(step Name) {
	step.Customize(func(settings *Decoded) (reported error) {
		settings.Field = limit

		return
	})
}
`
)

// theLiteralParameter is the name those function literals declare for their
// parameter, which the declarations holding them write nowhere themselves.
const theLiteralParameter = "settings"

func TestAPackageImportingADeniedPath_ShouldBeNamedWithThePathItImports(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"decode.go": importingSource})

	offenders, err := modulerules.OffendingImporters(t.Context(), probed, everyPackageInTheModule, modulerules.EveryPackage, []string{importedPath})
	if err != nil {
		t.Fatalf("walk the package graph of %s: %v", probed, err)
	}

	expect.Expect(t, offenders).To(expect.Equal([]string{
		fmt.Sprintf("  %s imports %s", probedModulePath, importedPath),
	}))
}

func TestAPathNoPackageOfTheModuleImports_ShouldBeReportedNowhere(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"decode.go": importingSource})

	offenders, err := modulerules.OffendingImporters(t.Context(), probed, everyPackageInTheModule, modulerules.EveryPackage, []string{unimportedPath})
	if err != nil {
		t.Fatalf("walk the package graph of %s: %v", probed, err)
	}

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

func TestAPathOnlyATestFileImports_ShouldBeReportedAtTheWiderScopeAlone(t *testing.T) {
	reportsPerScope := map[string]struct {
		scope    modulerules.Scope
		reported []string
	}{
		"every package of the module": {
			scope: modulerules.EveryPackage,
			reported: []string{
				fmt.Sprintf("  %s [%s.test] imports %s", probedModulePath, probedModulePath, importedPath),
			},
		},
		"the production packages alone": {
			scope:    modulerules.ProductionPackagesOnly,
			reported: nil,
		},
	}

	for scope, expected := range reportsPerScope {
		t.Run(scope, func(t *testing.T) {
			probed := aModuleWhoseFilesAre(t, map[string]string{
				"clean.go":       cleanSource,
				"decode_test.go": importingTestSource,
			})

			offenders, err := modulerules.OffendingImporters(t.Context(), probed, everyPackageInTheModule, expected.scope, []string{importedPath})
			if err != nil {
				t.Fatalf("walk the package graph of %s: %v", probed, err)
			}

			expect.Expect(t, offenders).To(expect.Equal(expected.reported))
		})
	}
}

func TestACallThroughARenamedImport_ShouldBeNamedWithItsPosition(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{
		"reach.go": callingSource,
		"clean.go": cleanSource,
	})

	offenders, err := modulerules.CallsTo(probed, unimportedPath, scannedCall)
	if err != nil {
		t.Fatalf("scan the files under %s: %v", probed, err)
	}

	expect.Expect(t, offenders).To(expect.Equal([]string{
		fmt.Sprintf("  %s:%d:%d calls %s.%s",
			filepath.Join(probed, "reach.go"), callAtLine, callAtColumn, callAlias, scannedCall),
	}))
}

func TestAFileCallingNothingDenied_ShouldBeReportedNowhere(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"clean.go": cleanSource})

	offenders, err := modulerules.CallsTo(probed, unimportedPath, scannedCall)
	if err != nil {
		t.Fatalf("scan the files under %s: %v", probed, err)
	}

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

func TestAScannedGoFile_ShouldReportItsImportsAndEveryDeclarationItHolds(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"scan.go": scannedSource})

	scanned, err := modulerules.ScanGoFiles(probed, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("scan the Go files under %s: %v", probed, err)
	}

	expect.Expect(t, scanned).To(expect.Equal([]modulerules.GoFile{{
		Path:    filepath.Join(probed, "scan.go"),
		Package: probedModulePath,
		Imports: []string{importedPath},
		Declarations: []modulerules.Declaration{
			{Kind: modulerules.TypeDeclaration, Name: "Decoded", Exported: true},
			{
				Kind:     modulerules.TypeDeclaration,
				Name:     "Name",
				Exported: true,
				Names:    []modulerules.UsedName{{Name: "string"}},
			},
			{Kind: modulerules.ConstDeclaration, Name: "limit"},
			{
				Kind:     modulerules.ConstDeclaration,
				Name:     "Labelled",
				Exported: true,
				Type:     declaredType,
				Names:    []modulerules.UsedName{{Name: declaredType}},
			},
			{
				Kind:     modulerules.VarDeclaration,
				Name:     "Decoder",
				Exported: true,
				Names:    []modulerules.UsedName{{Qualifier: "json", Name: "NewDecoder"}},
			},
			{
				Kind:       modulerules.FuncDeclaration,
				Name:       "Scan",
				Exported:   true,
				Parameters: []modulerules.Parameter{{Name: "into", Type: "*Decoded"}, {Name: "from", Type: "string"}},
				Statements: []modulerules.Statement{
					{
						Written:   "Decoder(nil)",
						Name:      "Decoder",
						Call:      true,
						Arguments: []modulerules.Statement{{Written: "nil", Name: "nil"}},
					},
					{Written: "_ = from"},
				},
				Names: []modulerules.UsedName{
					{Name: "into"}, {Name: "Decoded"}, {Name: "from"}, {Name: "string"},
					{Name: "Decoder"}, {Name: "nil"},
				},
			},
		},
	}}))
}

func TestAScannedMethodChain_ShouldReportEveryExpressionItsNamesAreSelectedFrom(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"chain.go": chainedSource})

	scanned, err := modulerules.ScanGoFiles(probed, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("scan the Go files under %s: %v", probed, err)
	}

	theStep := modulerules.Statement{Written: "step", Name: "step"}
	theGivenCall := modulerules.Statement{
		Written:   "step.Given()",
		Name:      "Given",
		On:        &theStep,
		Arguments: []modulerules.Statement{},
		Call:      true,
	}
	expect.Expect(t, scanned).To(expect.Equal([]modulerules.GoFile{{
		Path:    filepath.Join(probed, "chain.go"),
		Package: probedModulePath,
		Imports: []string{},
		Declarations: []modulerules.Declaration{{
			Kind:       modulerules.FuncDeclaration,
			Name:       "Chain",
			Exported:   true,
			Parameters: []modulerules.Parameter{{Name: "step", Type: declaredType}},
			Statements: []modulerules.Statement{{
				Written:   "step.Given().Workload(limit)",
				Name:      "Workload",
				On:        &theGivenCall,
				Arguments: []modulerules.Statement{{Written: "limit", Name: "limit"}},
				Call:      true,
			}},
			Names: []modulerules.UsedName{
				{Name: "step"},
				{Name: declaredType},
				{Qualifier: theGivenCall.Written, Name: "Workload"},
				{Qualifier: theStep.Written, Name: "Given"},
				{Name: "limit"},
			},
		}},
	}}))
}

// Neither list a function literal declares names in is a name the declaration
// holding it writes: the expected names below hold the parameter's own type and
// the result's, and neither "settings" nor "reported" on its own.
func TestADeclarationHoldingAFunctionLiteral_ShouldReportTheNamesItUsesAndNotTheOnesThatLiteralDeclares(t *testing.T) {
	cases := []struct {
		name   string
		source string
		used   []modulerules.UsedName
	}{
		{
			name:   "a function literal declaring a parameter",
			source: literalSource,
			used: []modulerules.UsedName{
				{Name: "step"},
				{Name: declaredType},
				{Qualifier: "step", Name: "Customize"},
				{Name: "Decoded"},
				{Qualifier: theLiteralParameter, Name: "Field"},
				{Name: "limit"},
			},
		},
		{
			name:   "a function literal naming its own result as well",
			source: literalResultSource,
			used: []modulerules.UsedName{
				{Name: "step"},
				{Name: declaredType},
				{Qualifier: "step", Name: "Customize"},
				{Name: "Decoded"},
				{Name: "error"},
				{Qualifier: theLiteralParameter, Name: "Field"},
				{Name: "limit"},
			},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			probed := aModuleWhoseFilesAre(t, map[string]string{"reach.go": one.source})

			scanned, err := modulerules.ScanGoFiles(probed, modulerules.ProductionGoFilesOnly)
			if err != nil {
				t.Fatalf("scan the Go files under %s: %v", probed, err)
			}

			expect.Expect(t, theOneDeclarationOf(t, scanned).Names).To(expect.Equal(one.used))
		})
	}
}

// theOneDeclarationOf is the single declaration of a single scanned file, which a
// rule read on one declaration alone is asserted over.
func theOneDeclarationOf(t *testing.T, scanned []modulerules.GoFile) modulerules.Declaration {
	t.Helper()

	if len(scanned) != 1 || len(scanned[0].Declarations) != 1 {
		t.Fatalf("one scanned file holding one declaration, but: %+v", scanned)
	}

	return scanned[0].Declarations[0]
}

func TestAGoFileOfTestsAlone_ShouldBeScannedAtTheWiderFileSetAlone(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"scan_test.go": importingTestSource})

	shipped, err := modulerules.ScanGoFiles(probed, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("scan what the probed module ships: %v", err)
	}
	every, err := modulerules.ScanGoFiles(probed, modulerules.EveryGoFile)
	if err != nil {
		t.Fatalf("scan every Go file of the probed module: %v", err)
	}

	expect.Expect(t, shipped).To(expect.HaveLen[modulerules.GoFile](0))
	expect.Expect(t, every).To(expect.HaveLen[modulerules.GoFile](1))
}

func TestADirectoryUnderNoModule_ShouldBeRefusedRatherThanWalked(t *testing.T) {
	outsideAnyModule := t.TempDir()

	_, err := modulerules.Root(t.Context(), outsideAnyModule)

	if err == nil {
		t.Fatalf("a directory under no module is refused, but %s was accepted", outsideAnyModule)
	}
}

func TestTheRootOfAModule_ShouldBeTheDirectoryHoldingItsModuleFile(t *testing.T) {
	probed := aModuleWhoseFilesAre(t, map[string]string{"clean.go": cleanSource})

	root, err := modulerules.Root(t.Context(), probed)
	if err != nil {
		t.Fatalf("locate the root of the module in %s: %v", probed, err)
	}

	expect.Expect(t, root).To(expect.Equal(probed))
}

// aModuleWhoseFilesAre is the named default for a module the rules are asserted
// over: a directory of its own holding a module file and the named sources, each
// declaring the one package. The module requires nothing, so the graph walk needs
// no download.
func aModuleWhoseFilesAre(t *testing.T, sources map[string]string) string {
	t.Helper()

	root := t.TempDir()
	written := map[string]string{"go.mod": fmt.Sprintf("module %s\n\ngo 1.27\n", probedModulePath)}
	maps.Copy(written, sources)

	for name, content := range written {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s of the probed module: %v", name, err)
		}
	}

	return root
}

// The sources a keyed literal is read on: a struct literal whose keys name the
// fields it sets, a map literal whose keys are values it reads, and a slice of
// structs whose elements leave their type out.
const (
	aKeyedStructLiteralSource = `package probe

func Tolerate() Toleration {
	return pod.Toleration{Key: node.NotReady, TolerationSeconds: wait}
}
`
	aMapLiteralSource = `package probe

func Keyed() map[string]int {
	return map[string]int{key: count}
}
`
	anElidedStructLiteralSource = `package probe

func Tolerate() []pod.Toleration {
	return []pod.Toleration{{Key: node.NotReady}}
}
`
)

// The key of a struct literal names a field of the type written, which is no name
// the declaration reads: the fields a value of a package's type sets are that
// type's to declare. The key of a map literal is a value, so it stays a name used.
func TestAKeyedLiteral_ShouldReportTheNamesItsValuesUseAndNotTheFieldsItSets(t *testing.T) {
	cases := []struct {
		name   string
		source string
		used   []modulerules.UsedName
	}{
		{
			name:   "a struct literal of a package's type",
			source: aKeyedStructLiteralSource,
			used: []modulerules.UsedName{
				{Name: "Toleration"},
				{Qualifier: "pod", Name: "Toleration"},
				{Qualifier: "node", Name: "NotReady"},
				{Name: "wait"},
			},
		},
		{
			name:   "a map literal",
			source: aMapLiteralSource,
			used: []modulerules.UsedName{
				{Name: "string"},
				{Name: "int"},
				{Name: "key"},
				{Name: "count"},
			},
		},
		{
			name:   "a struct literal leaving its type out",
			source: anElidedStructLiteralSource,
			used: []modulerules.UsedName{
				{Qualifier: "pod", Name: "Toleration"},
				{Qualifier: "node", Name: "NotReady"},
			},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			probed := aModuleWhoseFilesAre(t, map[string]string{"keyed.go": one.source})

			scanned, err := modulerules.ScanGoFiles(probed, modulerules.ProductionGoFilesOnly)
			if err != nil {
				t.Fatalf("scan the Go files under %s: %v", probed, err)
			}

			expect.Expect(t, theOneDeclarationOf(t, scanned).Names).To(expect.Equal(one.used))
		})
	}
}
