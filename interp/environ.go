package interp

import (
	"reflect"
	"strings"

	"golang.org/x/tools/go/types"
)

type environ struct {
	interp *interp
	info   *types.Info
	scope  *types.Scope
	parent *environ
	objs   map[string]Object
	names  []string
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
	rval, ok := obj.Value.(reflect.Value)
	if !ok {
		// Must be untyped nil. Use zero value of type instead
		rval = reflect.Zero(typ)
	}
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
	for _, name := range env.names {
		_, t := env.scope.LookupParent(name)
		switch t.(type) {
		case *types.Var:
			lines = append(lines, "var "+name+" "+TypeString(t.Type()))
		}
	}
	if len(lines) == 0 {
		return "", 0
	}
	return strings.Join(lines, ";") + ";", len(lines)
}
