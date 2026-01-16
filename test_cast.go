package main

import (
	"fmt"
	"github.com/xwb1989/sqlparser"
)

func main() {
	sql := "SELECT CAST(amount AS INTEGER) FROM test"
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		fmt.Println("Not a SELECT statement")
		return
	}

	for _, selExpr := range selectStmt.SelectExprs {
		if aliasedExpr, ok := selExpr.(*sqlparser.AliasedExpr); ok {
			if funcExpr, ok := aliasedExpr.Expr.(*sqlparser.FuncExpr); ok {
				fmt.Printf("Function: %s\n", funcExpr.Name.String())
				fmt.Printf("Args: %+v\n", funcExpr.Exprs)
			} else {
				fmt.Printf("Expression: %+v\n", aliasedExpr.Expr)
			}
		}
	}
}