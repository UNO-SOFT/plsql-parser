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
//
// The nocomment subcommand writes the source with its comments replaced by
// spaces of the same length, keeping byte offsets and line numbers intact.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"slices"

	"github.com/UNO-SOFT/cli"
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
	app := cli.Command{Name: "plsql-parser"}

	flags := flag.NewFlagSet("boundaries", flag.ContinueOnError)
	flagBoundariesWithLine := flags.Bool("line", false, "include 1-based begin_line/end_line in the output")
	flagBoundariesIndent := flags.Bool("indent", false, "pretty-print the JSON")
	boundariesCmd := cli.Command{Name: "boundaries", Flags: flags,
		FlagConfigs: []cli.FlagConfig{
			{Name: "line", Short: "l"},
			{Name: "indent", Short: "i"},
		},
		Summary: "print JSON of function boundaries",
		Usage:   "boundaries [flags] file.sql ...",
		Exec: func(ctx context.Context, state *cli.State) error {
			args :=
				// cache in-source line numbers till position
				// this caching results 200ms speedup for an 500kiB source
				state.Args

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
	app.SubCommands = append(app.SubCommands, &boundariesCmd)

	nocommentFlags := flag.NewFlagSet("nocomment", flag.ContinueOnError)
	flagNoCommentOut := nocommentFlags.String("out", "-", "output file name")
	nocommentCmd := cli.Command{Name: "nocomment", Flags: nocommentFlags,
		FlagConfigs: []cli.FlagConfig{
			{Name: "out", Short: "o"},
		},
		Summary: "replace comments with spaces",
		Usage:   "nocomment [flags] [file.sql ...]",
		Description: `Write the source with line comments (a line whose first non-space
character starts "--") and block comments ("/*" up to "*/", possibly spanning
lines) replaced by spaces of the same length. Line breaks are preserved, so
byte offsets and line numbers stay valid. Comment markers inside string
literals and quoted identifiers are left untouched. Reads stdin if no file
(or "-") is given.`,
		Exec: func(ctx context.Context, state *cli.State) error {
			paths := state.Args
			if len(paths) == 0 {
				paths = []string{"-"}
			}
			for _, path := range paths {
				src, mode, err := readSource(path, state.Stdin)
				if err != nil {
					return err
				}
				out, err := plsql.NoComment(src)
				if err != nil {
					if path == "-" {
						return err
					}
					return fmt.Errorf("%s: %w", path, err)
				}
				if *flagNoCommentOut == "" || *flagNoCommentOut == "-" {
					_, err = state.Stdout.Write(out)
				} else {
					err = os.WriteFile(*flagNoCommentOut, out, mode)
				}
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	app.SubCommands = append(app.SubCommands, &nocommentCmd)

	if err := cli.Parse(&app, os.Args[1:]); err != nil {
		cli.PrintHelp(os.Stderr, &app)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return cli.Run(ctx, &app, nil)
}

// readSource returns the contents and permission bits of path, or reads
// stdin when path is "-".
func readSource(path string, stdin io.Reader) ([]byte, os.FileMode, error) {
	if path == "-" {
		src, err := io.ReadAll(stdin)
		return src, 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return src, fi.Mode().Perm(), nil
}
