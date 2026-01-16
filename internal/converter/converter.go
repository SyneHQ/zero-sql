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

// CTE represents a Common Table Expression
type CTE struct {
	Name  string
	Query string
}

// Converter handles the conversion from SQL to MongoDB aggregation pipelines
type Converter struct {
	options  *Options
	ilikeMap map[string]bool
	ctes     []CTE // Common Table Expressions from WITH clause
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

	// Handle WITH clauses (Common Table Expressions)
	processedQuery, err := c.preprocessWITHClauses(processedQuery)
	if err != nil {
		return "", nil, fmt.Errorf("failed to preprocess WITH clauses: %w", err)
	}

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

	// Handle CAST expressions - remove CAST() wrapper and keep inner expression
	// This handles: CAST(expression AS type) -> expression
	castRegex := regexp.MustCompile(`(?i)\bCAST\s*\(\s*(.+?)\s+AS\s+[^)]+\s*\)`)
	processedQuery = castRegex.ReplaceAllStringFunc(processedQuery, func(match string) string {
		// Extract the expression inside CAST()
		submatch := castRegex.FindStringSubmatch(match)
		if len(submatch) >= 2 {
			expression := submatch[1]
			return strings.TrimSpace(expression)
		}
		return match // Return original if parsing fails
	})

	// Handle PostgreSQL type casting syntax: expr::TYPE -> expr
	postgresCastRegex := regexp.MustCompile(`::\w+`)
	processedQuery = postgresCastRegex.ReplaceAllString(processedQuery, "")

	// UNNEST function calls are left as-is for special handling during CTE processing

	return processedQuery, ilikePositions, nil
}

// preprocessWITHClauses extracts and processes WITH clauses (Common Table Expressions)
func (c *Converter) preprocessWITHClauses(sqlQuery string) (string, error) {
	// Check if query starts with WITH
	withRegex := regexp.MustCompile(`(?i)^\s*WITH\s+`)
	if !withRegex.MatchString(sqlQuery) {
		return sqlQuery, nil // No WITH clause, return as-is
	}

	if c.options.Verbose {
		fmt.Printf("WITH clause detected in query\n")
	}

	// Parse CTEs manually by finding AS keywords and matching parentheses
	c.ctes = c.parseCTEs(sqlQuery)

	if c.options.Verbose {
		fmt.Printf("Found %d CTEs\n", len(c.ctes))
		for _, cte := range c.ctes {
			fmt.Printf("CTE: %s -> %s\n", cte.Name, cte.Query)
		}
	}

	// Find the main SELECT query (the first SELECT not inside CTE parentheses)
	// Simple approach: find the last CTE closing paren, then the next SELECT
	lastCTEEnd := -1
	for _, cte := range c.ctes {
		// Find where this CTE ends in the original query
		cteEndPattern := regexp.MustCompile(regexp.QuoteMeta(cte.Query) + `\s*\)\s*,?\s*`)
		loc := cteEndPattern.FindStringIndex(sqlQuery)
		if loc != nil && loc[1] > lastCTEEnd {
			lastCTEEnd = loc[1]
		}
	}

	// Now find the first SELECT after the last CTE
	mainQueryStart := -1
	if lastCTEEnd > 0 {
		remaining := sqlQuery[lastCTEEnd:]
		selectIndex := strings.Index(strings.ToUpper(remaining), "SELECT")
		if selectIndex >= 0 {
			mainQueryStart = lastCTEEnd + selectIndex
		}
	}

	if mainQueryStart >= 0 {
		result := strings.TrimSpace(sqlQuery[mainQueryStart:])
		if c.options.Verbose {
			fmt.Printf("Main query: %s\n", result)
		}
		return result, nil
	}

	// Fallback: find any SELECT after WITH
	withIndex := strings.Index(strings.ToUpper(sqlQuery), "WITH")
	selectIndex := strings.LastIndex(strings.ToUpper(sqlQuery), "SELECT")
	if selectIndex > withIndex {
		result := strings.TrimSpace(sqlQuery[selectIndex:])
		if c.options.Verbose {
			fmt.Printf("Main query (fallback): %s\n", result)
		}
		return result, nil
	}

	return "", fmt.Errorf("could not extract main query from WITH clause")
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

// buildCTEPipeline builds the pipeline for a Common Table Expression
func (c *Converter) buildCTEPipeline(cte CTE) ([]map[string]interface{}, error) {
	// Check for UNNEST usage in the CTE query
	if c.hasUnnestFunction(cte.Query) {
		if c.options.Verbose {
			fmt.Printf("CTE %s contains UNNEST, using special handling\n", cte.Name)
		}
		return c.buildCTEPipelineWithUnnest(cte)
	}

	if c.options.Verbose {
		fmt.Printf("CTE %s does not contain UNNEST, using regular pipeline\n", cte.Name)
	}

	// Parse the CTE query
	cteStmt, err := sqlparser.Parse(cte.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CTE %s: %w", cte.Name, err)
	}

	cteSelectStmt, ok := cteStmt.(*sqlparser.Select)
	if !ok {
		return nil, fmt.Errorf("CTE %s must be a SELECT statement, got: %T", cte.Name, cteStmt)
	}

	// Build pipeline for the CTE
	pipeline, err := c.buildPipeline(cteSelectStmt)
	if err != nil {
		return nil, fmt.Errorf("failed to build pipeline for CTE %s: %w", cte.Name, err)
	}

	return pipeline, nil
}

// buildCTEPipelineWithUnnest builds a pipeline for CTEs that use UNNEST
func (c *Converter) buildCTEPipelineWithUnnest(cte CTE) ([]map[string]interface{}, error) {
	var pipeline []map[string]interface{}

	if c.options.Verbose {
		fmt.Printf("Building CTE pipeline with UNNEST for %s\n", cte.Name)
	}

	// Parse the CTE query to understand the structure
	cteStmt, err := sqlparser.Parse(cte.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CTE %s: %w", cte.Name, err)
	}

	cteSelectStmt, ok := cteStmt.(*sqlparser.Select)
	if !ok {
		return nil, fmt.Errorf("CTE %s must be a SELECT statement, got: %T", cte.Name, cteStmt)
	}

	// Get the base collection and initial pipeline stages
	fromCollection, fromPipeline, err := c.buildFromClause(cteSelectStmt.From)
	if err != nil {
		return nil, fmt.Errorf("failed to build FROM clause for CTE %s: %w", cte.Name, err)
	}

	if c.options.Verbose {
		fmt.Printf("CTE %s from collection: %s, initial stages: %d\n", cte.Name, fromCollection, len(fromPipeline))
	}

	pipeline = append(pipeline, fromPipeline...)

	// Find UNNEST expressions and add $unwind stages
	foundUnnest := false
	if c.options.Verbose {
		fmt.Printf("CTE %s has %d select expressions\n", cte.Name, len(cteSelectStmt.SelectExprs))
	}
	for i, selectExpr := range cteSelectStmt.SelectExprs {
		if c.options.Verbose {
			fmt.Printf("Select expr %d: %T - %+v\n", i, selectExpr, selectExpr)
		}
		if aliasedExpr, ok := selectExpr.(*sqlparser.AliasedExpr); ok {
			if c.options.Verbose {
				fmt.Printf("  Aliased expr: %+v\n", aliasedExpr)
				fmt.Printf("  Expr type: %T\n", aliasedExpr.Expr)
			}
			if funcExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				if c.options.Verbose {
					fmt.Printf("  Function: %s\n", funcExpr.Name.String())
				}
				if strings.ToUpper(funcExpr.Name.String()) == "UNNEST" {
					foundUnnest = true
					if c.options.Verbose {
						fmt.Printf("Found UNNEST function with %d expressions\n", len(funcExpr.Exprs))
					}
					if len(funcExpr.Exprs) == 1 {
						// Extract the array field from UNNEST argument
						if c.options.Verbose {
							fmt.Printf("UNNEST exprs[0] type: %T, value: %+v\n", funcExpr.Exprs[0], funcExpr.Exprs[0])
						}
						if aliasedExpr, ok := funcExpr.Exprs[0].(*sqlparser.AliasedExpr); ok {
							// The UNNEST argument is an AliasedExpr, get its inner expression
							expr := aliasedExpr.Expr
							if c.options.Verbose {
								fmt.Printf("UNNEST inner argument: %T - %+v\n", expr, expr)
							}
							arrayField, err := c.extractValue(expr)
							if err != nil {
								return nil, fmt.Errorf("failed to extract array field from UNNEST: %w", err)
							}
							if c.options.Verbose {
								fmt.Printf("Extracted array field: %T - %+v\n", arrayField, arrayField)
							}
							if arrayFieldStr, ok := arrayField.(string); ok {
								if c.options.Verbose {
									fmt.Printf("Adding $unwind for field: %s\n", arrayFieldStr)
								}
								unwindStage := map[string]interface{}{
									"$unwind": map[string]interface{}{
										"path":                       arrayFieldStr,
										"preserveNullAndEmptyArrays": false,
									},
								}
								pipeline = append(pipeline, unwindStage)

								// Create project stage for the unwound data
								alias := aliasedExpr.As.String()
								if alias == "" {
									alias = "item" // default alias for UNNEST
								}

								projectFields := map[string]interface{}{
									alias: "$" + strings.TrimPrefix(arrayFieldStr, "$"),
								}

								// Add other select expressions (non-UNNEST)
								for _, otherSelectExpr := range cteSelectStmt.SelectExprs {
									if otherAliasedExpr, ok := otherSelectExpr.(*sqlparser.AliasedExpr); ok {
										if otherFuncExpr, ok := otherAliasedExpr.Expr.(*sqlparser.FuncExpr); !ok || strings.ToUpper(otherFuncExpr.Name.String()) != "UNNEST" {
											otherAlias := otherAliasedExpr.As.String()
											if otherAlias == "" {
												if otherVal, err := c.extractValue(otherAliasedExpr.Expr); err == nil {
													if otherStr, ok := otherVal.(string); ok {
														otherAlias = strings.TrimPrefix(otherStr, "$")
													}
												}
											}
											if otherAlias != "" {
												projectFields[otherAlias] = "$" + otherAlias
											}
										}
									}
								}

								projectStage := map[string]interface{}{
									"$project": projectFields,
								}
								pipeline = append(pipeline, projectStage)

								if c.options.Verbose {
									fmt.Printf("Added $project stage with fields: %+v\n", projectFields)
								}
							}
						}
					}
				}
			}
		}
	}

	if !foundUnnest {
		if c.options.Verbose {
			fmt.Printf("No UNNEST found in CTE %s, this shouldn't happen\n", cte.Name)
		}
	}

	// Handle WHERE clause
	if cteSelectStmt.Where != nil {
		whereStage, err := c.BuildMatchStage(cteSelectStmt.Where.Expr)
		if err != nil {
			return nil, fmt.Errorf("failed to build WHERE clause for CTE %s: %w", cte.Name, err)
		}
		if whereStage != nil {
			pipeline = append(pipeline, whereStage)
		}
	}

	// Handle LIMIT
	if cteSelectStmt.Limit != nil {
		limitStages, err := c.buildLimitStage(cteSelectStmt.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to build LIMIT clause for CTE %s: %w", cte.Name, err)
		}
		pipeline = append(pipeline, limitStages...)
	}

	if c.options.Verbose {
		fmt.Printf("CTE %s pipeline completed with %d stages\n", cte.Name, len(pipeline))
	}

	return pipeline, nil
}

// hasUnnestFunction checks if a query contains UNNEST functions
func (c *Converter) hasUnnestFunction(query string) bool {
	return strings.Contains(strings.ToUpper(query), "UNNEST(")
}

// parseCTEs manually parses Common Table Expressions from the WITH clause
func (c *Converter) parseCTEs(sqlQuery string) []CTE {
	var ctes []CTE

	// Convert to uppercase for case-insensitive matching
	upperQuery := strings.ToUpper(sqlQuery)

	// Find all "AS (" positions
	asIndex := strings.Index(upperQuery, " AS (")
	for asIndex >= 0 {
		// Find the CTE name (word before "AS")
		beforeAS := sqlQuery[:asIndex]
		words := strings.Fields(beforeAS)
		if len(words) == 0 {
			break
		}
		cteName := words[len(words)-1]

		// Skip "WITH" if it's there
		if strings.ToUpper(cteName) == "WITH" {
			asIndex = strings.Index(upperQuery[asIndex+4:], " AS (")
			if asIndex >= 0 {
				asIndex += 4 // adjust for the slice
			}
			continue
		}

		// Find the matching closing parenthesis
		openParenIndex := asIndex + 4 // position of "(" after "AS "
		if openParenIndex >= len(sqlQuery) || sqlQuery[openParenIndex] != '(' {
			break
		}

		closeParenIndex := c.findMatchingParen(sqlQuery, openParenIndex)
		if closeParenIndex == -1 {
			break
		}

		// Extract the CTE query
		cteQuery := sqlQuery[openParenIndex+1 : closeParenIndex]
		cteQuery = strings.TrimSpace(cteQuery)

		ctes = append(ctes, CTE{
			Name:  cteName,
			Query: cteQuery,
		})

		// Continue searching for more CTEs
		remaining := sqlQuery[closeParenIndex+1:]
		nextASIndex := strings.Index(strings.ToUpper(remaining), " AS (")
		if nextASIndex >= 0 {
			asIndex = closeParenIndex + 1 + nextASIndex
		} else {
			break
		}
	}

	return ctes
}

// findMatchingParen finds the index of the matching closing parenthesis
func (c *Converter) findMatchingParen(s string, openIndex int) int {
	if openIndex >= len(s) || s[openIndex] != '(' {
		return -1
	}

	depth := 1
	for i := openIndex + 1; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findCTE finds a CTE by name
func (c *Converter) findCTE(name string) *CTE {
	for i := range c.ctes {
		if c.ctes[i].Name == name {
			return &c.ctes[i]
		}
	}
	return nil
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
		tableName := sqlparser.String(fromExpr.Expr)

		// Check if this is a CTE reference
		if cte := c.findCTE(tableName); cte != nil {
			// This is a CTE reference - build the CTE pipeline and use it as a subquery
			ctePipeline, err := c.buildCTEPipeline(*cte)
			if err != nil {
				return "", nil, fmt.Errorf("failed to build CTE pipeline for %s: %w", tableName, err)
			}
			pipeline = append(pipeline, ctePipeline...)
			fromCollection = "" // CTE doesn't have a collection name
		} else {
			fromCollection = tableName
		}

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

	// Handle DISTINCT clause
	if strings.ToUpper(strings.TrimSpace(selectStmt.Distinct)) == "DISTINCT" {
		distinctStage, err := c.buildDistinctStage(selectStmt.SelectExprs)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build DISTINCT clause: %w", err)
		}
		pipeline = append(pipeline, distinctStage...)
	} else {
		// Handle GROUP BY clause (only if not DISTINCT)
		if len(selectStmt.GroupBy) > 0 {
			groupStage, err := c.BuildGroupStage(selectStmt.GroupBy, selectStmt.SelectExprs)
			if err != nil {
				return "", nil, fmt.Errorf("failed to build GROUP BY clause: %w", err)
			}
			pipeline = append(pipeline, map[string]interface{}{"$group": groupStage})
		}
	}

	// Handle HAVING clause (after GROUP BY or DISTINCT)
	if selectStmt.Having != nil {
		if len(selectStmt.GroupBy) == 0 && strings.ToUpper(strings.TrimSpace(selectStmt.Distinct)) != "DISTINCT" {
			return "", nil, fmt.Errorf("HAVING clause requires GROUP BY or DISTINCT")
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

	// Skip $project stage for DISTINCT queries (they create their own)
	if strings.ToUpper(strings.TrimSpace(selectStmt.Distinct)) == "DISTINCT" {
		return stages, nil
	}

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

// buildDistinctStage builds $group and $project stages for DISTINCT queries
func (c *Converter) buildDistinctStage(selectExprs sqlparser.SelectExprs) ([]map[string]interface{}, error) {
	var stages []map[string]interface{}

	// Create $group stage that groups by all selected fields
	group := map[string]interface{}{
		"_id": make(map[string]interface{}),
	}

	idGroup := group["_id"].(map[string]interface{})

	for i, selExpr := range selectExprs {
		fieldName := fmt.Sprintf("field_%d", i)

		switch expr := selExpr.(type) {
		case *sqlparser.StarExpr:
			// SELECT DISTINCT * - not supported for DISTINCT
			return nil, fmt.Errorf("SELECT DISTINCT * is not supported")
		case *sqlparser.AliasedExpr:
			// For aliased expressions, we need to handle them properly
			switch e := expr.Expr.(type) {
			case *sqlparser.ColName:
				colName := c.getFullColumnName(e)
				idGroup[fieldName] = "$" + colName
			default:
				return nil, fmt.Errorf("DISTINCT with complex expressions not yet supported: %T", e)
			}
		default:
			return nil, fmt.Errorf("unsupported select expression in DISTINCT: %T", expr)
		}
	}

	stages = append(stages, map[string]interface{}{"$group": group})

	// Create $project stage to extract the distinct fields
	project := make(map[string]interface{})

	for i, selExpr := range selectExprs {
		fieldName := fmt.Sprintf("field_%d", i)

		switch expr := selExpr.(type) {
		case *sqlparser.AliasedExpr:
			outputFieldName := ""
			if !expr.As.IsEmpty() {
				outputFieldName = expr.As.String()
			} else if colName, ok := expr.Expr.(*sqlparser.ColName); ok {
				outputFieldName = colName.Name.String()
			}

			if outputFieldName != "" {
				project[outputFieldName] = "$_id." + fieldName
			}
		}
	}

	project["_id"] = 0
	stages = append(stages, map[string]interface{}{"$project": project})

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
