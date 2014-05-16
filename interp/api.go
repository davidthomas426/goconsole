package interp

import (
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

type Interpreter interface {
	Run(src string) (bool, error)
}

func NewInterpreter(pkgs []*Package, pkgMap map[string]*types.Package, typeMap *typeutil.Map) Interpreter {
	return newInterp(pkgs, pkgMap, typeMap)
}

type Package struct {
	Name string
	Objs map[string]Object
	Pkg  *types.Package
}

func (pkg *Package) Lookup(s string) (Object, bool) {
	v, ok := pkg.Objs[s]
	return v, ok
}

type Object struct {
	Value interface{}
	Typ   types.Type
	Sim   bool
}
