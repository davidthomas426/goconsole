package interp

import (
	"log"
	"reflect"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

// TODO: Handle "generic" types
//   * slice types
//   * chan types
//   * map types
//   * array types (must be simulated)

var simFuncType reflect.Type

func init() {
	var simFunc func([]Object) []Object
	simFuncType = reflect.TypeOf(simFunc)
}

func getReflectDir(dir types.ChanDir) reflect.ChanDir {
	var rdir reflect.ChanDir
	switch dir {
	case types.SendOnly:
		rdir = reflect.SendDir
	case types.RecvOnly:
		rdir = reflect.RecvDir
	case types.SendRecv:
		rdir = reflect.BothDir
	default:
		log.Fatal("Unexpected channel direction")
	}
	return rdir
}

func getReflectType(typeMap *typeutil.Map, typ types.Type) (reflect.Type, bool) {
	rt := typeMap.At(typ)
	if rt == nil {
		// If it's a function type that isn't in typeMap, use a simulated function
		switch typ := typ.(type) {
		case *types.Signature:
			return simFuncType, true
		case *types.Pointer:
			t, _ := getReflectType(typeMap, typ.Elem())
			if t != nil {
				return reflect.PtrTo(t), false
			}
		case *types.Slice:
			elem, _ := getReflectType(typeMap, typ.Elem())
			if elem != nil {
				return reflect.SliceOf(elem), false
			}
		case *types.Array:
			elem, _ := getReflectType(typeMap, typ.Elem())
			if elem != nil {
				return reflect.SliceOf(elem), true
			}
		case *types.Chan:
			elem, _ := getReflectType(typeMap, typ.Elem())
			dir := typ.Dir()
			rdir := getReflectDir(dir)
			if elem != nil {
				return reflect.ChanOf(rdir, elem), false
			}
		case *types.Map:
			key, _ := getReflectType(typeMap, typ.Key())
			elem, _ := getReflectType(typeMap, typ.Elem())
			if key != nil && elem != nil {
				return reflect.MapOf(key, elem), false
			}
		}
		return nil, false
	}
	return rt.(reflect.Type), false
}

func addBasicTypes(typeMap *typeutil.Map) {
	// bool
	var xBool bool
	typeMap.Set(types.Typ[types.Bool], reflect.TypeOf(xBool))
	// int
	var xInt int
	typeMap.Set(types.Typ[types.Int], reflect.TypeOf(xInt))
	// int8
	var xInt8 int8
	typeMap.Set(types.Typ[types.Int8], reflect.TypeOf(xInt8))
	// int16
	var xInt16 int16
	typeMap.Set(types.Typ[types.Int16], reflect.TypeOf(xInt16))
	// int32
	var xInt32 int32
	typeMap.Set(types.Typ[types.Int32], reflect.TypeOf(xInt32))
	// int64
	var xInt64 int64
	typeMap.Set(types.Typ[types.Int64], reflect.TypeOf(xInt64))
	// uint
	var xUint uint
	typeMap.Set(types.Typ[types.Uint], reflect.TypeOf(xUint))
	// uint8
	var xUint8 uint8
	typeMap.Set(types.Typ[types.Uint8], reflect.TypeOf(xUint8))
	// uint16
	var xUint16 uint16
	typeMap.Set(types.Typ[types.Uint16], reflect.TypeOf(xUint16))
	// uint32
	var xUint32 uint32
	typeMap.Set(types.Typ[types.Uint32], reflect.TypeOf(xUint32))
	// uint64
	var xUint64 uint64
	typeMap.Set(types.Typ[types.Uint64], reflect.TypeOf(xUint64))
	// uintptr
	var xUintptr uintptr
	typeMap.Set(types.Typ[types.Uintptr], reflect.TypeOf(xUintptr))
	// float32
	var xFloat32 float32
	typeMap.Set(types.Typ[types.Float32], reflect.TypeOf(xFloat32))
	// float64
	var xFloat64 float64
	typeMap.Set(types.Typ[types.Float64], reflect.TypeOf(xFloat64))
	// complex64
	var xComplex64 complex64
	typeMap.Set(types.Typ[types.Complex64], reflect.TypeOf(xComplex64))
	// complex128
	var xComplex128 complex128
	typeMap.Set(types.Typ[types.Complex128], reflect.TypeOf(xComplex128))
	// string
	var xString string
	typeMap.Set(types.Typ[types.String], reflect.TypeOf(xString))
	// struct{}
	var xEmptyStruct struct{}
	typEmptyStruct := types.NewStruct([]*types.Var{}, []string{})
	typeMap.Set(typEmptyStruct, reflect.TypeOf(xEmptyStruct))
	// interface{}
	var xEmptyInterface interface{}
	typEmptyInterface := types.NewInterface([]*types.Func{}, []*types.Named{})
	typeMap.Set(typEmptyInterface, reflect.TypeOf(&xEmptyInterface).Elem())
}
