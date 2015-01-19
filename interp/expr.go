package interp

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"golang.org/x/tools/go/exact"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/go/types/typeutil"
)

func (env *environ) evalExprs(exprs []ast.Expr) []Object {
	var objs []Object
	if len(exprs) == 1 {
		// Single argument expression, potentially multi-valued
		objs = env.Eval(exprs[0])
	} else {
		// Multiple argument expressions, each single-valued
		objs = make([]Object, len(exprs))
		for i, expr := range exprs {
			objs[i] = env.Eval(expr)[0]
		}
	}
	return objs
}

func isMapIndexExpr(env *environ, expr ast.Expr) bool {
	if e, isIndexExpr := expr.(*ast.IndexExpr); isIndexExpr {
		if _, isMap := env.info.TypeOf(e.X).Underlying().(*types.Map); isMap {
			return true
		}
	}
	return false
}

func (env *environ) Eval(expr ast.Expr) []Object {
	// Check for constant
	tv := env.info.Types[expr]
	if tv.Type == types.Typ[types.UntypedNil] {
		obj := Object{
			Value: tv.Value,
			Typ:   tv.Type,
		}
		return []Object{obj}
	}
	if tv.Value != nil {
		// It's a constant. Just return the exact.Value
		// Note: We actually want to convert it to a reflect.Value if it's a typed constant, since
		//   we know we can represent it and for convenience. But we're not doing that yet.
		if isTyped(tv.Type) {
			// It's a typed constant. Convert to a reflect.Value.
			obj := Object{
				Value: convertExactToReflect(env.interp.typeMap, tv),
				Typ:   tv.Type,
			}
			return []Object{obj}
		}
		// It's an untyped constant. Just return the exact.Value as the value.
		obj := Object{
			Value: tv.Value,
			Typ:   tv.Type,
		}
		return []Object{obj}
	}
	// Not a constant expression, so we have to evaluate it ourselves
	typ := tv.Type
	switch e := expr.(type) {
	case *ast.FuncLit:
		// TODO: Simulated functions, to interact with each other correctly inside the
		// interpreter, should not be "func([]reflect.Value)[]reflect.Value" but instead
		// "func([]Object)[]Object". This is because a simulated function's arguments may
		// themselves be simulated functions! To handle this case correctly, the implementation
		// of the function's body needs to know which arguments are simulated, which means
		// the arguments must be of type Object rather than reflect.Value.
		//
		// Unsimulated functions do not have this problem as long as we guarantee that an
		// unsimulated function type's parameter types will also be unsimulated. We will make
		// sure that this guarantee holds.

		// TODO: avoid simulating function types when possible
		rtyp, sim := getReflectType(env.interp.typeMap, typ)
		if sim {
			// We must simulate the function type we want to create
			f := createSimulatedFunc(env, e)
			rf := reflect.ValueOf(f)
			obj := Object{
				Value: rf,
				Typ:   typ,
				Sim:   true,
			}
			return []Object{obj}
		}
		// We can actually create a function of the right type
		f := createUnsimulatedFunc(env, e, rtyp)
		obj := Object{
			Value: f,
			Typ:   typ,
		}
		return []Object{obj}

	case *ast.StarExpr:
		// Because we have a StarExpr at this point in Eval, we know
		// it is a unary "*" expression rather than a pointer type
		xObj := env.Eval(e.X)[0]
		xVal := xObj.Value.(reflect.Value)
		newVal := xVal.Elem()
		if !newVal.IsValid() {
			// Nil pointer dereference!
			panic("goconsole: Nil pointer dereference")
		}
		obj := Object{
			Value: newVal,
			Typ:   typ,
		}
		return []Object{obj}
	case *ast.UnaryExpr:
		// TODO: implement unary expressions
		switch e.Op {
		case token.AND:
			xObj := env.Eval(e.X)[0]
			xVal := xObj.Value.(reflect.Value)
			newVal := xVal.Addr()
			obj := Object{
				Value: newVal,
				Typ:   typ,
			}
			return []Object{obj}
		case token.ARROW:
			xObj := env.Eval(e.X)[0]
			xVal := xObj.Value.(reflect.Value)
			var valTyp types.Type
			commaOk := false
			switch tup := typ.(type) {
			case *types.Tuple:
				valTyp = tup.At(0).Type()
				commaOk = true
			default:
				valTyp = typ
			}
			newVal, ok := xVal.Recv()
			_, sim := getReflectType(env.interp.typeMap, typ)
			valObj := Object{
				Value: newVal,
				Typ:   valTyp,
				Sim:   sim,
			}
			if commaOk {
				okObj := Object{
					Value: reflect.ValueOf(ok),
					Typ:   types.Typ[types.Bool],
					Sim:   false,
				}
				return []Object{valObj, okObj}
			}
			return []Object{valObj}
		default:
			log.Fatalf("Unary operator %q not implemented", e.Op)
		}

	case *ast.TypeAssertExpr:
		// type assertion x.(T)

		// if T is interface type:
		//    * assert that x's dynamic type implements T
		//    * if so, value of expr is T(val in x) or [T(val in x), true]
		//    * otherwise, runtime panic or [ zero val of T, false ]

		// if T is of non-interface type:
		//    * assert that x's dynamic type is identical to T
		//    * if so, value of expr is (val in x) or [ (val in x), true]
		//    * otherwise, runtime panic or [ zero val of T, false ]

		// types T and V are identical if and only if all of the following are true:
		//    * values of T are assignable to V,
		//    * both or neither of T and V are named types (that is, isNamed(T) == isNamed(V)),
		//    * (T is not a channel type) OR (dir of T == dir of V)

		eTyp := typ

		// If the expression has tuple type, then it's a "comma, ok" type assertion
		_, commaOk := eTyp.(*types.Tuple)

		toTyp := env.info.TypeOf(e.Type)
		toRtyp, sim := getReflectType(env.interp.typeMap, toTyp)
		if toRtyp == nil {
			log.Fatalf("Couldn't get reflect type: %v", toTyp)
		}

		obj := env.Eval(e.X)[0]
		objVal := obj.Value.(reflect.Value)
		dynamicVal := objVal.Elem()
		dynamicRtyp := dynamicVal.Type()

		var resultObj Object
		var assertSuccess bool

		switch typ.Underlying().(type) {
		case *types.Interface:
			// assert that dynamic type of obj implements typ
			// TODO This should be a runtime panic if not

			assertSuccess = objVal.Type().Implements(toRtyp)

			if assertSuccess {
				resultVal := dynamicVal.Convert(toRtyp)
				resultObj = Object{
					Sim:   sim,
					Typ:   toTyp,
					Value: resultVal,
				}
			} else if !commaOk {
				// TODO this should be a runtime panic
				err := fmt.Errorf("interface conversion: interface is %v, not %v", dynamicRtyp, toRtyp)
				panic(err)
			} else {
				resultObj = Object{
					Sim:   sim,
					Typ:   toTyp,
					Value: reflect.Zero(toRtyp),
				}
			}

		default:
			isNamed := func(t reflect.Type) bool {
				return len(t.Name()) > 0
			}

			areIdentical := func(t1, t2 reflect.Type) bool {
				if !t1.AssignableTo(t2) {
					return false
				}
				if isNamed(t1) != isNamed(t2) {
					return false
				}
				if t1.Kind() == reflect.Chan && t1.ChanDir() != t2.ChanDir() {
					return false
				}
				return true
			}

			assertSuccess = areIdentical(dynamicRtyp, toRtyp)
			if assertSuccess {
				resultVal := dynamicVal.Convert(toRtyp)
				resultObj = Object{
					Sim:   sim,
					Typ:   toTyp,
					Value: resultVal,
				}
			} else if !commaOk {
				// TODO this should be a runtime panic
				err := fmt.Errorf("interface conversion: interface is %v, not %v", dynamicRtyp, toRtyp)
				panic(err)
			} else {
				resultObj = Object{
					Sim:   sim,
					Typ:   toTyp,
					Value: reflect.Zero(toRtyp),
				}
			}
		}

		// Return the appropriate
		if commaOk {
			successObj := Object{
				Sim:   false,
				Typ:   types.Typ[types.Bool],
				Value: reflect.ValueOf(assertSuccess),
			}
			return []Object{resultObj, successObj}
		} else {
			return []Object{resultObj}
		}

	case *ast.BinaryExpr:
		left := env.Eval(e.X)[0]
		right := env.Eval(e.Y)[0]

		var obj Object

		switch e.Op {
		case token.LSS, token.GTR, token.LEQ, token.GEQ, token.EQL, token.NEQ:
			obj = doBinaryComparisonOp(env, left, right, e.Op, typ)
		default:
			obj = doBinaryOp(env, left, right, e.Op)
		}

		return []Object{obj}
	case *ast.Ident:
		val, _ := env.lookupParent(e.String())
		return []Object{val}
	case *ast.ParenExpr:
		return env.Eval(e.X)
	case *ast.SelectorExpr:
		// TODO: implement selector expressions!
		sel, ok := env.info.Selections[e]
		if !ok {
			// Then this selector expression denotes a package object
			obj := env.info.Uses[e.Sel]
			p := obj.Pkg().Name()
			v, ok := env.interp.pkgs[p].Lookup(obj.Name())
			if !ok {
				log.Fatalf("Package object %q not found", obj)
			}
			return []Object{v}
		}
		switch sel.Kind() {
		case types.FieldVal:
			xo := env.Eval(e.X)[0]
			v := xo.Value.(reflect.Value)
			if sel.Indirect() {
				v = v.Elem()
			}
			obj := Object{
				Value: v.FieldByIndex(sel.Index()),
				Typ:   sel.Type(),
			}
			return []Object{obj}
		case types.MethodVal:
			log.Fatal("Method values not yet implemented:", sel.String())
		case types.MethodExpr:
			log.Fatal("Method expressions not yet implemented:", sel.String())
		}
	case *ast.CallExpr:
		switch env.getCallExprKind(e) {
		case builtinKind:
			return env.evalBuiltinCall(e, false)
		case conversionKind:
			// Get the type we're converting to
			typ := env.info.TypeOf(e.Fun)
			rtyp, sim := getReflectType(env.interp.typeMap, typ)
			if rtyp == nil {
				log.Fatal("Failed to obtain reflect.Type to represent type:", typ)
			}

			// Evaluate the value to be converted
			argObj := env.Eval(e.Args[0])[0]

			var val reflect.Value
			argVal, ok := argObj.Value.(reflect.Value)
			if !ok {
				// This means it's a conversion of nil
				// Use the zero value
				val = reflect.Zero(rtyp)
			} else {
				val = argVal.Convert(rtyp)
			}

			obj := Object{
				Value: val,
				Typ:   typ,
				Sim:   sim,
			}
			return []Object{obj}
		case callKind:
			return env.evalFuncCall(e, false)
		}
	case *ast.IndexExpr:
		resultTyp := typ
		collTyp := env.info.TypeOf(e.X)

		// Figure out which type of object we're indexing
		objTyp := collTyp.Underlying()
		switch objTyp := objTyp.(type) {
		case *types.Array:
			log.Fatal("Array indexing not implemented yet")
		case *types.Map:
			keyObj := env.Eval(e.Index)[0]
			keyVal, ok := keyObj.Value.(reflect.Value)
			if !ok {
				// Must be untyped nil. Use zero value of type.
				rtyp, _ := getReflectType(env.interp.typeMap, objTyp.Key())
				if rtyp == nil {
					log.Fatalf("couldn't get reflect.Type corresponding to %q", objTyp)
				}
				keyVal = reflect.Zero(rtyp)
			}
			commaOk := false
			var boolTyp types.Type
			if tupleTyp, ok := resultTyp.(*types.Tuple); ok {
				// "Comma-ok" map index expression
				resultTyp = tupleTyp.At(0).Type()
				boolTyp = tupleTyp.At(1).Type()
				commaOk = true
			}
			rtyp, sim := getReflectType(env.interp.typeMap, resultTyp)
			if rtyp == nil {
				log.Fatal("Couldn't get reflect.Type of result type of map index expression")
			}
			mapObj := env.Eval(e.X)[0]
			mapVal := mapObj.Value.(reflect.Value)
			resultVal := mapVal.MapIndex(keyVal)
			keyFound := true
			if !resultVal.IsValid() {
				// The key wasn't found in the map (including case where map is nil)
				resultVal = reflect.Zero(rtyp)
				keyFound = false
			}
			resultObj := Object{
				Value: resultVal,
				Typ:   resultTyp,
				Sim:   sim,
			}
			if commaOk {
				foundObj := Object{
					Value: reflect.ValueOf(keyFound),
					Typ:   boolTyp,
					Sim:   false,
				}
				return []Object{resultObj, foundObj}
			}
			return []Object{resultObj}

		case *types.Slice:
			indObj := env.Eval(e.Index)[0]
			ind := int(indObj.Value.(reflect.Value).Int())
			sliceObj := env.Eval(e.X)[0]
			sliceVal := sliceObj.Value.(reflect.Value)
			resultVal := sliceVal.Index(ind)
			rtyp, sim := getReflectType(env.interp.typeMap, resultTyp)
			if rtyp == nil {
				log.Fatal("Couldn't get reflect.Type of result type of slice index expression")
			}
			resultObj := Object{
				Value: resultVal,
				Typ:   resultTyp,
				Sim:   sim,
			}
			return []Object{resultObj}
		case *types.Basic:
			log.Fatal("String indexing not implemented yet")
		case *types.Pointer:
			log.Fatal("Pointer-to-array indexing not implemented yet")
		}

		log.Fatalf("Unhandled expression type: %T", e)
	default:
		log.Fatalf("Unhandled expression type: %T", e)
	}
	return []Object{}
}

func isTyped(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return !ok || t.Info()&types.IsUntyped == 0
}

func convertExactToReflect(typeMap *typeutil.Map, tv types.TypeAndValue) reflect.Value {
	ev := tv.Value
	rtyp, _ := getReflectType(typeMap, tv.Type)
	if rtyp == nil {
		// This should not happen
		log.Fatal("Couldn't get a reflect.Type from the provided types.Type")
	}
	// TODO: Is it a problem that we're making constants settable?
	rv := reflect.New(rtyp).Elem()
	switch tv.Type.Underlying().(*types.Basic).Kind() {
	case types.Bool:
		rv.SetBool(exact.BoolVal(ev))
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		i64, _ := exact.Int64Val(ev)
		rv.SetInt(i64)
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		ui64, _ := exact.Uint64Val(ev)
		rv.SetUint(ui64)
	case types.Float32, types.Float64:
		f64, _ := exact.Float64Val(ev)
		rv.SetFloat(f64)
	case types.Complex64, types.Complex128:
		real64, _ := exact.Float64Val(exact.Real(ev))
		imag64, _ := exact.Float64Val(exact.Imag(ev))
		c128 := complex(real64, imag64)
		rv.SetComplex(c128)
	case types.String:
		rv.SetString(exact.StringVal(ev))
	default:
		return reflect.Value{}
	}
	return rv
}
