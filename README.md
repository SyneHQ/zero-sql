# Zero-SQL

A comprehensive SQL-to-MongoDB converter that transforms complex SQL queries into MongoDB aggregation pipelines. Supports 25+ SQL functions, advanced analytics, and full query capabilities.

> Download cli from here <a href='https://github.com/SyneHQ/zero-sql/releases'>Click here</a>

![ZERO-BANNER](https://c72gdackzgkn7zoa.public.blob.vercel-storage.com/zerosql.png)

**🚀 New: 25+ SQL Functions Supported** - String manipulation, math operations, date functions, and conditional expressions!

## Features

Zero-SQL supports a comprehensive set of SQL features:

### Core SQL Features
- **SELECT statements** with column selection, aliases, and DISTINCT
- **Quoted identifiers** (column names with spaces or special characters)
- **FROM clauses** with table references
- **JOIN operations** (INNER, LEFT, RIGHT)
- **WHERE clauses** with complex conditions (AND, OR, nested conditions)
- **Comparison operators** (=, !=, >, <, >=, <=, LIKE, ILIKE, IN, NOT IN)
- **NULL checks** (IS NULL, IS NOT NULL)
- **BETWEEN clauses** with date ranges and numeric ranges
- **ORDER BY clauses** with ASC/DESC
- **LIMIT and OFFSET**
- **GROUP BY** with aggregation functions (COUNT, SUM, AVG, MIN, MAX, STDDEV, VARIANCE)
- **HAVING clauses**
- **Case-insensitive pattern matching** with ILIKE

### String Functions
- **UPPER(text)** - Convert text to uppercase
- **LOWER(text)** - Convert text to lowercase
- **CONCAT(str1, str2, ...)** - Concatenate strings
- **SUBSTRING(text, start, length)** / **SUBSTR(text, start, length)** - Extract substring
- **LENGTH(text)** / **LEN(text)** - Get string length
- **REPLACE(text, find, replace)** - Replace substrings

### Mathematical Functions
- **ABS(number)** - Absolute value
- **CEIL(number)** - Ceiling (round up)
- **FLOOR(number)** - Floor (round down)
- **POWER(base, exponent)** / **POW(base, exponent)** - Power function
- **SQRT(number)** - Square root
- **MOD(dividend, divisor)** - Modulo operation

### Date/Time Functions
- **YEAR(date)** - Extract year from date
- **MONTH(date)** - Extract month from date
- **DAY(date)** - Extract day of month from date
- **DATEADD(date, interval, unit)** - Add time interval to date
- **DATEDIFF(end_date, start_date, unit)** - Calculate date difference
- **STRFTIME(date, format)** - Format date using strftime patterns

### Conditional Functions
- **COALESCE(val1, val2, ...)** - Return first non-null value
- **NULLIF(expr1, expr2)** - Return null if expressions are equal
- **ROUND(value, decimals)** - Round numeric values

## Installation

### From Source

```bash
git clone https://github.com/synhq/zero-sql
cd zero-sql
go build -o zero-sql build
```

### Using Go Install

```bash
go install github.com/synhq/zero-sql
```

## Usage

### Basic Usage

```bash
zero-sql "SELECT name, age FROM users WHERE age > 18"
```

### Command Line Options

```bash
Usage:
  zero-sql [SQL_QUERY] [flags]

Flags:
  -f, --format string        Output format (json, bson) (default "json")
  -h, --help                 help for zero-sql
  -c, --include-collection   Include collection information in the output
  -p, --pretty               Pretty print the output (default true)
  -v, --verbose              Verbose output
```

### Examples

#### Simple SELECT with WHERE

```bash
zero-sql "SELECT name, email FROM users WHERE active = true"
```

Output:
```json
[
  {
    "$match": {
      "active": true
    }
  },
  {
    "$project": {
      "_id": 0,
      "name": "$name",
      "email": "$email"
    }
  }
]
```

#### Quoted Column Names

Zero-SQL fully supports quoted identifiers for column names with spaces or special characters:

```bash
zero-sql 'SELECT "User Name", "Email Address" FROM users WHERE "Created Date" > '\''2023-01-01'\'''
```

Output:
```json
[
  {
    "$match": {
      "Created Date": {
        "$gt": "2023-01-01"
      }
    }
  },
  {
    "$project": {
      "_id": 0,
      "User Name": "$User Name",
      "Email Address": "$Email Address"
    }
  }
]
```

#### Complex E-commerce Analytics Example

```bash
zero-sql 'SELECT YEAR("Order Date") as order_year, MONTH("Order Date") as order_month, UPPER("Category") as category_upper, COUNT(*) as total_orders, ROUND(SUM("Total Amount"), 2) as revenue, ROUND(AVG("Total Amount"), 2) as avg_order_value, CONCAT('\''$'\'', ROUND(SUM("Total Amount"), 0)) as formatted_revenue FROM orders WHERE "Order Date" BETWEEN '\''2023-01-01'\'' AND '\''2023-12-31'\'' AND "Status" IN ('\''completed'\'', '\''shipped'\'') GROUP BY YEAR("Order Date"), MONTH("Order Date"), "Category" ORDER BY revenue DESC LIMIT 20;'
```

Output:
```json
[
  {
    "$match": {
      "$and": [
        {
          "Order Date": {
            "$gte": "2023-01-01",
            "$lte": "2023-12-31"
          }
        },
        {
          "Status": {
            "$in": ["completed", "shipped"]
          }
        }
      ]
    }
  },
  {
    "$group": {
      "_id": {
        "category": "$Category",
        "group_0": {"$year": "$Order Date"},
        "group_1": {"$month": "$Order Date"}
      },
      "avg_order_value": {"$avg": "$Total Amount"},
      "revenue": {"$sum": "$Total Amount"},
      "total_orders": {"$sum": 1}
    }
  },
  {
    "$project": {
      "_id": 0,
      "avg_order_value": {"$round": ["$avg_order_value", 2]},
      "category_upper": {"$toUpper": "$_id.category"},
      "formatted_revenue": {"$concat": ["$", {"$round": ["$revenue", 0]}]},
      "order_month": "$_id.group_1",
      "order_year": "$_id.group_0",
      "revenue": {"$round": ["$revenue", 2]},
      "total_orders": "$total_orders"
    }
  },
  {
    "$sort": {"revenue": -1}
  },
  {
    "$limit": 20
  }
]
```

#### JOIN Operations

##### Simple INNER JOIN
```bash
zero-sql "SELECT u.name, p.title FROM users u JOIN posts p ON u.id = p.user_id"
```

Output:
```json
[
  {
    "$lookup": {
      "from": "posts",
      "localField": "id",
      "foreignField": "user_id",
      "as": "p"
    }
  },
  {
    "$unwind": "$p"
  },
  {
    "$project": {
      "_id": 0,
      "name": "$u.name",
      "title": "$p.title"
    }
  }
]
```

##### LEFT JOIN
```bash
zero-sql "SELECT u.name, p.title FROM users u LEFT JOIN posts p ON u.id = p.user_id"
```

Output:
```json
[
  {
    "$lookup": {
      "from": "posts",
      "localField": "id",
      "foreignField": "user_id",
      "as": "p"
    }
  },
  {
    "$unwind": {
      "path": "$p",
      "preserveNullAndEmptyArrays": true
    }
  },
  {
    "$project": {
      "_id": 0,
      "name": "$u.name",
      "title": "$p.title"
    }
  }
]
```

##### Multiple JOINs
```bash
zero-sql "SELECT u.name, p.title, c.name as category FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id"
```

Output:
```json
[
  {
    "$lookup": {
      "from": "posts",
      "localField": "id",
      "foreignField": "user_id",
      "as": "p"
    }
  },
  {
    "$unwind": "$p"
  },
  {
    "$lookup": {
      "from": "categories",
      "localField": "category_id",
      "foreignField": "id",
      "as": "c"
    }
  },
  {
    "$unwind": "$c"
  },
  {
    "$project": {
      "_id": 0,
      "category": "$c.name",
      "name": "$u.name",
      "title": "$p.title"
    }
  }
]
```

#### GROUP BY with Aggregation

```bash
zero-sql "SELECT status, COUNT(*) as total FROM orders GROUP BY status"
```

Output:
```json
[
  {
    "$group": {
      "_id": {
        "status": "$status"
      },
      "total": {
        "$sum": 1
      }
    }
  }
]
```

#### Complex WHERE Conditions

```bash
zero-sql "SELECT * FROM products WHERE (price > 100 AND category = 'electronics') OR (price < 50 AND category = 'books')"
```

#### LIKE and ILIKE Pattern Matching

Zero-SQL supports both case-sensitive (LIKE) and case-insensitive (ILIKE) pattern matching:

```bash
# Case-sensitive pattern matching
zero-sql "SELECT name FROM users WHERE email LIKE '%@gmail.com'"

# Case-insensitive pattern matching (PostgreSQL style)
zero-sql "SELECT name FROM users WHERE email ILIKE '%@GMAIL.COM'"
```

ILIKE Output:
```json
[
  {
    "$match": {
      "email": {
        "$regex": ".*@GMAIL.COM",
        "$options": "i"
      }
    }
  },
  {
    "$project": {
      "_id": 0,
      "name": "$name"
    }
  }
]
```

#### BETWEEN Clauses

```bash
zero-sql 'SELECT * FROM transactions WHERE "Date" BETWEEN '\''2023-01-01'\'' AND '\''2023-12-31'\'''
```

Output:
```json
[
  {
    "$match": {
      "Date": {
        "$gte": "2023-01-01",
        "$lte": "2023-12-31"
      }
    }
  }
]
```

#### ORDER BY and LIMIT

```bash
zero-sql "SELECT name, created_at FROM users ORDER BY created_at DESC LIMIT 10"
```

#### String Functions

```bash
# Convert case and concatenate names
zero-sql "SELECT UPPER(name), LOWER(email), CONCAT(first_name, ' ', last_name) as full_name FROM users"

# Extract substrings and get lengths
zero-sql "SELECT SUBSTRING(name, 1, 10) as short_name, LENGTH(description) as desc_length FROM products"

# Replace text in strings
zero-sql "SELECT REPLACE(description, 'old', 'new') as updated_desc FROM products"
```

#### Mathematical Functions

```bash
# Round prices and calculate absolute values
zero-sql "SELECT CEIL(price), FLOOR(discount), ABS(amount) FROM orders"

# Power and square root calculations
zero-sql "SELECT POWER(quantity, 2), SQRT(price), MOD(total, 100) FROM products"
```

#### Date Functions

```bash
# Extract date components
zero-sql "SELECT YEAR(created_at), MONTH(created_at), DAY(created_at) FROM orders"

# Date arithmetic
zero-sql "SELECT DATEADD(created_at, 30, 'day') as due_date FROM invoices"

# Date formatting with STRFTIME
zero-sql "SELECT STRFTIME(created_at, '%Y-%m-%d') as date_formatted FROM orders"
```

#### Conditional Functions

```bash
# Handle null values
zero-sql "SELECT COALESCE(nickname, first_name, 'Unknown') as display_name FROM users"

# Null comparisons
zero-sql "SELECT NULLIF(amount, 0) as valid_amount FROM transactions"

# Rounding with precision
zero-sql "SELECT ROUND(price, 2), ROUND(SUM(amount), 2) as total FROM orders GROUP BY category"
```

#### DISTINCT Queries

```bash
# Get unique categories
zero-sql "SELECT DISTINCT category, status FROM products"

# Count distinct values
zero-sql "SELECT COUNT(DISTINCT category) as unique_categories FROM products"
```

## Supported SQL Features

### Core SQL Syntax
- **SELECT Clause**: Column selection, aliases, DISTINCT, aggregation functions
- **FROM Clause**: Table references with aliases and quoted identifiers
- **JOIN Clause**: INNER, LEFT, RIGHT joins with complex conditions
- **WHERE Clause**: Comparison operators, pattern matching, NULL checks, ranges
- **GROUP BY Clause**: Single/multiple columns with aggregation functions
- **HAVING Clause**: Filter aggregated results
- **ORDER BY Clause**: Ascending/descending with aliases and expressions
- **LIMIT/OFFSET**: Result pagination

### Function Categories

#### SELECT Clause
- Column selection: `SELECT name, age`
- Wildcard: `SELECT *`
- DISTINCT: `SELECT DISTINCT category, status`
- Column aliases: `SELECT name AS full_name`
- Quoted identifiers: `SELECT "User Name", "Email Address"`
- Aggregation functions: `COUNT()`, `SUM()`, `AVG()`, `MIN()`, `MAX()`
- String functions: `UPPER()`, `LOWER()`, `CONCAT()`, `SUBSTRING()`, `LENGTH()`, `REPLACE()`
- Math functions: `ABS()`, `CEIL()`, `FLOOR()`, `POWER()`, `SQRT()`, `MOD()`, `ROUND()`
- Date functions: `YEAR()`, `MONTH()`, `DAY()`, `DATEADD()`, `DATEDIFF()`, `STRFTIME()`
- Conditional functions: `COALESCE()`, `NULLIF()`

### FROM Clause
- Table references: `FROM users`
- Table aliases: `FROM users u`
- Quoted table names: `FROM "User Data"`

### JOIN Clause
- INNER JOIN: `INNER JOIN posts ON users.id = posts.user_id`
- LEFT JOIN: `LEFT JOIN posts ON users.id = posts.user_id`
- Table aliases in JOINs: `FROM users u JOIN posts p ON u.id = p.user_id`
- Multiple JOINs: `FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id`
- JOINs with WHERE conditions: `FROM users u JOIN posts p ON u.id = p.user_id WHERE u.active = true`

### WHERE Clause
- Comparison operators: `=`, `!=`, `<>`, `>`, `<`, `>=`, `<=`
- Pattern matching: `LIKE` (case-sensitive), `ILIKE` (case-insensitive)
- List operations: `IN`, `NOT IN`
- Range operations: `BETWEEN`, `NOT BETWEEN`
- NULL checks: `IS NULL`, `IS NOT NULL`
- Logical operators: `AND`, `OR`, `NOT`
- Parentheses for grouping: `(condition1 OR condition2) AND condition3`
- Quoted column names: `WHERE "Created Date" > '2023-01-01'`

### GROUP BY Clause
- Single column: `GROUP BY status`
- Multiple columns: `GROUP BY category, status`
- Quoted column names: `GROUP BY "Product Category", "Region"`
- Works with aggregation functions in SELECT

### HAVING Clause
- Filter aggregated results: `HAVING COUNT(*) > 5`
- Supports same operators as WHERE clause

### ORDER BY Clause
- Ascending: `ORDER BY name` or `ORDER BY name ASC`
- Descending: `ORDER BY created_at DESC`
- Multiple columns: `ORDER BY category, name DESC`
- Order by aliases: `ORDER BY Total_Amount DESC`
- Quoted column names: `ORDER BY "Created Date" DESC`

### LIMIT and OFFSET
- Limit results: `LIMIT 10`
- Skip results: `OFFSET 20` or `LIMIT 20, 10`

## Advanced Features

### Quoted Identifiers

Zero-SQL fully supports PostgreSQL-style quoted identifiers, allowing you to use column names with:
- Spaces: `"User Name"`
- Special characters: `"Email@Address"`
- Reserved keywords: `"Order"`
- Mixed case: `"UserID"`

### Case-Insensitive Pattern Matching

The `ILIKE` operator provides PostgreSQL-compatible case-insensitive pattern matching:

```sql
-- These are equivalent for matching
WHERE name ILIKE '%john%'  -- Matches: John, JOHN, john, JoHn
WHERE name LIKE '%john%'   -- Matches only: john
```

### Complex Aggregation Queries

Zero-SQL handles sophisticated business intelligence queries with:
- Multiple aggregation functions in a single query
- Complex WHERE clauses with multiple conditions
- GROUP BY with multiple columns including quoted identifiers
- ORDER BY with computed column aliases
- Proper handling of date ranges and IN clauses
- String manipulation and formatting functions
- Mathematical computations and rounding
- Date extraction and arithmetic operations
- Conditional expressions and null handling

### Function Support

Zero-SQL now supports 25+ SQL functions for comprehensive data transformation:

**String Functions**: UPPER, LOWER, CONCAT, SUBSTRING, LENGTH, REPLACE
**Math Functions**: ABS, CEIL, FLOOR, POWER, SQRT, MOD, ROUND
**Date Functions**: YEAR, MONTH, DAY, DATEADD, DATEDIFF, STRFTIME
**Conditional Functions**: COALESCE, NULLIF

These functions can be used in SELECT, WHERE, GROUP BY, HAVING, and ORDER BY clauses, enabling complex data transformations and calculations directly in your SQL queries.

## MongoDB Output

The tool generates MongoDB aggregation pipelines using these stages:

- `$lookup` - for JOIN operations
- `$unwind` - to flatten joined arrays
- `$match` - for WHERE and HAVING clauses
- `$group` - for GROUP BY clauses with proper aggregation handling
- `$project` - for SELECT column specification
- `$sort` - for ORDER BY clauses
- `$skip` - for OFFSET
- `$limit` - for LIMIT

## Error Handling

Zero-SQL provides detailed error messages for:

- Invalid SQL syntax
- Unsupported SQL features
- Type mismatches in quoted identifiers
- Missing required clauses
- Malformed aggregation functions

Use the `--verbose` flag for additional debugging information.

## Limitations

Current limitations include:

- Only SELECT statements are supported (no INSERT, UPDATE, DELETE)
- Subqueries are not yet supported
- Window functions (ROW_NUMBER, RANK, etc.) are not supported
- UNION operations are not supported
- CTEs (Common Table Expressions) are not supported
- Some advanced PostgreSQL-specific features may not be available

## Contributing

Contributions are welcome! Please see the [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Architecture

The project follows a clean architecture with clear separation of concerns:

```
zero-sql/
├── cmd/                 # CLI command definitions
│   └── root.go         # Main command setup
├── internal/converter/  # Core conversion logic
│   ├── converter.go    # Main converter with pipeline building
│   ├── parser.go       # SQL AST parsing with 25+ function handlers
│   └── operators.go    # SQL to MongoDB operator mappings
├── main.go             # Application entry point
├── go.mod              # Go module definition
└── README.md           # This file
```

The converter package is the heart of the application, handling the conversion from SQL Abstract Syntax Trees (AST) to MongoDB aggregation pipelines. It includes specialized handling for quoted identifiers, complex expressions, and 25+ SQL functions including string manipulation, mathematical operations, date functions, and conditional expressions.

## Usage

```bash
# Basic usage
zero-sql "SELECT name, age FROM users WHERE age > 18"

# With quoted column names
zero-sql 'SELECT "User Name", "Email Address" FROM users WHERE "Created Date" > '\''2023-01-01'\'''

# With collection information (helps avoid MongoDB namespace errors)
zero-sql --include-collection "SELECT name, age FROM users WHERE age > 18 LIMIT 5"

# Pretty print JSON output
zero-sql --pretty "SELECT * FROM products"

# Verbose mode
zero-sql --verbose "SELECT name FROM users"

# Complex aggregation query
zero-sql 'SELECT "Department", AVG("Salary") as "Average Salary" FROM employees WHERE "Hire Date" > '\''2020-01-01'\'' GROUP BY "Department" ORDER BY "Average Salary" DESC'

# String functions
zero-sql "SELECT UPPER(name), CONCAT(first_name, ' ', last_name) as full_name FROM users"

# Math and date functions
zero-sql "SELECT ROUND(price, 2), YEAR(created_at), DATEADD(created_at, 30, 'day') FROM orders"

# DISTINCT queries
zero-sql "SELECT DISTINCT category, status FROM products ORDER BY category"
```

## Troubleshooting

### MongoDB Aggregation Namespace Error

If you encounter an error like `(InvalidNamespace) {aggregate: 1} is not valid for '$limit'; a collection is required`, this means that when executing the generated aggregation pipeline in MongoDB, the collection name is not being specified correctly.

**Solution**: Use the `--include-collection` flag to get both the collection name and the pipeline:

```bash
# This outputs collection name and pipeline
zero-sql --include-collection "SELECT name FROM users LIMIT 10"
```

**Output**:
```json
{
  "collection": "users",
  "pipeline": [
    {
      "$limit": 10
    }
  ]
}
```

Then, in your MongoDB client/driver, use the collection name when executing the aggregation:

```javascript
// MongoDB shell
db.users.aggregate([{"$limit": 10}])

// Node.js with MongoDB driver
await db.collection("users").aggregate([{"$limit": 10}]).toArray()
```

### Quoted Identifiers

When using quoted column names in your SQL queries, make sure to:

1. Use double quotes (`"`) for column names, not single quotes (`'`)
2. Escape quotes properly in your shell commands
3. Use the exact case as stored in your MongoDB collection

```bash
# Correct
zero-sql 'SELECT "User Name" FROM users'

# Incorrect - will be treated as a string literal
zero-sql "SELECT 'User Name' FROM users"
``` 