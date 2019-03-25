// Copyright 2018 Tamás Gulácsi. All rights reserved.

package plsqlparser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
)

var Warning = errors.New("WARNING")

type WalkFunc func(context.Context, ConvertMap) error

func Parse(ctx context.Context, w io.Writer, r io.Reader, withAntlr bool, walk WalkFunc) error {
	dec := json.NewDecoder(r)
	var buf strings.Builder
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, ok := t.(json.Delim); ok {
			continue
		}
		if withAntlr {
			M, err := ParseAntlr(t.(string))
			if err != nil {
				if strings.Contains(err.Error(), "extraneous input 'AFED' expecting '('") {
					log.Println(err)
					continue
				}
				return err
			}
			buf.Reset()
			if err = walk(ctx, M); err != nil {
				if errors.Cause(err) == Warning {
					log.Println(err)
					continue
				}
				return err
			}
			qry := buf.String()
			fmt.Fprintf(w, "\n--%s--\n%s", M.Table, qry)
		} else {
			var ii insertInto
			if err := ii.Parse(t.(string)); err != nil {
				return err
			}
			fmt.Println(ii)
		}
	}
	return nil
}

type insertInto struct {
	Table    string
	IsSelect bool
	Fields   []string
	Values   []expression
}

func (ii *insertInto) Parse(text string) error {
	fmt.Println(text)
	var mode uint8

	gettok2 := func() token {
		if text == "" {
			return token{Err: io.EOF}
		}
		tok, rem := gettok(text)
		text = rem
		return tok
	}

	for text != "" {
		tok := gettok2()
		if tok.Err == io.EOF {
			break
		}
		if tok.Err != nil {
			return tok.Err
		}
		switch mode {
		case 0:
			if tok.Type == AtomTok && tok.Value == "INSERT" {
				mode = 1
			}
		case 1:
			if tok.Type == AtomTok && tok.Value == "INTO" {
				mode = 2
			}
		case 2:
			if tok.Type == AtomTok {
				ii.Table = tok.Value
				mode = 3
			}
		case 3:
			if tok.Type == OpenParenTok {
				mode = 4
			}
		case 4:
			switch tok.Type {
			case AtomTok:
				ii.Fields = append(ii.Fields, tok.Value)
			case CloseParenTok:
				mode = 5
			}
		case 5:
			if tok.Type == AtomTok {
				switch tok.Value {
				case "VALUES":
					mode = 6
				case "SELECT":
					ii.IsSelect = true
					mode = 7
				}
			}
		case 6:
			if tok.Type == OpenParenTok {
				mode = 8
			}
		case 8:
			ii.Values = append(ii.Values, expression{Value: tok.Value})

		case 7:
			log.Printf("%d%q rem:%d", tok.Type, tok.Value, len(text))
			toks, err := slurpTokens(nil, gettok2, nil)
			if err != nil {
				return err
			}
			sel, err := parseSelect(toks)
			if err != nil {
				return err
			}
			ii.Values = append(ii.Values, sel.Fields...)
		}
	}
	return nil
}

type selectStatement struct {
	Fields []expression
}

func parseSelect(tokens []token) (selectStatement, error) {
	ss := selectStatement{Fields: []expression{{}}}
	var n int
	for len(tokens) != 0 {
		tok := tokens[0]
		tokens = tokens[1:]
		n++
		switch tok.Type {
		case CommaTok:
			ss.Fields = append(ss.Fields, expression{})

		case OpenParenTok:
			rest, sub, err := parseTillCloseParen(tokens)
			if err != nil {
				return ss, err
			}
			tokens = rest
			ss.Fields[0] = expression{Expr: sub}

		default:
			ss.Fields[0].Expr = append(ss.Fields[0].Expr, expression{Value: tok.Value})
		}
	}
	return ss, nil
}

type expression struct {
	Value string
	Expr  expressions
}
type expressions []expression

func parseTillCloseParen(tokens []token) ([]token, expressions, error) {
	var expr expressions
	for len(tokens) > 0 {
		tok := tokens[0]
		tokens = tokens[1:]
		if tok.Type == OpenParenTok {
			rest, sub, err := parseTillCloseParen(tokens)
			if err != nil {
				return tokens, expr, err
			}
			tokens = rest
			expr = append(expr, expression{Expr: sub})
		}
		if tok.Type == CloseParenTok {
			return tokens, expr, nil
		}
		expr = append(expr, expression{Value: tok.Value})
	}
	return tokens, expr, nil
}

func slurpTokens(tokens []token, gettok func() token, till func(token) bool) ([]token, error) {
	for {
		tok := gettok()
		if tok.Err == io.EOF {
			break
		}
		if tok.Err != nil {
			return tokens, tok.Err
		}
		tokens = append(tokens, tok)
		if till != nil && till(tok) {
			return tokens, nil
		}
	}
	return tokens, nil
}

type token struct {
	Type  tokenType
	Value string
	Err   error
}
type tokenType uint8

const (
	OpenParenTok = tokenType(iota + 1)
	CloseParenTok
	CommaTok
	AtomTok
	StringTok
	NumberTok
	OpTok
	LineCommentTok
	BlockCommentTok
	EndTok
)

func gettok(text string) (token, string) {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return token{Type: EndTok}, ""
	}
	if len(text) > 1 {
		switch text[:2] {
		case "--":
			if i := strings.Index(text, "\n"); i >= 2 {
				return token{Type: LineCommentTok, Value: text[2:i]}, text[i+1:]
			}
		case "/*":
			if i := strings.Index(text, "*/"); i >= 2 {
				return token{Type: BlockCommentTok, Value: text[2:i]}, text[i+2:]
			}
		case "||", ":=":
			return token{Type: OpTok, Value: text[:2]}, text[2:]
		}
	}
	switch text[0] {
	case '(':
		return token{Type: OpenParenTok}, text[1:]
	case ')':
		return token{Type: CloseParenTok}, text[1:]
	case ',':
		return token{Type: CommaTok}, text[1:]
	case '\'':
		if i := strings.IndexByte(text[1:], '\''); i >= 0 {
			return token{Type: StringTok, Value: text[1 : 1+i]}, text[1+i+1:]
		}
	case ';':
		return token{Type: EndTok}, text[1:]
	case '-', '+', '=', '*', '<', '>', '/':
		return token{Type: OpTok, Value: text[:1]}, text[1:]
	}
	r, size := utf8.DecodeRuneInString(text)
	if isDigit(r) {
		var i int
		if i = strings.IndexFunc(text[size:], notDigitDot); i < 0 {
			i = len(text) - size
		}
		return token{Type: NumberTok, Value: text[:size+i]}, text[size+i:]
	}
	if isBeginName(r) {
		var i int
		if i = strings.IndexFunc(text[size:], notInName); i < 0 {
			i = len(text) - size
		}
		return token{Type: AtomTok, Value: text[:size+i]}, text[size+i:]
	}

	return token{Type: AtomTok, Err: errors.Errorf("unknown : %q", text)}, ""
}

func isBeginName(r rune) bool {
	return r == '.' || r == '"' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_'
}
func isInName(r rune) bool    { return r == '.' || r == '"' || '0' <= r && r <= '9' || isBeginName(r) }
func notInName(r rune) bool   { return !isInName(r) }
func isDigit(r rune) bool     { return '0' <= r && r <= '9' }
func notDigitDot(r rune) bool { return !(r == '.' || isDigit(r)) }
