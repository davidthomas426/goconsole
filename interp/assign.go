package interp

import (
	"go/ast"
	"log"
	"reflect"

	"golang.org/x/tools/go/types"
)

// TODO: this only covers very simple assignment. There are more complicated rules
//   not yet implemented (see http://golang.org/ref/spec#Assignments), such as:
//   1) nil
//   2) Blank identifier

func (env *environ) getAssignmentLhs(exprs []ast.Expr) ([]Object, map[int]bool) {
	objs := make([]Object, len(exprs))
	mapIndexExprs := make(map[int]bool)
	for i, expr := range exprs {
		if isMapIndexExpr(env, expr) {
			mapIndexExprs[i] = true
		}
		objs[i] = env.Eval(expr)[0]
	}
	return objs, mapIndexExprs
}

// assignObj assigns value of rObj to lObj.
// lObj.Value must be a settable reflect.Value.
// rObj.Value must be a reflect.Value unless it represents untyped nil.
func assignObj(lObj, rObj Object) {
	lVal := lObj.Value.(reflect.Value)
	switch rVal := rObj.Value.(type) {
	case reflect.Value:
		lVal.Set(rVal)
	default:
		// Must be untyped nil
		lVal.Set(reflect.Zero(lVal.Type()))
	}
}

func (env *environ) assignMapIndex(expr ast.Expr, rObj Object) {
	indexExpr := expr.(*ast.IndexExpr)
	mapObj := env.Eval(indexExpr.X)[0]
	keyObj := env.Eval(indexExpr.Index)[0]

	mapVal := mapObj.Value.(reflect.Value)
	keyVal := keyObj.Value.(reflect.Value)

	rVal, ok := rObj.Value.(reflect.Value)
	if ok {
		mapVal.SetMapIndex(keyVal, rVal)
	} else {
		// Must be untyped nil
		elemTyp := mapObj.Typ.(*types.Map).Elem()
		rTyp, _ := getReflectType(env.interp.typeMap, elemTyp)
		if rTyp == nil {
			log.Fatal("Failed to obtain reflect.Type to represent type:", elemTyp)
		}
		mapVal.SetMapIndex(keyVal, reflect.Zero(rTyp))
	}
}
