package formula

import (
	"strings"

	"xl/document/value"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/shopspring/decimal"
)

type Boolean bool
type Sheet string
type FuncName string
type String string

func (b *Boolean) Capture(values []string) error {
	*b = Boolean(strings.EqualFold(values[0], "TRUE"))
	return nil
}

func (s *Sheet) Capture(values []string) error {
	values[0] = strings.TrimRight(values[0], "!")
	// remove first and last char
	l := len(values[0])
	if l >= 2 && values[0][0] == '\'' && values[0][l-1] == '\'' {
		values[0] = values[0][1 : len(values[0])-1]
	}
	// replace double quotes with single quote
	values[0] = strings.Replace(values[0], `''`, `'`, -1)
	*s = Sheet(values[0])
	return nil
}

func (f *FuncName) Capture(values []string) error {
	*f = FuncName(strings.TrimRight(values[0], "("))
	return nil
}

func (s *String) Capture(values []string) error {
	// remove first and last char
	values[0] = values[0][1 : len(values[0])-1]
	// replace double quotes with single quote
	values[0] = strings.Replace(values[0], `""`, `"`, -1)
	*s = String(values[0])
	return nil
}

type Expression struct {
	Equality *Equality `"=" @@`
}

type Equality struct {
	Comparison *Comparison `@@`
	Op         string      `[ @( "<>" | "=" )`
	Next       *Equality   `  @@ ]`
}

type Comparison struct {
	Addition *Addition   `@@`
	Op       string      `[ @( ">" | ">=" | "<" | "<=" )`
	Next     *Comparison `  @@ ]`
}

type Addition struct {
	Multiplication *Multiplication `@@`
	Op             string          `[ @( "-" | "+" )`
	Next           *Addition       `  @@ ]`
}

type Multiplication struct {
	Unary *Unary          `@@`
	Op    string          `[ @( "/" | "*" )`
	Next  *Multiplication `  @@ ]`
}

type Unary struct {
	Op      string   `( @( "-" | "+" )`
	Unary   *Unary   `  @@ )`
	Primary *Primary `| @@`
}

type Primary struct {
	SubExpression *Equality  `"(" @@ ")" `
	Number        *float64   `| @Number`
	String        *String    `| @String`
	Boolean       *Boolean   `| @("TRUE" | "FALSE")`
	Func          *Func      `| @@`
	CellRange     *CellRange `| @@`
}

type Func struct {
	Name      FuncName    `@FuncName`
	Arguments []*Equality `@@ { "," @@ } ")"`
}

type CellRange struct {
	Cell   *Cell `@@`
	CellTo *Cell `[ ":" @@ ]`
}

type Cell struct {
	Sheet *Sheet `[ @Sheet ]`
	Cell  string `@Cell`
}

var lex = lexer.Must(lexer.Regexp(
	`(\s+)` +
		`|^=` +
		`|(?P<Operators><>|<=|>=|[-+*/()=<>,:])` +
		`|(?P<Number>\d*\.?\d+([eE][-+]?\d+)?)` +
		`|(?P<String>"([^"]|"")*")` +
		`|(?P<Boolean>(?i)TRUE|FALSE)` +
		`|(?P<FuncName>[A-z0-9]+)\(` +
		`|(?P<Sheet>[A-z0-9_]+|'([^']|'')*')!` +
		`|(?P<Cell>[A-z]+[1-9][0-9]*)`,
))

// Parse parses the formula, extracts variables from it and builds
// functions chain that perform the expression representing by the formula..
func Parse(source string) (Function, *VarBin, error) {
	p, err := participle.Build(
		&Expression{},
		participle.Lexer(lex),
		participle.CaseInsensitive("Boolean"),
	)
	if err != nil {
		return nil, nil, err
	}
	expression := &Expression{}
	if err = p.ParseString(source, expression); err != nil {
		return nil, nil, err
	}
	vb := &VarBin{}
	f, _ := buildFuncFromEquality(expression.Equality, vb)
	return f, vb, nil
}

func buildFuncFromEquality(eq *Equality, vars *VarBin) (Function, int) {
	if eq.Next == nil {
		return buildFuncFromComparison(eq.Comparison, vars)
	}
	subFunc1, consumedArgs1 := buildFuncFromComparison(eq.Comparison, vars)
	subFunc2, consumedArgs2 := buildFuncFromEquality(eq.Next, vars)
	f := func(args []value.Value) (value.Value, error) {
		var v1, v2 value.Value
		var err error
		if v1, err = subFunc1(args); err != nil {
			return value.Value{}, err
		}
		if v2, err = subFunc2(args[consumedArgs1:]); err != nil {
			return value.Value{}, err
		}
		return evalOperator(eq.Op, v1, v2)
	}
	return f, consumedArgs1 + consumedArgs2
}

func buildFuncFromComparison(cmp *Comparison, vars *VarBin) (Function, int) {
	if cmp.Next == nil {
		return buildFuncFromAddition(cmp.Addition, vars)
	}
	subFunc1, consumedArgs1 := buildFuncFromAddition(cmp.Addition, vars)
	subFunc2, consumedArgs2 := buildFuncFromComparison(cmp.Next, vars)
	f := func(args []value.Value) (value.Value, error) {
		var v1, v2 value.Value
		var err error
		if v1, err = subFunc1(args); err != nil {
			return value.Value{}, err
		}
		if v2, err = subFunc2(args[consumedArgs1:]); err != nil {
			return value.Value{}, err
		}
		return evalOperator(cmp.Op, v1, v2)
	}
	return f, consumedArgs1 + consumedArgs2
}

func buildFuncFromAddition(a *Addition, vars *VarBin) (Function, int) {
	if a.Next == nil {
		return buildFuncFromMultiplication(a.Multiplication, vars)
	}
	subFunc1, consumedArgs1 := buildFuncFromMultiplication(a.Multiplication, vars)
	subFunc2, consumedArgs2 := buildFuncFromAddition(a.Next, vars)
	f := func(args []value.Value) (value.Value, error) {
		var v1, v2 value.Value
		var err error
		if v1, err = subFunc1(args); err != nil {
			return value.Value{}, err
		}
		if v2, err = subFunc2(args[consumedArgs1:]); err != nil {
			return value.Value{}, err
		}
		return evalOperator(a.Op, v1, v2)
	}
	return f, consumedArgs1 + consumedArgs2
}

func buildFuncFromMultiplication(m *Multiplication, vars *VarBin) (Function, int) {
	if m.Next == nil {
		return buildFuncFromUnary(m.Unary, vars)
	}
	subFunc1, consumedArgs1 := buildFuncFromUnary(m.Unary, vars)
	subFunc2, consumedArgs2 := buildFuncFromMultiplication(m.Next, vars)
	f := func(args []value.Value) (value.Value, error) {
		var v1, v2 value.Value
		var err error
		if v1, err = subFunc1(args); err != nil {
			return value.Value{}, err
		}
		if v2, err = subFunc2(args[consumedArgs1:]); err != nil {
			return value.Value{}, err
		}
		return evalOperator(m.Op, v1, v2)
	}
	return f, consumedArgs1 + consumedArgs2
}

func buildFuncFromUnary(u *Unary, vars *VarBin) (Function, int) {
	if u.Unary != nil {
		subFunc, consumedArgs := buildFuncFromUnary(u.Unary, vars)
		f := func(args []value.Value) (value.Value, error) {
			v, err := subFunc(args)
			if err != nil {
				return value.Value{}, err
			}
			return evalOperator(u.Op, v)
		}
		return f, consumedArgs
	} else if u.Primary.SubExpression != nil {
		return buildFuncFromEquality(u.Primary.SubExpression, vars)
	} else if u.Primary.Func != nil {
		consumedArgs := make([]int, len(u.Primary.Func.Arguments))
		subFunc := make([]Function, len(u.Primary.Func.Arguments))
		totalConsumedArgs := 0
		for i := range u.Primary.Func.Arguments {
			subFunc[i], consumedArgs[i] = buildFuncFromEquality(u.Primary.Func.Arguments[i], vars)
			totalConsumedArgs += consumedArgs[i]
		}
		f := func(args []value.Value) (value.Value, error) {
			var err error
			values := make([]value.Value, len(u.Primary.Func.Arguments))
			ca := 0
			for i := range u.Primary.Func.Arguments {
				values[i], err = subFunc[i](args[ca:])
				if err != nil {
					return value.Value{}, err
				}
				ca += consumedArgs[i]
			}
			return evalFunc(string(u.Primary.Func.Name), values)
		}
		return f, totalConsumedArgs
	} else if u.Primary.Boolean != nil {
		f := func([]value.Value) (value.Value, error) {
			return value.NewBoolValue(bool(*u.Primary.Boolean)), nil
		}
		return f, 0
	} else if u.Primary.Number != nil {
		f := func([]value.Value) (value.Value, error) {
			return value.NewDecimalValue(decimal.NewFromFloat(*u.Primary.Number)), nil
		}
		return f, 0
	} else if u.Primary.String != nil {
		f := func([]value.Value) (value.Value, error) {
			return value.NewStringValue(string(*u.Primary.String)), nil
		}
		return f, 0
	} else {
		vars.Vars = append(vars.Vars, newVar(u.Primary.CellRange))
		f := func(args []value.Value) (value.Value, error) {
			if len(args) == 0 {
				panic("too few arguments")
			}
			return args[0], nil
		}
		return f, 1
	}
}