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
	everyPackageInTheModule = "./..."

	probedModulePath = "probed"

	importedPath   = "encoding/json"
	unimportedPath = "k8s.io/client-go/rest"

	// The calling file imports the package under callAlias, so the scan has to
	// resolve the alias rather than match the text of the call.
	scannedCall = "InClusterConfig"
	callAlias   = "rename"

	callAtLine   = 5
	callAtColumn = 18

	declaredType = "Name"
)

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

	// chainedSource chains a method onto a call, the shape a Scenario writes
	// its Steps in.
	chainedSource = `package probed

func Chain(step Name) {
	step.Given().Workload(limit)
}
`

	// literalSource passes a function literal to a call, the shape a Scenario
	// writes full access in.
	literalSource = `package probed

func Reach(step Name) {
	step.Customize(func(settings *Decoded) {
		settings.Field = limit
	})
}
`

	// No Scenario writes a literal naming its result today, but the rules read
	// every function literal alike. The body writes that result nowhere, so the
	// declared list is the one place the scan can read it from.
	literalResultSource = `package probed

func Report(step Name) {
	step.Customize(func(settings *Decoded) (reported error) {
		settings.Field = limit

		return
	})
}
`
)

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

// The module requires nothing, so the graph walk needs no download.
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

// The fields a struct literal sets are its type's to declare. The key of a map
// literal is a value, so it stays a name used.
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
