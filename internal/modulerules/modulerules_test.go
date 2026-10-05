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

	// declaredType is the type scannedSource writes on the one constant that names
	// one, which is what a rule over a declaration of a named type reads.
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
)

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
			{Kind: modulerules.TypeDeclaration, Name: "Name", Exported: true},
			{Kind: modulerules.ConstDeclaration, Name: "limit"},
			{Kind: modulerules.ConstDeclaration, Name: "Labelled", Exported: true, Type: declaredType},
			{Kind: modulerules.VarDeclaration, Name: "Decoder", Exported: true},
			{
				Kind:       modulerules.FuncDeclaration,
				Name:       "Scan",
				Exported:   true,
				Parameters: []modulerules.Parameter{{Name: "into", Type: "*Decoded"}, {Name: "from", Type: "string"}},
				Statements: []string{"Decoder(nil)", "_ = from"},
			},
		},
	}}))
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
