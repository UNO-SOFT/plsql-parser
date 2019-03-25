// Copyright 2018 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"log"
	"strings"
	"unicode"

	"bramp.net/antlr4/plsql"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"github.com/pkg/errors"
)

var _ = ParseAntlr

func ParseAntlr(text string) (ConvertMap, error) {
	// Setup the input (which this parser expects to be uppercased).
	text = strings.TrimPrefix(upper(strings.TrimSpace(text)), "INSERT ")

	// Create the Lexer
	lexer := plsql.NewPlSqlLexer(antlr.NewInputStream(text))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	// Create the Parser
	parser := plsql.NewPlSqlParser(stream)
	parser.BuildParseTrees = true

	// Finally walk the tree
	wl := &iiWalkListener{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	parser.AddErrorListener(wl)
	tree := parser.Single_table_insert()
	antlr.ParseTreeWalkerDefault.Walk(wl, tree)
	if false && wl.ConvertMap.Select != nil {
		if n, m := len(wl.ConvertMap.Fields), len(wl.ConvertMap.Select.Fields); n < m {
			wl.ConvertMap.Select.Fields = wl.ConvertMap.Select.Fields[:n]
		} else if n > m {
			wl.ConvertMap.Fields = wl.ConvertMap.Fields[:m]
		}
	}

	log.Println(wl.ConvertMap)
	return wl.ConvertMap, errors.Wrap(wl.Err, text)
}

type iiWalkListener struct {
	*plsql.BasePlSqlParserListener

	*antlr.DefaultErrorListener
	ConvertMap
	Err error
}

func (wl *iiWalkListener) ReportAmbiguity(recognizer antlr.Parser, dfa *antlr.DFA, startIndex, stopIndex int, exact bool, ambigAlts *antlr.BitSet, configs antlr.ATNConfigSet) {
	log.Printf("AMBIGUITY at %d:%d", startIndex, stopIndex)
}

func (wl *iiWalkListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	wl.Err = errors.Errorf("%d:%d: %s (%v)", line, column, msg, e)
}

//func (wl *iiWalkListener) ExitSingle_table_insert(ctx *plsql.Single_table_insertContext) {
//log.Printf("--INTO %v", ctx.GetChildren())
//}
func (wl *iiWalkListener) ExitExpressions(ctx *plsql.ExpressionsContext) {
	if wl.Values != nil {
		return
	}
	for _, expr := range ctx.AllExpression() {
		wl.Values = append(wl.Values, expr.GetText())
	}
}
func (wl *iiWalkListener) ExitInsert_into_clause(ctx *plsql.Insert_into_clauseContext) {
	if wl.Table != "" {
		return
	}
	wl.Table = ctx.General_table_ref().GetText()
	for _, col := range ctx.AllColumn_name() {
		wl.Fields = append(wl.Fields, col.GetText())
	}
}

func (wl *iiWalkListener) ExitSelect_list_elements(ctx *plsql.Select_list_elementsContext) {
	if wl.Select == nil {
		wl.Select = &selectStmt{}
	} else if wl.Select.From != nil {
		return
	}

	wl.Select.Fields = append(wl.Select.Fields, ctx.GetStart().GetInputStream().GetText(ctx.GetStart().GetStart(), ctx.GetStop().GetStop()))
}

func (wl *iiWalkListener) ExitFrom_clause(ctx *plsql.From_clauseContext) {
	if wl.Select == nil {
		wl.Select = &selectStmt{}
	} else if wl.Select.From != nil {
		return
	}
	for _, tbl := range ctx.Table_ref_list().(*plsql.Table_ref_listContext).AllTable_ref() {
		tbl := tbl.(*plsql.Table_refContext)
		aux := tbl.Table_ref_aux().(*plsql.Table_ref_auxContext)
		name := aux.Table_ref_aux_internal().GetText()
		var alias string
		if a := aux.Table_alias(); a != nil {
			alias = a.GetText()
		}
		wl.Select.From = append(wl.Select.From, TableWithAlias{Table: name, Alias: alias})
	}
}

func upper(text string) string {
	var inString bool
	return strings.Map(func(r rune) rune {
		if r == '\'' {
			inString = !inString
			return r
		}
		if inString {
			return r
		}
		return unicode.ToUpper(r)
	},
		text)
}
