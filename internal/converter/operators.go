package converter

import (
	"fmt"
	"strings"
)

// SQLToMongoOperator maps SQL comparison operators to MongoDB's query operators
var SQLToMongoOperator = map[string]string{
	"=":      "$eq",
	"!=":     "$ne",
	"<>":     "$ne",
	">":      "$gt",
	"<":      "$lt",
	">=":     "$gte",
	"<=":     "$lte",
	"LIKE":   "$regex",
	"ILIKE":  "$regex",
	"IN":     "$in",
	"NOT IN": "$nin",
}

// AggregationFunctions maps SQL aggregation functions to MongoDB equivalents
var AggregationFunctions = map[string]string{
	"COUNT": "$sum",
	"SUM":   "$sum",
	"AVG":   "$avg",
	"MIN":   "$min",
	"MAX":   "$max",
}

// TransformationFunctions maps SQL transformation functions to MongoDB equivalents
var TransformationFunctions = map[string]string{
	"STRFTIME": "$dateToString",
	"ROUND":    "$round",
	"UNNEST":   "$unwind", // Array expansion function
	// String functions
	"UPPER":     "$toUpper",
	"LOWER":     "$toLower",
	"CONCAT":    "$concat",
	"SUBSTR":    "$substr",
	"SUBSTRING": "$substr",
	"LENGTH":    "$strLenBytes",
	"LEN":       "$strLenBytes",
	"REPLACE":   "$replaceAll",
	// Math functions
	"ABS":   "$abs",
	"CEIL":  "$ceil",
	"FLOOR": "$floor",
	"POWER": "$pow",
	"POW":   "$pow",
	"SQRT":  "$sqrt",
	"MOD":   "$mod",
	// Date functions
	"YEAR":     "$year",
	"MONTH":    "$month",
	"DAY":      "$dayOfMonth",
	"DATEADD":  "$dateAdd",
	"DATEDIFF": "$dateDiff",
	// Conditional functions
	"COALESCE": "$ifNull",
	"NULLIF":   "$cond",
}

// ConvertLikePattern converts SQL LIKE pattern to MongoDB regex pattern
func ConvertLikePattern(pattern string, caseInsensitive bool) map[string]interface{} {
	// Convert SQL wildcards to regex
	regexPattern := strings.ReplaceAll(pattern, "%", ".*")
	regexPattern = strings.ReplaceAll(regexPattern, "_", ".")

	// Escape special regex characters except for our converted wildcards
	regexPattern = escapeRegexSpecialChars(regexPattern)

	result := map[string]interface{}{
		"$regex": regexPattern,
	}

	if caseInsensitive {
		result["$options"] = "i"
	}

	return result
}

// escapeRegexSpecialChars escapes special regex characters while preserving .* and .
func escapeRegexSpecialChars(pattern string) string {
	// This is a simplified escape - in production, you'd want more comprehensive escaping
	specialChars := []string{"^", "$", "(", ")", "[", "]", "{", "}", "|", "+", "?", "\\"}

	for _, char := range specialChars {
		pattern = strings.ReplaceAll(pattern, char, "\\"+char)
	}

	return pattern
}

// ConvertSQLOperator converts a SQL operator to its MongoDB equivalent
func ConvertSQLOperator(sqlOp string) (string, error) {
	upperOp := strings.ToUpper(sqlOp)

	if mongoOp, exists := SQLToMongoOperator[upperOp]; exists {
		return mongoOp, nil
	}

	return "", fmt.Errorf("unsupported SQL operator: %s", sqlOp)
}

// ConvertAggregationFunction converts a SQL aggregation function to MongoDB equivalent
func ConvertAggregationFunction(sqlFunc string) (string, error) {
	upperFunc := strings.ToUpper(sqlFunc)

	if mongoFunc, exists := AggregationFunctions[upperFunc]; exists {
		return mongoFunc, nil
	}

	return "", fmt.Errorf("unsupported aggregation function: %s", sqlFunc)
}

// ConvertTransformationFunction converts a SQL transformation function to MongoDB equivalent
func ConvertTransformationFunction(sqlFunc string) (string, error) {
	upperFunc := strings.ToUpper(sqlFunc)

	if mongoFunc, exists := TransformationFunctions[upperFunc]; exists {
		return mongoFunc, nil
	}

	return "", fmt.Errorf("unsupported transformation function: %s", sqlFunc)
}
