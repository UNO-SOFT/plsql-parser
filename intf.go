// Copyright 2018 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"context"
	"database/sql"
	"fmt"

	_ "gopkg.in/goracle.v2"
)

type ConvertMap struct {
	Table  string
	Fields []string
	Select *selectStmt
	Values []string
}

type selectStmt struct {
	From   []TableWithAlias
	Fields []string
}
type TableWithAlias struct {
	Alias, Table string
}

type querier interface {
	Query(string, ...interface{}) (*sql.Rows, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}
type execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func (s *selectStmt) String() string {
	return fmt.Sprintf("FROM=%v, FIELDS=%v", s.From, s.Fields)
}
func (M ConvertMap) String() string {
	x := interface{}(M.Select)
	if M.Select == nil {
		x = M.Values
	}
	return fmt.Sprintf("%s %v %v", M.Table, M.Fields, x)
}
