package goconsole

import (
	"reflect"
	"strings"

	"code.google.com/p/go.tools/go/types"
)

type environ struct {
	interp *interp
	info   *types.Info
	scope  *types.Scope
	parent *environ
	objs   map[string]Object
}

func (env *environ) lookup(s string) (Object, bool) {
	v, ok := env.objs[s]
	return v, ok
}

func (env *environ) lookupParent(s string) (Object, bool) {
	if v, ok := env.objs[s]; ok {
		return v, true
	}
	if env.parent == nil {
		return Object{}, false
	}
	v, ok := env.parent.lookupParent(s)
	return v, ok
}

func (env *environ) addVar(varInfo *types.Var, typ reflect.Type, obj Object) {
	varName := varInfo.Name()
	varType := varInfo.Type()
	sim := false
	if typ == nil {
		// The caller did not provide a reflect.Type to use as the type
		typ, sim = getReflectType(env.interp.typeMap, varType)
	}
	// Create a variable of the right type with the zero value, then set its value from obj
	newVal := reflect.New(typ).Elem()
	rval := obj.Value.(reflect.Value)
	newVal.Set(rval)
	newObj := Object{
		Value: newVal,
		Typ:   varType,
		Sim:   sim,
	}
	env.objs[varName] = newObj
}

func (env *environ) dumpScope() (string, int) {
	lines := []string{}
	currNames := map[string]bool{}
	getLines := func(scope *types.Scope) {
		for _, name := range scope.Names() {
			if currNames[name] {
				continue
			}
			t := scope.Lookup(name)
			switch t.(type) {
			case *types.Var:
				currNames[name] = true
				lines = append(lines, "var "+name+" "+TypeString(t.Type()))
			}
		}
	}
	// Go up at most three levels
	for i, scope := 0, env.scope; i < 3 && scope != nil; i, scope = i+1, scope.Parent() {
		getLines(scope)
	}
	if len(lines) == 0 {
		return "", 0
	}
	return strings.Join(lines, ";") + ";", len(lines)
}
