# Line splice and acceptance JSON

Spec format (`changes.json`):

```json
{
  "file_ref": "../new.sql",
  "pre_checks": [
    {"type": "md5", "expect": "<old md5>"},
    {"type": "wc-l", "expect": 100}
  ],
  "hunks": [
    {"op": "replace", "from": 10, "to": 12,
     "lines": [{"extract": [11, 13]}]},
    {"op": "insert", "from": 50, "lines": ["  -- a literal new line"]},
    {"op": "delete", "from": 99, "to": 100}
  ],
  "post_checks": [
    {"type": "md5", "expect": "<new md5>"},
    {"type": "wc-l", "expect": 99}
  ]
}
```

- Line indices refer to **the original** file for every hunk. Replacements and deletions use inclusive ends; insertion is immediately **after** `from` (zero means before first line). Apply order is bottom-up.
- `extract` retrieves the **full bytes** of `file_ref` lines; the file path is resolved relative to the spec JSON. Literal strings have no newline characters; one LF is appended per literal. Existing lines should be extracted, not retyped. UTF-8-only spec, no CRLF.
- The optional `old_lines` array checks the original text on each replaced/deleted source line (generated automatically by `spec-from-diff`); this is a safety assertion. The output of `spec-from-diff` has post md5 and wc-l but cannot automatically know the original file hash — add `pre_checks` before any `apply`.
- `patch` is pure with respect to input; `apply` requires a regular file, makes `.pre-apply.bak`, validates pre/post conditions and atomically commits or rolls back. `verify` requires `post_checks`.
- Do not put source data from the wrong version in `file_ref`. A spec generated from `diff OLD NEW` references NEW lines.

Standalone `checklist.py` accepts `[{"type": "...", ...}]` or `{"checks": [...]}`. Check types: `md5` (`file`, `expect` hex), `wc-l` (`file`, `expect` number), `count` (`file`, `substring`, `expect`), `ends-with` (`file`, `bytes` UTF-8 or `hex`), `no-crlf` (`file`), and `cmd` (`command`, `expect_exit` default 0, optional `stdout_contains`). All checks print one line and an overall **ALL PASS** or **FAILED**; `--json result.json` stores a structured verdict.

`balance.py` supports `--json` to print its results to stdout, `--delta` to compare two files, `--expected-residual` to assert package-end accounting, and `--prestripped` when files are already nocomment output. An END token in `END IF` is not an ordinary block END; keep `END_total` and `END_block` distinct.

`findrepeats.py` supports `scan FILE [--range A B] [--min-len N] [--mode exact|skeleton|both] [--top N] [--match regex] [--json]` and `unique FILE A B [--norm] [--json]`. `scan` is for discovery; **only** `unique` checks the complete file for whole-byte matching. Do not use a `scan` family as an automatic replacement plan.

## Go counterparts (identical JSON input)

The Go module is in `go/` and requires Go 1.21+. From that directory:

```sh
go run ./cmd/splice patch /path/changes.json /path/old.sql /path/candidate.sql
go run ./cmd/splice apply /path/changes.json /path/target.sql
go run ./cmd/splice verify /path/changes.json /path/candidate.sql
go run ./cmd/splice spec-from-diff /path/change.diff /path/new.sql /path/generated.json
go run ./cmd/balance --prestripped --delta --json /path/old.nocomment.sql /path/new.nocomment.sql
go run ./cmd/findrepeats scan /path/old.sql --range 1 1000 --json
go run ./cmd/checklist /path/acceptance.json --json /path/verdict.json
```

The Go implementation preserves the Python spec/checklist semantics, including
`file_ref` and checklist paths relative to the JSON file, `old_lines` audit,
original 1-based line indices, byte-preserving `extract`, atomic apply/restore,
and the same pass/fail verdict structure. It does not require Python at runtime.
Use [go/README.md](../go/README.md) for build and test commands. The Go
`findrepeats --match` flag uses RE2 rather than Python's `re` engine.
