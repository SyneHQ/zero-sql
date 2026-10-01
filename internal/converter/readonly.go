package converter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xwb1989/sqlparser"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type roDocument = map[string]interface{}
type roArray = []interface{}

var roIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,119}$`)

type roCompiler struct {
	collection, alias string
	nodes             int
}
type roSelection struct {
	name                string
	column              string
	literal             interface{}
	isLiteral           bool
	aggregate, argument string
	star                bool
}

// ConvertReadOnlySQLToMongoWithCollection compiles a bounded, explicit SELECT
// subset without any legacy preprocessing or mutable converter state. Decimal
// constants are canonical Extended JSON and must be decoded as Extended JSON.
func (c *Converter) ConvertReadOnlySQLToMongoWithCollection(sql string) (string, []map[string]interface{}, error) {
	if err := roInputBudget(sql); err != nil {
		return "", nil, err
	}
	statement, err := sqlparser.Parse(sql)
	if err != nil {
		return "", nil, fmt.Errorf("invalid restricted SQL")
	}
	selectSQL, ok := statement.(*sqlparser.Select)
	if !ok {
		return "", nil, fmt.Errorf("only SELECT is supported")
	}
	compiler := new(roCompiler)
	pipeline, err := compiler.compile(selectSQL)
	if err != nil {
		return "", nil, err
	}
	encoded, err := json.Marshal(pipeline)
	if err != nil || len(encoded) > 128<<10 {
		return "", nil, fmt.Errorf("compiled SQL exceeds pipeline budget")
	}
	return compiler.collection, pipeline, nil
}

func roInputBudget(sql string) error {
	if len(sql) == 0 || len(sql) > 64<<10 {
		return fmt.Errorf("restricted SQL must be 1..65536 bytes")
	}
	// Use the parser's tokenizer so quotes and comments are not rewritten.
	tokenizer := sqlparser.NewStringTokenizer(sql)
	depth := 0
	for count := 0; ; count++ {
		token, _ := tokenizer.Scan()
		if token == 0 {
			break
		}
		if count >= 4096 {
			return fmt.Errorf("restricted SQL exceeds token budget")
		}
		switch token {
		case ';':
			return fmt.Errorf("statement terminators are unsupported")
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth > 64 || depth < 0 {
			return fmt.Errorf("restricted SQL exceeds nesting budget")
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced SQL parentheses")
	}
	return nil
}

func (c *roCompiler) node(depth int) error {
	c.nodes++
	if depth > 64 || c.nodes > 4096 {
		return fmt.Errorf("restricted SQL exceeds expression budget")
	}
	return nil
}

func (c *roCompiler) compile(s *sqlparser.Select) ([]roDocument, error) {
	if s.Distinct != "" || s.Having != nil || s.Lock != "" || s.Cache != "" || s.Hints != "" {
		return nil, fmt.Errorf("DISTINCT, HAVING, locks and query hints are unsupported")
	}
	if len(s.From) != 1 || len(s.SelectExprs) == 0 || len(s.SelectExprs) > 128 || len(s.GroupBy) > 16 || len(s.OrderBy) > 1 {
		return nil, fmt.Errorf("unsupported collection, projection, grouping or ordering shape")
	}
	table, ok := s.From[0].(*sqlparser.AliasedTableExpr)
	if !ok || table.Hints != nil || len(table.Partitions) != 0 {
		return nil, fmt.Errorf("joins, subqueries and table hints are unsupported")
	}
	name, ok := table.Expr.(sqlparser.TableName)
	if !ok || !name.Qualifier.IsEmpty() || !roIdentifier.MatchString(name.Name.String()) {
		return nil, fmt.Errorf("one unqualified collection is required")
	}
	c.collection, c.alias = name.Name.String(), table.As.String()
	if c.alias != "" && !roIdentifier.MatchString(c.alias) {
		return nil, fmt.Errorf("invalid table alias")
	}
	selections := make([]roSelection, 0, len(s.SelectExprs))
	names := map[string]bool{}
	hasAggregate, star := false, false
	for _, expression := range s.SelectExprs {
		if all, ok := expression.(*sqlparser.StarExpr); ok {
			if len(s.SelectExprs) != 1 || !all.TableName.IsEmpty() {
				return nil, fmt.Errorf("only a sole unqualified star is supported")
			}
			star = true
			continue
		}
		aliased, ok := expression.(*sqlparser.AliasedExpr)
		if !ok {
			return nil, fmt.Errorf("unsupported SELECT expression")
		}
		selection, err := c.selection(aliased)
		if err != nil {
			return nil, err
		}
		if names[selection.name] {
			return nil, fmt.Errorf("duplicate output aliases are unsupported")
		}
		names[selection.name] = true
		hasAggregate = hasAggregate || selection.aggregate != ""
		selections = append(selections, selection)
	}
	grouping := hasAggregate || len(s.GroupBy) != 0
	if star && grouping {
		return nil, fmt.Errorf("star projection with grouping is unsupported")
	}
	var pipeline []roDocument
	if s.Where != nil {
		predicate, err := c.predicate(s.Where.Expr, 0)
		if err != nil {
			return nil, err
		}
		pipeline = append(pipeline, roDocument{"$match": roDocument{"$expr": predicate.yes}})
	}
	if grouping {
		stages, err := c.group(s.GroupBy, selections)
		if err != nil {
			return nil, err
		}
		pipeline = append(pipeline, stages...)
		if len(s.OrderBy) == 1 {
			field, err := c.groupOrder(s.OrderBy[0], selections)
			if err != nil {
				return nil, err
			}
			pipeline = append(pipeline, roDocument{"$sort": roDocument{field: roDirection(s.OrderBy[0])}})
		}
	} else {
		if len(s.OrderBy) == 1 {
			field, err := c.rowOrder(s.OrderBy[0], selections)
			if err != nil {
				return nil, err
			}
			if field != "" {
				pipeline = append(pipeline, roDocument{"$sort": roDocument{field: roDirection(s.OrderBy[0])}})
			}
		}
		if !star {
			projection := roDocument{"_id": 0}
			for _, selection := range selections {
				if selection.isLiteral {
					projection[selection.name] = roLiteral(selection.literal)
				} else {
					projection[selection.name] = roField(selection.column)
				}
			}
			pipeline = append(pipeline, roDocument{"$project": projection})
		}
	}
	if s.Limit != nil {
		count, err := roPositiveInteger(s.Limit.Rowcount)
		if err != nil {
			return nil, err
		}
		if s.Limit.Offset != nil {
			offset, err := roPositiveInteger(s.Limit.Offset)
			if err != nil {
				return nil, err
			}
			if offset > 0 {
				pipeline = append(pipeline, roDocument{"$skip": offset})
			}
		}
		if count == 0 {
			pipeline = append(pipeline, roDocument{"$match": roDocument{"$expr": false}})
		} else {
			pipeline = append(pipeline, roDocument{"$limit": count})
		}
	}
	return pipeline, nil
}

func (c *roCompiler) selection(expression *sqlparser.AliasedExpr) (roSelection, error) {
	selection := roSelection{name: expression.As.String()}
	switch value := expression.Expr.(type) {
	case *sqlparser.ColName:
		column, err := c.column(value)
		if err != nil {
			return selection, err
		}
		selection.column = column
		if selection.name == "" {
			selection.name = column
		}
	case *sqlparser.FuncExpr:
		function := strings.ToLower(value.Name.String())
		if !value.Qualifier.IsEmpty() || value.Distinct || len(value.Exprs) != 1 {
			return selection, fmt.Errorf("unsupported aggregate arguments")
		}
		switch function {
		case "count", "sum", "avg", "min", "max":
		default:
			return selection, fmt.Errorf("unsupported SQL function")
		}
		selection.aggregate = function
		if selection.name == "" {
			selection.name = function
		}
		if all, ok := value.Exprs[0].(*sqlparser.StarExpr); ok {
			if function != "count" || !all.TableName.IsEmpty() {
				return selection, fmt.Errorf("only COUNT(*) accepts star")
			}
			selection.star = true
		} else {
			argument, ok := value.Exprs[0].(*sqlparser.AliasedExpr)
			if !ok || !argument.As.IsEmpty() {
				return selection, fmt.Errorf("aggregate requires one column")
			}
			column, ok := argument.Expr.(*sqlparser.ColName)
			if !ok {
				return selection, fmt.Errorf("aggregate requires one column")
			}
			var err error
			selection.argument, err = c.column(column)
			if err != nil {
				return selection, err
			}
		}
	default:
		literal, err := roScalarLiteral(expression.Expr)
		if err != nil {
			return selection, err
		}
		if selection.name == "" {
			return selection, fmt.Errorf("literal projections require an alias")
		}
		selection.literal, selection.isLiteral = literal, true
	}
	if !roIdentifier.MatchString(selection.name) {
		return selection, fmt.Errorf("invalid output alias")
	}
	return selection, nil
}

func (c *roCompiler) column(column *sqlparser.ColName) (string, error) {
	if !roIdentifier.MatchString(column.Name.String()) || !column.Qualifier.Qualifier.IsEmpty() {
		return "", fmt.Errorf("unsupported column reference")
	}
	if !column.Qualifier.IsEmpty() {
		expected := c.collection
		if c.alias != "" {
			expected = c.alias
		}
		if column.Qualifier.Name.String() != expected {
			return "", fmt.Errorf("column qualifier does not match collection")
		}
	}
	return column.Name.String(), nil
}

func roLiteral(value interface{}) roDocument { return roDocument{"$literal": value} }
func roField(name string) roDocument         { return roDocument{"$ifNull": roArray{"$" + name, nil}} }
func roNonNull(value interface{}) roDocument { return roDocument{"$ne": roArray{value, nil}} }
func roDirection(order *sqlparser.Order) int {
	if order.Direction == sqlparser.DescScr {
		return -1
	}
	return 1
}

func (c *roCompiler) rowOrder(order *sqlparser.Order, selections []roSelection) (string, error) {
	column, ok := order.Expr.(*sqlparser.ColName)
	if !ok {
		return "", fmt.Errorf("ORDER BY requires one column or projection alias")
	}
	if column.Qualifier.IsEmpty() {
		for _, selection := range selections {
			if selection.name == column.Name.String() {
				if selection.isLiteral {
					return "", nil
				}
				return selection.column, nil
			}
		}
	}
	return c.column(column)
}

func (c *roCompiler) groupOrder(order *sqlparser.Order, selections []roSelection) (string, error) {
	column, ok := order.Expr.(*sqlparser.ColName)
	if !ok || !column.Qualifier.IsEmpty() {
		return "", fmt.Errorf("aggregate ORDER BY requires a selected output alias")
	}
	for _, selection := range selections {
		if selection.name == column.Name.String() {
			return selection.name, nil
		}
	}
	return "", fmt.Errorf("aggregate ORDER BY requires a selected output alias")
}

func roPositiveInteger(expression sqlparser.Expr) (int64, error) {
	value, ok := expression.(*sqlparser.SQLVal)
	if !ok || value.Type != sqlparser.IntVal {
		return 0, fmt.Errorf("LIMIT and OFFSET require nonnegative integer literals")
	}
	parsed, err := strconv.ParseInt(string(value.Val), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("LIMIT or OFFSET is outside int64 range")
	}
	return parsed, nil
}

func roScalarLiteral(expression sqlparser.Expr) (interface{}, error) {
	sign := ""
	if unary, ok := expression.(*sqlparser.UnaryExpr); ok {
		if unary.Operator != "+" && unary.Operator != "-" {
			return nil, fmt.Errorf("unsupported unary expression")
		}
		sign = unary.Operator
		expression = unary.Expr
	}
	switch value := expression.(type) {
	case *sqlparser.NullVal:
		if sign == "" {
			return nil, nil
		}
	case sqlparser.BoolVal:
		if sign == "" {
			return bool(value), nil
		}
	case *sqlparser.SQLVal:
		switch value.Type {
		case sqlparser.StrVal:
			if sign == "" {
				return string(value.Val), nil
			}
		case sqlparser.IntVal:
			integer, err := strconv.ParseInt(sign+string(value.Val), 10, 64)
			if err == nil {
				return integer, nil
			}
			return nil, fmt.Errorf("SQL integer is outside int64 range")
		case sqlparser.FloatVal:
			decimal, err := bson.ParseDecimal128(sign + string(value.Val))
			if err != nil {
				return nil, fmt.Errorf("SQL decimal is outside Decimal128 precision or range")
			}
			return roDocument{"$numberDecimal": decimal.String()}, nil
		}
	}
	return nil, fmt.Errorf("unsupported scalar expression")
}
