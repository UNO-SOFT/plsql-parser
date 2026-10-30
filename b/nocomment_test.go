// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoComment(t *testing.T) {
	sp := func(n int) string { return strings.Repeat(" ", n) }
	for _, tc := range []struct{ name, src, want string }{
		{"empty", "", ""},
		{"no comment", "SELECT 1 FROM dual;\n", "SELECT 1 FROM dual;\n"},
		{"line comment", "  -- bc\nd\n", "       \nd\n"},
		{"line comment at EOF", "-- bc", "     "},
		{"indented line comment", "\t-- x\n", "     \n"},
		{
			"trailing comment kept",
			"a := 1; -- not a line comment\n",
			"a := 1; -- not a line comment\n",
		},
		{"block comment", "a /* b\nc */ d", "a     \n     d"},
		{"block comment at EOF", "a/*x*/", "a     "},
		{"block comment at line start", "/* a */\nb", "       \nb"},
		{
			"mixed",
			"  -- lead\nx := 1; /* mid */ -- tail\n/* a\nb */\n",
			sp(9) + "\nx := 1; " + sp(9) + " -- tail\n" + sp(4) + "\n" + sp(4) + "\n",
		},
		{"crlf", "-- x\r\ny\r\n", "    \r\ny\r\n"},
		{"dash", "a - b -- c", "a - b -- c"},
		{
			"string",
			"x := '-- not a comment';\n-- real\n",
			"x := '-- not a comment';\n" + sp(7) + "\n",
		},
		{"string with block marker", "x := '/* a */';", "x := '/* a */';"},
		{
			"multi-line string with comment line",
			"x := 'a\n-- not a comment\nb';\n-- yes\n",
			"x := 'a\n-- not a comment\nb';\n" + sp(6) + "\n",
		},
		{"escaped quote", "x := 'it''s -- x';", "x := 'it''s -- x';"},
		{"quoted identifier", `"a--b"`, `"a--b"`},
		{"quoted identifier with block marker", `"a/*b*/"`, `"a/*b*/"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NoComment([]byte(tc.src))
			if err != nil {
				t.Fatalf("NoComment(%q): %v", tc.src, err)
			}
			if string(got) != tc.want {
				t.Fatalf("NoComment(%q) = %q, want %q", tc.src, got, tc.want)
			}
			if len(got) != len(tc.src) {
				t.Fatalf("length changed: %d -> %d", len(tc.src), len(got))
			}
			for i, c := range []byte(tc.src) {
				if (c == '\n' || c == '\r') && got[i] != c {
					t.Fatalf("line break at byte %d changed: %q -> %q", i, c, got[i])
				}
			}
		})
	}
}

func TestNoCommentErrors(t *testing.T) {
	for _, src := range []string{"/* x", "x := 'abc", `x := "abc`} {
		if _, err := NoComment([]byte(src)); err == nil {
			t.Fatalf("NoComment(%q) must fail", src)
		}
	}
}

func TestNoCommentIdempotent(t *testing.T) {
	src := []byte("CREATE OR REPLACE PROCEDURE p IS -- c\nBEGIN /* x\ny */ NULL; END p;\n")
	once, err := NoComment(src)
	if err != nil {
		t.Fatalf("NoComment: %v", err)
	}
	twice, err := NoComment(once)
	if err != nil {
		t.Fatalf("NoComment: %v", err)
	}
	if !bytes.Equal(once, twice) {
		t.Fatalf("NoComment is not idempotent:\n%q\n%q", once, twice)
	}
}
