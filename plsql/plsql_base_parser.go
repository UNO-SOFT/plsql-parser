package parser

import (
	"github.com/antlr/antlr4/runtime/Go/antlr"
)

// PlSqlBaseParser implementation.
type PlSqlBaseParser struct {
	*antlr.BaseParser
	isVersion12 bool
	isVersion10 bool
}

func (p *PlSqlBaseParser) IsVersion12() bool {
	return p.isVersion12
}

func (p *PlSqlBaseParser) IsVersion10() bool {
	return p.isVersion10
}
