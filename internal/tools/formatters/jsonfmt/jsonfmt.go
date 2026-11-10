// Package jsonfmt 提供 JSON 格式化/压缩工具：
// 输入即时重算，支持 2/4 空格缩进切换与压缩模式，
// 输出经 chroma 语法高亮展示，语法错误在信息条显示行列位置。
package jsonfmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// tool 是 JSON 格式化工具模型。
type tool struct {
	ws	toolkit.Workspace
	header	toolkit.SettingHeader

	indent		int	// 缩进空格数，2 或 4
	minify		bool	// 是否压缩输出
	sortProperties	bool
	revision	uint64
	running, queued	bool
}

type formatDue struct{ revision uint64 }
type formatDone struct {
	revision	uint64
	output, display	string
	err		error
}

const backgroundFormatThreshold = 32 << 10

func newTool() *tool {
	t := &tool{
		ws:	toolkit.NewWorkspace("Paste JSON…", ""),
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Indentation", "Sort JSON Properties alphabetically"},
			Descriptions:	[]string{"Select which indentation you want to use", "Whether properties within the JSON should be sorted alphabetically"},
			Icons:		[]string{"󰉶", "󰒺"},
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

func (t *tool) ID() string		{ return "formatters.jsonfmt" }
func (t *tool) Title() string		{ return "JSON Formatter" }
func (t *tool) ShortTitle() string	{ return "JSON" }
func (t *tool) Category() string	{ return "formatters" }
func (t *tool) Icon() string		{ return "󰘦" }
func (t *tool) Description() string	{ return "Indent, minify and validate JSON" }

func (t *tool) PaneStatusHints() []ui.StatusHint { return t.ws.StatusHints() }

// Init 完成首次渲染。
func (t *tool) Init() tea.Cmd {
	t.render()
	return nil
}

// Update 处理选项按键、窗口尺寸与输入按键；输入即时重算。
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
			t.ws.SetOutputDisplay(event.output, event.display)
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
	setting, option, handled := t.header.Pick(msg, t.actions()...)
	if handled {
		switch {
		case setting == 0 && option >= 0:
			// 下拉框选中：直接设置缩进方式，浮层不改变布局。
			t.chooseIndentation(option)
			return t, t.scheduleFormat()
		case setting == 0:
			// 紧凑布局的回退按钮条：点击即轮换（无浮层空间）。
			t.cycleIndentation()
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		case setting == 1:
			t.sortProperties = !t.sortProperties
			return t, t.scheduleFormat()
		}
		return t, nil
	}

	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return t, t.header.Resize(sz, &t.ws, t.actions()...)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+s":
			t.sortProperties = !t.sortProperties
			return t, t.scheduleFormat()
		case "alt+2":
			t.indent, t.minify = 2, false
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		case "alt+4":
			t.indent, t.minify = 4, false
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		case "alt+m":
			t.minify = !t.minify
			t.header.Relayout(&t.ws, t.actions()...)
			return t, t.scheduleFormat()
		case "tab", "shift+tab", "ctrl+y":
			// 焦点切换与复制不改变输入，交给 Workspace 后无需重算。
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

// cycleIndentation 轮换缩进方式（紧凑布局的按钮回退路径）。
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

// actions 返回设置条的当前取值（顺序与 header 的 Names 一致）。
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
	sort := "Off"
	if t.sortProperties {
		sort = "On"
	}
	return []string{indentation, sort}
}

// View 渲染顶部选项行与公共工作区；下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.header.View(t.actions()...) + "\n" + t.ws.View())
}

// render 依据当前输入重算输出；错误写入 Workspace 信息条，不崩溃。
func (t *tool) render() {
	out, display, err := formatSnapshot(t.ws.Input.Value(), t.indent, t.minify, t.sortProperties)
	t.ws.SetError(err)
	t.ws.SetOutputDisplay(out, display)
}

func formatSnapshot(src string, indent int, minify, sortProperties bool) (string, string, error) {
	if strings.TrimSpace(src) == "" {
		return "", "", nil
	}
	if sortProperties {
		sorted, err := sortJSON(src)
		if err != nil {
			return "", "", err
		}
		src = sorted
	}
	out, err := FormatJSON(src, indent, minify)
	if err != nil {
		return "", "", err
	}
	display := highlight(out)
	if display == "" {
		display = out
	}
	return out, display, nil
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
	src, indent, minify, sortProperties := t.ws.Input.Value(), t.indent, t.minify, t.sortProperties
	return func() tea.Msg {
		out, display, err := formatSnapshot(src, indent, minify, sortProperties)
		return toolkit.BackgroundToolMsg{Tool: t, Message: formatDone{revision: revision, output: out, display: display, err: err}}
	}
}

// FormatJSON 把 src 按指定缩进格式化或压缩；错误带行列位置。
func FormatJSON(src string, indent int, minify bool) (string, error) {
	var buf bytes.Buffer
	var err error
	if minify {
		err = json.Compact(&buf, []byte(src))
	} else {
		indentation := "\t"
		if indent > 0 {
			indentation = strings.Repeat(" ", indent)
		}
		err = json.Indent(&buf, []byte(src), "", indentation)
	}
	if err != nil {
		return "", jsonError(src, err)
	}
	return buf.String(), nil
}

// jsonError 把标准库错误翻译成带行列位置的中文错误。
func jsonError(src string, err error) error {
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		line, col := lineCol(src, syn.Offset)
		return fmt.Errorf("JSON syntax error at line %d, column %d: %s", line, col, syn.Error())
	}
	return fmt.Errorf("JSON parse failed: %w", err)
}

// lineCol 把字节偏移换算为 1-based 行列号。
func lineCol(src string, offset int64) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(src)) {
		offset = int64(len(src))
	}
	line, col = 1, 1
	for i := 0; i < int(offset); i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// jsonHighlightStyle 基于 theme 色板构建 chroma 样式：
// JSON 键/数字走 BlueBright、字符串 Blue、true/false/null 走 Magenta、
// 错误 Danger，不用 github-dark 预置样式的 256 色输出（脱离色板）；不设 Background 条目，terminal16m formatter 还会清空
// 背景色——高亮文本落在面板 Surface 底上，不自带底色序列。
func jsonHighlightStyle() *chroma.Style {
	return chroma.MustNewStyle("devtoys", chroma.StyleEntries{
		chroma.Text:		string(theme.Text),
		chroma.Punctuation:	string(theme.Text),
		chroma.NameTag:		string(theme.BlueBright),
		chroma.LiteralString:	string(theme.Blue),
		chroma.LiteralNumber:	string(theme.BlueBright),
		chroma.Keyword:		string(theme.Magenta),
		chroma.KeywordConstant:	string(theme.Magenta),
		chroma.Operator:	string(theme.Text),
		chroma.Error:		string(theme.Danger),
	})
}

// highlight 用 chroma 对 JSON 做真彩终端高亮（terminal16m + theme 色）；
// 失败时返回空串回退纯文本。
func highlight(src string) string {
	lexer := lexers.Get("json")
	formatter := formatters.Get("terminal16m")
	style := jsonHighlightStyle()
	if lexer == nil || formatter == nil || style == nil {
		return ""
	}
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), "\n")
}
