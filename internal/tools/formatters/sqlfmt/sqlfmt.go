// Package sqlfmt provides a dialect-aware SQL formatter.
package sqlfmt

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// tool 是 SQL 格式化工具模型。
type tool struct {
	ws		toolkit.Workspace
	header		toolkit.SettingHeader
	indent		int
	leadingComma	bool
	dialect		int
	revision	uint64
	running, queued	bool
}

// dialectNames 按 dialects 的顺序返回方言显示名（下拉框选项与下标一一对应）。
func dialectNames() []string {
	names := make([]string, 0, len(dialects))
	for _, d := range dialects {
		names = append(names, d.name)
	}
	return names
}

func newTool() *tool {
	t := &tool{
		ws:	toolkit.NewWorkspace("Paste SQL…", ""),
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Language", "Indentation", "Leading comma"},
			Descriptions:	[]string{"Which SQL language to use", "Select which indentation you want to use", "Whether to use leading commas or not"},
			Icons:		[]string{"󰆼", "󰉶", "󰒺"},
			Switches:	[]bool{false, false, true},
			Options:	[][]string{dialectNames(), {"2 spaces", "4 spaces", "1 tab"}, nil},
		},
		indent:		2,
		dialect:	standardSQLDialectIndex,
	}
	t.ws.SetOutputExtendable(true)
	return t
}

func init()	{ toolkit.Register(newTool()) }

// 元信息。

func (t *tool) ID() string		{ return "formatters.sqlfmt" }
func (t *tool) Title() string		{ return "SQL Formatter" }
func (t *tool) ShortTitle() string	{ return "SQL" }
func (t *tool) Category() string	{ return "formatters" }
func (t *tool) Icon() string		{ return "󰆼" }
func (t *tool) Description() string	{ return "Format SQL statements" }

func (t *tool) PaneStatusHints() []ui.StatusHint { return t.ws.StatusHints() }

func (t *tool) Init() tea.Cmd	{ return t.schedule() }

func (t *tool) actions() []string {
	indentation := "2 spaces"
	if t.indent == 4 {
		indentation = "4 spaces"
	} else if t.indent == 0 {
		indentation = "1 tab"
	}
	leading := "Off"
	if t.leadingComma {
		leading = "On"
	}
	return []string{dialects[t.dialect].name, indentation, leading}
}

type sqlDue struct{ revision uint64 }
type sqlDone struct {
	revision	uint64
	output		string
	err		error
}

func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case sqlDue:
		if event.revision != t.revision {
			return t, nil
		}
		if t.running {
			t.queued = true
			return t, nil
		}
		return t, t.start(event.revision)
	case sqlDone:
		t.running = false
		if event.revision == t.revision {
			t.ws.SetError(event.err)
			if event.err != nil {
				t.ws.SetOutput("")
			} else {
				t.ws.SetOutputDisplay(event.output, sqlDisplay(event.output))
			}
		}
		if t.queued {
			t.queued = false
			return t, t.start(t.revision)
		}
		return t, nil
	case toolkit.InputClearedMsg:
		return t, t.schedule()
	case tea.WindowSizeMsg:
		// 极矮终端放弃设置条与内容区之间的空行，保证 View 行数==终端高度。
		if event.Height <= 8 {
			t.header.ContentGap = 0
		} else {
			t.header.ContentGap = 1
		}
		return t, t.header.Resize(event, &t.ws, t.actions()...)
	}
	setting, choice, handled := t.header.Pick(msg, t.actions()...)
	if handled && setting < 0 {
		// 下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+d":
			setting, choice, handled = t.header.Shortcut(0, t.actions()...)
		case "alt+i":
			setting, choice, handled = t.header.Shortcut(1, t.actions()...)
		case "alt+c":
			setting, choice, handled = 2, -1, true
		}
	}
	if handled && setting < 0 {
		// 快捷键只是打开了下拉框，等待键盘或鼠标选择。
		return t, nil
	}
	switch {
	case setting == 0 && choice >= 0:
		t.dialect = choice
	case setting == 0:
		// 紧凑布局的回退按钮条与 alt+d：轮换方言。
		t.dialect = (t.dialect + 1) % len(dialects)
	case setting == 1 && choice >= 0:
		t.indent = []int{2, 4, 0}[choice]
	case setting == 1:
		switch t.indent {
		case 2:
			t.indent = 4
		case 4:
			t.indent = 0
		default:
			t.indent = 2
		}
	case setting == 2:
		t.leadingComma = !t.leadingComma
	}
	if handled {
		t.header.Relayout(&t.ws, t.actions()...)
		return t, t.schedule()
	}
	before := t.ws.Input.Value()
	cmd := t.ws.UpdateBelow(msg, t.header.Height(t.actions()...))
	if t.ws.Input.Value() != before {
		cmd = tea.Batch(cmd, t.schedule())
	}
	return t, cmd
}

// HandlesEscape 下拉浮层展开时 esc 只收起浮层，不返回工具列表。
func (t *tool) HandlesEscape() bool	{ return t.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.header.View(t.actions()...) + "\n" + t.ws.View())
}
func (t *tool) schedule() tea.Cmd {
	t.revision++
	t.queued = false
	src := t.ws.Input.Value()
	if strings.TrimSpace(src) == "" {
		t.ws.SetError(nil)
		t.ws.SetOutput("")
		return nil
	}
	if len(src) > maxSQLBytes {
		t.ws.SetError(fmt.Errorf("SQL input exceeds 1 MiB"))
		t.ws.SetOutput("")
		return nil
	}
	t.ws.SetError(nil)
	t.ws.SetOutput("Formatting…")
	rev := t.revision
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: sqlDue{revision: rev}} })
}
func (t *tool) start(rev uint64) tea.Cmd {
	t.running = true
	src, language, leading := t.ws.Input.Value(), dialects[t.dialect].id, t.leadingComma
	indent := strings.Repeat(" ", t.indent)
	if t.indent == 0 {
		indent = "\t"
	}
	return func() tea.Msg {
		out, err := formatDialect(src, language, indent, leading)
		return toolkit.BackgroundToolMsg{Tool: t, Message: sqlDone{revision: rev, output: out, err: err}}
	}
}

func sqlDisplay(text string) string {
	text = strings.ReplaceAll(text, "\t", "    ")
	text = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\r' {
			return '␍'
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, text)
	lexer := lexers.Get("sql")
	formatter := formatters.Get("terminal256")
	style := styles.Get("github-dark")
	if lexer == nil || formatter == nil || style == nil {
		return text
	}
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return text
	}
	var display strings.Builder
	if err := formatter.Format(&display, style, iterator); err != nil {
		return text
	}
	return strings.TrimRight(display.String(), "\n")
}
