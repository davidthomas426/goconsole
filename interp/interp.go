package interp

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"log"
	"strings"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

type interp struct {
	oldSrc       string
	topEnv       *environ
	pkgs         map[string]*Package
	checker      *checker
	typeMap      *typeutil.Map
	stmtLists    []string
	stmtListLens []int
}

func newInterp(pkgs []*Package, pkgMap map[string]*types.Package, typeMap *typeutil.Map) Interpreter {
	// Setup package map
	pkgObjMap := map[string]*Package{}
	for _, pkg := range pkgs {
		pkgObjMap[pkg.Name] = pkg
	}
	addBasicTypes(typeMap)
	i := &interp{
		pkgs: pkgObjMap,
		topEnv: &environ{
			objs: map[string]Object{},
		},
		checker: newChecker(pkgs, pkgMap),
		typeMap: typeMap,
	}
	i.topEnv.interp = i
	return i
}

type checker struct {
	config types.Config
	errs   []error
}

func newChecker(pkgs []*Package, pkgMap map[string]*types.Package) *checker {
	var c *checker
	c = &checker{
		config: types.Config{
			Error: func(err error) {
				switch e := err.(type) {
				case types.Error:
					// Ignore errors about unused variables, imports, and labels
					if !strings.Contains(e.Msg, "but not used") && !strings.Contains(e.Msg, "is not used") {
						c.errs = append(c.errs, err)
					}
				default:
					c.errs = append(c.errs, err)
				}
			},
			Packages: pkgMap,
		},
		errs: []error{},
	}
	return c
}

func (i *interp) Run(src string) (bool, error) {
	src = strings.TrimSpace(src)
	if len(src) == 0 {
		if i.oldSrc == "" {
			return false, nil
		}
		return true, nil
	}

	if i.oldSrc != "" {
		src = i.oldSrc + "\n" + src
		i.oldSrc = ""
	}

	// TODO: Only dump out declarations after each input rather than entire history of source.
	var allSrcBuf bytes.Buffer
	allSrcBuf.WriteString("package p;import(")
	for _, pkg := range i.pkgs {
		fmt.Fprintf(&allSrcBuf, "%q;", pkg.Pkg.Path())
	}
	allSrcBuf.WriteString(");func _(){")

	// Add previous code, one stmtList at a time, each in a nested scope
	for _, stmtList := range i.stmtLists {
		allSrcBuf.WriteString(stmtList)
		allSrcBuf.WriteString("\n{")
	}
	// Add current code in the innermost scope and close the scopes
	allSrcBuf.WriteString(src)
	allSrcBuf.WriteString("\n")
	for _ = range i.stmtLists {
		allSrcBuf.WriteString("}")
	}
	allSrcBuf.WriteString("}")

	// Get the source "file" as a string
	allSrc := allSrcBuf.String()
	fileSize := len(allSrc)

	// Parse it
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input", allSrc, 0)
	if err != nil {
		if errList, ok := err.(scanner.ErrorList); ok {
			for j, err := range errList {
				// Check if the error is at EOF or at a closing brace we added
				if err.Pos.Offset >= fileSize-1-len(i.stmtLists) {
					// If this is the first error, it actually just means the source is incomplete,
					// unless there is a superfluous '}' at the end of their code
					if j == 0 && err.Msg != "expected declaration, found '}'" {
						i.oldSrc = src
						return true, nil
					}
				}
			}
		} else {
			log.Fatal("Parsing yielded a non-nil error that's not a scanner.ErrorList")
		}
		return false, err
	}

	if len(file.Decls) != 2 {
		// The input must have done something strange with braces
		err := fmt.Errorf("Unexpected '}'")
		return false, err
	}

	// Walk down the scopes to the inner statement list, checking that nothing
	// looks wrong along the way
	stmtList := file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body.List
	for j := range i.stmtLists {
		if len(stmtList) != i.stmtListLens[j]+1 {
			// There must be an extra closing brace that escaped our block statement
			err := fmt.Errorf("Unexpected '}'")
			return false, err
		}
		blockStmt, ok := stmtList[len(stmtList)-1].(*ast.BlockStmt)
		if !ok {
			err := fmt.Errorf("Parse error")
			return false, err
		}
		stmtList = blockStmt.List
	}
	if len(stmtList) == 0 {
		return false, nil
	}

	// Clear the type-checker errors and create a struct to hold type info
	i.checker.errs = i.checker.errs[:0]
	info := types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
	}
	// Type check the statement list
	files := []*ast.File{file}
	pkg, _ := i.checker.config.Check("", fset, files, &info)
	if len(i.checker.errs) > 0 {
		return false, i.checker.errs[0]
	}

	// Walk down the scopes to the inner statement list, checking that nothing
	// looks wrong along the way
	pkgScope := pkg.Scope().Child(0)
	undScope := pkgScope.Child(pkgScope.NumChildren() - 1)
	currScope := undScope
	for _ = range i.stmtLists {
		currScope = currScope.Child(currScope.NumChildren() - 1)
	}
	// get the scope of the block stmt containing user code
	i.topEnv.scope = currScope
	i.topEnv.info = &info

	// Run each statement in the list
	for _, stmt := range stmtList {
		i.topEnv.runStmt(stmt, true)
	}

	// Add current input to the stmtLists slice for next time
	i.stmtLists = append(i.stmtLists, src)
	i.stmtListLens = append(i.stmtListLens, len(stmtList))

	return false, nil
}
