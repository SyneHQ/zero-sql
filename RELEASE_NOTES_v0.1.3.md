# Zero-SQL v0.1.3 Release Notes

## 🎉 New Features

### MongoDB Collection Information Support
- **Collection Extraction**: New `--include-collection` CLI flag that outputs both collection name and aggregation pipeline
- **Enhanced API**: Added `ConvertSQLToMongoWithCollection()` method to the public API
- **Structured Output**: New `ConversionResult` struct containing collection name and pipeline stages
- **Namespace Error Fix**: Solves MongoDB `(InvalidNamespace) {aggregate: 1} is not valid for '$limit'; a collection is required` errors

### CLI Enhancements
- **New Flag**: `--include-collection` / `-c` flag for collection-aware output
- **Better Help**: Updated help documentation with collection information examples
- **Backward Compatibility**: Existing behavior unchanged when flag is not used

## 🔧 API Changes

### New Public Methods
```go
// New method that returns collection name and pipeline
result, err := conv.ConvertSQLToMongoWithCollection(sqlQuery)
fmt.Printf("Collection: %s\n", result.Collection)
fmt.Printf("Pipeline: %v\n", result.Pipeline)
```

### New Types
```go
// ConversionResult holds both collection name and pipeline
type ConversionResult struct {
    Collection string                   `json:"collection"`
    Pipeline   []map[string]interface{} `json:"pipeline"`
}
```

## 🚀 Usage Examples

### CLI Usage
```bash
# Traditional output (pipeline only)
zero-sql "SELECT name FROM users LIMIT 10"

# New collection-aware output
zero-sql --include-collection "SELECT name FROM users LIMIT 10"
```

### API Usage
```go
import "github.com/synehq/zero-sql/pkg/zerosql"

conv := zerosql.New(&zerosql.Options{Verbose: false})

// Get collection and pipeline together
result, err := conv.ConvertSQLToMongoWithCollection("SELECT * FROM users LIMIT 5")
if err != nil {
    return err
}

// Execute in MongoDB with proper collection
// db.collection(result.Collection).aggregate(result.Pipeline)
```

## 🐛 Bug Fixes

### MongoDB Namespace Errors
- **Issue**: Generated pipelines caused `InvalidNamespace` errors when executed in MongoDB
- **Root Cause**: MongoDB aggregation requires explicit collection specification
- **Solution**: Extract collection name from SQL FROM clause and provide it alongside pipeline
- **Impact**: Eliminates common MongoDB execution errors when using generated pipelines

## 🔄 Troubleshooting

### MongoDB Aggregation Errors
If you encounter namespace errors like:
```
(InvalidNamespace) {aggregate: 1} is not valid for '$limit'; a collection is required
```

**Solution**: Use the new `--include-collection` flag:
```bash
zero-sql --include-collection "SELECT name FROM users LIMIT 10"
```

This outputs:
```json
{
  "collection": "users",
  "pipeline": [{"$limit": 10}]
}
```

Then execute in MongoDB:
```javascript
// MongoDB shell
db.users.aggregate([{"$limit": 10}])

// Node.js with MongoDB driver
await db.collection("users").aggregate([{"$limit": 10}]).toArray()
```

## 📝 Documentation Updates

- **README**: Added troubleshooting section for MongoDB namespace errors
- **CLI Help**: Updated examples to include new `--include-collection` flag
- **Usage Guide**: Added collection-aware execution examples

## ⚡ Performance & Compatibility

- **Zero Breaking Changes**: All existing functionality remains unchanged
- **Backward Compatible**: Existing scripts and integrations continue to work
- **Optional Feature**: Collection information is opt-in via flag or method choice
- **Same Performance**: No performance impact on existing workflows

---

**Full Changelog**: https://github.com/synehq/zero-sql/compare/v0.0.2...v0.1.3 