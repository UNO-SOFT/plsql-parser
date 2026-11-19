// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"bytes"
	"fmt"
	"io"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

var LegacyEncoding = charmap.ISO8859_2

func NoComment(src []byte) ([]byte, error) {
	return Mask(src, 0)
}

// Mask returns a copy of src of the same length in which line comments
// (-- up to end of line), block comments (/* ... */), string literals
// ('...' with ” escaping) are replaced by spaces.
// Newlines are preserved, so every byte offset computed on
// the result refers to the same position in the original input, and
// line numbers stay valid. Only keywords outside comments, strings and
// quoted identifiers are visible to keyword scanning on the result.
func Mask(src []byte, maskStringLit byte) ([]byte, error) {
	// Validate source is UTF-8
	if _, err := io.Copy(
		io.Discard,
		(&encoding.Decoder{Transformer: encoding.UTF8Validator}).
			Reader(bytes.NewReader(src)),
	); err != nil {
		var err error
		if src, err = LegacyEncoding.NewDecoder().Bytes(src); err != nil {
			return src, err
		}
	}
	out := append([]byte(nil), src...)
	i, n := 0, len(src)
	for i < n {
		switch c := src[i]; {
		case c == '-' && i+1 < n && src[i+1] == '-':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for i < n && src[i] != '\n' && src[i] != '\r' {
				out[i] = ' '
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			start := i
			out[i], out[i+1] = ' ', ' '
			i += 2
			closed := false
			for i < n {
				if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i += 2
					closed = true
					break
				}
				if src[i] != '\n' && src[i] != '\r' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated /* comment", start)
			}
		case c == '\'':
			start := i
			i++
			closed := false
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' { // escaped quote ('')
						if maskStringLit != 0 {
							out[i], out[i+1] = maskStringLit, maskStringLit
						}
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				if src[i] != '\n' && src[i] != '\r' {
					if maskStringLit != 0 {
						out[i] = maskStringLit
					}
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated string literal", start)
			}

		default:
			i++
		}
	}
	return out, nil
}
