package parser

import "github.com/antlr4-go/antlr/v4"

// PlSqlLexerBase is the base for the generated PlSqlLexer.
type PlSqlLexerBase struct {
	*antlr.BaseLexer
}

// IsNewlineAtPos returns true if there is a newline at the given
// position relative to the current character lookahead.
func (l *PlSqlLexerBase) IsNewlineAtPos(pos int) bool {
	la := l.GetInputStream().LA(pos)
	return la == -1 || la == '\n'
}

// IsQQuoteDelimiter returns true if the Q-quote opening delimiter is
// closed by its matching delimiter (e.g. '[' by ']').
func (l *PlSqlLexerBase) IsQQuoteDelimiter() bool {
	input := l.GetInputStream()
	opening := input.LA(l.TokenStartCharIndex + 2 - l.GetCharIndex())
	closing := input.LA(-2)
	switch opening {
	case '[':
		return closing == ']'
	case '{':
		return closing == '}'
	case '<':
		return closing == '>'
	case '(':
		return closing == ')'
	default:
		return closing == opening
	}
}
