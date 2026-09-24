// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Command plsqlparse parses PL/SQL source files and prints a JSON array
// of {type, begin, end} objects describing the source extent of every
// package, package body, procedure and function found, nested ones
// included. begin/end are byte offsets into the original file: begin
// points at the first byte of the construct (CREATE at top level, the
// PROCEDURE/FUNCTION keyword when nested), end just past the terminating
// ';'. Files may use any single-byte encoding (e.g. ISO-8859-2).
package main

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"

	plsql "github.com/UNO-SOFT/plsql-parser/b"
)

type outObj struct {
	plsql.Object
	BeginLine int `json:",omitzero"`
	EndLine   int `json:",omitzero"`
}

func main() {
	if err := Main(); err != nil {
		log.Fatal(err)
	}
}

func Main() error {
	withLine := flag.Bool("line", false, "include 1-based begin_line/end_line in the output")
	indent := flag.Bool("indent", false, "pretty-print the JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: plsqlparse [flags] file.sql ...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		return flag.ErrHelp
	}

	// cache in-source line numbers till position
	type posLine struct{ Pos, Line int }
	posCmp := func(a, b posLine) int { return cmp.Compare(a.Pos, b.Pos) }
	cache := make(map[*[]byte][]posLine)
	lineOf := func(src *[]byte, pos int) int {
		i, ok := slices.BinarySearchFunc(cache[src], posLine{Pos: pos}, posCmp)
		if ok {
			return cache[src][i].Line
		} else if i > 0 {
			prev := cache[src][i-1]
			n := prev.Line + 1 + bytes.Count((*src)[prev.Pos:pos], []byte{'\n'})
			slices.Insert(cache[src], i, posLine{Pos: pos, Line: n})
			return n
		}
		n := 1 + bytes.Count((*src)[:pos], []byte{'\n'})
		lines := append(make([]posLine, 1, len(cache[src])+1), cache[src]...)
		lines[0] = posLine{Pos: pos, Line: n}
		cache[src] = lines
		return n
	}

	out := make(map[string][]outObj, flag.NArg())
	for _, path := range flag.Args() {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		objs, err := plsql.Parse(src)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out[path] = slices.Grow(out[path], len(objs))
		for _, o := range objs {
			oo := outObj{Object: o}
			if *withLine {
				oo.BeginLine = lineOf(&src, o.Begin)
				oo.EndLine = lineOf(&src, o.End)
			}
			out[path] = append(out[path], oo)
		}
		clear(cache)
	}
	var opts []jsontext.Options
	if *indent {
		opts = append(opts, jsontext.WithIndent("  "))
	}
	return json.MarshalWrite(os.Stdout, out, opts...)
}
