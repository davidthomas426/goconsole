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

func (env *environ) evalFuncCall(callExpr *ast.CallExpr, async bool) []Object {
	funObj := env.Eval(callExpr.Fun)[0]
	fun := funObj.Value.(reflect.Value)

	if funObj.Sim {
		var args []Object
		if len(callExpr.Args) == 1 {
			// Single argument expression, potentially multi-valued
			args = env.Eval(callExpr.Args[0])
		} else {
			// Multiple argument expressions, each single-valued
			args = make([]Object, len(callExpr.Args))
			for i, argExpr := range callExpr.Args {
				args[i] = env.Eval(argExpr)[0]
			}
		}

		// Call by actually calling it
		funVal := fun.Interface().(func([]Object) []Object)
		if !async {
			results := funVal(args)
			return results
		} else {
			go funVal(args)
			return nil
		}
	} else {
		var args []reflect.Value
		if len(callExpr.Args) == 1 {
			// Single argument expression, potentially multi-valued
			vals := env.Eval(callExpr.Args[0])
			args = make([]reflect.Value, len(vals))
			for i, val := range vals {
				args[i] = val.Value.(reflect.Value)
			}
		} else {
			// Multiple argument expressions, each single-valued
			args = make([]reflect.Value, len(callExpr.Args))
			for i, argExpr := range callExpr.Args {
				argObj := env.Eval(argExpr)[0]

				rv, ok := argObj.Value.(reflect.Value)
				if !ok {
					// Must be passing untyped nil
					// Use zero value of type instead
					rtyp := fun.Type().In(i)
					rv = reflect.Zero(rtyp)
				}
				args[i] = rv
			}
		}

		if !async {
			resultVals := fun.Call(args)

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
			go fun.Call(args)
			return nil
		}

	}
}
