// Package xmlfmt 提供 XML 格式化/压缩工具：
// 基于原生扫描器做词法切分与配对校验（不改动原文的实体与命名空间写法），
// 格式化用 2 空格缩进，语法错误在信息条显示行号。
package xmlfmt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lestrrat-go/helium"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// 词法标记类型。
const (
	tokText		= iota	// 字符数据（含 CDATA）
	tokOpen			// 开始标签 <a ...>
	tokClose		// 结束标签 </a>
	tokComment		// 注释 <!-- -->
	tokPI			// 处理指令 <? ?>
	tokDirective		// 声明 <!DOCTYPE ...>
)

// xmlToken 是一个原始 XML 词法单元；标签类 token 保留原文。
type xmlToken struct {
	kind		int
	name		string	// 元素名（tokOpen/tokClose）
	text		string	// 原始文本
	line		int	// 起始行（1-based）
	selfClosed	bool	// 开始标签是否以 /> 结束
}

// openTag 是配对校验用的未闭合开始标签记录。
type openTag struct {
	name	string
	line	int
	inline	bool
}

// tool 是 XML 格式化工具模型。
type tool struct {
	ws		toolkit.Workspace
	header		toolkit.SettingHeader
	indent		int
	newLineAttrs	bool

	minify		bool	// 是否压缩输出
	revision	uint64
	running, queued	bool
}

type formatDue struct{ revision uint64 }
type formatDone struct {
	revision	uint64
	output		string
	err		error
}

const backgroundFormatThreshold = 32 << 10

func newTool() *tool {
	t := &tool{
		ws:	toolkit.NewWorkspace("Paste XML…", ""),
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Indentation", "Put attributes on a new line"},
			Descriptions:	[]string{"Select which indentation you want to use", "Whether to put attributes on a new line"},
			Icons:		[]string{"󰉶", "󰘦"},
			Switches:	[]bool{false, true},
			Options:	[][]string{{"2 spaces", "4 spaces", "1 tab", "Minified"}, nil},
		},
		indent:	2,
	}
	t.ws.SetOutputExtendable(true)
	return t
}

func init()	{ toolkit.Register(newTool()) }

// 元信息。

func (t *tool) ID() string		{ return "formatters.xmlfmt" }
func (t *tool) Title() string		{ return "XML Formatter" }
func (t *tool) ShortTitle() string	{ return "XML" }
func (t *tool) Category() string	{ return "formatters" }
func (t *tool) Icon() string		{ return "󰗀" }
func (t *tool) Description() string	{ return "Indent and validate XML" }

// Init 完成首次渲染。
func (t *tool) Init() tea.Cmd {
	t.render()
	return nil
}

// Update 处理选项按键、窗口尺寸与输入按键；输入即时重算。
func (t *tool) actions() []string {
	indentation := "2 spaces"
	switch {
	case t.minify:
		indentation = "Minified"
	case t.indent == 4:
		indentation = "4 spaces"
	case t.indent == 0:
		indentation = "1 tab"
	}
	attrs := "Off"
	if t.newLineAttrs {
		attrs = "On"
	}
	return []string{indentation, attrs}
}
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case formatDue:
		if event.revision != t.revision {
			return t, nil
		}
		if t.running {
			t.queued = true
			return t, nil
		}
		return t, t.startFormat(event.revision)
	case formatDone:
		t.running = false
		if event.revision == t.revision {
			t.ws.SetError(event.err)
			t.ws.SetOutputDisplay(event.output, highlightXML(event.output))
		}
		if t.queued {
			t.queued = false
			return t, t.startFormat(t.revision)
		}
		return t, nil
	}
	if _, ok := msg.(toolkit.InputClearedMsg); ok {
		return t, t.scheduleFormat()
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return t, t.header.Resize(sz, &t.ws, t.actions()...)
	}
	setting, choice, handled := t.header.Pick(msg, t.actions()...)
	if handled && setting < 0 {
		// 缩进下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+i":
			setting, choice, handled = t.header.Shortcut(0, t.actions()...)
		case "alt+a":
			setting, choice, handled = 1, -1, true
		case "alt+m":
			t.minify = !t.minify
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		}
	}
	if handled && setting < 0 {
		// alt+i 只是打开了缩进下拉框，等待键盘或鼠标选择。
		return t, nil
	}
	if handled {
		switch {
		case setting == 0 && choice >= 0:
			t.chooseIndentation(choice)
			return t, t.scheduleFormat()
		case setting == 0:
			// 紧凑布局的回退按钮条与 alt+i：轮换缩进方式。
			t.cycleIndentation()
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		case setting == 1:
			t.newLineAttrs = !t.newLineAttrs
			return t, t.scheduleFormat()
		}
	}
	before := t.ws.Input.Value()
	cmd := t.ws.UpdateBelow(msg, t.header.Height(t.actions()...))
	if before != t.ws.Input.Value() {
		cmd = tea.Batch(cmd, t.scheduleFormat())
	}
	return t, cmd
}

// chooseIndentation 按下拉框的选项下标设置缩进方式（顺序与 Options 一致）。
func (t *tool) chooseIndentation(option int) {
	switch option {
	case 0:
		t.indent, t.minify = 2, false
	case 1:
		t.indent, t.minify = 4, false
	case 2:
		t.indent, t.minify = 0, false
	default:
		t.minify = true
	}
}

// cycleIndentation 轮换缩进方式（紧凑布局与 alt+i 的路径）。
func (t *tool) cycleIndentation() {
	switch {
	case t.minify:
		t.indent, t.minify = 2, false
	case t.indent == 2:
		t.indent = 4
	case t.indent == 4:
		t.indent = 0
	default:
		t.minify = true
	}
}

// HandlesEscape 下拉浮层展开时 esc 只收起浮层，不返回工具列表。
func (t *tool) HandlesEscape() bool	{ return t.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.header.View(t.actions()...) + "\n" + t.ws.View())
}

// render 依据当前输入重算输出；错误写入 Workspace 信息条，不崩溃。
func (t *tool) render() {
	out, err := formatSnapshot(t.ws.Input.Value(), t.indent, t.minify, t.newLineAttrs)
	t.ws.SetError(err)
	t.ws.SetOutputDisplay(out, highlightXML(out))
}

func highlightXML(src string) string {
	if src == "" {
		return ""
	}
	lexer := lexers.Get("xml")
	formatter := formatters.Get("terminal256")
	style := styles.Get("github-dark")
	if lexer == nil || formatter == nil || style == nil {
		return ""
	}
	tokens, err := lexer.Tokenise(nil, src)
	if err != nil {
		return ""
	}
	var output bytes.Buffer
	if err := formatter.Format(&output, style, tokens); err != nil {
		return ""
	}
	return strings.TrimRight(output.String(), "\n")
}

func formatSnapshot(src string, indent int, minify, newLineAttrs bool) (string, error) {
	if strings.TrimSpace(src) == "" {
		return "", nil
	}
	unit := "\t"
	if indent > 0 {
		unit = strings.Repeat(" ", indent)
	}
	return formatXML(src, minify, unit, newLineAttrs)
}

func (t *tool) scheduleFormat() tea.Cmd {
	t.revision++
	t.queued = false
	if len(t.ws.Input.Value()) < backgroundFormatThreshold {
		t.render()
		return nil
	}
	t.ws.SetError(nil)
	t.ws.SetOutput("")
	t.ws.Output.SetContent("Formatting…")
	revision := t.revision
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return toolkit.BackgroundToolMsg{Tool: t, Message: formatDue{revision: revision}}
	})
}

func (t *tool) startFormat(revision uint64) tea.Cmd {
	t.running = true
	src, indent, minify, attrs := t.ws.Input.Value(), t.indent, t.minify, t.newLineAttrs
	return func() tea.Msg {
		out, err := formatSnapshot(src, indent, minify, attrs)
		return toolkit.BackgroundToolMsg{Tool: t, Message: formatDone{revision: revision, output: out, err: err}}
	}
}

// FormatXML 格式化（minify=false，2 空格缩进）或压缩 XML；错误带行号。
func FormatXML(src string, minify bool) (string, error)	{ return formatXML(src, minify, "  ", false) }

func formatXML(src string, minify bool, unit string, newLineAttrs bool) (string, error) {
	toks, err := tokenizeXML(src)
	if err != nil {
		return "", err
	}
	inlineElements := findInlineElements(toks)

	var b strings.Builder
	atLineStart := true
	// writeLine 在格式化模式下按缩进换行输出；压缩模式下原样拼接。
	writeLine := func(indent int, s string) {
		if minify {
			b.WriteString(s)
			return
		}
		if !atLineStart {
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat(unit, indent))
		b.WriteString(s)
		atLineStart = false
	}

	// stack 记录未闭合的开始标签，用于配对校验与行号报错。
	var stack []openTag
	rootSeen := false

	for k := 0; k < len(toks); k++ {
		tok := toks[k]
		parentInline := len(stack) > 0 && stack[len(stack)-1].inline
		writeToken := func(indent int, value string) {
			if parentInline {
				b.WriteString(value)
				atLineStart = false
			} else {
				writeLine(indent, value)
			}
		}
		switch tok.kind {
		case tokOpen:
			if len(stack) == 0 {
				if rootSeen {
					return "", fmt.Errorf("line %d: unexpected element after root", tok.line)
				}
				rootSeen = true
			}
			// 自闭合判定：开始标签紧跟同名结束标签时合并为 <a … />。
			if !tok.selfClosed && k+1 < len(toks) &&
				toks[k+1].kind == tokClose && toks[k+1].name == tok.name {
				raw := selfClose(tok.text, minify)
				if newLineAttrs && !minify {
					raw = formatAttrs(raw, unit, depthOf(stack))
				}
				writeToken(depthOf(stack), raw)
				k++
				continue
			}
			raw := tok.text
			if newLineAttrs && !minify {
				raw = formatAttrs(raw, unit, depthOf(stack))
			}
			writeToken(depthOf(stack), raw)
			if tok.selfClosed {
				continue
			}
			stack = append(stack, openTag{name: tok.name, line: tok.line, inline: parentInline || inlineElements[k]})
		case tokClose:
			if len(stack) == 0 {
				return "", fmt.Errorf("line %d: unexpected closing tag </%s>", tok.line, tok.name)
			}
			top := stack[len(stack)-1]
			if top.name != tok.name {
				return "", fmt.Errorf("line %d: closing tag </%s> does not match <%s>",
					tok.line, tok.name, top.name)
			}
			stack = stack[:len(stack)-1]
			if top.inline {
				b.WriteString(tok.text)
				atLineStart = false
			} else {
				writeLine(depthOf(stack), tok.text)
			}
		case tokText:
			if onlyXMLWhitespace(tok.text) && !parentInline {
				continue
			}
			if len(stack) == 0 {
				return "", fmt.Errorf("line %d: character data outside root element", tok.line)
			}
			if parentInline {
				b.WriteString(tok.text)
				atLineStart = false
			} else {
				writeLine(depthOf(stack), tok.text)
			}
		default:	// 注释、处理指令、声明：原样保留
			writeToken(depthOf(stack), tok.text)
		}
	}
	if len(stack) > 0 {
		top := stack[len(stack)-1]
		return "", fmt.Errorf("line %d: unclosed tag <%s>", top.line, top.name)
	}
	if !rootSeen && strings.TrimSpace(src) != "" {
		return "", fmt.Errorf("line 1: no root element")
	}
	if rootSeen {
		// The lexer preserves original spelling, while a real XML parser checks
		// attributes, entities, namespaces and other well-formedness rules.
		if _, err := helium.NewParser().IgnoreEncoding(true).Parse(context.Background(), []byte(src)); err != nil {
			detail := strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, err.Error())
			return "", fmt.Errorf("invalid XML: %s", detail)
		}
	}
	return b.String(), nil
}

// Mark elements whose direct text or xml:space setting makes inserted
// indentation significant. Their descendants remain inline too.
func findInlineElements(tokens []xmlToken) map[int]bool {
	inline := make(map[int]bool)
	var stack []int
	for i, tok := range tokens {
		switch tok.kind {
		case tokOpen:
			if !tok.selfClosed {
				if preservesSpace(tok.text) {
					inline[i] = true
				}
				stack = append(stack, i)
			}
		case tokClose:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case tokText:
			if len(stack) > 0 && (!onlyXMLWhitespace(tok.text) || (tok.text != "" && !strings.ContainsAny(tok.text, "\r\n"))) {
				inline[stack[len(stack)-1]] = true
			}
		}
	}
	return inline
}

func onlyXMLWhitespace(text string) bool {
	for i := 0; i < len(text); i++ {
		if !isXMLSpace(text[i]) {
			return false
		}
	}
	return true
}

func preservesSpace(raw string) bool {
	i := 1
	for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '>' {
		i++
	}
	for i < len(raw) {
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		start := i
		for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '=' && raw[i] != '>' && raw[i] != '/' {
			i++
		}
		if i == start {
			return false
		}
		name := raw[start:i]
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			return false
		}
		i++
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || (raw[i] != '\'' && raw[i] != '"') {
			return false
		}
		quote := raw[i]
		i++
		start = i
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i >= len(raw) {
			return false
		}
		if name == "xml:space" && raw[start:i] == "preserve" {
			return true
		}
		i++
	}
	return false
}

// formatAttrs preserves each attribute value while laying attributes on separate lines.
func formatAttrs(raw, unit string, depth int) string {
	end := len(raw) - 1
	if end < 2 || raw[end] != '>' {
		return raw
	}
	suffix := ">"
	if raw[end-1] == '/' {
		suffix = "/>"
		end--
	}
	i := 1
	for i < end && !isXMLSpace(raw[i]) {
		i++
	}
	if i >= end {
		return raw
	}
	prefix := raw[:i]
	var attrs []string
	for i < end {
		for i < end && isXMLSpace(raw[i]) {
			i++
		}
		if i == end {
			break
		}
		start := i
		for i < end && !isXMLSpace(raw[i]) && raw[i] != '=' {
			i++
		}
		for i < end && isXMLSpace(raw[i]) {
			i++
		}
		if i >= end || raw[i] != '=' {
			return raw
		}
		i++
		for i < end && isXMLSpace(raw[i]) {
			i++
		}
		if i >= end || (raw[i] != '\'' && raw[i] != '"') {
			return raw
		}
		quote := raw[i]
		i++
		for i < end && raw[i] != quote {
			i++
		}
		if i >= end {
			return raw
		}
		i++
		attrs = append(attrs, raw[start:i])
	}
	if len(attrs) == 0 {
		return raw
	}
	return prefix + "\n" + strings.Repeat(unit, depth+1) + strings.Join(attrs, "\n"+strings.Repeat(unit, depth+1)) + suffix
}
func isXMLSpace(b byte) bool	{ return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

// depthOf 由未闭合标签数量推得当前缩进层级。
func depthOf(stack []openTag) int	{ return len(stack) }

// selfClose 把 "<a …>" 改写为自闭合形式；已经是 /> 的原样返回。
func selfClose(raw string, minify bool) string {
	s := strings.TrimRight(raw, " \t")
	if strings.HasSuffix(s, "/>") {
		return raw
	}
	if strings.HasSuffix(s, ">") {
		if minify {
			return s[:len(s)-1] + "/>"
		}
		return s[:len(s)-1] + " />"
	}
	return raw
}

// isNameStart 判断字节是否可作为 XML 名字的起始字符（含多字节名字）。
func isNameStart(c byte) bool {
	return c == '_' || c == ':' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// tokenizeXML 把源文本切分为词法单元；不合法处返回带行号的错误。
func tokenizeXML(src string) ([]xmlToken, error) {
	var toks []xmlToken
	line := 1
	i := 0

	// advanceLine 统计 [from, to) 内的换行，维护当前行号。
	advanceLine := func(from, to int) {
		line += strings.Count(src[from:to], "\n")
	}

	for i < len(src) {
		if src[i] != '<' {	// 字符数据
			start, textLine := i, line
			for i < len(src) && src[i] != '<' {
				i++
			}
			advanceLine(start, i)
			toks = append(toks, xmlToken{kind: tokText, text: src[start:i], line: textLine})
			continue
		}

		tokLine := line
		switch {
		case strings.HasPrefix(src[i:], "<!--"):	// 注释
			rel := strings.Index(src[i+4:], "-->")
			if rel < 0 {
				return nil, fmt.Errorf("line %d: unclosed comment", tokLine)
			}
			end := i + 4 + rel + 3
			advanceLine(i, end)
			toks = append(toks, xmlToken{kind: tokComment, text: src[i:end], line: tokLine})
			i = end

		case strings.HasPrefix(src[i:], "<![CDATA["):	// CDATA 视作字符数据
			rel := strings.Index(src[i+9:], "]]>")
			if rel < 0 {
				return nil, fmt.Errorf("line %d: unclosed CDATA", tokLine)
			}
			end := i + 9 + rel + 3
			advanceLine(i, end)
			toks = append(toks, xmlToken{kind: tokText, text: src[i:end], line: tokLine})
			i = end

		case strings.HasPrefix(src[i:], "<?"):	// 处理指令
			rel := strings.Index(src[i+2:], "?>")
			if rel < 0 {
				return nil, fmt.Errorf("line %d: unclosed processing instruction", tokLine)
			}
			end := i + 2 + rel + 2
			advanceLine(i, end)
			inner := strings.TrimSpace(src[i+2 : i+2+rel])
			name := inner
			if n := strings.IndexAny(inner, " \t\r\n"); n >= 0 {
				name = inner[:n]
			}
			toks = append(toks, xmlToken{kind: tokPI, text: src[i:end], line: tokLine, name: name})
			i = end

		case strings.HasPrefix(src[i:], "</"):	// 结束标签
			rel := strings.IndexByte(src[i:], '>')
			if rel < 0 {
				return nil, fmt.Errorf("line %d: unclosed end tag", tokLine)
			}
			name := strings.TrimSpace(src[i+2 : i+rel])
			if name == "" || strings.ContainsAny(name, " \t\r\n") {
				return nil, fmt.Errorf("line %d: invalid end tag name", tokLine)
			}
			end := i + rel + 1
			advanceLine(i, end)
			toks = append(toks, xmlToken{kind: tokClose, text: src[i:end], name: name, line: tokLine})
			i = end

		case strings.HasPrefix(src[i:], "<!"):	// DOCTYPE 等声明（处理内部子集 [ ]）
			j, subset := i+2, 0
			var quote byte
			for j < len(src) {
				c := src[j]
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '\'' || c == '"' {
					quote = c
				} else if strings.HasPrefix(src[j:], "<!--") {
					end := strings.Index(src[j+4:], "-->")
					if end < 0 {
						return nil, fmt.Errorf("line %d: unclosed declaration comment", tokLine)
					}
					j += 4 + end + 3
					continue
				} else if c == '[' {
					subset++
				} else if c == ']' {
					subset--
				} else if c == '>' && subset <= 0 {
					break
				} else if c == '<' && subset == 0 {
					return nil, fmt.Errorf("line %d: unclosed declaration", tokLine)
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("line %d: unclosed declaration", tokLine)
			}
			end := j + 1
			advanceLine(i, end)
			toks = append(toks, xmlToken{kind: tokDirective, text: src[i:end], line: tokLine})
			i = end

		default:	// 开始标签
			if i+1 >= len(src) || !isNameStart(src[i+1]) {
				return nil, fmt.Errorf("line %d: invalid '<'", tokLine)
			}
			j, quote := i+1, byte(0)
			for j < len(src) {
				c := src[j]
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '"' || c == '\'' {
					quote = c
				} else if c == '>' {
					break
				} else if c == '<' {
					return nil, fmt.Errorf("line %d: unclosed start tag", tokLine)
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("line %d: unclosed start tag", tokLine)
			}
			end := j + 1
			advanceLine(i, end)
			inner := strings.TrimSpace(src[i+1 : j])
			selfClosed := strings.HasSuffix(inner, "/")
			if selfClosed {
				inner = strings.TrimSpace(inner[:len(inner)-1])
			}
			name := inner
			if n := strings.IndexAny(inner, " \t\r\n"); n >= 0 {
				name = inner[:n]
			}
			if name == "" {
				return nil, fmt.Errorf("line %d: invalid tag name", tokLine)
			}
			toks = append(toks, xmlToken{
				kind:	tokOpen, text: src[i:end], name: name,
				line:	tokLine, selfClosed: selfClosed,
			})
			i = end
		}
	}
	return toks, nil
}
