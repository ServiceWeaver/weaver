// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generate

import (
	"fmt"
	"go/types"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const weaverPackagePath = "github.com/ServiceWeaver/weaver"

// typeSet holds type information needed by the code generator.
type typeSet struct {
	pkg            *packages.Package
	imported       []importPkg          // imported packages
	importedByPath map[string]importPkg // imported, indexed by path
	importedByName map[string]importPkg // imported, indexed by name

	automarshals          *typeutil.Map // types that implement AutoMarshal
	automarshalCandidates *typeutil.Map // types that declare themselves AutoMarshal

	// If checked[t] != nil, then checked[t] is the cached result of calling
	// check(pkg, t, string[]{}). Otherwise, if checked[t] == nil, then t has
	// not yet been checked for serializability. Read typeutil.Map's
	// documentation for why checked shouldn't be a map[types.Type]bool.
	checked typeutil.Map

	// If sizes[t] != nil, then sizes[t] == sizeOfType(t).
	sizes typeutil.Map

	// If measurable[t] != nil, then measurable[t] == isMeasurableType(t).
	measurable typeutil.Map
}

// importPkg is a package imported by the generated code.
type importPkg struct {
	path  string // e.g., "github.com/ServiceWeaver/weaver"
	pkg   string // e.g., "weaver", "context", "time"
	alias string // e.g., foo in `import foo "context"`
	local bool   // are we in this package?
}

// name returns the name by which the imported package should be referenced in
// the generated code. If the package is imported without an alias, like this:
//
//	import "context"
//
// then the name is the same as the package name (e.g., "context"). However, if
// a package is imported with an alias, then the name is the alias:
//
//	import thisIsAnAlias "context"
//
// If the package is local, an empty string is returned.
func (i importPkg) name() string {
	if i.local {
		return ""
	} else if i.alias != "" {
		return i.alias
	}
	return i.pkg
}

// qualify returns the provided member of the package, qualified with the
// package name. For example, the "Context" type inside the "context" package
// is qualified "context.Context". The "Now" function inside the "time" package
// is qualified "time.Now". Note that the package name is not prefixed when
// qualifying members of the local package.
func (i importPkg) qualify(member string) string {
	if i.local {
		return member
	}
	return fmt.Sprintf("%s.%s", i.name(), member)
}

// newTypeSet returns the container for types found in pkg.
func newTypeSet(pkg *packages.Package, automarshals, automarshalCandidates *typeutil.Map) *typeSet {
	return &typeSet{
		pkg:                   pkg,
		imported:              []importPkg{},
		importedByPath:        map[string]importPkg{},
		importedByName:        map[string]importPkg{},
		automarshals:          automarshals,
		automarshalCandidates: automarshalCandidates,
	}
}

// importPackage imports a package with the provided path and package name. The
// package is imported with an alias if there is a package name clash.
func (tset *typeSet) importPackage(path, pkg string) importPkg {
	newImportPkg := func(path, pkg, alias string, local bool) importPkg {
		i := importPkg{path: path, pkg: pkg, alias: alias, local: local}
		tset.imported = append(tset.imported, i)
		tset.importedByPath[i.path] = i
		tset.importedByName[i.name()] = i
		return i
	}

	if imp, ok := tset.importedByPath[path]; ok {
		// This package has already been imported.
		return imp
	}

	if _, ok := tset.importedByName[pkg]; !ok {
		// Import the package without an alias.
		return newImportPkg(path, pkg, "", path == tset.pkg.PkgPath)
	}

	// Find an unused alias.
	var alias string
	counter := 1
	for {
		alias = fmt.Sprintf("%s%d", pkg, counter)
		if _, ok := tset.importedByName[alias]; !ok {
			break
		}
		counter++
	}
	return newImportPkg(path, pkg, alias, path == tset.pkg.PkgPath)
}

// imports returns the list of packages to import in generated code.
func (tset *typeSet) imports() []importPkg {
	sort.Slice(tset.imported, func(i, j int) bool {
		return tset.imported[i].path < tset.imported[j].path
	})
	return tset.imported
}

// checkSerializable checks that type t is serializable.
func (tset *typeSet) checkSerializable(t types.Type) []error {
	// lineage can generate a human readable description of the lineage of a
	// checked type. As check recurses on type t, it encounters a number of
	// nested types. For example, if we have the following type A
	//
	//     type A struct{ x []chan int }
	//
	// then check(A) will encounter the types A, struct{ x []chan int }, []chan
	// int, chan int, and int. We associate each of these types with a
	// corresponding "path", a concise description of the relationship between
	// the root type and the nested types. For example, the type chan int has
	// path A.x[0].
	//
	// lineage is a stack that stores a history of these paths as check
	// traverses a type. For example, if we call check(A), then lineage will
	// look like this when the chan int is discovered:
	//
	//     []pathAndType{
	//         pathAndType{"A", A},
	//         pathAndType{"A.x", []chan int},
	//         pathAndType{"A.x[0]", chan int},
	//     }
	//
	// This lineage is printed in error messages as:
	//
	//     A (type A)
	//     A.x (type []chan int)
	//     A.x[0] (type chan int)
	//
	// Note that for brevity, not every encountered type is entered into the
	// lineage.
	type pathAndType struct {
		path string
		t    types.Type
	}
	var lineage []pathAndType

	var errors []error
	addError := func(err error) {
		var builder strings.Builder

		// If the lineage is trivial, then don't show it.
		if len(lineage) > 1 {
			fmt.Fprintf(&builder, "\n    ")
			for i, pn := range lineage {
				fmt.Fprintf(&builder, "%v (type %v)", pn.path, pn.t.String())
				if i < len(lineage)-1 {
					fmt.Fprintf(&builder, "\n    ")
				}
			}
		}

		qualifier := func(pkg *types.Package) string { return pkg.Name() }
		err = fmt.Errorf("%s: %w%s", types.TypeString(t, qualifier), err, builder.String())
		errors = append(errors, err)
	}

	// stack contains the set of types encountered in the call stack of check.
	// It's used to detect recursive types.
	//
	// More specifically, the check function below is performing an implicit
	// depth first search of the graph of types formed by t. We record the
	// stack of visited types in stack and know we have a recursive type if we
	// ever run into a type that is already in stack.
	//
	// For example, consider the following types:
	//
	//   type A struct { b: *B }
	//   type B struct { a: *A }
	//
	// Calling check on A will yield a call stack that looks something like:
	//
	//   check(A)
	//     check(struct { b: *B })
	//       check(*B)
	//         check(B)
	//           check(struct { a: *A })
	//             check(*A)
	//               check(A)
	//
	// When performing the second check(A) call, stack includes A, struct { b:
	// *B }, *B, B, struct { a: *A }, and *A. Because we called check on A and
	// A is already in stack, we detect a recursive type and mark A as not
	// serializable.
	var stack typeutil.Map

	// check recursively checks whether a type t is serializable. See lineage
	// above for a description of path. record is true if the current type
	// should be recorded in lineage. There are a few things worth noting:
	//
	//   (1) The results of calling check are memoized in tset.checked, but not
	//       for some trivial arguments. Some arguments like chan int are not
	//       memoized because they are trivial to check and because not
	//       memoizing can lead to a clearer error message.
	//
	//   (2) Consider the type t = struct { x chan int; y chan int }. t is not
	//       serializable because neither x nor y is serializable. check
	//       reports errors for both x and y as not serializable.
	//       Alternatively, check could find that x is not serializable and
	//       then immediately report that t is not serializable, skipping y
	//       completely. check doesn't do this. check will inspect a type fully
	//       to report the full set of errors.
	//
	// Note that the function also takes the parent type pt. This is needed in cases
	// whether we need to know the type of the parent type t (e.g., a named type
	// that is a proto is serializable iff the parent type is a pointer).
	var check func(t types.Type, path string, record bool) bool

	check = func(t types.Type, path string, record bool) bool {
		if record {
			lineage = append(lineage, pathAndType{path, t})
			defer func() { lineage = lineage[:len(lineage)-1] }()
		}

		// Return early if we've already checked this type.
		if result := tset.checked.At(t); result != nil {
			b := result.(bool)
			if b {
				return true
			}
			// We've already encountered type t and determined that it is not
			// serializable. We won't recurse down type t to compute the full
			// lineage and explanation of why t isn't serializable because we
			// already did that when determining t wasn't serializable in the
			// first place. Instead, we instruct the user to read the
			// previously reported error.
			addError(fmt.Errorf("not a serializable type; see above for details"))
			return false
		}

		// Check for recursive types.
		if stack.At(t) != nil {
			addError(fmt.Errorf("serialization of recursive types not currently supported"))
			tset.checked.Set(t, false)
			return false
		}
		stack.Set(t, struct{}{})
		defer func() { stack.Delete(t) }()

		switch x := t.(type) {
		case *types.Named:
			// No need to check if x is an unexported type from another package
			// since the Go compiler takes care of that.

			// Check if the type implements one of the marshaler interfaces.
			if tset.isProto(x) || tset.automarshals.At(t) != nil || tset.implementsAutoMarshal(x) || tset.hasMarshalBinary(x) {
				tset.checked.Set(t, true)
				break
			}

			// If the underlying type is not a struct, then we simply recurse
			// on the underlying type.
			s, ok := x.Underlying().(*types.Struct)
			if !ok {
				tset.checked.Set(t, check(x.Underlying(), path, false))
				break
			}

			// If the underlying type is a struct that has not been declared to
			// implement the AutoMarshal interface, then it is not
			// serializable.
			if tset.automarshalCandidates.At(t) == nil {
				// TODO(mwhittaker): Print out a link to documentation on
				// weaver.AutoMarshal.
				addError(fmt.Errorf("named structs are not serializable by default. Consider using weaver.AutoMarshal."))
				tset.checked.Set(t, false)
				break
			}

			// If the underlying type is a struct that has been declared to
			// implement the AutoMarshal interface but hasn't yet been checked,
			// then we need to recurse to detect cycles.
			serializable := true
			for i := 0; i < s.NumFields(); i++ {
				f := s.Field(i)
				// We store the result of calling check in b rather than
				// writing serializable = serializable && check(...) because we
				// don't want to short circuit and avoid calling check.
				b := check(f.Type(), path+"."+f.Name(), true)
				serializable = serializable && b
			}
			tset.checked.Set(t, serializable)

		case *types.Interface:
			// TODO(sanjay): Support types.Interface only if we can figure out
			// a way to instantiate the type.
			addError(fmt.Errorf("serialization of interfaces not currently supported"))
			tset.checked.Set(t, false)

		case *types.Struct:
			addError(fmt.Errorf("struct literals are not serializable"))
			tset.checked.Set(t, false)

		case *types.Basic:
			switch x.Kind() {
			case types.Bool,
				types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
				types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
				types.Float32, types.Float64,
				types.Complex64, types.Complex128,
				types.String:
				// Supported.
				tset.checked.Set(t, true)
			default:
				if isInvalid(t) {
					addError(fmt.Errorf("Maybe you forgot to run `go mod tidy`? Also try running `go build` to diagnose further."))
				} else {
					addError(fmt.Errorf("unsupported basic type"))
				}
				// For a better error message, we don't memoize this.
				return false
			}

		case *types.Array:
			tset.checked.Set(t, check(x.Elem(), path+"[0]", true))

		case *types.Slice:
			tset.checked.Set(t, check(x.Elem(), path+"[0]", true))

		case *types.Pointer:
			tset.checked.Set(t, check(x.Elem(), "(*"+path+")", true))

		case *types.Map:
			keySerializable := check(x.Key(), path+".key", true)
			valSerializable := check(x.Elem(), path+".value", true)
			tset.checked.Set(t, keySerializable && valSerializable)

		default:
			addError(fmt.Errorf("not a serializable type"))
			// For a better error message, we don't memoize this.
			return false
		}

		return tset.checked.At(t).(bool)
	}

	check(t, t.String(), true)
	return errors
}

// isFixedSizeType returns whether the provided type has a fixed serialization
// size. Here is a summary of which types are fixed sized:
//
//   - Every basic type (e.g., bool, int) except string is fixed sized.
//   - The array type [N]t is fixed sized if t is fixed sized.
//   - A struct is fixed sized if the types of its fields are fixed sized.
//   - A named type is fixed sized if its underlying type is fixed sized.
func (tset *typeSet) isFixedSizeType(t types.Type) bool {
	return tset.sizeOfType(t) >= 0
}

// sizeOfType returns the size of the serialization of t if t is fixed size, or
// returns -1 otherwise.
func (tset *typeSet) sizeOfType(t types.Type) int {
	// let s(t) be the size of type t.
	//
	//   s(basic) = size of basic
	//   s([N]t) = N * s(t), if t is fixed size
	//   s(struct{..., fi:ti, ...}) = sum of s(ti), if every ti is fixed size
	//   s(type t u) = s(u)
	//   s(_) = -1
	if size := tset.sizes.At(t); size != nil {
		return size.(int)
	}

	switch x := t.(type) {
	case *types.Basic:
		switch x.Kind() {
		case types.Bool, types.Int8, types.Uint8:
			return 1
		case types.Int16, types.Uint16:
			return 2
		case types.Int32, types.Uint32, types.Float32:
			return 4
		case types.Int, types.Int64, types.Uint, types.Uint64, types.Float64, types.Complex64:
			return 8
		case types.Complex128:
			return 16
		default:
			return -1
		}

	case *types.Array:
		n := tset.sizeOfType(x.Elem())
		if n < 0 || x.Len() < 0 {
			tset.sizes.Set(t, -1)
			return -1
		}
		size := int(x.Len()) * n
		tset.sizes.Set(t, size)
		return size

	case *types.Struct:
		size := 0
		for i := 0; i < x.NumFields(); i++ {
			n := tset.sizeOfType(x.Field(i).Type())
			if n < 0 {
				tset.sizes.Set(t, -1)
				return -1
			}
			size += n
		}
		tset.sizes.Set(t, size)
		return size

	case *types.Named:
		size := tset.sizeOfType(x.Underlying())
		tset.sizes.Set(t, size)
		return size

	default:
		return -1
	}
}

// isMeasurable returns whether the provided type is measurable.
//
// Informally, we say a type is measurable if we can cheaply compute the size
// of its serialization at runtime. Some examples:
//
//   - Every fixed size type (e.g., int, bool, [3]int, struct{x, y int}) is
//     measurable (with some restrictions on package locality; see below).
//   - Strings are not fixed size, but they are measurable because we can
//     cheaply compute the length of a string at runtime.
//   - []string is not measurable because computing the size of the
//     serialization of a []string would require us to compute the length of
//     every string in the slice. This is a potentially expensive operation
//     if the slice contains a large number of strings, so we consider
//     []string to be not measurable.
//   - For simplicity, we only consider a type measurable if the type and all
//     its nested types are package local. For example, a struct { x
//     otherpackage.T } is not measurable, even if otherpackage.T is
//     measurable. We make an exception for weaver.AutoMarshal.
func (tset *typeSet) isMeasurable(t types.Type) bool {
	rootPkg := tset.pkg.Types

	// let m(t) be whether t is measurable.
	//
	//     m(basic type) = true
	//     m(*t) = m(t)
	//     m([N]t) = true if t is fixed size.
	//     m([]t) = true if t is fixed size.
	//     m(map[k]v) = true if k and v are fixed size.
	//     m(struct{..., fi:ti, ...}) = true, if every ti is measurable.
	//     m(weaver.AutoMarshal) = true
	//     m(type t u) = m(u), if t is package local
	//     m(_) = false
	if result := tset.measurable.At(t); result != nil {
		return result.(bool)
	}

	switch x := t.(type) {
	case *types.Basic:
		switch x.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64,
			types.Complex64, types.Complex128,
			types.String:
			// No need to memoize basic types.
			return true
		default:
			return false
		}

	case *types.Pointer:
		tset.measurable.Set(t, tset.isMeasurable(x.Elem()))

	case *types.Array:
		tset.measurable.Set(t, tset.isFixedSizeType(x.Elem()))

	case *types.Slice:
		tset.measurable.Set(t, tset.isFixedSizeType(x.Elem()))

	case *types.Map:
		tset.measurable.Set(t, tset.isFixedSizeType(x.Key()) && tset.isFixedSizeType(x.Elem()))

	case *types.Struct:
		measurable := true
		for i := 0; i < x.NumFields() && measurable; i++ {
			f := x.Field(i)
			if f.Pkg() != rootPkg {
				measurable = false
				break
			}
			measurable = measurable && tset.isMeasurable(f.Type())
		}
		tset.measurable.Set(t, measurable)

	case *types.Named:
		if isWeaverAutoMarshal(x) {
			tset.measurable.Set(t, true)
		} else if x.Obj().Pkg() != rootPkg {
			tset.measurable.Set(t, false)
		} else {
			tset.measurable.Set(t, tset.isMeasurable(x.Underlying()))
		}

	default:
		return false
	}

	return tset.measurable.At(t).(bool)
}

// genTypeString returns the string representation of t as to be printed
// in the generated code, updating import definitions to account for the
// returned type string.
//
// Since this call has side-effects (i.e., updating import definitions), it
// should only be called when the returned type string is written into
// the generated file; otherwise, the generated code may end up with spurious
// imports.
func (tset *typeSet) genTypeString(t types.Type) string {
	// qualifier is passed to types.TypeString(Type, Qualifier) to determine
	// how packages are printed when pretty printing types. For this qualifier,
	// types in the root package are printed without their package name, while
	// types outside the root package are printed with their package name. For
	// example, if we're in root package foo, then the type foo.Bar is printed
	// as Bar, while the type io.Reader is printed as io.Reader. See [1] for
	// more information on qualifiers and pretty printing types.
	//
	// [1]: https://github.com/golang/example/tree/master/gotypes#formatting-support
	var qualifier = func(pkg *types.Package) string {
		if pkg == tset.pkg.Types {
			return ""
		}
		return tset.importPackage(pkg.Path(), pkg.Name()).name()
	}
	return types.TypeString(t, qualifier)
}

// isInvalid returns true iff the given type is invalid.
func isInvalid(t types.Type) bool {
	return t.String() == "invalid type"
}

// implementsError returns whether the provided type is a concrete type that
// implements error.
func (tset *typeSet) implementsError(t types.Type) bool {
	if _, ok := t.Underlying().(*types.Interface); ok {
		return false
	}
	obj, _, _ := types.LookupFieldOrMethod(t, true, tset.pkg.Types, "Error")
	method, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return false
	}
	if args := sig.Params(); args.Len() != 0 {
		return false
	}
	if results := sig.Results(); results.Len() != 1 || !isString(results.At(0).Type()) {
		return false
	}
	return true
}

// isProto returns whether the provided type is a concrete type that implements
// the proto.Message interface.
func (tset *typeSet) isProto(t types.Type) bool {
	if _, ok := t.Underlying().(*types.Interface); ok {
		// A superinterface of proto.Message does "implement" proto.Message,
		// but we only accept concrete types that implement the interface.
		return false
	}

	obj, _, _ := types.LookupFieldOrMethod(t, true, tset.pkg.Types, "ProtoReflect")
	method, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return false
	}
	recv, args, results := sig.Recv(), sig.Params(), sig.Results()
	if args.Len() != 0 || results.Len() != 1 {
		return false
	}
	if !isProtoMessage(results.At(0).Type()) {
		return false
	}
	// Check the receiver. We avoid complicated embeddings by requiring that
	// the method is defined on the type itself.
	//
	// TODO(mwhittaker): Relax this requirement if it becomes annoying.
	if p, ok := recv.Type().(*types.Pointer); ok {
		return types.Identical(p.Elem(), t)
	} else {
		return types.Identical(recv.Type(), t)
	}
}

func isProtoMessage(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	const protoreflect = "google.golang.org/protobuf/reflect/protoreflect"
	return n.Obj().Pkg().Path() == protoreflect && n.Obj().Name() == "Message"
}

// implementsAutoMarshal returns whether the provided type is a concrete
// type that implements the weaver.AutoMarshal interface.
func (tset *typeSet) implementsAutoMarshal(t types.Type) bool {
	if _, ok := t.Underlying().(*types.Interface); ok {
		// A superinterface of AutoMarshal does "implement" the interface,
		// but we only accept concrete types that implement the interface.
		return false
	}

	obj, _, _ := types.LookupFieldOrMethod(t, true, tset.pkg.Types, "WeaverMarshal")
	marshal, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	obj, _, _ = types.LookupFieldOrMethod(t, true, tset.pkg.Types, "WeaverUnmarshal")
	unmarshal, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	return isWeaverMarshal(t, marshal) && isWeaverUnmarshal(t, unmarshal)
}

// isWeaverMarshal returns true if m is WeaverMarshal(*codegen.Encoder).
func isWeaverMarshal(t types.Type, m *types.Func) bool {
	if m.Name() != "WeaverMarshal" {
		return false
	}
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return false
	}
	recv, args, results := sig.Recv(), sig.Params(), sig.Results()
	if args.Len() != 1 || results.Len() != 0 {
		return false
	}
	if !isEncoderPtr(args.At(0).Type()) {
		return false
	}
	// We avoid complicated embeddings by requiring that the method is defined
	// on the type itself.
	//
	// TODO(mwhittaker): Relax this requirement if it becomes annoying.
	if p, ok := recv.Type().(*types.Pointer); ok {
		return types.Identical(p.Elem(), t)
	} else {
		return types.Identical(recv.Type(), t)
	}
}

// isEncoderPtr returns whether t is *codegen.Encoder.
func isEncoderPtr(t types.Type) bool {
	p, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := p.Elem().(*types.Named)
	if !ok {
		return false
	}
	path := path.Join(weaverPackagePath, "runtime", "codegen")
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == path && n.Obj().Name() == "Encoder"
}

// isWeaverUnmarshal returns true if m is WeaverUnmarshal(*codegen.Decoder).
func isWeaverUnmarshal(t types.Type, m *types.Func) bool {
	if m.Name() != "WeaverUnmarshal" {
		return false
	}
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return false
	}
	recv, args, results := sig.Recv(), sig.Params(), sig.Results()
	if args.Len() != 1 || results.Len() != 0 {
		return false
	}
	if !isDecoderPtr(args.At(0).Type()) {
		return false
	}
	// We avoid complicated embeddings by requiring that the method is defined
	// on the type itself.
	//
	// TODO(mwhittaker): Relax this requirement if it becomes annoying.
	if p, ok := recv.Type().(*types.Pointer); ok {
		return types.Identical(p.Elem(), t)
	} else {
		return types.Identical(recv.Type(), t)
	}
}

// isDecoderPtr returns whether t is *codegen.Decoder.
func isDecoderPtr(t types.Type) bool {
	p, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := p.Elem().(*types.Named)
	if !ok {
		return false
	}
	path := path.Join(weaverPackagePath, "runtime", "codegen")
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == path && n.Obj().Name() == "Decoder"
}

// hasMarshalBinary returns whether the provided type is a concrete type that
// implements the encoding.BinaryMarshaler and binary.BinaryUnmarshaler
// interfaces.
func (tset *typeSet) hasMarshalBinary(t types.Type) bool {
	if _, ok := t.Underlying().(*types.Interface); ok {
		// A superinterface of BinaryMarshaler and BinaryUnmarshaler does
		// "implement" the interfaces, but we only accept concrete types that
		// implement the interfaces.
		return false
	}

	obj, _, _ := types.LookupFieldOrMethod(t, true, tset.pkg.Types, "MarshalBinary")
	marshal, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	obj, _, _ = types.LookupFieldOrMethod(t, true, tset.pkg.Types, "UnmarshalBinary")
	unmarshal, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	return isMarshalBinary(t, marshal) && isUnmarshalBinary(t, unmarshal)
}

func isByteSlice(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	e, ok := s.Elem().(*types.Basic)
	if !ok {
		return false
	}
	return e.Kind() == types.Byte
}

// isMarshalBinary returns true if m is MarshalBinary() ([]byte, error).
func isMarshalBinary(t types.Type, m *types.Func) bool {
	if m.Name() != "MarshalBinary" {
		return false
	}
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return false
	}
	recv, args, results := sig.Recv(), sig.Params(), sig.Results()
	if args.Len() != 0 || results.Len() != 2 {
		return false
	}
	if !isByteSlice(results.At(0).Type()) {
		return false
	}
	if !isError(results.At(1).Type()) {
		return false
	}

	// We avoid complicated embeddings by requiring that the method is defined
	// on the type itself.
	//
	// TODO(mwhittaker): Relax this requirement if it becomes annoying.
	if p, ok := recv.Type().(*types.Pointer); ok {
		return types.Identical(p.Elem(), t)
	} else {
		return types.Identical(recv.Type(), t)
	}
}

// isUnmarshalBinary returns true if m is UnmarshalBinary([]byte) error
func isUnmarshalBinary(t types.Type, m *types.Func) bool {
	if m.Name() != "UnmarshalBinary" {
		return false
	}
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return false
	}
	recv, args, results := sig.Recv(), sig.Params(), sig.Results()
	if args.Len() != 1 || results.Len() != 1 {
		return false
	}
	if !isByteSlice(args.At(0).Type()) {
		return false
	}
	if !isError(results.At(0).Type()) {
		return false
	}

	// We avoid complicated embeddings by requiring that the method is defined
	// on the type itself.
	//
	// TODO(mwhittaker): Relax this requirement if it becomes annoying.
	if p, ok := recv.Type().(*types.Pointer); ok {
		return types.Identical(p.Elem(), t)
	} else {
		return types.Identical(recv.Type(), t)
	}
}

// isWeaverType returns true iff t is a named type from the weaver package with
// the specified name and n type arguments.
func isWeaverType(t types.Type, name string, n int) bool {
	named, ok := t.(*types.Named)
	return ok &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == weaverPackagePath &&
		named.Obj().Name() == name &&
		named.TypeArgs().Len() == n
}

func isWeaverImplements(t types.Type) bool {
	return isWeaverType(t, "Implements", 1)
}

func isWeaverRef(t types.Type) bool {
	return isWeaverType(t, "Ref", 1)
}

func isWeaverListener(t types.Type) bool {
	return isWeaverType(t, "Listener", 0)
}

func isWeaverMain(t types.Type) bool {
	return isWeaverType(t, "Main", 0)
}

func isWeaverWithRouter(t types.Type) bool {
	return isWeaverType(t, "WithRouter", 1)
}

func isWeaverAutoMarshal(t types.Type) bool {
	return isWeaverType(t, "AutoMarshal", 0)
}

func isWeaverNotRetriable(t types.Type) bool {
	return isWeaverType(t, "NotRetriable", 0)
}

func isString(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.String
}

func isContext(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg().Path() == "context" && n.Obj().Name() == "Context"
}

func isError(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() == nil && n.Obj().Name() == "error"
}

// isPrimitiveRouter returns whether the provided type is a valid primitive
// router type (i.e. an integer, a float, or a string).
func isPrimitiveRouter(t types.Type) bool {
	b, ok := t.(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.Float32, types.Float64, types.String:
		return true
	default:
		return false
	}
}

// isValidRouterType returns whether the provided type is a valid router type.
// A router type can be one of the following: an integer (signed or unsigned),
// a float, or a string. Alternatively, it can be a struct that may optioanly
// embed the weaver.AutoMarshal struct and rest of the fields must be either
// integers, floats, or strings.
func isValidRouterType(t types.Type) bool {
	t = t.Underlying()
	if isPrimitiveRouter(t) {
		return true
	}
	s, ok := t.(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < s.NumFields(); i++ {
		ft := s.Field(i).Type()
		if !isPrimitiveRouter(ft) && !isWeaverAutoMarshal(ft) {
			return false
		}
	}
	return true
}
