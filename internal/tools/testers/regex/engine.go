package regex

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	regexp2 "github.com/dlclark/regexp2/v2"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

const maxPatternBytes = 32 << 10
const maxTextBytes = 1 << 20
const maxMatches = 1000

type matchResult struct {
	text, display, matchesDisplay	string
	err				error
}
type hit struct {
	start, end		int
	utf16Start, utf16End	int
	groups			[]group
}
type group struct {
	number			int
	name, value		string
	matched			bool
	start, end		int
	utf16Start, utf16End	int
}

type cheatSheetEntry struct{ section, syntax, description string }

var cheatSheet = []cheatSheetEntry{
	{"Character classes", `.`, "Any character"},
	{"Character classes", `[Abc]`, "Character set"},
	{"Character classes", `[^Abc]`, "Negated character set"},
	{"Character classes", `[A-Z]`, "Character range"},
	{"Character classes", `[\s\S]`, "Any character, including line breaks"},
	{"Character classes", `\X`, "Unicode grapheme cluster"},
	{"Character classes", `\w`, "Word character"},
	{"Character classes", `\W`, "Non-word character"},
	{"Character classes", `\d`, "Digit"},
	{"Character classes", `\D`, "Non-digit"},
	{"Character classes", `\s`, "Whitespace"},
	{"Character classes", `\S`, "Non-whitespace"},
	{"Character classes", `\h`, "Horizontal whitespace"},
	{"Character classes", `\H`, "Non-horizontal whitespace"},
	{"Character classes", `\v`, "Vertical whitespace"},
	{"Character classes", `\V`, "Non-vertical whitespace"},
	{"Character classes", `\R`, "Line break"},
	{"Character classes", `\N`, "Non-line-break character"},
	{"Character classes", `\p{L}`, "Unicode category"},
	{"Character classes", `\P{L}`, "Negated Unicode category"},
	{"Character classes", `\p{Han}`, "Unicode script"},
	{"Character classes", `\P{Han}`, "Negated Unicode script"},
	{"Anchors", `\A`, "Beginning of input"},
	{"Anchors", `\Z`, "End of input or before final line break"},
	{"Anchors", `\z`, "Strict end of input"},
	{"Anchors", `^`, "Beginning of line"},
	{"Anchors", `$`, "End of line"},
	{"Anchors", `\b`, "Word boundary"},
	{"Anchors", `\B`, "Non-word boundary"},
	{"Anchors", `\G`, "Previous match end"},
	{"Escaped characters", `\+`, "Reserved character"},
	{"Escaped characters", `\000`, "Octal escape"},
	{"Escaped characters", `\xFF`, "Hexadecimal escape"},
	{"Escaped characters", `\x{FF}`, "Unicode escape"},
	{"Escaped characters", `\cI`, "Control character"},
	{"Escaped characters", `\Q...\E`, "Escaped literal sequence"},
	{"Escaped characters", `\t`, "Tab"},
	{"Escaped characters", `\n`, "Line feed"},
	{"Escaped characters", `\f`, "Form feed"},
	{"Escaped characters", `\r`, "Carriage return"},
	{"Escaped characters", `\0`, "Null character"},
	{"Escaped characters", `\a`, "Bell"},
	{"Escaped characters", `\e`, "Escape character"},
	{"Groups and references", `(ABC)`, "Capturing group"},
	{"Groups and references", `(?<name>ABC)`, "Named capturing group"},
	{"Groups and references", `\k'name'`, "Named backreference"},
	{"Groups and references", `\1`, "Numbered backreference"},
	{"Groups and references", `(?|(a)|(b))`, "Branch reset group"},
	{"Groups and references", `(?:ABC)`, "Non-capturing group"},
	{"Groups and references", `(?>ABC)`, "Atomic group"},
	{"Groups and references", `(?(DEFINE)(?'foo'ABC))`, "Define subpattern"},
	{"Groups and references", `\g'1'`, "Numbered subroutine"},
	{"Groups and references", `\g'name'`, "Named subroutine"},
	{"Lookaround", `(?=ABC)`, "Positive lookahead"},
	{"Lookaround", `(?!ABC)`, "Negative lookahead"},
	{"Lookaround", `(?<=ABC)`, "Positive lookbehind"},
	{"Lookaround", `(?<!ABC)`, "Negative lookbehind"},
	{"Lookaround", `\K`, "Reset reported match start"},
	{"Quantifiers and alternation", `+`, "One or more"},
	{"Quantifiers and alternation", `*`, "Zero or more"},
	{"Quantifiers and alternation", `{1,3}`, "Repeat range"},
	{"Quantifiers and alternation", `?`, "Optional"},
	{"Quantifiers and alternation", `?`, "Lazy quantifier"},
	{"Quantifiers and alternation", `+`, "Possessive quantifier"},
	{"Quantifiers and alternation", `|`, "Alternation"},
	{"Special", `(?#comment)`, "Inline comment"},
	{"Special", `(?(?=A)B|C)`, "Conditional expression"},
	{"Special", `(?(1)B|C)`, "Group conditional"},
	{"Special", `(?R)`, "Recursive pattern"},
	{"Special", `(?i)`, "Inline option"},
	{"Substitution", `$0`, "Entire match"},
	{"Substitution", `$1`, "Capture group"},
	{"Substitution", `\n`, "Escaped character"},
}

// matchRegex runs on a background worker. regexp2's timeout bounds each
// backtracking attempt; the aggregate deadline bounds repeated searches.
func matchRegex(pattern, text string, global bool, flags regexp2.RegexOptions) matchResult {
	if pattern == "" {
		return matchResult{text: "Enter a regular expression to match.", display: formatPanel("", nil, false)}
	}
	if len(pattern) > maxPatternBytes {
		return matchResult{err: fmt.Errorf("regular expression exceeds 32 KiB")}
	}
	if len(text) > maxTextBytes {
		return matchResult{err: fmt.Errorf("input text exceeds 1 MiB")}
	}
	re, err := regexp2.Compile(pattern, flags)
	if err != nil {
		return matchResult{err: fmt.Errorf("invalid regular expression: %v", err)}
	}
	if text == "" {
		return matchResult{text: "Enter text to match.", display: formatPanel("", nil, false)}
	}
	re.MatchTimeout = 200 * time.Millisecond
	deadline := time.Now().Add(500 * time.Millisecond)
	var hits []hit
	truncated := false
	match, err := re.FindStringMatch(text)
	for match != nil && err == nil {
		start, length := match.ByteRange()
		h := hit{start: start, end: start + length}
		for _, number := range re.GetGroupNumbers() {
			if number == 0 {
				continue
			}
			if len(h.groups) >= 100 {
				truncated = true
				break
			}
			g := match.GroupByNumber(number)
			if g == nil {
				continue
			}
			name := re.GroupNameFromNumber(number)
			if name == fmt.Sprint(number) {
				name = ""
			}
			item := group{number: number, name: name, matched: len(g.Captures) > 0}
			if item.matched {
				item.start, item.end = g.ByteRange()
				item.end += item.start
				item.value = excerpt(g.String(), 256)
			}
			h.groups = append(h.groups, item)
		}
		hits = append(hits, h)
		if !global {
			break
		}
		if len(hits) >= maxMatches {
			truncated = true
			break
		}
		if time.Now().After(deadline) {
			return matchResult{err: fmt.Errorf("matching exceeded the time limit")}
		}
		match, err = re.FindNextMatch(match)
	}
	if err != nil {
		return matchResult{err: fmt.Errorf("matching failed or timed out: %v", err)}
	}
	setUTF16Offsets(text, hits)
	raw := formatResult(text, hits, truncated, false)
	display := formatPanel(text, hits, truncated)
	return matchResult{text: raw, display: display, matchesDisplay: formatMatchPanel(text, hits, truncated)}
}

// .NET reports locations in UTF-16 code units, while regexp2 uses UTF-8 byte ranges.
func setUTF16Offsets(text string, hits []hit) {
	positions := make(map[int]int, len(hits)*2)
	for _, h := range hits {
		positions[h.start] = 0
		positions[h.end] = 0
		for _, g := range h.groups {
			if g.matched {
				positions[g.start] = 0
				positions[g.end] = 0
			}
		}
	}
	units := 0
	for offset, r := range text {
		if _, ok := positions[offset]; ok {
			positions[offset] = units
		}
		units++
		if r > 0xffff {
			units++
		}
	}
	if _, ok := positions[len(text)]; ok {
		positions[len(text)] = units
	}
	for i := range hits {
		hits[i].utf16Start = positions[hits[i].start]
		hits[i].utf16End = positions[hits[i].end]
		for j := range hits[i].groups {
			if hits[i].groups[j].matched {
				hits[i].groups[j].utf16Start = positions[hits[i].groups[j].start]
				hits[i].groups[j].utf16End = positions[hits[i].groups[j].end]
			}
		}
	}
}

func excerpt(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
func displayText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return '␉'
		case r == '\r':
			return '␍'
		case r < 0x20 || r == 0x7f:
			return '�'
		default:
			return r
		}
	}, s)
}
func formatResult(text string, hits []hit, truncated, safe bool) string {
	convert := func(s string) string {
		if safe {
			return displayText(s)
		}
		return s
	}
	var b strings.Builder
	sorted := slices.Clone(hits)
	slices.SortFunc(sorted, func(a, b hit) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return a.end - b.end
	})
	last := 0
	for _, h := range sorted {
		if h.start < last || h.start < 0 || h.end > len(text) || h.end < h.start {
			continue
		}
		b.WriteString(convert(text[last:h.start]))
		b.WriteString(matchStyle.Render(convert(text[h.start:h.end])))
		last = h.end
	}
	b.WriteString(convert(text[last:]))
	b.WriteString("\n\n")
	if len(hits) == 0 {
		b.WriteString("No matches.")
		return b.String()
	}
	fmt.Fprintf(&b, "%d matches:\n", len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "\n[%d] %d..%d  %s\n", i+1, h.utf16Start, h.utf16End, matchStyle.Render(convert(excerpt(text[h.start:h.end], 256))))
		for _, g := range h.groups {
			label := fmt.Sprintf("Group %d", g.number)
			if g.name != "" {
				label = fmt.Sprintf("Group %d (%s)", g.number, g.name)
			}
			if !g.matched {
				fmt.Fprintf(&b, "    %s: no match\n", label)
			} else {
				fmt.Fprintf(&b, "    %s: %s\n", label, matchStyle.Render(convert(g.value)))
			}
		}
	}
	if truncated {
		b.WriteString("\nResults limited to 1,000 matches or 100 groups per match.\n")
	}
	return b.String()
}

func formatPanel(text string, hits []hit, truncated bool) string {
	return theme.Eyebrow("Match information") + "\n" + formatMatchPanel(text, hits, truncated) + "\n\n" + theme.Eyebrow("Cheat sheet") + "\n" + formatCheatSheet()
}

// matchRow 是匹配结果表的一行：名称 / 位置 / 取值（分组行名称缩进）。
type matchRow struct{ name, location, value string }

// formatMatchPanel 渲染匹配结果表：名称、位置（UTF-16 下标）、取值三列；
// 列宽取内容的最大宽度，取值列占满剩余宽度。
func formatMatchPanel(text string, hits []hit, truncated bool) string {
	var rows []matchRow
	for i, h := range hits {
		value := displayText(excerpt(text[h.start:h.end], 96))
		rows = append(rows, matchRow{fmt.Sprintf("Match %d", i+1), fmt.Sprintf("%d-%d", h.utf16Start, h.utf16End), matchStyle.Render(value)})
		for _, g := range h.groups {
			label := "  " + fmt.Sprintf("Group %d", g.number)
			if g.name != "" {
				label = "  Group " + g.name
			}
			if !g.matched {
				rows = append(rows, matchRow{label, "—", ""})
				continue
			}
			rows = append(rows, matchRow{label, fmt.Sprintf("%d-%d", g.utf16Start, g.utf16End), matchStyle.Render(displayText(g.value))})
		}
	}
	var b strings.Builder
	if len(rows) == 0 {
		b.WriteString("Name     Location    Value\nNo matches.")
	} else {
		nameWidth, locationWidth, valueWidth := ui.LabelWidth(len("Name")), ui.LabelWidth(len("Location")), ui.LabelWidth(len("Value"))
		for _, r := range rows {
			nameWidth = max(nameWidth, ui.Width(r.name))
			locationWidth = max(locationWidth, ui.Width(r.location))
			valueWidth = max(valueWidth, ui.Width(r.value))
		}
		table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
			{Title: "Name", Width: nameWidth},
			{Title: "Location", Width: locationWidth},
			{Title: "Value", Width: valueWidth},
		}}
		b.WriteString(table.Header())
		for _, r := range rows {
			_, line := table.Row(r.name, r.location, r.value)
			b.WriteString("\n" + line)
		}
	}
	if truncated {
		b.WriteString("\n\nResults limited to 1,000 matches or 100 groups per match.")
	}
	return b.String()
}

func formatCheatSheet() string	{ return formatCheatSheetAtWidth(60) }

func formatCheatSheetAtWidth(width int) string {
	var b strings.Builder
	width = max(12, width)
	// 列序与 jsonpath 页统一：Syntax | Description。语法列定宽，描述列占满剩余宽度。
	syntaxWidth := min(20, max(5, width/3))
	table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
		{Title: "Syntax", Width: syntaxWidth},
		{Title: "Description", Width: max(1, width-syntaxWidth-ui.Space2)},
	}}
	b.WriteString(table.Header())
	section := ""
	for _, entry := range cheatSheet {
		if entry.section != section {
			section = entry.section
			// 分区标题走统一眉标角色；源码存自然大小写，形态由角色决定。
			b.WriteString("\n\n" + theme.Eyebrow(section))
		}
		_, line := table.Row(cheatSyntaxStyle.Render(entry.syntax), entry.description)
		b.WriteString("\n" + line)
	}
	return b.String()
}

func formatHighlightedText(text string, hits []hit) string {
	var b strings.Builder
	sorted := slices.Clone(hits)
	slices.SortFunc(sorted, func(a, b hit) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return a.end - b.end
	})
	last := 0
	for _, h := range sorted {
		if h.start < last || h.start < 0 || h.end > len(text) || h.end < h.start {
			continue
		}
		b.WriteString(displayText(text[last:h.start]))
		b.WriteString(matchStyle.Render(displayText(text[h.start:h.end])))
		last = h.end
	}
	b.WriteString(displayText(text[last:]))
	return b.String()
}
