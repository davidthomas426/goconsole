package goconsole

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

func (env *environ) Eval(expr ast.Expr) []Object {
	// Check for constant
	tv := env.info.Types[expr]
	if tv.Type == types.Typ[types.UntypedNil] {
		fmt.Printf("(%#v) %#v, %#v\n", tv.Type, tv.Value, tv)

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
	// I'm assuming it's typed, but let's check...
	fmt.Printf("Is it typed? ('%s' of type '%v'): %v\n", types.ExprString(expr), tv.Type, isTyped(tv.Type))

	// Not a constant expression, so we have to evaluate it ourselves
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
		typ := env.info.Types[e].Type
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
		typ := env.info.Types[expr].Type
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
			typ := env.info.Types[expr].Type
			obj := Object{
				Value: newVal,
				Typ:   typ,
			}
			return []Object{obj}
		default:
			log.Fatalf("Unary operator %q not implemented", e.Op)
		}
	case *ast.BinaryExpr:
		// This is oversimplified (what about && and || short-circuit eval?)
		var obj Object
		switch e.Op {
		case token.ADD:
			obj = operatorAdd(env, e.X, e.Y)
		case token.SUB:
			obj = operatorSubtract(env, e.X, e.Y)
		case token.MUL:
			obj = operatorMultiply(env, e.X, e.Y)
		case token.QUO:
			obj = operatorQuotient(env, e.X, e.Y)
		case token.REM:
			obj = operatorRemainder(env, e.X, e.Y)
		case token.AND:
			obj = operatorAnd(env, e.X, e.Y)
		case token.OR:
			obj = operatorOr(env, e.X, e.Y)
		case token.XOR:
			obj = operatorXor(env, e.X, e.Y)
		case token.AND_NOT:
			obj = operatorAndNot(env, e.X, e.Y)
		case token.SHR:
			obj = operatorShiftRight(env, e.X, e.Y)
		case token.SHL:
			obj = operatorShiftLeft(env, e.X, e.Y)
		default:
			// TODO: Implement other binary operators (comparisons, for example)
			log.Fatalf("Binary operator %v not implemented yet", e.Op)
		}
		return []Object{obj}
	case *ast.Ident:
		val, _ := env.lookupParent(e.String())
		return []Object{val}
	case *ast.ParenExpr:
		return env.Eval(e.X)
	case *ast.SelectorExpr:
		// TODO: implement selector expressions!
		sel := env.info.Selections[e]
		switch sel.Kind() {
		case types.FieldVal:
			fmt.Println("Field value:", sel.String())
		case types.MethodVal:
			fmt.Println("Method value:", sel.String())
		case types.MethodExpr:
			fmt.Println("Method expression:", sel.String())
		case types.PackageObj:
			fmt.Println("Qualified identifier:", sel.Obj().Pkg().Name(), sel.Obj().Name())
			p := sel.Obj().Pkg().Name()
			obj := sel.Obj().Name()
			v, ok := env.interp.pkgs[p].Lookup(obj)
			if !ok {
				log.Fatal("Package object not found")
			}
			return []Object{v}
		}
		log.Fatalf("This type of selector expression not yet implemented")
	case *ast.CallExpr:
		switch env.getCallExprKind(e) {
		case builtinKind:
			// TODO: implement builtins
			log.Fatal("Builtins not handled yet")
		case conversionKind:
			// Get the type we're converting to
			typ := env.info.Types[e.Fun].Type
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
			fmt.Printf("sig: %T: %+[1]v\n", env.info.Types[e.Fun].Type)

			funObj := env.Eval(e.Fun)[0]
			fmt.Println(funObj)
			fun := funObj.Value.(reflect.Value)

			if funObj.Sim {
				var args []Object
				if len(e.Args) == 1 {
					// Single argument expression, potentially multi-valued
					args = env.Eval(e.Args[0])
				} else {
					// Multiple argument expressions, each single-valued
					args = make([]Object, len(e.Args))
					for i, argExpr := range e.Args {
						args[i] = env.Eval(argExpr)[0]
					}
				}

				// Call by actually calling it
				funVal := fun.Interface().(func([]Object) []Object)
				results := funVal(args)
				return results
			} else {
				var args []reflect.Value
				if len(e.Args) == 1 {
					// Single argument expression, potentially multi-valued
					vals := env.Eval(e.Args[0])
					args = make([]reflect.Value, len(vals))
					for i, val := range vals {
						args[i] = val.Value.(reflect.Value)
					}
				} else {
					// Multiple argument expressions, each single-valued
					args = make([]reflect.Value, len(e.Args))
					for i, argExpr := range e.Args {
						argObj := env.Eval(argExpr)[0]
						args[i] = argObj.Value.(reflect.Value)
					}
				}

				resultVals := fun.Call(args)

				// Wrap the output values in Objects
				results := make([]Object, len(resultVals))
				for i, resVal := range resultVals {
					results[i] = Object{
						Value: resVal,
						Typ:   funObj.Typ.Underlying().(*types.Signature).Results().At(i).Type(),
					}
				}
				return results
			}
		}
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
