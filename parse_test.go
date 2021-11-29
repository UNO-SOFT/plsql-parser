// Copyright 2018 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package plsqlparser_test

import (
	"testing"

	plsqlparser "github.com/UNO-SOFT/plsql-parser"
)

func TestParseExampleTest(t *testing.T) {
	p, err := plsqlparser.ParseToConvertMap(`BEGIN INSERT INTO tbl SELECT * FROM Tbl2; END;`)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(p)
}
