// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package findrepeats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(name string) string { return filepath.Join("testdata", name) }
func TestScanAndUnique(t *testing.T) {
	families, err := Scan(fixture("repeats.sql"), 1, 28, 4, "skeleton")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range families {
		if reflect.DeepEqual(f.Starts, []int{2, 9, 22}) && f.Occurrences == 3 {
			found = true
			if len(f.VariableLines) == 0 || f.NextDivergence == nil {
				t.Fatal("missing divergences")
			}
		}
		for _, start := range f.Starts {
			if start == 16 {
				t.Fatal("comment was included")
			}
		}
	}
	if !found {
		t.Fatalf("missing 3-site skeleton family: %v", families)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "u.sql")
	if err := os.WriteFile(path, []byte("A\n B\nC\nX\nA\n  B\nC\n"), 0644); err != nil {
		t.Fatal(err)
	}
	exact, err := Unique(path, 1, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	norm, err := Unique(path, 1, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Count != 1 || norm.Count != 2 || norm.Ranges[1].From != 5 {
		t.Fatal(exact, norm)
	}
}
func TestMaskCommentsAndRejectCR(t *testing.T) {
	text := "q'[ this /* isn't a comment */ ]'\n-- BEGIN\nx /* END IF */ y\n"
	cleaned := MaskComments(text)
	if !strings.Contains(cleaned, "isn't a comment") || strings.Contains(cleaned, "-- BEGIN") || strings.Contains(cleaned, "/* END IF */") {
		t.Errorf("bad masking: %q", cleaned)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cr.sql")
	if err := os.WriteFile(path, []byte("a\r\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadLines(path); err == nil {
		t.Fatal("CRLF accepted")
	}
}
