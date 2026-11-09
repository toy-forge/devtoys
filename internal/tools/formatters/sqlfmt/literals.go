package sqlfmt

import (
	"fmt"
	"strings"
)

type protectedLiteral struct{ marker, text string }

// Protect literal forms which the upstream v4 tokenizer does not recognize.
// Markers are valid SQL identifiers and are guaranteed absent from the input.
func protectDialectLiterals(src, language string) (string, []protectedLiteral, error) {
	prefix := "__DEVTOYS_SQL_LITERAL_"
	upper := strings.ToUpper(src)
	for strings.Contains(upper, prefix) {
		prefix += "X"
	}
	var b strings.Builder
	var values []protectedLiteral
	mark := func(text string) {
		name := fmt.Sprintf("%s%d__", prefix, len(values))
		values = append(values, protectedLiteral{name, text})
		b.WriteString(name)
	}
	for i := 0; i < len(src); {
		start := i
		// Skip comments and ordinary quoted strings/identifiers intact, so text
		// resembling a dialect-specific literal within them is never interpreted.
		if strings.HasPrefix(src[i:], "--") || (src[i] == '#' && (language == "mysql" || language == "mariadb")) {
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				b.WriteString(src[i:])
				break
			}
			i += end
			b.WriteString(src[start:i])
			continue
		}
		if strings.HasPrefix(src[i:], "/*") {
			i += 2
			depth := 1
			nested := false
			for i < len(src) && depth > 0 {
				if strings.HasPrefix(src[i:], "/*") {
					depth++
					nested = true
					i += 2
				} else if strings.HasPrefix(src[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if nested && (language == "postgresql" || language == "redshift" || language == "tsql") {
				if depth != 0 {
					return "", nil, fmt.Errorf("unterminated nested block comment")
				}
				mark(src[start:i])
			} else {
				b.WriteString(src[start:i])
			}
			continue
		}
		boundary := i == 0 || !isWordChar(src[i-1])
		if boundary && (language == "postgresql" || language == "redshift") && src[i] == '$' {
			end := i + 1
			if end < len(src) && src[end] != '$' {
				if !isTagStart(src[end]) {
					end = i
				} else {
					end++
					for end < len(src) && isTagPart(src[end]) {
						end++
					}
				}
			}
			if end > i && end < len(src) && src[end] == '$' {
				delimiter := src[i : end+1]
				closeAt := strings.Index(src[end+1:], delimiter)
				if closeAt < 0 {
					return "", nil, fmt.Errorf("unterminated PostgreSQL dollar-quoted string")
				}
				i = end + 1 + closeAt + len(delimiter)
				mark(src[start:i])
				continue
			}
		}
		if boundary && language == "plsql" {
			q := i
			if q+1 < len(src) && (src[q] == 'n' || src[q] == 'N') && (src[q+1] == 'q' || src[q+1] == 'Q') {
				q++
			}
			if q+2 < len(src) && (src[q] == 'q' || src[q] == 'Q') && src[q+1] == '\'' {
				open := src[q+2]
				close := open
				switch open {
				case '[':
					close = ']'
				case '{':
					close = '}'
				case '(':
					close = ')'
				case '<':
					close = '>'
				}
				if open == ' ' || open == '\t' || open == '\n' || open == '\r' || open == '\'' || open >= 0x80 {
					return "", nil, fmt.Errorf("invalid Oracle alternative quote delimiter")
				}
				end := q + 3
				for end+1 < len(src) && !(src[end] == close && src[end+1] == '\'') {
					end++
				}
				if end+1 >= len(src) {
					return "", nil, fmt.Errorf("unterminated Oracle alternative-quoted string")
				}
				i = end + 2
				mark(src[start:i])
				continue
			}
		}
		if boundary && (language == "postgresql" || language == "redshift") && i+1 < len(src) && (src[i] == 'E' || src[i] == 'e') && src[i+1] == '\'' {
			end, ok := quotedEnd(src, i+1, '\'', true)
			if !ok {
				return "", nil, fmt.Errorf("unterminated PostgreSQL escape string")
			}
			i = end
			mark(src[start:i])
			continue
		}
		if boundary {
			prefixEnd := i
			if (src[i] == 'N' || src[i] == 'n' || src[i] == 'B' || src[i] == 'b' || src[i] == 'X' || src[i] == 'x') && i+1 < len(src) && src[i+1] == '\'' {
				prefixEnd = i + 1
			}
			if (language == "postgresql" || language == "redshift") && i+2 < len(src) && (src[i] == 'U' || src[i] == 'u') && src[i+1] == '&' && (src[i+2] == '\'' || src[i+2] == '"') {
				prefixEnd = i + 2
			}
			if (language == "mysql" || language == "mariadb") && src[i] == '_' {
				j := i + 1
				for j < len(src) && (isTagPart(src[j]) || src[j] == '$') {
					j++
				}
				if j > i+1 && j < len(src) && src[j] == '\'' {
					prefixEnd = j
				}
			}
			if prefixEnd > i {
				close := src[prefixEnd]
				backslash := language == "mysql" || language == "mariadb" || language == "spark" || language == "n1ql"
				end, ok := quotedEnd(src, prefixEnd, close, backslash)
				if !ok {
					return "", nil, fmt.Errorf("unterminated prefixed SQL literal")
				}
				i = end
				mark(src[start:i])
				continue
			}
		}
		if src[i] == '\'' || src[i] == '"' || src[i] == '`' || (src[i] == '[' && language == "tsql") {
			closing := src[i]
			if closing == '[' {
				closing = ']'
			}
			backslash := language == "mysql" || language == "mariadb" || language == "spark" || language == "n1ql"
			end, ok := quotedEnd(src, i, closing, backslash)
			if !ok {
				return "", nil, fmt.Errorf("unterminated SQL quoted text")
			}
			i = end
			mark(src[start:i])
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String(), values, nil
}
func quotedEnd(src string, start int, closing byte, backslash bool) (int, bool) {
	for i := start + 1; i < len(src); i++ {
		if backslash && src[i] == '\\' && i+1 < len(src) {
			i++
			continue
		}
		if src[i] == closing {
			if i+1 < len(src) && src[i+1] == closing {
				i++
				continue
			}
			return i + 1, true
		}
	}
	return len(src), false
}
func isTagStart(c byte) bool {
	return c == '_' || c >= 0x80 || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
func isTagPart(c byte) bool	{ return isTagStart(c) || c >= '0' && c <= '9' }

func isWordChar(c byte) bool {
	return c == '_' || c == '$' || c == '#' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}
