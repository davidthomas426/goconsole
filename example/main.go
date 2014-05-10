package main

import (
	"bufio"
	"fmt"
	"local/goconsole/goconsole"
	"local/trygotypes/p"
	"log"
	"os"
	"reflect"

	_ "code.google.com/p/go.tools/go/gcimporter"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

func setupTypeMap(typeMap *typeutil.Map) {
	// func(int) int
	var f func(int) int
	typeMap.Set(types.NewSignature(
		nil,
		nil,
		types.NewTuple(types.NewVar(0, nil, "x", types.Typ[types.Int])),
		types.NewTuple(types.NewVar(0, nil, "", types.Typ[types.Int])),
		false), reflect.TypeOf(f))

	// Named types
	var pmyint *p.MyInt
	pMyIntType := pPackageScope.Scope().LookupParent("MyInt").Type()
	pMyIntRtype := reflect.TypeOf(pmyint).Elem()
	typeMap.Set(pMyIntType, pMyIntRtype)

	var pmyfunc *p.MyFunc
	pMyFuncType := pPackageScope.Scope().LookupParent("MyFunc").Type()
	pMyFuncRtype := reflect.TypeOf(pmyfunc).Elem()
	typeMap.Set(pMyFuncType, pMyFuncRtype)
}

var fmtPackageScope *types.Package
var pPackageScope *types.Package

func main() {
	var err error
	pkgMap := map[string]*types.Package{}
	fmtPackageScope, err = types.DefaultImport(pkgMap, "fmt")
	if err != nil {
		log.Fatal(err)
	}
	pPackageScope, err = types.DefaultImport(pkgMap, "local/trygotypes/p")
	if err != nil {
		log.Fatal(err)
	}
	typeMap := new(typeutil.Map)
	setupTypeMap(typeMap)

	pkgs := []*goconsole.Package{
		&goconsole.Package{
			Name: "fmt",
			Pkg:  fmtPackageScope,
			Objs: map[string]goconsole.Object{
				"Println": goconsole.Object{
					Value: reflect.ValueOf(fmt.Println),
					Typ:   fmtPackageScope.Scope().Lookup("Println").Type(),
				},
				"Printf": goconsole.Object{
					Value: reflect.ValueOf(fmt.Printf),
					Typ:   fmtPackageScope.Scope().Lookup("Printf").Type(),
				},
			},
		},
		&goconsole.Package{
			Name: "p",
			Pkg:  pPackageScope,
			Objs: map[string]goconsole.Object{},
		},
	}

	interp := goconsole.NewInterpreter(pkgs, pkgMap, typeMap)
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
