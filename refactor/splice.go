// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Package splice implements line-numbered, byte-preserving SQL patching.
package refactor

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Spec map[string]any

type Hunk struct {
	Operation string
	Lines     []any
	OldLines  []any
	From      int
	To        int
	Index     int
	HasOld    bool
}

func LoadSpec(path string) (Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec Spec
	if err := decodeJSON(raw, &spec); err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, fmt.Errorf("spec requires a 'hunks' array")
	}
	if _, ok := spec["hunks"].([]any); !ok {
		return nil, fmt.Errorf("spec requires a 'hunks' array")
	}
	return spec, nil
}
func ResolveRef(spec Spec, specPath string) (string, error) {
	value, has := spec["file_ref"]
	if !has || value == nil {
		return "", nil
	}
	ref, ok := value.(string)
	if !ok || ref == "" {
		return "", fmt.Errorf("file_ref must be a nonempty path")
	}
	return resolvePath(filepath.Dir(specPath), ref)
}
func checkHunks(spec Spec, srcLen int) ([]Hunk, error) {
	raw, ok := spec["hunks"].([]any)
	if !ok {
		return nil, fmt.Errorf("spec requires a 'hunks' array")
	}
	hunks := make([]Hunk, 0, len(raw))
	seenInsert := map[int]bool{}
	for i, v := range raw {
		h, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each hunk must be an object")
		}
		op, _ := h["op"].(string)
		if op != "replace" && op != "insert" && op != "delete" {
			return nil, fmt.Errorf("hunk %d: unsupported op %q", i+1, op)
		}
		start, ok := numberInt(h["from"])
		if !ok {
			return nil, fmt.Errorf("hunk %d: from must be an integer", i+1)
		}
		end := start
		if v, has := h["to"]; has {
			var valid bool
			end, valid = numberInt(v)
			if !valid {
				return nil, fmt.Errorf("hunk %d: to must be an integer", i+1)
			}
		}
		lines := []any{}
		if v, has := h["lines"]; has {
			var valid bool
			lines, valid = v.([]any)
			if !valid {
				return nil, fmt.Errorf("hunk %d: lines must be an array", i+1)
			}
		}
		var old []any
		hasOld := false
		if v, has := h["old_lines"]; has {
			old, ok = v.([]any)
			if !ok {
				return nil, fmt.Errorf("hunk %d: old_lines must be an array", i+1)
			}
			hasOld = true
		}
		if op == "insert" {
			if start < 0 || start > srcLen || end != start {
				return nil, fmt.Errorf("hunk %d: insert after line 0..%d, to == from", i+1, srcLen)
			}
			if seenInsert[start] {
				return nil, fmt.Errorf("hunk %d: duplicate insertion after line %d", i+1, start)
			}
			seenInsert[start] = true
			if len(lines) == 0 {
				return nil, fmt.Errorf("hunk %d: insert requires lines", i+1)
			}
		} else {
			if start < 1 || start > end || end > srcLen {
				return nil, fmt.Errorf("hunk %d: range %d..%d outside 1..%d", i+1, start, end, srcLen)
			}
			if op == "replace" && len(lines) == 0 {
				return nil, fmt.Errorf("hunk %d: replace requires lines", i+1)
			}
			if op == "delete" && len(lines) > 0 {
				return nil, fmt.Errorf("hunk %d: delete must not include replacement lines", i+1)
			}
		}
		hunks = append(hunks, Hunk{Operation: op, From: start, To: end, Lines: lines, OldLines: old, HasOld: hasOld, Index: i + 1})
	}
	ranges := []Hunk{}
	for _, h := range hunks {
		if h.Operation != "insert" {
			ranges = append(ranges, h)
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].From < ranges[j].From })
	for i := 1; i < len(ranges); i++ {
		prev, next := ranges[i-1], ranges[i]
		if next.From <= prev.To {
			return nil, fmt.Errorf("overlapping line ranges (hunks %d, %d)", prev.Index, next.Index)
		}
	}
	for after := range seenInsert {
		for _, h := range ranges {
			if h.From <= after && after < h.To {
				return nil, fmt.Errorf("insert after line %d overlaps hunk %d [%d..%d]", after, h.Index, h.From, h.To)
			}
		}
	}
	sort.SliceStable(hunks, func(i, j int) bool {
		if hunks[i].From != hunks[j].From {
			return hunks[i].From > hunks[j].From
		}
		return hunks[i].Operation == "insert" && hunks[j].Operation != "insert"
	})
	return hunks, nil
}
func materializeEntries(entries []any, refLines [][]byte) ([][]byte, error) {
	result := [][]byte{}
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			if strings.ContainsAny(v, "\r\n") {
				return nil, fmt.Errorf("literal line must not contain a newline; use one entry per line")
			}
			result = append(result, append([]byte(v), '\n'))
		case map[string]any:
			if len(v) != 1 {
				return nil, fmt.Errorf("unsupported line entry %v", entry)
			}
			raw, has := v["extract"]
			if !has {
				return nil, fmt.Errorf("unsupported line entry %v", entry)
			}
			pair, ok := raw.([]any)
			if !ok {
				if ints, isInts := raw.([]int); isInts && len(ints) == 2 {
					pair = []any{ints[0], ints[1]}
					ok = true
				}
			}
			if !ok || len(pair) != 2 {
				return nil, fmt.Errorf("invalid extract range %v", raw)
			}
			first, ok1 := numberInt(pair[0])
			last, ok2 := numberInt(pair[1])
			if !ok1 || !ok2 || first < 1 || first > last {
				return nil, fmt.Errorf("invalid extract range %v", raw)
			}
			if refLines == nil {
				return nil, fmt.Errorf("extracted lines require spec.file_ref")
			}
			if last > len(refLines) {
				return nil, fmt.Errorf("extract %v outside file_ref (1..%d)", raw, len(refLines))
			}
			result = append(result, refLines[first-1:last]...)
		default:
			return nil, fmt.Errorf("unsupported line entry %v", entry)
		}
	}
	return result, nil
}
func PatchBytes(spec Spec, specPath string, original []byte) ([]byte, error) {
	lines := byteLines(original)
	ordered, err := checkHunks(spec, len(lines))
	if err != nil {
		return nil, err
	}
	ref, err := ResolveRef(spec, specPath)
	if err != nil {
		return nil, err
	}
	var refLines [][]byte
	if ref != "" {
		data, e := os.ReadFile(ref)
		if e != nil {
			return nil, e
		}
		refLines = byteLines(data)
	}
	for _, h := range ordered {
		if h.Operation == "insert" {
			extra, e := materializeEntries(h.Lines, refLines)
			if e != nil {
				return nil, e
			}
			lines = append(lines[:h.From], append(extra, lines[h.From:]...)...)
			continue
		}
		if h.HasOld {
			if len(h.OldLines) != h.To-h.From+1 {
				return nil, fmt.Errorf("hunk at %d: old_lines count mismatch", h.From)
			}
			for i := 0; i < len(h.OldLines); i++ {
				wanted, ok := h.OldLines[i].(string)
				if !ok {
					return nil, fmt.Errorf("hunk at %d..%d: source lines differ from audited diff", h.From, h.To)
				}
				actual, err := requireUTF8(stripLF(lines[h.From-1+i]))
				if err != nil {
					return nil, err
				}
				if actual != wanted {
					return nil, fmt.Errorf("hunk at %d..%d: source lines differ from audited diff", h.From, h.To)
				}
			}
		}
		replacement := [][]byte{}
		if h.Operation == "replace" {
			replacement, err = materializeEntries(h.Lines, refLines)
			if err != nil {
				return nil, err
			}
		}
		lines = append(lines[:h.From-1], append(replacement, lines[h.To:]...)...)
	}
	return joinLines(lines), nil
}
func specChecks(spec Spec, phase string) ([]Check, error) {
	raw, has := spec[phase+"_checks"]
	if !has {
		raw = spec[phase]
	}
	if raw == nil {
		return []Check{}, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s_checks must be a list of checklist check objects", phase)
	}
	result := make([]Check, 0, len(arr))
	for _, obj := range arr {
		m, ok := obj.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s_checks must be a list of checklist check objects", phase)
		}
		result = append(result, m)
	}
	return result, nil
}
func AssertChecks(spec Spec, phase, specPath, target string) error {
	checks, err := specChecks(spec, phase)
	if err != nil {
		return err
	}
	if len(checks) == 0 {
		if phase == "post" {
			return fmt.Errorf("no post_checks in spec: verify requires postconditions")
		}
		return nil
	}
	results, ok := evaluateChecks(checks, filepath.Dir(specPath), target)
	for _, r := range results {
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		value := r.Actual
		if r.Error != "" {
			value = r.Error
		}
		fmt.Printf("%s %s %s: %v\n", status, phase, r.Name, value)
	}
	if !ok {
		return fmt.Errorf("%s checks FAILED", phase)
	}
	return nil
}
func atomicBytes(dest string, content []byte, mode os.FileMode) error {
	parent := filepath.Dir(dest)
	temp, err := os.CreateTemp(parent, "."+filepath.Base(dest)+".*.tmp")
	if err != nil {
		return err
	}
	path := temp.Name()
	defer os.Remove(path)
	if _, err = temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Chmod(mode & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(path, dest); err != nil {
		return err
	}
	if dir, err := os.Open(parent); err == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}
func ApplySpec(spec Spec, specPath, target string) (err error) {
	info, e := os.Lstat(target)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("apply requires an existing regular, non-symlink TARGET")
	}
	before, e := os.ReadFile(target)
	if e != nil {
		return e
	}
	mode := info.Mode()
	backup := target + ".pre-apply.bak"
	if bi, be := os.Lstat(backup); be == nil {
		if bi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlinked backup %s", backup)
		}
		old, e := os.ReadFile(backup)
		if e != nil {
			return e
		}
		if !bytes.Equal(before, old) {
			return fmt.Errorf("refusing to overwrite different pre-apply backup: %s", backup)
		}
	} else if !os.IsNotExist(be) {
		return be
	}
	if err := atomicBytes(backup, before, mode); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			contents, e := os.ReadFile(backup)
			if e == nil {
				e = atomicBytes(target, contents, mode)
			}
			if e != nil {
				fmt.Fprintf(os.Stderr, "CRITICAL: restore failed after apply error: %v\n", e)
			} else {
				fmt.Fprintf(os.Stderr, "RESTORED target from %s\n", backup)
			}
		}
	}()
	if err = AssertChecks(spec, "pre", specPath, target); err != nil {
		return err
	}
	var after []byte
	after, err = PatchBytes(spec, specPath, before)
	if err != nil {
		return err
	}
	var work string
	work, err = os.MkdirTemp(filepath.Dir(target), "plsql-splice-")
	if err != nil {
		return err
	}
	candidate := filepath.Join(work, filepath.Base(target))
	if err = os.WriteFile(candidate, after, 0600); err == nil {
		err = AssertChecks(spec, "post", specPath, candidate)
	}
	if rmErr := os.RemoveAll(work); rmErr != nil && err == nil {
		err = rmErr
	}
	if err != nil {
		return err
	}
	if err = atomicBytes(target, after, mode); err != nil {
		return err
	}
	if err = AssertChecks(spec, "post", specPath, target); err != nil {
		return err
	}
	fmt.Printf("APPLIED atomically: %s (backup: %s)\n", target, backup)
	return nil
}

var diffHeader = regexp.MustCompile(`^(\d+)(?:,(\d+))?([acd])(\d+)(?:,(\d+))?$`)

func SpecFromDiff(diff, newfile, outSpec string) (Spec, error) {
	raw, err := os.ReadFile(diff)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(raw, []byte{'\r'}) {
		return nil, fmt.Errorf("normal-format diff must be LF-only")
	}
	txt, err := requireUTF8(raw)
	if err != nil {
		return nil, err
	}
	rows := strings.Split(txt, "\n")
	if len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	newData, err := os.ReadFile(newfile)
	if err != nil {
		return nil, err
	}
	newLines := byteLines(newData)
	hunks := []any{}
	for idx := 0; idx < len(rows); {
		match := diffHeader.FindStringSubmatch(rows[idx])
		if match == nil {
			return nil, fmt.Errorf("diff line %d: expected normal-format hunk header: %.100q", idx+1, rows[idx])
		}
		idx++
		a, _ := strconv.Atoi(match[1])
		b := a
		if match[2] != "" {
			b, _ = strconv.Atoi(match[2])
		}
		p, _ := strconv.Atoi(match[4])
		q := p
		if match[5] != "" {
			q, _ = strconv.Atoi(match[5])
		}
		op := match[3]
		oldCount := 0
		newCount := 0
		if op == "c" || op == "d" {
			oldCount = b - a + 1
		}
		if op == "c" || op == "a" {
			newCount = q - p + 1
		}
		old := []any{}
		fresh := []string{}
		for k := 0; k < oldCount; k++ {
			if idx >= len(rows) || !strings.HasPrefix(rows[idx], "< ") {
				return nil, fmt.Errorf("diff line %d: expected an old-side '< ' line", idx+1)
			}
			old = append(old, rows[idx][2:])
			idx++
		}
		if op == "c" {
			if idx >= len(rows) || rows[idx] != "---" {
				return nil, fmt.Errorf("diff line %d: missing '---' separator", idx+1)
			}
			idx++
		}
		for k := 0; k < newCount; k++ {
			if idx >= len(rows) || !strings.HasPrefix(rows[idx], "> ") {
				return nil, fmt.Errorf("diff line %d: expected a new-side '> ' line", idx+1)
			}
			fresh = append(fresh, rows[idx][2:])
			idx++
		}
		if idx < len(rows) && rows[idx] == `\ No newline at end of file` {
			return nil, fmt.Errorf("no-newline diff markers are not supported; use LF-terminated SQL")
		}
		if newCount > 0 {
			if p < 1 || q > len(newLines) {
				return nil, fmt.Errorf("diff hunk's new lines %d..%d out of NEWFILE", p, q)
			}
			for k, s := range fresh {
				actual, e := requireUTF8(stripLF(newLines[p-1+k]))
				if e != nil {
					return nil, e
				}
				if actual != s {
					return nil, fmt.Errorf("diff's > lines %d..%d do not match NEWFILE bytes", p, q)
				}
			}
		}
		mapped := map[string]string{"c": "replace", "a": "insert", "d": "delete"}
		h := map[string]any{"op": mapped[op], "from": a}
		if op != "a" {
			h["to"] = b
			h["old_lines"] = old
		}
		if newCount > 0 {
			h["lines"] = []any{map[string]any{"extract": []int{p, q}}}
		}
		hunks = append(hunks, h)
	}
	newAbs, err := AbsolutePath(newfile)
	if err != nil {
		return nil, err
	}
	specAbs, err := AbsolutePath(outSpec)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(filepath.Dir(specAbs), newAbs)
	if err != nil {
		return nil, err
	}
	digest := md5.Sum(newData)
	spec := Spec{"file_ref": rel, "hunks": hunks, "post_checks": []any{
		map[string]any{"type": "md5", "expect": fmt.Sprintf("%x", digest)},
		map[string]any{"type": "wc-l", "expect": bytes.Count(newData, []byte{'\n'})},
	}}
	if _, err = checkHunks(spec, 1000000000000); err != nil {
		return nil, err
	}
	if err = WriteJSON(outSpec, spec); err != nil {
		return nil, err
	}
	fmt.Printf("GENERATED %s: %d hunks; extract source %s\n", outSpec, len(hunks), rel)
	return spec, nil
}
