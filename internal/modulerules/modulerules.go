// Package modulerules holds the package-graph walk and the call-rule scan the
// lab's module-rule tests share.
//
// Its callers are test files alone: no production package imports it, and the
// import-graph test asserts that absence, which is what keeps the package
// test-only without a linter rule
// (docs/adr/2026-10-04-module-rule-test-support.md). It imports the standard
// library alone, so a package reaching it is unaffected by the lab's own
// layering rules.
//
// Each helper reports its findings as the lines of a report, indented so they
// read as a list under the message a test prints them with, and an empty result
// means the rule holds.
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

// Scope selects which of a module's own packages a report covers.
type Scope int

const (
	// EveryPackage covers the module's production packages together with the test
	// variants go list reports beside them, which is the scope of a rule that
	// governs the lab's tests as much as the lab itself.
	EveryPackage Scope = iota

	// ProductionPackagesOnly drops those test variants, which is the scope of a
	// rule that governs what the lab ships and leaves its tests free to reach
	// further.
	ProductionPackagesOnly
)

// FileSet selects which Go files under a root a scan covers.
type FileSet int

const (
	// EveryGoFile covers the test files beside a package's sources too, which is
	// the scope of a rule that governs the lab's tests as much as the lab itself.
	EveryGoFile FileSet = iota

	// ProductionGoFilesOnly drops the test files, which is the scope of a rule
	// that governs what the lab ships and leaves its tests free to reach further.
	ProductionGoFilesOnly
)

// DeclarationKind is which of Go's top-level declarations a declaration is.
type DeclarationKind string

const (
	// FuncDeclaration is a top-level func declaration.
	FuncDeclaration DeclarationKind = "func"
	// TypeDeclaration is a top-level type declaration.
	TypeDeclaration DeclarationKind = "type"
	// ConstDeclaration is one name of a top-level const declaration.
	ConstDeclaration DeclarationKind = "const"
	// VarDeclaration is one name of a top-level var declaration.
	VarDeclaration DeclarationKind = "var"
)

// Parameter is one parameter of a declared function, its type as the source
// writes it.
type Parameter struct {
	Name string
	Type string
}

// Statement is one top-level statement of a function body, or one expression
// inside one, as a rule over it reads it: the source as written, the name it
// names, the expression a method is called on, and the arguments a call takes.
//
// A rule reads the shape the parser saw rather than the text of a line, so the
// layout gofmt gives a long call cannot defeat it.
type Statement struct {
	// Written is the statement or the expression as the source writes it.
	Written string

	// Name is the name this expression names: the identifier it is, or the one a
	// selector or a call names last. A statement naming none, such as an
	// assignment, carries the empty name.
	Name string

	// On is the expression the name is selected from, where the source writes one:
	// the package or value before the dot.
	On *Statement

	// Arguments are the arguments of a call, in the order written.
	Arguments []Statement

	// Call reports whether this expression is a call, which is what tells a call
	// taking no argument from the name it calls.
	Call bool
}

// UsedName is one name a declaration uses: the name itself, and what the source
// writes before the dot where it writes anything — the package a qualified name
// reaches through, or the expression a field or method is selected from.
type UsedName struct {
	Qualifier string
	Name      string
}

// Declaration is one top-level declaration of a Go file: what kind it is, the
// name it declares, the type a constant or variable names, the parameters of a
// function, the top-level statements of that function's body, and every name the
// declaration uses.
//
// The type and the parameters are printed back from the syntax tree, and the
// statements carry the shape the parser saw, so a rule over them reads the way
// the source does and a layout change cannot defeat it.
type Declaration struct {
	Kind     DeclarationKind
	Name     string
	Exported bool
	// Type is the type a constant or variable declaration writes, and is empty
	// where the source writes none, so a rule over a declaration of a named type
	// reads an untyped one as naming no type rather than as naming the right one.
	Type       string
	Parameters []Parameter
	Statements []Statement
	// Names is every name this declaration uses, each once and in the order it
	// first writes them: the types it names, the values it reads and the functions
	// it calls. Neither the name it declares itself nor a name a function literal
	// inside it declares for itself is one of them.
	Names []UsedName
}

// GoFile is one Go source file as scanned: the package it declares, the paths it
// imports, and the top-level declarations it holds.
type GoFile struct {
	Path         string
	Package      string
	Imports      []string
	Declarations []Declaration
}

// ScanGoFiles scans the Go files under root, in path order, reporting what each
// imports and what each declares. A root holding no Go file is reported as no
// file rather than as an error, so a rule over an empty place names what is
// missing itself.
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

// Root is the directory holding the module file of the module dir belongs to, so
// a package pattern resolved against it covers that whole module.
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

// OffendingImporters names every package of the module rooted at dir, in the
// graph of pattern, that imports a denied path, as "<importer> imports <denied
// path>".
//
// The report is built from the package graph rather than from the lint
// configuration, so a depguard rule that is scoped wrongly or never runs cannot
// make a rule pass silently.
//
// Only the module's own packages are reported, which is the scope the depguard
// rules these reports assert a second time have: a dependency reaching a denied
// path inside its own tree would make a rule unsatisfiable rather than
// meaningful. What the rules govern is the lab's own imports
// (docs/system-design.md § Threat Model).
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

	// go list emits concatenated objects rather than one array, and the graph runs
	// to a few thousand packages once the Kubernetes client is required, so it is
	// decoded one package at a time and only the offenders are kept
	// (docs/system-design.md § Scale and Load).
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

// CallsTo names every position in the Go files under root that calls function in
// the package at importPath, as "<position> calls <alias>.<function>".
//
// The call is resolved through each file's own import alias rather than through
// the text "<package>.<function>", so renaming the import on the way in cannot
// hide the call.
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

// importsOf are the paths parsed imports, in the order the file names them.
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

// declarationsOf is every top-level declaration of parsed but its imports, which
// a scan reports as the file's own imports instead.
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

// namesOf is every name one type, constant or variable declaration declares, a
// grouped declaration reported name by name, each with the type its own line
// writes.
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

// parametersOf are the parameters declared takes, each name with the type the
// source writes beside it. A parameter the source leaves unnamed carries the
// empty name.
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

// statementsOf are the top-level statements of the body declared holds, each the
// shape the parser saw: the expression a statement makes where it makes one, and
// the source as written otherwise.
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

// expressionOf is one expression as a rule reads it: the name it names, the
// expression that name is selected from, and the arguments of a call, each read
// the same way. An expression of a shape no rule here reads, such as a literal or
// a composite, carries the source it was written as and no name, which a rule
// reads as naming nothing rather than as naming the wrong thing.
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

// argumentsOf are the arguments of a call, in the order written.
func argumentsOf(positions *token.FileSet, written []ast.Expr) []Statement {
	arguments := make([]Statement, 0, len(written))
	for _, argument := range written {
		arguments = append(arguments, expressionOf(positions, argument))
	}

	return arguments
}

// namesUsedIn is every name the nodes use, each once and in the order they are
// first written: a qualified name under the qualifier its source writes, and an
// unqualified one on its own. The name a selector selects is not reported a
// second time on its own, so a rule over an unqualified name reads the names
// written unqualified alone. A function literal brings the names it declares for
// itself with it, its parameters and its results, so those are no names the nodes
// use either.
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

// namesDeclaredBy are the names a function type declares for itself: those of
// its parameters and those of its results, a result the source leaves unnamed
// contributing none. It is read off the literal that writes them rather than off
// every function type, so the parameters a declaration of its own takes stay
// names it writes.
func namesDeclaredBy(written *ast.FuncType) []*ast.Ident {
	declared := namesDeclaredIn(written.Params)
	// A function type naming no result holds no list for one, which is why the
	// results are read through the field rather than beside the parameters.
	if written.Results != nil {
		declared = append(declared, namesDeclaredIn(written.Results)...)
	}

	return declared
}

// namesDeclaredIn are the names one such list declares, a field the source leaves
// unnamed contributing none.
func namesDeclaredIn(fields *ast.FieldList) []*ast.Ident {
	var declared []*ast.Ident
	for _, field := range fields.List {
		declared = append(declared, field.Names...)
	}

	return declared
}

// theBlankIdentifier is the name an assignment discards a value under, which is
// no name a rule reads.
const theBlankIdentifier = "_"

// appendOnce is used with name appended, where it does not already hold it, so a
// name written twice is reported once.
func appendOnce(used []UsedName, name UsedName) []UsedName {
	if slices.Contains(used, name) {
		return used
	}

	return append(used, name)
}

// typeAndValuesOf are the parts of a constant or variable declaration that use
// names: the type it writes, where it writes one, and the values it is set to.
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

// printed is node as the source writes it. A node that cannot be printed is
// reported as that failure rather than as the empty string, so a rule reading it
// fails instead of passing on nothing.
func printed(positions *token.FileSet, node ast.Node) string {
	var written strings.Builder
	if err := printer.Fprint(&written, positions, node); err != nil {
		return fmt.Sprintf("<unprintable node at %s: %v>", positions.Position(node.Pos()), err)
	}

	return written.String()
}

// modulePath is the import path of the module the directory dir belongs to.
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

// isOwnedBy reports whether importPath is the module itself or a package under
// it.
func isOwnedBy(importPath, module string) bool {
	return importPath == module || strings.HasPrefix(importPath, module+"/")
}

// packagePathOf is the path of the package a listed entry belongs to. go list
// names the variant it compiles for a test binary "<package> [<binary>]", so the
// bracketed suffix is dropped before the module's own packages are picked out;
// without that, a module's own test variants would read as another module's
// packages and no rule would reach them.
func packagePathOf(importPath string) string {
	own, _, _ := strings.Cut(importPath, " [")

	return own
}

// isTestVariant reports whether a listed package is one go list synthesized for
// a test binary rather than a package the lab ships: the variant compiled for a
// test names the package it tests, and the test binary's own main package is
// named after it.
func isTestVariant(importPath, forTest string) bool {
	return forTest != "" || strings.HasSuffix(importPath, ".test")
}

// isDenied reports whether importPath is one of the denied paths or a package
// under one, so denying a path denies its subtree.
func isDenied(importPath string, denied []string) bool {
	return slices.ContainsFunc(denied, func(deniedPath string) bool {
		return importPath == deniedPath || strings.HasPrefix(importPath, deniedPath+"/")
	})
}

// aliasesOf is every name importPath is reachable under in parsed, which is its
// explicit alias where it has one and its package name otherwise.
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

// callPositions is where parsed calls alias.function.
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
