package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/synehq/zero-sql/internal/converter"

	"github.com/spf13/cobra"
)

var (
	outputFormat string
	prettyPrint  bool
	verbose      bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "zero-sql [SQL_QUERY]",
	Short: "Convert SQL queries to MongoDB aggregation pipelines",
	Long: `Zero-SQL is a robust CLI tool that converts SQL queries to MongoDB aggregation pipelines.
	
It supports:
- SELECT statements with column selection and aliases
- FROM clauses with table references
- JOIN operations (INNER, LEFT, RIGHT)
- WHERE clauses with complex conditions (AND, OR, nested conditions)
- Comparison operators (=, !=, >, <, >=, <=, LIKE, ILIKE, IN, NOT IN)
- ORDER BY clauses
- LIMIT and OFFSET
- GROUP BY with aggregation functions (COUNT, SUM, AVG, MIN, MAX)
- HAVING clauses

Examples:
  zero-sql "SELECT name, age FROM users WHERE age > 18"
  zero-sql "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id WHERE u.active = true"
  zero-sql "SELECT u.name, p.title, c.name FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id"
  zero-sql "SELECT u.name, p.title FROM users u LEFT JOIN posts p ON u.id = p.user_id"
  zero-sql --format=json --pretty "SELECT COUNT(*) as total FROM orders GROUP BY status"`,
	Args: cobra.ExactArgs(1),
	RunE: runConvert,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&outputFormat, "format", "f", "json", "Output format (json, bson)")
	rootCmd.Flags().BoolVarP(&prettyPrint, "pretty", "p", true, "Pretty print the output")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
}

// runConvert is the main execution function for the convert command
func runConvert(cmd *cobra.Command, args []string) error {
	sqlQuery := strings.TrimSpace(args[0])

	if sqlQuery == "" {
		return fmt.Errorf("SQL query cannot be empty")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Converting SQL query: %s\n", sqlQuery)
	}

	// Create converter with options
	conv := converter.New(&converter.Options{
		Verbose: verbose,
	})

	// Convert SQL to MongoDB pipeline
	pipeline, err := conv.ConvertSQLToMongo(sqlQuery)
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Output the result
	return outputResult(pipeline)
}

// outputResult formats and outputs the MongoDB pipeline
func outputResult(pipeline []map[string]interface{}) error {
	switch strings.ToLower(outputFormat) {
	case "json":
		return outputJSON(pipeline)
	case "bson":
		return outputBSON(pipeline)
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// outputJSON outputs the pipeline as JSON
func outputJSON(pipeline []map[string]interface{}) error {
	var output []byte
	var err error

	if prettyPrint {
		output, err = json.MarshalIndent(pipeline, "", "  ")
	} else {
		output, err = json.Marshal(pipeline)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// outputBSON outputs the pipeline in BSON-like format (JSON with MongoDB extended syntax)
func outputBSON(pipeline []map[string]interface{}) error {
	// For now, BSON output is the same as JSON
	// In a more robust implementation, this could use the official MongoDB Go driver's BSON package
	return outputJSON(pipeline)
}
