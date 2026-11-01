// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package splice

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(name string) string { return filepath.Join("testdata", name) }
func read(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if e := os.WriteFile(path, data, 0644); e != nil {
		t.Fatal(e)
	}
}

func TestAllOperationsAndPreserveBytes(t *testing.T) {
	dir := t.TempDir()
	source := []byte("one\ntwo\nthree\nfour\nfive\n")
	ref := filepath.Join(dir, "ref.sql")
	write(t, ref, []byte("egy\nnew V\nextra Árvíztűrő\n"))
	spec := Spec{"file_ref": "ref.sql", "hunks": []any{
		map[string]any{"op": "insert", "from": 0, "lines": []any{"header"}},
		map[string]any{"op": "replace", "from": 2, "to": 3, "lines": []any{map[string]any{"extract": []any{2, 3}}}},
		map[string]any{"op": "delete", "from": 5, "to": 5},
		map[string]any{"op": "insert", "from": 4, "lines": []any{"added"}},
	}}
	actual, err := PatchBytes(spec, filepath.Join(dir, "spec.json"), source)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("header\none\nnew V\nextra Árvíztűrő\nfour\nadded\n")
	if !bytes.Equal(actual, expected) {
		t.Fatalf("patch mismatch\n%q\n%q", actual, expected)
	}
	// A C1 control byte is NOT normalized, and a last non-LF line remains intact.
	spec = Spec{"hunks": []any{map[string]any{"op": "replace", "from": 2, "lines": []any{"done"}}}}
	actual, err = PatchBytes(spec, filepath.Join(dir, "spec.json"), []byte("A\xc2\x84 B\nlast"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, []byte("A\xc2\x84 B\ndone\n")) {
		t.Fatal("modified untouched control bytes", actual)
	}
}
func TestRejectOverlapAndAuditedSource(t *testing.T) {
	spec := Spec{"hunks": []any{map[string]any{"op": "delete", "from": 1, "to": 3}, map[string]any{"op": "replace", "from": 2, "to": 4, "lines": []any{"bad"}}}}
	_, err := PatchBytes(spec, "spec.json", []byte("a\nb\nc\nd\n"))
	if err == nil || !strings.Contains(err.Error(), "Overlapping") {
		t.Fatalf("missing overlap error: %v", err)
	}
	spec["hunks"] = []any{map[string]any{"op": "replace", "from": 2, "to": 2, "old_lines": []any{"WRONG"}, "lines": []any{"new"}}}
	_, err = PatchBytes(spec, "spec.json", []byte("a\nb\nc\nd\n"))
	if err == nil || !strings.Contains(err.Error(), "source lines differ") {
		t.Fatalf("bad audit accepted: %v", err)
	}
	spec["hunks"] = []any{map[string]any{"op": "insert", "from": 0, "lines": []any{"first"}}, map[string]any{"op": "insert", "from": 0, "lines": []any{"second"}}}
	_, err = PatchBytes(spec, "spec.json", []byte("a\n"))
	if err == nil {
		t.Fatal("duplicate insertion accepted")
	}
}
func TestDiffRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "from-diff.json")
	spec, err := SpecFromDiff(fixture("diff.normal"), fixture("new.sql"), path)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := PatchBytes(spec, path, read(t, fixture("old.sql")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, read(t, fixture("new.sql"))) {
		t.Fatal("diff round-trip diverged")
	}
	checks, err := SpecChecks(spec, "post")
	if err != nil || len(checks) != 2 {
		t.Fatal("bad post checks", err)
	}
}
func TestAtomicApplyAndRollback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.sql")
	old := []byte("old\n")
	write(t, target, old)
	specPath := filepath.Join(dir, "spec.json")
	spec := Spec{"hunks": []any{map[string]any{"op": "replace", "from": 1, "lines": []any{"new"}}},
		"pre_checks":  []any{map[string]any{"type": "md5", "expect": fmt.Sprintf("%x", md5.Sum(old))}},
		"post_checks": []any{map[string]any{"type": "md5", "expect": fmt.Sprintf("%x", md5.Sum([]byte("new\n")))}, map[string]any{"type": "wc-l", "expect": 1}},
	}
	if err := ApplySpec(spec, specPath, target); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(t, target), []byte("new\n")) || !bytes.Equal(read(t, target+".pre-apply.bak"), old) {
		t.Fatal("bad result or backup")
	}
	if err := ApplySpec(spec, specPath, target); err == nil || !strings.Contains(err.Error(), "Refusing to overwrite different") {
		t.Fatal("modified target accepted", err)
	}
	write(t, target, old)
	spec["post_checks"] = []any{map[string]any{"type": "md5", "expect": strings.Repeat("0", 32)}}
	if err := ApplySpec(spec, specPath, target); err == nil || !strings.Contains(err.Error(), "post checks FAILED") {
		t.Fatal("bad postconditions accepted", err)
	}
	if !bytes.Equal(read(t, target), old) {
		t.Fatal("failed apply was not restored")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file.sql.pre-apply.bak", target); err != nil {
		t.Fatal(err)
	}
	if err := ApplySpec(spec, specPath, target); err == nil {
		t.Fatal("symlinked target accepted")
	}
}
