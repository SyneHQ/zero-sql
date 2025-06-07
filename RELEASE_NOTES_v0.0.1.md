# Zero-SQL v0.0.1 Release Notes

**Release Date:** [Release Date]

We're excited to announce the initial release of Zero-SQL v0.0.1! 🎉

Zero-SQL is a robust Go package and CLI tool that converts SQL queries to MongoDB aggregation pipelines, making it easy to migrate from relational databases to MongoDB or work with both paradigms seamlessly.

## 🚀 Initial Features

### Core SQL Support
- **SELECT statements** with column selection and aliases
- **FROM clauses** with table references and aliases
- **WHERE clauses** with complex conditional logic
- **ORDER BY** clauses with ascending/descending sort
- **LIMIT and OFFSET** for result pagination
- **GROUP BY** with aggregation functions
- **HAVING clauses** for filtered aggregations

### JOIN Operations
- **INNER JOIN** - Standard inner joins between tables
- **LEFT JOIN** - Left outer joins with null preservation
- **RIGHT JOIN** - Right outer joins
- **Multiple JOINs** - Chain multiple join operations
- **Table aliases** - Support for aliased tables in joins

### Advanced WHERE Conditions
- **Comparison operators**: `=`, `!=`, `<>`, `>`, `<`, `>=`, `<=`
- **Pattern matching**: `LIKE`, `ILIKE` for case-insensitive searches
- **List operations**: `IN`, `NOT IN`
- **NULL checks**: `IS NULL`, `IS NOT NULL`
- **Logical operators**: `AND`, `OR`, `NOT`
- **Parentheses grouping** for complex conditional logic

### Aggregation Functions
- `COUNT()` - Count records
- `SUM()` - Sum numeric values
- `AVG()` - Calculate averages
- `MIN()` - Find minimum values
- `MAX()` - Find maximum values

### CLI Tool Features
- **Multiple output formats**: JSON and BSON
- **Pretty printing** for readable output
- **Verbose mode** for debugging
- **Interactive usage** with command-line flags

## 🛠 MongoDB Pipeline Generation

Zero-SQL generates optimized MongoDB aggregation pipelines using:
- `$lookup` stages for JOIN operations
- `$unwind` stages to flatten joined arrays
- `$match` stages for WHERE and HAVING clauses
- `$group` stages for GROUP BY operations
- `$project` stages for SELECT column specification
- `$sort` stages for ORDER BY clauses
- `$skip` and `$limit` stages for pagination

## 📦 Installation

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

## 🏗 Architecture

Clean architecture with separation of concerns:
- **CLI layer** (`cmd/`) - Command-line interface
- **Converter package** (`internal/converter/`) - Core conversion logic
- **Parser helpers** - SQL AST parsing utilities
- **Operator mappings** - SQL to MongoDB operator translations

## 🔧 Usage Examples

### Basic Query
```bash
zero-sql "SELECT name, age FROM users WHERE age > 18"
```

### Complex JOIN with Aggregation
```bash
zero-sql "SELECT u.name, COUNT(p.id) as post_count FROM users u LEFT JOIN posts p ON u.id = p.user_id GROUP BY u.name HAVING COUNT(p.id) > 5"
```

## ⚠️ Known Limitations

- Only SELECT statements are currently supported
- Subqueries are not yet implemented
- Window functions are not available
- Some advanced SQL features may not be supported

## 🔮 What's Next

Future releases will focus on:
- Subquery support
- Additional SQL statement types (INSERT, UPDATE, DELETE)
- Window functions
- Performance optimizations
- Enhanced error reporting

## 🙏 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on how to get involved.

## 📄 License

Zero-SQL is licensed under the MIT License. See [LICENSE](LICENSE) for details.

---

**Download:** [GitHub Releases](https://github.com/SyneHQ/zero-sql/releases)

**Report Issues:** [GitHub Issues](https://github.com/SyneHQ/zero-sql/issues)

**Documentation:** [README.md](README.md) 