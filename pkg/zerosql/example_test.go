// Package zerosql provides examples for the ZeroSQL converter.
package zerosql_test

import (
	"fmt"
	"log"

	"github.com/synehq/zero-sql/pkg/zerosql"
)

// Example demonstrates basic SQL to MongoDB conversion.
func Example() {
	converter := zerosql.New(nil)

	// Simple SELECT query
	pipeline, err := converter.ConvertSQLToMongo("SELECT name, age FROM users WHERE age > 18")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Generated %d pipeline stages\n", len(pipeline))
	// Output: Generated 2 pipeline stages
}

// ExampleConverter_ConvertSQLToMongoWithCollection demonstrates conversion with collection info.
func ExampleConverter_ConvertSQLToMongoWithCollection() {
	converter := zerosql.New(nil)

	result, err := converter.ConvertSQLToMongoWithCollection(
		"SELECT status, COUNT(*) as count FROM users GROUP BY status")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Collection: %s\n", result.Collection)
	fmt.Printf("Pipeline stages: %d\n", len(result.Pipeline))
	// Output:
	// Collection: users
	// Pipeline stages: 1
}

// Example shows advanced analytics with functions.
func Example_advancedAnalytics() {
	converter := zerosql.New(nil)

	query := `
		SELECT
			CAST(strftime(created_at, '%Y') AS INTEGER) as year,
			CAST(strftime(created_at, '%m') AS INTEGER) as month,
			COUNT(*) as order_count,
			ROUND(SUM(total), 2) as revenue,
			ROUND(AVG(total), 2) as avg_order
		FROM orders
		WHERE created_at >= '2023-01-01'
		GROUP BY year, month
		ORDER BY year DESC, month DESC
		LIMIT 12
	`

	result, err := converter.ConvertSQLToMongoWithCollection(query)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Advanced analytics query converted to %d pipeline stages\n", len(result.Pipeline))
	// Output: Advanced analytics query converted to 5 pipeline stages
}
