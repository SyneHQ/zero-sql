package converter

import (
	"fmt"
	"strconv"

	"github.com/xwb1989/sqlparser"
)

func (c *roCompiler) group(expressions sqlparser.GroupBy, selections []roSelection) ([]roDocument, error) {
	keys := roDocument{}
	columns := map[string]string{}
	for i, expression := range expressions {
		column, ok := expression.(*sqlparser.ColName)
		if !ok {
			return nil, fmt.Errorf("GROUP BY requires column references")
		}
		name, err := c.column(column)
		if err != nil {
			return nil, err
		}
		if _, exists := columns[name]; exists {
			return nil, fmt.Errorf("duplicate GROUP BY columns are unsupported")
		}
		key := "g" + strconv.Itoa(i)
		columns[name] = key
		keys[key] = roField(name)
	}
	group := roDocument{"_id": nil}
	if len(keys) > 0 {
		group["_id"] = keys
	}
	projection := roDocument{"_id": 0}
	defaults := roDocument{}
	for i, selection := range selections {
		if selection.isLiteral {
			projection[selection.name] = roLiteral(selection.literal)
			defaults[selection.name] = selection.literal
			continue
		}
		if selection.aggregate == "" {
			key, exists := columns[selection.column]
			if !exists {
				return nil, fmt.Errorf("selected column must occur in GROUP BY")
			}
			projection[selection.name] = roField("_id." + key)
			continue
		}
		valueName := "a" + strconv.Itoa(i)
		value := roField(selection.argument)
		switch selection.aggregate {
		case "count":
			if selection.star {
				group[valueName] = roDocument{"$sum": 1}
			} else {
				group[valueName] = roDocument{"$sum": roDocument{"$cond": roArray{roNonNull(value), 1, 0}}}
			}
			projection[selection.name] = "$" + valueName
			defaults[selection.name] = 0
		case "sum", "avg", "min", "max":
			// Decimal128 avoids MongoDB's int64 SUM overflow promotion to an
			// inexact double. NULL/missing remain NULL. Non-numeric values cause
			// an execution error instead of MongoDB silently ignoring them.
			operand := roNumeric(value)
			group[valueName] = roDocument{"$" + selection.aggregate: operand}
			projection[selection.name] = "$" + valueName
			defaults[selection.name] = nil
			if selection.aggregate == "sum" {
				countName := "n" + strconv.Itoa(i)
				group[countName] = roDocument{"$sum": roDocument{"$cond": roArray{roNonNull(value), 1, 0}}}
				projection[selection.name] = roDocument{"$cond": roArray{roDocument{"$gt": roArray{"$" + countName, 0}}, "$" + valueName, nil}}
			}
		default:
			return nil, fmt.Errorf("unsupported aggregate")
		}
	}
	stages := []roDocument{{"$group": group}, {"$project": projection}}
	if len(keys) > 0 {
		return stages, nil
	}
	// MongoDB $group emits no document for empty input; SQL global aggregates
	// emit one row (COUNT=0, others NULL). $facet retains that empty-input case.
	return []roDocument{
		{"$facet": roDocument{"rows": stages}},
		{"$project": roDocument{"_id": 0, "row": roDocument{"$ifNull": roArray{roDocument{"$arrayElemAt": roArray{"$rows", 0}}, roLiteral(defaults)}}}},
		{"$replaceWith": "$row"},
	}, nil
}

func roNumeric(value interface{}) roDocument {
	// The error conversion is row-dependent, so it is evaluated only for a
	// non-null, non-numeric value. The query fails before producing a false sum.
	invalid := roDocument{"$convert": roDocument{"input": roDocument{"$concat": roArray{"Unsupported SQL numeric input: ", roDocument{"$type": value}}}, "to": "decimal"}}
	converted := roDocument{"$convert": roDocument{"input": value, "to": "decimal"}}
	return roDocument{"$cond": roArray{roNonNull(value), roDocument{"$cond": roArray{roDocument{"$isNumber": value}, converted, invalid}}, nil}}
}
