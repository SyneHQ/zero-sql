# Zero-SQL

A robust go package that converts SQL queries to MongoDB aggregation pipelines with support for complex queries and quoted identifiers.

> Download cli from here <a href='https://github.com/SyneHQ/zero-sql/releases'>Click here</a>

![ZERO-BANNER](https://c72gdackzgkn7zoa.public.blob.vercel-storage.com/zerosql.png)

## Features

Zero-SQL supports a comprehensive set of SQL features:

- **SELECT statements** with column selection and aliases
- **Quoted identifiers** (column names with spaces or special characters)
- **FROM clauses** with table references
- **JOIN operations** (INNER, LEFT, RIGHT)
- **WHERE clauses** with complex conditions (AND, OR, nested conditions)
- **Comparison operators** (=, !=, >, <, >=, <=, LIKE, ILIKE, IN, NOT IN)
- **NULL checks** (IS NULL, IS NOT NULL)
- **BETWEEN clauses** with date ranges and numeric ranges
- **ORDER BY clauses** with ASC/DESC
- **LIMIT and OFFSET**
- **GROUP BY** with aggregation functions (COUNT, SUM, AVG, MIN, MAX)
- **HAVING clauses**
- **Case-insensitive pattern matching** with ILIKE

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

#### Complex Investment Analysis Example

```bash
zero-sql 'SELECT _id, "Date", "Description", "Operation", SUM("Amount") AS Total_Amount, COUNT(_id) AS Investment_Count, AVG("Amount") AS Average_Investment_Amount FROM investments WHERE "Date" BETWEEN '\''2023-01-01'\'' AND '\''2023-12-31'\'' AND "Operation" IN ('\''Deposit'\'', '\''Withdrawal'\'') GROUP BY _id, "Date", "Description", "Operation" ORDER BY Total_Amount DESC LIMIT 50;'
```

Output:
```json
[
  {
    "$match": {
      "$and": [
        {
          "Date": {
            "$gte": "2023-01-01",
            "$lte": "2023-12-31"
          }
        },
        {
          "Operation": {
            "$in": ["Deposit", "Withdrawal"]
          }
        }
      ]
    }
  },
  {
    "$group": {
      "Average_Investment_Amount": {"$avg": "$Amount"},
      "Investment_Count": {
        "$sum": {
          "$cond": [{"$ne": ["$_id", null]}, 1, 0]
        }
      },
      "Total_Amount": {"$sum": "$Amount"},
      "_id": {
        "Date": "$Date",
        "Description": "$Description", 
        "Operation": "$Operation",
        "_id": "$_id"
      }
    }
  },
  {
    "$sort": {"Total_Amount": -1}
  },
  {
    "$limit": 50
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

## Supported SQL Features

### SELECT Clause
- Column selection: `SELECT name, age`
- Wildcard: `SELECT *`
- Column aliases: `SELECT name AS full_name`
- Quoted identifiers: `SELECT "User Name", "Email Address"`
- Aggregation functions: `COUNT()`, `SUM()`, `AVG()`, `MIN()`, `MAX()`

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

- Only SELECT statements are supported
- Subqueries are not yet supported
- Window functions are not supported
- HAVING clauses with complex function expressions need additional development
- Some advanced SQL features may not be available

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
│   ├── parser.go       # SQL AST parsing helpers with quoted identifier support
│   └── operators.go    # SQL to MongoDB operator mappings
├── main.go             # Application entry point
├── go.mod              # Go module definition
└── README.md           # This file
```

The converter package is the heart of the application, handling the conversion from SQL Abstract Syntax Trees (AST) to MongoDB aggregation pipelines. It includes specialized handling for quoted identifiers and complex expression parsing.

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