module github.com/UNO-SOFT/plsql-parser

require (
	bramp.net/antlr4 v0.0.0-20171126210556-f17519e6f52b
	github.com/antlr/antlr4 v0.0.0-20180728001836-7d0787e29ca8
	github.com/pkg/errors v0.8.1
	gopkg.in/goracle.v2 v2.12.4
)

replace bramp.net/antlr4 => ./vendor/bramp.net/antlr4/

replace github.com/antlr/antlr4 => ./vendor/github.com/antlr/antlr4
