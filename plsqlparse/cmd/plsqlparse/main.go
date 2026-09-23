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
	"encoding/json"
	"flag"
	"fmt"
	"os"

	plsql "db_igfb/plsqlparse/plsql"
)

type outObj struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Begin     int    `json:"begin"`
	End       int    `json:"end"`
	BeginLine int    `json:"begin_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func main() {
	withName := flag.Bool("name", false, "include the object name (lowercase) in the output")
	withLine := flag.Bool("line", false, "include 1-based begin_line/end_line in the output")
	indent := flag.Bool("indent", false, "pretty-print the JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: plsqlparse [flags] file.sql ...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	out := make([]outObj, 0, 64)
	for _, path := range flag.Args() {
		src, err := os.ReadFile(path)
		if err != nil {
			fail(err)
		}
		objs, err := plsql.Parse(src)
		if err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
		for _, o := range objs {
			oo := outObj{Type: o.Type, Begin: o.Begin, End: o.End}
			if *withName {
				oo.Name = o.Name
			}
			if *withLine {
				oo.BeginLine = lineOf(src, o.Begin)
				oo.EndLine = lineOf(src, o.End)
			}
			out = append(out, oo)
		}
	}
	var b []byte
	var err error
	if *indent {
		b, err = json.MarshalIndent(out, "", "  ")
	} else {
		b, err = json.Marshal(out)
	}
	if err != nil {
		fail(err)
	}
	os.Stdout.Write(append(b, '\n'))
}

func lineOf(src []byte, off int) int {
	return 1 + bytes.Count(src[:off], []byte{'\n'})
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "plsqlparse:", err)
	os.Exit(1)
}
