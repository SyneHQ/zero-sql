package zerosql

import (
	"github.com/synehq/zero-sql/internal/converter"
)

// Converter wraps the internal converter to provide a public API
type Converter struct {
	conv *converter.Converter
}

// Options holds configuration for the converter
type Options struct {
	Verbose bool
}

// New creates a new Converter instance
func New(opts *Options) *Converter {
	if opts == nil {
		opts = &Options{}
	}

	conv := converter.New(&converter.Options{
		Verbose: opts.Verbose,
	})

	return &Converter{
		conv: conv,
	}
}

// ConvertSQLToMongo converts a SQL query to MongoDB aggregation pipeline
func (c *Converter) ConvertSQLToMongo(sqlQuery string) ([]map[string]interface{}, error) {
	return c.conv.ConvertSQLToMongo(sqlQuery)
}
