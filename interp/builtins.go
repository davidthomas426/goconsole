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
		argObj := env.evalFuncArgs(callExpr.Args)[0]
		argObj.Value.(reflect.Value).Close()
	case "complex":
		log.Fatal("complex function not implemented yet")
	case "len":
		log.Fatal("len function not implemented yet")
	case "make":
		obj := env.evalMake(callExpr.Args)
		return []Object{obj}
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

func (env *environ) evalMake(argExprs []ast.Expr) Object {
	// TODO: not finished!
	typeExpr := argExprs[0]
	typ := env.info.Types[typeExpr].Type
	rtyp, sim := getReflectType(env.interp.typeMap, typ)
	if rtyp == nil {
		log.Fatal("Failed to get reflect.Type to make")
	}
	switch rtyp.Kind() {
	case reflect.Chan:
		buffer := 0
		if len(argExprs) > 1 {
			args := env.evalFuncArgs(argExprs[1:])
			buffer = int(args[0].Value.(reflect.Value).Int())
		}
		chanVal := reflect.MakeChan(rtyp, buffer)
		return Object{
			Value: chanVal,
			Typ:   typ,
			Sim:   sim,
		}
	case reflect.Map:
		// We are forced to ignore a length if given, since the reflect package
		// does not provide any way to specify it.
		mapVal := reflect.MakeMap(rtyp)
		return Object{
			Value: mapVal,
			Typ:   typ,
			Sim:   sim,
		}
	case reflect.Slice:
		args := env.evalFuncArgs(argExprs[1:])
		sliceLen := int(args[0].Value.(reflect.Value).Int())
		sliceCap := sliceLen
		if len(args) > 1 {
			sliceCap = int(args[1].Value.(reflect.Value).Int())
		}
		sliceVal := reflect.MakeSlice(rtyp, sliceLen, sliceCap)
		return Object{
			Value: sliceVal,
			Typ:   typ,
			Sim:   sim,
		}
	default:
		log.Fatal("make function called with unexpected type")
	}
	return Object{
		Value: nil,
		Typ:   typ,
		Sim:   sim,
	}
}
