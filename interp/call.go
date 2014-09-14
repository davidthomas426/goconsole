package interp

import (
	"go/ast"
	"reflect"

	"code.google.com/p/go.tools/go/types"
)

type callExprKind int

const (
	callKind       callExprKind = iota // The expression is a function or method call
	builtinKind                        // The expression is a builtin call
	conversionKind                     // The expression is a conversion
)

func (env *environ) getCallExprKind(callExpr *ast.CallExpr) callExprKind {
	kindFromObj := func(obj types.Object) callExprKind {
		switch obj.(type) {
		case *types.Builtin:
			return builtinKind
		case *types.TypeName:
			return conversionKind
		}
		return callKind
	}

	var kindFromSubExpr func(e ast.Expr) callExprKind
	kindFromSubExpr = func(e ast.Expr) callExprKind {
		switch e := e.(type) {
		case *ast.Ident:
			obj := env.info.Uses[e]
			return kindFromObj(obj)
		case *ast.SelectorExpr:
			obj := env.info.Uses[e.Sel]
			return kindFromObj(obj)
		case *ast.ArrayType, *ast.ChanType, *ast.InterfaceType,
			*ast.FuncType, *ast.MapType, *ast.StructType:
			return conversionKind
		case *ast.ParenExpr:
			return kindFromSubExpr(e.X)
		case *ast.StarExpr:
			return kindFromSubExpr(e.X)
		}
		return callKind
	}
	return kindFromSubExpr(callExpr.Fun)
}

func (env *environ) evalFuncArgs(argExprs []ast.Expr) []Object {
	return env.evalExprs(argExprs)
}

// callFunWithObjs calls the given function on the arguments given as a slice of Object.
// It first converts the arguments to a slice of reflect.Value. It assumes that any Object
// in the given slice whose Value field is not a reflect.Value is an untyped nil, which
// should always be true in practice.
func callFunWithObjs(fun reflect.Value, argObjs []Object) []reflect.Value {
	argVals := make([]reflect.Value, len(argObjs))
	funType := fun.Type()
	for i, argObj := range argObjs {
		argVal, ok := argObj.Value.(reflect.Value)
		if !ok {
			// Must be untyped nil. Use zero value of type instead
			var rtyp reflect.Type
			if funType.IsVariadic() && i == funType.NumIn()-1 {
				rtyp = funType.In(i).Elem()
			} else {
				rtyp = fun.Type().In(i)
			}
			argVal = reflect.Zero(rtyp)
		}
		argVals[i] = argVal
	}
	return fun.Call(argVals)
}

func (env *environ) evalFuncCall(callExpr *ast.CallExpr, async bool) []Object {
	funObj := env.Eval(callExpr.Fun)[0]
	fun := funObj.Value.(reflect.Value)

	argObjs := env.evalFuncArgs(callExpr.Args)
	if funObj.Sim {
		// Call by actually calling it
		funVal := fun.Interface().(func([]Object) []Object)
		if !async {
			results := funVal(argObjs)
			return results
		} else {
			go funVal(argObjs)
			return nil
		}
	} else {
		// Now call the function on the args
		if !async {
			resultVals := callFunWithObjs(fun, argObjs)

			// Wrap the output values in Objects
			results := make([]Object, len(resultVals))
			for i, resVal := range resultVals {
				results[i] = Object{
					Value: resVal,
					Typ:   funObj.Typ.Underlying().(*types.Signature).Results().At(i).Type(),
				}
			}
			return results
		} else {
			go callFunWithObjs(fun, argObjs)
			return nil
		}

	}
}
