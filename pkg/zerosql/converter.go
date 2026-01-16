// Package zerosql provides SQL to MongoDB aggregation pipeline conversion.
//
// ZeroSQL is a powerful SQL-to-MongoDB converter that translates standard SQL queries
// into MongoDB aggregation pipelines. It supports a comprehensive set of SQL features
// including complex aggregations, date functions, string manipulation, and more.
//
// # Basic Usage
//
//	import "github.com/synehq/zero-sql/pkg/zerosql"
//
//	converter := zerosql.New(nil)
//	pipeline, err := converter.ConvertSQLToMongo("SELECT * FROM users WHERE age > 18")
//	if err != nil {
//	    // handle error
//	}
//
// # Supported SQL Features
//
// ## Queries
//   - SELECT with column aliases
//   - WHERE clauses with complex conditions
//   - ORDER BY with ASC/DESC
//   - LIMIT and OFFSET
//   - DISTINCT queries
//
// ## Aggregations
//   - COUNT(*), SUM, AVG, MIN, MAX
//   - GROUP BY with expressions and aliases
//   - HAVING clauses
//
// ## Joins
//   - INNER JOIN, LEFT JOIN
//   - Multiple table joins
//
// ## Functions
//   - String: UPPER, LOWER, CONCAT, SUBSTR, LENGTH, REPLACE
//   - Math: ABS, CEIL, FLOOR, ROUND, POWER, SQRT, MOD
//   - Date: YEAR, MONTH, DAY, DATEADD, DATEDIFF, STRFTIME
//   - Type: CAST (preprocessed)
//   - Conditional: COALESCE, NULLIF
//
// ## Operators
//   - Comparison: =, !=, <>, <, <=, >, >=
//   - Logical: AND, OR, NOT
//   - Pattern: LIKE, ILIKE with % and _ wildcards
//   - Range: BETWEEN
//   - Sets: IN, NOT IN
//   - Null checks: IS NULL, IS NOT NULL
//
// # Notes
//
// - CAST expressions are preprocessed to remove type casting syntax
// - Complex expressions in GROUP BY are supported through aliases
// - The converter generates optimized MongoDB aggregation pipelines
package zerosql

import (
	"github.com/synehq/zero-sql/internal/converter"
)

// Converter provides SQL to MongoDB aggregation pipeline conversion.
//
// It wraps the internal converter implementation and provides a clean,
// documented public API for converting SQL queries to MongoDB pipelines.
type Converter struct {
	conv *converter.Converter
}

// Options holds configuration options for the converter.
type Options struct {
	// Verbose enables detailed logging during conversion.
	Verbose bool
}

// ConversionResult contains the result of a SQL to MongoDB conversion.
//
// It includes both the target MongoDB collection name and the aggregation
// pipeline stages needed to execute the equivalent query.
type ConversionResult struct {
	// Collection is the MongoDB collection name extracted from the SQL query.
	Collection string `json:"collection"`
	// Pipeline contains the MongoDB aggregation pipeline stages.
	Pipeline []map[string]interface{} `json:"pipeline"`
}

// New creates a new Converter instance with the provided options.
//
// If opts is nil, default options will be used.
func New(opts *Options) *Converter {
	if opts == nil {
		opts = &Options{}
	}

	conv := converter.New(&converter.Options{
		Verbose: opts.Verbose,
	})

	return &Converter{
		conv: conv,
	}
}

// ConvertSQLToMongo converts a SQL SELECT query to MongoDB aggregation pipeline stages.
//
// This method returns only the aggregation pipeline stages. For queries that reference
// specific collections, use ConvertSQLToMongoWithCollection instead.
//
// Parameters:
//   - sqlQuery: A valid SQL SELECT query string
//
// Returns:
//   - []map[string]interface{}: MongoDB aggregation pipeline stages
//   - error: Conversion error if the SQL query is invalid or unsupported
//
// Example:
//
//	pipeline, err := converter.ConvertSQLToMongo("SELECT name, age FROM users WHERE age > 18")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// pipeline contains the MongoDB aggregation stages
func (c *Converter) ConvertSQLToMongo(sqlQuery string) ([]map[string]interface{}, error) {
	return c.conv.ConvertSQLToMongo(sqlQuery)
}

// ConvertSQLToMongoWithCollection converts a SQL SELECT query to MongoDB aggregation pipeline
// and returns both the collection name and pipeline stages.
//
// This is the recommended method for most use cases as it provides complete information
// needed to execute the query against MongoDB.
//
// Parameters:
//   - sqlQuery: A valid SQL SELECT query string
//
// Returns:
//   - *ConversionResult: Contains collection name and pipeline stages
//   - error: Conversion error if the SQL query is invalid or unsupported
//
// Example:
//
//	result, err := converter.ConvertSQLToMongoWithCollection(
//	    "SELECT name, COUNT(*) as count FROM users GROUP BY name")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Collection: %s\n", result.Collection)
//	// Execute pipeline against result.Collection
func (c *Converter) ConvertSQLToMongoWithCollection(sqlQuery string) (*ConversionResult, error) {
	collection, pipeline, err := c.conv.ConvertSQLToMongoWithCollection(sqlQuery)
	if err != nil {
		return nil, err
	}

	return &ConversionResult{
		Collection: collection,
		Pipeline:   pipeline,
	}, nil
}
