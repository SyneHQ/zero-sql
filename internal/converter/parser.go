package converter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xwb1989/sqlparser"
)

// BuildMatchStage recursively builds the $match stage from a WHERE expression
func (c *Converter) BuildMatchStage(expr sqlparser.Expr) (map[string]interface{}, error) {
	switch e := expr.(type) {
	case *sqlparser.ComparisonExpr:
		return c.buildComparison(e)
	case *sqlparser.AndExpr:
		left, err := c.BuildMatchStage(e.Left)
		if err != nil {
			return nil, fmt.Errorf("failed to parse left side of AND: %w", err)
		}
		right, err := c.BuildMatchStage(e.Right)
		if err != nil {
			return nil, fmt.Errorf("failed to parse right side of AND: %w", err)
		}
		return map[string]interface{}{"$and": []interface{}{left, right}}, nil
	case *sqlparser.OrExpr:
		left, err := c.BuildMatchStage(e.Left)
		if err != nil {
			return nil, fmt.Errorf("failed to parse left side of OR: %w", err)
		}
		right, err := c.BuildMatchStage(e.Right)
		if err != nil {
			return nil, fmt.Errorf("failed to parse right side of OR: %w", err)
		}
		return map[string]interface{}{"$or": []interface{}{left, right}}, nil
	case *sqlparser.ParenExpr:
		return c.BuildMatchStage(e.Expr)
	case *sqlparser.NotExpr:
		inner, err := c.BuildMatchStage(e.Expr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse NOT expression: %w", err)
		}
		return map[string]interface{}{"$not": inner}, nil
	case *sqlparser.IsExpr:
		return c.buildIsExpression(e)
	case *sqlparser.RangeCond:
		return c.buildRangeCondition(e)
	default:
		return nil, fmt.Errorf("unsupported WHERE clause expression: %T", e)
	}
}

// buildComparison handles comparison expressions like =, >, <, LIKE, etc.
func (c *Converter) buildComparison(expr *sqlparser.ComparisonExpr) (map[string]interface{}, error) {
	var colName string

	// Handle different types of left expressions
	switch leftExpr := expr.Left.(type) {
	case *sqlparser.ColName:
		colName = c.getFullColumnName(leftExpr)
	case *sqlparser.SQLVal:
		// Handle quoted identifiers that might be parsed as string values
		if leftExpr.Type == sqlparser.StrVal {
			colName = string(leftExpr.Val)
		} else {
			return nil, fmt.Errorf("comparison must be applied to a column, got SQLVal of type %v", leftExpr.Type)
		}
	default:
		return nil, fmt.Errorf("left side of comparison must be a column, got: %T", expr.Left)
	}

	// Handle IN and NOT IN separately as they have different value structures
	if strings.ToUpper(expr.Operator) == "IN" || strings.ToUpper(expr.Operator) == "NOT IN" {
		return c.buildInExpression(colName, expr.Right, expr.Operator)
	}

	// Handle LIKE - check if it was originally ILIKE
	if strings.ToUpper(expr.Operator) == "LIKE" {
		// Extract the pattern to check if this was originally ILIKE
		val, err := c.extractValue(expr.Right)
		if err == nil {
			if pattern, ok := val.(string); ok {
				// Check if this pattern was originally from an ILIKE operator
				key := fmt.Sprintf("ilike:%s", pattern)
				wasIlike := c.ilikeMap != nil && c.ilikeMap[key]
				return c.buildLikeExpression(colName, expr.Right, wasIlike)
			}
		}
		return c.buildLikeExpression(colName, expr.Right, false)
	}

	val, err := c.extractValue(expr.Right)
	if err != nil {
		return nil, fmt.Errorf("failed to extract value from comparison: %w", err)
	}

	mongoOp, err := ConvertSQLOperator(expr.Operator)
	if err != nil {
		return nil, err
	}

	if mongoOp == "$eq" {
		// For equality, we can use simplified syntax
		return map[string]interface{}{colName: val}, nil
	}

	return map[string]interface{}{colName: map[string]interface{}{mongoOp: val}}, nil
}

// buildInExpression handles IN and NOT IN operators
func (c *Converter) buildInExpression(colName string, rightExpr sqlparser.Expr, operator string) (map[string]interface{}, error) {
	valTuple, ok := rightExpr.(sqlparser.ValTuple)
	if !ok {
		return nil, fmt.Errorf("IN/NOT IN operator requires a tuple of values")
	}

	var values []interface{}
	for _, val := range valTuple {
		v, err := c.extractValue(val)
		if err != nil {
			return nil, fmt.Errorf("failed to extract value from IN clause: %w", err)
		}
		values = append(values, v)
	}

	mongoOp, err := ConvertSQLOperator(operator)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{colName: map[string]interface{}{mongoOp: values}}, nil
}

// buildLikeExpression handles LIKE and ILIKE operators
func (c *Converter) buildLikeExpression(colName string, rightExpr sqlparser.Expr, caseInsensitive bool) (map[string]interface{}, error) {
	val, err := c.extractValue(rightExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract value from LIKE expression: %w", err)
	}

	pattern, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("LIKE/ILIKE operator requires a string value")
	}

	regexCondition := ConvertLikePattern(pattern, caseInsensitive)
	return map[string]interface{}{colName: regexCondition}, nil
}

// buildIsExpression handles IS NULL and IS NOT NULL
func (c *Converter) buildIsExpression(expr *sqlparser.IsExpr) (map[string]interface{}, error) {
	var colName string

	// Handle different types of expressions
	switch exprType := expr.Expr.(type) {
	case *sqlparser.ColName:
		colName = c.getFullColumnName(exprType)
	case *sqlparser.SQLVal:
		// Handle quoted identifiers that might be parsed as string values
		if exprType.Type == sqlparser.StrVal {
			colName = string(exprType.Val)
		} else {
			return nil, fmt.Errorf("IS expression must be applied to a column, got SQLVal of type %v", exprType.Type)
		}
	default:
		return nil, fmt.Errorf("IS expression must be applied to a column, got: %T", expr.Expr)
	}

	switch expr.Operator {
	case "is null":
		return map[string]interface{}{colName: nil}, nil
	case "is not null":
		return map[string]interface{}{colName: map[string]interface{}{"$ne": nil}}, nil
	default:
		return nil, fmt.Errorf("unsupported IS operator: %s", expr.Operator)
	}
}

// buildRangeCondition handles BETWEEN expressions
func (c *Converter) buildRangeCondition(expr *sqlparser.RangeCond) (map[string]interface{}, error) {
	var colName string

	// Handle different types of left expressions
	switch leftExpr := expr.Left.(type) {
	case *sqlparser.ColName:
		colName = c.getFullColumnName(leftExpr)
	case *sqlparser.SQLVal:
		// Handle quoted identifiers that might be parsed as string values
		if leftExpr.Type == sqlparser.StrVal {
			colName = string(leftExpr.Val)
		} else {
			return nil, fmt.Errorf("BETWEEN expression must be applied to a column, got SQLVal of type %v", leftExpr.Type)
		}
	default:
		return nil, fmt.Errorf("BETWEEN expression must be applied to a column, got: %T", expr.Left)
	}

	fromVal, err := c.extractValue(expr.From)
	if err != nil {
		return nil, fmt.Errorf("failed to extract FROM value in BETWEEN: %w", err)
	}

	toVal, err := c.extractValue(expr.To)
	if err != nil {
		return nil, fmt.Errorf("failed to extract TO value in BETWEEN: %w", err)
	}

	if expr.Operator == "between" {
		return map[string]interface{}{
			colName: map[string]interface{}{
				"$gte": fromVal,
				"$lte": toVal,
			},
		}, nil
	} else if expr.Operator == "not between" {
		return map[string]interface{}{
			"$or": []interface{}{
				map[string]interface{}{colName: map[string]interface{}{"$lt": fromVal}},
				map[string]interface{}{colName: map[string]interface{}{"$gt": toVal}},
			},
		}, nil
	}

	return nil, fmt.Errorf("unsupported BETWEEN operator: %s", expr.Operator)
}

// BuildProjectStage constructs the $project stage from the SELECT expressions
func (c *Converter) BuildProjectStage(selectExprs sqlparser.SelectExprs, fromTable string) (map[string]interface{}, error) {
	project := make(map[string]interface{})

	// If it's `SELECT *`, include all fields
	if len(selectExprs) == 1 {
		if _, ok := selectExprs[0].(*sqlparser.StarExpr); ok {
			// Return empty project to include all fields
			return project, nil
		}
	}

	for _, selExpr := range selectExprs {
		switch expr := selExpr.(type) {
		case *sqlparser.StarExpr:
			// SELECT * - include all fields
			return project, nil
		case *sqlparser.AliasedExpr:
			if err := c.handleAliasedExpression(expr, project); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported select expression type: %T", selExpr)
		}
	}

	// Exclude _id by default unless explicitly included
	if _, exists := project["_id"]; !exists {
		project["_id"] = 0
	}

	return project, nil
}

// handleAliasedExpression processes aliased expressions in SELECT clause
func (c *Converter) handleAliasedExpression(expr *sqlparser.AliasedExpr, project map[string]interface{}) error {
	outputFieldName := ""
	if !expr.As.IsEmpty() {
		outputFieldName = expr.As.String()
	}

	switch e := expr.Expr.(type) {
	case *sqlparser.ColName:
		fieldName := c.getFullColumnName(e)
		if outputFieldName == "" {
			outputFieldName = e.Name.String()
		}
		project[outputFieldName] = "$" + fieldName
	case *sqlparser.SQLVal:
		// Handle quoted identifiers that might be parsed as string values
		if e.Type == sqlparser.StrVal {
			fieldName := string(e.Val)
			if outputFieldName == "" {
				outputFieldName = fieldName
			}
			project[outputFieldName] = "$" + fieldName
		} else {
			return fmt.Errorf("unsupported SQLVal type in SELECT: %v", e.Type)
		}
	case *sqlparser.FuncExpr:
		if err := c.handleAggregationFunction(e, outputFieldName, project); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported expression in SELECT: %T", expr.Expr)
	}

	return nil
}

// handleAggregationFunction processes aggregation functions like COUNT, SUM, etc.
func (c *Converter) handleAggregationFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	funcName := expr.Name.String()

	if alias == "" {
		alias = strings.ToLower(funcName)
	}

	mongoFunc, err := ConvertAggregationFunction(funcName)
	if err != nil {
		return err
	}

	// Handle different aggregation functions
	if strings.ToUpper(funcName) == "COUNT" {
		if len(expr.Exprs) == 1 {
			if _, ok := expr.Exprs[0].(*sqlparser.StarExpr); ok {
				// COUNT(*)
				project[alias] = map[string]interface{}{mongoFunc: 1}
				return nil
			}
		}
		// COUNT(column) - count non-null values
		project[alias] = map[string]interface{}{
			"$sum": map[string]interface{}{
				"$cond": []interface{}{
					map[string]interface{}{"$ne": []interface{}{"$" + c.getColumnNameFromExpr(expr.Exprs[0]), nil}},
					1,
					0,
				},
			},
		}
	} else {
		// Other aggregation functions (SUM, AVG, MIN, MAX)
		if len(expr.Exprs) != 1 {
			return fmt.Errorf("%s function requires exactly one argument", funcName)
		}
		columnName := c.getColumnNameFromExpr(expr.Exprs[0])
		project[alias] = map[string]interface{}{mongoFunc: "$" + columnName}
	}

	return nil
}

// BuildSortStage constructs the $sort stage from ORDER BY clause
func (c *Converter) BuildSortStage(orderBy sqlparser.OrderBy) (map[string]interface{}, error) {
	sort := make(map[string]interface{})

	for _, order := range orderBy {
		var colName string

		// Handle different types of expressions in ORDER BY
		switch expr := order.Expr.(type) {
		case *sqlparser.ColName:
			colName = c.getFullColumnName(expr)
		case *sqlparser.SQLVal:
			// Handle quoted identifiers that might be parsed as string values
			if expr.Type == sqlparser.StrVal {
				colName = string(expr.Val)
			} else {
				return nil, fmt.Errorf("ORDER BY only supports column names, got SQLVal of type %v", expr.Type)
			}
		default:
			return nil, fmt.Errorf("ORDER BY only supports column names, got: %T", order.Expr)
		}

		direction := 1 // ASC
		if order.Direction == "desc" {
			direction = -1
		}

		sort[colName] = direction
	}

	return sort, nil
}

// BuildGroupStage constructs the $group stage from GROUP BY clause
func (c *Converter) BuildGroupStage(groupBy sqlparser.GroupBy, selectExprs sqlparser.SelectExprs) (map[string]interface{}, error) {
	group := map[string]interface{}{
		"_id": make(map[string]interface{}),
	}

	// Handle GROUP BY columns
	idGroup := group["_id"].(map[string]interface{})
	for _, expr := range groupBy {
		var colName, fieldName string

		// Handle different types of expressions in GROUP BY
		switch groupExpr := expr.(type) {
		case *sqlparser.ColName:
			colName = c.getFullColumnName(groupExpr)
			fieldName = groupExpr.Name.String()
		case *sqlparser.SQLVal:
			// Handle quoted identifiers that might be parsed as string values
			if groupExpr.Type == sqlparser.StrVal {
				colName = string(groupExpr.Val)
				fieldName = colName
			} else {
				return nil, fmt.Errorf("GROUP BY only supports column names, got SQLVal of type %v", groupExpr.Type)
			}
		default:
			return nil, fmt.Errorf("GROUP BY only supports column names, got: %T", expr)
		}

		idGroup[fieldName] = "$" + colName
	}

	// Handle aggregation functions in SELECT
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			if funcExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				alias := funcExpr.Name.String()
				if !aliasedExpr.As.IsEmpty() {
					alias = aliasedExpr.As.String()
				}

				if err := c.handleAggregationFunction(funcExpr, alias, group); err != nil {
					return nil, err
				}
			}
		}
	}

	return group, nil
}

// extractValue extracts the actual value from a SQL expression
func (c *Converter) extractValue(expr sqlparser.Expr) (interface{}, error) {
	switch v := expr.(type) {
	case *sqlparser.SQLVal:
		return c.convertSQLVal(v)
	case *sqlparser.ColName:
		// Reference to another field
		return "$" + c.getFullColumnName(v), nil
	case *sqlparser.NullVal:
		return nil, nil
	case sqlparser.BoolVal:
		return bool(v), nil
	default:
		return nil, fmt.Errorf("unsupported value type: %T", expr)
	}
}

// convertSQLVal converts SQLVal to appropriate Go type
func (c *Converter) convertSQLVal(val *sqlparser.SQLVal) (interface{}, error) {
	switch val.Type {
	case sqlparser.StrVal:
		return string(val.Val), nil
	case sqlparser.IntVal:
		return strconv.ParseInt(string(val.Val), 10, 64)
	case sqlparser.FloatVal:
		return strconv.ParseFloat(string(val.Val), 64)
	case sqlparser.HexVal:
		return string(val.Val), nil
	case sqlparser.BitVal:
		return string(val.Val), nil
	default:
		return string(val.Val), nil
	}
}

// getFullColumnName returns the full column name with table prefix if available
func (c *Converter) getFullColumnName(col *sqlparser.ColName) string {
	if !col.Qualifier.IsEmpty() {
		return col.Qualifier.Name.String() + "." + col.Name.String()
	}
	return col.Name.String()
}

// getColumnNameFromExpr extracts column name from expression
func (c *Converter) getColumnNameFromExpr(expr sqlparser.SelectExpr) string {
	if aliasedExpr, ok := expr.(*sqlparser.AliasedExpr); ok {
		switch e := aliasedExpr.Expr.(type) {
		case *sqlparser.ColName:
			return c.getFullColumnName(e)
		case *sqlparser.SQLVal:
			// Handle quoted identifiers that might be parsed as string values
			if e.Type == sqlparser.StrVal {
				return string(e.Val)
			}
		}
	}
	return ""
}
