package converter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xwb1989/sqlparser"
)

// Options holds configuration options for the converter
type Options struct {
	Verbose bool
}

// Converter handles the conversion from SQL to MongoDB aggregation pipelines
type Converter struct {
	options  *Options
	ilikeMap map[string]bool
}

// New creates a new converter instance with the given options
func New(opts *Options) *Converter {
	if opts == nil {
		opts = &Options{}
	}
	return &Converter{
		options: opts,
	}
}

// preprocessSQL handles SQL syntax that the parser doesn't natively support
func (c *Converter) preprocessSQL(sqlQuery string) (string, map[string]bool, error) {
	// Track which LIKE operations should be case-insensitive
	ilikePositions := make(map[string]bool)

	processedQuery := sqlQuery

	// Find all ILIKE patterns and their values before converting them to LIKE
	ilikeRegex := regexp.MustCompile(`(?i)\b(\w+)\s+ILIKE\s+('[^']*'|"[^"]*")`)
	matches := ilikeRegex.FindAllStringSubmatch(sqlQuery, -1)

	// Mark the specific patterns that were ILIKE
	for _, match := range matches {
		if len(match) >= 3 {
			pattern := strings.Trim(match[2], `'"`)
			key := fmt.Sprintf("ilike:%s", pattern)
			ilikePositions[key] = true
		}
	}

	// Now replace all ILIKE with LIKE
	ilikeReplaceRegex := regexp.MustCompile(`(?i)\bILIKE\b`)
	processedQuery = ilikeReplaceRegex.ReplaceAllString(processedQuery, "LIKE")

	return processedQuery, ilikePositions, nil
}

// parseSQL preprocesses and parses the SQL query, returning the SELECT statement
func (c *Converter) parseSQL(sqlQuery string) (*sqlparser.Select, error) {
	if strings.TrimSpace(sqlQuery) == "" {
		return nil, fmt.Errorf("SQL query cannot be empty")
	}

	// Preprocess SQL to handle ILIKE and other unsupported syntax
	processedQuery, ilikeMap, err := c.preprocessSQL(sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to preprocess SQL query: %w", err)
	}

	// Store ILIKE mapping in converter for later use
	c.ilikeMap = ilikeMap

	stmt, err := sqlparser.Parse(processedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SQL query: %w", err)
	}

	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		return nil, fmt.Errorf("only SELECT statements are supported, got: %T", stmt)
	}

	if c.options.Verbose {
		fmt.Printf("Parsing SQL statement: %+v\n", selectStmt)
	}

	return selectStmt, nil
}

// ConvertSQLToMongo parses the SQL query and converts it into a MongoDB aggregation pipeline
func (c *Converter) ConvertSQLToMongo(sqlQuery string) ([]map[string]interface{}, error) {
	selectStmt, err := c.parseSQL(sqlQuery)
	if err != nil {
		return nil, err
	}

	return c.buildPipeline(selectStmt)
}

// ConvertSQLToMongoWithCollection parses the SQL query and returns both collection name and pipeline
func (c *Converter) ConvertSQLToMongoWithCollection(sqlQuery string) (string, []map[string]interface{}, error) {
	selectStmt, err := c.parseSQL(sqlQuery)
	if err != nil {
		return "", nil, err
	}

	return c.buildPipelineWithCollection(selectStmt)
}

// buildPipeline constructs the MongoDB aggregation pipeline from the parsed SELECT statement
func (c *Converter) buildPipeline(selectStmt *sqlparser.Select) ([]map[string]interface{}, error) {
	_, pipeline, err := c.buildPipelineWithCollection(selectStmt)
	return pipeline, err
}

// buildFromClause extracts the collection name and builds JOIN stages from the FROM clause
func (c *Converter) buildFromClause(from []sqlparser.TableExpr) (string, []map[string]interface{}, error) {
	if len(from) == 0 {
		return "", nil, fmt.Errorf("FROM clause is required")
	}

	var pipeline []map[string]interface{}
	var fromCollection string

	// Handle different types of FROM expressions
	switch fromExpr := from[0].(type) {
	case *sqlparser.AliasedTableExpr:
		// Simple table reference: FROM table
		fromCollection = sqlparser.String(fromExpr.Expr)

		// Handle additional JOINs
		if len(from) > 1 {
			joinStages, err := c.buildJoinStages(from[1:])
			if err != nil {
				return "", nil, fmt.Errorf("failed to build JOIN stages: %w", err)
			}
			pipeline = append(pipeline, joinStages...)
		}
	case *sqlparser.JoinTableExpr:
		// JOIN syntax: FROM table1 JOIN table2 ON condition
		// Handle nested JOINs by extracting the base table and all JOINs
		baseTable, allJoins, err := c.extractJoinStructure(fromExpr)
		if err != nil {
			return "", nil, fmt.Errorf("failed to extract JOIN structure: %w", err)
		}
		fromCollection = baseTable

		// Process all JOINs
		for _, joinExpr := range allJoins {
			joinStages, err := c.buildSingleJoin(joinExpr)
			if err != nil {
				return "", nil, fmt.Errorf("failed to build JOIN stage: %w", err)
			}
			pipeline = append(pipeline, joinStages...)
		}

		// Handle additional JOINs
		if len(from) > 1 {
			additionalJoins, err := c.buildJoinStages(from[1:])
			if err != nil {
				return "", nil, fmt.Errorf("failed to build additional JOIN stages: %w", err)
			}
			pipeline = append(pipeline, additionalJoins...)
		}
	default:
		return "", nil, fmt.Errorf("unsupported FROM clause format: %T", fromExpr)
	}

	return fromCollection, pipeline, nil
}

// buildPipelineWithCollection constructs the MongoDB aggregation pipeline and returns collection name
func (c *Converter) buildPipelineWithCollection(selectStmt *sqlparser.Select) (string, []map[string]interface{}, error) {
	var pipeline []map[string]interface{}

	// Handle FROM clause
	fromCollection, joinStages, err := c.buildFromClause(selectStmt.From)
	if err != nil {
		return "", nil, err
	}
	pipeline = append(pipeline, joinStages...)

	// Handle WHERE clause using $match
	if selectStmt.Where != nil {
		matchStage, err := c.BuildMatchStage(selectStmt.Where.Expr)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build WHERE clause: %w", err)
		}
		pipeline = append(pipeline, map[string]interface{}{"$match": matchStage})
	}

	// Handle GROUP BY clause
	if len(selectStmt.GroupBy) > 0 {
		groupStage, err := c.BuildGroupStage(selectStmt.GroupBy, selectStmt.SelectExprs)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build GROUP BY clause: %w", err)
		}
		pipeline = append(pipeline, map[string]interface{}{"$group": groupStage})
	}

	// Handle HAVING clause (after GROUP BY)
	if selectStmt.Having != nil {
		if len(selectStmt.GroupBy) == 0 {
			return "", nil, fmt.Errorf("HAVING clause requires GROUP BY")
		}
		havingStage, err := c.BuildMatchStage(selectStmt.Having.Expr)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build HAVING clause: %w", err)
		}
		pipeline = append(pipeline, map[string]interface{}{"$match": havingStage})
	}

	// Handle SELECT columns using $project
	projectStages, err := c.buildProjectStage(selectStmt, fromCollection)
	if err != nil {
		return "", nil, err
	}
	pipeline = append(pipeline, projectStages...)

	// Handle ORDER BY and LIMIT clauses
	sortLimitStages, err := c.buildSortAndLimitStages(selectStmt)
	if err != nil {
		return "", nil, err
	}
	pipeline = append(pipeline, sortLimitStages...)

	return fromCollection, pipeline, nil
}

// needsProjectStage determines if a $project stage is needed based on the query structure
func (c *Converter) needsProjectStage(selectExprs sqlparser.SelectExprs, groupBy sqlparser.GroupBy) bool {
	if len(groupBy) == 0 {
		return true // Non-GROUP BY queries always need $project
	}

	// Check if there are transformation functions in SELECT
	for _, selExpr := range selectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			if funcExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				funcName := strings.ToUpper(funcExpr.Name.String())
				if _, isTransformFunc := TransformationFunctions[funcName]; isTransformFunc {
					return true
				}
			}
		}
	}

	// Check if there are function expressions in GROUP BY
	for _, groupExpr := range groupBy {
		if _, ok := groupExpr.(*sqlparser.FuncExpr); ok {
			return true
		}
	}

	return false // Simple GROUP BY queries don't need $project
}

// buildProjectStage builds the appropriate $project stage based on query structure
func (c *Converter) buildProjectStage(selectStmt *sqlparser.Select, fromCollection string) ([]map[string]interface{}, error) {
	var stages []map[string]interface{}

	if len(selectStmt.GroupBy) > 0 && c.needsProjectStage(selectStmt.SelectExprs, selectStmt.GroupBy) {
		projectStage, err := c.BuildGroupProjectStage(selectStmt.SelectExprs, selectStmt.GroupBy)
		if err != nil {
			return nil, fmt.Errorf("failed to build GROUP BY SELECT clause: %w", err)
		}
		if len(projectStage) > 0 {
			stages = append(stages, map[string]interface{}{"$project": projectStage})
		}
	} else if len(selectStmt.GroupBy) == 0 {
		// For non-GROUP BY queries, use the regular project stage
		projectStage, err := c.BuildProjectStage(selectStmt.SelectExprs, fromCollection)
		if err != nil {
			return nil, fmt.Errorf("failed to build SELECT clause: %w", err)
		}
		if len(projectStage) > 0 {
			stages = append(stages, map[string]interface{}{"$project": projectStage})
		}
	}

	return stages, nil
}

// buildSortAndLimitStages builds $sort and $limit/$skip stages from ORDER BY and LIMIT clauses
func (c *Converter) buildSortAndLimitStages(selectStmt *sqlparser.Select) ([]map[string]interface{}, error) {
	var stages []map[string]interface{}

	// Handle ORDER BY clause using $sort
	if len(selectStmt.OrderBy) > 0 {
		sortStage, err := c.BuildSortStage(selectStmt.OrderBy)
		if err != nil {
			return nil, fmt.Errorf("failed to build ORDER BY clause: %w", err)
		}
		stages = append(stages, map[string]interface{}{"$sort": sortStage})
	}

	// Handle LIMIT clause using $limit
	if selectStmt.Limit != nil {
		limitStage, err := c.buildLimitStage(selectStmt.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to build LIMIT clause: %w", err)
		}
		stages = append(stages, limitStage...)
	}

	return stages, nil
}

// buildSingleJoin constructs $lookup and $unwind stages for a single JOIN
func (c *Converter) buildSingleJoin(joinExpr *sqlparser.JoinTableExpr) ([]map[string]interface{}, error) {
	aliasedTable, ok := joinExpr.RightExpr.(*sqlparser.AliasedTableExpr)
	if !ok {
		return nil, fmt.Errorf("unsupported JOIN table expression")
	}

	joinCollection := sqlparser.String(aliasedTable.Expr)
	joinAlias := joinCollection
	if !aliasedTable.As.IsEmpty() {
		joinAlias = aliasedTable.As.String()
	}

	// Extract join condition (ON clause)
	if joinExpr.Condition.On == nil {
		return nil, fmt.Errorf("JOIN requires an ON clause")
	}

	localField, foreignField, err := c.extractJoinCondition(joinExpr.Condition.On)
	if err != nil {
		return nil, fmt.Errorf("failed to extract JOIN condition: %w", err)
	}

	lookupStage := map[string]interface{}{
		"$lookup": map[string]interface{}{
			"from":         joinCollection,
			"localField":   localField,
			"foreignField": foreignField,
			"as":           joinAlias,
		},
	}

	var stages []map[string]interface{}
	stages = append(stages, lookupStage)

	// Add $unwind for INNER JOINs, $unwind with preserveNullAndEmptyArrays for LEFT JOINs
	unwindStage := map[string]interface{}{
		"$unwind": "$" + joinAlias,
	}

	if strings.ToUpper(joinExpr.Join) == "LEFT JOIN" {
		unwindStage["$unwind"] = map[string]interface{}{
			"path":                       "$" + joinAlias,
			"preserveNullAndEmptyArrays": true,
		}
	}

	stages = append(stages, unwindStage)
	return stages, nil
}

// extractJoinStructure recursively extracts the base table and all JOIN expressions
func (c *Converter) extractJoinStructure(joinExpr *sqlparser.JoinTableExpr) (string, []*sqlparser.JoinTableExpr, error) {
	var allJoins []*sqlparser.JoinTableExpr

	// Add current JOIN to the list
	allJoins = append(allJoins, joinExpr)

	// Find the base table by traversing the left side
	current := joinExpr
	for {
		switch leftExpr := current.LeftExpr.(type) {
		case *sqlparser.AliasedTableExpr:
			// Found the base table
			baseTable := sqlparser.String(leftExpr.Expr)
			return baseTable, allJoins, nil
		case *sqlparser.JoinTableExpr:
			// Another JOIN on the left, add it to the list and continue
			allJoins = append([]*sqlparser.JoinTableExpr{leftExpr}, allJoins...)
			current = leftExpr
		default:
			return "", nil, fmt.Errorf("unsupported left expression in JOIN: %T", leftExpr)
		}
	}
}

// buildJoinStages constructs $lookup and $unwind stages for JOIN clauses
func (c *Converter) buildJoinStages(joins []sqlparser.TableExpr) ([]map[string]interface{}, error) {
	var stages []map[string]interface{}

	for _, join := range joins {
		joinExpr, ok := join.(*sqlparser.JoinTableExpr)
		if !ok {
			return nil, fmt.Errorf("unsupported JOIN format")
		}

		joinStages, err := c.buildSingleJoin(joinExpr)
		if err != nil {
			return nil, err
		}
		stages = append(stages, joinStages...)
	}

	return stages, nil
}

// extractJoinCondition extracts the local and foreign field names from a JOIN ON condition
func (c *Converter) extractJoinCondition(onExpr sqlparser.Expr) (string, string, error) {
	comparison, ok := onExpr.(*sqlparser.ComparisonExpr)
	if !ok || comparison.Operator != "=" {
		return "", "", fmt.Errorf("JOIN ON clause must be an equality comparison")
	}

	left, okLeft := comparison.Left.(*sqlparser.ColName)
	right, okRight := comparison.Right.(*sqlparser.ColName)
	if !okLeft || !okRight {
		return "", "", fmt.Errorf("JOIN condition must compare columns")
	}

	localField := left.Name.String()
	foreignField := right.Name.String()

	return localField, foreignField, nil
}

// buildLimitStage constructs $skip and $limit stages from the LIMIT clause
func (c *Converter) buildLimitStage(limit *sqlparser.Limit) ([]map[string]interface{}, error) {
	var stages []map[string]interface{}

	// Handle OFFSET (LIMIT offset, count)
	if limit.Offset != nil {
		offsetVal, err := c.extractValue(limit.Offset)
		if err != nil {
			return nil, fmt.Errorf("failed to extract OFFSET value: %w", err)
		}
		if offset, ok := offsetVal.(int64); ok {
			stages = append(stages, map[string]interface{}{"$skip": offset})
		} else {
			return nil, fmt.Errorf("OFFSET must be a number")
		}
	}

	// Handle LIMIT count
	if limit.Rowcount != nil {
		limitVal, err := c.extractValue(limit.Rowcount)
		if err != nil {
			return nil, fmt.Errorf("failed to extract LIMIT value: %w", err)
		}
		if count, ok := limitVal.(int64); ok {
			stages = append(stages, map[string]interface{}{"$limit": count})
		} else {
			return nil, fmt.Errorf("LIMIT must be a number")
		}
	}

	return stages, nil
}
