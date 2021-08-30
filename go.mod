module github.com/UNO-SOFT/plsql-parser

go 1.17

require (
	bramp.net/antlr4 v0.0.0-20171126210556-f17519e6f52b
	github.com/antlr/antlr4 v0.0.0-20180728001836-7d0787e29ca8
	github.com/godror/godror v0.25.3
	github.com/pkg/errors v0.9.1
)

require github.com/go-logfmt/logfmt v0.5.0 // indirect

replace bramp.net/antlr4 => ./vendor/bramp.net/antlr4/

replace github.com/antlr/antlr4 => ./vendor/github.com/antlr/antlr4
