package analyzer

import (
	"math/rand/v2"
	"slices"
	"strings"
	"unicode"

	"github.com/toy-forge/devtoys/internal/toolkit/editor"
)

// The order follows the desktop groups: line endings, case, and line sorting.
var operationLabels = []string{
	"LF", "CRLF", "lowercase", "UPPERCASE", "Sentence case", "Title Case",
	"camelCase", "PascalCase", "snake_case", "CONSTANT_CASE", "kebab-case", "COBOL-CASE", "Train-Case",
	"aLtErNaTiNg", "AlTeRnAtInG", "Random case",
	"Sort A–Z", "Sort Z–A", "Sort last A–Z", "Sort last Z–A", "Reverse lines", "Shuffle lines", "Original",
}

func (m *Model) applyOperation(index int) {
	if index < 0 || index >= len(operationLabels) {
		return
	}
	text := m.ws.Input.Value()
	result := text
	if index == len(operationLabels)-1 {
		result = m.original
	} else {
		result = transform(text, index)
	}
	if result == text {
		return
	}
	// Replacing the editor without checking its limits can silently truncate a
	// conversion that expands CR-only text into thousands of LF-delimited lines.
	probe := editor.New()
	probe.CharLimit = m.ws.Input.CharLimit
	if err := probe.CheckInsert(result); err != nil {
		m.ws.SetError(err)
		return
	}
	m.ws.Input.SetValue(result)
	m.last = result
	m.report(analyze(result))
}

func detectLineEndings(text string) string {
	crlf, lf, cr := false, false, false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				crlf = true
				i++
			} else {
				cr = true
			}
		case '\n':
			lf = true
		}
	}
	if n := boolCount(crlf) + boolCount(lf) + boolCount(cr); n > 1 {
		return "Mixed"
	} else if n == 0 {
		return "Unknown"
	}
	if crlf {
		return "CRLF"
	}
	if cr {
		return "CR"
	}
	return "LF"
}
func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
func normalizeLines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func transform(text string, action int) string {
	switch action {
	case 0:
		return normalizeLines(text)
	case 1:
		return strings.ReplaceAll(normalizeLines(text), "\n", "\r\n")
	case 2:
		return strings.ToLower(text)
	case 3:
		return strings.ToUpper(text)
	case 4, 5:
		return sentenceOrTitle(text, action == 5)
	case 6, 7:
		return joinedCase(text, action == 7, 0, false)
	case 8:
		return joinedCase(text, false, '_', false)
	case 9:
		return joinedCase(text, false, '_', true)
	case 10:
		return joinedCase(text, false, '-', false)
	case 11:
		return joinedCase(text, false, '-', true)
	case 12:
		return joinedCase(text, true, '-', false)
	case 13, 14, 15:
		var out strings.Builder
		for i, r := range []rune(text) {
			upper := (i%2 == 1) == (action == 13)
			if action == 15 {
				upper = rand.IntN(2) == 0
			}
			if upper {
				r = unicode.ToUpper(r)
			} else {
				r = unicode.ToLower(r)
			}
			out.WriteRune(r)
		}
		return out.String()
	case 16, 17, 18, 19, 20, 21:
		return sortLines(text, action)
	}
	return text
}
func sentenceOrTitle(text string, title bool) string {
	var out strings.Builder
	start := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start {
				r = unicode.ToUpper(r)
			} else {
				r = unicode.ToLower(r)
			}
			start = false
		} else if title || r == '\n' || r == '\r' || isSentenceEnd(r) {
			start = true
		}
		out.WriteRune(r)
	}
	return out.String()
}
func joinedCase(text string, upperFirst bool, separator rune, upperAll bool) string {
	runes := []rune(text)
	var out strings.Builder
	start := true
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if upperAll || (upperFirst && start) || (separator == 0 && !upperFirst && start && i > 0 && runes[i-1] != '\n' && runes[i-1] != '\r') {
				r = unicode.ToUpper(r)
			} else {
				r = unicode.ToLower(r)
			}
			out.WriteRune(r)
			start = false
		} else if r == '\r' || r == '\n' {
			out.WriteRune(r)
			start = true
		} else if separator == 0 {
			start = true
		} else if !start && i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
			out.WriteRune(separator)
			start = true
		}
	}
	return out.String()
}
func sortLines(text string, action int) string {
	if text == "" {
		return text
	}
	end := "\n"
	switch detectLineEndings(text) {
	case "CRLF":
		end = "\r\n"
	case "CR":
		end = "\r"
	}
	lines := strings.Split(normalizeLines(text), "\n")
	switch action {
	case 16:
		slices.Sort(lines)
	case 17:
		slices.Sort(lines)
		slices.Reverse(lines)
	case 18, 19:
		slices.SortStableFunc(lines, func(a, b string) int {
			if action == 19 {
				a, b = b, a
			}
			return strings.Compare(lastWord(a), lastWord(b))
		})
	case 20:
		slices.Reverse(lines)
	case 21:
		rand.Shuffle(len(lines), func(i, j int) { lines[i], lines[j] = lines[j], lines[i] })
	}
	return strings.Join(lines, end)
}

func lastWord(text string) string {
	runes := []rune(text)
	end := len(runes)
	for end > 0 && !unicode.IsLetter(runes[end-1]) && !unicode.IsDigit(runes[end-1]) {
		end--
	}
	start := end
	for start > 0 && (unicode.IsLetter(runes[start-1]) || unicode.IsDigit(runes[start-1])) {
		start--
	}
	return string(runes[start:end])
}
