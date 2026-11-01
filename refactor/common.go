// Copyright 2026 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

// Package common provides byte-preserving file and JSON utilities shared by the
// Go counterparts of the Python plsql-refactor scripts.
package refactor

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ByteLines splits only on LF. An unterminated last line is preserved verbatim.
func ByteLines(data []byte) [][]byte {
	if len(data) == 0 {
		return [][]byte{}
	}
	pieces := bytes.Split(data, []byte{'\n'})
	lines := make([][]byte, 0, len(pieces))
	for i, p := range pieces {
		if i == len(pieces)-1 && len(p) == 0 {
			break
		}
		row := append([]byte(nil), p...)
		if i < len(pieces)-1 {
			row = append(row, '\n')
		}
		lines = append(lines, row)
	}
	return lines
}

func JoinLines(lines [][]byte) []byte { return bytes.Join(lines, nil) }

func StripLF(line []byte) []byte { return bytes.TrimSuffix(line, []byte{'\n'}) }

func RequireUTF8(data []byte) (string, error) {
	if !utf8.Valid(data) {
		return "", fmt.Errorf("invalid UTF-8 text")
	}
	return string(data), nil
}

func ExpandUser(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
func Absolute(path string) (string, error) {
	expanded, err := ExpandUser(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}
func Resolve(base, path string) (string, error) {
	expanded, err := ExpandUser(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(base, expanded)
	}
	return filepath.Abs(expanded)
}

func DecodeJSON(data []byte, dest any) error {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	if err := json.UnmarshalDecode(dec, dest, json.WithUnmarshalers(
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, val *any) error {
			// If the next value to be decoded is a JSON number,
			// then provide a concrete Go type to unmarshal into.
			if dec.PeekKind() == '0' {
				*val = jsontext.Value(nil)
			}
			// Return ErrUnsupported to fallback on default unmarshal behavior.
			return errors.ErrUnsupported
		}),
	),
	); err != nil {
		return err
	}
	if dec.PeekKind() != jsontext.KindInvalid {
		return fmt.Errorf("unexpected trailing JSON data")
	}
	return nil
}

func PrettyJSON(value any) ([]byte, error) {
	return json.Marshal(value, jsontext.WithIndent("  "))
}
func PrintJSON(w io.Writer, value any) error {
	return json.MarshalWrite(w, value, jsontext.WithIndent("  "))
}

func WriteJSON(path string, value any) error {
	data, err := PrettyJSON(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func NumberInt(v any) (int, bool) {
	switch n := v.(type) {
	case jsontext.Value:
		if n.Kind() == jsontext.KindNumber {
			if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
				return int(i), true
			} else if f, err := strconv.ParseFloat(string(n), 64); err == nil {
				if float64(int64(f)) == f {
					return int(f), true
				}
			}
		}
		return 0, false
	case int:
		return n, true
	default:
		return 0, false
	}
}
func ExpectInt(v any) (int, error) {
	n, ok := NumberInt(v)
	if !ok {
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
	return n, nil
}
func AsString(v any) (string, bool) { s, ok := v.(string); return s, ok }
