// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import "fmt"

// NoComment returns a copy of src of the same length in which whole-line
// comments and block comments are replaced by spaces.
//
// A line comment is a line whose first non-space character starts "--"; the
// rest of that line (including its indentation) is blanked, but its line
// terminator is kept. A trailing "-- ..." after code is not a line comment
// and is left untouched. A block comment is "/*" up to the next "*/" and may
// span lines; everything inside it is blanked except the line terminators.
//
// Line terminators are preserved, so the result has the same number of lines
// as the input and every byte offset computed on the result refers to the
// same position in the original source. Comment markers inside string
// literals ('...' with ” escaping) and quoted identifiers ("..." with ""
// escaping) do not start comments and are left untouched. Input may use any
// single-byte encoding (e.g. ISO-8859-2); all offsets are byte offsets.
func NoComment(src []byte) ([]byte, error) {
	out := append([]byte(nil), src...)
	i, n := 0, len(src)
	for i < n {
		// A line comment starts a line: only indentation may precede "--".
		if i == 0 || src[i-1] == '\n' || src[i-1] == '\r' {
			j := i
			for j < n && (src[j] == ' ' || src[j] == '\t' || src[j] == '\f' || src[j] == '\v') {
				j++
			}
			if j+1 < n && src[j] == '-' && src[j+1] == '-' {
				for ; i < n && src[i] != '\r' && src[i] != '\n'; i++ {
					out[i] = ' '
				}
				continue
			}
		}
		switch c := src[i]; {
		case c == '/' && i+1 < n && src[i+1] == '*':
			start := i
			closed := false
			for i < n {
				if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i += 2
					closed = true
					break
				}
				if src[i] != '\r' && src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated /* comment", start)
			}
		case c == '\'' || c == '"':
			end := skipQuoted(src, i, c)
			if end < 0 {
				what := "string literal"
				if c == '"' {
					what = "quoted identifier"
				}
				return nil, fmt.Errorf("byte offset %d: unterminated %s", i, what)
			}
			i = end
		default:
			i++
		}
	}
	return out, nil
}

// skipQuoted returns the offset just past the string literal or quoted
// identifier opened at pos by the quote byte q, or -1 if it is not closed
// before the end of src. A doubled quote (qq) is an escaped quote and does
// not close the literal.
func skipQuoted(src []byte, pos int, q byte) int {
	i, n := pos+1, len(src)
	for i < n {
		if src[i] == q {
			if i+1 < n && src[i+1] == q {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return -1
}
