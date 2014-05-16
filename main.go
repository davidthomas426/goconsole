package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"local/goconsole/interp"

	_ "code.google.com/p/go.tools/go/gcimporter"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

type Import struct {
	LocalName string
	Path      string
}

type Package struct {
	Path    string
	Name    string
	Types   []Type
	Objects []Object
	typeMap *typeutil.Map
}

type Type struct {
	CheckerType string
	TypeString  string
}

type Object struct {
	Name      string
	Qualified string
}

type Interp struct {
	Imports  []Import
	Packages []Package
}

func visitedType(typ types.Type) bool {
	return typeMap.At(typ) != nil
}

func (p *Package) AddType(typ types.Type, t Type) {
	typeMap.Set(typ, struct{}{})
	p.Types = append(p.Types, t)
}

var typeMap = new(typeutil.Map)

func main() {
	importSet := map[Import]bool{
		Import{Path: "bufio"}:                  true,
		Import{Path: "fmt"}:                    true,
		Import{Path: "local/goconsole/interp"}: true,
		Import{Path: "os"}:                     true,
		Import{LocalName: "_", Path: "code.google.com/p/go.tools/go/gcimporter"}: true,
		Import{Path: "code.google.com/p/go.tools/go/types"}:                      true,
		Import{Path: "code.google.com/p/go.tools/go/types/typeutil"}:             true,
	}

	if len(os.Args) >= 2 {
		// At least one package to import provided on command line
		importSet[Import{Path: "log"}] = true
		importSet[Import{Path: "reflect"}] = true
	}

	pkgMap := map[string]*types.Package{}
	var pkgs []Package
	for _, path := range os.Args[1:] {
		tpkg, err := types.DefaultImport(pkgMap, path)
		if err != nil {
			log.Fatal(err)
		}
		imp := Import{Path: path}
		importSet[imp] = true
		pkg := &Package{
			Path: path,
			Name: tpkg.Name(),
		}
		for _, name := range tpkg.Scope().Names() {
			obj := tpkg.Scope().Lookup(name)
			if obj.Exported() {
				processObj(importSet, pkg, obj)
			}
		}
		pkgs = append(pkgs, *pkg)
	}

	imports := make([]Import, 0, len(importSet))
	for imp := range importSet {
		imports = append(imports, imp)
	}

	interp := &Interp{
		Imports:  imports,
		Packages: pkgs,
	}

	workDir, err := ioutil.TempDir("", "goconsole")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(workDir)

	fn := filepath.Join(workDir, "goconsole.go")

	srcFile, err := os.Create(fn)
	if err != nil {
		log.Panic(err)
	}
	defer srcFile.Close()

	err = interpTmpl.Execute(srcFile, interp)
	if err != nil {
		log.Panic(err)
	}

	srcFile.Close()

	cmd := exec.Command("go", "run", fn)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
}

func processObj(importSet map[Import]bool, pkg *Package, obj types.Object) {
	switch obj := obj.(type) {
	case *types.TypeName:
		cts := fmt.Sprintf("scope.Lookup(%q).Type()", obj.Name())
		processType(importSet, pkg, obj.Type(), cts)
	case *types.Func, *types.Var:
		processVar(importSet, pkg, obj)
	}
}

func processType(importSet map[Import]bool, pkg *Package, typ types.Type, checkerTypeStr string) {
	// For certain types we want to do nothing
	switch typ.(type) {
	case *types.Basic, *types.Interface:
		return
	}

	t := Type{
		CheckerType: checkerTypeStr,
		TypeString:  interp.TypeString(typ),
	}
	index := len(pkg.Types)
	pkg.AddType(typ, t)

	switch typ := typ.(type) {
	case *types.Named:
		if typ.Obj().Pkg() != nil && !typ.Obj().Exported() {
			fmt.Fprintln(os.Stderr, t.TypeString, "not exported!")
		}

		// Named types have underlying types
		if undTyp := typ.Underlying(); !visitedType(undTyp) && !hasUnexportedType(undTyp) {
			undCts := fmt.Sprintf("t%d.Underlying()", index)
			processType(importSet, pkg, undTyp, undCts)
		}
		// Add a new import, if needed
		if tpkg := typ.Obj().Pkg(); tpkg != nil {
			imp := Import{
				Path: tpkg.Path(),
			}
			importSet[imp] = true
		}
	case *types.Signature:
		// Note: checking params and results for unexported types would be redundant at this point,
		//       since we already know that the function type didn't have unexported types.
		for i := 0; i < typ.Params().Len(); i++ {
			if paramType := typ.Params().At(i).Type(); !visitedType(paramType) {
				paramCts := fmt.Sprintf("t%d.(*types.Signature).Params().At(%d).Type()", index, i)
				processType(importSet, pkg, paramType, paramCts)
			}
		}
		for i := 0; i < typ.Results().Len(); i++ {
			if resultType := typ.Results().At(i).Type(); !visitedType(resultType) {
				resultCts := fmt.Sprintf("t%d.(*types.Signature).Results().At(%d).Type()", index, i)
				processType(importSet, pkg, resultType, resultCts)
			}
		}
	case *types.Struct:
		// Note: checking fields for unexported types would be redundant (see case for *types.Signature)
		for i := 0; i < typ.NumFields(); i++ {
			if fieldType := typ.Field(i).Type(); !visitedType(fieldType) {
				fieldCts := fmt.Sprintf("t%d.(*types.Struct).Field(%d).Type()", index, i)
				processType(importSet, pkg, fieldType, fieldCts)
			}
		}
	case *types.Slice:
		elemType := typ.Elem()
		elemCts := fmt.Sprintf("t%d.(*types.Slice).Elem()", index)
		processType(importSet, pkg, elemType, elemCts)
	case *types.Pointer:
		elemType := typ.Elem()
		elemCts := fmt.Sprintf("t%d.(*types.Pointer).Elem()", index)
		processType(importSet, pkg, elemType, elemCts)
	case *types.Map:
		keyType := typ.Key()
		keyCts := fmt.Sprintf("t%d.(*types.Map).Key()", index)
		processType(importSet, pkg, keyType, keyCts)
		elemType := typ.Elem()
		elemCts := fmt.Sprintf("t%d.(*types.Map).Elem()", index)
		processType(importSet, pkg, elemType, elemCts)
	default:
		// TODO: Handle other types (chan, array)
	}
}

func processVar(importSet map[Import]bool, pkg *Package, obj types.Object) {
	if !obj.Exported() {
		return
	}
	// Add an Object to pkg.Objects
	o := Object{
		Name:      obj.Name(),
		Qualified: pkg.Name + "." + obj.Name(),
	}
	pkg.Objects = append(pkg.Objects, o)

	if typ := obj.Type(); !visitedType(typ) && !hasUnexportedType(typ) {
		cts := fmt.Sprintf("scope.Lookup(%q).Type()", obj.Name())
		processType(importSet, pkg, obj.Type(), cts)
	}
}

func hasUnexportedType(typ types.Type) bool {
	switch typ := typ.(type) {
	case *types.Signature:
		for i := 0; i < typ.Params().Len(); i++ {
			if hasUnexportedType(typ.Params().At(i).Type()) {
				return true
			}
		}
		for i := 0; i < typ.Results().Len(); i++ {
			if hasUnexportedType(typ.Results().At(i).Type()) {
				return true
			}
		}
		return false
	case *types.Struct:
		for i := 0; i < typ.NumFields(); i++ {
			if hasUnexportedType(typ.Field(i).Type()) {
				return true
			}
		}
		return false
	case *types.Named:
		return typ.Obj().Pkg() != nil && !typ.Obj().Exported()
	case *types.Slice:
		return hasUnexportedType(typ.Elem())
	case *types.Pointer:
		return hasUnexportedType(typ.Elem())
	case *types.Map:
		return hasUnexportedType(typ.Key()) || hasUnexportedType(typ.Elem())
	}
	return false
}

var interpTmpl = template.Must(template.New("interp").Parse(interpStr))

var interpStr = `
package main

import ({{range .Imports}}
	{{with .LocalName}}{{.}} {{end}}{{printf "%q" .Path}}{{end}}
)

func main() {
	pkgMap := map[string]*types.Package{}
	typeMap := new(typeutil.Map)
	pkgs := []*interp.Package{}
{{range .Packages}}
	{
		tpkg, err := types.DefaultImport(pkgMap, {{printf "%q" .Path}})
		if err != nil {
			log.Fatal(err)
		}
		scope := tpkg.Scope()
		pkg := &interp.Package{
			Name: {{printf "%q" .Name}},
			Pkg:  tpkg,
			Objs: map[string]interp.Object{},
		}
		pkgs = append(pkgs, pkg)

	{{range $index, $typ := .Types}}
		t{{$index}} := {{$typ.CheckerType}}
		var pv{{$index}} *{{$typ.TypeString}}
		rt{{$index}} := reflect.TypeOf(pv{{$index}}).Elem()
		typeMap.Set(t{{$index}}, rt{{$index}})
	{{end}}
	{{range .Objects}}
		pkg.Objs[{{printf "%q" .Name}}] = interp.Object{
			Value: reflect.ValueOf({{.Qualified}}),
			Typ: scope.Lookup({{printf "%q" .Name}}).Type(),
		}
	{{end}}
	}
{{end}}

	interp := interp.NewInterpreter(pkgs, pkgMap, typeMap)
	scan := bufio.NewScanner(os.Stdin)
	fmt.Print(">>> ")
	for scan.Scan() {
		src := scan.Text()
		incomplete, err := interp.Run(src)
		if err != nil {
			fmt.Println(err)
		}
		if incomplete {
			fmt.Print("... ")
		} else {
			fmt.Print(">>> ")
		}
	}
	fmt.Println()
}
`
