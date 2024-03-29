// Copyright 2018 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package plsqlparser

import (
	"fmt"
	"io"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
)

var Warning = errors.New("WARNING")

type InsertInto struct {
	Table    string
	IsSelect bool
	Fields   []string
	Values   []Expression
}

// Parse the text, naively.
func (ii *InsertInto) Parse(text string) error {
	fmt.Println(text)
	var mode uint8

	gettok2 := func() Token {
		if text == "" {
			return Token{Err: io.EOF}
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
			ii.Values = append(ii.Values, Expression{Value: tok.Value})

		case 7:
			log.Printf("%d%q rem:%d", tok.Type, tok.Value, len(text))
			toks, err := slurpTokens(nil, gettok2, nil)
			if err != nil {
				return err
			}
			sel, err := ParseSelect(toks)
			if err != nil {
				return err
			}
			ii.Values = append(ii.Values, sel.Fields...)
		}
	}
	return nil
}

type SelectStatement struct {
	Fields []Expression
}

func ParseSelect(tokens []Token) (SelectStatement, error) {
	ss := SelectStatement{Fields: []Expression{{}}}
	var n int
	for len(tokens) != 0 {
		tok := tokens[0]
		tokens = tokens[1:]
		n++
		switch tok.Type {
		case CommaTok:
			ss.Fields = append(ss.Fields, Expression{})

		case OpenParenTok:
			rest, sub, err := ParseTillCloseParen(tokens)
			if err != nil {
				return ss, err
			}
			tokens = rest
			ss.Fields[0] = Expression{Expr: sub}

		default:
			ss.Fields[0].Expr = append(ss.Fields[0].Expr, Expression{Value: tok.Value})
		}
	}
	return ss, nil
}

type Expression struct {
	Value string
	Expr  Expressions
}
type Expressions []Expression

func ParseTillCloseParen(tokens []Token) ([]Token, Expressions, error) {
	var expr Expressions
	for len(tokens) > 0 {
		tok := tokens[0]
		tokens = tokens[1:]
		if tok.Type == OpenParenTok {
			rest, sub, err := ParseTillCloseParen(tokens)
			if err != nil {
				return tokens, expr, err
			}
			tokens = rest
			expr = append(expr, Expression{Expr: sub})
		}
		if tok.Type == CloseParenTok {
			return tokens, expr, nil
		}
		expr = append(expr, Expression{Value: tok.Value})
	}
	return tokens, expr, nil
}

func slurpTokens(tokens []Token, gettok func() Token, till func(Token) bool) ([]Token, error) {
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

type Token struct {
	Type  TokenType
	Value string
	Err   error
}
type TokenType uint8

const (
	OpenParenTok = TokenType(iota + 1)
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

func gettok(text string) (Token, string) {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return Token{Type: EndTok}, ""
	}
	if len(text) > 1 {
		switch text[:2] {
		case "--":
			if i := strings.Index(text, "\n"); i >= 2 {
				return Token{Type: LineCommentTok, Value: text[2:i]}, text[i+1:]
			}
		case "/*":
			if i := strings.Index(text, "*/"); i >= 2 {
				return Token{Type: BlockCommentTok, Value: text[2:i]}, text[i+2:]
			}
		case "||", ":=":
			return Token{Type: OpTok, Value: text[:2]}, text[2:]
		}
	}
	switch text[0] {
	case '(':
		return Token{Type: OpenParenTok}, text[1:]
	case ')':
		return Token{Type: CloseParenTok}, text[1:]
	case ',':
		return Token{Type: CommaTok}, text[1:]
	case '\'':
		if i := strings.IndexByte(text[1:], '\''); i >= 0 {
			return Token{Type: StringTok, Value: text[1 : 1+i]}, text[1+i+1:]
		}
	case ';':
		return Token{Type: EndTok}, text[1:]
	case '-', '+', '=', '*', '<', '>', '/':
		return Token{Type: OpTok, Value: text[:1]}, text[1:]
	}
	r, size := utf8.DecodeRuneInString(text)
	if isDigit(r) {
		var i int
		if i = strings.IndexFunc(text[size:], notDigitDot); i < 0 {
			i = len(text) - size
		}
		return Token{Type: NumberTok, Value: text[:size+i]}, text[size+i:]
	}
	if isBeginName(r) {
		var i int
		if i = strings.IndexFunc(text[size:], notInName); i < 0 {
			i = len(text) - size
		}
		return Token{Type: AtomTok, Value: text[:size+i]}, text[size+i:]
	}

	return Token{Type: AtomTok, Err: errors.Errorf("unknown : %q", text)}, ""
}

func isBeginName(r rune) bool {
	return r == '.' || r == '"' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_'
}
func isInName(r rune) bool    { return r == '.' || r == '"' || '0' <= r && r <= '9' || isBeginName(r) }
func notInName(r rune) bool   { return !isInName(r) }
func isDigit(r rune) bool     { return '0' <= r && r <= '9' }
func notDigitDot(r rune) bool { return !(r == '.' || isDigit(r)) }
