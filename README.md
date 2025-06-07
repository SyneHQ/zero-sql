# Zero-SQL

A robust go package that converts SQL queries to MongoDB aggregation pipelines.

> Download cli from here <a href='https://github.com/SyneHQ/zero-sql/releases'>Click here</a>

![ZERO-BANNER](https://c72gdackzgkn7zoa.public.blob.vercel-storage.com/zerosql.png)

## Features

Zero-SQL supports a comprehensive set of SQL features:

- **SELECT statements** with column selection and aliases
- **FROM clauses** with table references
- **JOIN operations** (INNER, LEFT, RIGHT)
- **WHERE clauses** with complex conditions (AND, OR, nested conditions)
- **Comparison operators** (=, !=, >, <, >=, <=, LIKE, ILIKE, IN, NOT IN)
- **NULL checks** (IS NULL, IS NOT NULL)
- **ORDER BY clauses** with ASC/DESC
- **LIMIT and OFFSET**
- **GROUP BY** with aggregation functions (COUNT, SUM, AVG, MIN, MAX)
- **HAVING clauses**

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
  -f, --format string   Output format (json, bson) (default "json")
  -h, --help           help for zero-sql
  -p, --pretty         Pretty print the output (default true)
  -v, --verbose        Verbose output
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

#### LIKE and Pattern Matching

```bash
zero-sql "SELECT name FROM users WHERE email ILIKE '%@gmail.com'"
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
- Aggregation functions: `COUNT()`, `SUM()`, `AVG()`, `MIN()`, `MAX()`

### FROM Clause
- Table references: `FROM users`
- Table aliases: `FROM users u`

### JOIN Clause
- INNER JOIN: `INNER JOIN posts ON users.id = posts.user_id`
- LEFT JOIN: `LEFT JOIN posts ON users.id = posts.user_id`
- Table aliases in JOINs: `FROM users u JOIN posts p ON u.id = p.user_id`
- Multiple JOINs: `FROM users u JOIN posts p ON u.id = p.user_id JOIN categories c ON p.category_id = c.id`
- JOINs with WHERE conditions: `FROM users u JOIN posts p ON u.id = p.user_id WHERE u.active = true`

### WHERE Clause
- Comparison operators: `=`, `!=`, `<>`, `>`, `<`, `>=`, `<=`
- Pattern matching: `LIKE`, `ILIKE`
- List operations: `IN`, `NOT IN`
- NULL checks: `IS NULL`, `IS NOT NULL`
- Logical operators: `AND`, `OR`, `NOT`
- Parentheses for grouping: `(condition1 OR condition2) AND condition3`

### GROUP BY Clause
- Single column: `GROUP BY status`
- Multiple columns: `GROUP BY category, status`
- Works with aggregation functions in SELECT

### HAVING Clause
- Filter aggregated results: `HAVING COUNT(*) > 5`
- Supports same operators as WHERE clause

### ORDER BY Clause
- Ascending: `ORDER BY name` or `ORDER BY name ASC`
- Descending: `ORDER BY created_at DESC`
- Multiple columns: `ORDER BY category, name DESC`

### LIMIT and OFFSET
- Limit results: `LIMIT 10`
- Skip results: `OFFSET 20` or `LIMIT 20, 10`

## MongoDB Output

The tool generates MongoDB aggregation pipelines using these stages:

- `$lookup` - for JOIN operations
- `$unwind` - to flatten joined arrays
- `$match` - for WHERE and HAVING clauses
- `$group` - for GROUP BY clauses
- `$project` - for SELECT column specification
- `$sort` - for ORDER BY clauses
- `$skip` - for OFFSET
- `$limit` - for LIMIT

## Error Handling

Zero-SQL provides detailed error messages for:

- Invalid SQL syntax
- Unsupported SQL features
- Type mismatches
- Missing required clauses

Use the `--verbose` flag for additional debugging information.

## Limitations

Current limitations include:

- Only SELECT statements are supported
- Subqueries are not yet supported
- Window functions are not supported
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
│   ├── parser.go       # SQL AST parsing helpers
│   └── operators.go    # SQL to MongoDB operator mappings
├── main.go             # Application entry point
├── go.mod              # Go module definition
└── README.md           # This file
```

The converter package is the heart of the application, handling the conversion from SQL Abstract Syntax Trees (AST) to MongoDB aggregation pipelines. 