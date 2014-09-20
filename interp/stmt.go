package interp

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
)

type stmtResult interface {
	stmtResult() // Marker method
}

type returnResult []Object
type breakResult string
type continueResult string

func (r returnResult) stmtResult()   {}
func (r breakResult) stmtResult()    {}
func (r continueResult) stmtResult() {}

func (env *environ) runStmt(stmt ast.Stmt, label string, topLevel bool) stmtResult {
	switch stmt := stmt.(type) {
	case *ast.ReturnStmt:
		if topLevel {
			log.Fatal("Return from top-level not allowed")
		}
		resObjs := env.evalExprs(stmt.Results)
		return returnResult(resObjs)
	case *ast.BranchStmt:
		label := ""
		if stmt.Label != nil {
			label = stmt.Label.Name
		}
		switch stmt.Tok {
		case token.BREAK:
			return breakResult(label)
		case token.CONTINUE:
			return continueResult(label)
		case token.GOTO:
			log.Fatal("Goto statements not implemented")
		case token.FALLTHROUGH:
			log.Fatal("Fallthrough statements not implemented")
		}
	case *ast.AssignStmt:
		switch stmt.Tok {
		case token.DEFINE:
			// Short variable declaration
			// Collect LHS identifiers
			idents := make([]*ast.Ident, len(stmt.Lhs))
			for i, lhs := range stmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					// LHS is identifier
					idents[i] = ident
				} else {
					log.Fatal("short declaration LHS not identifier!")
				}
			}
			// Evaluate RHS expressions
			var rhs []Object
			if len(stmt.Rhs) == 1 {
				// Single rhs expression, potentially multi-valued
				rhs = env.Eval(stmt.Rhs[0])
			} else {
				// Multiple rhs expressions, each single-valued
				rhs = make([]Object, len(stmt.Lhs))
				for i, expr := range stmt.Rhs {
					rhs[i] = env.Eval(expr)[0]
				}
			}
			// Create LHS variables and set them to RHS
			for i, ident := range idents {
				obj := rhs[i]
				rval := obj.Value.(reflect.Value)
				identDef := env.info.Defs[ident]
				if identDef == nil || identDef.Pos() != ident.Pos() {
					// Redeclaration: variable already exists in current scope. Just assign the new value.
					lhs, _ := env.lookup(ident.Name)
					lval := lhs.Value.(reflect.Value)
					lval.Set(rval)
				} else {
					// New variable declaration. Create new variable with the right value.
					typ := rval.Type()
					sv := reflect.New(typ).Elem()
					sv.Set(rval)
					obj.Value = sv
					// Add the name we're declaring to env.names if it's a new name
					if _, ok := env.lookup(ident.Name); !ok {
						env.names = append(env.names, ident.Name)
					}
					// Add the object to env.objs
					env.objs[ident.Name] = obj
				}
			}
		case token.ASSIGN:
			// assignment
			// TODO: this only covers very simple assignment. There are more complicated rules
			//   not yet implemented (see http://golang.org/ref/spec#Assignments), such as:
			//   1) nil
			//   2) Blank identifier
			lhs := make([]Object, len(stmt.Lhs))
			isMapIndexExpr := make([]bool, len(stmt.Lhs))
			rhs := make([]reflect.Value, len(stmt.Lhs))
			for i, expr := range stmt.Lhs {
				if e, isIndexExpr := expr.(*ast.IndexExpr); isIndexExpr {
					if _, isMap := env.info.TypeOf(e.X).Underlying().(*types.Map); isMap {
						isMapIndexExpr[i] = true
						continue
					}
				}
				lhs[i] = env.Eval(expr)[0]
			}
			if len(stmt.Rhs) == 1 {
				// Single rhs expression, potentially multi-valued
				robjs := env.Eval(stmt.Rhs[0])
				for i, robj := range robjs {
					rv, ok := robj.Value.(reflect.Value)
					if !ok {
						// This means that rval is untyped.
						// In assignment context, this should only happen if it's untyped nil
						rhs[i] = reflect.ValueOf(nil)
					}
					rhs[i] = rv
				}
			} else {
				// Multiple rhs expressions, each single-valued
				for i, expr := range stmt.Rhs {
					robj := env.Eval(expr)[0]
					rv, ok := robj.Value.(reflect.Value)
					if !ok {
						rhs[i] = reflect.ValueOf(nil)
					}
					rhs[i] = rv
				}
			}

			// Set the lhs objects' values with results of rhs expressions
			for i, obj := range lhs {
				if isMapIndexExpr[i] {
					indexExpr := stmt.Lhs[i].(*ast.IndexExpr)
					mapObj := env.Eval(indexExpr.X)[0]
					keyObj := env.Eval(indexExpr.Index)[0]

					mapVal := mapObj.Value.(reflect.Value)
					keyVal := keyObj.Value.(reflect.Value)

					if rhs[i].IsValid() {
						mapVal.SetMapIndex(keyVal, rhs[i])
					} else {
						// Must be untyped nil
						elemTyp := mapObj.Typ.(*types.Map).Elem()
						rTyp, _ := getReflectType(env.interp.typeMap, elemTyp)
						if rTyp == nil {
							log.Fatal("Failed to obtain reflect.Type to represent type:", elemTyp)
						}
						mapVal.SetMapIndex(keyVal, reflect.Zero(rTyp))
					}
				} else {
					v := obj.Value.(reflect.Value)
					if rhs[i].IsValid() {
						v.Set(rhs[i])
					} else {
						// Must be untyped nil
						v.Set(reflect.Zero(v.Type()))
					}
				}
			}
		}
	case *ast.ExprStmt:
		// If we're not at top level, then only call expressions and receive operations are valid statements
		if !topLevel {
			if _, ok := stmt.X.(*ast.CallExpr); !ok {
				// TODO: handle this error better
				log.Fatal("Expression used as statement inappropriately")
			}
			// TODO: what about receive operations?
		}
		objs := env.Eval(stmt.X)
		if topLevel {
			for _, obj := range objs {
				// TODO: do something better than print the results to stdout
				switch v := obj.Value.(type) {
				case reflect.Value:
					fmt.Printf("=> %s: %v\n", TypeString(obj.Typ), v.Interface())
				case exact.Value, nil:
					fmt.Printf("=> %s: %v\n", TypeString(obj.Typ), v)
				}
			}
		}
	case *ast.GoStmt:
		callKind := env.getCallExprKind(stmt.Call)
		if callKind == builtinKind {
			env.evalBuiltinCall(stmt.Call, true)
		} else {
			env.evalFuncCall(stmt.Call, true)
		}
	case *ast.SendStmt:
		chanObj := env.Eval(stmt.Chan)[0]
		sentObj := env.Eval(stmt.Value)[0]
		chanVal := chanObj.Value.(reflect.Value)
		sentVal := sentObj.Value.(reflect.Value)
		chanVal.Send(sentVal)
	case *ast.ForStmt:
		// Set up scope and environment for the for statement
		forScope := env.scope
		forClauseEnv := env
		if stmt.Init != nil {
			forScope = env.info.Scopes[stmt]
			forClauseEnv = &environ{
				info:   env.info,
				interp: env.interp,
				scope:  forScope,
				parent: env,
				objs:   map[string]Object{},
			}
			forClauseEnv.runStmt(stmt.Init, "", false)
		}
		for {
			if stmt.Cond != nil {
				condObj := forClauseEnv.Eval(stmt.Cond)[0]
				if !condObj.Value.(reflect.Value).Bool() {
					break
				}
			}
			if stmtRes := forClauseEnv.runStmt(stmt.Body, "", false); stmtRes != nil {
				switch stmtRes := stmtRes.(type) {
				case breakResult:
					if string(stmtRes) == "" || string(stmtRes) == label {
						return nil
					}
				case continueResult:
					if string(stmtRes) == "" || string(stmtRes) == label {
						if stmt.Post != nil {
							forClauseEnv.runStmt(stmt.Post, "", false)
						}
						continue
					}
				}
				return stmtRes
			}
			if stmt.Post != nil {
				forClauseEnv.runStmt(stmt.Post, "", false)
			}
		}
	case *ast.IfStmt:
		// Set up scope and environment for the for statement
		ifScope := env.scope
		ifClauseEnv := env
		if stmt.Init != nil {
			ifScope = env.info.Scopes[stmt]
			ifClauseEnv = &environ{
				info:   env.info,
				interp: env.interp,
				scope:  ifScope,
				parent: env,
				objs:   map[string]Object{},
			}
			ifClauseEnv.runStmt(stmt.Init, "", false)
		}
		var stmtRes stmtResult
		condObj := ifClauseEnv.Eval(stmt.Cond)[0]
		cond := false
		if condVal, ok := condObj.Value.(reflect.Value); ok {
			cond = condVal.Bool()
		} else {
			ev := condObj.Value.(exact.Value)
			cond = exact.BoolVal(ev)
		}
		if cond {
			stmtRes = ifClauseEnv.runStmt(stmt.Body, "", false)
		} else {
			if stmt.Else != nil {
				stmtRes = ifClauseEnv.runStmt(stmt.Else, "", false)
			}
		}
		return stmtRes
	case *ast.BlockStmt:
		blockScope := env.info.Scopes[stmt]
		blockEnv := &environ{
			info:   env.info,
			interp: env.interp,
			scope:  blockScope,
			parent: env,
			objs:   map[string]Object{},
		}
		for _, st := range stmt.List {
			if stmtRes := blockEnv.runStmt(st, "", false); stmtRes != nil {
				return stmtRes
			}
		}
	default:
		log.Fatalf("Unhandled statement type: %T", stmt)
	}
	return nil
}
