package converter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xwb1989/sqlparser"
)

// SQL UNKNOWN is represented by both yes and no being false. Keeping both
// predicates prevents NOT and NOT IN from accidentally matching null values.
type roTruth struct{ yes, no interface{} }

func roAnd(a, b roTruth) roTruth {
	return roTruth{roDocument{"$and": roArray{a.yes, b.yes}}, roDocument{"$or": roArray{a.no, b.no}}}
}
func roOr(a, b roTruth) roTruth {
	return roTruth{roDocument{"$or": roArray{a.yes, b.yes}}, roDocument{"$and": roArray{a.no, b.no}}}
}
func roNot(a roTruth) roTruth { return roTruth{a.no, a.yes} }

func (c *roCompiler) scalar(expression sqlparser.Expr) (interface{}, error) {
	if column, ok := expression.(*sqlparser.ColName); ok {
		name, err := c.column(column)
		if err != nil {
			return nil, err
		}
		return roField(name), nil
	}
	value, err := roScalarLiteral(expression)
	if err != nil {
		return nil, err
	}
	return roLiteral(value), nil
}

func roCompare(left, right interface{}, operator string) roTruth {
	known := roDocument{"$and": roArray{roNonNull(left), roNonNull(right)}}
	// MongoDB otherwise orders unrelated BSON types (for example a string is
	// greater than a number). That would silently invent SQL comparison results.
	leftType, rightType := roDocument{"$type": left}, roDocument{"$type": right}
	compatible := roDocument{"$or": roArray{
		roDocument{"$and": roArray{roDocument{"$isNumber": left}, roDocument{"$isNumber": right}}},
		roDocument{"$and": roArray{roDocument{"$eq": roArray{leftType, rightType}}, roDocument{"$in": roArray{leftType, roArray{"string", "bool", "date"}}}}},
	}}
	comparison := roDocument{"$cond": roArray{compatible, roDocument{operator: roArray{left, right}}, roTypeError(left)}}
	return roTruth{
		roDocument{"$cond": roArray{known, comparison, false}},
		roDocument{"$cond": roArray{known, roDocument{"$not": roArray{comparison}}, false}},
	}
}

func roTypeError(value interface{}) roDocument {
	// Row-dependent input avoids an invalid constant being folded in an
	// otherwise unused branch. MongoDB evaluates only the chosen $cond branch.
	return roDocument{"$convert": roDocument{"input": roDocument{"$concat": roArray{"Unsupported SQL operand type: ", roDocument{"$type": value}}}, "to": "decimal"}}
}

func (c *roCompiler) predicate(expression sqlparser.Expr, depth int) (roTruth, error) {
	if err := c.node(depth); err != nil {
		return roTruth{}, err
	}
	switch value := expression.(type) {
	case *sqlparser.ParenExpr:
		return c.predicate(value.Expr, depth+1)
	case *sqlparser.NotExpr:
		child, err := c.predicate(value.Expr, depth+1)
		return roNot(child), err
	case *sqlparser.AndExpr:
		left, err := c.predicate(value.Left, depth+1)
		if err != nil {
			return roTruth{}, err
		}
		right, err := c.predicate(value.Right, depth+1)
		return roAnd(left, right), err
	case *sqlparser.OrExpr:
		left, err := c.predicate(value.Left, depth+1)
		if err != nil {
			return roTruth{}, err
		}
		right, err := c.predicate(value.Right, depth+1)
		return roOr(left, right), err
	case sqlparser.BoolVal:
		return roTruth{bool(value), !bool(value)}, nil
	case *sqlparser.NullVal:
		return roTruth{false, false}, nil
	case *sqlparser.IsExpr:
		operand, err := c.scalar(value.Expr)
		if err != nil {
			return roTruth{}, err
		}
		equal := roDocument{"$eq": roArray{operand, nil}}
		truth := roTruth{equal, roDocument{"$not": roArray{equal}}}
		switch value.Operator {
		case sqlparser.IsNullStr:
			return truth, nil
		case sqlparser.IsNotNullStr:
			return roNot(truth), nil
		default:
			return roTruth{}, fmt.Errorf("only IS NULL and IS NOT NULL are supported")
		}
	case *sqlparser.RangeCond:
		left, err := c.scalar(value.Left)
		if err != nil {
			return roTruth{}, err
		}
		lower, err := c.scalar(value.From)
		if err != nil {
			return roTruth{}, err
		}
		upper, err := c.scalar(value.To)
		if err != nil {
			return roTruth{}, err
		}
		truth := roAnd(roCompare(left, lower, "$gte"), roCompare(left, upper, "$lte"))
		if value.Operator == sqlparser.BetweenStr {
			return truth, nil
		}
		if value.Operator == sqlparser.NotBetweenStr {
			return roNot(truth), nil
		}
		return roTruth{}, fmt.Errorf("unsupported range operator")
	case *sqlparser.ComparisonExpr:
		if value.Escape != nil {
			return roTruth{}, fmt.Errorf("explicit LIKE ESCAPE is unsupported")
		}
		left, err := c.scalar(value.Left)
		if err != nil {
			return roTruth{}, err
		}
		if value.Operator == sqlparser.InStr || value.Operator == sqlparser.NotInStr {
			items, ok := value.Right.(sqlparser.ValTuple)
			if !ok || len(items) < 1 || len(items) > 128 {
				return roTruth{}, fmt.Errorf("IN requires a bounded literal list")
			}
			yes, no := roArray{}, roArray{}
			for _, item := range items {
				literal, err := roScalarLiteral(item)
				if err != nil {
					return roTruth{}, err
				}
				comparison := roCompare(left, roLiteral(literal), "$eq")
				yes = append(yes, comparison.yes)
				no = append(no, comparison.no)
			}
			truth := roTruth{roDocument{"$or": yes}, roDocument{"$and": no}}
			if value.Operator == sqlparser.NotInStr {
				truth = roNot(truth)
			}
			return truth, nil
		}
		if value.Operator == sqlparser.LikeStr || value.Operator == sqlparser.NotLikeStr {
			literal, ok := value.Right.(*sqlparser.SQLVal)
			if !ok || literal.Type != sqlparser.StrVal {
				return roTruth{}, fmt.Errorf("LIKE requires a string literal")
			}
			pattern, err := roLike(string(literal.Val))
			if err != nil {
				return roTruth{}, err
			}
			isString := roDocument{"$eq": roArray{roDocument{"$type": left}, "string"}}
			match := roDocument{"$regexMatch": roDocument{"input": roDocument{"$cond": roArray{isString, left, ""}}, "regex": pattern, "options": "s"}}
			checked := roDocument{"$cond": roArray{isString, match, roTypeError(left)}}
			truth := roTruth{roDocument{"$cond": roArray{roNonNull(left), checked, false}}, roDocument{"$cond": roArray{roNonNull(left), roDocument{"$not": roArray{checked}}, false}}}
			if value.Operator == sqlparser.NotLikeStr {
				truth = roNot(truth)
			}
			return truth, nil
		}
		right, err := c.scalar(value.Right)
		if err != nil {
			return roTruth{}, err
		}
		operators := map[string]string{sqlparser.EqualStr: "$eq", sqlparser.NotEqualStr: "$ne", sqlparser.LessThanStr: "$lt", sqlparser.LessEqualStr: "$lte", sqlparser.GreaterThanStr: "$gt", sqlparser.GreaterEqualStr: "$gte"}
		operator, ok := operators[value.Operator]
		if !ok {
			return roTruth{}, fmt.Errorf("unsupported comparison operator")
		}
		return roCompare(left, right, operator), nil
	default:
		return roTruth{}, fmt.Errorf("unsupported WHERE expression")
	}
}

func roLike(pattern string) (string, error) {
	var output strings.Builder
	// PCRE's absolute anchors prevent '$' from matching before a final newline.
	output.WriteString(`\A`)
	escaped := false
	for _, char := range pattern {
		if escaped {
			output.WriteString(regexp.QuoteMeta(string(char)))
			escaped = false
			continue
		}
		switch char {
		case '\\':
			escaped = true
		case '%':
			output.WriteString(".*")
		case '_':
			output.WriteByte('.')
		default:
			output.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	if escaped {
		return "", fmt.Errorf("LIKE pattern ends in an escape")
	}
	output.WriteString(`\z`)
	return output.String(), nil
}
