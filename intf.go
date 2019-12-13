// Copyright 2018 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/godror/godror"
)

type ConvertMap struct {
	Table  string
	Fields []Chunk
	Select *selectStmt
	Values []Chunk

	InsertInto Chunk
}

type Chunk struct {
	Text        string
	Start, Stop int
}

func (t Chunk) String() string { return t.Text }

type selectStmt struct {
	Chunk
	From    []TableWithAlias
	Fields  []Chunk
	Values  []Chunk
	Aliases []Chunk
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
	return fmt.Sprintf("FROM=%v, FIELDS=%v ALIASES=%v TEXT=%s", s.From, s.Fields, s.Aliases, s.Text)
}
func (M ConvertMap) String() string {
	x := interface{}(M.Select)
	if M.Select == nil {
		x = M.Values
	}
	return fmt.Sprintf("%s %v %v", M.Table, M.Fields, x)
}
