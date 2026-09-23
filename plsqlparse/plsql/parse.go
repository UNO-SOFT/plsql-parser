// Package plsql parses PL/SQL source code and reports the source extent
// of every package, procedure and function, including nested ones.
//
// It is comment-, string- and encoding-safe: block comments, line
// comments and string literals are masked out before keyword scanning,
// so program units that only exist inside comments are never reported,
// and keywords inside strings cannot confuse end detection. Input may
// use any single-byte encoding (e.g. ISO-8859-2); all offsets are byte
// offsets into the original source.
package plsql

import (
	"fmt"
	"os"
	"strings"
)

// Object describes a PL/SQL program unit: its type and the byte offsets
// of its source extent. Begin points at the first byte of the construct
// (the "CREATE" keyword for top level objects, the PROCEDURE/FUNCTION
// keyword for nested ones), End just past the terminating ';' of its
// final END (or of the ';' for declarations).
type Object struct {
	Type  string `json:"type"`
	Name  string `json:"-"`
	Begin int    `json:"begin"`
	End   int    `json:"end"`
}

// Recognized object types.
const (
	TypePackage     = "package"
	TypePackageBody = "package body"
	TypeProcedure   = "procedure"
	TypeFunction    = "function"
)

type parser struct {
	m    []byte // masked source, offset-aligned with the input
	objs []Object
}

// Parse scans PL/SQL source and returns the extents of all package, package
// body, procedure and function constructs in source order, outer objects
// before their nested ones. Byte offsets refer to src.
func Parse(src []byte) ([]Object, error) {
	m, err := mask(src)
	if err != nil {
		return nil, err
	}
	p := &parser{m: m}
	if err := p.topLevel(); err != nil {
		return nil, err
	}
	return p.objs, nil
}

// ParseFile reads path and parses it. The file may use any single byte
// encoding (e.g. ISO-8859-2): all offsets are byte based.
func ParseFile(path string) ([]Object, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	objs, err := Parse(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return objs, nil
}

func (p *parser) topLevel() error {
	i := 0
	for {
		w, ws, we := nextWord(p.m, i)
		if w == "" {
			if ws >= len(p.m) {
				return nil
			}
			i = ws + 1
			continue
		}
		if !strings.EqualFold(w, "create") {
			i = we
			continue
		}
		createAt := ws
		i = we
		if w2, _, we2 := nextWord(p.m, i); strings.EqualFold(w2, "or") {
			if w3, _, we3 := nextWord(p.m, we2); strings.EqualFold(w3, "replace") {
				i = we3
			} else {
				i = we2
			}
		}
		wk, kws, kwe := nextWord(p.m, i)
		switch {
		case strings.EqualFold(wk, "package"):
			typ := TypePackage
			j := kwe
			if wb, _, web := nextWord(p.m, j); strings.EqualFold(wb, "body") {
				typ = TypePackageBody
				j = web
			}
			name, nwe, err := p.identAt(j, kws, "package name")
			if err != nil {
				return err
			}
			// header up to IS/AS (tolerates AUTHID and similar clauses)
			hw, _, hwe := nextWord(p.m, nwe)
			for hw != "" && !strings.EqualFold(hw, "is") && !strings.EqualFold(hw, "as") {
				hw, _, hwe = nextWord(p.m, hwe)
			}
			if hw == "" {
				return fmt.Errorf("byte offset %d: %s %s: IS/AS not found", kws, typ, name)
			}
			end, kids, err := p.packageEnd(typ, name, hwe)
			if err != nil {
				return err
			}
			p.objs = append(p.objs, Object{Type: typ, Name: name, Begin: createAt, End: end})
			p.objs = append(p.objs, kids...)
			i = end
		case strings.EqualFold(wk, "function"), strings.EqualFold(wk, "procedure"):
			typ := strings.ToLower(wk)
			name, nwe, err := p.identAt(kwe, kws, typ+" name")
			if err != nil {
				return err
			}
			isDef, at, err := classify(p.m, nwe)
			if err != nil {
				return err
			}
			if !isDef { // bare top level declaration
				p.objs = append(p.objs, Object{Type: typ, Name: name, Begin: createAt, End: at + 1})
				i = at + 1
				continue
			}
			_, _, aend := nextWord(p.m, at) // skip the IS/AS keyword
			end, kids, err := p.subprogram(typ, name, aend)
			if err != nil {
				return err
			}
			p.objs = append(p.objs, Object{Type: typ, Name: name, Begin: createAt, End: end})
			p.objs = append(p.objs, kids...)
			i = end
		default: // CREATE TABLE etc: keep scanning
			i = kwe
		}
	}
}

// identAt reads an object name at pos (schema qualification with a single
// '.' is tolerated), returning the lowercased name and the offset after it.
func (p *parser) identAt(pos, errAt int, what string) (string, int, error) {
	w, _, we := nextWord(p.m, pos)
	if w == "" || !isIdentStart(w[0]) {
		return "", pos, fmt.Errorf("byte offset %d: %s expected", errAt, what)
	}
	if we < len(p.m) && p.m[we] == '.' {
		w2, _, we2 := nextWord(p.m, we+1)
		if w2 != "" && isIdentStart(w2[0]) {
			return strings.ToLower(w + "." + w2), we2, nil
		}
	}
	return strings.ToLower(w), we, nil
}

// packageEnd parses a package spec or body after its IS/AS keyword and
// returns the offset just past the final ';' together with the entries
// of the subprograms declared or defined inside.
func (p *parser) packageEnd(typ, name string, pos int) (int, []Object, error) {
	stop, stopPos, kids, err := p.declSection(pos)
	if err != nil {
		return 0, nil, err
	}
	if stop == stopBegin { // package body with initialization block
		end, err := p.blockStatements(stopPos)
		if err != nil {
			return 0, nil, fmt.Errorf("%s %s: %w", typ, name, err)
		}
		return end, kids, nil
	}
	end, err := p.consumeEndTail(stopPos)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", typ, name, err)
	}
	return end, kids, nil
}

// subprogram parses a function/procedure body after its IS/AS keyword and
// returns the offset just past the terminating ';' together with the
// entries of subprograms declared or defined inside it.
func (p *parser) subprogram(typ, name string, pos int) (int, []Object, error) {
	stop, stopPos, kids, err := p.declSection(pos)
	if err != nil {
		return 0, nil, err
	}
	if stop != stopBegin {
		return 0, nil, fmt.Errorf("%s %s: byte offset %d: BEGIN expected", typ, name, stopPos)
	}
	end, err := p.blockStatements(stopPos)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", typ, name, err)
	}
	return end, kids, nil
}

const (
	stopBegin = iota
	stopEnd
)

// declSection scans the declaration section starting at pos until the
// BEGIN of the executable part (returns stopBegin and the offset just
// after the BEGIN keyword) or until the final END of the enclosing
// package (returns stopEnd and the offset of the END keyword). Along the
// way it records every procedure/function declaration (ends at ';') and
// definition (recursively parsed, ends past its final ';').
func (p *parser) declSection(pos int) (int, int, []Object, error) {
	var kids []Object
	i := pos
	for {
		w, ws, we := nextWord(p.m, i)
		if w == "" {
			if ws >= len(p.m) {
				return 0, 0, nil, fmt.Errorf("byte offset %d: unterminated declaration section (EOF)", pos)
			}
			i = ws + 1
			continue
		}
		switch {
		case strings.EqualFold(w, "begin"):
			return stopBegin, we, kids, nil
		case strings.EqualFold(w, "end"):
			return stopEnd, ws, kids, nil
		case strings.EqualFold(w, "procedure"), strings.EqualFold(w, "function"):
			typ := strings.ToLower(w)
			name, nwe, err := p.identAt(we, ws, typ+" name")
			if err != nil {
				return 0, 0, nil, err
			}
			isDef, at, err := classify(p.m, nwe)
			if err != nil {
				return 0, 0, nil, err
			}
			if isDef {
				_, _, aend := nextWord(p.m, at) // skip the IS/AS keyword
				end, subKids, err := p.subprogram(typ, name, aend)
				if err != nil {
					return 0, 0, nil, err
				}
				kids = append(kids, Object{Type: typ, Name: name, Begin: ws, End: end})
				kids = append(kids, subKids...)
				i = end
			} else {
				kids = append(kids, Object{Type: typ, Name: name, Begin: ws, End: at + 1})
				i = at + 1
			}
		default: // type, cursor, constant, variable, pragma, exception...: skip to ';'
			sem, ok := findSemi(p.m, we)
			if !ok {
				return 0, 0, nil, fmt.Errorf("byte offset %d: ';' not found in declaration", ws)
			}
			i = sem + 1
		}
	}
}

// blockStatements scans the executable statements starting right after a
// BEGIN keyword and returns the offset just past the ';' of the END that
// closes this block. BEGIN and CASE open nested scopes, bare END closes
// one, END IF / END LOOP close constructs without scope and END CASE
// closes a CASE. Nested procedure/function definitions cannot occur here
// (they live in declaration sections, which declSection already handled
// recursively), so plain depth counting suffices.
func (p *parser) blockStatements(pos int) (int, error) {
	depth := 1
	i := pos
	for {
		w, ws, we := nextWord(p.m, i)
		if w == "" {
			if ws >= len(p.m) {
				return 0, fmt.Errorf("byte offset %d: unterminated block (EOF), END missing", pos)
			}
			i = ws + 1
			continue
		}
		switch {
		case strings.EqualFold(w, "begin"), strings.EqualFold(w, "case"):
			depth++
			i = we
		case strings.EqualFold(w, "end"):
			nw, _, nwe := nextWord(p.m, we)
			if strings.EqualFold(nw, "if") || strings.EqualFold(nw, "loop") {
				i = nwe
				continue
			}
			if strings.EqualFold(nw, "case") {
				depth--
				i = nwe
				continue
			}
			depth--
			if depth == 0 {
				return p.consumeEndTail(ws)
			}
			i = we
		default:
			i = we
		}
	}
}

// consumeEndTail consumes the optional trailing label and the terminating
// ';' after an END keyword starting at pos, returning the offset just
// past the ';'.
func (p *parser) consumeEndTail(pos int) (int, error) {
	_, _, we := nextWord(p.m, pos)
	sem, ok := findSemi(p.m, we)
	if !ok {
		return 0, fmt.Errorf("byte offset %d: ';' expected after END", pos)
	}
	return sem + 1, nil
}
