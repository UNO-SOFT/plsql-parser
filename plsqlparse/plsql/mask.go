package plsql

import "fmt"

// mask returns a copy of src of the same length in which line comments
// (-- up to end of line), block comments (/* ... */), string literals
// ('...' with ” escaping) and quoted identifiers ("...") are replaced
// by spaces. Newlines are preserved, so every byte offset computed on
// the result refers to the same position in the original input, and
// line numbers stay valid. Only keywords outside comments, strings and
// quoted identifiers are visible to keyword scanning on the result.
func mask(src []byte) ([]byte, error) {
	out := make([]byte, len(src))
	copy(out, src)
	i, n := 0, len(src)
	for i < n {
		switch c := src[i]; {
		case c == '-' && i+1 < n && src[i+1] == '-':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for i < n && src[i] != '\n' {
				out[i] = ' '
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			start := i
			out[i], out[i+1] = ' ', ' '
			i += 2
			closed := false
			for i < n {
				if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i += 2
					closed = true
					break
				}
				if src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated /* comment", start)
			}
		case c == '\'':
			start := i
			out[i] = ' '
			i++
			closed := false
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' { // escaped quote ('')
						out[i], out[i+1] = ' ', ' '
						i += 2
						continue
					}
					out[i] = ' '
					i++
					closed = true
					break
				}
				if src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated string literal", start)
			}
		case c == '"':
			start := i
			out[i] = ' '
			i++
			closed := false
			for i < n {
				if src[i] == '"' {
					out[i] = ' '
					i++
					closed = true
					break
				}
				if src[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("byte offset %d: unterminated quoted identifier", start)
			}
		default:
			i++
		}
	}
	return out, nil
}
