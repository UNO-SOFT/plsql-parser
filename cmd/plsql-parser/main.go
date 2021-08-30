// Copyright 2021 Tamás Gulácsi. All rights reserved.

package main

import (
	"io"
	"log"
	"os"

	plsqlparser "github.com/UNO-SOFT/plsql-parser"
)

func main() {
	if err := Main(); err != nil {
		log.Fatalf("ERROR: %+v", err)
	}
}
func Main() error {
	text, _ := io.ReadAll(os.Stdin)
	return plsqlparser.ChromaParse(string(text))
}
