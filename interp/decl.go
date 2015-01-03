package interp

import (
	"go/ast"
	"log"
	"reflect"
)

func getSettableZeroVal(typ reflect.Type) reflect.Value {
	if typ.Kind() != reflect.Array {
		return reflect.New(typ).Elem()
	}
	log.Fatal("getSettableZeroVal: array types not implemented yet")
	return reflect.Value{}
}

func (env *environ) getDeclVars(exprs []ast.Expr) []Object {
	lhs := make([]Object, len(exprs))
	for i, expr := range exprs {
		ident := expr.(*ast.Ident)
		identDef := env.info.Defs[ident]
		if identDef == nil || identDef.Pos() != ident.Pos() {
			// Redeclaration: variable already exists in current scope. Look up the object.
			obj, _ := env.lookup(ident.Name)
			lhs[i] = obj
		} else {
			// New variable declaration. Create new variable with the right type.
			typ := env.info.TypeOf(expr)
			rtyp, sim := getReflectType(env.interp.typeMap, typ)
			if rtyp == nil {
				log.Fatalf("couldn't get reflect.Type corresponding to %q", typ)
			}
			val := getSettableZeroVal(rtyp)
			obj := Object{
				Value: val,
				Typ:   typ,
				Sim:   sim,
			}
			// Add the name we're declaring to env.names if it's a new name
			if _, ok := env.lookup(ident.Name); !ok {
				env.names = append(env.names, ident.Name)
			}
			// Add the object to env.objs
			env.objs[ident.Name] = obj
			lhs[i] = obj
		}
	}
	return lhs
}
