package interp

import (
	"fmt"
	"go/ast"
	"log"
	"reflect"
)

// TODO: make() built-in
//   * chan types
//   * slice types
//   * map types

func (env *environ) evalBuiltinCall(callExpr *ast.CallExpr, async bool) []Object {
	// TODO: implement builtins
	builtinName := callExpr.Fun.(*ast.Ident).Name
	var results []Object
	switch builtinName {
	case "append":
		log.Fatal("append function not implemented yet")
	case "cap":
		log.Fatal("cap function not implemented yet")
	case "close":
		log.Fatal("close function not implemented yet")
	case "complex":
		log.Fatal("complex function not implemented yet")
	case "len":
		log.Fatal("len function not implemented yet")
	case "make":
		log.Fatal("make function not implemented yet")
	case "new":
		log.Fatal("new function not implemented yet")
	case "panic":
		log.Fatal("panic function not implemented yet")
	case "print":
		// Just forward to fmt.Print
		fun := reflect.ValueOf(fmt.Print)
		argObjs := env.evalFuncArgs(callExpr.Args)
		if async {
			go callFunWithObjs(fun, argObjs)
		} else {
			callFunWithObjs(fun, argObjs)
		}
	case "println":
		// Just forward to fmt.Println
		fun := reflect.ValueOf(fmt.Println)
		argObjs := env.evalFuncArgs(callExpr.Args)
		if async {
			go callFunWithObjs(fun, argObjs)
		} else {
			callFunWithObjs(fun, argObjs)
		}
	case "real":
		log.Fatal("real function not implemented yet")
	case "recover":
		log.Fatal("recover function not implemented yet")
	}
	return results
}
