// Package jsonyaml implements the bidirectional JSON/YAML converter.
package jsonyaml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// model 持有公共工作区与上一次的输入内容，用于检测输入变化。
type model struct {
	ws	toolkit.Workspace
	before	string	// 上一次触发重算时的输入内容
	header	toolkit.SettingHeader
	mode	int
	indent	int
}

func init() {
	toolkit.Register(newTool())
}

// newTool 创建工具模型并注册到 toolkit。
func newTool() *model {
	m := &model{
		ws:	toolkit.NewWorkspace("Paste JSON or YAML…", ""),
		mode:	0,	// 自动检测方向
		indent:	2,
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Conversion", "Indentation"},
			Descriptions:	[]string{"Select which conversion mode you want to use", "Select which indentation you want to use"},
			Icons:		[]string{"󰦝", "󰉶"},
			Options:	[][]string{{"Auto", "JSON to YAML", "YAML to JSON"}, {"2 spaces", "4 spaces"}},
		},
	}
	m.ws.SetOutputExtendable(true)
	return m
}

func (m *model) ID() string		{ return "converters.jsonyaml" }
func (m *model) Title() string		{ return "JSON <> YAML Converter" }
func (m *model) ShortTitle() string	{ return "JSON <> YAML" }
func (m *model) Category() string	{ return "converters" }
func (m *model) Icon() string		{ return "⇄" }
func (m *model) Description() string	{ return "Convert between JSON and YAML" }

func (m *model) Init() tea.Cmd	{ return nil }

func (m *model) actions() []string {
	modes := []string{"Auto", "JSON to YAML", "YAML to JSON"}
	return []string{modes[m.mode], fmt.Sprintf("%d spaces", m.indent)}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		// 极矮终端放弃设置条与内容区之间的空行，保证 View 行数==终端高度。
		if sz.Height <= 8 {
			m.header.ContentGap = 0
		} else {
			m.header.ContentGap = 1
		}
		return m, m.header.Resize(sz, &m.ws, m.actions()...)
	}
	if imported, ok := msg.(toolkit.FileImportMsg); ok {
		var value any
		if json.Valid([]byte(imported.Text)) {
			m.mode = 1
		} else if yaml.Unmarshal([]byte(imported.Text), &value) == nil {
			m.mode = 2
		}
	}
	changed := false
	conversionChanged := false
	setting, choice, handled := m.header.Pick(msg, m.actions()...)
	if handled && setting < 0 {
		// 下拉框展开/收起/选择期间，事件全部由浮层消费。
		return m, nil
	}
	if handled {
		switch {
		case setting == 0 && choice >= 0:
			m.mode = choice
			changed, conversionChanged = true, true
		case setting == 0:
			// 紧凑布局的回退按钮条：点击即轮换方向。
			m.mode = (m.mode + 1) % 3
			changed, conversionChanged = true, true
		case setting == 1 && choice >= 0:
			m.indent = []int{2, 4}[choice]
			changed = true
		case setting == 1:
			if m.indent == 2 {
				m.indent = 4
			} else {
				m.indent = 2
			}
			changed = true
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+t":
			// 打开转换方向下拉框；设置条不可展开（紧凑布局）时退回切换。
			if s, _, _ := m.header.Shortcut(0, m.actions()...); s != 0 {
				break
			}
			if m.mode == 1 {
				m.mode = 2
			} else {
				m.mode = 1
			}
			changed = true
			conversionChanged = true
		case "alt+i":
			// 打开缩进下拉框；设置条不可展开（紧凑布局）时退回切换。
			if s, _, _ := m.header.Shortcut(1, m.actions()...); s != 1 {
				break
			}
			if m.indent == 2 {
				m.indent = 4
			} else {
				m.indent = 2
			}
			changed = true
		}
	}
	if changed {
		if conversionChanged && m.before != "" && m.ws.Err == nil {
			m.ws.Input.SetValue(m.ws.OutputText())
			m.before = m.ws.Input.Value()
		}
		m.header.Relayout(&m.ws, m.actions()...)
		m.recompute(m.ws.Input.Value())
		return m, nil
	}
	cmd := m.ws.UpdateBelow(msg, m.header.Height(m.actions()...))
	if cur := m.ws.Input.Value(); cur != m.before {
		m.before = cur
		m.recompute(cur)
	}
	return m, cmd
}

// HandlesEscape 下拉浮层展开时 esc 只收起浮层，不返回工具列表。
func (m *model) HandlesEscape() bool	{ return m.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (m *model) View() string {
	return m.header.Overlay(m.header.View(m.actions()...) + "\n" + m.ws.View())
}

// recompute converts the input using the selected direction.
func (m *model) recompute(input string) {
	m.ws.SetError(nil)

	if strings.TrimSpace(input) == "" {
		m.ws.SetOutput("Waiting for input…")
		return
	}

	// Parse JSON first when explicitly converting JSON to YAML. Mode zero remains
	// an internal auto-detect fallback for compatibility with direct callers.
	var jsonVal any
	jsonErr := json.Unmarshal([]byte(input), &jsonVal)
	if m.mode == 1 && jsonErr != nil {
		m.ws.SetError(fmt.Errorf("invalid JSON: %w", jsonErr))
		return
	}
	if m.mode != 2 && jsonErr == nil {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(m.indent)
		err := enc.Encode(jsonVal)
		_ = enc.Close()
		out := buf.Bytes()
		if err != nil {
			m.ws.SetError(fmt.Errorf("YAML conversion failed: %w", err))
			return
		}
		text := string(out)
		m.ws.SetOutputDisplay(text, highlight(text, "yaml"))
		return
	}

	// 否则按 YAML 解析，方向为 YAML → JSON。
	var yamlVal any
	yamlErr := yaml.Unmarshal([]byte(input), &yamlVal)
	if yamlErr != nil {
		if m.mode == 2 {
			m.ws.SetError(fmt.Errorf("invalid YAML: %w", yamlErr))
			return
		}
		m.ws.SetError(fmt.Errorf("could not detect format: %s", bothErrors(input, jsonErr, yamlErr)))
		return
	}

	out, err := json.MarshalIndent(yamlVal, "", strings.Repeat(" ", m.indent))
	if err != nil {
		m.ws.SetError(fmt.Errorf("JSON conversion failed: %w", err))
		return
	}
	text := string(out)
	m.ws.SetOutputDisplay(text, highlight(text, "json"))
}

func highlight(src, language string) string {
	lexer := lexers.Get(language)
	formatter := formatters.Get("terminal256")
	style := styles.Get("github-dark")
	if lexer == nil || formatter == nil || style == nil {
		return src
	}
	iterator, err := lexer.Tokenise(nil, src)
	if err != nil {
		return src
	}
	var display strings.Builder
	if err := formatter.Format(&display, style, iterator); err != nil {
		return src
	}
	return strings.TrimRight(display.String(), "\n")
}

// bothErrors 汇总两个方向的解析错误，附上 JSON 出错的行列位置。
func bothErrors(input string, jsonErr, yamlErr error) string {
	var b strings.Builder
	if se, ok := jsonErr.(*json.SyntaxError); ok {
		line, col := lineColAt(input, int(se.Offset))
		fmt.Fprintf(&b, "JSON syntax error at line %d, column %d: %s", line, col, se.Error())
	} else if te, ok := jsonErr.(*json.UnmarshalTypeError); ok {
		line, col := lineColAt(input, int(te.Offset))
		fmt.Fprintf(&b, "JSON type error at line %d, column %d: %s", line, col, te.Error())
	} else {
		fmt.Fprintf(&b, "JSON parse error: %v", jsonErr)
	}
	fmt.Fprintf(&b, "; YAML parse error: %v", yamlErr)
	return b.String()
}

// lineColAt 把字节偏移换算为 1 起始的行号与列号。
func lineColAt(s string, offset int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < offset && i < len(s); i++ {
		if s[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
