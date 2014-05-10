package goconsole

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
	oldSrc  string
	topEnv  *environ
	pkgs    map[string]*Package
	checker *checker
	typeMap *typeutil.Map
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

	declSrc, _ := i.topEnv.dumpScope()
	var allSrcBuf bytes.Buffer
	allSrcBuf.WriteString("package p;")
	for _, pkg := range i.pkgs {
		fmt.Fprintf(&allSrcBuf, "import \"%s\";", pkg.Pkg.Path())
	}
	allSrcBuf.WriteString(declSrc)
	allSrcBuf.WriteString("func _(){")
	allSrcBuf.WriteString(src)
	allSrcBuf.WriteString("\n}")
	allSrc := allSrcBuf.String()
	fileSize := len(allSrc)

	// Parse it
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input", allSrc, 0)
	if err != nil {
		if errList, ok := err.(scanner.ErrorList); ok {
			for j, err := range errList {
				// Check if the error is at EOF or at the closing brace we added
				//fmt.Println("pos:", err.Pos.Offset, "filesize:", fileSize)
				if err.Pos.Offset == fileSize || err.Pos.Offset == fileSize-1 {
					// If this is the first error, it actually just means the source is incomplete,
					// unless there is a superfluous '}' at the end of their code
					if j == 0 && err.Msg != "expected declaration, found '}'" {
						i.oldSrc = src
						return true, nil
					}
				} else {
					fmt.Println("Parse error:", err)
				}
			}
		} else {
			log.Fatal("Parsing yielded a non-nil error that's not a scanner.ErrorList")
		}
		return false, err
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
		fmt.Println("Type error:", i.checker.errs[0])
		return false, i.checker.errs[0]
	}
	// get the scope of the wrapper function for the user code
	ps := pkg.Scope().Child(0)
	nc := ps.NumChildren()
	i.topEnv.scope = ps.Child(nc - 1)
	i.topEnv.info = &info

	// Extract the statement list provided by the user
	stmtList := file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body.List
	if len(stmtList) == 0 {
		return false, nil
	}
	// Run each statement in the list
	for _, stmt := range stmtList {
		i.topEnv.runStmt(stmt, true)
	}
	return false, nil
}
