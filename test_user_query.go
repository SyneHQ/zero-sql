package main

import (
	"fmt"
	"encoding/json"

	"github.com/synehq/zero-sql/internal/converter"
)

func main() {
	// User's query without CAST (since CAST isn't supported by sqlparser)
	sqlQuery := `
SELECT
  strftime(created_at, '%Y') as year,
  strftime(created_at, '%m') as month,
  strftime(created_at, '%Y-%m') as year_month,
  COUNT(*) as order_count,
  ROUND(SUM(total), 2) as total_revenue,
  ROUND(AVG(total), 2) as avg_order_value
FROM orders
GROUP BY strftime(created_at, '%Y'), strftime(created_at, '%m'), strftime(created_at, '%Y-%m')
ORDER BY year, month
LIMIT 50
`

	c := converter.New(nil)
	pipeline, err := c.ConvertSQLToMongo(sqlQuery)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Pretty print the pipeline
	jsonBytes, err := json.MarshalIndent(pipeline, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Printf("Generated MongoDB aggregation pipeline:\n%s\n", string(jsonBytes))
}