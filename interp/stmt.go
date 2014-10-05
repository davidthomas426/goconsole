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
		// First, get LHS
		var lhs []Object
		var mapIndexExprs map[int]bool
		switch stmt.Tok {
		case token.DEFINE:
			// Short variable declaration
			lhs = env.getDeclVars(stmt.Lhs)
		case token.ASSIGN:
			lhs, mapIndexExprs = env.getAssignmentLhs(stmt.Lhs)
		}

		// Second, evaluate RHS
		rhs := env.evalExprs(stmt.Rhs)

		// Finally, do the assignment
		for i, _ := range lhs {
			if mapIndexExprs[i] {
				env.assignMapIndex(stmt.Lhs[i], rhs[i])
			} else {
				assignObj(lhs[i], rhs[i])
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
	case *ast.SelectStmt:
		// Need to set up data structures to pass to env.runSelect
		clauses := stmt.Body.List
		cases := make([]reflect.SelectCase, len(clauses))
		ctxs := make([]selectCaseContext, len(clauses))
		for i, clause := range clauses {
			clause := clause.(*ast.CommClause)
			ctxs[i].stmts = clause.Body
			switch commStmt := clause.Comm.(type) {
			case nil:
				cases[i].Dir = reflect.SelectDefault
			case *ast.SendStmt:
				cases[i].Dir = reflect.SelectSend
				chanObj := env.Eval(commStmt.Chan)[0]
				sendObj := env.Eval(commStmt.Value)[0]
				cases[i].Chan = chanObj.Value.(reflect.Value)
				switch sendVal := sendObj.Value.(type) {
				case reflect.Value:
					cases[i].Send = sendVal
				default:
					// Must be untyped nil
					fmt.Println("sent untyped nil")
					elemTyp := chanObj.Typ.Underlying().(*types.Chan).Elem()
					rTyp, _ := getReflectType(env.interp.typeMap, elemTyp)
					if rTyp == nil {
						log.Fatal("Failed to obtain reflect.Type to represent type:", elemTyp)
					}
					cases[i].Send = reflect.Zero(rTyp)
				}
			default:
				cases[i].Dir = reflect.SelectRecv

				// Extract the recv expression and set lhs and tok in the context if applicable.
				var recvExpr ast.Expr
				switch stmt := commStmt.(type) {
				case *ast.ExprStmt:
					recvExpr = stmt.X
				case *ast.AssignStmt:
					ctxs[i].lhs = stmt.Lhs
					ctxs[i].tok = stmt.Tok
					recvExpr = stmt.Rhs[0] // Must only be one, from spec
				}
				// Drill down into paren exprs
			removeParens:
				for {
					switch expr := recvExpr.(type) {
					case *ast.ParenExpr:
						// Remove parens
						recvExpr = expr.X
					default:
						// If it's not a ParenExpr, we're done
						break removeParens
					}
				}

				// Evaluate the channel operand of the receive expression
				chanExpr := recvExpr.(*ast.UnaryExpr).X
				chanObj := env.Eval(chanExpr)[0]
				cases[i].Chan = chanObj.Value.(reflect.Value)
			}
		}
		env.runSelect(clauses, cases, ctxs)
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
