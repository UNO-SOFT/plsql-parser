// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Package findrepeats discovers repeating PL/SQL line families, ignoring
// commented-out code, and independently checks whole-file uniqueness.
package refactor

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var singleString = regexp.MustCompile(`'(?:''|[^'])*'`)

func normalizeLine(line, mode string) string {
	if mode == "skeleton" {
		line = singleString.ReplaceAllString(line, "'<literal>'")
	}
	return strings.Join(strings.Fields(line), " ")
}

func ReadLines(path string) ([][]byte, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if bytes.Contains(data, []byte{'\r'}) {
		return nil, nil, fmt.Errorf("expected LF-only input; a CR byte was detected")
	}
	raw := bytes.Split(data, []byte{'\n'})
	if len(data) == 0 {
		raw = [][]byte{}
	} else if data[len(data)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	texts := make([]string, len(raw))
	for i, line := range raw {
		text, e := requireUTF8(line)
		if e != nil {
			return nil, nil, e
		}
		texts[i] = text
	}
	return raw, texts, nil
}

// maskComments replaces both inline/block comments with spaces; strings and
// Oracle q-literals are preserved so comment markers inside them stay inert.
func maskComments(text string) string {
	source := []rune(text)
	out := append([]rune(nil), source...)
	n := len(source)
	pairs := map[rune]rune{'[': ']', '(': ')', '{': '}', '<': '>'}
	for i := 0; i < n; {
		start := i
		if i+1 < n && source[i] == '-' && source[i+1] == '-' {
			i += 2
			for i < n && source[i] != '\n' {
				i++
			}
			blank(out, start, i)
			continue
		}
		if i+1 < n && source[i] == '/' && source[i+1] == '*' {
			i += 2
			for i+1 < n && !(source[i] == '*' && source[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			} else {
				i = n
			}
			blank(out, start, i)
			continue
		}
		if (source[i] == 'q' || source[i] == 'Q') && i+2 < n && source[i+1] == '\'' && (i == 0 || !isWord(source[i-1])) {
			closer := source[i+2]
			if paired, ok := pairs[closer]; ok {
				closer = paired
			}
			found := false
			for j := i + 3; j+1 < n; j++ {
				if source[j] == closer && source[j+1] == '\'' {
					i = j + 2
					found = true
					break
				}
			}
			if found {
				continue
			}
		}
		if source[i] == '\'' || source[i] == '"' {
			quote := source[i]
			i++
			for i < n {
				if source[i] == quote {
					i++
					if i < n && source[i] == quote {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		i++
	}
	return string(out)
}

type Sample struct {
	Text string `json:"text"`
	Line int    `json:"line"`
}
type Variation struct {
	Samples []Sample `json:"samples"`
	Offset  int      `json:"offset"`
}
type Divergence struct {
	Samples     []Sample `json:"samples"`
	OffsetAfter int      `json:"offset_after"`
}
type Family struct {
	NextDivergence *Divergence `json:"next_divergence"`
	Mode           string      `json:"mode"`
	Starts         []int       `json:"starts"`
	Preview        []string    `json:"preview"`
	VariableLines  []Variation `json:"variable_lines"`
	Length         int         `json:"length"`
	Occurrences    int         `json:"occurrences"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func allEqualAt(normalized []string, starts []int, shift int) bool {
	first := ""
	for i, start := range starts {
		index := start + shift
		if index < 0 || index >= len(normalized) || normalized[index] == "" {
			return false
		}
		if i == 0 {
			first = normalized[index]
		} else if first != normalized[index] {
			return false
		}
	}
	return true
}
func makeSignature(starts []int, length int, mode string) string {
	var b strings.Builder
	b.WriteString(mode)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(length))
	for _, s := range starts {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(s))
	}
	return b.String()
}
func allFamilies(lines, original []string, low, high, minimum int, mode string) []Family {
	seg := lines[low-1 : high]
	normalized := make([]string, len(seg))
	for i, line := range seg {
		normalized[i] = normalizeLine(line, mode)
	}
	size := len(normalized)
	grams := map[string][]int{}
	keys := []string{} // retain discovery order, as Python's dictionaries do
	for start := 0; start+minimum <= size; start++ {
		window := normalized[start : start+minimum]
		if !allNonblank(window) {
			continue
		}
		key := strings.Join(window, "\x00")
		if _, exists := grams[key]; !exists {
			keys = append(keys, key)
		}
		grams[key] = append(grams[key], start)
	}
	candidates := []Family{}
	seen := map[string]int{}
	for _, key := range keys {
		offsets := grams[key]
		if len(offsets) < 2 {
			continue
		}
		begin := 0
		for allEqualAt(normalized, offsets, -begin-1) {
			begin++
		}
		end := minimum
		for allEqualAt(normalized, offsets, end) {
			end++
		}
		abs := make([]int, len(offsets))
		seenStarts := map[int]bool{}
		duplicate := false
		for i, p := range offsets {
			abs[i] = low + p - begin
			if seenStarts[abs[i]] {
				duplicate = true
			}
			seenStarts[abs[i]] = true
		}
		if duplicate {
			continue
		}
		length := end + begin
		sig := makeSignature(abs, length, mode)
		if _, exists := seen[sig]; exists {
			continue
		}
		seen[sig] = len(candidates)
		candidates = append(candidates, Family{Mode: mode, Starts: abs, Length: length, Occurrences: len(abs)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Length != candidates[j].Length {
			return candidates[i].Length > candidates[j].Length
		}
		return candidates[i].Occurrences > candidates[j].Occurrences
	})
	kept := []Family{}
	for _, family := range candidates {
		contained := false
		for _, older := range kept {
			if older.Occurrences != family.Occurrences {
				continue
			}
			inside := true
			for i, b := range family.Starts {
				a := older.Starts[i]
				if !(a <= b && b+family.Length <= a+older.Length) {
					inside = false
					break
				}
			}
			if inside {
				contained = true
				break
			}
		}
		if !contained {
			kept = append(kept, family)
		}
	}
	for index := range kept {
		f := &kept[index]
		f.Preview = []string{}
		for j := 0; j < min(f.Length, 4); j++ {
			f.Preview = append(f.Preview, strings.TrimSpace(original[f.Starts[0]-1+j]))
		}
		f.VariableLines = []Variation{}
		for relative := 0; relative < f.Length; relative++ {
			values := []string{}
			different := false
			for _, start := range f.Starts {
				value := normalizeLine(original[start-1+relative], "exact")
				if len(values) > 0 && value != values[0] {
					different = true
				}
				values = append(values, value)
			}
			if !different {
				continue
			}
			samples := []Sample{}
			for i, start := range f.Starts {
				if len(samples) == 3 {
					break
				}
				samples = append(samples, Sample{Line: start + relative, Text: truncate(values[i], 230)})
			}
			f.VariableLines = append(f.VariableLines, Variation{Offset: relative + 1, Samples: samples})
			if len(f.VariableLines) == 8 {
				break
			}
		}
		for step := 0; step < 8; step++ {
			following := []Sample{}
			unique := map[string]bool{}
			for _, start := range f.Starts {
				at := start - 1 + f.Length + step
				if at >= len(lines) {
					continue
				}
				value := truncate(strings.TrimSpace(original[at]), 180)
				following = append(following, Sample{Line: start + f.Length + step, Text: value})
				unique[value] = true
			}
			if len(following) < len(f.Starts) || len(unique) > 1 {
				f.NextDivergence = &Divergence{OffsetAfter: step + 1, Samples: following[:min(3, len(following))]}
				break
			}
		}
	}
	return kept
}
func allNonblank(rows []string) bool {
	for _, s := range rows {
		if s == "" {
			return false
		}
	}
	return true
}
func ScanFamilies(path string, low, high, minimum int, mode string) ([]Family, error) {
	_, original, err := ReadLines(path)
	if err != nil {
		return nil, err
	}
	if !(1 <= low && low <= high && high <= len(original)) {
		return nil, fmt.Errorf("--range must satisfy 1 <= A <= B <= %d", len(original))
	}
	if minimum < 2 {
		return nil, fmt.Errorf("--min-len must be >= 2")
	}
	if mode != "both" && mode != "exact" && mode != "skeleton" {
		return nil, fmt.Errorf("invalid mode %s", mode)
	}
	active := strings.Split(maskComments(strings.Join(original, "\n")), "\n")
	families := []Family{}
	modes := []string{mode}
	if mode == "both" {
		modes = []string{"exact", "skeleton"}
	}
	for _, m := range modes {
		families = append(families, allFamilies(active, original, low, high, minimum, m)...)
	}
	exact := map[string]bool{}
	for _, f := range families {
		if f.Mode == "exact" {
			exact[makeSignature(f.Starts, f.Length, "")] = true
		}
	}
	filtered := []Family{}
	for _, f := range families {
		if f.Mode == "skeleton" && exact[makeSignature(f.Starts, f.Length, "")] {
			continue
		}
		filtered = append(filtered, f)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		scoreA, scoreB := a.Occurrences*a.Length, b.Occurrences*b.Length
		if scoreA != scoreB {
			return scoreA > scoreB
		}
		if a.Length != b.Length {
			return a.Length > b.Length
		}
		return a.Occurrences > b.Occurrences
	})
	return filtered, nil
}

type Range struct {
	From int `json:"from"`
	To   int `json:"to"`
}
type UniqueResult struct {
	File   string  `json:"file"`
	Mode   string  `json:"mode"`
	Query  []int   `json:"query"`
	Ranges []Range `json:"ranges"`
	Count  int     `json:"count"`
}

func Unique(path string, start, end int, normalized bool) (UniqueResult, error) {
	raw, texts, err := ReadLines(path)
	if err != nil {
		return UniqueResult{}, err
	}
	if !(1 <= start && start <= end && end <= len(raw)) {
		return UniqueResult{}, fmt.Errorf("unique range must satisfy 1 <= A <= B <= %d", len(raw))
	}
	rows := make([]string, len(raw))
	for i, r := range raw {
		if normalized {
			rows[i] = normalizeLine(texts[i], "exact")
		} else {
			rows[i] = string(r)
		}
	}
	size := end - start + 1
	needle := rows[start-1 : end]
	ranges := []Range{}
	for i := 0; i+size <= len(rows); i++ {
		matched := true
		for k := 0; k < size; k++ {
			if rows[i+k] != needle[k] {
				matched = false
				break
			}
		}
		if matched {
			ranges = append(ranges, Range{From: i + 1, To: i + size})
		}
	}
	mode := "byte"
	if normalized {
		mode = "normalized"
	}
	return UniqueResult{File: path, Query: []int{start, end}, Mode: mode, Count: len(ranges), Ranges: ranges}, nil
}
