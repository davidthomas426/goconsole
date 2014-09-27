package interp

import (
	"go/ast"
	"go/token"
	"reflect"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

// Assumes we want an Object wrapping a settable reflect.Value with the zero value
func getObjectOfType(typeMap *typeutil.Map, typ types.Type) Object {
	rtyp, sim := getReflectType(typeMap, typ)
	val := reflect.New(rtyp).Elem()
	return Object{
		Value: val,
		Typ:   typ,
		Sim:   sim,
	}
}

func createUnsimulatedFunc(env *environ, funcLit *ast.FuncLit, rtyp reflect.Type) reflect.Value {
	funcType := env.info.Types[funcLit].Type.(*types.Signature)
	funcScope := env.info.Scopes[funcLit.Type]
	funcParams := funcType.Params()
	funcResults := funcType.Results()

	closureEnv := environ{
		info:   env.info,
		interp: env.interp,
		scope:  env.scope,
		parent: env.parent,
		objs:   map[string]Object{},
	}
	vis := newVisitor(env, &closureEnv, funcLit)
	ast.Walk(vis, funcLit)
	funcVal := func(in []reflect.Value) (results []reflect.Value) {
		// 1) Create new environment that "inherits" from closureEnv
		funcEnv := &environ{
			info:   closureEnv.info,
			interp: closureEnv.interp,
			scope:  funcScope,
			parent: &closureEnv,
			objs:   map[string]Object{},
		}

		// 2) Add parameters to environment with values from `in`
		for i := 0; i < funcParams.Len(); i++ {
			// Add variable to environment for this param
			param := funcParams.At(i)
			obj := Object{
				Value: in[i],
				Typ:   param.Type(),
				Sim:   false, // parameters to unsimulated function type must be unsimulated
			}
			funcEnv.addVar(param, nil, obj)
		}

		// 3) If we have named result parameters, add those to the environment with zero value
		// TODO: add named results
		var resultObjs []Object
		if funcResults.Len() > 0 {
			// TODO: I should probably stick with a []Object so named results will be easier
			//   Then I can copy the reflect.Value out of each result at the end into a slice
			//   to actually return from the function.
			resultObjs = make([]Object, funcResults.Len())
			results = make([]reflect.Value, funcResults.Len())
			for i, _ := range results {
				resultObjs[i] = getObjectOfType(funcEnv.interp.typeMap, funcResults.At(i).Type())
				results[i] = resultObjs[i].Value.(reflect.Value)
			}

		}

		// 4) Evaluate the body of the function (topLevel=false)
		//     Note: If results are returned, handle them
		stmtRes := funcEnv.runStmt(funcLit.Body, "", false)
		if res, ok := stmtRes.(returnResult); ok {
			for i, resObj := range res {
				assignObj(resultObjs[i], resObj)
			}
		}
		return
	}
	return reflect.MakeFunc(rtyp, funcVal)
}

func createSimulatedFunc(env *environ, funcLit *ast.FuncLit) func([]Object) []Object {
	funcType := env.info.Types[funcLit].Type.(*types.Signature)
	funcScope := env.info.Scopes[funcLit.Type]
	funcParams := funcType.Params()
	funcResults := funcType.Results()

	// Make an environment to hold the variables this function closes over.
	// We'll use this as the parent environment of calls instead of env.
	// That way, if the user rebinds the names of variables that this function
	// closes over, the function will continue referencing the old variables.
	closureEnv := environ{
		info:   env.info,
		interp: env.interp,
		scope:  env.scope,
		parent: env.parent,
		objs:   map[string]Object{},
	}
	vis := newVisitor(env, &closureEnv, funcLit)
	ast.Walk(vis, funcLit)
	return func(in []Object) (results []Object) {
		// 1) Create new environment that "inherits" from closureEnv
		funcEnv := &environ{
			info:   closureEnv.info,
			interp: closureEnv.interp,
			scope:  funcScope,
			parent: &closureEnv,
			objs:   map[string]Object{},
		}

		// 2) Add parameters to environment with values from `in`
		for i := 0; i < funcParams.Len(); i++ {
			// Add variable to environment for this param
			param := funcParams.At(i)
			funcEnv.addVar(param, nil, in[i])
		}

		// 3) If we have named result parameters, add those to the environment with zero value
		// TODO: add named results
		if funcResults.Len() > 0 {
			results = make([]Object, funcResults.Len())
			for i, _ := range results {
				// Make an Object of the right type with the zero value.
				// On return with values, we will assign given values to these Objects
				results[i] = getObjectOfType(funcEnv.interp.typeMap, funcResults.At(i).Type())
			}
		}

		// 4) Evaluate the body of the function (topLevel=false)
		//     Note: If results are returned, handle them
		stmtRes := funcEnv.runStmt(funcLit.Body, "", false)
		if res, ok := stmtRes.(returnResult); ok {
			for i, resObj := range res {
				assignObj(results[i], resObj)
			}
		}
		return
	}
}

type visitor struct {
	oldEnv, newEnv *environ
	begin, end     token.Pos
}

func (v *visitor) Visit(node ast.Node) (w ast.Visitor) {
	if id, ok := node.(*ast.Ident); ok {
		if obj, ok := v.oldEnv.info.Uses[id]; ok {
			idPos := obj.Pos()
			if idPos == 0 {
				// Identifier is defined outside this package, so we don't care.
				return nil
			}
			// Check for PkgName, as these don't count as "closed over"
			if _, isPkgName := obj.(*types.PkgName); isPkgName {
				// We're done visiting this node
				return nil
			}
			if !(idPos > v.begin && idPos < v.end) {
				// Add the closed-over variable to newEnv
				if val, ok := v.oldEnv.lookupParent(id.Name); ok {
					v.newEnv.objs[id.Name] = val
				}
			}
		}
	}
	return v
}

func newVisitor(oldEnv, newEnv *environ, funcLit *ast.FuncLit) ast.Visitor {
	return &visitor{oldEnv: oldEnv, newEnv: newEnv, begin: funcLit.Pos(), end: funcLit.End()}
}
