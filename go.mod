module github.com/UNO-SOFT/plsql-parser

go 1.17

require (
	bramp.net/antlr4 v0.0.0-20171126210556-f17519e6f52b
	github.com/alecthomas/chroma v0.9.2
	github.com/antlr/antlr4 v0.0.0-20180728001836-7d0787e29ca8
	github.com/godror/godror v0.26.1
	github.com/pkg/errors v0.9.1
)

require (
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

replace bramp.net/antlr4 => ./v/bramp.net/antlr4/

replace github.com/antlr/antlr4 => ./v/github.com/antlr/antlr4
