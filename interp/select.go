package interp

import (
	"go/ast"
	"go/token"
	"log"
	"reflect"
)

// reflect.Select takes a []reflect.SelectCase, which includes a channel, a direction,
// the value to send (if sending), and does the select. It returns the index of the
// chosen case and the two results of the receive (if a receive case was chosen).

// We'll keep another slice of select case contexts, encapsulating the stmtlist associated
// with each case (which can be length 0). In the receive case, the context must also include
// information about what to do with the receive results:
//  * short decl, assignment, or neither?
//  * zero, one, or two expressions to declare or assign to

type selectCaseContext struct {
	lhs   []ast.Expr  // left-hand side expressions of assignment in recv stmt (nil if not applicable)
	tok   token.Token // DEFINE or ASSIGN in recv stmt (ILLEGAL if not applicable)
	stmts []ast.Stmt  // statement list to execute if case is chosen
}

func (env *environ) runSelect(clauses []ast.Stmt, cases []reflect.SelectCase, ctxs []selectCaseContext) stmtResult {
	chosen, recv, recvOK := reflect.Select(cases)
	ctx := ctxs[chosen]
	clause := clauses[chosen]

	// Create new environment that "inherits" from env
	caseEnv := &environ{
		info:   env.info,
		interp: env.interp,
		scope:  env.info.Scopes[clause],
		parent: env,
		objs:   map[string]Object{},
	}

	if cases[chosen].Dir == reflect.SelectRecv {
		var lhs []Object
		var mapIndexExprs map[int]bool
		switch ctx.tok {
		// Handle  short decl or assignment, if it exists (only possible in recv)
		case token.DEFINE:
			lhs = caseEnv.getDeclVars(ctx.lhs)
		case token.ASSIGN:
			lhs, mapIndexExprs = caseEnv.getAssignmentLhs(ctx.lhs)
		}

		// Get the RHS
		rhs := []Object{{Value: recv}, {Value: reflect.ValueOf(recvOK)}} // Typ and Sim don't matter

		// Do the assignment
		for i, _ := range lhs {
			if mapIndexExprs[i] {
				env.assignMapIndex(ctx.lhs[i], rhs[i])
			} else {
				assignObj(lhs[i], rhs[i])
			}
		}
	}
	// In any case, run the statement list
	for _, stmt := range ctx.stmts {
		// TODO: Handle label
		stmtRes := caseEnv.runStmt(stmt, "", false)
		if stmtRes != nil {
			// TODO: Handle stmtRes
			log.Fatal("return/break from select statement not yet implemented")
		}
	}
	return nil
}
