# Zero-SQL v0.0.2 Release Notes

## 🎉 New Features

### Public API Package
- **Public Converter Package**: Introduced `pkg/zerosql` package to expose converter functionality publicly
- **External Integration**: The converter can now be imported and used by external Go modules
- **Simplified API**: Clean and simple API with `zerosql.New()` and `zerosql.Options{}`

## 🔧 Breaking Changes

### Import Path Changes
- **Old**: `github.com/synehq/zero-sql/internal/converter` (internal package)
- **New**: `github.com/synehq/zero-sql/pkg/zerosql` (public package)

### Usage Changes
```go
// Before (internal package - not accessible externally)
conv := converter.New(&converter.Options{
    Verbose: true,
})

// After (public package)
conv := zerosql.New(&zerosql.Options{
    Verbose: true,
})
```

## 🚀 Improvements

- **Better Modularity**: Clear separation between internal implementation and public API
- **External Usage**: Can now be integrated into other Go projects as a dependency
- **Maintained Functionality**: All existing converter features remain unchanged

## 📦 Installation

For external projects, you can now import and use zero-sql as a library:

```go
import "github.com/synehq/zero-sql/pkg/zerosql"

conv := zerosql.New(&zerosql.Options{
    Verbose: true,
})
pipeline, err := conv.ConvertSQLToMongo("SELECT * FROM users WHERE age > 18")
```

## 🔄 Migration Guide

If you were using the internal package (which wasn't officially supported):

1. Update import: `github.com/synehq/zero-sql/internal/converter` → `github.com/synehq/zero-sql/pkg/zerosql`
2. Update constructor: `converter.New()` → `zerosql.New()`
3. Update options: `converter.Options{}` → `zerosql.Options{}`

---

**Full Changelog**: https://github.com/synehq/zero-sql/compare/v0.0.1...v0.0.2 