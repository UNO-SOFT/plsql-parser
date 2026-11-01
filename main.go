// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Command plsqlparse parses PL/SQL source files and prints a JSON array
// of {type, begin, end} objects describing the source extent of every
// package, package body, procedure and function found, nested ones
// included. begin/end are byte offsets into the original file: begin
// points at the first byte of the construct (CREATE at top level, the
// PROCEDURE/FUNCTION keyword when nested), end just past the terminating
// ';'. Files may use any single-byte encoding (e.g. ISO-8859-2).
//
// The nocomment subcommand writes the source with its comments replaced by
// spaces of the same length, keeping byte offsets and line numbers intact.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/UNO-SOFT/cli"
	plsql "github.com/UNO-SOFT/plsql-parser/b"
	"github.com/UNO-SOFT/plsql-parser/refactor"
)

type outObj struct {
	plsql.Object
	BeginLine int `json:",omitzero"`
	EndLine   int `json:",omitzero"`
}

func main() {
	if err := Main(); err != nil {
		log.Fatal(err)
	}
}

func Main() error {
	app := cli.Command{Name: "plsql-parser"}

	flags := flag.NewFlagSet("boundaries", flag.ContinueOnError)
	flagBoundariesWithLine := flags.Bool("line", false, "include 1-based begin_line/end_line in the output")
	flagBoundariesIndent := flags.Bool("indent", false, "pretty-print the JSON")
	boundariesCmd := cli.Command{Name: "boundaries", Flags: flags,
		FlagConfigs: []cli.FlagConfig{
			{Name: "line", Short: "l"},
			{Name: "indent", Short: "i"},
		},
		Summary: "print JSON of function boundaries",
		Usage:   "boundaries [flags] file.sql ...",
		Exec: func(ctx context.Context, state *cli.State) error {
			args :=
				// cache in-source line numbers till position
				// this caching results 200ms speedup for an 500kiB source
				state.Args

			type posLine struct{ Pos, Line int }
			posCmp := func(a, b posLine) int { return cmp.Compare(a.Pos, b.Pos) }
			cache := make(map[*[]byte][]posLine)
			lineOf := func(src *[]byte, pos int) int {
				// return 1 + bytes.Count((*src)[:pos], []byte{'\n'})

				i, ok := slices.BinarySearchFunc(cache[src], posLine{Pos: pos}, posCmp)
				if ok {
					return cache[src][i].Line
				} else if i > 0 {
					prev := cache[src][i-1]
					n := prev.Line + bytes.Count((*src)[prev.Pos:pos], []byte{'\n'})
					cache[src] = slices.Insert(cache[src], i, posLine{Pos: pos, Line: n})
					return n
				}
				n := 1 + bytes.Count((*src)[:pos], []byte{'\n'})
				lines := append(make([]posLine, 1, len(cache[src])+1), cache[src]...)
				lines[0] = posLine{Pos: pos, Line: n}
				cache[src] = lines
				return n
			}

			out := make(map[string][]outObj, len(args))
			for _, path := range args {
				src, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				objs, err := plsql.Parse(src)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				out[path] = slices.Grow(out[path], len(objs))
				for _, o := range objs {
					oo := outObj{Object: o}
					if *flagBoundariesWithLine {
						oo.BeginLine = lineOf(&src, o.Begin)
						oo.EndLine = lineOf(&src, o.End)
					}
					out[path] = append(out[path], oo)
				}
				clear(cache)
			}
			var opts []jsontext.Options
			if *flagBoundariesIndent {
				opts = append(opts, jsontext.WithIndent("  "))
			}
			return json.MarshalWrite(os.Stdout, out, opts...)
		},
	}
	app.SubCommands = append(app.SubCommands, &boundariesCmd)

	refactorCmd := &cli.Command{Name: "refactor",
		Summary: "refactoring helpers",
	}
	app.SubCommands = append(app.SubCommands, refactorCmd)

	flags = flag.NewFlagSet("refactor-balance", flag.ContinueOnError)
	refactorBalanceDeltaFlag := flags.Bool("delta", false, "show file2 - file1 counters")
	refactorBalanceJSONFlag := flags.Bool("json", false, "output JSON report")
	refactorBalanceExpectedFlag := flags.Int("expected-residual", 0, "assert residual package END count")
	refactorBalanceCmd := cli.Command{Name: "balance", Flags: flags,
		Summary: "check the BEGIN/END balances",
		Usage:   "balance [--expected-residual N] [--delta] [--json] FILE [FILE2]",
		Exec: func(ctx context.Context, state *cli.State) error {
			report, err := refactor.CreateBalanceReport(state.Args, *refactorBalanceExpectedFlag)
			if err != nil {
				return err
			}
			if *refactorBalanceJSONFlag {
				data, err := refactor.PrettyJSON(report)
				if err != nil {
					return err
				}
				fmt.Print(string(data))
				return nil
			}
			for _, file := range report.Files {
				fmt.Printf("FILE %s (%d lines)\n  ", file.File, file.Lines)
				for n, key := range refactor.BalanceCounterKeys {
					if n > 0 {
						fmt.Print("  ")
					}
					fmt.Printf("%s=%d", key, file.Counts[key])
				}
				fmt.Println()
				keys := []string{}
				for key := range file.Checks {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				for _, key := range keys {
					status := "PASS"
					if !file.Checks[key] {
						status = "FAIL"
					}
					fmt.Printf("  %s %s\n", status, key)
				}
			}
			if *refactorBalanceDeltaFlag && report.Delta != nil {
				fmt.Println("DELTA file2 - file1")
				for _, key := range refactor.BalanceCounterKeys {
					fmt.Printf("  %-10s %+d\n", key, report.Delta[key])
				}
			}
			if report.Passed {
				fmt.Println("ALL PASS")
				return nil
			}
			fmt.Println("FAILED")
			return fmt.Errorf("failed")
		},
	}
	refactorCmd.SubCommands = append(refactorCmd.SubCommands, &refactorBalanceCmd)

	flags = flag.NewFlagSet("checklist", flag.ContinueOnError)
	refactorChecklistJSON := flags.String("json", "", "write verdict JSON to file")
	refactorChecklistCmd := cli.Command{Name: "checklist", Flags: flags,
		Summary: "check",
		Usage:   "checklist CHECKS.json [--json OUT.json]",
		Exec: func(ctx context.Context, state *cli.State) error {
			if len(state.Args) != 1 {
				return fmt.Errorf("needs CHECKS.json")
			}
			results, passed, err := refactor.LoadAndEvaluate(state.Args[0], "")
			if err != nil {
				return err
			}
			refactor.DisplayChecks(results, passed)
			if *refactorChecklistJSON != "" {
				var dest string
				dest, err = refactor.ExpandUser(*refactorChecklistJSON)
				if err == nil {
					err = refactor.WriteJSON(dest, map[string]any{
						"passed":  passed,
						"verdict": map[bool]string{true: "ALL PASS", false: "FAILED"}[passed],
						"results": results,
					})
				}
			}
			return err
		},
	}
	refactorCmd.SubCommands = append(refactorCmd.SubCommands, &refactorChecklistCmd)

	flags = flag.NewFlagSet("rfr-unique", flag.ContinueOnError)
	rfrFlagNorm := flags.Bool("norm", false, "normalize whitespace")
	rfrFlagAsJSON := flags.Bool("json", false, "JSON output")
	rfrUniqueCmd := cli.Command{Name: "unique", Flags: flags,
		Summary: "unique",
		Exec: func(ctx context.Context, state *cli.State) error {
			if len(state.Args) != 3 {
				return fmt.Errorf("need 3 args")
			}
			start, err := strconv.Atoi(state.Args[1])
			if err != nil {
				return err
			}
			end, err := strconv.Atoi(state.Args[2])
			if err != nil {
				return err
			}
			r, err := refactor.Unique(state.Args[0], start, end, *rfrFlagNorm)
			if err != nil {
				return err
			}
			if *rfrFlagAsJSON {
				return refactor.PrintJSON(os.Stdout, r)
			}
			fmt.Printf("%s count=%d query=%d..%d\n", strings.ToUpper(r.Mode), r.Count, start, end)
			parts := []string{}
			for _, interval := range r.Ranges {
				parts = append(parts, fmt.Sprintf("%d..%d", interval.From, interval.To))
			}
			fmt.Println("MATCH RANGES: " + strings.Join(parts, ", "))
			return nil
		},
	}

	flags = flag.NewFlagSet("scan", flag.ContinueOnError)
	rfrScanMinLen := flags.Int("min-len", 4, "minimum sequence length")
	rfrScanMode := flags.String("mode", "both", "exact, skeleton, both")
	rfrScanTop := flags.Int("top", 30, "maximum families to print (0 means all)")
	rfrScanPattern := flags.String("match", "", "regexp filter")
	rfrScanAsJSON := flags.Bool("json", false, "JSON output")
	rfrScanRange := flags.String("range", "", "from:till")
	rfrScanCmd := cli.Command{Name: "scan", Flags: flags,
		Summary: "scan",
		Exec: func(ctx context.Context, state *cli.State) error {
			if len(state.Args) != 1 {
				return fmt.Errorf("needs one arg")
			}
			_, lines, err := refactor.ReadLines(state.Args[0])
			if err != nil {
				return err
			}
			var low, high int64
			if *rfrScanRange == "" {
				low = 1
				high = int64(len(lines))
			} else if a, b, ok := strings.Cut(*rfrScanRange, ":"); !ok {
				return fmt.Errorf("range should be a:b format, got %s", *rfrScanRange)
			} else if low, err = strconv.ParseInt(a, 10, 32); err != nil {
				return err
			} else if high, err = strconv.ParseInt(b, 10, 32); err != nil {
				return err
			}

			families, err := refactor.ScanFamilies(state.Args[0], int(low), int(high), *rfrScanMinLen, *rfrScanMode)
			if err != nil {
				return err
			}
			if *rfrScanPattern != "" {
				re, err := regexp.Compile("(?i)" + *rfrScanPattern)
				if err != nil {
					return err
				}
				filtered := []refactor.Family{}
				for _, fam := range families {
					matched := false
					for j := range fam.Length {
						if re.MatchString(lines[fam.Starts[0]-1+j]) {
							matched = true
							break
						}
					}
					if matched {
						filtered = append(filtered, fam)
					}
				}
				families = filtered
			}
			if top := *rfrScanTop; top > 0 && len(families) > top {
				families = families[:top]
			}
			if *rfrScanAsJSON {
				return refactor.PrintJSON(os.Stdout, map[string]any{
					"file":     state.Args[0],
					"range":    []int64{low, high},
					"families": families,
				})
			}
			fmt.Printf("SCAN %s %d..%d: %d families shown\n", state.Args[0], low, high, len(families))
			truncate := func(s string, n int) string {
				r := []rune(s)
				if len(r) > n {
					return string(r[:n])
				}
				return s
			}

			for i, f := range families {
				starts := make([]string, 0, len(f.Starts))
				for _, s := range f.Starts {
					starts = append(starts, strconv.Itoa(s))
				}
				fmt.Printf("[%d] %s %d occurrences x %d lines: %s\n", i+1, f.Mode, f.Occurrences, f.Length, strings.Join(starts, ", "))
				fmt.Println("    " + strings.Join(f.Preview[:min(2, len(f.Preview))], " / "))
				for _, v := range f.VariableLines[:min(3, len(f.VariableLines))] {
					p := make([]string, 0, len(v.Samples))
					for _, s := range v.Samples {
						p = append(p, fmt.Sprintf("%d:%s", s.Line, truncate(s.Text, 85)))
					}
					fmt.Printf("    VARIABLE at +%d: %s\n", v.Offset-1, strings.Join(p, " | "))
				}
				if f.NextDivergence != nil {
					p := make([]string, 0, len(f.NextDivergence.Samples))
					for _, s := range f.NextDivergence.Samples {
						p = append(p, fmt.Sprintf("%d:%s", s.Line, truncate(s.Text, 80)))
					}
					fmt.Printf("    NEXT DIVERGENCE +%d: %s\n", f.NextDivergence.OffsetAfter, strings.Join(p, " | "))
				}
			}
			return nil
		}}
	refactorFindRepeatsCmd := cli.Command{Name: "findrepeats",
		SubCommands: []*cli.Command{&rfrUniqueCmd, &rfrScanCmd},
	}
	refactorCmd.SubCommands = append(refactorCmd.SubCommands, &refactorFindRepeatsCmd)

	rsplSpecFromDiffCmd := cli.Command{Name: "spec-from-diff",
		Summary: "create SPEC from a diff",
		Usage:   "spec-from-diff DIFF NEW OUT",
		Exec: func(ctx context.Context, state *cli.State) error {
			if len(state.Args) != 3 {
				return fmt.Errorf("needs 3 args (got %d)", len(state.Args))
			}
			_, err := refactor.SpecFromDiff(state.Args[0], state.Args[1], state.Args[2])
			return err
		},
	}

	rsplSpec := func(args []string, want int) (string, refactor.Spec, error) {
		if len(args) != want {
			return "", refactor.Spec{}, fmt.Errorf("needs %d args (got %d)", want, len(args))
		}
		specPath, err := refactor.AbsolutePath(args[0])
		if err != nil {
			return args[0], refactor.Spec{}, err
		}
		spec, err := refactor.LoadSpec(specPath)
		return args[0], spec, err
	}
	rsplPatchCmd := cli.Command{Name: "patch",
		Usage: "patch SPEC SRC OUT",
		Exec: func(ctx context.Context, state *cli.State) error {
			specPath, spec, err := rsplSpec(state.Args, 3)
			if err != nil {
				return err
			}
			src, err := refactor.AbsolutePath(state.Args[1])
			if err != nil {
				return err
			}
			out, err := refactor.AbsolutePath(state.Args[2])
			if err != nil {
				return err
			}
			ref, err := refactor.ResolveRef(spec, specPath)
			if err != nil {
				return err
			}
			if src == out || out == ref {
				return fmt.Errorf("patch OUT must differ from SRC and spec.file_ref")
			}
			content, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			patched, err := refactor.PatchBytes(spec, specPath, content)
			if err != nil {
				return err
			}
			if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				return err
			}
			if err = os.WriteFile(out, patched, 0644); err != nil {
				return err
			}
			fmt.Printf("PATCHED %s\n", out)
			return nil
		},
	}

	rsplVerifyCmd := cli.Command{Name: "verify",
		Usage: "verify SPEC OUT",
		Exec: func(ctx context.Context, state *cli.State) error {
			specPath, spec, err := rsplSpec(state.Args, 2)
			if err != nil {
				return err
			}
			file, err := refactor.AbsolutePath(state.Args[1])
			if err != nil {
				return err
			}
			if err = refactor.AssertChecks(spec, "post", specPath, file); err != nil {
				return err
			}
			fmt.Println("ALL PASS")
			return nil
		},
	}

	rsplApplyCmd := cli.Command{Name: "apply", Usage: "apply SPEC OUT",
		Exec: func(ctx context.Context, state *cli.State) error {
			specPath, spec, err := rsplSpec(state.Args, 2)
			if err != nil {
				return err
			}
			target, err := refactor.AbsolutePath(state.Args[1])
			if err != nil {
				return err
			}
			return refactor.ApplySpec(spec, specPath, target)
		},
	}

	refactorSpliceCmd := cli.Command{Name: "splice",
		Usage: "splice [patch SPEC SRC OUT | apply SPEC TARGET | verify SPEC FILE | spec-from-diff DIFF NEW OUT]",
		SubCommands: []*cli.Command{
			&rsplSpecFromDiffCmd, &rsplPatchCmd,
			&rsplVerifyCmd, &rsplApplyCmd,
		},
	}
	refactorCmd.SubCommands = append(refactorCmd.SubCommands, &refactorSpliceCmd)

	nocommentFlags := flag.NewFlagSet("nocomment", flag.ContinueOnError)
	flagNoCommentOut := nocommentFlags.String("out", "-", "output file name")
	nocommentCmd := cli.Command{Name: "nocomment", Flags: nocommentFlags,
		FlagConfigs: []cli.FlagConfig{
			{Name: "out", Short: "o"},
		},
		Summary: "replace comments with spaces",
		Usage:   "nocomment [flags] [file.sql ...]",
		Description: `Write the source with line comments (a line whose first non-space
character starts "--") and block comments ("/*" up to "*/", possibly spanning
lines) replaced by spaces of the same length. Line breaks are preserved, so
byte offsets and line numbers stay valid. Comment markers inside string
literals and quoted identifiers are left untouched. Reads stdin if no file
(or "-") is given.`,
		Exec: func(ctx context.Context, state *cli.State) error {
			paths := state.Args
			if len(paths) == 0 {
				paths = []string{"-"}
			}
			for _, path := range paths {
				src, mode, err := readSource(path, state.Stdin)
				if err != nil {
					return err
				}
				out, err := plsql.NoComment(src)
				if err != nil {
					if path == "-" {
						return err
					}
					return fmt.Errorf("%s: %w", path, err)
				}
				if *flagNoCommentOut == "" || *flagNoCommentOut == "-" {
					_, err = state.Stdout.Write(out)
				} else {
					err = os.WriteFile(*flagNoCommentOut, out, mode)
				}
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	app.SubCommands = append(app.SubCommands, &nocommentCmd)

	args := make([]string, 0, len(os.Args[1:]))
	for i := 0; i < len(os.Args)-1; i++ {
		a := os.Args[1+i]
		// --range a b  ->  --range a:b
		if (a == "--range" || a == "-range") && i+2 < len(os.Args)-1 {
			args = append(args, "--range", os.Args[1+i]+":"+os.Args[1+i+1])
			i += 2
			continue
		}
		args = append(args, a)
	}

	if err := cli.Parse(&app, args); err != nil {
		cli.PrintHelp(os.Stderr, &app)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return cli.Run(ctx, &app, nil)
}

// readSource returns the contents and permission bits of path, or reads
// stdin when path is "-".
func readSource(path string, stdin io.Reader) ([]byte, os.FileMode, error) {
	if path == "-" {
		src, err := io.ReadAll(stdin)
		return src, 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return src, fi.Mode().Perm(), nil
}
