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

package logging

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/operators"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Query is a filter for log entries.
//
// # Syntax
//
// Queries are written using a subset of the CEL language [1]. Thus, every
// syntactically valid query is also a syntactically valid CEL program.
// Specifically, a query is a CEL program over the following fields:
//
//   - app: string
//   - version: string
//   - full_version: string
//   - component: string
//   - full_component: string
//   - node: string
//   - full_node: string
//   - time: timestamp
//   - level: string
//   - source: string
//   - msg: string
//   - attrs: map[string]string
//
// A query is restricted to:
//
//   - boolean algebra (!, &&, ||),
//   - equalities and inequalities (==, !=, <, <=, >, >=),
//   - the string operations "contains" and "matches",
//   - map indexing (attrs["foo"]) and membership ("foo" in attrs), and
//   - constant strings, timestamps, and ints.
//
// All equalities and inequalities must look like `app == "todo"` or
// `attrs["foo"] == "bar"`; i.e. a field or attribute on the left and a constant
// on the right.
//
// # Semantics
//
// Queries have the same semantics as CEL programs except for one small
// exception. An attribute expression like `attrs["foo"]` has an implicit
// membership test `"foo" in attrs`. For example, the query
//
//	attrs["foo"] == "bar"
//
// is evaluated like the CEL program
//
//	"foo" in attrs && attrs["foo"] == "bar"
//
// TODO(mwhittaker): Expand the set of valid queries. For example, we can allow
// more constant expressions on the right hand side of a comparison. We can
// also allow fields on the right and constants on the left.
//
// [1]: https://opensource.google/projects/cel
type Query = string

// There are three phases in the lifecycle of query: First, the parse function
// parses and type checks a string-vaued query into a *cel.Ast. Second, the
// compile function compiles a *cel.Ast into an executable *cel.Program. Third,
// the matches function matches a compiled program against a log entry.
//
// Note that we use *cel.Ast as both an AST and as a compilation target. That
// is, we parse a query into a *cel.Ast, but executing this AST as a CEL
// program would not have the correct semantics. We instead have to rewrite
// this AST into a different AST with the correct semantics. This is much
// clearer for the GKE deployer where we parse a query into a *cel.Ast and then
// transpile the AST into a Google Cloud Logging query. The confusing
// difference here is that we transpile from CEL to CEL.

// env returns the cel.Env needed to compile a query.
//
// TODO(mwhittaker): Only make this environment once.
func env() (*cel.Env, error) {
	return cel.NewEnv(cel.Declarations(
		decls.NewVar("app", decls.String),
		decls.NewVar("version", decls.String),
		decls.NewVar("full_version", decls.String),
		decls.NewVar("component", decls.String),
		decls.NewVar("full_component", decls.String),
		decls.NewVar("node", decls.String),
		decls.NewVar("full_node", decls.String),
		decls.NewVar("time", decls.Timestamp),
		decls.NewVar("level", decls.String),
		decls.NewVar("source", decls.String),
		decls.NewVar("msg", decls.String),
		decls.NewVar("attrs", decls.NewMapType(decls.String, decls.String)),
	))
}

// Parse parses and type-checks a query.
func Parse(query Query) (*cel.Ast, error) {
	_, ast, err := parse(query)
	return ast, err
}

// parse parses and type-checks a query.
func parse(query Query) (*cel.Env, *cel.Ast, error) {
	// Build the environment.
	env, err := env()
	if err != nil {
		return nil, nil, fmt.Errorf("Parse(%s) environment error: %w", query, err)
	}

	// Parse and type-check the query.
	ast, issues := env.Compile(query)
	if issues != nil && issues.Err() != nil {
		return nil, nil, fmt.Errorf("Parse(%s) compilation error: %w", query, issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, nil, fmt.Errorf("Parse(%s) type error: got %v, want %v", query, ast.OutputType(), "bool")
	}

	// Restrict the query.
	if err := restrict(ast.Expr()); err != nil {
		return nil, nil, fmt.Errorf("Parse(%s) restriction error: %w", query, err)
	}

	return env, ast, nil
}

// restrict recursively walks an expression, checking to see if it conforms to
// the restricted subset of CEL used to write queries. If the expression does
// conform the subset, then nil is returned. Otherwise, an error is returned
// that explains why the expression doesn't conform.
//
// This function and related functions borrow heavily from [1].
//
// [1]: https://github.com/google/cel-go/blob/8e5d9877f0ab106269dee64e5bf10c5315281830/parser/unparser.go
func restrict(e *exprpb.Expr) error {
	// TODO(mwhittaker): Handle macros [1].
	//
	// [1]: https://github.com/google/cel-go/blob/8e5d9877f0ab106269dee64e5bf10c5315281830/parser/unparser.go#L58-L61

	switch e.ExprKind.(type) {
	case *exprpb.Expr_CallExpr:
		// Note that CEL represents operators like || and ! as calls.
		return restrictCall(e.GetCallExpr())
	default:
		return fmt.Errorf("unsupported expression: %v", e)
	}
}

func restrictCall(e *exprpb.Expr_Call) error {
	switch e.GetFunction() {
	// !
	case operators.LogicalNot:
		return restrict(e.Args[0])

	// &&, ||
	case operators.LogicalAnd, operators.LogicalOr:
		for i := 0; i < 2; i++ {
			if err := restrict(e.Args[i]); err != nil {
				return err
			}
		}
		return nil

	// ==, !=, <, <=, >, >=
	case operators.Equals, operators.NotEquals,
		operators.Less, operators.LessEquals,
		operators.Greater, operators.GreaterEquals:
		if err := restrictField(e.Args[0]); err != nil {
			return err
		}
		return restrictLiteral(e.Args[1])

	// contains, matches
	case "contains", "matches":
		if err := restrictField(e.Target); err != nil {
			return err
		}
		return restrictLiteral(e.Args[0])

	// in
	case operators.In:
		if err := restrictLiteral(e.Args[0]); err != nil {
			return err
		}
		return restrictField(e.Args[1])

	default:
		return fmt.Errorf("unsupported call: %v", e)
	}
}

// restrictField checks whether the provided expression is a log entry field,
// either an identifier like `line` or an attribute expression like `attrs["foo"]`.
func restrictField(e *exprpb.Expr) error {
	switch t := e.ExprKind.(type) {
	case *exprpb.Expr_IdentExpr:
		return nil
	case *exprpb.Expr_CallExpr:
		fn := t.CallExpr.Function
		if fn == operators.Index { // Map [] operator.
			if tg := t.CallExpr.Args[0].GetIdentExpr(); tg == nil || tg.GetName() != "attrs" {
				return fmt.Errorf(`unsupported map target, want "attrs", got %v`, t.CallExpr.Args[0])
			}
			if i := t.CallExpr.Args[1].GetConstExpr(); i == nil || i.GetStringValue() == "" {
				return fmt.Errorf("unsupported map index, want a non-empty string constant, got %v", t.CallExpr.Args[1])
			}
			return nil
		}
		return fmt.Errorf("unsupported function %s: %v", fn, t)
	default:
		return fmt.Errorf("unsupported field: %v", e)
	}
}

// restrictLiteral checks whether the provided expression is a literal (e.g.,
// 42, "foo").
func restrictLiteral(e *exprpb.Expr) error {
	switch e.ExprKind.(type) {
	case *exprpb.Expr_ConstExpr:
		return nil
	case *exprpb.Expr_CallExpr:
		call := e.GetCallExpr()
		if f := call.Function; f != "timestamp" {
			return fmt.Errorf("unsupported literal: %v", e)
		}
		return restrictLiteral(call.Args[0])
	default:
		return fmt.Errorf("unsupported literal: %v", e)
	}
}

// rewrite rewrites an expression parsed from a query into a CEL expression
// with the same semantics as the query. Specifically, binary expressions over
// attributes, like `attrs["foo"] == "bar"`, are translated to include an implicit
// membership test like `"foo" in attrs && attrs["foo"] == "bar"`.
func rewrite(e *exprpb.Expr) (*exprpb.Expr, error) {
	e = proto.Clone(e).(*exprpb.Expr)
	return rewriteExpr(e)
}

func rewriteExpr(e *exprpb.Expr) (*exprpb.Expr, error) {
	switch e.ExprKind.(type) {
	case *exprpb.Expr_CallExpr:
		// Note that CEL represents operators like || and ! as calls.
		call, err := rewriteCall(e.GetCallExpr())
		if err != nil {
			return nil, err
		}
		return callexpr(call), nil
	default:
		return nil, fmt.Errorf("unsupported expression: %v", e)
	}
}

func rewriteCall(e *exprpb.Expr_Call) (*exprpb.Expr_Call, error) {
	switch e.GetFunction() {
	// !
	case operators.LogicalNot:
		sub, err := rewriteExpr(e.Args[0])
		e.Args[0] = sub
		return e, err

	// &&, ||
	case operators.LogicalAnd, operators.LogicalOr:
		for i := 0; i < 2; i++ {
			sub, err := rewriteExpr(e.Args[i])
			if err != nil {
				return nil, err
			}
			e.Args[i] = sub
		}
		return e, nil

	// ==, !=, <, <=, >, >=
	case operators.Equals, operators.NotEquals,
		operators.Less, operators.LessEquals,
		operators.Greater, operators.GreaterEquals:
		attrs, attr, ok := explodeIndex(e.Args[0])
		if !ok {
			// There is no attrs["foo"] expression, so we don't have to
			// rewrite the expression.
			return e, nil
		}
		// Inject a `"foo" in attrs` check.
		contains := callexpr(binop(attr, operators.In, attrs))
		return binop(contains, operators.LogicalAnd, callexpr(e)), nil

	// contains, matches
	case "contains", "matches":
		attrs, attr, ok := explodeIndex(e.Target)
		if !ok {
			return e, nil
		}
		contains := callexpr(binop(attr, operators.In, attrs))
		return binop(contains, operators.LogicalAnd, callexpr(e)), nil

	// in
	case operators.In:
		return e, nil

	default:
		return nil, fmt.Errorf("unsupported call: %v", e)
	}
}

// callexpr wraps an Expr_Call into an Expr.
func callexpr(call *exprpb.Expr_Call) *exprpb.Expr {
	return &exprpb.Expr{ExprKind: &exprpb.Expr_CallExpr{CallExpr: call}}
}

// binop returns the ExprCall with the provided operator and operands.
func binop(lhs *exprpb.Expr, op string, rhs *exprpb.Expr) *exprpb.Expr_Call {
	return &exprpb.Expr_Call{Function: op, Args: []*exprpb.Expr{lhs, rhs}}
}

// explodeIndex deconstructs an index expression. If the provided expression e
// has the form m[k], then it returns m, k, true. Otherwise, it returns nil,
// nil, false. For example, `attrs["foo"]` returns `attrs, "foo", true`.
func explodeIndex(e *exprpb.Expr) (*exprpb.Expr, *exprpb.Expr, bool) {
	if _, ok := e.ExprKind.(*exprpb.Expr_CallExpr); ok {
		call := e.GetCallExpr()
		if call.GetFunction() == operators.Index {
			return call.Args[0], call.Args[1], true
		}
	}
	return nil, nil, false
}

// format returns a string representation of the provided expression. The
// returned string is not intended to be read by humans. It's overly
// parenthesized and very ugly.
func format(e *exprpb.Expr) (string, error) {
	var b strings.Builder
	err := formatExpr(&b, e)
	return b.String(), err
}

func formatExpr(w io.Writer, e *exprpb.Expr) error {
	fmt.Fprint(w, "(")
	defer fmt.Fprint(w, ")")

	switch e.ExprKind.(type) {
	case *exprpb.Expr_IdentExpr:
		fmt.Fprint(w, e.GetIdentExpr().Name)
		return nil
	case *exprpb.Expr_ConstExpr:
		return formatConst(w, e.GetConstExpr())
	case *exprpb.Expr_CallExpr:
		// Note that CEL represents operators like || and ! as calls.
		return formatCall(w, e.GetCallExpr())
	default:
		return fmt.Errorf("unsupported expression: %v", e)
	}
}

func formatCall(w io.Writer, e *exprpb.Expr_Call) error {
	fmt.Fprint(w, "(")
	defer fmt.Fprint(w, ")")

	ops := map[string]string{
		operators.LogicalNot:    "!",
		"timestamp":             "timestamp",
		operators.LogicalAnd:    "&&",
		operators.LogicalOr:     "||",
		operators.Equals:        "==",
		operators.NotEquals:     "!=",
		operators.Less:          "<",
		operators.LessEquals:    "<=",
		operators.Greater:       ">",
		operators.GreaterEquals: ">=",
		operators.In:            "in",
	}

	switch f := e.GetFunction(); f {
	// !, timestamp
	case operators.LogicalNot, "timestamp":
		fmt.Fprint(w, ops[f])
		return formatExpr(w, e.Args[0])

	// &&, ||, ==, !=, <, <=, >, >=, in
	case operators.LogicalAnd, operators.LogicalOr,
		operators.Equals, operators.NotEquals,
		operators.Less, operators.LessEquals,
		operators.Greater, operators.GreaterEquals,
		operators.In:
		if err := formatExpr(w, e.Args[0]); err != nil {
			return err
		}
		fmt.Fprintf(w, " %s ", ops[f])
		return formatExpr(w, e.Args[1])

	// []
	case operators.Index:
		if err := formatExpr(w, e.Args[0]); err != nil {
			return err
		}
		fmt.Fprintf(w, "[")
		err := formatExpr(w, e.Args[1])
		fmt.Fprintf(w, "]")
		return err

	// contains, matches
	case "contains", "matches":
		if err := formatExpr(w, e.Target); err != nil {
			return err
		}
		fmt.Fprintf(w, ".%s", f)
		return formatExpr(w, e.Args[0])

	default:
		return fmt.Errorf("unsupported call: %v", e)
	}
}

// formatConst formats the provided const into w.
func formatConst(w io.Writer, c *exprpb.Constant) error {
	// This implementation was borrowed from [1].
	//
	// [1]: https://github.com/google/cel-go/blob/v0.12.5/parser/unparser.go#L250
	switch c.GetConstantKind().(type) {
	case *exprpb.Constant_Int64Value:
		fmt.Fprint(w, strconv.FormatInt(c.GetInt64Value(), 10))
		return nil
	case *exprpb.Constant_StringValue:
		fmt.Fprint(w, strconv.Quote(c.GetStringValue()))
		return nil
	default:
		return fmt.Errorf("unsupported constant: %v", c)
	}
}

// compile compiles a query into a cel.Program.
func compile(env *cel.Env, ast *cel.Ast) (cel.Program, error) {
	// CEL does not provide any helper functions to rewrite ASTs. Instead, we
	// have to massage the AST, format it, and then parse it again.
	e, err := rewrite(ast.Expr())
	if err != nil {
		return nil, fmt.Errorf("compile rewrite: %w", err)
	}
	q, err := format(e)
	if err != nil {
		return nil, fmt.Errorf("compile format: %w", err)
	}
	ast, issues := env.Compile(q)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile: %w", issues.Err())
	}
	prog, err := env.Program(ast, cel.EvalOptions(cel.OptTrackState))
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return prog, nil
}

// matches returns whether the provided compiled query matches the provided log
// entry.
func matches(prog cel.Program, entry *protos.LogEntry) (bool, error) {
	if entry == nil {
		return false, nil
	}
	attrs := make(map[string]string, len(entry.Attrs))
	for i := 0; i+1 < len(entry.Attrs); i += 2 {
		attrs[entry.Attrs[i]] = entry.Attrs[i+1]
	}
	out, _, err := prog.Eval(map[string]interface{}{
		"app":            entry.App,
		"version":        Shorten(entry.Version),
		"full_version":   entry.Version,
		"component":      ShortenComponent(entry.Component),
		"full_component": entry.Component,
		"node":           Shorten(entry.Node),
		"full_node":      entry.Node,
		"time":           timestamppb.New(time.UnixMicro(entry.TimeMicros)),
		"level":          entry.Level,
		"source":         fmt.Sprintf("%s:%d", entry.File, entry.Line),
		"msg":            entry.Msg,
		"attrs":          attrs,
	})
	if err != nil {
		if out != nil {
			// Successful eval to an error result: we interpret this as a non-match.
			return false, nil
		}
		// Unsuccessful eval: that's an error.
		return false, err
	}
	b, err := out.ConvertToNative(reflect.TypeOf(true))
	if err != nil {
		return false, err
	}
	return b.(bool), nil
}
