package interp

import (
	"go/ast"
	"log"
)

// TODO: make() built-in
//   * chan types
//   * slice types
//   * map types

func (env *environ) evalBuiltinCall(callExpr *ast.CallExpr, async bool) []Object {
	// TODO: implement builtins
	log.Fatal("Builtins not implemented yet")
	return nil
}
