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
		// Check if it's an aggregation function first
		if _, err := ConvertAggregationFunction(e.Name.String()); err == nil {
			if err := c.handleAggregationFunction(e, outputFieldName, project); err != nil {
				return err
			}
		} else {
			// Handle transformation functions
			if err := c.handleTransformationFunction(e, outputFieldName, project); err != nil {
				return err
			}
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

// handleTransformationFunction processes transformation functions like STRFTIME, CAST, ROUND, etc.
func (c *Converter) handleTransformationFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	funcName := expr.Name.String()

	if alias == "" {
		alias = strings.ToLower(funcName)
	}

	// Handle different transformation functions
	switch strings.ToUpper(funcName) {
	case "STRFTIME":
		return c.handleStrftimeFunction(expr, alias, project)
	case "ROUND":
		return c.handleRoundFunction(expr, alias, project)
	// String functions
	case "UPPER":
		return c.handleUpperFunction(expr, alias, project)
	case "LOWER":
		return c.handleLowerFunction(expr, alias, project)
	case "CONCAT":
		return c.handleConcatFunction(expr, alias, project)
	case "SUBSTR", "SUBSTRING":
		return c.handleSubstringFunction(expr, alias, project)
	case "LENGTH", "LEN":
		return c.handleLengthFunction(expr, alias, project)
	case "REPLACE":
		return c.handleReplaceFunction(expr, alias, project)
	// Math functions
	case "ABS":
		return c.handleAbsFunction(expr, alias, project)
	case "CEIL":
		return c.handleCeilFunction(expr, alias, project)
	case "FLOOR":
		return c.handleFloorFunction(expr, alias, project)
	case "POWER", "POW":
		return c.handlePowerFunction(expr, alias, project)
	case "SQRT":
		return c.handleSqrtFunction(expr, alias, project)
	case "MOD":
		return c.handleModFunction(expr, alias, project)
	// Date functions
	case "YEAR":
		return c.handleYearFunction(expr, alias, project)
	case "MONTH":
		return c.handleMonthFunction(expr, alias, project)
	case "DAY":
		return c.handleDayFunction(expr, alias, project)
	case "DATEADD":
		return c.handleDateAddFunction(expr, alias, project)
	case "DATEDIFF":
		return c.handleDateDiffFunction(expr, alias, project)
	// Conditional functions
	case "COALESCE":
		return c.handleCoalesceFunction(expr, alias, project)
	case "NULLIF":
		return c.handleNullifFunction(expr, alias, project)
	default:
		return fmt.Errorf("unsupported transformation function: %s", funcName)
	}
}

// handleStrftimeFunction handles STRFTIME(date, format) -> $dateToString
func (c *Converter) handleStrftimeFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 2 {
		return fmt.Errorf("STRFTIME function requires exactly 2 arguments: date and format")
	}

	// Extract date expression
	_, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract date expression from STRFTIME: %w", err)
	}
	dateField := c.getColumnNameFromExpr(expr.Exprs[0])

	// Extract format string
	formatExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract format expression from STRFTIME: %w", err)
	}
	formatVal, err := c.extractValue(formatExpr)
	if err != nil {
		return fmt.Errorf("failed to extract format from STRFTIME: %w", err)
	}
	format, ok := formatVal.(string)
	if !ok {
		return fmt.Errorf("STRFTIME format must be a string")
	}

	// Convert SQLite strftime format to MongoDB dateToString format
	mongoFormat := c.convertStrftimeFormat(format)

	project[alias] = map[string]interface{}{
		"$dateToString": map[string]interface{}{
			"date":   "$" + dateField,
			"format": mongoFormat,
		},
	}

	return nil
}

// handleRoundFunction handles ROUND(value, decimals) -> $round
func (c *Converter) handleRoundFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) < 1 || len(expr.Exprs) > 2 {
		return fmt.Errorf("ROUND function requires 1 or 2 arguments")
	}

	// Extract value expression
	_, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract value expression from ROUND: %w", err)
	}
	valueField := c.getColumnNameFromExpr(expr.Exprs[0])

	// Default precision is 0
	precision := 0
	if len(expr.Exprs) == 2 {
		precisionExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
		if err != nil {
			return fmt.Errorf("failed to extract precision expression from ROUND: %w", err)
		}
		precisionVal, err := c.extractValue(precisionExpr)
		if err != nil {
			return fmt.Errorf("failed to extract precision from ROUND: %w", err)
		}
		if p, ok := precisionVal.(int64); ok {
			precision = int(p)
		} else {
			return fmt.Errorf("ROUND precision must be an integer")
		}
	}

	if precision == 0 {
		project[alias] = map[string]interface{}{
			"$round": "$" + valueField,
		}
	} else {
		project[alias] = map[string]interface{}{
			"$round": []interface{}{"$" + valueField, precision},
		}
	}

	return nil
}

// buildFunctionExpression builds MongoDB expression for function calls in GROUP BY or SELECT
func (c *Converter) buildFunctionExpression(expr *sqlparser.FuncExpr) (interface{}, error) {
	funcName := strings.ToUpper(expr.Name.String())

	switch funcName {
	case "STRFTIME":
		return c.buildStrftimeExpression(expr)
	case "ROUND":
		return c.buildRoundExpression(expr)
	default:
		return nil, fmt.Errorf("unsupported function in GROUP BY: %s", funcName)
	}
}

// buildStrftimeExpression builds $dateToString expression for STRFTIME
func (c *Converter) buildStrftimeExpression(expr *sqlparser.FuncExpr) (interface{}, error) {
	if len(expr.Exprs) != 2 {
		return nil, fmt.Errorf("STRFTIME function requires exactly 2 arguments: date and format")
	}

	// Extract date expression
	dateExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return nil, fmt.Errorf("failed to extract date expression from STRFTIME: %w", err)
	}
	dateValue, err := c.extractValue(dateExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract date from STRFTIME: %w", err)
	}

	// Extract format string
	formatExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return nil, fmt.Errorf("failed to extract format expression from STRFTIME: %w", err)
	}
	formatVal, err := c.extractValue(formatExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract format from STRFTIME: %w", err)
	}
	format, ok := formatVal.(string)
	if !ok {
		return nil, fmt.Errorf("STRFTIME format must be a string")
	}

	// Convert SQLite strftime format to MongoDB dateToString format
	mongoFormat := c.convertStrftimeFormat(format)

	return map[string]interface{}{
		"$dateToString": map[string]interface{}{
			"date":   dateValue,
			"format": mongoFormat,
		},
	}, nil
}

// buildRoundExpression builds $round expression for ROUND
func (c *Converter) buildRoundExpression(expr *sqlparser.FuncExpr) (interface{}, error) {
	if len(expr.Exprs) < 1 || len(expr.Exprs) > 2 {
		return nil, fmt.Errorf("ROUND function requires 1 or 2 arguments")
	}

	// Extract value expression
	valueExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return nil, fmt.Errorf("failed to extract value expression from ROUND: %w", err)
	}
	valueValue, err := c.extractValue(valueExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract value from ROUND: %w", err)
	}

	// Default precision is 0
	if len(expr.Exprs) == 1 {
		return map[string]interface{}{
			"$round": valueValue,
		}, nil
	}

	// With precision
	precisionExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return nil, fmt.Errorf("failed to extract precision expression from ROUND: %w", err)
	}
	precisionVal, err := c.extractValue(precisionExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract precision from ROUND: %w", err)
	}

	return map[string]interface{}{
		"$round": []interface{}{valueValue, precisionVal},
	}, nil
}

// convertStrftimeFormat converts SQLite strftime format to MongoDB dateToString format
func (c *Converter) convertStrftimeFormat(sqliteFormat string) string {
	// Basic conversion of common strftime formats to MongoDB formats
	// SQLite strftime: %Y %m %d %H %M %S
	// MongoDB dateToString: %Y %m %d %H %M %S

	mongoFormat := strings.ReplaceAll(sqliteFormat, "%Y", "%Y") // Year with century
	mongoFormat = strings.ReplaceAll(mongoFormat, "%m", "%m")   // Month as decimal
	mongoFormat = strings.ReplaceAll(mongoFormat, "%d", "%d")   // Day of month
	mongoFormat = strings.ReplaceAll(mongoFormat, "%H", "%H")   // Hour (24-hour)
	mongoFormat = strings.ReplaceAll(mongoFormat, "%M", "%M")   // Minute
	mongoFormat = strings.ReplaceAll(mongoFormat, "%S", "%S")   // Second

	return mongoFormat
}

// ===== STRING FUNCTIONS =====

// handleUpperFunction handles UPPER(string) -> $toUpper
func (c *Converter) handleUpperFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("UPPER function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from UPPER: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from UPPER: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$toUpper": input,
	}

	return nil
}

// handleLowerFunction handles LOWER(string) -> $toLower
func (c *Converter) handleLowerFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("LOWER function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from LOWER: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from LOWER: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$toLower": input,
	}

	return nil
}

// handleConcatFunction handles CONCAT(str1, str2, ...) -> $concat
func (c *Converter) handleConcatFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) < 2 {
		return fmt.Errorf("CONCAT function requires at least 2 arguments")
	}

	var concatArgs []interface{}
	for _, arg := range expr.Exprs {
		innerExpr, err := c.extractExprFromSelectExpr(arg)
		if err != nil {
			return fmt.Errorf("failed to extract expression from CONCAT: %w", err)
		}
		value, err := c.extractValue(innerExpr)
		if err != nil {
			return fmt.Errorf("failed to extract value from CONCAT: %w", err)
		}
		concatArgs = append(concatArgs, value)
	}

	project[alias] = map[string]interface{}{
		"$concat": concatArgs,
	}

	return nil
}

// handleSubstringFunction handles SUBSTR/SUBSTRING(string, start, length) -> $substr
func (c *Converter) handleSubstringFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) < 2 || len(expr.Exprs) > 3 {
		return fmt.Errorf("SUBSTR/SUBSTRING function requires 2 or 3 arguments")
	}

	// Extract string
	stringExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract string from SUBSTR: %w", err)
	}
	input, err := c.extractValue(stringExpr)
	if err != nil {
		return fmt.Errorf("failed to extract string value from SUBSTR: %w", err)
	}

	// Extract start position
	startExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract start position from SUBSTR: %w", err)
	}
	start, err := c.extractValue(startExpr)
	if err != nil {
		return fmt.Errorf("failed to extract start position value from SUBSTR: %w", err)
	}

	// Extract length (optional)
	var length interface{} = 1 // Default length
	if len(expr.Exprs) == 3 {
		lengthExpr, err := c.extractExprFromSelectExpr(expr.Exprs[2])
		if err != nil {
			return fmt.Errorf("failed to extract length from SUBSTR: %w", err)
		}
		length, err = c.extractValue(lengthExpr)
		if err != nil {
			return fmt.Errorf("failed to extract length value from SUBSTR: %w", err)
		}
	}

	project[alias] = map[string]interface{}{
		"$substr": []interface{}{input, start, length},
	}

	return nil
}

// handleLengthFunction handles LENGTH/LEN(string) -> $strLenBytes
func (c *Converter) handleLengthFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("LENGTH/LEN function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from LENGTH: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from LENGTH: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$strLenBytes": input,
	}

	return nil
}

// handleReplaceFunction handles REPLACE(string, find, replace) -> $replaceAll
func (c *Converter) handleReplaceFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 3 {
		return fmt.Errorf("REPLACE function requires exactly 3 arguments")
	}

	// Extract string
	stringExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract string from REPLACE: %w", err)
	}
	input, err := c.extractValue(stringExpr)
	if err != nil {
		return fmt.Errorf("failed to extract string value from REPLACE: %w", err)
	}

	// Extract find string
	findExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract find string from REPLACE: %w", err)
	}
	find, err := c.extractValue(findExpr)
	if err != nil {
		return fmt.Errorf("failed to extract find string value from REPLACE: %w", err)
	}

	// Extract replace string
	replaceExpr, err := c.extractExprFromSelectExpr(expr.Exprs[2])
	if err != nil {
		return fmt.Errorf("failed to extract replace string from REPLACE: %w", err)
	}
	replace, err := c.extractValue(replaceExpr)
	if err != nil {
		return fmt.Errorf("failed to extract replace string value from REPLACE: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$replaceAll": map[string]interface{}{
			"input":       input,
			"find":        find,
			"replacement": replace,
		},
	}

	return nil
}

// ===== MATH FUNCTIONS =====

// handleAbsFunction handles ABS(number) -> $abs
func (c *Converter) handleAbsFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("ABS function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from ABS: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from ABS: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$abs": input,
	}

	return nil
}

// handleCeilFunction handles CEIL(number) -> $ceil
func (c *Converter) handleCeilFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("CEIL function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from CEIL: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from CEIL: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$ceil": input,
	}

	return nil
}

// handleFloorFunction handles FLOOR(number) -> $floor
func (c *Converter) handleFloorFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("FLOOR function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from FLOOR: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from FLOOR: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$floor": input,
	}

	return nil
}

// handlePowerFunction handles POWER/POW(base, exponent) -> $pow
func (c *Converter) handlePowerFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 2 {
		return fmt.Errorf("POWER/POW function requires exactly 2 arguments")
	}

	baseExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract base from POWER: %w", err)
	}
	base, err := c.extractValue(baseExpr)
	if err != nil {
		return fmt.Errorf("failed to extract base value from POWER: %w", err)
	}

	exponentExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract exponent from POWER: %w", err)
	}
	exponent, err := c.extractValue(exponentExpr)
	if err != nil {
		return fmt.Errorf("failed to extract exponent value from POWER: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$pow": []interface{}{base, exponent},
	}

	return nil
}

// handleSqrtFunction handles SQRT(number) -> $sqrt
func (c *Converter) handleSqrtFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("SQRT function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from SQRT: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from SQRT: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$sqrt": input,
	}

	return nil
}

// handleModFunction handles MOD(dividend, divisor) -> $mod
func (c *Converter) handleModFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 2 {
		return fmt.Errorf("MOD function requires exactly 2 arguments")
	}

	dividendExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract dividend from MOD: %w", err)
	}
	dividend, err := c.extractValue(dividendExpr)
	if err != nil {
		return fmt.Errorf("failed to extract dividend value from MOD: %w", err)
	}

	divisorExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract divisor from MOD: %w", err)
	}
	divisor, err := c.extractValue(divisorExpr)
	if err != nil {
		return fmt.Errorf("failed to extract divisor value from MOD: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$mod": []interface{}{dividend, divisor},
	}

	return nil
}

// ===== DATE FUNCTIONS =====

// handleYearFunction handles YEAR(date) -> $year
func (c *Converter) handleYearFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("YEAR function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from YEAR: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from YEAR: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$year": input,
	}

	return nil
}

// handleMonthFunction handles MONTH(date) -> $month
func (c *Converter) handleMonthFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("MONTH function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from MONTH: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from MONTH: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$month": input,
	}

	return nil
}

// handleDayFunction handles DAY(date) -> $dayOfMonth
func (c *Converter) handleDayFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 1 {
		return fmt.Errorf("DAY function requires exactly 1 argument")
	}

	innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract expression from DAY: %w", err)
	}
	input, err := c.extractValue(innerExpr)
	if err != nil {
		return fmt.Errorf("failed to extract value from DAY: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$dayOfMonth": input,
	}

	return nil
}

// handleDateAddFunction handles DATEADD(date, interval, unit) -> $dateAdd
func (c *Converter) handleDateAddFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 3 {
		return fmt.Errorf("DATEADD function requires exactly 3 arguments: date, interval, unit")
	}

	// Extract date
	dateExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract date from DATEADD: %w", err)
	}
	date, err := c.extractValue(dateExpr)
	if err != nil {
		return fmt.Errorf("failed to extract date value from DATEADD: %w", err)
	}

	// Extract interval
	intervalExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract interval from DATEADD: %w", err)
	}
	interval, err := c.extractValue(intervalExpr)
	if err != nil {
		return fmt.Errorf("failed to extract interval value from DATEADD: %w", err)
	}

	// Extract unit
	unitExpr, err := c.extractExprFromSelectExpr(expr.Exprs[2])
	if err != nil {
		return fmt.Errorf("failed to extract unit from DATEADD: %w", err)
	}
	unit, err := c.extractValue(unitExpr)
	if err != nil {
		return fmt.Errorf("failed to extract unit value from DATEADD: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$dateAdd": map[string]interface{}{
			"startDate": date,
			"unit":      unit,
			"amount":    interval,
		},
	}

	return nil
}

// handleDateDiffFunction handles DATEDIFF(end_date, start_date, unit) -> $dateDiff
func (c *Converter) handleDateDiffFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 3 {
		return fmt.Errorf("DATEDIFF function requires exactly 3 arguments: end_date, start_date, unit")
	}

	// Extract end date
	endDateExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract end date from DATEDIFF: %w", err)
	}
	endDate, err := c.extractValue(endDateExpr)
	if err != nil {
		return fmt.Errorf("failed to extract end date value from DATEDIFF: %w", err)
	}

	// Extract start date
	startDateExpr, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract start date from DATEDIFF: %w", err)
	}
	startDate, err := c.extractValue(startDateExpr)
	if err != nil {
		return fmt.Errorf("failed to extract start date value from DATEDIFF: %w", err)
	}

	// Extract unit
	unitExpr, err := c.extractExprFromSelectExpr(expr.Exprs[2])
	if err != nil {
		return fmt.Errorf("failed to extract unit from DATEDIFF: %w", err)
	}
	unit, err := c.extractValue(unitExpr)
	if err != nil {
		return fmt.Errorf("failed to extract unit value from DATEDIFF: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$dateDiff": map[string]interface{}{
			"startDate": startDate,
			"endDate":   endDate,
			"unit":      unit,
		},
	}

	return nil
}

// ===== CONDITIONAL FUNCTIONS =====

// handleCoalesceFunction handles COALESCE(val1, val2, ...) -> $ifNull with nested $ifNull
func (c *Converter) handleCoalesceFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) < 1 {
		return fmt.Errorf("COALESCE function requires at least 1 argument")
	}

	if len(expr.Exprs) == 1 {
		// Single argument - just return the value
		innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[0])
		if err != nil {
			return fmt.Errorf("failed to extract expression from COALESCE: %w", err)
		}
		value, err := c.extractValue(innerExpr)
		if err != nil {
			return fmt.Errorf("failed to extract value from COALESCE: %w", err)
		}
		project[alias] = value
		return nil
	}

	// Build nested $ifNull structure
	var ifNull interface{}

	// Start from the last argument and work backwards
	for i := len(expr.Exprs) - 1; i >= 0; i-- {
		innerExpr, err := c.extractExprFromSelectExpr(expr.Exprs[i])
		if err != nil {
			return fmt.Errorf("failed to extract expression from COALESCE: %w", err)
		}
		value, err := c.extractValue(innerExpr)
		if err != nil {
			return fmt.Errorf("failed to extract value from COALESCE: %w", err)
		}

		if ifNull == nil {
			ifNull = value
		} else {
			ifNull = map[string]interface{}{
				"$ifNull": []interface{}{value, ifNull},
			}
		}
	}

	project[alias] = ifNull
	return nil
}

// handleNullifFunction handles NULLIF(expr1, expr2) -> $cond
func (c *Converter) handleNullifFunction(expr *sqlparser.FuncExpr, alias string, project map[string]interface{}) error {
	if len(expr.Exprs) != 2 {
		return fmt.Errorf("NULLIF function requires exactly 2 arguments")
	}

	// Extract expr1
	expr1, err := c.extractExprFromSelectExpr(expr.Exprs[0])
	if err != nil {
		return fmt.Errorf("failed to extract first expression from NULLIF: %w", err)
	}
	val1, err := c.extractValue(expr1)
	if err != nil {
		return fmt.Errorf("failed to extract first value from NULLIF: %w", err)
	}

	// Extract expr2
	expr2, err := c.extractExprFromSelectExpr(expr.Exprs[1])
	if err != nil {
		return fmt.Errorf("failed to extract second expression from NULLIF: %w", err)
	}
	val2, err := c.extractValue(expr2)
	if err != nil {
		return fmt.Errorf("failed to extract second value from NULLIF: %w", err)
	}

	project[alias] = map[string]interface{}{
		"$cond": []interface{}{
			map[string]interface{}{"$eq": []interface{}{val1, val2}},
			nil,
			val1,
		},
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

	// Build a map of aliases to their expressions from SELECT clause
	aliasMap := make(map[string]sqlparser.Expr)
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok && !aliasedExpr.As.IsEmpty() {
			aliasMap[aliasedExpr.As.String()] = aliasedExpr.Expr
		}
	}

	// Handle GROUP BY columns
	idGroup := group["_id"].(map[string]interface{})
	for i, expr := range groupBy {
		// Handle different types of expressions in GROUP BY
		switch groupExpr := expr.(type) {
		case *sqlparser.ColName:
			colName := c.getFullColumnName(groupExpr)
			fieldName := groupExpr.Name.String()

			// Check if this is an alias from the SELECT clause
			if aliasedExpr, isAlias := aliasMap[fieldName]; isAlias {
				// Use the original expression instead of the alias
				fieldName = fmt.Sprintf("group_%d", i)
				aliasExpr, err := c.buildFunctionExpression(aliasedExpr.(*sqlparser.FuncExpr))
				if err != nil {
					return nil, fmt.Errorf("failed to build GROUP BY alias expression: %w", err)
				}
				idGroup[fieldName] = aliasExpr
			} else {
				// Regular column reference
				idGroup[fieldName] = "$" + colName
			}
		case *sqlparser.FuncExpr:
			// Handle function expressions in GROUP BY (like CAST(strftime(...)))
			// Use generic field name for complex expressions
			fieldName := fmt.Sprintf("group_%d", i)
			funcExpr, err := c.buildFunctionExpression(groupExpr)
			if err != nil {
				return nil, fmt.Errorf("failed to build GROUP BY expression: %w", err)
			}
			idGroup[fieldName] = funcExpr
		case *sqlparser.SQLVal:
			// Handle quoted identifiers that might be parsed as string values
			if groupExpr.Type == sqlparser.StrVal {
				colName := string(groupExpr.Val)
				idGroup[colName] = "$" + colName
			} else {
				return nil, fmt.Errorf("GROUP BY only supports column names and functions, got SQLVal of type %v", groupExpr.Type)
			}
		default:
			return nil, fmt.Errorf("GROUP BY only supports column names and functions, got: %T", expr)
		}
	}

	// Handle aggregation functions in SELECT (including nested ones within transformation functions)
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			if funcExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				funcName := strings.ToUpper(funcExpr.Name.String())
				alias := funcExpr.Name.String()
				if !aliasedExpr.As.IsEmpty() {
					alias = aliasedExpr.As.String()
				}

				// Check if it's a transformation function containing aggregation functions
				if _, isTransformFunc := TransformationFunctions[funcName]; isTransformFunc {
					// For transformation functions, check if they contain aggregation functions
					if c.containsAggregationFunction(funcExpr) {
						if err := c.handleNestedAggregationFunction(funcExpr, alias, group); err != nil {
							return nil, err
						}
					}
					// Otherwise, transformation functions are handled in $project, not $group
				} else if _, isAggFunc := AggregationFunctions[funcName]; isAggFunc {
					// Handle direct aggregation functions
					if err := c.handleAggregationFunction(funcExpr, alias, group); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return group, nil
}

// BuildGroupProjectStage constructs the $project stage for GROUP BY queries
func (c *Converter) BuildGroupProjectStage(selectExprs sqlparser.SelectExprs, groupBy sqlparser.GroupBy) (map[string]interface{}, error) {
	project := make(map[string]interface{})

	// Map group field indices to their corresponding SELECT aliases
	// For now, assume they correspond by position (this is a simplification)
	groupIndexToAlias := make(map[int]string)
	groupFuncCount := 0
	selectFuncCount := 0

	// Count GROUP BY function expressions
	for _, groupExpr := range groupBy {
		if _, ok := groupExpr.(*sqlparser.FuncExpr); ok {
			groupFuncCount++
		}
	}

	// Map SELECT function expressions to GROUP BY indices
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			if _, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				if !aliasedExpr.As.IsEmpty() && selectFuncCount < groupFuncCount {
					groupIndexToAlias[selectFuncCount] = aliasedExpr.As.String()
					selectFuncCount++
				}
			}
		}
	}

	// Add the grouped fields - these should reference the _id fields
	for i, alias := range groupIndexToAlias {
		groupFieldName := fmt.Sprintf("group_%d", i)
		project[alias] = "$_id." + groupFieldName
	}

	// Handle all SELECT expressions
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			outputFieldName := ""
			if !aliasedExpr.As.IsEmpty() {
				outputFieldName = aliasedExpr.As.String()
			}

			// Check if this is one of the grouped fields - if so, skip it as it's already handled
			isGroupedField := false
			for _, alias := range groupIndexToAlias {
				if alias == outputFieldName {
					isGroupedField = true
					break
				}
			}
			if isGroupedField {
				continue
			}

			switch e := aliasedExpr.Expr.(type) {
			case *sqlparser.FuncExpr:
				funcName := strings.ToUpper(e.Name.String())
				if _, isTransformFunc := TransformationFunctions[funcName]; isTransformFunc {
					// Handle transformation functions applied to aggregations
					if funcName == "ROUND" {
						// Find the inner aggregation function and reference its result
						if len(e.Exprs) >= 1 {
							if innerAliasedExpr, ok := e.Exprs[0].(*sqlparser.AliasedExpr); ok {
								if innerFuncExpr, ok := innerAliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
									innerFuncName := strings.ToUpper(innerFuncExpr.Name.String())
									// Use the function name as the field name (SUM, AVG, etc.)
									innerFieldName := innerFuncName
									if !innerAliasedExpr.As.IsEmpty() {
										innerFieldName = innerAliasedExpr.As.String()
									}

									// Apply ROUND to the aggregated field
									if len(e.Exprs) == 2 {
										precisionExpr, err := c.extractExprFromSelectExpr(e.Exprs[1])
										if err == nil {
											precision, err := c.extractValue(precisionExpr)
											if err == nil {
												project[outputFieldName] = map[string]interface{}{
													"$round": []interface{}{"$" + innerFieldName, precision},
												}
											}
										} else {
											project[outputFieldName] = map[string]interface{}{
												"$round": "$" + innerFieldName,
											}
										}
									} else {
										project[outputFieldName] = map[string]interface{}{
											"$round": "$" + innerFieldName,
										}
									}
								}
							}
						}
					} else {
						// Other transformation functions
						if err := c.handleTransformationFunction(e, outputFieldName, project); err != nil {
							return nil, err
						}
					}
				} else if _, isAggFunc := AggregationFunctions[funcName]; isAggFunc {
					// Direct aggregation function - just reference the field
					project[outputFieldName] = "$" + outputFieldName
				}
			}
		}
	}

	// Exclude _id by default unless explicitly included
	if _, exists := project["_id"]; !exists {
		project["_id"] = 0
	}

	return project, nil
}

// containsAggregationFunction checks if a function expression contains aggregation functions
func (c *Converter) containsAggregationFunction(funcExpr *sqlparser.FuncExpr) bool {
	for _, expr := range funcExpr.Exprs {
		if aliasedExpr, ok := expr.(*sqlparser.AliasedExpr); ok {
			if innerFuncExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				if _, isAggFunc := AggregationFunctions[strings.ToUpper(innerFuncExpr.Name.String())]; isAggFunc {
					return true
				}
			}
		}
	}
	return false
}

// handleNestedAggregationFunction handles transformation functions that contain aggregation functions
func (c *Converter) handleNestedAggregationFunction(funcExpr *sqlparser.FuncExpr, alias string, group map[string]interface{}) error {
	funcName := strings.ToUpper(funcExpr.Name.String())

	switch funcName {
	case "ROUND":
		// ROUND(aggregation_function, precision)
		if len(funcExpr.Exprs) >= 1 {
			if innerAliasedExpr, ok := funcExpr.Exprs[0].(*sqlparser.AliasedExpr); ok {
				if innerFuncExpr, ok := innerAliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
					innerFuncName := strings.ToUpper(innerFuncExpr.Name.String())
					if _, isAggFunc := AggregationFunctions[innerFuncName]; isAggFunc {
						// Handle the inner aggregation function
						innerAlias := innerFuncExpr.Name.String()
						if !innerAliasedExpr.As.IsEmpty() {
							innerAlias = innerAliasedExpr.As.String()
						}
						if err := c.handleAggregationFunction(innerFuncExpr, innerAlias, group); err != nil {
							return err
						}
					}
				}
			}
		}
	default:
		return fmt.Errorf("unsupported nested function: %s", funcName)
	}

	return nil
}

// functionsEqual checks if two function expressions are equivalent
func (c *Converter) functionsEqual(func1, func2 *sqlparser.FuncExpr) bool {
	if func1.Name.String() != func2.Name.String() || len(func1.Exprs) != len(func2.Exprs) {
		return false
	}

	// Simple comparison - in a full implementation, you'd want to compare the actual expressions
	return true
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

// extractExprFromSelectExpr extracts sqlparser.Expr from sqlparser.SelectExpr
func (c *Converter) extractExprFromSelectExpr(selExpr sqlparser.SelectExpr) (sqlparser.Expr, error) {
	if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
		return aliasedExpr.Expr, nil
	}
	return nil, fmt.Errorf("expected AliasedExpr, got %T", selExpr)
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
