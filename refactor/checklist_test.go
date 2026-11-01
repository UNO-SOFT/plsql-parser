package refactor

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllCheckTypesAndFailures(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("START\nKÜLDÉS\nEXIT\n")
	if err := os.WriteFile(filepath.Join(dir, "live.sql"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	checks := []Check{
		{"type": "md5", "file": "live.sql", "expect": fmt.Sprintf("%x", md5.Sum(raw))},
		{"type": "wc-l", "file": "live.sql", "expect": 3},
		{"type": "count", "file": "live.sql", "substring": "KÜLDÉS", "expect": 1},
		{"type": "ends-with", "file": "live.sql", "bytes": "EXIT\n"},
		{"type": "ends-with", "file": "live.sql", "hex": "455849540a"},
		{"type": "no-crlf", "file": "live.sql"},
		{"type": "cmd", "command": "printf 'ready\\n'", "stdout_contains": "ready"},
		// Substring matching must test the ENTIRE stdout, not the 1200-char excerpt.
		{"type": "cmd", "command": "printf '%01201dREADY' 0", "stdout_contains": "READY"},
	}
	verdict, ok := evaluateChecks(checks, dir, "")
	if !ok {
		t.Fatal(verdict)
	}
	for _, r := range verdict {
		if !r.Pass {
			t.Fatal("failed", r)
		}
		if r.Type == "cmd" {
			a := r.Actual.(map[string]any)
			if _, has := a["_stdout_full_match"]; has {
				t.Fatal("private field leaked")
			}
		}
	}
	bad := append(checks, Check{"type": "wc-l", "file": "live.sql", "expect": 999})
	results, ok := evaluateChecks(bad, dir, "")
	if ok || results[len(results)-1].Pass {
		t.Fatal("false pass")
	}
	if _, passed := evaluateChecks(nil, dir, ""); passed {
		t.Fatal("empty checks passed")
	}
	// Explicitly exercise {file} substitution and single-quote-safe shell escaping.
	name := "O'Brien.sql"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res, ok := evaluateChecks([]Check{{"type": "cmd", "command": "wc -l < {file}", "stdout_contains": "1"}}, dir, filepath.Join(dir, name))
	if !ok {
		t.Fatal(res)
	}
	if strings.Contains(fmt.Sprint(res), "_stdout_full_match") {
		t.Fatal("private detail leaked")
	}
}
