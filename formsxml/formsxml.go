// Copyright 2023, 2026 Tamas Gulacsi. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package formsxml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func RewriteXML(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	enc := xml.NewEncoder(w)
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		switch x := tok.(type) {
		case xml.StartElement:
			for _, a := range x.Attr {
				switch a.Name.Local {
				case "TriggerText", "ProgramUnitText", "WhereClause":
					if strings.IndexByte(a.Value, '\n') >= 0 && !strings.Contains(a.Value, "--&amp;#10;\n") {
						a.Value = strings.ReplaceAll(a.Value, "\n", "--&amp;#10;\n")
					}
				}
			}
		}
		if err := enc.EncodeToken(tok); err != nil {
			return err
		}
	}
	return enc.Close()
}

var rewriteXMLreplacer = strings.NewReplacer(
	`xmlns="http://xmlns.oracle.com/Forms" `, ``,
	`--&amp;`, `&amp;`,
	`&#xA;`, `&amp;#10;`,
	`&#x9;`, `&#9;`,
	`&#39;`, `'`,
)

func RewriteXMLFull(w io.Writer, r io.Reader) error {
	var buf strings.Builder
	// io.Copy(&buf, r)
	if err := RewriteXML(&buf, r); err != nil {
		return err
	}
	// go run formsxml.go | sed -e '/xmlns="http:\/\/xmlns.oracle.com\/Forms"/ s,xmlns="http://xmlns.oracle.com/Forms",,' \
	//   -e '/--&amp;/ s/--&amp;/\&amp;/g' \
	//   -e '/&#/ { s/&#xA;/\&amp;#10;/g; s/&#x9;/\&#9;/g; s/&#39;/'"'"'/g; }'

	_, err := io.WriteString(w,
		strings.Replace(rewriteXMLreplacer.Replace(buf.String()),
			"<Module version", `<Module xmlns="http://xmlns.oracle.com/Forms" version`, 1,
		))
	return err
}

var filterXMLReplacer = strings.NewReplacer(
	`--&amp;#10;`, "\n", `&#10;`, "\n",
	`&#9;`, "\t",
)

func FilterXMLText(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	enc := xml.NewEncoder(w)
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
					v := strings.TrimSpace(filterXMLReplacer.Replace(a.Value))
					if v != "" {
						nm := x.Name.Local
						if a.Name.Local == "WhereClause" {
							nm += "." + a.Name.Local
						}
						fmt.Fprintf(w, "\n--BEGIN %[1]s %[2]s\n%[3]s\n--END %[1]s %[2]s;\n",
							nm, strings.Join(path[1:], "/"), v,
						)
					}
				}
			}
		}
	}
	return enc.Close()
}

func FindTables(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	enc := xml.NewEncoder(w)
	var blockName, qdsName string
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
				blockName, qdsName = "", ""
			}
		case xml.StartElement:
			if x.Name.Local == "Block" {
				for _, a := range x.Attr {
					if a.Name.Local == "Name" {
						blockName = a.Value
					} else if a.Name.Local == "QueryDataSourceName" {
						qdsName = a.Value
						break
					}
				}
			} else if qdsName != "" && x.Name.Local == "Item" {
				var fName string
				var skip bool
				for _, a := range x.Attr {
					// logger.Debug(fName, "name", a.Name.Local, "val", a.Value)
					switch a.Name.Local {
					case "Name":
						fName = a.Value
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
					fmt.Println(blockName + "\t" + qdsName + "." + fName)
				}
			}

		}
	}
	return enc.Close()
}
