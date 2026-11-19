// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"bytes"
	// "log"
)

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
		// log.Printf("%d. %c", i, src[i])
		switch c := src[i]; {
		case c == '-' && i+1 < n && src[i+1] == '-':
			end := bytes.IndexByte(src[i+2:], '\n')
			if end < 0 {
				end = len(src) - 1 - i - 2
			}
			for range 2 + end + 1 {
				if src[i] != '\t' && src[i] != '\r' && src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}

		case c == '/' && i+1 < n && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				end = len(src) - 1 - i - 2 - 2
			}
			for range 2 + end + 2 {
				if src[i] != '\r' && src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}

		case c == '\'':
			if end := bytes.IndexByte(src[i+1:], '\''); end < 0 {
				i = len(src)
			} else {
				i += 1 + end + 1
			}
		default:
			i++
		}
	}
	return out, nil
}
