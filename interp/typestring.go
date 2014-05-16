// Copyright 2013 The Go Authors. All rights reserved.

// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//   * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//   * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//   * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.

// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// This file implements printing of types.

// This is a custom version of this file derived from
// code.google.com/p/go.tools/go/types/typestring.go.
// The way named types are printed is different, to match
// the way the language parses the type (e.g., using the package
// name instead of the package path in qualified type names).
// GcCompatibilityMode is gone, too.

package interp

import (
	"bytes"
	"fmt"

	"code.google.com/p/go.tools/go/types"
)

// TypeString returns the string representation of typ.
// Named types are printed package-qualified if they
// do not belong to this package.
func TypeString(typ types.Type) string {
	var buf bytes.Buffer
	WriteType(&buf, typ)
	return buf.String()
}

// WriteType writes the string representation of typ to buf.
// Named types are printed package-qualified if they
// do not belong to this package.
func WriteType(buf *bytes.Buffer, typ types.Type) {
	writeType(buf, nil, typ, make([]types.Type, 8))
}

func writeType(buf *bytes.Buffer, this *types.Package, typ types.Type, visited []types.Type) {
	// Theoretically, this is a quadratic lookup algorithm, but in
	// practice deeply nested composite types with unnamed component
	// types are uncommon. This code is likely more efficient than
	// using a map.
	for _, t := range visited {
		if t == typ {
			fmt.Fprintf(buf, "○%T", typ) // cycle to typ
			return
		}
	}
	visited = append(visited, typ)

	switch t := typ.(type) {
	case nil:
		buf.WriteString("<nil>")

	case *types.Basic:
		if t.Kind() == types.UnsafePointer {
			buf.WriteString("unsafe.")
		}
		buf.WriteString(t.Name())

	case *types.Array:
		fmt.Fprintf(buf, "[%d]", t.Len())
		writeType(buf, this, t.Elem(), visited)

	case *types.Slice:
		buf.WriteString("[]")
		writeType(buf, this, t.Elem(), visited)

	case *types.Struct:
		buf.WriteString("struct{")
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			if i > 0 {
				buf.WriteString("; ")
			}
			if !f.Anonymous() {
				buf.WriteString(f.Name())
				buf.WriteByte(' ')
			}
			writeType(buf, this, f.Type(), visited)
			if tag := t.Tag(i); tag != "" {
				fmt.Fprintf(buf, " %q", tag)
			}
		}
		buf.WriteByte('}')

	case *types.Pointer:
		buf.WriteByte('*')
		writeType(buf, this, t.Elem(), visited)

	case *types.Tuple:
		writeTuple(buf, this, t, false, visited)

	case *types.Signature:
		buf.WriteString("func")
		writeSignature(buf, this, t, visited)

	case *types.Interface:
		// We write the source-level methods and embedded types rather
		// than the actual method set since resolved method signatures
		// may have non-printable cycles if parameters have anonymous
		// interface types that (directly or indirectly) embed the
		// current interface. For instance, consider the result type
		// of m:
		//
		//     type T interface{
		//         m() interface{ T }
		//     }
		//
		buf.WriteString("interface{")

		// print explicit interface methods and embedded types
		for i := 0; i < t.NumExplicitMethods(); i++ {
			m := t.ExplicitMethod(i)
			if i > 0 {
				buf.WriteString("; ")
			}
			buf.WriteString(m.Name())
			writeSignature(buf, this, m.Type().(*types.Signature), visited)
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			typ := t.Embedded(i)
			if i > 0 || t.NumExplicitMethods() > 0 {
				buf.WriteString("; ")
			}
			writeType(buf, this, typ, visited)
		}

		buf.WriteByte('}')

	case *types.Map:
		buf.WriteString("map[")
		writeType(buf, this, t.Key(), visited)
		buf.WriteByte(']')
		writeType(buf, this, t.Elem(), visited)

	case *types.Chan:
		var s string
		var parens bool
		switch t.Dir() {
		case types.SendRecv:
			s = "chan "
			// chan (<-chan T) requires parentheses
			if c, _ := t.Elem().(*types.Chan); c != nil && c.Dir() == types.RecvOnly {
				parens = true
			}
		case types.SendOnly:
			s = "chan<- "
		case types.RecvOnly:
			s = "<-chan "
		default:
			panic("unreachable")
		}
		buf.WriteString(s)
		if parens {
			buf.WriteByte('(')
		}
		writeType(buf, this, t.Elem(), visited)
		if parens {
			buf.WriteByte(')')
		}

	case *types.Named:
		s := "<Named w/o object>"
		if obj := t.Obj(); obj != nil {
			if pkg := obj.Pkg(); pkg != nil && pkg != this {
				buf.WriteString(pkg.Name())
				buf.WriteByte('.')
			}
			// TODO(gri): function-local named types should be displayed
			// differently from named types at package level to avoid
			// ambiguity.
			s = obj.Name()
		}
		buf.WriteString(s)

	default:
		// For externally defined implementations of Type.
		buf.WriteString(t.String())
	}
}

func writeTuple(buf *bytes.Buffer, this *types.Package, tup *types.Tuple, variadic bool, visited []types.Type) {
	buf.WriteByte('(')
	if tup != nil {
		for i := 0; i < tup.Len(); i++ {
			v := tup.At(i)
			if i > 0 {
				buf.WriteString(", ")
			}
			if v.Name() != "" {
				buf.WriteString(v.Name())
				buf.WriteByte(' ')
			}
			typ := v.Type()
			if variadic && i == tup.Len()-1 {
				buf.WriteString("...")
				typ = typ.(*types.Slice).Elem()
			}
			writeType(buf, this, typ, visited)
		}
	}
	buf.WriteByte(')')
}

// WriteSignature writes the representation of the signature sig to buf,
// without a leading "func" keyword.
// Named types are printed package-qualified if they
// do not belong to this package.
func WriteSignature(buf *bytes.Buffer, sig *types.Signature) {
	writeSignature(buf, nil, sig, make([]types.Type, 8))
}

func writeSignature(buf *bytes.Buffer, this *types.Package, sig *types.Signature, visited []types.Type) {
	writeTuple(buf, this, sig.Params(), sig.Variadic(), visited)

	n := sig.Results().Len()
	if n == 0 {
		// no result
		return
	}

	buf.WriteByte(' ')
	if n == 1 && sig.Results().At(0).Name() == "" {
		// single unnamed result
		writeType(buf, this, sig.Results().At(0).Type(), visited)
		return
	}

	// multiple or named result(s)
	writeTuple(buf, this, sig.Results(), false, visited)
}
