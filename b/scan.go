// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"fmt"
	"strings"
)

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c == '#' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\f', '\v':
		return true
	}
	return false
}

// nextWord returns the next identifier word at or after pos, skipping
// whitespace. If the next non-space byte is not an identifier character
// (e.g. ';', '(', '/') an empty word is returned with start=end=pos;
// at end of input word is empty and start=end=len(m).
func nextWord(m []byte, pos int) (string, int, int) {
	i, n := pos, len(m)
	for i < n && isSpace(m[i]) {
		i++
	}
	if i >= n || !isIdentStart(m[i]) {
		if i >= n {
			return "", n, n
		}
		return "", i, i
	}
	start := i
	for i < n && isIdentChar(m[i]) {
		i++
	}
	return string(m[start:i]), start, i
}

// findSemi returns the offset of the next ';' at parenthesis depth 0,
// searching from pos.
func findSemi(m []byte, pos int) (int, bool) {
	depth := 0
	for i := pos; i < len(m); i++ {
		switch m[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// classify reports whether the subprogram header starting at pos (just
// after its name) is terminated by ';' (a forward or spec declaration)
// or reaches an IS/AS keyword (a definition). For a definition it returns
// the offset of the IS/AS keyword, for a declaration the offset of the
// terminating ';'.
func classify(m []byte, pos int) (isDef bool, at int, err error) {
	depth := 0
	i := pos
	for i < len(m) {
		c := m[i]
		switch {
		case c == '(':
			depth++
			i++
		case c == ')':
			if depth > 0 {
				depth--
			}
			i++
		case isIdentStart(c):
			w, ws, we := nextWord(m, i)
			if depth == 0 && (strings.EqualFold(w, "is") || strings.EqualFold(w, "as")) {
				return true, ws, nil
			}
			i = we
		case c == ';':
			if depth == 0 {
				return false, i, nil
			}
			i++
		default:
			i++
		}
	}
	return false, len(m), fmt.Errorf("byte offset %d: unterminated subprogram header", pos)
}
