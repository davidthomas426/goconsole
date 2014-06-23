package interp

import (
	"go/ast"

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
