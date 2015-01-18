package interp

import "reflect"

// doInc implements the IncDec statement for ++
func doInc(obj Object) {
	val := obj.Value.(reflect.Value)

	switch val.Kind() {
	case reflect.Int:
		sum := int(val.Int()) + 1
		val.SetInt(int64(sum))
	case reflect.Int8:
		sum := int8(val.Int()) + 1
		val.SetInt(int64(sum))
	case reflect.Int16:
		sum := int16(val.Int()) + 1
		val.SetInt(int64(sum))
	case reflect.Int32:
		sum := int32(val.Int()) + 1
		val.SetInt(int64(sum))
	case reflect.Int64:
		sum := int64(val.Int()) + 1
		val.SetInt(int64(sum))
	case reflect.Uint:
		sum := uint(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Uint8:
		sum := uint8(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Uint16:
		sum := uint16(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Uint32:
		sum := uint32(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Uint64:
		sum := uint64(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Uintptr:
		sum := uintptr(val.Uint()) + 1
		val.SetUint(uint64(sum))
	case reflect.Float32:
		sum := float32(val.Float()) + 1
		val.SetFloat(float64(sum))
	case reflect.Float64:
		sum := float64(val.Float()) + 1
		val.SetFloat(float64(sum))
	case reflect.Complex64:
		sum := complex64(val.Complex()) + 1
		val.SetComplex(complex128(sum))
	case reflect.Complex128:
		sum := complex128(val.Complex()) + 1
		val.SetComplex(complex128(sum))
	default:
		panic("Type error: Invalid operand to ++: " + TypeString(obj.Typ))
	}
}

// doDec implements the IncDec statement for --
func doDec(obj Object) {
	val := obj.Value.(reflect.Value)

	switch val.Kind() {
	case reflect.Int:
		sum := int(val.Int()) - 1
		val.SetInt(int64(sum))
	case reflect.Int8:
		sum := int8(val.Int()) - 1
		val.SetInt(int64(sum))
	case reflect.Int16:
		sum := int16(val.Int()) - 1
		val.SetInt(int64(sum))
	case reflect.Int32:
		sum := int32(val.Int()) - 1
		val.SetInt(int64(sum))
	case reflect.Int64:
		sum := int64(val.Int()) - 1
		val.SetInt(int64(sum))
	case reflect.Uint:
		sum := uint(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Uint8:
		sum := uint8(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Uint16:
		sum := uint16(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Uint32:
		sum := uint32(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Uint64:
		sum := uint64(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Uintptr:
		sum := uintptr(val.Uint()) - 1
		val.SetUint(uint64(sum))
	case reflect.Float32:
		sum := float32(val.Float()) - 1
		val.SetFloat(float64(sum))
	case reflect.Float64:
		sum := float64(val.Float()) - 1
		val.SetFloat(float64(sum))
	case reflect.Complex64:
		sum := complex64(val.Complex()) - 1
		val.SetComplex(complex128(sum))
	case reflect.Complex128:
		sum := complex128(val.Complex()) - 1
		val.SetComplex(complex128(sum))
	default:
		panic("Type error: Invalid operand to ++: " + TypeString(obj.Typ))
	}
}
