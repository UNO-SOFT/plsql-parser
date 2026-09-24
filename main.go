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
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

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
	app := ff.Command{Name: "plsql-parser"}

	flags := ff.NewFlagSet("boundaries")
	flagBoundariesWithLine := flags.Bool('l', "line", "include 1-based begin_line/end_line in the output")
	flagBoundariesIndent := flags.Bool('i', "indent", "pretty-print the JSON")
	boundariesCmd := ff.Command{Name: "boundaries", Flags: flags,
		ShortHelp: "print JSON of function boundaries",
		Usage:     "boundaries [flags] file.sql ...",
		Exec: func(ctx context.Context, args []string) error {
			// cache in-source line numbers till position
			// this caching results 200ms speedup for an 500kiB source
			type posLine struct{ Pos, Line int }
			posCmp := func(a, b posLine) int { return cmp.Compare(a.Pos, b.Pos) }
			cache := make(map[*[]byte][]posLine)
			lineOf := func(src *[]byte, pos int) int {
				// return 1 + bytes.Count((*src)[:pos], []byte{'\n'})

				i, ok := slices.BinarySearchFunc(cache[src], posLine{Pos: pos}, posCmp)
				if ok {
					return cache[src][i].Line
				} else if i > 0 {
					prev := cache[src][i-1]
					n := prev.Line + bytes.Count((*src)[prev.Pos:pos], []byte{'\n'})
					cache[src] = slices.Insert(cache[src], i, posLine{Pos: pos, Line: n})
					return n
				}
				n := 1 + bytes.Count((*src)[:pos], []byte{'\n'})
				lines := append(make([]posLine, 1, len(cache[src])+1), cache[src]...)
				lines[0] = posLine{Pos: pos, Line: n}
				cache[src] = lines
				return n
			}

			out := make(map[string][]outObj, len(args))
			for _, path := range args {
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
					if *flagBoundariesWithLine {
						oo.BeginLine = lineOf(&src, o.Begin)
						oo.EndLine = lineOf(&src, o.End)
					}
					out[path] = append(out[path], oo)
				}
				clear(cache)
			}
			var opts []jsontext.Options
			if *flagBoundariesIndent {
				opts = append(opts, jsontext.WithIndent("  "))
			}
			return json.MarshalWrite(os.Stdout, out, opts...)
		},
	}
	app.Subcommands = append(app.Subcommands, &boundariesCmd)

	if err := app.Parse(os.Args[1:]); err != nil {
		ffhelp.Command(&app).WriteTo(os.Stderr)
		if errors.Is(err, ff.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return app.Run(ctx)
}
