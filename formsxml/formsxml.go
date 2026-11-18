// Copyright 2023, 2026 Tamas Gulacsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package formsxml

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// RewriteXML rewrites the xml replacing whitespace entities (&amp;#10;) with \n.
func RewriteXML(w io.Writer, r io.Reader) error {
	var repl *strings.Replacer
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if bytes.IndexByte(scanner.Bytes(), '&') < 0 {
			if _, err := w.Write(scanner.Bytes()); err != nil {
				return err
			}
		} else {
			if repl == nil {
				repl = strings.NewReplacer(
					"&amp;#10;", "\n",
					"&#x9;", "\t",
					"&#39;", "'",
				)
			}
			if _, err := io.WriteString(w, repl.Replace(scanner.Text())); err != nil {
				return err
			}
		}
		w.Write([]byte{'\n'})
	}
	return scanner.Err()
}

var rewriteXMLreplacer = strings.NewReplacer(
	`xmlns="http://xmlns.oracle.com/Forms" `, ``,
	`--&amp;`, `&amp;`,
	`&#xA;`, `&amp;#10;`,
	`&#x9;`, `&#9;`,
	`&#39;`, `'`,
)

var filterXMLReplacer = strings.NewReplacer(
	`--&amp;#10;`, "\n", `&#10;`, "\n",
	`&#9;`, "\t",
)

func FilterXMLText(w io.Writer, r io.Reader, jsonOut bool) error {
	dec := xml.NewDecoder(r)
	type result struct {
		Name, Path, Text string
	}
	print := func(res result) error {
		_, err := fmt.Fprintf(w, "\n--BEGIN %[1]s %[2]s\n%[3]s\n--END %[1]s %[2]s;\n",
			res.Name, res.Path, res.Text,
		)
		return err
	}
	if jsonOut {
		enc := jsontext.NewEncoder(w)
		print = func(res result) error {
			return json.MarshalEncode(enc, res)
		}
	}

	var path []string
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		switch x := tok.(type) {
		case xml.EndElement:
			path = path[:len(path)-1]
		case xml.StartElement:
			var name string
			for _, b := range x.Attr {
				if b.Name.Local == "Name" {
					name = b.Value
				}
			}
			path = append(path, name)
			for _, a := range x.Attr {
				switch a.Name.Local {
				case "TriggerText", "ProgramUnitText", "WhereClause":
					var res result
					res.Text = strings.TrimSpace(filterXMLReplacer.Replace(a.Value))
					if res.Text != "" {
						res.Name = x.Name.Local
						if a.Name.Local == "WhereClause" {
							res.Name += "." + a.Name.Local
						}
						res.Path = strings.Join(path[1:], "/")
						if err := print(res); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func FindTables(w io.Writer, r io.Reader, jsonOut bool) error {
	type result struct {
		Block, QDS string
		Fields     []string
	}
	close := func() error { return nil }
	print := func(res result) error {
		if len(res.Fields) == 0 {
			return nil
		}
		slices.Sort(res.Fields)
		for _, f := range res.Fields {
			if _, err := fmt.Println(res.Block + "\t" + res.QDS + "." + f); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		enc := jsontext.NewEncoder(w)
		print = func(res result) error {
			if len(res.Fields) == 0 {
				return nil
			}
			slices.Sort(res.Fields)
			return json.MarshalEncode(enc, res)
		}
	}
	dec := xml.NewDecoder(r)
	var res result
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		switch x := tok.(type) {
		case xml.EndElement:
			if x.Name.Local == "Block" {
				print(res)
				res.Block, res.QDS, res.Fields = "", "", res.Fields[:0]
			}
		case xml.StartElement:
			if x.Name.Local == "Block" {
				print(res)
				for _, a := range x.Attr {
					if a.Name.Local == "Name" {
						if res.Block != "" && res.Block != a.Value {
							print(res)
							res.Fields = res.Fields[:0]
						}
						res.Block = a.Value
					} else if a.Name.Local == "QueryDataSourceName" {
						if res.QDS != "" && res.QDS != a.Value {
							print(res)
							res.Fields = res.Fields[:0]
						}
						res.QDS = a.Value
						break
					}
				}
			} else if res.QDS != "" && x.Name.Local == "Item" {
				var f string
				var skip bool
				for _, a := range x.Attr {
					// logger.Debug(fName, "name", a.Name.Local, "val", a.Value)
					switch a.Name.Local {
					case "Name":
						f = a.Value
					case "DatabaseItem":
						skip = a.Value == "false"
					case "DataType":
						skip = a.Value == ""
					case "ItemType":
						skip = !strings.HasSuffix(a.Value, "Item")
					case "ParentType":
						skip = a.Value == "15"
					}
				}
				if !skip {
					res.Fields = append(res.Fields, f)
				}
			}
		}
	}
	return close()
}
