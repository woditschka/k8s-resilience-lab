// Package modulerules holds the package-graph walk and the source scans the
// lab's module-rule tests share.
//
// Its callers are test files alone, which the import-graph test asserts, so the
// package stays test-only without a linter rule. It imports the standard
// library alone, so a package reaching it is unaffected by the lab's layering
// rules.
//
// Each helper reports its findings as indented report lines, and an empty
// result means the rule holds.
package modulerules

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Scope chooses whether a rule over the package graph also reads the test
// variants go list reports.
type Scope int

const (
	// EveryPackage includes the test variants go list reports, for a rule that
	// governs the lab's tests as much as the lab itself.
	EveryPackage Scope = iota

	// ProductionPackagesOnly drops the test variants, for a rule that governs
	// what the lab ships and leaves its tests free to reach further.
	ProductionPackagesOnly
)

// FileSet chooses whether a scan reads the test files along with the files the
// lab ships.
type FileSet int

const (
	EveryGoFile FileSet = iota

	ProductionGoFilesOnly
)

// DeclarationKind is the keyword a top-level declaration opens with.
type DeclarationKind string

const (
	FuncDeclaration  DeclarationKind = "func"
	TypeDeclaration  DeclarationKind = "type"
	ConstDeclaration DeclarationKind = "const"
	VarDeclaration   DeclarationKind = "var"
)

// Parameter is one parameter of a function declaration, its type printed back
// from the syntax tree.
type Parameter struct {
	Name string
	Type string
}

// Statement is a statement or an expression as the parser saw it rather than
// the text of a line, so the layout gofmt gives a long call cannot defeat a
// rule.
type Statement struct {
	Written string

	// A statement naming nothing, such as an assignment, carries the empty
	// Name.
	Name string

	On *Statement

	Arguments []Statement

	// Call tells a call taking no argument from the name it calls.
	Call bool
}

// UsedName is a name a declaration uses; on a qualified name, Qualifier is the
// package or the expression the name is selected from.
type UsedName struct {
	Qualifier string
	Name      string
}

// Declaration prints its type and parameters back from the syntax tree, so a
// rule reads them the way the source does and a layout change cannot defeat it.
type Declaration struct {
	Kind     DeclarationKind
	Name     string
	Exported bool
	// Type is empty where the source writes none, so an untyped declaration
	// reads as naming no type rather than the right one.
	Type       string
	Parameters []Parameter
	Statements []Statement
	// Names excludes the name declared itself, names a function literal inside
	// declares for itself, and fields a struct literal inside sets.
	Names []UsedName
}

// GoFile is one Go file as the parser saw it, which a rule reads instead of the
// file's text.
type GoFile struct {
	Path         string
	Package      string
	Imports      []string
	Declarations []Declaration
}

// ScanGoFiles reports a root holding no Go file as no file rather than as an
// error, so a rule over an empty place names what is missing itself.
func ScanGoFiles(root string, files FileSet) ([]GoFile, error) {
	var scanned []GoFile
	positions := token.NewFileSet()

	walked := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		if files == ProductionGoFilesOnly && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(positions, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		scanned = append(scanned, GoFile{
			Path:         path,
			Package:      parsed.Name.Name,
			Imports:      importsOf(parsed),
			Declarations: declarationsOf(positions, parsed),
		})

		return nil
	})
	if walked != nil {
		return nil, fmt.Errorf("read the Go files under %s: %w", root, walked)
	}

	return scanned, nil
}

func Root(ctx context.Context, dir string) (string, error) {
	command := exec.CommandContext(ctx, "go", "env", "GOMOD")
	command.Dir = dir
	command.Stderr = os.Stderr

	located, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("locate the module file of %s: %w", dir, err)
	}

	moduleFile := strings.TrimSpace(string(located))
	if moduleFile == "" || moduleFile == os.DevNull {
		return "", fmt.Errorf("no module file covers %s, so a module-wide rule cannot be asserted", dir)
	}

	return filepath.Dir(moduleFile), nil
}

// OffendingImporters builds its report from the package graph rather than the
// lint configuration, so a depguard rule scoped wrongly or never run cannot
// make a rule pass silently. Only the module's own packages are reported, as
// depguard's rules are: a dependency reaching a denied path inside its own tree
// would make a rule unsatisfiable rather than meaningful.
func OffendingImporters(ctx context.Context, dir, pattern string, scope Scope, denied []string) ([]string, error) {
	own, err := modulePath(ctx, dir)
	if err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, "go", "list", "-deps", "-test", "-json", pattern)
	command.Dir = dir
	command.Stderr = os.Stderr

	listing, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("read the output of go list: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("run go list in %s: %w", dir, err)
	}

	// go list emits concatenated objects rather than one array, and the graph
	// runs to a few thousand packages, so it is decoded one package at a time
	// and only the offenders are kept.
	var offenders []string
	packages := json.NewDecoder(listing)
	for packages.More() {
		var listed struct {
			ImportPath string
			ForTest    string
			Imports    []string
		}
		if err := packages.Decode(&listed); err != nil {
			return nil, fmt.Errorf("decode the output of go list: %w", err)
		}

		if !isOwnedBy(packagePathOf(listed.ImportPath), own) {
			continue
		}
		if scope == ProductionPackagesOnly && isTestVariant(listed.ImportPath, listed.ForTest) {
			continue
		}
		for _, imported := range listed.Imports {
			if isDenied(imported, denied) {
				offenders = append(offenders, fmt.Sprintf("  %s imports %s", listed.ImportPath, imported))
			}
		}
	}

	if err := command.Wait(); err != nil {
		return nil, fmt.Errorf("go list in %s: %w", dir, err)
	}

	slices.Sort(offenders)

	return offenders, nil
}

// CallsTo resolves the call through each file's own import alias, so renaming
// the import cannot hide the call.
func CallsTo(root, importPath, function string) ([]string, error) {
	var offenders []string
	files := token.NewFileSet()

	walked := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		parsed, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		for _, alias := range aliasesOf(parsed, importPath) {
			for _, position := range callPositions(files, parsed, alias, function) {
				offenders = append(offenders, fmt.Sprintf("  %s calls %s.%s", position, alias, function))
			}
		}

		return nil
	})
	if walked != nil {
		return nil, fmt.Errorf("read the Go files under %s: %w", root, walked)
	}

	slices.Sort(offenders)

	return offenders, nil
}

func importsOf(parsed *ast.File) []string {
	imported := make([]string, 0, len(parsed.Imports))
	for _, declared := range parsed.Imports {
		path, err := strconv.Unquote(declared.Path.Value)
		if err != nil {
			continue
		}
		imported = append(imported, path)
	}

	return imported
}

func declarationsOf(positions *token.FileSet, parsed *ast.File) []Declaration {
	var declared []Declaration
	for _, declaration := range parsed.Decls {
		switch shape := declaration.(type) {
		case *ast.FuncDecl:
			declared = append(declared, Declaration{
				Kind:       FuncDeclaration,
				Name:       shape.Name.Name,
				Exported:   shape.Name.IsExported(),
				Parameters: parametersOf(positions, shape),
				Statements: statementsOf(positions, shape),
				Names:      namesUsedIn(positions, shape.Type, shape.Body),
			})
		case *ast.GenDecl:
			if shape.Tok == token.IMPORT {
				continue
			}
			declared = append(declared, namesOf(positions, shape)...)
		}
	}

	return declared
}

func namesOf(positions *token.FileSet, declaration *ast.GenDecl) []Declaration {
	var declared []Declaration
	for _, spec := range declaration.Specs {
		var names []*ast.Ident
		var named string
		var used []UsedName
		switch shape := spec.(type) {
		case *ast.TypeSpec:
			names = []*ast.Ident{shape.Name}
			used = namesUsedIn(positions, shape.Type)
		case *ast.ValueSpec:
			names = shape.Names
			if shape.Type != nil {
				named = printed(positions, shape.Type)
			}
			used = namesUsedIn(positions, typeAndValuesOf(shape)...)
		}

		for _, name := range names {
			declared = append(declared, Declaration{
				Kind:     DeclarationKind(declaration.Tok.String()),
				Name:     name.Name,
				Exported: name.IsExported(),
				Type:     named,
				Names:    used,
			})
		}
	}

	return declared
}

// A parameter the source leaves unnamed carries the empty name.
func parametersOf(positions *token.FileSet, declared *ast.FuncDecl) []Parameter {
	var parameters []Parameter
	for _, field := range declared.Type.Params.List {
		written := printed(positions, field.Type)
		if len(field.Names) == 0 {
			parameters = append(parameters, Parameter{Type: written})

			continue
		}
		for _, name := range field.Names {
			parameters = append(parameters, Parameter{Name: name.Name, Type: written})
		}
	}

	return parameters
}

func statementsOf(positions *token.FileSet, declared *ast.FuncDecl) []Statement {
	if declared.Body == nil {
		return nil
	}

	statements := make([]Statement, 0, len(declared.Body.List))
	for _, statement := range declared.Body.List {
		expression, isExpression := statement.(*ast.ExprStmt)
		if !isExpression {
			statements = append(statements, Statement{Written: printed(positions, statement)})

			continue
		}
		statements = append(statements, expressionOf(positions, expression.X))
	}

	return statements
}

// An expression of a shape no rule reads, such as a literal, carries its source
// and no name, so a rule reads it as naming nothing rather than the wrong
// thing.
func expressionOf(positions *token.FileSet, expression ast.Expr) Statement {
	read := Statement{Written: printed(positions, expression)}
	switch shape := expression.(type) {
	case *ast.Ident:
		read.Name = shape.Name
	case *ast.SelectorExpr:
		selected := expressionOf(positions, shape.X)
		read.Name, read.On = shape.Sel.Name, &selected
	case *ast.CallExpr:
		called := expressionOf(positions, shape.Fun)
		read.Name, read.On, read.Call = called.Name, called.On, true
		read.Arguments = argumentsOf(positions, shape.Args)
	}

	return read
}

func argumentsOf(positions *token.FileSet, written []ast.Expr) []Statement {
	arguments := make([]Statement, 0, len(written))
	for _, argument := range written {
		arguments = append(arguments, expressionOf(positions, argument))
	}

	return arguments
}

// The name a selector selects is not reported again on its own, so a rule over
// unqualified names reads only those written unqualified. Names a function
// literal declares for itself are excluded.
func namesUsedIn(positions *token.FileSet, nodes ...ast.Node) []UsedName {
	var used []UsedName
	broughtAlong := make(map[*ast.Ident]bool)

	for _, node := range nodes {
		if node == nil {
			continue
		}
		ast.Inspect(node, func(visited ast.Node) bool {
			switch shape := visited.(type) {
			case *ast.SelectorExpr:
				broughtAlong[shape.Sel] = true
				if qualifier, isName := shape.X.(*ast.Ident); isName {
					broughtAlong[qualifier] = true
					used = appendOnce(used, UsedName{Qualifier: qualifier.Name, Name: shape.Sel.Name})

					return true
				}
				used = appendOnce(used, UsedName{Qualifier: printed(positions, shape.X), Name: shape.Sel.Name})
			case *ast.FuncLit:
				for _, declared := range namesDeclaredBy(shape.Type) {
					broughtAlong[declared] = true
				}
			case *ast.CompositeLit:
				for _, field := range fieldsSetBy(shape) {
					broughtAlong[field] = true
				}
			case *ast.Ident:
				if !broughtAlong[shape] && shape.Name != theBlankIdentifier {
					used = appendOnce(used, UsedName{Name: shape.Name})
				}
			}

			return true
		})
	}

	return used
}

// A map literal's keys are values and a slice literal's are indices, so only a
// struct literal sets fields; a literal leaving its type out is read as a
// struct, the element type a source most often leaves out.
func fieldsSetBy(literal *ast.CompositeLit) []*ast.Ident {
	switch literal.Type.(type) {
	case *ast.MapType, *ast.ArrayType:
		return nil
	}

	var fields []*ast.Ident
	for _, element := range literal.Elts {
		keyed, isKeyed := element.(*ast.KeyValueExpr)
		if !isKeyed {
			continue
		}
		if field, isAName := keyed.Key.(*ast.Ident); isAName {
			fields = append(fields, field)
		}
	}

	return fields
}

// These are read off the literal rather than off every function type, so the
// parameters a declaration of its own takes stay names it writes.
func namesDeclaredBy(written *ast.FuncType) []*ast.Ident {
	declared := namesDeclaredIn(written.Params)
	// A function type naming no result holds no list for one.
	if written.Results != nil {
		declared = append(declared, namesDeclaredIn(written.Results)...)
	}

	return declared
}

func namesDeclaredIn(fields *ast.FieldList) []*ast.Ident {
	var declared []*ast.Ident
	for _, field := range fields.List {
		declared = append(declared, field.Names...)
	}

	return declared
}

const theBlankIdentifier = "_"

func appendOnce(used []UsedName, name UsedName) []UsedName {
	if slices.Contains(used, name) {
		return used
	}

	return append(used, name)
}

func typeAndValuesOf(spec *ast.ValueSpec) []ast.Node {
	written := make([]ast.Node, 0, len(spec.Values)+1)
	if spec.Type != nil {
		written = append(written, spec.Type)
	}
	for _, value := range spec.Values {
		written = append(written, value)
	}

	return written
}

// A node that cannot be printed is reported as that failure rather than as the
// empty string, so a rule reading it fails instead of passing on nothing.
func printed(positions *token.FileSet, node ast.Node) string {
	var written strings.Builder
	if err := printer.Fprint(&written, positions, node); err != nil {
		return fmt.Sprintf("<unprintable node at %s: %v>", positions.Position(node.Pos()), err)
	}

	return written.String()
}

func modulePath(ctx context.Context, dir string) (string, error) {
	command := exec.CommandContext(ctx, "go", "list", "-m")
	command.Dir = dir
	command.Stderr = os.Stderr

	listed, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read the module path in %s: %w", dir, err)
	}

	return strings.TrimSpace(string(listed)), nil
}

func isOwnedBy(importPath, module string) bool {
	return importPath == module || strings.HasPrefix(importPath, module+"/")
}

// go list names a test variant "<package> [<binary>]", so the suffix is
// dropped; otherwise the module's own test variants would read as another
// module's packages.
func packagePathOf(importPath string) string {
	own, _, _ := strings.Cut(importPath, " [")

	return own
}

func isTestVariant(importPath, forTest string) bool {
	return forTest != "" || strings.HasSuffix(importPath, ".test")
}

// Denying a path denies its subtree.
func isDenied(importPath string, denied []string) bool {
	return slices.ContainsFunc(denied, func(deniedPath string) bool {
		return importPath == deniedPath || strings.HasPrefix(importPath, deniedPath+"/")
	})
}

func aliasesOf(parsed *ast.File, importPath string) []string {
	var aliases []string
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != importPath {
			continue
		}

		if imported.Name != nil {
			aliases = append(aliases, imported.Name.Name)
			continue
		}
		aliases = append(aliases, path[strings.LastIndex(path, "/")+1:])
	}

	return aliases
}

func callPositions(files *token.FileSet, parsed *ast.File, alias, function string) []string {
	var positions []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		selected, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || selected.Sel.Name != function {
			return true
		}
		if qualifier, isName := selected.X.(*ast.Ident); isName && qualifier.Name == alias {
			positions = append(positions, files.Position(call.Pos()).String())
		}

		return true
	})

	return positions
}
