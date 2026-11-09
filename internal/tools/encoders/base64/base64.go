// Package base64 提供 Base64 编解码工具：
// 编码/解码方向随时切换，支持 UTF-8/ASCII 与逐行处理；
// 输入即时重算，错误显示在信息条。
package base64

import (
	"encoding/base64"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// tool 是 Base64 工具模型，复用 toolkit.Workspace 双栏布局。
type tool struct {
	toolkit.Workspace
	header	toolkit.SettingHeader

	encode		bool	// true=编码，false=解码
	urlSafe		bool	// true=URL-safe 字母表（-、_）
	pad		bool	// true=输出补齐 = padding
	ascii		bool
	multiline	bool
}

// newTool 创建工具实例并在 init 中注册。
func newTool() *tool {
	t := &tool{
		Workspace:	toolkit.NewWorkspace("Enter text to encode or decode…", ""),
		encode:		true,
		urlSafe:	false,
		pad:		true,
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Conversion", "Encoding", "Multiline"},
			Descriptions:	[]string{"Select which conversion mode you want to use", "Select which encoding you want to use", "Whether to encode / decode each line separately"},
			Icons:		[]string{"", "󰌆", "󰦝"},
			Switches:	[]bool{true, false, true},
			Options:	[][]string{nil, {"UTF-8", "ASCII"}, nil},
			Groups:		[]toolkit.SettingGroup{{Title: "Conversion", Icon: "󰦝", Primary: 0, Children: []int{1, 2}}},
		},
	}
	t.SetStacked(true)
	t.Workspace.SetOutputExtendable(true)
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newTool()) }

func (t *tool) ID() string		{ return "encoders.base64" }
func (t *tool) Title() string		{ return "Base64 Text Encoder / Decoder" }
func (t *tool) ShortTitle() string	{ return "Base64 Text" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰌆" }
func (t *tool) Description() string {
	return "Encode and decode Base64 text with UTF-8 or ASCII encoding"
}

func (t *tool) Init() tea.Cmd	{ return nil }

// Update 处理本工具的选项快捷键，其余按键转发给公共工作区。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 尺寸消息必须先于 header.Index 处理：Index 会以当前宽度初始化
	// 共享按钮条，零宽度初始化会让后续渲染滞留在逐行换行的旧布局上。
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		// 极矮终端放弃设置条与内容区之间的空行，保证 View 行数==终端高度。
		if sz.Height <= 8 {
			t.header.ContentGap = 0
		} else {
			t.header.ContentGap = 1
		}
		return t, t.header.Resize(sz, &t.Workspace, t.actions()...)
	}
	setting, choice, handled := t.header.Pick(msg, t.actions()...)
	if handled && setting < 0 {
		// 字符集下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if handled && setting == 1 && choice >= 0 {
		t.ascii = choice == 1
		t.relayoutHeader()
		t.recalc()
		return t, nil
	}
	switch {
	case handled && setting == 0:
		msg = tea.KeyMsg{Type: tea.KeyCtrlT}
	case handled && setting == 1:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}, Alt: true}
	case handled && setting == 2:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}, Alt: true}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+t":	// 切换 编码 ⇄ 解码
			t.encode = !t.encode
			t.relayoutHeader()
			t.recalc()
			return t, nil
		case "alt+e":
			// 打开字符集下拉框；设置条不可展开（紧凑布局）时退回切换。
			if s, _, _ := t.header.Shortcut(1, t.actions()...); s < 0 {
				return t, nil
			}
			t.ascii = !t.ascii
			t.relayoutHeader()
			t.recalc()
			return t, nil
		case "alt+l":
			t.multiline = !t.multiline
			t.relayoutHeader()
			t.recalc()
			return t, nil
		}
	}
	cmd := t.Workspace.UpdateBelow(msg, t.header.Height(t.actions()...))
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.Workspace.Err != nil {
		return t, cmd
	}
	t.recalc()
	return t, cmd
}

// actions 返回设置条各选项的当前取值。
func (t *tool) actions() []string {
	direction, charset, lines := "Decode", "UTF-8", "Off"
	if t.encode {
		direction = "Encode"
	}
	if t.ascii {
		charset = "ASCII"
	}
	if t.multiline {
		lines = "On"
	}
	return []string{direction, charset, lines}
}

// HandlesEscape 字符集下拉浮层展开时 esc 只收起浮层。
func (t *tool) HandlesEscape() bool	{ return t.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.header.View(t.actions()...) + "\n" + t.Workspace.View())
}

// relayoutHeader 在选项值变化后刷新设置条布局。
// SettingHeader 的紧凑回退按钮条由 ActionHeader 共享，只有指针方法会
// 刷新按钮，因此先用一次无副作用的命中查询刷新缓存，再重排工作区。
func (t *tool) relayoutHeader() {
	t.header.Index(tea.MouseMsg{Action: tea.MouseActionRelease}, t.actions()...)
	t.header.Relayout(&t.Workspace, t.actions()...)
}

// recalc 依据当前输入与选项即时重算输出，错误写入信息条。
func (t *tool) recalc() {
	in := t.Input.Value()
	if in == "" {
		t.SetError(nil)
		t.SetOutput("")
		return
	}
	convert := func(input string) (string, error) {
		if t.encode {
			if t.ascii {
				input = strings.Map(func(r rune) rune {
					if r > 127 {
						return '?'
					}
					return r
				}, input)
			}
			enc := base64.StdEncoding
			if t.urlSafe {
				enc = base64.URLEncoding
			}
			if !t.pad {
				enc = enc.WithPadding(base64.NoPadding)
			}
			return enc.EncodeToString([]byte(input)), nil
		}
		decoded, err := decodeBase64(stripSpace(input))
		if err != nil {
			return "", err
		}
		if t.ascii {
			for i, b := range decoded {
				if b > 127 {
					decoded[i] = '?'
				}
			}
		}
		return string(decoded), nil
	}
	var out string
	var err error
	if !t.multiline {
		out, err = convert(in)
	} else {
		var result strings.Builder
		for line := range strings.SplitAfterSeq(in, "\n") {
			ending := ""
			if strings.HasSuffix(line, "\n") {
				line = strings.TrimSuffix(line, "\n")
				ending = "\n"
				if strings.HasSuffix(line, "\r") {
					line = strings.TrimSuffix(line, "\r")
					ending = "\r\n"
				}
			}
			var converted string
			converted, err = convert(line)
			if err != nil {
				break
			}
			result.WriteString(converted)
			result.WriteString(ending)
		}
		out = result.String()
	}
	if err != nil {
		t.SetError(err)
		t.SetOutput("")
		return
	}
	t.SetError(nil)
	t.SetOutput(out)
}

// decodeBase64 依次尝试四种 Base64 变体解码，全部失败才报错。
func decodeBase64(s string) ([]byte, error) {
	encs := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encs {
		b, err := enc.DecodeString(s)
		if err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("invalid Base64 data (standard and URL-safe alphabets tried)")
}

// stripSpace 去掉字符串中的所有空白字符。
func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
}
