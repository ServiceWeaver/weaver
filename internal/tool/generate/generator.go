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
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ServiceWeaver/weaver/internal/files"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// TODO(rgrandl): Modify the generator code to use only the types package. Right
// now we are doing code generation relying both on the types and ast packages,
// which can be confusing and also we might do unnecessary work.

const (
	generatedCodeFile = "weaver_gen.go"

	Usage = `Generate code for a Service Weaver application.

Usage:
  weaver generate [packages]

Description:
  "weaver generate" generates code for the Service Weaver applications in the provided
  packages. For example, "weaver generate . ./foo" will generate code for the
  Service Weaver applications in the current directory and in the ./foo directory. For
  every package, the generated code is placed in a weaver_gen.go file in the
  package's directory.  For example, running "weaver generate . ./foo" will
  create ./weaver_gen.go and ./foo/weaver_gen.go.

  You specify packages for "weaver generate" in the same way you specify
  packagesfor go build, go test, go vet, etc. See "go help packages" for more
  information.

  Rather than invoking "weaver generate" directly, you can place a line of the
  following form in one of the .go files in the package:

      //go:generate weaver generate

  and then use the normal "go generate" command.

Examples:
  # Generate code for the package in the current directory.
  weaver generate

  # Same as "weaver generate".
  weaver generate .

  # Generate code for the package in the ./foo directory.
  weaver generate ./foo

  # Generate code for all packages in all subdirectories of current directory.
  weaver generate ./...`
)

// ErrorList holds a list of errors.
type ErrorList []error

func (list ErrorList) Error() string {
	var b strings.Builder
	for _, err := range list {
		fmt.Fprintln(&b, err.Error())
	}
	return b.String()
}

// Generate generates Service Weaver code for the specified packages.
// The list of supplied packages are treated similarly to the arguments
// passed to "go build" (see "go help packages" for details).
func Generate(dir string, pkgs []string) error {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode:      packages.NeedName | packages.NeedSyntax | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:       dir,
		Fset:      fset,
		ParseFile: parseNonWeaverGenFile,
	}
	pkgList, err := packages.Load(cfg, pkgs...)
	if err != nil {
		return err
	}

	var automarshals typeutil.Map
	var errs []error
	for _, p := range pkgList {
		g := &generator{
			pkg:     p,
			tset:    newTypeSet(p, &automarshals, &typeutil.Map{}),
			fileset: fset,
			errors:  nil,
		}
		g.processPackage(p)
		errs = append(errs, g.errors...)
	}
	if len(errs) != 0 {
		return ErrorList(errs)
	}
	return nil
}

// parseNonWeaverGenFile parses a Go file, expcept for weaver_gen.go files whose
// contents are ignored since those contents may reference types that no longer
// exist.
func parseNonWeaverGenFile(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
	if filepath.Base(filename) == generatedCodeFile {
		return parser.ParseFile(fset, filename, src, parser.PackageClauseOnly)
	}
	return parser.ParseFile(fset, filename, src, parser.ParseComments|parser.DeclarationErrors)
}

type generator struct {
	pkg            *packages.Package
	tset           *typeSet
	fileset        *token.FileSet
	errors         []error
	components     []*component
	types          []types.Type // all types that need to be serialized
	sizeFuncNeeded typeutil.Map // types that need a serviceweaver_size_* function
	generated      typeutil.Map // memo cache for generateEncDecMethodsFor
}

func (g *generator) addError(pos token.Pos, err error) {
	g.errors = append(g.errors, fmt.Errorf("%v: %w", g.fileset.Position(pos), err))
}

func (g *generator) errorf(pos token.Pos, format string, args ...interface{}) {
	g.addError(pos, fmt.Errorf(format, args...))
}

func (g *generator) processPackage(pkg *packages.Package) {
	// Abort if there are any errors loading the package.
	for _, err := range pkg.Errors {
		g.errors = append(g.errors, err)
	}
	if len(g.errors) > 0 {
		return
	}

	// Find all weaver.AutoMarshal annotations.
	for _, f := range pkg.Syntax {
		fname := g.fileset.Position(f.Package).Filename
		if filepath.Base(fname) == generatedCodeFile {
			continue
		}
		for _, t := range g.findAutoMarshals(f) {
			g.tset.automarshalCandidates.Set(t, struct{}{})
		}
	}

	for _, t := range g.tset.automarshalCandidates.Keys() {
		n := t.(*types.Named)
		if errs := g.tset.checkSerializable(n); len(errs) > 0 {
			g.errorf(n.Obj().Pos(), "type %v is not serializable", t)
			for _, err := range errs {
				g.addError(n.Obj().Pos(), err)
			}
		} else {
			g.tset.automarshals.Set(t, struct{}{})
		}
	}

	// Find and process all components.
	for _, f := range pkg.Syntax {
		fname := g.fileset.Position(f.Package).Filename
		if filepath.Base(fname) == generatedCodeFile {
			continue
		}
		g.findComponents(f)
	}

	if len(g.errors) == 0 && len(g.components)+g.tset.automarshalCandidates.Len() > 0 {
		g.generate()
	}
}

func (g *generator) findComponents(f *ast.File) {
	// Find declarations and their positions.
	decls := map[token.Pos]ast.Decl{}
	for _, d := range f.Decls {
		decls[d.Pos()] = d
	}

	// Find types that implement components. E.g., something that looks like:
	//	type something struct {
	//		weaver.Implements[SomeComponentType]
	//		...
	//	}
	for _, d := range f.Decls {
		gendecl, ok := d.(*ast.GenDecl)
		if !ok || gendecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range gendecl.Specs {
			g.processComponentImplementation(f, spec)
		}
	}
}

// findAutoMarshals returns the types in the provided file which embed the
// weaver.AutoMarshal struct.
func (g *generator) findAutoMarshals(f *ast.File) []*types.Named {
	var automarshals []*types.Named
	for _, decl := range f.Decls {
		gendecl, ok := decl.(*ast.GenDecl)
		if !ok || gendecl.Tok != token.TYPE {
			// This is not a type declaration.
			continue
		}

		// Recall that a type declaration can have multiple type specs. We have
		// to iterate over all of them. For example:
		//
		//     type (
		//         a struct{} // Spec 1
		//         b struct{} // Spec 2
		//     )
		for _, spec := range gendecl.Specs {
			pos := spec.Pos()
			position := g.fileset.Position(pos)
			typespec, ok := spec.(*ast.TypeSpec)
			if !ok {
				panic(fmt.Errorf("%v: type declaration has non-TypeSpec spec: %v", position, spec))
			}

			// Extract the type's name.
			def, ok := g.pkg.TypesInfo.Defs[typespec.Name]
			if !ok {
				panic(fmt.Errorf("%v: name %v not found", position, typespec.Name))
			}

			// Check for an embedded AutoMarshal.
			n, ok := def.Type().(*types.Named)
			if !ok {
				// For type aliases like `type Int = int`, Int has type int and
				// not type Named. We ignore these.
				continue
			}
			t, ok := g.typeof(typespec.Type).(*types.Struct)
			if !ok {
				continue
			}
			automarshal := false
			for i := 0; i < t.NumFields(); i++ {
				f := t.Field(i)
				if f.Embedded() && isWeaverAutoMarshal(f.Type()) {
					automarshal = true
					break
				}
			}
			if !automarshal {
				continue
			}

			// Ignore generic types. Generic types don't play well with
			// embedded AutoMarshals. For example, consider the following type
			// declaration:
			//
			//     type Register[A any] struct {
			//         weaver.AutoMarshal
			//         a A
			//     }
			//
			// Is Register[A] serializable? It depends on A. Plus, we cannot
			// really generate WeaverMarshal and WeaverUnmarshal methods for
			// specific instantiations of Register[A]. Because of these
			// complications, we ignore generic types.
			//
			// TODO(mwhittaker): Handle generics somehow?
			if n.TypeParams() != nil { // generics have non-nil TypeParams()
				name := g.tset.typeString(n)
				g.addError(pos, fmt.Errorf("generic struct %v cannot embed weaver.AutoMarshal. See serviceweaver.dev/docs.html#serializable-types for more information.", name))
				continue
			}

			automarshals = append(automarshals, n)
		}
	}
	return automarshals
}

// typeof returns the type of the provided expression.
func (g *generator) typeof(e ast.Expr) types.Type {
	return g.pkg.TypesInfo.Types[e].Type
}

func (g *generator) processComponentImplementation(file *ast.File, spec ast.Spec) {
	t, ok := spec.(*ast.TypeSpec)
	if !ok {
		return
	}
	implName := t.Name.Name
	s, ok := t.Type.(*ast.StructType)
	if !ok {
		return
	}

	var componentType *types.Named // The component interface type
	var routerType *types.Named    // Router implementation (if any)
	var hasConfig bool             // Does struct contain weaver.WithConfig[] field?

	for _, f := range s.Fields.List {
		if len(f.Names) != 0 {
			continue // Only an embedded field counts
		}
		typeAndValue, ok := g.tset.pkg.TypesInfo.Types[f.Type]
		if !ok {
			continue
		}
		t := typeAndValue.Type

		switch {
		case isWeaverImplements(t):
			arg := t.(*types.Named).TypeArgs().At(0)
			cn, ok := arg.(*types.Named)
			if !ok {
				g.errorf(spec.Pos(), "weaver.Implements argument is not a named type.")
				return
			}
			if cn.Obj().Pkg() != g.tset.pkg.Types {
				g.errorf(spec.Pos(), "weaver.Implements argument %s is a type outside the current package.", g.tset.typeString(cn))
				return
			}
			if _, ok = cn.Underlying().(*types.Interface); !ok {
				g.errorf(f.Pos(), "weaver.Implements argument %s is not an interface.", g.tset.typeString(cn))
				return
			}
			componentType = cn
		case isWeaverWithConfig(t):
			hasConfig = true
		case isWeaverWithRouter(t):
			arg := t.(*types.Named).TypeArgs().At(0)
			rt, ok := arg.(*types.Named)
			if !ok {
				g.errorf(f.Pos(), "weaver.WithRouter argument %s must be a type with routing methods", arg)
				return
			}
			routerType = rt
			continue
		}
	}
	if componentType == nil {
		return
	}

	if t.TypeParams != nil && t.TypeParams.NumFields() != 0 {
		g.errorf(spec.Pos(), "component implementation cannot be generic")
		return
	}

	comp := &component{
		name:      componentType.Obj().Name(),
		pos:       spec.Pos(),
		fullName:  filepath.Join(componentType.Obj().Pkg().Path(), componentType.Obj().Name()),
		implName:  implName,
		intf:      componentType.Underlying().(*types.Interface),
		file:      file,
		router:    routerType,
		hasConfig: hasConfig,
	}
	g.processMethods(comp)
	if len(g.errors) > 0 {
		return
	}
	if len(comp.methods) == 0 {
		g.errorf(spec.Pos(), "Implemented component type %s has no exported methods (must export at least one method).", g.tset.typeString(componentType))
		return
	}
	g.components = append(g.components, comp)
}

type component struct {
	name          string           // component interface name
	pos           token.Pos        // Location of component implementation
	fullName      string           // package-prefixed component interface name
	implName      string           // name of the component implementation type
	intf          *types.Interface // component's interface type
	file          *ast.File        // file that contains component's implementation
	methods       []*types.Func
	router        *types.Named    // router type for the component, or nil if there is no router.
	hasConfig     bool            // True iff implementation contains a weaver.WithConfig field.
	routingKey    types.Type      // routing key, or nil if there is no router.
	routedMethods map[string]bool // the set of methods with a routing function
}

// processMethods fills in the method information for the given component.
func (g *generator) processMethods(comp *component) {
	pretty := g.tset.typeString
	for i := 0; i < comp.intf.NumMethods(); i++ {
		m := comp.intf.Method(i)
		if !m.Exported() {
			continue
		}
		mt, ok := m.Type().(*types.Signature)
		if !ok || mt == nil { // Should never happen.
			g.errorf(m.Pos(), "method %s doesn't have a signature", m.Name())
			continue
		}

		// First argument must be context.Context.
		bad := func(bad, format string, arg ...any) string {
			msg := fmt.Sprintf(format, arg...)
			return fmt.Sprintf(
				"Method `%s%s %s` of Service Weaver component %q has incorrect %s types. %s",
				m.Name(), pretty(mt.Params()), pretty(mt.Results()), comp.name, bad, msg)
		}
		if mt.Params().Len() < 1 || !isContext(mt.Params().At(0).Type()) {
			g.errorf(m.Pos(), bad("argument", "The first argument must have type context.Context."))
			continue
		}

		// All arguments but context.Context must be serializable.
		for i := 1; i < mt.Params().Len(); i++ {
			arg := mt.Params().At(i)
			errs := g.tset.checkSerializable(arg.Type())
			for _, err := range errs {
				g.addError(arg.Pos(), err)
			}
			if len(errs) > 0 {
				// TODO(mwhittaker): Print a link to documentation on which types are serializable.
				g.errorf(m.Pos(), bad("argument",
					"Argument %d has type %v, which is not serializable. All arguments, besides the initial context.Context, must be serializable.",
					i, pretty(arg.Type())))
			}
			g.types = append(g.types, arg.Type())
		}

		// Last result must be error.
		if mt.Results().Len() < 1 || mt.Results().At(mt.Results().Len()-1).Type().String() != "error" {
			// TODO(mwhittaker): If the function doesn't return anything, don't
			// print mt.Results.
			g.errorf(m.Pos(), bad("return", "The last return must have type error."))
			continue
		}

		// All results but error must be serializable.
		for i := 0; i < mt.Results().Len()-1; i++ {
			res := mt.Results().At(i)
			for _, err := range g.tset.checkSerializable(res.Type()) {
				g.addError(res.Pos(), err)
			}
			g.types = append(g.types, res.Type())
		}

		comp.methods = append(comp.methods, m)
	}

	// Find routing information if needed.
	routingKey, routedMethods, err := g.routerMethods(comp)
	if err != nil {
		g.errorf(comp.pos, err.Error())
	}
	comp.routingKey = routingKey
	comp.routedMethods = routedMethods

	// Sort into deterministic order.
	loc := func(pos token.Pos) (string, int) {
		p := g.fileset.Position(pos)
		return p.Filename, p.Offset
	}
	sort.Slice(comp.methods, func(i, j int) bool {
		file1, offset1 := loc(comp.methods[i].Pos())
		file2, offset2 := loc(comp.methods[j].Pos())
		if file1 != file2 {
			return file1 < file2
		}
		return offset1 < offset2
	})
}

// routerMethods returns the routing key and the set of routed methods for comp.
//
// A developer can annotate a Service Weaver component with a router, like this:
//
//	type foo struct {
//		weaver.Implements[Foo]
//		weaver.WithRouter[fooRouter]
//		...
//	}
//	func (*foo) A(context.Context) error {...}
//	func (*foo) B(context.Context, int) error {...}
//	func (*foo) C(context.Context, int) error {...}
//
//	type fooRouter struct{}
//	func (fooRouter) A(context.Context) int {...}
//	func (fooRouter) B(context.Context, int) int {...}
func (g *generator) routerMethods(comp *component) (types.Type, map[string]bool, error) {
	pretty := g.tset.typeString
	// XXX Use g.errorf() so we print the correct position in error messages.
	router := comp.router
	if router == nil {
		return nil, nil, nil
	}

	// Get all component methods.
	componentMethods := map[string]*types.Signature{}
	for _, m := range comp.methods {
		componentMethods[m.Name()] = m.Type().(*types.Signature)
	}

	// Verify that every router method corresponds to a component method
	// (this is so we avoid errors where one has been renamed but not the other).
	// Also check that they all have the return type.
	var routingKey types.Type
	routedMethods := map[string]bool{}
	for i, n := 0, router.NumMethods(); i < n; i++ {
		m := router.Method(i)
		name := m.Name()
		componentMethod, ok := componentMethods[name]
		if !ok {
			return nil, nil, fmt.Errorf("Routing function %q does not match any method of %q.",
				name, comp.name)
		}
		mt := m.Type().(*types.Signature)

		// Router method args must match component method args.
		if !types.Identical(mt.Params(), componentMethod.Params()) {
			return nil, nil, fmt.Errorf("Component %q method arguments %s do not match router method arguments %s",
				name, pretty(componentMethod.Params()), pretty(mt.Params()))
		}

		// All router methods must have the same routable return type.
		if mt.Results().Len() != 1 {
			return nil, nil, fmt.Errorf("Routing function %q must return exactly one value (it returns %d)", m.Name(), mt.Results().Len())
		}
		ret := mt.Results().At(0).Type()
		if i == 0 {
			if !isValidRouterType(ret) {
				return nil, nil, fmt.Errorf("Router method %q has invalid routing key type %q. A routing key type should be an integer, float, string, or a struct with every field being an integer, float, or string.",
					name, pretty(ret))
			}
			routingKey = ret
		} else if !types.Identical(ret, routingKey) {
			return nil, nil, fmt.Errorf("Return type of %q (%s) does not match previously seen routing key type (%s)",
				m.Name(), pretty(ret), pretty(routingKey))
		}

		routedMethods[name] = true
	}

	if routingKey == nil {
		return nil, nil, fmt.Errorf("No routing methods found on declarated router type (%s) for component %q",
			router.Obj().Name(), comp.name)
	}

	return routingKey, routedMethods, nil
}

type printFn func(format string, args ...interface{})

func (g *generator) generate() {
	// Process components in deterministic order.
	sort.Slice(g.components, func(i, j int) bool {
		return g.components[i].name < g.components[j].name
	})

	// Generate the file body.
	var body bytes.Buffer
	{
		fn := func(format string, args ...interface{}) {
			fmt.Fprintln(&body, fmt.Sprintf(format, args...))
		}
		g.generateRegisteredComponents(fn)
		g.generateLocalStubs(fn)
		g.generateClientStubs(fn)
		g.generateServerStubs(fn)
		g.generateAutoMarshalMethods(fn)
		g.generateRouterMethods(fn)
		g.generateEncDecMethods(fn)

		// append the size methods
		if g.sizeFuncNeeded.Len() > 0 {
			fn(`// Size implementations.`)
			fn(``)
			keys := g.sizeFuncNeeded.Keys()
			sort.Slice(keys, func(i, j int) bool {
				x, y := keys[i], keys[j]
				return x.String() < y.String()
			})
			for _, t := range keys {
				g.generateSizeFunction(fn, t)
			}
		}
	}

	// Generate the file header. This should be done at the very end to ensure
	// that all types added to the body have been imported.
	var header bytes.Buffer
	{
		fn := func(format string, args ...interface{}) {
			fmt.Fprintln(&header, fmt.Sprintf(format, args...))
		}
		g.generateImports(fn)
	}

	// Create a generated file.
	filename := filepath.Join(g.pkgDir(), generatedCodeFile)
	dst := files.NewWriter(filename)
	defer dst.Cleanup()

	fmtAndWrite := func(buf bytes.Buffer) {
		// Format the code.
		b := buf.Bytes()
		if formatted, err := format.Source(b); err != nil {
			// If format.Source fails, we write out the unformatted code. If it
			// succeeds, we write out the formatted code.
			g.errors = append(g.errors, err)
		} else {
			b = formatted
		}

		// Write to dst.
		if _, err := io.Copy(dst, bytes.NewReader(b)); err != nil {
			g.errors = append(g.errors, err)
		}
	}

	fmtAndWrite(header)
	fmtAndWrite(body)

	if err := dst.Close(); err != nil {
		g.errors = append(g.errors, err)
	}
}

// pkgDir returns the directory of the package.
func (g *generator) pkgDir() string {
	if len(g.pkg.Syntax) == 0 {
		panic(fmt.Errorf("package %v has no source files", g.pkg))
	}
	f := g.pkg.Syntax[0]
	fname := g.fileset.Position(f.Package).Filename
	return filepath.Dir(fname)
}

// generateImports generates code to import all the dependencies.
func (g *generator) generateImports(p printFn) {
	p("package %s", g.pkg.Name)
	p("")
	p(`// Code generated by "weaver generate". DO NOT EDIT.`)
	p(`import (`)
	for _, imp := range g.tset.imports() {
		if imp.alias == "" {
			p(`	%s`, strconv.Quote(imp.path))
		} else {
			p(`	%s %s`, imp.alias, strconv.Quote(imp.path))
		}
	}
	p(`)`)
}

// generateRegisteredComponents generates code that registers the components with Service Weaver.
func (g *generator) generateRegisteredComponents(p printFn) {
	if len(g.components) == 0 {
		return
	}

	g.tset.importPackage("context", "context")
	p(``)
	p(`func init() {`)
	for _, comp := range g.components {
		name := comp.name

		// E.g.,
		//   func(impl any, caller string, tracer trace.Tracer) any {
		//       return foo_local_stub{imple: impl.(Foo), tracer: tracer, ...}
		//   }
		localStubFn := fmt.Sprintf(`func(impl any, tracer %v) any { return %s_local_stub{impl: impl.(%s), tracer: tracer } }`, g.trace().qualify("Tracer"), notExported(name), name)

		// E.g.,
		//   func(stub *codegen.Stub, caller string) any {
		//       return Foo_stub{stub: stub, ...}
		//   }
		var b strings.Builder
		for _, m := range comp.methods {
			fmt.Fprintf(&b, ", %sMetrics: %s(%s{Caller: caller, Component: %q, Method: %q})", notExported(m.Name()), g.codegen().qualify("MethodMetricsFor"), g.codegen().qualify("MethodLabels"), comp.fullName, m.Name())
		}
		clientStubFn := fmt.Sprintf(`func(stub %s, caller string) any { return %s_client_stub{stub: stub %s } }`,
			g.codegen().qualify("Stub"), notExported(name), b.String())

		// E.g.,
		//   func(impl any, addLoad func(uint64, float64)) codegen.Server {
		//       return foo_server_stub{impl: impl.(Foo), addLoad: addLoad}
		//   }
		serverStubFn := fmt.Sprintf(`func(impl any, addLoad func(uint64, float64)) %s { return %s_server_stub{impl: impl.(%s), addLoad: addLoad } }`, g.codegen().qualify("Server"), notExported(name), name)

		// E.g.,
		//	weaver.Register(weaver.Registration{
		//	    Props: codegen.ComponentProperties{},
		//	    ...,
		//	})
		reflect := g.tset.importPackage("reflect", "reflect")
		p(`	%s(%s{`, g.codegen().qualify("Register"), g.codegen().qualify("Registration"))
		p(`		Name: %q,`, comp.fullName)
		// To get a reflect.Type for an interface, we have to first get a type
		// of its pointer and then resolve the underlying type. See:
		//   https://pkg.go.dev/reflect#example-TypeOf
		p(`		Iface: %s((*%s)(nil)).Elem(),`, reflect.qualify("TypeOf"), name)
		p(`		New: func() any { return &%s{} },`, comp.implName)
		if comp.hasConfig {
			p(`		ConfigFn: func(i any) any { return i.(*%s).WithConfig.Config() },`, comp.implName)
		}
		if comp.router != nil {
			p(`		Routed: true,`)
		}
		p(`		LocalStubFn: %s,`, localStubFn)
		p(`		ClientStubFn: %s,`, clientStubFn)
		p(`		ServerStubFn: %s,`, serverStubFn)
		p(`	})`)
	}
	p(`}`)
}

// generateLocalStubs generates code that creates stubs for the local components.
func (g *generator) generateLocalStubs(p printFn) {
	p(``)
	p(``)
	p(`// Local stub implementations.`)

	var b strings.Builder
	for _, comp := range g.components {
		stub := notExported(comp.name) + "_local_stub"
		p(``)
		p(`type %s struct{`, stub)
		p(`	impl %s`, comp.name)
		p(`	tracer %s`, g.trace().qualify("Tracer"))
		p(`}`)
		for _, m := range comp.methods {
			mt := m.Type().(*types.Signature)
			{
				// Make list of args for function signature.
				b.Reset()
				for i := 1; i < mt.Params().Len(); i++ { // Skip initial context.Context
					at := mt.Params().At(i).Type()
					fmt.Fprintf(&b, ", a%d %s", i-1, g.tset.genTypeString(at))
				}
				argList := b.String()

				// Make list of results for function signature.
				b.Reset()
				for i := 0; i < mt.Results().Len()-1; i++ { // Skip final error
					rt := mt.Results().At(i).Type()
					fmt.Fprintf(&b, "r%d %s, ", i, g.tset.genTypeString(rt))
				}
				resultList := b.String()

				p(``)
				p(`func (s %s) %s(ctx context.Context%s) (%serr error) {`,
					stub,
					m.Name(),
					argList,
					resultList,
				)
			}

			// Create a child span iff tracing is enabled in ctx.
			p(`	span := %s(ctx)`, g.trace().qualify("SpanFromContext"))
			p(`	if span.SpanContext().IsValid() {`)
			p(`		// Create a child span for this method.`)
			p(`		ctx, span = s.tracer.Start(ctx, "%s.%s.%s", trace.WithSpanKind(trace.SpanKindInternal))`, g.pkg.Name, comp.name, m.Name())
			p(`		defer func() {`)
			p(`			if err != nil {`)
			p(`				span.RecordError(err)`)
			p(`				span.SetStatus(%s, err.Error())`, g.codes().qualify("Error"))
			p(`			}`)
			p(`			span.End()`)
			p(`		}()`)
			p(`	}`)

			// Call the local method.
			b.Reset()
			fmt.Fprintf(&b, "ctx")
			for i := 1; i < mt.Params().Len(); i++ {
				fmt.Fprintf(&b, ", a%d", i-1)
			}
			argList := b.String()
			p(``)
			p(`	return s.impl.%s(%s)`, m.Name(), argList)
			p(`}`)
		}
	}
}

// generateClientStubs generates code that creates client stubs for the registered components.
func (g *generator) generateClientStubs(p printFn) {
	p(``)
	p(``)
	p(`// Client stub implementations.`)

	var b strings.Builder
	for _, comp := range g.components {
		stub := notExported(comp.name) + "_client_stub"
		p(``)
		p(`type %s struct{`, stub)
		p(`	stub %s`, g.codegen().qualify("Stub"))
		for _, m := range comp.methods {
			p(`	%sMetrics *%s`, notExported(m.Name()), g.codegen().qualify("MethodMetrics"))
		}
		p(`}`)

		// Assign method indices in sorted order.
		mlist := make([]string, len(comp.methods))
		for i, m := range comp.methods {
			mlist[i] = m.Name()
		}
		sort.Strings(mlist)
		methodIndex := make(map[string]int, len(mlist))
		for i, m := range mlist {
			methodIndex[m] = i
		}

		for _, m := range comp.methods {
			mt := m.Type().(*types.Signature)

			// Make list of args for function signature.
			b.Reset()
			for i := 1; i < mt.Params().Len(); i++ { // Skip initial context.Context
				at := mt.Params().At(i).Type()
				fmt.Fprintf(&b, ", a%d %s", i-1, g.tset.genTypeString(at))
			}
			argList := b.String()

			// Make list of results for function signature.
			b.Reset()
			for i := 0; i < mt.Results().Len()-1; i++ { // Skip final error
				rt := mt.Results().At(i).Type()
				fmt.Fprintf(&b, "r%d %s, ", i, g.tset.genTypeString(rt))
			}
			resultList := b.String()

			p(``)
			p(`func (s %s) %s(ctx context.Context%s) (%serr error) {`,
				stub,
				m.Name(),
				argList,
				resultList,
			)

			// Update metrics.
			p(`	// Update metrics.`)
			p(`	start := %s()`, g.time().qualify("Now"))
			p(`	s.%sMetrics.Count.Add(1)`, notExported(m.Name()))
			p(``)

			// Create a child span iff tracing is enabled in ctx.
			p(`	span := %s(ctx)`, g.trace().qualify("SpanFromContext"))
			p(`	if span.SpanContext().IsValid() {`)
			p(`		// Create a child span for this method.`)
			p(`		ctx, span = s.stub.Tracer().Start(ctx, "%s.%s.%s", trace.WithSpanKind(trace.SpanKindClient))`, g.pkg.Name, comp.name, m.Name())
			p(`	}`)

			// Handle cleanup.
			p(``)
			p(`	defer func() {`)
			p(`		// Catch and return any panics detected during encoding/decoding/rpc.`)
			p(`		if err == nil {`)
			p(`			err = %s(recover())`, g.codegen().qualify("CatchPanics"))
			p(`		}`)
			p(`		err = s.stub.WrapError(err)`)
			p(``)
			p(`		if err != nil {`)
			p(`			span.RecordError(err)`)
			p(`			span.SetStatus(%s, err.Error())`, g.codes().qualify("Error"))
			p(`			s.%sMetrics.ErrorCount.Add(1)`, notExported(m.Name()))
			p(`		}`)
			p(`		span.End()`)
			p(``)
			p(`		s.%sMetrics.Latency.Put(float64(time.Since(start).Microseconds()))`, notExported(m.Name()))
			p(`	}()`)
			p(``)

			preallocated := false
			if mt.Params().Len() > 1 {
				// Preallocate a perfectly sized buffer if possible.
				canPreallocate := true
				for i := 1; i < mt.Params().Len(); i++ { // Skip initial context.Context
					if !g.preallocatable(mt.Params().At(i).Type()) {
						canPreallocate = false
						break
					}
				}
				if canPreallocate {
					p("")
					p("	// Preallocate a buffer of the right size.")
					p("	size := 0")
					for i := 1; i < mt.Params().Len(); i++ {
						at := mt.Params().At(i).Type()
						p("	size += %s", g.size(fmt.Sprintf("a%d", i-1), at))
					}
					p("	enc := %s", g.codegen().qualify("NewEncoder()"))
					p("	enc.Reset(size)")
					preallocated = true
				}
			}

			// Invoke call.Encode.
			b.Reset()
			if mt.Params().Len() > 1 {
				p(``)
				p(`	// Encode arguments.`)
				if !preallocated {
					p("	enc := %s", g.codegen().qualify("NewEncoder()"))
				}
			}
			for i := 1; i < mt.Params().Len(); i++ { // Skip initial context.Context
				at := mt.Params().At(i).Type()
				arg := fmt.Sprintf("a%d", i-1)
				p(`	%s`, g.encode("enc", arg, at))
			}

			// Set the routing key, if there is one.
			if comp.routedMethods[m.Name()] {
				p(``)
				p(`	// Set the shardKey.`)
				p(`     var r %s`, g.tset.genTypeString(comp.router))
				n := mt.Params().Len()
				args := make([]string, n)
				args[0] = "ctx"
				for i := 1; i < n; i++ {
					args[i] = fmt.Sprintf("a%d", i-1)
				}
				p(`	shardKey := _hash%s(r.%s(%s))`, exported(comp.name), m.Name(), strings.Join(args, ", "))
			} else {
				p(`	var shardKey uint64`)
			}

			// Invoke call.Run.
			p(``)
			p(`	// Call the remote method.`)
			data := "nil"
			if mt.Params().Len() > 1 {
				data = "enc.Data()"
				p(`	s.%sMetrics.BytesRequest.Put(float64(len(enc.Data())))`, notExported(m.Name()))
			} else {
				p(`	s.%sMetrics.BytesRequest.Put(0)`, notExported(m.Name()))
			}
			p(`	var results []byte`)
			p(`	results, err = s.stub.Run(ctx, %d, %s, shardKey)`, methodIndex[m.Name()], data)
			p(`	if err != nil {`)
			p(`		return`)
			p(`	}`)
			p(`	s.%sMetrics.BytesReply.Put(float64(len(results)))`, notExported(m.Name()))

			// Invoke call.Decode.
			b.Reset()
			p(``)
			p(`	// Decode the results.`)
			p(`	dec := %s(results)`, g.codegen().qualify("NewDecoder"))
			for i := 0; i < mt.Results().Len()-1; i++ { // Skip final error
				rt := mt.Results().At(i).Type()
				res := fmt.Sprintf("r%d", i)
				if x, ok := rt.(*types.Pointer); ok && (g.tset.isProto(x) || g.tset.hasMarshalBinary(x)) {
					// To decode a pointer *t where t is a proto or
					// BinaryUnmarshaler, we need to instantiate a zero value
					// of type t before calling the appropriate decoding
					// function. For all other types, this is unnecessary.
					tmp := fmt.Sprintf("tmp%d", i)
					p(`	var %s %s`, tmp, g.tset.genTypeString(x.Elem()))
					p(`	%s`, g.decode("dec", ref(tmp), x.Elem()))
					p(`	%s = %s`, res, ref(tmp))
				} else {
					p(`	%s`, g.decode("dec", ref(res), rt))
				}
			}
			p(`	err = dec.Error()`)

			p(`	return`)
			p(`}`)
		}
	}
}

// preallocatable returns whether we can preallocate a buffer of the right size
// to encode the provided type.
func (g *generator) preallocatable(t types.Type) bool {
	return g.tset.isMeasurable(t) && g.isWeaverEncoded(t)
}

// isWeaverEncoded returns whether the provided type is encoded using the
// Service Weaver encoding logic. Most serializable types use the
// Service Weaver encoding format, but some types default to using proto
// serialization or MarshalBinary methods instead.
//
// REQUIRES: t is serializable.
func (g *generator) isWeaverEncoded(t types.Type) bool {
	if g.tset.isProto(t) || g.tset.hasMarshalBinary(t) {
		return false
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
			return true
		default:
			panic(fmt.Sprintf("isWeaverEncoded: unexpected type: %v", t))
		}

	case *types.Pointer:
		return g.isWeaverEncoded(x.Elem())

	case *types.Array:
		return g.isWeaverEncoded(x.Elem())

	case *types.Slice:
		return g.isWeaverEncoded(x.Elem())

	case *types.Map:
		return g.isWeaverEncoded(x.Key()) && g.isWeaverEncoded(x.Elem())

	case *types.Struct:
		for i := 0; i < x.NumFields(); i++ {
			f := x.Field(i)
			if !g.isWeaverEncoded(f.Type()) {
				return false
			}
		}
		return true

	case *types.Named:
		if s, ok := x.Underlying().(*types.Struct); ok {
			for i := 0; i < s.NumFields(); i++ {
				f := s.Field(i)
				if !g.isWeaverEncoded(f.Type()) {
					return false
				}
			}
			return true
		}
		return g.isWeaverEncoded(x.Underlying())

	default:
		panic(fmt.Sprintf("size: unexpected type %v", t))
	}
}

// size returns a go expression that evaluates to the size of the provided
// expression e of the provided type t.
//
// REQUIRES: t is serializable, measurable, serviceweaver-encoded.
func (g *generator) size(e string, t types.Type) string {
	g.findSizeFuncNeededs(t)

	// size(e: basic type t) = fixedsize(t)
	// size(e: string) = 4 + len(e)
	// size(e: *t) = serviceweaver_size_ptr_t(e)
	// size(e: [N]t) = 4 + len(e) * fixedsize(t)
	// size(e: []t) = 4 + len(e) * fixedsize(t)
	// size(e: map[k]v) = 4 + len(e) * (fixedsize(k) + fixedsize(v))
	// size(e: struct{...}) = serviceweaver_size_struct_XXXXXXXX(e)
	// size(e: weaver.AutoMarshal) = 0
	// size(e: type t struct{...}) = serviceweaver_size_t(e)
	// size(e: type t u) = size(e: u)

	var f func(e string, t types.Type) string
	f = func(e string, t types.Type) string {
		switch x := t.(type) {
		case *types.Basic:
			switch x.Kind() {
			case types.Bool,
				types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
				types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
				types.Float32, types.Float64,
				types.Complex64, types.Complex128:
				return strconv.Itoa(g.tset.sizeOfType(t))
			case types.String:
				return fmt.Sprintf("(4 + len(%s))", e)
			default:
				panic(fmt.Sprintf("size: unexpected expression: %v", e))
			}

		case *types.Pointer:
			return fmt.Sprintf("serviceweaver_size_%s(%s)", sanitize(t), e)

		case *types.Array:
			return fmt.Sprintf("(4 + (len(%s) * %d))", e, g.tset.sizeOfType(x.Elem()))

		case *types.Slice:
			return fmt.Sprintf("(4 + (len(%s) * %d))", e, g.tset.sizeOfType(x.Elem()))

		case *types.Map:
			keySize := g.tset.sizeOfType(x.Key())
			elemSize := g.tset.sizeOfType(x.Elem())
			return fmt.Sprintf("(4 + (len(%s) * (%d + %d)))", e, keySize, elemSize)

		case *types.Struct:
			return fmt.Sprintf("serviceweaver_size_%s(&%s)", sanitize(t), e)

		case *types.Named:
			if isWeaverAutoMarshal(x) {
				// Avoid generating unnecessary serviceweaver_size_Automarshal
				// functions that always return 0.
				//
				// TODO(mwhittaker): This yields a `size += 0` line in the
				// generated code. Don't produce those lines.
				return "0"
			} else if _, ok := x.Underlying().(*types.Struct); ok {
				return fmt.Sprintf("serviceweaver_size_%s(&%s)", sanitize(t), e)
			}
			return f(e, x.Underlying())

		default:
			panic(fmt.Sprintf("size: unexpected expression: %v", e))
		}
	}
	return f(e, t)
}

// findSizeFuncNeededs finds any nested types within the provided type that
// require a weaver generated size function.
//
// We can compute the size of most measurable types without calling a function.
// For example, the size of a string s is just len(s). However, computing the
// size of a pointer or a struct benefits from having a separate size function:
//
// The size of a pointer p of type *t is 1 if p is nil or 1 + size(t) if p is
// not nil. Because go doesn't have a ternary operator, it's convenient to pull
// this logic into a function.
//
// The size of a struct is the sum of the sizes of its fields. We could compute
// this without a separate function, but deeply nested structs would yield a
// very large expression to compute the sum. For example, consider the
// following types:
//
//	type A struct { x, y B }
//	type B struct { x, y C }
//	type C struct { x, y D }
//	type D struct { x, y int }
//
// The size of x of type A would be
//
//	len(x.x.x.x) + len(x.x.x.y) + len(x.x.y.x) + len(x.x.y.y) +
//	len(x.y.x.y) + len(x.y.x.y) + len(x.y.y.x) + len(x.y.y.y) +
//	len(y.x.x.x) + len(y.x.x.y) + len(y.x.y.x) + len(y.x.y.y) +
//	len(y.y.x.y) + len(y.y.x.y) + len(y.y.y.x) + len(y.y.y.y)
//
// REQUIRES: t is serializable and measurable.
func (g *generator) findSizeFuncNeededs(t types.Type) {
	var f func(t types.Type)
	f = func(t types.Type) {
		switch x := t.(type) {
		case *types.Pointer:
			g.sizeFuncNeeded.Set(t, true)
			f(x.Elem())

		case *types.Array:
			f(x.Elem())

		case *types.Slice:
			f(x.Elem())

		case *types.Map:
			f(x.Key())
			f(x.Elem())

		case *types.Struct:
			g.sizeFuncNeeded.Set(t, true)
			for i := 0; i < x.NumFields(); i++ {
				f(x.Field(i).Type())
			}

		case *types.Named:
			if isWeaverAutoMarshal(x) {
				return
			}
			if s, ok := x.Underlying().(*types.Struct); ok {
				g.sizeFuncNeeded.Set(t, true)
				for i := 0; i < s.NumFields(); i++ {
					f(s.Field(i).Type())
				}
			} else {
				f(x.Underlying())
			}
		}
	}
	f(t)
}

// generateSizeFunction prints, using p, a go function that returns the size of
// the serialization of a value of the provided type.
//
// REQUIRES: t is a type found by findSizeFuncNeededs.
func (g *generator) generateSizeFunction(p printFn, t types.Type) {
	p("// serviceweaver_size_%s returns the size (in bytes) of the serialization", sanitize(t))
	p("// of the provided type.")

	switch x := t.(type) {
	case *types.Pointer:
		// For example:
		//
		//     func serviceweaver_size_ptr_string(x *string) int {
		//         if x == nil {
		//             return 1
		//         } else {
		//             return 1 + len(*x)
		//         }
		//     }
		p("func serviceweaver_size_%s(x %s) int {", sanitize(t), g.tset.genTypeString(t))
		p("	if x == nil {")
		p("		return 1")
		p("	} else {")
		p("		return 1 + %s", g.size("*x", x.Elem()))
		p("	}")
		p("}")

	case *types.Named:
		// For example:
		//
		//     type A struct { x int, y string }
		//
		//     func serviceweaver_size_ptr_A(x *A) int {
		//         size := 0
		//         size += 8
		//         size += len(x.y)
		//         return size
		//     }
		s := x.Underlying().(*types.Struct)
		p("func serviceweaver_size_%s(x *%s) int {", sanitize(t), g.tset.genTypeString(t))
		p("	size := 0")
		for i := 0; i < s.NumFields(); i++ {
			f := s.Field(i)
			p("	size += %s", g.size(fmt.Sprintf("x.%s", f.Name()), f.Type()))
		}
		p("	return size")
		p("}")

	case *types.Struct:
		// Same as Named.
		p("func serviceweaver_size_%s(x *%s) int {", sanitize(t), g.tset.genTypeString(t))
		p("	size := 0")
		for i := 0; i < x.NumFields(); i++ {
			f := x.Field(i)
			p("	size += %s", g.size(fmt.Sprintf("x.%s", f.Name()), f.Type()))
		}
		p("	return size")
		p("}")

	default:
		panic(fmt.Sprintf("generateSizeFunction: unexpected type: %v", t))
	}
}

// generateServerStubs generates code that creates server stubs for the registered components.
func (g *generator) generateServerStubs(p printFn) {
	p(``)
	p(``)
	p(`// Server stub implementations.`)

	var b strings.Builder

	for _, comp := range g.components {
		stub := fmt.Sprintf("%s_server_stub", notExported(comp.name))
		p(``)
		p(`type %s struct{`, stub)
		p(`	impl %s`, comp.name)
		p(`	addLoad func(key uint64, load float64)`)
		p(`}`)
		p(``)
		p(`// GetStubFn implements the stub.Server interface.`)
		p(`func (s %s) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {`, stub)
		p(`	switch method {`)
		for _, m := range comp.methods {
			p(`	case "%s":`, m.Name())
			p(`		return s.%s`, notExported(m.Name()))
		}
		p(`	default:`)
		p(`		return nil`)
		p(`	}`)
		p(`}`)

		// Generate server stub implementation for the methods exported by the component.
		for _, m := range comp.methods {
			mt := m.Type().(*types.Signature)

			p(``)
			p(`func (s %s) %s(ctx context.Context, args []byte) (res []byte, err error) {`,
				stub, notExported(m.Name()))

			// Handle errors triggered during execution.
			p(`	// Catch and return any panics detected during encoding/decoding/rpc.`)
			p(`	defer func() {`)
			p(`		if err == nil {`)
			p(`			err = %s(recover())`, g.codegen().qualify("CatchPanics"))
			p(`		}`)
			p(`	}()`)

			if mt.Params().Len() > 1 {
				p(``)
				p(`	// Decode arguments.`)
				p(`	dec := %s(args)`, g.codegen().qualify("NewDecoder"))
			}
			b.Reset()
			for i := 1; i < mt.Params().Len(); i++ { // Skip initial context.Context
				at := mt.Params().At(i).Type()
				arg := fmt.Sprintf("a%d", i-1)
				if x, ok := at.(*types.Pointer); ok && (g.tset.isProto(x) || g.tset.hasMarshalBinary(x)) {
					// To decode a pointer *t where t is a proto or
					// BinaryUnmarshaler, we need to instantiate a zero value
					// of type t before calling the appropriate decoding
					// function. For all other types, this is unnecessary.
					tmp := fmt.Sprintf("tmp%d", i)
					p(`	var %s %s`, tmp, g.tset.genTypeString(x.Elem()))
					p(`	%s`, g.decode("dec", ref(tmp), x.Elem()))
					p(`	%s := %s`, arg, ref(tmp))
				} else {
					p(`	var %s %s`, arg, g.tset.genTypeString(at))
					p(`	%s`, g.decode("dec", ref(arg), at))
				}
			}

			b.Reset()
			fmt.Fprintf(&b, "ctx")
			for i := 1; i < mt.Params().Len(); i++ {
				fmt.Fprintf(&b, ", a%d", i-1)
			}
			argList := b.String()

			// Add load, if needed.
			if comp.routedMethods[m.Name()] {
				p(`     var r %s`, g.tset.genTypeString(comp.router))
				p(`	s.addLoad(_hash%s(r.%s(%s)), 1.0)`, exported(comp.name), m.Name(), argList)
			}

			b.Reset()
			p(``)
			p(`	// TODO(rgrandl): The deferred function above will recover from panics in the`)
			p(`	// user code: fix this.`)
			p(`	// Call the local method.`)
			for i := 0; i < mt.Results().Len()-1; i++ { // Skip final error
				if b.Len() == 0 {
					fmt.Fprintf(&b, "r%d", i)
				} else {
					fmt.Fprintf(&b, ", r%d", i)
				}
			}

			var res string
			if b.Len() == 0 {
				res = "appErr"
			} else {
				res = fmt.Sprintf("%s, appErr", b.String())
			}

			p(`	%s := s.impl.%s(%s)`, res, m.Name(), argList)

			p(``)
			p(`	// Encode the results.`)
			p(` enc := %s()`, g.codegen().qualify("NewEncoder"))

			b.Reset()
			for i := 0; i < mt.Results().Len()-1; i++ { // Skip final error
				rt := mt.Results().At(i).Type()
				res := fmt.Sprintf("r%d", i)
				p(`	%s`, g.encode("enc", res, rt))
			}
			p(`	enc.Error(appErr)`)
			p(`	return enc.Data(), nil`)
			p(`}`)
		}
	}
}

// generateAutoMarshalMethods generates WeaverMarshal and WeaverUnmarshal methods
// for any types that declares itself as weaver.AutoMarshal.
func (g *generator) generateAutoMarshalMethods(p printFn) {
	if g.tset.automarshalCandidates.Len() > 0 {
		p(``)
		p(`// AutoMarshal implementations.`)
	}

	// Sort the types so the generated methods appear in deterministic order.
	sorted := g.tset.automarshalCandidates.Keys()
	sort.Slice(sorted, func(i, j int) bool {
		ti, tj := sorted[i], sorted[j]
		return ti.String() < tj.String()
	})

	ts := g.tset.genTypeString
	for _, t := range sorted {
		var innerTypes []types.Type
		s := t.Underlying().(*types.Struct)

		// Generate AutoMarshal assertion.
		p(``)
		p(`var _ %s = &%s{}`, g.codegen().qualify("AutoMarshal"), ts(t))

		// Generate WeaverMarshal method.
		fmt := g.tset.importPackage("fmt", "fmt")
		p(``)
		p(`func (x *%s) WeaverMarshal(enc *%s) {`, ts(t), g.codegen().qualify("Encoder"))
		p(`	if x == nil {`)
		p(`		panic(%s("%s.WeaverMarshal: nil receiver"))`, fmt.qualify("Errorf"), ts(t))
		p(`	}`)
		for i := 0; i < s.NumFields(); i++ {
			fi := s.Field(i)
			if !isWeaverAutoMarshal(fi.Type()) {
				p(`	%s`, g.encode("enc", "x."+fi.Name(), fi.Type()))
				innerTypes = append(innerTypes, fi.Type())
			}
		}
		p(`}`)

		// Generate WeaverUnmarshal method.
		p(``)
		p(`func (x *%s) WeaverUnmarshal(dec *%s) {`, ts(t), g.codegen().qualify("Decoder"))
		p(`	if x == nil {`)
		p(`		panic(%s("%s.WeaverUnmarshal: nil receiver"))`, fmt.qualify("Errorf"), ts(t))
		p(`	}`)
		for i := 0; i < s.NumFields(); i++ {
			fi := s.Field(i)
			if !isWeaverAutoMarshal(fi.Type()) {
				p(`	%s`, g.decode("dec", "&x."+fi.Name(), fi.Type()))
			}
		}
		p(`}`)

		// Generate encoding/decoding methods for any inner types.
		for _, inner := range innerTypes {
			g.generateEncDecMethodsFor(p, inner)
		}
	}
}

// generateRouterMethods generates methods for router types.
func (g *generator) generateRouterMethods(p printFn) {
	printed := false
	for _, comp := range g.components {
		if comp.routingKey != nil {
			if !printed {
				p(`// Router methods.`)
				p(``)
				printed = true
			}
			g.generateRouterMethodsFor(p, comp, comp.routingKey)
		}
	}
}

// generateRouterMethodsFor generates router methods for the provided router type.
func (g *generator) generateRouterMethodsFor(p printFn, comp *component, t types.Type) {
	p(`// _hash%s returns a 64 bit hash of the provided value.`, exported(comp.name))
	p(`func _hash%s(r %s) uint64 {`, exported(comp.name), g.tset.genTypeString(t))
	p(`	var h %s`, g.codegen().qualify("Hasher"))
	if isPrimitiveRouter(t.Underlying()) {
		tname := t.Underlying().String()
		p(`	h.Write%s(%s(r))`, exported(tname), tname)
	} else {
		s := t.Underlying().(*types.Struct)
		for i := 0; i < s.NumFields(); i++ {
			f := s.Field(i)
			tname := f.Type().Underlying().String()
			p(`	h.Write%s(%s(r.%s))`, exported(tname), tname, f.Name())
		}
	}
	p(`	return h.Sum64()`)
	p(`}`)
	p(``)

	p(`// _orderedCode%s returns an order-preserving serialization of the provided value.`, exported(comp.name))
	p(`func _orderedCode%s(r %s) %s {`, exported(comp.name), g.tset.genTypeString(t), g.codegen().qualify("OrderedCode"))
	p(`	var enc %s`, g.codegen().qualify("OrderedEncoder"))
	if isPrimitiveRouter(t.Underlying()) {
		p(`	enc.Write%s(%s(r))`, exported(t.Underlying().String()), t.Underlying().String())
	} else {
		s := t.Underlying().(*types.Struct)
		for i := 0; i < s.NumFields(); i++ {
			f := s.Field(i)
			p(`	enc.Write%s(r.%s)`, exported(f.Type().String()), f.Name())
		}
	}
	p(`	return enc.Encode()`)
	p(`}`)
}

// ref returns an expression equivalent to "&e", removing any redundant "&*" at
// the beginning of the returned expression.
func ref(e string) string {
	if len(e) == 0 {
		return "&"
	}
	if e[0] == '*' {
		return e[1:]
	}
	return "&" + e
}

// deref returns an expression equivalent to "*e", removing any redundant "*&"
// at the beginning of the returned expression.
func deref(e string) string {
	if len(e) == 0 {
		return "*"
	}
	if e[0] == '&' {
		return e[1:]
	}
	return "*" + e
}

// encode returns a statement that encodes the expression e of type t into stub
// of type *codegen.Encoder. For example, encode("enc", "1", int) is
// "enc.Int(1)".
func (g *generator) encode(stub, e string, t types.Type) string {
	f := func(t types.Type) string {
		return fmt.Sprintf("serviceweaver_enc_%s", sanitize(t))
	}

	// Let enc(stub, e: t) be the statement that encodes e into stub. [t] is
	// shorthand for sanitize(t). under(t) is the underlying type of t.
	//
	// enc(stub, e: basic type t) = stub.[t](e)
	// enc(stub, e: *t) = serviceweaver_enc_[*t](&stub, e)
	// enc(stub, e: [N]t) = serviceweaver_enc_[[N]t](&stub, &e)
	// enc(stub, e: []t) = serviceweaver_enc_[[]t](&stub, e)
	// enc(stub, e: map[k]v) = serviceweaver_enc_[map[k]v](&stub, e)
	// enc(stub, e: struct{...}) = serviceweaver_enc_[struct{...}](&stub, &e)
	// enc(stub, e: type t u) = stub.EncodeProto(&e)           // t implements proto.Message
	// enc(stub, e: type t u) = (e).WeaverMarshal(stub)         // t implements AutoMarshal
	// enc(stub, e: type t u) = stub.EncodeBinaryMarshaler(&e) // t implements BinaryMarshaler
	// enc(stub, e: type t u) = serviceweaver_enc_[t](&stub, &e)       // under(u) = struct{...}
	// enc(stub, e: type t u) = enc(&stub, under(t)(e))        // otherwise
	switch x := t.(type) {
	case *types.Basic:
		switch x.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64,
			types.Complex64, types.Complex128,
			types.String:
			return fmt.Sprintf("%s.%s(%s)", stub, exported(x.Name()), e)
		default:
			panic(fmt.Sprintf("encode: unexpected expression: %v (type %T)", e, t))
		}

	case *types.Pointer:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, e)

	case *types.Array:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, ref(e))

	case *types.Slice:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, e)

	case *types.Map:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, e)

	case *types.Struct:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, ref(e))

	case *types.Named:
		if g.tset.isProto(x) {
			return fmt.Sprintf("%s.EncodeProto(%s)", stub, ref(e))
		}
		if g.tset.automarshals.At(x) != nil || g.tset.implementsAutoMarshal(x) {
			return fmt.Sprintf("(%s).WeaverMarshal(%s)", e, stub)
		}
		if g.tset.hasMarshalBinary(x) {
			return fmt.Sprintf("%s.EncodeBinaryMarshaler(%s)", stub, ref(e))
		}
		under := x.Underlying()
		if _, ok := under.(*types.Struct); ok {
			return fmt.Sprintf("%s(%s, %s)", f(x), stub, ref(e))
		}
		return g.encode(stub, fmt.Sprintf("(%s)(%s)", g.tset.genTypeString(x.Underlying()), e), under)

	default:
		panic(fmt.Sprintf("encode: unexpected expression: %v (type %T)", e, t))
	}
}

// decode returns a statement that decodes a value of type t from stub of
// *codegen.Decoder into the pointer v of type *t. For example, decode("dec",
// "p", int) is "*p = dec.Int()".
func (g *generator) decode(stub, v string, t types.Type) string {
	f := func(t types.Type) string {
		return fmt.Sprintf("serviceweaver_dec_%s", sanitize(t))
	}

	// Let dec(stub, v: t) be the statement that decodes a value of type t from
	// stub into v of type *t. [t] is shorthand for sanitize(t). under(t) is
	// the underlying type of t.
	//
	// dec(stub, v: basic type t) = *v := stub.[t](e)
	// dec(stub, v: *t) = *v := serviceweaver_dec_[*t](&stub)
	// dec(stub, v: [N]t) = serviceweaver_dec_[[N]t](stub, v)
	// dec(stub, v: []t) = v := *v = serviceweaver_dec_[[]t](stub)
	// dec(stub, v: map[k]v) = *v := serviceweaver_dec_[map[k]v](stub)
	// dec(stub, v: struct{...}) = serviceweaver_dec_[struct{...}](stub, &v)
	// dec(stub, v: type t u) = stub.DecodeProto(v)             // t implements proto.Message
	// dec(stub, v: type t u) = (v).WeaverUnmarshal(stub)        // t implements AutoMarshal
	// dec(stub, v: type t u) = stub.DecodeBinaryUnmarshaler(v) // t implements BinaryUnmarshaler
	// dec(stub, v: type t u) = serviceweaver_dec_[t](stub, v)          // under(u) = struct{...}
	// dec(stub, v: type t u) = dec(stub, (*under(t))(v))       // otherwise
	switch x := t.(type) {
	case *types.Basic:
		switch x.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64,
			types.Complex64, types.Complex128,
			types.String:
			return fmt.Sprintf("%s = %s.%s()", deref(v), stub, exported(x.Name()))
		default:
			panic(fmt.Sprintf("encode: unexpected expression: %v (type %T)", v, t))
		}

	case *types.Pointer:
		return fmt.Sprintf("%s = %s(%s)", deref(v), f(x), stub)

	case *types.Array:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, v)

	case *types.Slice:
		return fmt.Sprintf("%s = %s(%s)", deref(v), f(x), stub)

	case *types.Map:
		return fmt.Sprintf("%s = %s(%s)", deref(v), f(x), stub)

	case *types.Struct:
		return fmt.Sprintf("%s(%s, %s)", f(x), stub, v)

	case *types.Named:
		if g.tset.isProto(x) {
			return fmt.Sprintf("%s.DecodeProto(%s)", stub, v)
		}
		if g.tset.automarshals.At(x) != nil || g.tset.implementsAutoMarshal(x) {
			return fmt.Sprintf("(%s).WeaverUnmarshal(%s)", v, stub)
		}
		if g.tset.hasMarshalBinary(x) {
			return fmt.Sprintf("%s.DecodeBinaryUnmarshaler(%s)", stub, v)
		}
		under := x.Underlying()
		if _, ok := under.(*types.Struct); ok {
			return fmt.Sprintf("%s(%s, %s)", f(x), stub, v)
		}
		return g.decode(stub, fmt.Sprintf("(*%s)(%s)", g.tset.genTypeString(x.Underlying()), v), under)

	default:
		panic(fmt.Sprintf("encode: unexpected expression: %v (type %T)", v, t))
	}
}

// generateEncDecMethods generates all necessary encoding and decoding methods.
func (g *generator) generateEncDecMethods(p printFn) {
	printedHeader := false
	printer := func(format string, args ...interface{}) {
		if !printedHeader {
			p(`// Encoding/decoding implementations.`)
			printedHeader = true
		}
		p(format, args...)
	}
	for _, t := range g.types {
		g.generateEncDecMethodsFor(printer, t)
	}
}

// generateEncDecMethodsFor generates any necessary encoding and decoding
// methods for the provided type. generateEncDecMethodsFor is memoized; it will
// generate code for a type at most once.
func (g *generator) generateEncDecMethodsFor(p printFn, t types.Type) {
	if g.generated.At(t) != nil {
		// We already generated encoding/decoding methods for this type.
		return
	}
	g.generated.Set(t, true)

	ts := g.tset.genTypeString
	switch x := t.(type) {
	case *types.Basic:
		// Basic types don't need encoding or decoding methods. Instead, we
		// call methods directly on a codegen.Encoder or codegen.Decoder
		// (e.g., enc.Int(42), dec.Bool()).

	case *types.Pointer:
		if g.tset.isProto(x) || g.tset.hasMarshalBinary(x) {
			// Types implementing proto.Marshal or encoding.BinaryMarshaler and
			// encoding.BinaryUnmarshaler don't need encoding or decoding
			// methods. Instead, we call methods directly on a
			// codegen.Encoder or codegen.Decoder (e.g.,
			// enc.EncodeProto(x), dec.DecodeBinaryUnmarshaler(x)).
			return
		}

		g.generateEncDecMethodsFor(p, x.Elem())

		p(``)
		p(`func serviceweaver_enc_%s(enc *%s, arg %s) {`, sanitize(x), g.codegen().qualify("Encoder"), ts(x))
		p(`	if arg == nil {`)
		p(`		enc.Bool(false)`)
		p(`	} else {`)
		p(`		enc.Bool(true)`)
		p(`		%s`, g.encode("enc", "*arg", x.Elem()))
		p(`	}`)
		p(`}`)

		p(``)
		p(`func serviceweaver_dec_%s(dec *%s) %s {`, sanitize(x), g.codegen().qualify("Decoder"), ts(x))
		p(`	if !dec.Bool() {`)
		p(`		return nil`)
		p(`	}`)
		p(`	var res %s`, ts(x.Elem()))
		p(`	%s`, g.decode("dec", "&res", x.Elem()))
		p(`	return &res`)
		p(`}`)

	case *types.Array:
		g.generateEncDecMethodsFor(p, x.Elem())

		// Note that arg is never nil.
		p(``)
		p(`func serviceweaver_enc_%s(enc *%s, arg *%s) {`, sanitize(x), g.codegen().qualify("Encoder"), ts(x))
		p(`	for i := 0; i < %d; i++ {`, x.Len())
		p(`		%s`, g.encode("enc", "arg[i]", x.Elem()))
		p(`	}`)
		p(`}`)

		// Note that res is never nil.
		p(``)
		p(`func serviceweaver_dec_%s(dec *%s, res *%s) {`, sanitize(x), g.codegen().qualify("Decoder"), ts(x))
		p(`	for i := 0; i < %d; i++ {`, x.Len())
		p(`		%s`, g.decode("dec", "&res[i]", x.Elem()))
		p(`	}`)
		p(`}`)

	case *types.Slice:
		g.generateEncDecMethodsFor(p, x.Elem())

		p(``)
		p(`func serviceweaver_enc_%s(enc *%s, arg %s) {`, sanitize(x), g.codegen().qualify("Encoder"), ts(x))
		p(`	if arg == nil {`)
		p(`		enc.Len(-1)`)
		p(`		return`)
		p(`	}`)
		p(`	enc.Len(len(arg))`)
		p(`	for i := 0; i < len(arg); i++ {`)
		p(`		%s`, g.encode("enc", "arg[i]", x.Elem()))
		p(`	}`)
		p(`}`)

		p(``)
		p(`func serviceweaver_dec_%s(dec *%s) %s {`, sanitize(x), g.codegen().qualify("Decoder"), ts(x))
		p(`	n := dec.Len()`)
		p(`	if n == -1 {`)
		p(`		return nil`)
		p(`	}`)
		p(`	res := make(%s, n)`, ts(x))
		p(`	for i := 0; i < n; i++ {`)
		p(`		%s`, g.decode("dec", "&res[i]", x.Elem()))
		p(`	}`)
		p(`	return res`)
		p(`}`)

	case *types.Map:
		g.generateEncDecMethodsFor(p, x.Key())
		g.generateEncDecMethodsFor(p, x.Elem())

		p(``)
		p(`func serviceweaver_enc_%s(enc *%s, arg %s) {`, sanitize(x), g.codegen().qualify("Encoder"), ts(x))
		p(`	if arg == nil {`)
		p(`		enc.Len(-1)`)
		p(`		return`)
		p(`	}`)
		p(`	enc.Len(len(arg))`)
		p(`	for k, v := range arg {`)
		p(`		%s`, g.encode("enc", "k", x.Key()))
		p(`		%s`, g.encode("enc", "v", x.Elem()))
		p(`	}`)
		p(`}`)

		p(``)
		p(`func serviceweaver_dec_%s(dec *%s) %s {`, sanitize(x), g.codegen().qualify("Decoder"), ts(x))
		p(`	n := dec.Len()`)
		p(`	if n == -1 {`)
		p(`		return nil`)
		p(`	}`)
		p(`	res := make(%s, n)`, ts(x))
		p(`	var k %s`, ts(x.Key()))
		p(`	var v %s`, ts(x.Elem()))
		p(`	for i := 0; i < n; i++ {`)
		p(`		%s`, g.decode("dec", "&k", x.Key()))
		p(`		%s`, g.decode("dec", "&v", x.Elem()))
		p(`		res[k] = v`)
		p(`	}`)
		p(`	return res`)
		p(`}`)

	case *types.Struct:
		// Struct literals are not serializable.
		panic(fmt.Sprintf("generateEncDecFor: unexpected type: %v", t))

	case *types.Named:
		if g.tset.isProto(x) || g.tset.automarshals.At(x) != nil || g.tset.implementsAutoMarshal(x) || g.tset.hasMarshalBinary(x) {
			// Types implementing proto.Marshal, weaver.AutoMarshal, or
			// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler don't
			// need encoding or decoding methods. Instead, we call methods
			// directly on a codegen.Encoder or codegen.Decoder (e.g.,
			// enc.EncodeProto(x), dec.DecodeBinaryUnmarshaler(x)).
			return
		}
		// If a named type t is not a struct, e.g. `type t int`, then we
		// encode and decode values of type by casting it to its underlying
		// type (e.g., enc.Int(int(x)) where x has type t).
		g.generateEncDecMethodsFor(p, x.Underlying())

	default:
		panic(fmt.Sprintf("generateEncDecFor: unexpected type: %v", t))
	}
}

// codegen imports and returns the codegen package.
func (g *generator) codegen() importPkg {
	path := fmt.Sprintf("%s/runtime/codegen", weaverPackagePath)
	return g.tset.importPackage(path, "codegen")
}

// time imports and returns the time package.
func (g *generator) time() importPkg {
	return g.tset.importPackage("time", "time")
}

// trace imports and returns the otel trace package.
func (g *generator) trace() importPkg {
	return g.tset.importPackage("go.opentelemetry.io/otel/trace", "trace")
}

// codes imports and returns the otel codes package.
func (g *generator) codes() importPkg {
	return g.tset.importPackage("go.opentelemetry.io/otel/codes", "codes")
}

// sanitize generates a (somewhat pretty printed) name for the provided type
// that is a valid go identifier [1]. sanitize also produces unique names. That
// is, if u != t, then sanitize(u) != sanitize(t).
//
// Some examples:
//
//   - map[int]string -> map_int_string_589aebd1
//   - map[int][]X    -> map_int_slice_X_ac498abc
//   - []int          -> slice_int_1048ebf9
//   - [][]string     -> slice_slice_string_13efa8aa
//   - [20]int        -> array_20_int_00ae9a0a
//   - *int           -> ptr_int_916711b2
//
// [1]: https://go.dev/ref/spec#Identifiers
func sanitize(t types.Type) string {
	var sanitize func(types.Type) string
	sanitize = func(t types.Type) string {
		switch x := t.(type) {
		case *types.Pointer:
			return fmt.Sprintf("ptr_%s", sanitize(x.Elem()))

		case *types.Slice:
			return fmt.Sprintf("slice_%s", sanitize(x.Elem()))

		case *types.Array:
			return fmt.Sprintf("array_%d_%s", x.Len(), sanitize(x.Elem()))

		case *types.Map:
			keyName := sanitize(x.Key())
			valName := sanitize(x.Elem())
			return fmt.Sprintf("map_%s_%s", keyName, valName)

		case *types.Named:
			// A named type can either be an plain type or an instantiation of
			// a generic type. Consider the following code, for example.
			//
			//     type Plain int
			//     type Generic[A, B, C any] struct{ x A; y B; z C }
			//     func f(x Generic[int, bool, string]) {}
			//
			// Both `Plain` and `Generic[int, bool, string]` are named types.
			// We sanitize `Generic[int, bool, string]` to
			// `Generic_int_bool_string`.
			n := x.TypeArgs().Len()
			if n == 0 {
				// This is a plain type.
				return x.Obj().Name()
			}

			// This is an instantiated type.
			parts := make([]string, 1+n)
			parts[0] = x.Obj().Name()
			for i := 0; i < n; i++ {
				parts[i+1] = sanitize(x.TypeArgs().At(i))
			}
			return strings.Join(parts, "_")

		case *types.Struct:
			// To keep sanitized struct names short, we simple output "struct".
			// The hash suffix below will ensure the names are unique.
			return "struct"

		case *types.Basic:
			switch x.Kind() {
			case types.Bool,
				types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
				types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
				types.Float32, types.Float64,
				types.Complex64, types.Complex128,
				types.String:
				return x.Name()
			}
		}
		panic(fmt.Sprintf("generator: unable to generate named type suffic for type: %v\n", t))
	}

	// To ensure that sanitize returns unique names, we append a hash of a
	// unique name.
	hash := sha256.Sum256([]byte(uniqueName(t)))
	return fmt.Sprintf("%s_%x", sanitize(t), hash[:4])
}

// uniqueName returns a unique pretty printed representation of the provided
// type (e.g., "int", "map[int]bool"). The key property is that if u != t, then
// uniqueName(u) != uniqueName(t).
//
// Note that types.TypeString returns a pretty printed representation of a
// string, but it is not guaranteed to be unique. For example, if have `type
// int bool`, then TypeString returns "int" for both the named type int and the
// primitive type int.
func uniqueName(t types.Type) string {
	switch x := t.(type) {
	case *types.Pointer:
		return fmt.Sprintf("*%s", uniqueName(x.Elem()))

	case *types.Slice:
		return fmt.Sprintf("[]%s", uniqueName(x.Elem()))

	case *types.Array:
		return fmt.Sprintf("[%d]%s", x.Len(), uniqueName(x.Elem()))

	case *types.Map:
		keyName := uniqueName(x.Key())
		valName := uniqueName(x.Elem())
		return fmt.Sprintf("map[%s]%s", keyName, valName)

	case *types.Named:
		n := x.TypeArgs().Len()
		if n == 0 {
			// This is a plain type.
			return fmt.Sprintf("Named(%s.%s)", x.Obj().Pkg().Path(), x.Obj().Name())
		}

		// This is an instantiated type.
		base := fmt.Sprintf("Named(%s.%s)", x.Obj().Pkg().Path(), x.Obj().Name())
		parts := make([]string, n)
		for i := 0; i < n; i++ {
			parts[i] = uniqueName(x.TypeArgs().At(i))
		}
		return fmt.Sprintf("%s[%s]", base, strings.Join(parts, ", "))

	case *types.Struct:
		// Two structs are considered equal if they have the same fields with
		// the same names, types, and tags in the same order. See
		// https://go.dev/ref/spec#Type_identity.
		fields := make([]string, x.NumFields())
		var b strings.Builder
		for i := 0; i < x.NumFields(); i++ {
			b.Reset()
			f := x.Field(i)
			if !f.Embedded() {
				fmt.Fprintf(&b, "%s ", f.Name())
			}
			b.WriteString(uniqueName(f.Type()))
			if x.Tag(i) != "" {
				fmt.Fprintf(&b, " `%s`", x.Tag(i))
			}
			fields[i] = b.String()
		}
		return fmt.Sprintf("struct{%s}", strings.Join(fields, "; "))

	case *types.Basic:
		switch x.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64,
			types.Complex64, types.Complex128,
			types.String:
			return x.Name()
		}
	}
	// TODO(mwhittaker): What about Struct and Interface literals?
	panic(fmt.Sprintf("unsupported type %v (%T)", t, t))
}

// notExported sets the first character in the string to lowercase.
func notExported(name string) string {
	if len(name) == 0 {
		return name
	}
	a := []rune(name)
	a[0] = unicode.ToLower(a[0])
	return string(a)
}

// exported sets the first character in the string to uppercase.
func exported(name string) string {
	if len(name) == 0 {
		return name
	}
	a := []rune(name)
	a[0] = unicode.ToUpper(a[0])
	return string(a)
}
