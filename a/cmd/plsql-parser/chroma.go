// Copyright 2021 Tamás Gulácsi. All rights reserved.

package main

import (
	"log"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/lexers/p"
	"github.com/alecthomas/chroma/lexers/s"
)

func ChromaParse(text string) error {
	lexer := chroma.DelegatingLexer(s.SQL,
		chroma.DelegatingLexer(p.PLpgSQL, lexers.Fallback))
	opts := chroma.TokeniseOptions{EnsureLF: true}
	it, err := lexer.Tokenise(&opts, text)
	if err != nil {
		return err
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		log.Println(tok)
	}
	return nil
}
