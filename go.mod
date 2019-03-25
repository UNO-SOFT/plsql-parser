module github.com/UNO-SOFT/plsql-parser

require (
	bramp.net/antlr4 v0.0.0-20171126210556-f17519e6f52b
	github.com/alecthomas/template v0.0.0-20160405071501-a0175ee3bccc // indirect
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf // indirect
	github.com/antlr/antlr4 v0.0.0-20180728001836-7d0787e29ca8
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/json-iterator/go v0.0.0-20180806060727-1624edc4454b
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v0.0.0-20180718012357-94122c33edd3 // indirect
	github.com/pkg/errors v0.8.0
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.2.2 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/goracle.v2 v2.12.1
)

replace bramp.net/antlr4 v0.0.0-20171126210556-f17519e6f52b => ./vendor/bramp.net/antlr4/

replace github.com/antlr/antlr4 v0.0.0-20180728001836-7d0787e29ca8 => ./vendor/github.com/antlr/antlr4
