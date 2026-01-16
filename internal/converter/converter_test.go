package converter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverter_ConvertSQLToMongo(t *testing.T) {
	converter := New(&Options{Verbose: false})

	tests := []struct {
		name     string
		sql      string
		expected []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "Simple SELECT with WHERE",
			sql:  "SELECT name, age FROM users WHERE age > 18",
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"age": map[string]interface{}{"$gt": int64(18)},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
						"age":  "$age",
					},
				},
			},
		},
		{
			name:     "SELECT * FROM table",
			sql:      "SELECT * FROM users",
			expected: nil,
		},
		{
			name: "SELECT with LIKE operator",
			sql:  "SELECT name FROM users WHERE email LIKE '%@gmail.com'",
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"email": map[string]interface{}{
							"$regex": ".*@gmail.com",
						},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
					},
				},
			},
		},
		{
			name: "SELECT with ILIKE operator",
			sql:  "SELECT name FROM users WHERE email ILIKE '%@GMAIL.COM'",
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"email": map[string]interface{}{
							"$regex":   ".*@GMAIL.COM",
							"$options": "i",
						},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
					},
				},
			},
		},
		{
			name: "SELECT with IN operator",
			sql:  "SELECT name FROM users WHERE status IN ('active', 'pending')",
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"status": map[string]interface{}{
							"$in": []interface{}{"active", "pending"},
						},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
					},
				},
			},
		},
		{
			name: "SELECT with AND/OR conditions",
			sql:  "SELECT name FROM users WHERE (age > 18 AND status = 'active') OR name LIKE 'John%'",
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"$or": []interface{}{
							map[string]interface{}{
								"$and": []interface{}{
									map[string]interface{}{
										"age": map[string]interface{}{"$gt": int64(18)},
									},
									map[string]interface{}{
										"status": "active",
									},
								},
							},
							map[string]interface{}{
								"name": map[string]interface{}{
									"$regex": "John.*",
								},
							},
						},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
					},
				},
			},
		},
		{
			name: "SELECT with ORDER BY and LIMIT",
			sql:  "SELECT name, age FROM users ORDER BY age DESC LIMIT 10",
			expected: []map[string]interface{}{
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"name": "$name",
						"age":  "$age",
					},
				},
				{
					"$sort": map[string]interface{}{
						"age": -1,
					},
				},
				{
					"$limit": int64(10),
				},
			},
		},
		{
			name: "SELECT with GROUP BY and COUNT",
			sql:  "SELECT status, COUNT(*) as total FROM users GROUP BY status",
			expected: []map[string]interface{}{
				{
					"$group": map[string]interface{}{
						"_id": map[string]interface{}{
							"status": "$status",
						},
						"total": map[string]interface{}{
							"$sum": 1,
						},
					},
				},
			},
		},
		{
			name: "SELECT with STRFTIME function",
			sql:  "SELECT strftime(created_at, '%Y-%m-%d') as date FROM orders",
			expected: []map[string]interface{}{
				{
					"$project": map[string]interface{}{
						"_id":  0,
						"date": map[string]interface{}{
							"$dateToString": map[string]interface{}{
								"date":   "$created_at",
								"format": "%Y-%m-%d",
							},
						},
					},
				},
			},
		},
		{
			name: "SELECT with ROUND function",
			sql:  "SELECT ROUND(price, 2) as rounded_price FROM products",
			expected: []map[string]interface{}{
				{
					"$project": map[string]interface{}{
						"_id":           0,
						"rounded_price": map[string]interface{}{
							"$round": []interface{}{"$price", 2},
						},
					},
				},
			},
		},
		{
			name:    "Invalid SQL",
			sql:     "INVALID SQL QUERY",
			wantErr: true,
		},
		{
			name:    "Empty SQL",
			sql:     "",
			wantErr: true,
		},
		{
			name:    "Non-SELECT statement",
			sql:     "INSERT INTO users (name) VALUES ('John')",
			wantErr: true,
		},
		{
			name: "Simple INNER JOIN",
			sql:  "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id",
			expected: []map[string]interface{}{
				{
					"$lookup": map[string]interface{}{
						"from":         "posts",
						"localField":   "id",
						"foreignField": "user_id",
						"as":           "p",
					},
				},
				{
					"$unwind": "$p",
				},
				{
					"$project": map[string]interface{}{
						"_id":   0,
						"name":  "$u.name",
						"title": "$p.title",
					},
				},
			},
		},
		{
			name: "LEFT JOIN",
			sql:  "SELECT u.name, p.title FROM users u LEFT JOIN posts p ON u.id = p.user_id",
			expected: []map[string]interface{}{
				{
					"$lookup": map[string]interface{}{
						"from":         "posts",
						"localField":   "id",
						"foreignField": "user_id",
						"as":           "p",
					},
				},
				{
					"$unwind": map[string]interface{}{
						"path":                       "$p",
						"preserveNullAndEmptyArrays": true,
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":   0,
						"name":  "$u.name",
						"title": "$p.title",
					},
				},
			},
		},
		{
			name: "JOIN with WHERE conditions",
			sql:  "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id WHERE u.active = true",
			expected: []map[string]interface{}{
				{
					"$lookup": map[string]interface{}{
						"from":         "posts",
						"localField":   "id",
						"foreignField": "user_id",
						"as":           "p",
					},
				},
				{
					"$unwind": "$p",
				},
				{
					"$match": map[string]interface{}{
						"u.active": true,
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":   0,
						"name":  "$u.name",
						"title": "$p.title",
					},
				},
			},
		},
		{
			name: "SELECT with quoted column names",
			sql:  `SELECT "Name", "Email" FROM users WHERE "Age" > 18 AND "Status" IN ('active', 'pending')`,
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"$and": []interface{}{
							map[string]interface{}{
								"Age": map[string]interface{}{"$gt": int64(18)},
							},
							map[string]interface{}{
								"Status": map[string]interface{}{
									"$in": []interface{}{"active", "pending"},
								},
							},
						},
					},
				},
				{
					"$project": map[string]interface{}{
						"_id":   0,
						"Name":  "$Name",
						"Email": "$Email",
					},
				},
			},
		},
		{
			name: "Complex aggregation with quoted columns",
			sql:  `SELECT "Date", SUM("Amount") AS Total, COUNT(*) AS Count FROM transactions WHERE "Date" BETWEEN '2023-01-01' AND '2023-12-31' GROUP BY "Date" ORDER BY Total DESC LIMIT 10`,
			expected: []map[string]interface{}{
				{
					"$match": map[string]interface{}{
						"Date": map[string]interface{}{
							"$gte": "2023-01-01",
							"$lte": "2023-12-31",
						},
					},
				},
				{
					"$group": map[string]interface{}{
						"Count": map[string]interface{}{
							"$sum": 1,
						},
						"Total": map[string]interface{}{
							"$sum": "$Amount",
						},
						"_id": map[string]interface{}{
							"Date": "$Date",
						},
					},
				},
				{
					"$sort": map[string]interface{}{
						"Total": -1,
					},
				},
				{
					"$limit": int64(10),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.ConvertSQLToMongo(tt.sql)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_BuildMatchStage(t *testing.T) {
	converter := New(nil)

	tests := []struct {
		name        string
		sql         string
		expectedKey string
		expected    map[string]interface{}
	}{
		{
			name:        "Simple equality",
			sql:         "SELECT * FROM users WHERE name = 'John'",
			expectedKey: "$match",
			expected: map[string]interface{}{
				"name": "John",
			},
		},
		{
			name:        "Greater than comparison",
			sql:         "SELECT * FROM users WHERE age > 25",
			expectedKey: "$match",
			expected: map[string]interface{}{
				"age": map[string]interface{}{"$gt": int64(25)},
			},
		},
		{
			name:        "IS NULL check",
			sql:         "SELECT * FROM users WHERE deleted_at IS NULL",
			expectedKey: "$match",
			expected: map[string]interface{}{
				"deleted_at": nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := converter.ConvertSQLToMongo(tt.sql)
			require.NoError(t, err)

			var matchStage map[string]interface{}
			for _, stage := range pipeline {
				if match, exists := stage[tt.expectedKey]; exists {
					matchStage = match.(map[string]interface{})
					break
				}
			}

			assert.Equal(t, tt.expected, matchStage)
		})
	}
}

func TestConvertLikePattern(t *testing.T) {
	tests := []struct {
		name            string
		pattern         string
		caseInsensitive bool
		expected        map[string]interface{}
	}{
		{
			name:            "Simple wildcard",
			pattern:         "John%",
			caseInsensitive: false,
			expected: map[string]interface{}{
				"$regex": "John.*",
			},
		},
		{
			name:            "Case insensitive wildcard",
			pattern:         "john%",
			caseInsensitive: true,
			expected: map[string]interface{}{
				"$regex":   "john.*",
				"$options": "i",
			},
		},
		{
			name:            "Single character wildcard",
			pattern:         "J_hn",
			caseInsensitive: false,
			expected: map[string]interface{}{
				"$regex": "J.hn",
			},
		},
		{
			name:            "Multiple wildcards",
			pattern:         "%@%.com",
			caseInsensitive: false,
			expected: map[string]interface{}{
				"$regex": ".*@.*.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertLikePattern(tt.pattern, tt.caseInsensitive)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertSQLOperator(t *testing.T) {
	tests := []struct {
		sqlOp    string
		expected string
		wantErr  bool
	}{
		{"=", "$eq", false},
		{"!=", "$ne", false},
		{"<>", "$ne", false},
		{">", "$gt", false},
		{"<", "$lt", false},
		{">=", "$gte", false},
		{"<=", "$lte", false},
		{"LIKE", "$regex", false},
		{"ILIKE", "$regex", false},
		{"IN", "$in", false},
		{"NOT IN", "$nin", false},
		{"UNKNOWN", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.sqlOp, func(t *testing.T) {
			result, err := ConvertSQLOperator(tt.sqlOp)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIntegrationExamples(t *testing.T) {
	converter := New(&Options{Verbose: false})

	// Test cases that demonstrate real-world usage
	integrationTests := []struct {
		name        string
		sql         string
		description string
	}{
		{
			name:        "E-commerce product search",
			sql:         "SELECT name, price, category FROM products WHERE price BETWEEN 10 AND 100 AND category IN ('electronics', 'books') ORDER BY price ASC LIMIT 20",
			description: "Search for affordable products in specific categories",
		},
		{
			name:        "User analytics query",
			sql:         "SELECT status, COUNT(*) as user_count FROM users WHERE created_at > '2023-01-01' GROUP BY status",
			description: "Analyze user demographics by status",
		},
		{
			name:        "Content management",
			sql:         "SELECT title, author FROM articles WHERE published = true AND (title LIKE '%mongodb%' OR content LIKE '%database%') ORDER BY created_at DESC",
			description: "Find published articles about databases",
		},
		{
			name:        "Multiple JOINs",
			sql:         "SELECT u.name, p.title, c.name as category FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id",
			description: "Join users with posts and categories",
		},
		{
			name:        "LEFT JOIN with conditions",
			sql:         "SELECT u.name, p.title FROM users u LEFT JOIN posts p ON u.id = p.user_id WHERE u.active = true",
			description: "Users with their posts, including users without posts",
		},
	}

	for _, tt := range integrationTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			pipeline, err := converter.ConvertSQLToMongo(tt.sql)
			require.NoError(t, err, "SQL conversion should succeed")

			// Verify we get a valid pipeline
			assert.NotEmpty(t, pipeline, "Pipeline should not be empty")

			// Verify the pipeline can be marshaled to JSON (validates structure)
			jsonBytes, err := json.Marshal(pipeline)
			require.NoError(t, err, "Pipeline should be valid JSON")

			t.Logf("Generated pipeline: %s", string(jsonBytes))
		})
	}
}
