// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Package checklist implements the Python checklist.py JSON acceptance format.
package refactor

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

type Check map[string]any
type Result struct {
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Error    string `json:"error,omitempty"`
	Pass     bool   `json:"pass"`
}

func loadChecks(path string) ([]Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw any
	if err := decodeJSON(data, &raw); err != nil {
		return nil, err
	}
	if obj, ok := raw.(map[string]any); ok {
		raw = obj["checks"]
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("checklist must be a list or an object containing a 'checks' list")
	}
	result := make([]Check, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each check must be an object")
		}
		result = append(result, obj)
	}
	return result, nil
}

func pick(check Check, primary, fallback string) any {
	if v, ok := check[primary]; ok && v != nil && v != "" {
		return v
	}
	return check[fallback]
}
func number(v any) (int, error) {
	if i, ok := numberInt(v); ok {
		return i, nil
	}
	if s, ok := v.(string); ok {
		return strconv.Atoi(s)
	}
	return 0, fmt.Errorf("expected integer, got %v", v)
}
func shellQuote(value string) string {
	s, err := syntax.Quote(value, syntax.LangBash)
	if err != nil {
		panic(err)
	}
	return s
}
func shorten(value string, limit int) string {
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit])
	}
	return value
}

func evaluateOne(check Check, base, defaultFile string) (actual, expected any, err error) {
	kind, _ := pick(check, "type", "op").(string)
	if kind == "cmd" {
		command, _ := pick(check, "command", "shell").(string)
		if strings.TrimSpace(command) == "" {
			return nil, nil, fmt.Errorf("cmd requires a nonempty 'command'")
		}
		defaultQuoted := ""
		if defaultFile != "" {
			defaultQuoted = shellQuote(defaultFile)
		}
		command = strings.ReplaceAll(command, "{file}", defaultQuoted)
		timeoutSeconds := 120
		if v, ok := check["timeout"]; ok && v != nil {
			timeoutSeconds, err = number(v)
			if err != nil {
				return nil, nil, err
			}
		}
		if timeoutSeconds <= 0 {
			return nil, nil, fmt.Errorf("timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = base
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("command timed out after %ds", timeoutSeconds)
		}
		code := 0
		if runErr != nil {
			if exit, ok := runErr.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				return nil, nil, runErr
			}
		}
		expExit := 0
		if v, ok := check["expect_exit"]; ok && v != nil {
			expExit, err = number(v)
			if err != nil {
				return nil, nil, err
			}
		}
		stdoutStr := strings.ToValidUTF8(stdout.String(), "\ufffd")
		stderrStr := strings.ToValidUTF8(stderr.String(), "\ufffd")
		expected = map[string]any{"exit": expExit, "stdout_contains": check["stdout_contains"]}
		containsOK := true
		if check["stdout_contains"] != nil {
			containsOK = strings.Contains(stdoutStr, fmt.Sprint(check["stdout_contains"]))
		}
		actual = map[string]any{"exit": code, "stdout": shorten(stdoutStr, 1200), "stderr": shorten(stderrStr, 500), "_stdout_full_match": containsOK}
		return actual, expected, nil
	}
	switch kind {
	case "md5", "wc-l", "count", "ends-with", "no-crlf":
		file, hasFile := check["file"]
		path := defaultFile
		if hasFile && file != nil {
			s, ok := file.(string)
			if !ok {
				return nil, nil, fmt.Errorf("file must be a path string")
			}
			path, err = resolvePath(base, s)
			if err != nil {
				return nil, nil, err
			}
		}
		if path == "" {
			return nil, nil, fmt.Errorf("%s requires 'file'", kind)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, nil, e
		}
		switch kind {
		case "md5":
			v, ok := check["expect"]
			if !ok {
				return nil, nil, fmt.Errorf("missing expect")
			}
			expected = strings.ToLower(fmt.Sprint(v))
			digest := md5.Sum(data)
			actual = fmt.Sprintf("%x", digest)
		case "wc-l":
			v, ok := check["expect"]
			if !ok {
				return nil, nil, fmt.Errorf("missing expect")
			}
			expected, err = number(v)
			if err != nil {
				return nil, nil, err
			}
			actual = bytes.Count(data, []byte{'\n'})
		case "count":
			needle, ok := check["substring"].(string)
			if !ok || needle == "" {
				return nil, nil, fmt.Errorf("count requires nonempty 'substring'")
			}
			v, ok := check["expect"]
			if !ok {
				return nil, nil, fmt.Errorf("missing expect")
			}
			expected, err = number(v)
			if err != nil {
				return nil, nil, err
			}
			actual = bytes.Count(data, []byte(needle))
		case "ends-with":
			var suffix []byte
			if value, has := check["hex"]; has {
				h, ok := value.(string)
				if !ok {
					return nil, nil, fmt.Errorf("hex must be a string")
				}
				suffix, err = hex.DecodeString(strings.Join(strings.Fields(h), ""))
				if err != nil {
					return nil, nil, err
				}
			} else {
				literal, ok := check["bytes"].(string)
				if !ok {
					return nil, nil, fmt.Errorf("ends-with requires 'bytes' (UTF-8 string) or 'hex'")
				}
				suffix = []byte(literal)
			}
			actual = bytes.HasSuffix(data, suffix)
			expected = true
		case "no-crlf":
			actual = !bytes.Contains(data, []byte{'\r'})
			expected = true
		}
		return actual, expected, nil
	default:
		return nil, nil, fmt.Errorf("unknown check type: %q", kind)
	}
}

func evaluateChecks(checks []Check, base, defaultFile string) ([]Result, bool) {
	results := make([]Result, 0, len(checks))
	all := len(checks) > 0
	for i, check := range checks {
		kind, _ := pick(check, "type", "op").(string)
		name, _ := check["name"].(string)
		if name == "" {
			name = fmt.Sprintf("%d: %s", i+1, kind)
		}
		actual, expected, err := evaluateOne(check, base, defaultFile)
		result := Result{Name: name, Type: kind}
		if err != nil {
			result.Error = err.Error()
			all = false
		} else {
			result.Actual = actual
			result.Expected = expected
			// cmd compares only exit status and optional substring.
			if kind == "cmd" {
				a := actual.(map[string]any)
				e := expected.(map[string]any)
				result.Pass = a["exit"] == e["exit"]
				if e["stdout_contains"] != nil {
					result.Pass = result.Pass && a["_stdout_full_match"].(bool)
				}
				delete(a, "_stdout_full_match")
			} else {
				result.Pass = fmt.Sprintf("%T:%v", actual, actual) == fmt.Sprintf("%T:%v", expected, expected)
			}
			if !result.Pass {
				all = false
			}
		}
		results = append(results, result)
	}
	return results, all
}

func LoadAndEvaluate(config, defaultFile string) ([]Result, bool, error) {
	absolute, err := AbsolutePath(config)
	if err != nil {
		return nil, false, err
	}
	checks, err := loadChecks(absolute)
	if err != nil {
		return nil, false, err
	}
	results, ok := evaluateChecks(checks, filepath.Dir(absolute), defaultFile)
	return results, ok, nil
}
func DisplayChecks(results []Result, passed bool) {
	for _, r := range results {
		value := r.Actual
		if r.Error != "" {
			value = r.Error
		}
		fmt.Printf("%s %s: %v", map[bool]string{true: "PASS", false: "FAIL"}[r.Pass], r.Name, value)
		if !r.Pass && r.Error == "" {
			fmt.Printf(" (expected %v)", r.Expected)
		}
		fmt.Println()
	}
	if passed {
		fmt.Println("ALL PASS")
	} else {
		fmt.Println("FAILED")
	}
}
