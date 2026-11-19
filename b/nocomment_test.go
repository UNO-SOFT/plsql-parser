// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"bytes"
	"testing"
)

func TestNoComment(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"empty":                       {"", ""},
		"no comment":                  {"SELECT 1 FROM dual;\n", "SELECT 1 FROM dual;\n"},
		"line comment":                {"  -- bc\nd\n", "       \nd\n"},
		"line comment at EOF":         {"-- bc", "     "},
		"indented line comment":       {"\t-- x\n", "\t    \n"},
		"block comment":               {"a /* b\nc */ d", "a     \n     d"},
		"block comment at EOF":        {"a/*x*/", "a     "},
		"block comment at line start": {"/* a */\nb", "       \nb"},
		"mixed": {
			"  -- lead\nx := 1; /* mid */ -- tail\n/* a\nb */\n",
			"         \nx := 1;                  \n    \n    \n",
		},
		"crlf": {"-- x\r\ny\r\n", "    \r\ny\r\n"},
		"dash": {"a - b -- c", "a - b     "},
		"string": {
			"x := '-- not a comment';\n-- real\n",
			"x := '-- not a comment';\n       \n",
		},
		"string with block marker": {
			"x := '/* a */';",
			"x := '/* a */';",
		},
		"multi-line string with comment line": {
			"x := 'a\n-- not a comment\nb';\n-- yes\n",
			"x := 'a\n-- not a comment\nb';\n      \n",
		},
		"escaped quote": {
			"x := 'it''s -- x';",
			"x := 'it''s -- x';",
		},
		"mismatched quote in comment": {
			"a'x'b --'c', 'd, 'c'",
			"a'x'b               ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := NoComment([]byte(tc.src))
			if err != nil {
				t.Fatalf("NoComment(%q): %v", tc.src, err)
			}
			if string(got) != tc.want {
				t.Fatalf("NoComment(%q)[%d] = %q[%d], want %q[%d]", tc.src, len(tc.src), got, len(got), tc.want, len(tc.want))
			}
			for i, c := range []byte(tc.src) {
				if (c == '\n' || c == '\r') && got[i] != c {
					t.Fatalf("line break at byte %d changed: %q -> %q", i, c, got[i])
				}
			}
		})
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
