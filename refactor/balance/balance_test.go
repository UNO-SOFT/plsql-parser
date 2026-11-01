// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package balance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(name string) string { return filepath.Join("testdata", name) }
func TestLiteralMaskAndCounters(t *testing.T) {
	raw, e := os.ReadFile(fixture("balance_valid.sql"))
	if e != nil {
		t.Fatal(e)
	}
	residual := 2
	valid := Analyze(string(raw), residual)
	if !valid.Passed || valid.Counts["IF"] != 1 || valid.Counts["CASE_EXPR"] != 1 || valid.Counts["residual"] != 2 {
		t.Fatal(valid)
	}
	invalid, e := os.ReadFile(fixture("balance_invalid.sql"))
	if e != nil {
		t.Fatal(e)
	}
	bad := Analyze(string(invalid), residual)
	if bad.Passed || bad.Checks["IF == ENDIF"] {
		t.Fatal("invalid fixture passed", bad)
	}
	if got := MaskLiterals("x := 'END IF'' BEGIN';\n"); got != "x :=                 ;\n" {
		t.Errorf("mask mismatch: %q", got)
	}
	q := MaskLiterals("q'[ END IF ]' BEGIN \n")
	if strings.Contains(q, "IF") || !strings.Contains(q, "BEGIN") {
		t.Errorf("q literal was counted: %q", q)
	}
}

// The Go test binary doubles as a fake parser, keeping go test independent of Python.
func TestMain(m *testing.M) {
	if os.Getenv("PLSQL_REFACTOR_FAKE_PARSER") == "1" {
		if len(os.Args) != 3 || os.Args[1] != "nocomment" {
			os.Exit(2)
		}
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			os.Exit(2)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestParserAndPrestripped(t *testing.T) {
	t.Setenv("PLSQL_REFACTOR_FAKE_PARSER", "1")
	residual := 2
	report, err := CreateReport([]string{fixture("balance_valid.sql"), fixture("balance_invalid.sql")}, residual)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Files[0].Passed || report.Files[1].Passed || report.Delta == nil {
		t.Fatal(report)
	}
	stripped, err := Commentless(fixture("balance_valid.sql"))
	if err != nil || !strings.Contains(stripped, "BEGIN") {
		t.Fatal(err)
	}
}
