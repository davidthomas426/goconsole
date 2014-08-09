package interp

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"code.google.com/p/go.tools/go/exact"
)

func (env *environ) runStmt(stmt ast.Stmt, topLevel bool) {
	switch stmt := stmt.(type) {
	case *ast.ReturnStmt:
		if topLevel {
			log.Fatal("Return from top-level not allowed")
		}
		log.Fatal("Return statements not implemented yet")
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
			rhs := make([]reflect.Value, len(stmt.Lhs))
			for i, expr := range stmt.Lhs {
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

			for i, obj := range lhs {
				v := obj.Value.(reflect.Value)
				if rhs[i].IsValid() {
					v.Set(rhs[i])
				} else {
					v.Set(reflect.Zero(v.Type()))
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
		env.evalFuncCall(stmt.Call, true)
	}
}
