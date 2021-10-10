// Copyright 2018, 2021 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	plsql "github.com/UNO-SOFT/plsql-parser/plsql"
	"github.com/antlr/antlr4/runtime/Go/antlr"
)

//go:generate mkdir -p plsql
//go:generate sh -c "[ -e antlr-4.9.2-complete.jar ] || wget https://www.antlr.org/download/antlr-4.9.2-complete.jar"
//go:generate sh -c "[ -e PlSqlLexer.g4 ] || wget https://github.com/antlr/grammars-v4/raw/master/sql/plsql/PlSqlLexer.g4"
//go:generate sh -c "[ -e PlSqlParser.g4 ] || wget https://github.com/antlr/grammars-v4/raw/master/sql/plsql/PlSqlParser.g4"
//go:generate java -jar antlr-4.9.2-complete.jar -Dlanguage=Go -o plsql/ PlSqlLexer.g4 PlSqlParser.g4
//go:generate sed -i -e "s/self\\./p./; s/PlSqlLexerBase/PlSqlBaseLexer/" plsql/plsql_lexer.go
//go:generate sed -i -e "s/self\\./p./; s/p\\.isVersion/p.IsVersion/; s/PlSqlParserBase/PlSqlBaseParser/" plsql/plsql_parser.go
//go:generate sh -c "[ -e ./plsql/plsql_base_lexer.go ] || curl https://github.com/antlr/grammars-v4/raw/master/sql/plsql/Go/plsql_base_lexer.go | sed -e '/self/d; /input/ s/l\\.input/l.GetInputStream()/' >plsql/plsql_base_lexer.go"
//go:generate sh -c "[ -e ./plsql/plsql_base_parser.go ] || (cd plsql && wget https://github.com/antlr/grammars-v4/raw/master/sql/plsql/Go/plsql_base_parser.go)"

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
	return wl.ConvertMap, fmt.Errorf("%s: %w", text, wl.Err)
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
	wl.AddError(fmt.Errorf("%d:%d: %s (%v): %w", line, column, msg, e, wl.Err))
}

func (wl *iiWalkListener) ExitExpressions(ctx *plsql.ExpressionsContext) {
	if wl.Values != nil {
		return
	}
	for _, expr := range ctx.AllExpression() {
		wl.Values = append(wl.Values, tokenChunk(expr))
	}
}

func (wl *iiWalkListener) ExitInsert_into_clause(ctx *plsql.Insert_into_clauseContext) {
	wl.InsertInto = ctxChunk(ctx)
	if wl.Table != "" {
		return
	}
	wl.Table = ctx.General_table_ref().GetText()
	for _, col := range ctx.INTO().GetChildren() {
		log.Printf("col: %s %#v", col, col)
		//wl.Fields = append(wl.Fields, tokenChunk(col.GetPayload()))
	}
}

func ctxChunk(ctx interface {
	GetStart() antlr.Token
	GetStop() antlr.Token
}) Chunk {
	t := Chunk{Start: ctx.GetStart().GetStart(), Stop: ctx.GetStop().GetStop()}
	t.Text = ctx.GetStart().GetInputStream().GetText(t.Start, t.Stop)
	return t
}
func tokenChunk(token interface {
	GetStart() antlr.Token
	GetStop() antlr.Token
	GetText() string
}) Chunk {
	return Chunk{Start: token.GetStart().GetStart(), Stop: token.GetStop().GetStop(), Text: token.GetText()}
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
	t := ctxChunk(ctx)
	wl.Select.Values = append(wl.Select.Values, t)
	if strings.HasPrefix(t.Text, "CASE ") {
		if i := strings.LastIndexByte(t.Text, ' '); i >= 0 && strings.HasSuffix(t.Text[:i], "END") {
			t.Text = t.Text[i+1:]
		}
	}
	wl.Select.Aliases = append(wl.Select.Aliases, t)
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
	wl.Select.Chunk = Chunk{Start: wl.enterSelect.GetStart(), Stop: ctx.GetStop().GetStop()}
	wl.Select.Text = ctx.GetStart().GetInputStream().GetText(wl.Select.Chunk.Start, wl.Select.Chunk.Stop)
}
func (wl *iiWalkListener) ExitColumn_alias(ctx *plsql.Column_aliasContext) {
	wl.Select.Aliases[len(wl.Select.Aliases)-1] = ctxChunk(ctx)
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
