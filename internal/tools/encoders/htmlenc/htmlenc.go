// Package htmlenc 提供 HTML 实体转义与还原工具：
// 使用标准库 html 包处理 &amp; &lt; &gt; 等实体，输入即时重算。
package htmlenc

import (
	"html"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// tool 是 HTML 转义工具模型，复用 toolkit.Workspace 双栏布局。
type tool struct {
	toolkit.Workspace
	header	toolkit.SettingHeader

	encode	bool	// true=转义，false=还原
}

// newTool 创建工具实例并在 init 中注册。
func newTool() *tool {
	t := &tool{
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Conversion"},
			Descriptions:	[]string{"Select which conversion mode you want to use"},
			Icons:		[]string{"󰦝"},
			Switches:	[]bool{true},
		},
		Workspace:	toolkit.NewWorkspace("Enter HTML text to escape or unescape…", ""),
		encode:		true,
	}
	t.SetStacked(true)
	t.Workspace.SetOutputExtendable(true)
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newTool()) }

func (t *tool) ID() string		{ return "encoders.htmlenc" }
func (t *tool) Title() string		{ return "HTML Text Encoder / Decoder" }
func (t *tool) ShortTitle() string	{ return "HTML" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰗀" }
func (t *tool) Description() string	{ return "Escape and unescape HTML entities" }

func (t *tool) Init() tea.Cmd	{ return nil }

// Update 处理本工具的选项快捷键，其余按键转发给公共工作区。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.header.Index(msg, t.action()) == 0 {
		msg = tea.KeyMsg{Type: tea.KeyCtrlT}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+t":	// 切换 转义 ⇄ 还原
			t.encode = !t.encode
			t.Input.SetValue(t.OutputText())
			t.recalc()
			return t, nil
		}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return t, t.header.Resize(sz, &t.Workspace, t.action())
	}
	cmd := t.Workspace.UpdateBelow(msg, t.header.Height(t.action()))
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.Workspace.Err != nil {
		return t, cmd
	}
	t.recalc()
	return t, cmd
}

// View 渲染顶部选项栏 + 双栏工作区。
func (t *tool) action() string {
	if t.encode {
		return "Encode"
	}
	return "Decode"
}
func (t *tool) View() string	{ return t.header.View(t.action()) + "\n" + t.Workspace.View() }

// recalc 依据当前输入与方向即时重算输出。
// html.UnescapeString 对非法实体宽容处理（原样保留），不会报错。
func (t *tool) recalc() {
	in := t.Input.Value()
	if in == "" {
		t.SetError(nil)
		t.SetOutput("")
		return
	}
	t.SetError(nil)
	if t.encode {
		t.SetOutput(html.EscapeString(in))
		return
	}
	t.SetOutput(html.UnescapeString(in))
}
