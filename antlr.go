// Copyright 2018 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"strings"
	"unicode"

	"bramp.net/antlr4/plsql"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"github.com/pkg/errors"
)

type (
	ErrorListener = antlr.ErrorListener
	Tree          = antlr.Tree
)

// Export functionality of plsql package.
var (
	// NewPlSqlLexer is a copy of plsql.NewPlSqlLexer
	NewPlSqlLexer = plsql.NewPlSqlLexer
	// NewPlSqlParser is a copy of plsql.NewPlSqlParser
	NewPlSqlParser = plsql.NewPlSqlParser
)

// NewPlSqlStringLexer returns a new *PlSqlLexer with an input stream set to the given text.
func NewPlSqlStringLexer(text string) *plsql.PlSqlLexer {
	return plsql.NewPlSqlLexer(antlr.NewInputStream(text))
}

// NewPlSqlLexerParser returns a new *PlSqlParser, including a PlSqlLexer with the given text.
func NewPlSqlLexerParser(text string) *plsql.PlSqlParser {
	lexer := plsql.NewPlSqlLexer(antlr.NewInputStream(text))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	// Create the Parser
	parser := plsql.NewPlSqlParser(stream)
	parser.BuildParseTrees = true
	return parser
}

// NewPlSqlParserListener returns a *BaseWalkListener with the DefaultErrorListener set.
func NewPlSqlParserListener() *BaseWalkListener {
	return &BaseWalkListener{DefaultErrorListener: antlr.NewDefaultErrorListener()}
}

// ParseToConvertMap parses the text into a ConvertMap (INSERT INTO with SELECT statements only).
func ParseToConvertMap(text string) (ConvertMap, error) {
	// Setup the input (which this parser expects to be uppercased).
	text = strings.TrimPrefix(upper(strings.TrimSpace(text)), "INSERT ")

	parser := NewPlSqlLexerParser(text)

	// Finally walk the tree
	wl := &iiWalkListener{BaseWalkListener: BaseWalkListener{DefaultErrorListener: antlr.NewDefaultErrorListener()}}
	parser.AddErrorListener(wl)
	tree := parser.Single_table_insert()
	antlr.ParseTreeWalkerDefault.Walk(wl, tree)

	if wl.Err == nil {
		return wl.ConvertMap, nil
	}
	return wl.ConvertMap, errors.Wrap(wl.Err, text)
}

// BaseWalkListener is a minimal Walk Listener.
type BaseWalkListener struct {
	*plsql.BasePlSqlParserListener
	*antlr.DefaultErrorListener
	Ambiguity [][2]int
	Err       *Errors
}

// Walk the given Tree, with the optional parser's ErrorListener set to wl.
func (wl *BaseWalkListener) Walk(tree Tree, parser interface{ AddErrorListener(ErrorListener) }) error {
	if parser != nil {
		parser.AddErrorListener(wl)
	}
	antlr.ParseTreeWalkerDefault.Walk(wl, tree)
	return wl.Err
}

func (wl *BaseWalkListener) AddError(err error) {
	if err != nil {
		if wl.Err == nil {
			wl.Err = new(Errors)
		}
		wl.Err.Append(err)
	}
}

//func (wl *BaseWalkListener) EnterEveryRule(ctx antlr.ParserRuleContext) {
//fmt.Println("ENTER", ctx.GetStart())
//}
//func (wl *BaseWalkListener) ExitEveryRule(ctx antlr.ParserRuleContext) {
//fmt.Println("EXIT", ctx.GetStop())
//}

type iiWalkListener struct {
	BaseWalkListener
	ConvertMap

	enterSelect antlr.Token
}

func (wl *iiWalkListener) ReportAmbiguity(recognizer antlr.Parser, dfa *antlr.DFA, startIndex, stopIndex int, exact bool, ambigAlts *antlr.BitSet, configs antlr.ATNConfigSet) {
	//log.Printf("AMBIGUITY at %d:%d", startIndex, stopIndex)

	wl.Ambiguity = append(wl.Ambiguity, [2]int{startIndex, stopIndex})
}

func (wl *iiWalkListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	wl.AddError(errors.Errorf("%d:%d: %s (%v)", line, column, msg, e))
}

func (wl *iiWalkListener) ExitExpressions(ctx *plsql.ExpressionsContext) {
	if wl.Values != nil {
		return
	}
	for _, expr := range ctx.AllExpression() {
		wl.Values = append(wl.Values, expr.GetText())
	}
}

func (wl *iiWalkListener) ExitInsert_into_clause(ctx *plsql.Insert_into_clauseContext) {
	wl.InsertInto = ctx.GetStart().GetInputStream().GetText(ctx.GetStart().GetStart(), ctx.GetStop().GetStop())
	if wl.Table != "" {
		return
	}
	wl.Table = ctx.General_table_ref().GetText()
	for _, col := range ctx.AllColumn_name() {
		wl.Fields = append(wl.Fields, col.GetText())
	}
}

func (wl *iiWalkListener) ExitSelect_list_elements(ctx *plsql.Select_list_elementsContext) {
	if ctx.GetStart() == nil || ctx.GetStop() == nil || ctx.GetStart().GetInputStream() == nil {
		return
	}
	if wl.Select == nil {
		wl.Select = &selectStmt{}
	} else if wl.Select.From != nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && err != nil {
				wl.AddError(err)
			}
		}
	}()
	s := ctx.GetStart().GetInputStream().GetText(ctx.GetStart().GetStart(), ctx.GetStop().GetStop())
	wl.Select.Values = append(wl.Select.Values, s)
	if strings.HasPrefix(s, "CASE ") {
		if i := strings.LastIndexByte(s, ' '); i >= 0 && strings.HasSuffix(s[:i], "END") {
			s = s[i+1:]
		}
	}
	wl.Select.Aliases = append(wl.Select.Aliases, s)
}
func (wl *iiWalkListener) EnterSelect_statement(ctx *plsql.Select_statementContext) {
	if wl.enterSelect == nil {
		wl.enterSelect = ctx.GetStart()
	}
}
func (wl *iiWalkListener) ExitSelect_statement(ctx *plsql.Select_statementContext) {
	if wl.Select == nil {
		wl.Select = &selectStmt{}
	}
	wl.Select.Text =
		ctx.GetStart().GetInputStream().GetText(wl.enterSelect.GetStart(), ctx.GetStop().GetStop())
}
func (wl *iiWalkListener) ExitColumn_alias(ctx *plsql.Column_aliasContext) {
	wl.Select.Aliases[len(wl.Select.Aliases)-1] = ctx.GetStart().GetInputStream().GetText(ctx.GetStart().GetStart(), ctx.GetStop().GetStop())
}

func (wl *iiWalkListener) ExitFrom_clause(ctx *plsql.From_clauseContext) {
	if wl.Select == nil {
		wl.Select = &selectStmt{}
	} else if wl.Select.From != nil {
		return
	}
	wl.Select.From = []TableWithAlias{} // not nil
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

var _ = error((*Errors)(nil))

// Errors implements the "error" interface, and holds several errors.
type Errors struct {
	slice []error
}

func (es *Errors) Append(err error) {
	if err != nil {
		es.slice = append(es.slice, err)
	}
}
func (es *Errors) Error() string {
	if es == nil || len(es.slice) == 0 {
		return ""
	}
	var buf strings.Builder
	var notFirst bool
	for _, e := range es.slice {
		if e == nil {
			continue
		}
		if notFirst {
			buf.WriteByte('\n')
		} else {
			notFirst = true
		}
		buf.WriteString(e.Error())
	}
	return buf.String()
}
