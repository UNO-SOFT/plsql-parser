// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package plsql

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) []Object {
	t.Helper()
	objs, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkSpans(t, src, objs)
	return objs
}

// checkSpans verifies that every span is in range, ends just past a ';'
// and begins at the keyword it claims (CREATE for top level objects,
// PROCEDURE/FUNCTION for nested ones, judging by whether the span starts
// with "create").
func checkSpans(t *testing.T, src string, objs []Object) {
	t.Helper()
	for _, o := range objs {
		if o.Begin < 0 || o.End > len(src) || o.Begin >= o.End {
			t.Fatalf("%s %s: bad span %d..%d (len %d)", o.Type, o.Name, o.Begin, o.End, len(src))
		}
		if src[o.End-1] != ';' {
			t.Fatalf("%s %s: span does not end with ';': %q", o.Type, o.Name, src[o.Begin:o.End])
		}
		pre := strings.ToLower(src[o.Begin:min(len(src), o.Begin+6)])
		if pre != "create" && pre != "proced" && pre != "functi" {
			t.Fatalf("%s %s: span begins with %q, want CREATE/PROCEDURE/FUNCTION: %q",
				o.Type, o.Name, src[o.Begin:o.Begin+12], src[o.Begin:min(len(src), o.Begin+40)])
		}
	}
}

func wantObjects(t *testing.T, got []Object, want []Object) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d objects %+v, want %d: %+v", len(got), got, len(want), want)
	}
	for i, g := range got {
		w := want[i]
		if g.Type != w.Type || g.Name != w.Name || g.Begin != w.Begin || g.End != w.End {
			t.Fatalf("object %d: got %+v, want %+v", i, g, w)
		}
	}
}

func TestTopLevelFunctionAndProcedure(t *testing.T) {
	src := `ALTER SESSION SET CURRENT_SCHEMA = BRUNO_OWNER;
CREATE OR REPLACE
PROCEDURE DB_I_GDPR(p_tran_azon IN INTEGER, p_tipus IN VARCHAR2) IS
  v_db PLS_INTEGER := CASE WHEN 1=1 THEN 1 ELSE 2 END;
BEGIN
  /*
  PROCEDURE gone IS BEGIN NULL; END gone;
  */
  CASE p_tipus
    WHEN 'A' THEN NULL;
    ELSE NULL;
  END CASE;
  IF v_db IS NULL THEN NULL; END IF;
  FOR i IN 1..3 LOOP NULL; END LOOP;
  DBMS_OUTPUT.put_line('it''s a -- test /* here */ END;');
  DECLARE
    w NUMBER;
  BEGIN
    w := 1;
  END;
END DB_I_GDPR;
/
`
	objs := mustParse(t, src)
	wantObjects(t, objs, []Object{
		{Type: TypeProcedure, Name: "db_i_gdpr", Begin: strings.Index(src, "CREATE"), End: len(src) - 3},
	})
	// the CREATE prefix must be part of the span
	if src[objs[0].Begin:objs[0].Begin+6] != "CREATE" {
		t.Fatalf("begin must point at CREATE, got %q", src[objs[0].Begin:objs[0].Begin+6])
	}
	if !strings.Contains(src[objs[0].Begin:objs[0].End], "END DB_I_GDPR;") {
		t.Fatal("span must contain the final END")
	}
}

func TestTopLevelFunction(t *testing.T) {
	src := "CREATE OR REPLACE FUNCTION DB_I_TIMESTAMP(p_timestamp IN VARCHAR2) return timestamp is\nBEGIN\n  RETURN(TO_TIMESTAMP(p_timestamp, 'YYYY-MM-DD\"T\"HH24:MI:SS\".\"FF3'));\nEXCEPTION WHEN OTHERS THEN RETURN(NULL);\nEND DB_I_TIMESTAMP;\n"
	objs := mustParse(t, src)
	wantObjects(t, objs, []Object{
		{Type: TypeFunction, Name: "db_i_timestamp", Begin: 0, End: len(src) - 1},
	})
}

func TestPackageSpec(t *testing.T) {
	src := `CREATE OR REPLACE PACKAGE db_inca AS
  FUNCTION mehet RETURN VARCHAR2;
  FUNCTION g_konv(p_jelleg IN VARCHAR2, p_hataly IN DATE) RETURN VARCHAR2 DETERMINISTIC;/*
  commented out
  */
  PROCEDURE ktv(p_tran_azon IN INTEGER,
                p_tipus IN VARCHAR2);
  TYPE t IS TABLE OF VARCHAR2(4);
  c CONSTANT NUMBER := 1;
  g_x NUMBER := CASE WHEN 1=1 THEN 1 ELSE 2 END;
END db_inca;
`
	objs := mustParse(t, src)
	wantObjects(t, objs, []Object{
		{Type: TypePackage, Name: "db_inca", Begin: 0, End: len(src) - 1},
		{Type: TypeFunction, Name: "mehet", Begin: strings.Index(src, "FUNCTION mehet"), End: strings.Index(src, "mehet RETURN VARCHAR2;") + len("mehet RETURN VARCHAR2;")},
		{Type: TypeFunction, Name: "g_konv", Begin: strings.Index(src, "FUNCTION g_konv"), End: strings.Index(src, "DETERMINISTIC;") + len("DETERMINISTIC;")},
		{Type: TypeProcedure, Name: "ktv", Begin: strings.Index(src, "PROCEDURE ktv"), End: strings.Index(src, "p_tipus IN VARCHAR2);") + len("p_tipus IN VARCHAR2);")},
	})
}

func TestPackageBodyNested(t *testing.T) {
	src := `CREATE OR REPLACE
package body DB_i_kozos is
  --g_elozo_szerep VARCHAR2(1) := NULL; -- don't open a string here
  g_rang_u rang_tab_typ; g_rang_t rang_tab_typ;

  /*
  PROCEDURE szerep_urit IS
  BEGIN
    g_elozo_szerep := NULL;
  END szerep_urit;
  */

  FUNCTION rang_feltolt(p_tip IN VARCHAR2) RETURN rang_tab_typ IS
    v_rang rang_tab_typ := CASE p_tip WHEN 'T' THEN g_rang_t ELSE g_rang_u END;
    FUNCTION tul_e(x VARCHAR2) RETURN VARCHAR2 IS
    BEGIN
      RETURN x;
    END tul_e;
  BEGIN
    IF NOT v_rang.EXISTS('9') THEN
      FOR sor IN (SELECT CASE WHEN 1=1 THEN 'a' END ord FROM dual) LOOP
        v_rang(sor.ord) := 1;
      END LOOP;
    END IF;
    RETURN(v_rang);
  END rang_feltolt;

  PROCEDURE gone;

  FUNCTION mehet RETURN VARCHAR2 IS
    valasz VARCHAR2(1);
  BEGIN
    RETURN valasz;
  END mehet;
BEGIN
  rang_feltolt;
END DB_i_kozos;
`
	objs := mustParse(t, src)
	wantObjects(t, objs, []Object{
		{Type: TypePackageBody, Name: "db_i_kozos", Begin: 0, End: len(src) - 1},
		{Type: TypeFunction, Name: "rang_feltolt", Begin: strings.Index(src, "FUNCTION rang_feltolt"), End: strings.Index(src, "END rang_feltolt;") + len("END rang_feltolt;")},
		{Type: TypeFunction, Name: "tul_e", Begin: strings.Index(src, "FUNCTION tul_e"), End: strings.Index(src, "END tul_e;") + len("END tul_e;")},
		{Type: TypeProcedure, Name: "gone", Begin: strings.Index(src, "PROCEDURE gone;"), End: strings.Index(src, "PROCEDURE gone;") + len("PROCEDURE gone;")},
		{Type: TypeFunction, Name: "mehet", Begin: strings.Index(src, "FUNCTION mehet RETURN"), End: strings.Index(src, "END mehet;") + len("END mehet;")},
	})
	if !strings.Contains(src[objs[0].Begin:objs[0].End], "END DB_i_kozos;") {
		t.Fatal("package body span must contain the final END")
	}
}

func TestPackageBodyWithoutInitBlock(t *testing.T) {
	src := `CREATE OR REPLACE PACKAGE BODY p2 IS
  PROCEDURE q IS BEGIN NULL; END q;
END p2;`
	objs := mustParse(t, src)
	wantObjects(t, objs, []Object{
		{Type: TypePackageBody, Name: "p2", Begin: 0, End: len(src)},
		{Type: TypeProcedure, Name: "q", Begin: strings.Index(src, "PROCEDURE q IS"), End: strings.Index(src, "END q;") + len("END q;")},
	})
}

func TestUnterminatedInput(t *testing.T) {
	if _, err := Parse([]byte("x := 'abc")); err == nil {
		t.Fatal("unterminated string literal must fail")
	}
	if _, err := Parse([]byte("/* unclosed")); err == nil {
		t.Fatal("unterminated block comment must fail")
	}
	if _, err := Parse([]byte("CREATE OR REPLACE PROCEDURE p IS BEGIN NULL; END p")); err == nil {
		t.Fatal("missing final ';' must fail")
	}
	if _, err := Parse([]byte("CREATE OR REPLACE PROCEDURE p IS v NUMBER := 1;")); err == nil {
		t.Fatal("missing BEGIN must fail")
	}
}

func TestJSONShape(t *testing.T) {
	src := "CREATE OR REPLACE FUNCTION f RETURN NUMBER IS BEGIN RETURN 1; END f;"
	objs, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	b, err := json.Marshal(objs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `[{"Type":"FUNCTION","Name":"f","Begin":0,"End":` + strconv.Itoa(len(src)) + `}]`
	if string(b) != want {
		t.Fatalf("JSON: got %s, want %s", b, want)
	}
}

var promptRe = regexp.MustCompile(`(?m)^PROMPT Creating (FUNCTION|PROCEDURE|PACKAGE BODY|PACKAGE) (\S+) \.\.\.`)

func TestRealFiles(t *testing.T) {
	promptType := map[string]ObjectType{
		"FUNCTION": TypeFunction, "PROCEDURE": TypeProcedure,
		"PACKAGE": TypePackage, "PACKAGE BODY": TypePackageBody,
	}
	dis, err := os.ReadDir("testdata")
	if len(dis) == 0 {
		t.Skip(err)
	}
	for _, di := range dis {
		path := filepath.Join("testdata", di.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("%s not found (%v)", path, err)
		}
		t.Run(path, func(t *testing.T) {
			objs, err := Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			var top []Object
			for _, o := range objs {
				checkSpans(t, string(src), []Object{o})
				if o.Begin+6 <= len(src) && strings.EqualFold(string(src[o.Begin:o.Begin+6]), "create") {
					top = append(top, o)
				}
			}
			ms := promptRe.FindAllSubmatch(src, -1)
			if len(ms) != len(top) {
				t.Fatalf("got %d top level objects, want %d PROMPT lines", len(top), len(ms))
			}
			for i, m := range ms {
				typ, ok := promptType[string(m[1])]
				if !ok {
					t.Fatalf("unexpected PROMPT type %s", m[1])
				}
				if top[i].Type != typ || top[i].Name != strings.ToLower(string(m[2])) {
					t.Fatalf("object %d: got {%s %s}, want PROMPT {%s %s}",
						i, top[i].Type, top[i].Name, typ, strings.ToLower(string(m[2])))
				}
			}
			if len(objs) == len(top) {
				t.Fatal("expected nested objects to be reported")
			}
		})
	}
}
