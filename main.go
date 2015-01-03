package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/davidthomas426/goconsole/interp"

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
	CheckerType      string
	UseReflectString bool
	TypeString       string
	ReflectString    string
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
		Import{Path: "bufio"}:                                      true,
		Import{Path: "fmt"}:                                        true,
		Import{Path: "github.com/davidthomas426/goconsole/interp"}: true,
		Import{Path: "os"}:                                         true,
		Import{LocalName: "_", Path: "code.google.com/p/go.tools/go/gcimporter"}: true,
		Import{Path: "code.google.com/p/go.tools/go/types"}:                      true,
		Import{Path: "code.google.com/p/go.tools/go/types/typeutil"}:             true,
	}

	if len(os.Args) >= 2 {
		// At least one package to import provided on command line
		importSet[Import{Path: "log"}] = true
		importSet[Import{Path: "reflect"}] = true
	}


	pkgNames := make(map[string]bool)

	pkgMap := map[string]*types.Package{}
	var pkgs []Package
	var tpkgs []*types.Package
	for _, path := range os.Args[1:] {
		var tpkg *types.Package
		var err error
		if path == "unsafe" {
			tpkg = types.Unsafe
		} else {
			tpkg, err = types.DefaultImport(pkgMap, path)
			if err != nil {
				log.Fatal(err)
			}
		}
		tpkgs = append(tpkgs, tpkg)
		imp := Import{Path: path}
		importSet[imp] = true
		pkg := Package{
			Path: path,
			Name: tpkg.Name(),
		}
		pkgs = append(pkgs, pkg)
		// Store the local package name in pkgNames
		pkgNames[pkg.Name] = true
	}
	for i, tpkg := range tpkgs {
		for _, name := range tpkg.Scope().Names() {
			obj := tpkg.Scope().Lookup(name)
			if obj.Exported() {
				processObj(&pkgs[i], obj, pkgNames)
			}
		}
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

func processObj(pkg *Package, obj types.Object, pkgNames map[string]bool) {
	switch obj := obj.(type) {
	case *types.TypeName:
		if obj.Exported() {
			cts := fmt.Sprintf("scope.Lookup(%q).Type()", obj.Name())
			processType(pkg, obj.Type(), cts, "", false, pkgNames)
		}
	case *types.Func, *types.Var:
		processVar(pkg, obj, pkgNames)
	}
}

func addType(pkg *Package, typ types.Type, checkerTypeStr string, reflectTypeStr string, useReflectStr bool) {
	if visitedType(typ) {
		return
	}
	t := Type{
		CheckerType:      checkerTypeStr,
		TypeString:       interp.TypeString(typ),
		ReflectString:    reflectTypeStr,
		UseReflectString: useReflectStr,
	}
	pkg.AddType(typ, t)
}

func processType(pkg *Package, typ types.Type, checkerTypeStr string, reflectTypeStr string,
							useReflectString bool, pkgNames map[string]bool) {
	if visitedType(typ) {
		return
	}
	index := len(pkg.Types)

	// First, add the type itself
	addType(pkg, typ, checkerTypeStr, reflectTypeStr, useReflectString)

	// If it's a (nameless) channel type, add the other two directions
	switch typ := typ.(type) {
	case *types.Chan:
		// Add the other two directions of this channel type
		tBoth := types.NewChan(types.SendRecv, typ.Elem())
		tSend := types.NewChan(types.SendOnly, typ.Elem())
		tRecv := types.NewChan(types.RecvOnly, typ.Elem())
		chanTypes := []*types.Chan{tBoth, tSend, tRecv}
		typesDirs := []string{"types.SendRecv", "types.SendOnly", "types.RecvOnly"}
		reflectDirs := []string{"reflect.BothDir", "reflect.SendDir", "reflect.RecvDir"}
		for i, t := range chanTypes {
			if types.Identical(t, typ) {
				continue
			}
			tdir := typesDirs[i]
			rdir := reflectDirs[i]
			cts := fmt.Sprintf("types.NewChan(%s, t%d.(*types.Chan).Elem())", tdir, index)
			rts := fmt.Sprintf("reflect.ChanOf(%s, rt%d.Elem())", rdir, index)
			addType(pkg, t, cts, rts, true)
		}
	}

	// Recursively process components of the type
	undTyp := typ.Underlying()
	switch undTyp := undTyp.(type) {
	case *types.Array:
		// Process element type
		et := undTyp.Elem()
		ects := fmt.Sprintf("t%d.Underlying().(*types.Array).Elem()", index)
		erts := fmt.Sprintf("rt%d.Elem()", index)
		processType(pkg, et, ects, erts, true, pkgNames)
	case *types.Basic:
		// Nothing to do
	case *types.Chan:
		// Process element type
		et := undTyp.Elem()
		ects := fmt.Sprintf("t%d.Underlying().(*types.Chan).Elem()", index)
		erts := fmt.Sprintf("rt%d.Elem()", index)
		processType(pkg, et, ects, erts, true, pkgNames)
	case *types.Interface:
		// Nothing to do
	case *types.Map:
		// Process key and element types
		et := undTyp.Elem()
		ects := fmt.Sprintf("t%d.Underlying().(*types.Map).Elem()", index)
		erts := fmt.Sprintf("rt%d.Elem()", index)
		processType(pkg, et, ects, erts, true, pkgNames)

		kt := undTyp.Key()
		kcts := fmt.Sprintf("t%d.Underlying().(*types.Map).Key()", index)
		krts := fmt.Sprintf("rt%d.Key()", index)
		processType(pkg, kt, kcts, krts, true, pkgNames)
	case *types.Named:
		log.Fatal("what kind of type has an underlying type that's a *types.Named?!")
	case *types.Pointer:
		// Process element type
		et := undTyp.Elem()
		ects := fmt.Sprintf("t%d.Underlying().(*types.Pointer).Elem()", index)
		erts := fmt.Sprintf("rt%d.Elem()", index)
		processType(pkg, et, ects, erts, true, pkgNames)
	case *types.Signature:
		// Process parameter types and result types
		for i := 0; i < undTyp.Params().Len(); i++ {
			pt := undTyp.Params().At(i).Type()
			pcts := fmt.Sprintf("t%d.Underlying().(*types.Signature).Params().At(%d).Type()", index, i)
			prts := fmt.Sprintf("rt%d.In(%d)", index, i)
			processType(pkg, pt, pcts, prts, true, pkgNames)
		}
		for i := 0; i < undTyp.Results().Len(); i++ {
			rt := undTyp.Results().At(i).Type()
			rcts := fmt.Sprintf("t%d.Underlying().(*types.Signature).Results().At(%d).Type()", index, i)
			rrts := fmt.Sprintf("rt%d.Out(%d)", index, i)
			processType(pkg, rt, rcts, rrts, true, pkgNames)
		}
	case *types.Slice:
		// Process element type
		et := undTyp.Elem()
		ects := fmt.Sprintf("t%d.Underlying().(*types.Slice).Elem()", index)
		erts := fmt.Sprintf("rt%d.Elem()", index)
		processType(pkg, et, ects, erts, true, pkgNames)
	case *types.Struct:
		// Process exported field types
		for i := 0; i < undTyp.NumFields(); i++ {
			f := undTyp.Field(i)
			if f.Exported() {
				ft := undTyp.Field(i).Type()
				fcts := fmt.Sprintf("t%d.Underlying().(*types.Struct).Field(%d).Type()", index, i)
				frts := fmt.Sprintf("rt%d.Field(%d).Type", index, i)  // TODO: FIX THIS LINE
				processType(pkg, ft, fcts, frts, true, pkgNames)
			}
		}
	}

	// TODO: Add (or process?) the method type for each method in the type's method set
	// What does "the method type" mean?
	// Suppose we have the following definitions:
	//
	// type MyInt int
	//
	// func (n MyInt) Foo(x MyInt) MyInt {
	//     return n + x
	// }
	//
	// var n = MyInt(17)
	//
	// Then we can obtain a function type representing this method in a few ways:
	// 1) MyInt.Foo
	//    -> func(MyInt, MyInt) MyInt
	// 2) (*MyInt).Foo
	//    -> func(*MyInt, MyInt) MyInt
	// 3) n.Foo
	//    -> func(MyInt) MyInt
	// 4) (&n).Foo  (ditto)
	//    -> func(MyInt) MyInt
	//
	// Method expressions (1) and (2) are obtainable because the type is writable.
	// Method values (3) and (4) are obtainable because the type is obtainable.
	// Also note that (3) and (4) have the same type, so processing both in this way is redundant
	// but not harmful.
	//
	// The reflect package gives us a way to get most of these easily.
	// reflect.Type has "Method" and "MethodByName" methods that return (1) (or (2) if called on the ptr type).
	//   -> This is true unless typ is an interface type. If it's an interface type, then these
	//      methods on reflect.Type give the same type as (3) and (4). In that case, we can't get
	//      our hands on the method type as a reflect.Type to put in the typemap, but if we don't
	//      pick it up somewhere else (externally obtainable), we can simulate it easily with a closure.
	//
	// reflect.Value has "Method" and "MethodByName" methods that return (3) or (4).
	//

	// If underlying type is writable, add it, too
	// (we don't need to process it recursively because it has the same components as the current type,
	//  and either no methods for non-interface types or the same methods for interface types)
	if isWritable(undTyp, pkgNames) {
		// Since it's writable, we don't need to pass a reflectTypeStr
		cts := checkerTypeStr + ".Underlying()"
		addType(pkg, undTyp, cts, "", false)
	}
}

func processVar(pkg *Package, obj types.Object, pkgNames map[string]bool) {
	if !obj.Exported() {
		return
	}
	// Add an Object to pkg.Objects
	o := Object{
		Name:      obj.Name(),
		Qualified: pkg.Name + "." + obj.Name(),
	}
	pkg.Objects = append(pkg.Objects, o)

	if typ := obj.Type(); !visitedType(typ) {
		cts := fmt.Sprintf("scope.Lookup(%q).Type()", o.Name)
		rts := fmt.Sprintf("reflect.TypeOf(%s)", o.Qualified)
		processType(pkg, obj.Type(), cts, rts, true, pkgNames)
	}
}

func isWritable(typ types.Type, pkgNames map[string]bool) bool {
	return !hasUnexportedType(typ, pkgNames)
}

func hasUnexportedType(typ types.Type, pkgNames map[string]bool) bool {
	switch typ := typ.(type) {
	case *types.Array:
		return hasUnexportedType(typ.Elem(), pkgNames)
	case *types.Basic:
		return false
	case *types.Chan:
		return hasUnexportedType(typ.Elem(), pkgNames)
	case *types.Interface:
		// If I can write the embedded interfaces and the explicit methods, I can write the interface
		for i := 0; i < typ.NumEmbeddeds(); i++ {
			if hasUnexportedType(typ.Embedded(i), pkgNames) {
				return true
			}
		}
		for i := 0; i < typ.NumExplicitMethods(); i++ {
			if hasUnexportedType(typ.ExplicitMethod(i).Type(), pkgNames) {
				return true
			}
		}
		return false
	case *types.Map:
		return hasUnexportedType(typ.Key(), pkgNames) || hasUnexportedType(typ.Elem(), pkgNames)
	case *types.Named:
		pkg := typ.Obj().Pkg()
		return pkg != nil && (!pkgNames[pkg.Name()] || !typ.Obj().Exported())
	case *types.Pointer:
		return hasUnexportedType(typ.Elem(), pkgNames)
	case *types.Signature:
		// If I can write the parameters and the results, I can write the signature
		for i := 0; i < typ.Params().Len(); i++ {
			if hasUnexportedType(typ.Params().At(i).Type(), pkgNames) {
				return true
			}
		}
		for i := 0; i < typ.Results().Len(); i++ {
			if hasUnexportedType(typ.Results().At(i).Type(), pkgNames) {
				return true
			}
		}
		return false
	case *types.Slice:
		return hasUnexportedType(typ.Elem(), pkgNames)
	case *types.Struct:
		// If I can write the fields, I can write the struct
		for i := 0; i < typ.NumFields(); i++ {
			if hasUnexportedType(typ.Field(i).Type(), pkgNames) {
				return true
			}
		}
		return false
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
	// Silence errors about not using reflect package
	{{if .Packages}}_ = reflect.ValueOf{{end}}

	pkgMap := map[string]*types.Package{}
	typeMap := new(typeutil.Map)
	pkgs := []*interp.Package{}
{{range .Packages}}
	{
		{{if (eq "unsafe" .Path)}}tpkg := types.Unsafe
		_ = log.Fatal
		{{else}}tpkg, err := types.DefaultImport(pkgMap, {{printf "%q" .Path}})
		if err != nil {
			log.Fatal(err)
		}
		{{end}}scope := tpkg.Scope()
		_ = scope
		pkg := &interp.Package{
			Name: {{printf "%q" .Name}},
			Pkg:  tpkg,
			Objs: map[string]interp.Object{},
		}
		pkgs = append(pkgs, pkg)

	{{range $index, $typ := .Types}}
		t{{$index}} := {{$typ.CheckerType}}
		{{if (not $typ.UseReflectString)}}var pv{{$index}} *{{$typ.TypeString}}
		rt{{$index}} := reflect.TypeOf(pv{{$index}}).Elem()
		{{else}}rt{{$index}} := {{$typ.ReflectString}}
		{{end}}typeMap.Set(t{{$index}}, rt{{$index}})
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
