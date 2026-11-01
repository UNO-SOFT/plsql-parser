// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Package balance counts PL/SQL structure tokens after comment removal and
// literal masking. It does not validate SQL syntax or semantics.
package refactor

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode"

	plsql "github.com/UNO-SOFT/plsql-parser/b"
)

var BalanceCounterKeys = []string{"BEGIN", "IF", "ENDIF", "LOOP", "ENDLOOP", "CASE", "ENDCASE", "CASE_EXPR", "END_total", "END_block", "residual"}

// maskLiterals blanks single-quoted SQL strings, quoted identifiers and
// q-literals while preserving LF offsets.
func maskLiterals(text string) string {
	input := []rune(text)
	out := append([]rune(nil), input...)
	n := len(input)
	pairs := map[rune]rune{'[': ']', '(': ')', '{': '}', '<': '>'}
	for i := 0; i < n; {
		start := i
		if (input[i] == 'q' || input[i] == 'Q') && i+2 < n && input[i+1] == '\'' && (i == 0 || !isWord(input[i-1])) {
			close := input[i+2]
			if paired, ok := pairs[close]; ok {
				close = paired
			}
			at := -1
			for j := i + 3; j+1 < n; j++ {
				if input[j] == close && input[j+1] == '\'' {
					at = j
					break
				}
			}
			if at >= 0 {
				i = at + 2
				maskSegment(out, start, i)
				continue
			}
		}
		if input[i] != '\'' && input[i] != '"' {
			i++
			continue
		}
		quote := input[i]
		i++
		for i < n {
			if input[i] == quote {
				i++
				if i < n && input[i] == quote {
					i++
					continue
				}
				break
			}
			i++
		}
		maskSegment(out, start, i)
	}
	return string(out)
}

type token struct {
	text       string
	start, end int
}

func tokens(text string) ([]token, []rune) {
	s := []rune(text)
	result := []token{}
	for i := 0; i < len(s); {
		if !isWord(s[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isWord(s[j]) {
			j++
		}
		result = append(result, token{strings.ToUpper(string(s[i:j])), i, j})
		i = j
	}
	return result, s
}
func joinedByWhitespace(first, second token, chars []rune) bool {
	if first.end == second.start {
		return false
	}
	for _, r := range chars[first.end:second.start] {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

type FileResult struct {
	Counts           map[string]int  `json:"counts"`
	Checks           map[string]bool `json:"checks"`
	File             string          `json:"file"`
	ExpectedResidual int             `json:"expected_residual"`
	Lines            int             `json:"lines"`
	Passed           bool            `json:"passed"`
}

func analyze(text string, expectedResidual int) FileResult {
	ts, chars := tokens(maskLiterals(text))
	raw := map[string]int{}
	pairs := map[string]int{}
	for i, t := range ts {
		raw[t.text]++
		if t.text != "END" || i+1 >= len(ts) {
			continue
		}
		next := ts[i+1]
		if (next.text == "IF" || next.text == "LOOP" || next.text == "CASE") && joinedByWhitespace(t, next, chars) {
			pairs[next.text]++
		}
	}
	begin := raw["BEGIN"]
	endif, endloop, endcase := pairs["IF"], pairs["LOOP"], pairs["CASE"]
	endtotal, caseRaw := raw["END"], raw["CASE"]
	caseExpr := caseRaw - 2*endcase
	counter := map[string]int{
		"BEGIN":     begin,
		"IF":        raw["IF"] - endif,
		"ENDIF":     endif,
		"LOOP":      raw["LOOP"] - endloop,
		"ENDLOOP":   endloop,
		"CASE":      caseRaw - endcase,
		"ENDCASE":   endcase,
		"CASE_EXPR": caseExpr,
		"END_total": endtotal,
		"END_block": endtotal - endif - endloop - endcase - caseExpr,
		"residual":  endtotal - begin - endif - endloop - endcase - caseExpr,
	}
	checks := map[string]bool{
		"IF == ENDIF":                counter["IF"] == endif,
		"LOOP == ENDLOOP":            counter["LOOP"] == endloop,
		"CASE expression count >= 0": caseExpr >= 0,
		"END accounting":             endtotal == begin+endif+endloop+endcase+caseExpr+counter["residual"],
	}
	if expectedResidual != 0 {
		checks[fmt.Sprintf("residual == %d", expectedResidual)] = counter["residual"] == expectedResidual
	}
	passed := true
	for _, ok := range checks {
		if !ok {
			passed = false
		}
	}
	return FileResult{Counts: counter, Checks: checks, Passed: passed, ExpectedResidual: expectedResidual}
}

func commentless(path string) (string, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	data, err := plsql.NoComment(original)
	if err != nil {
		return "", err
	}
	if bytes.Count(data, []byte{'\n'}) != bytes.Count(original, []byte{'\n'}) {
		return "", fmt.Errorf("nocomment changed line count; refusing to analyze")
	}
	return requireUTF8(data)
}

type Report struct {
	Delta   map[string]int `json:"delta"`
	Warning string         `json:"warning"`
	Files   []FileResult   `json:"files"`
	Passed  bool           `json:"passed"`
}

func CreateBalanceReport(paths []string, expected int) (Report, error) {
	results := []FileResult{}
	passed := true
	for _, path := range paths {
		raw, err := commentless(path)
		if err != nil {
			return Report{}, err
		}
		result := analyze(raw, expected)
		result.File = path
		result.Lines = strings.Count(raw, "\n")
		results = append(results, result)
		if !result.Passed {
			passed = false
		}
	}
	var delta map[string]int
	if len(results) == 2 {
		delta = map[string]int{}
		for _, key := range BalanceCounterKeys {
			delta[key] = results[1].Counts[key] - results[0].Counts[key]
		}
	}
	return Report{Passed: passed, Files: results, Delta: delta, Warning: "Token accounting is not an Oracle syntax/semantics validation"}, nil
}
