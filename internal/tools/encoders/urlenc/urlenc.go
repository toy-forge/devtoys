// Package urlenc 提供 URL 编解码工具：
// 区分"Full URL"（按 scheme/host/path/query/fragment 分段转义）
// 与"Component"（整段 QueryEscape/QueryUnescape）两种作用范围。
package urlenc

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// scope 表示编码/解码的作用范围。
type scope int

const (
	scopeFullURL	scope	= iota	// 完整 URL：分段解析后逐段转义
	scopeComponent			// 组件：整段文本按一个组件处理
)

// tool 是 URL 编解码工具模型，复用 toolkit.Workspace 双栏布局。
type tool struct {
	toolkit.Workspace
	header	toolkit.SettingHeader

	encode		bool	// true=编码，false=解码
	multiline	bool
	scope		scope	// 作用范围：完整 URL / 组件
}

var (
	structKeyStyle	= lipgloss.NewStyle().Foreground(theme.Accent)	// 结构化输出的字段名
	structValStyle	= lipgloss.NewStyle().Foreground(theme.Text)
	structHintStye	= lipgloss.NewStyle().Foreground(theme.Muted)
)

// newTool 创建工具实例并在 init 中注册。
func newTool() *tool {
	t := &tool{
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Conversion", "Encoding / Decoding Multiline"},
			Descriptions:	[]string{"Select which conversion mode you want to use", "Encode / Decode each line separately"},
			Icons:		[]string{"󰦝", "󰦝"},
			Switches:	[]bool{true, true},
		},
		Workspace:	toolkit.NewWorkspace("Enter a URL or text to encode…", ""),
		encode:		true,
		scope:		scopeComponent,
	}
	t.SetStacked(true)
	t.Workspace.SetOutputExtendable(true)
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newTool()) }

func (t *tool) ID() string		{ return "encoders.urlenc" }
func (t *tool) Title() string		{ return "URL Encoder / Decoder" }
func (t *tool) ShortTitle() string	{ return "URL" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰦝" }
func (t *tool) Description() string {
	return "Percent-encode a full URL or component"
}

func (t *tool) Init() tea.Cmd	{ return nil }

// Update 处理本工具的选项快捷键，其余按键转发给公共工作区。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t.header.Index(msg, t.actions()...) {
	case 0:
		msg = tea.KeyMsg{Type: tea.KeyCtrlT}
	case 1:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}, Alt: true}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+t":	// 切换 编码 ⇄ 解码
			t.encode = !t.encode
			t.Input.SetValue(t.OutputText())
			t.header.Relayout(&t.Workspace, t.actions()...)
			t.recalc()
			return t, nil
		case "alt+l":
			t.multiline = !t.multiline
			t.header.Relayout(&t.Workspace, t.actions()...)
			t.recalc()
			return t, nil
		case "ctrl+o":	// 切换 完整 URL ⇄ 组件
			if t.scope == scopeFullURL {
				t.scope = scopeComponent
			} else {
				t.scope = scopeFullURL
			}
			t.header.Relayout(&t.Workspace, t.actions()...)
			t.recalc()
			return t, nil
		}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return t, t.header.Resize(sz, &t.Workspace, t.actions()...)
	}
	cmd := t.Workspace.UpdateBelow(msg, t.header.Height(t.actions()...))
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.Workspace.Err != nil {
		return t, cmd
	}
	t.recalc()
	return t, cmd
}

// View 渲染顶部选项栏 + 双栏工作区。
func (t *tool) actions() []string {
	direction, lines := "Decode", "Off"
	if t.encode {
		direction = "Encode"
	}
	if t.multiline {
		lines = "On"
	}
	return []string{direction, lines}
}
func (t *tool) View() string	{ return t.header.View(t.actions()...) + "\n" + t.Workspace.View() }

// recalc 依据当前输入与选项即时重算输出，错误写入信息条。
func (t *tool) recalc() {
	in := t.Input.Value()
	if strings.TrimSpace(in) == "" {
		t.SetError(nil)
		t.SetOutput("")
		return
	}

	convert := func(input string) (string, error) {
		if t.scope == scopeComponent {
			if t.encode {
				return url.QueryEscape(input), nil
			}
			out, err := url.QueryUnescape(input)
			if err != nil {
				return "", fmt.Errorf("invalid URL encoding: %w", err)
			}
			return out, nil
		}
		if t.encode {
			return encodeFullURL(input)
		}
		return decodeFullURL(input)
	}
	var out string
	var err error
	if !t.multiline {
		out, err = convert(in)
	} else {
		var result strings.Builder
		lineNumber := 0
		for line := range strings.SplitAfterSeq(in, "\n") {
			lineNumber++
			ending := ""
			content, hasNewline := strings.CutSuffix(line, "\n")
			if hasNewline {
				ending = "\n"
				if trimmed, hasCR := strings.CutSuffix(content, "\r"); hasCR {
					content, ending = trimmed, "\r\n"
				}
			} else {
				content = line
			}
			var converted string
			converted, err = convert(content)
			if err != nil {
				err = fmt.Errorf("line %d: %w", lineNumber, err)
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

// encodeFullURL 解析完整 URL 并逐段百分号转义：
// 路径按 / 分段用 PathEscape，查询参数用 QueryEncode，片段保留合法转义。
func encodeFullURL(in string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(in))
	if err != nil {
		return "", fmt.Errorf("cannot parse full URL (%v); press ctrl+o for component mode", err)
	}
	if u.Scheme == "" {
		return "", fmt.Errorf("missing URL scheme (e.g. https://); press ctrl+o for component mode")
	}

	var b strings.Builder
	b.WriteString(u.Scheme)
	if u.Opaque != "" {
		// 不透明 URL（如 mailto:x）没有路径/查询结构，原样保留。
		b.WriteString(":")
		b.WriteString(u.Opaque)
		return b.String(), nil
	}
	if u.Host != "" {
		b.WriteString("://")
		if u.User != nil {
			b.WriteString(u.User.String())
			b.WriteString("@")
		}
		b.WriteString(u.Host)
	}
	// 路径逐段转义，保留分隔符 / 本身。
	if u.Path != "" {
		segs := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		escaped := make([]string, 0, len(segs))
		for _, seg := range segs {
			escaped = append(escaped, url.PathEscape(seg))
		}
		b.WriteString("/")
		b.WriteString(strings.Join(escaped, "/"))
	}
	if u.ForceQuery || u.RawQuery != "" {
		q, qerr := url.ParseQuery(u.RawQuery)
		if qerr != nil {
			return "", fmt.Errorf("cannot parse query (%v); press ctrl+o for component mode", qerr)
		}
		b.WriteString("?")
		b.WriteString(q.Encode())
	}
	if u.Fragment != "" {
		b.WriteString("#")
		b.WriteString(u.EscapedFragment())
	}
	return b.String(), nil
}

// decodeFullURL 解析完整 URL，输出解码后的结构化组件。
func decodeFullURL(in string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(in))
	if err != nil {
		return "", fmt.Errorf("cannot parse full URL (%v); press ctrl+o for component mode", err)
	}
	if u.Scheme == "" {
		return "", fmt.Errorf("missing URL scheme (e.g. https://); press ctrl+o for component mode")
	}

	// 组件表：字段名列（取最长字段名）/ 取值列（取最长取值，不截断）。查询参数
	// 在 query 行之下以树形符号列出，键值对已解码。
	type component struct{ name, value string }
	var parts []component
	add := func(name, val string) { parts = append(parts, component{name, val}) }
	add("scheme", u.Scheme)
	if u.Opaque != "" {
		add("opaque", u.Opaque)
	}
	if u.Host != "" {
		add("host", u.Host)
	}
	if u.Path != "" {
		add("path", u.Path)
	}
	if u.RawQuery != "" {
		add("query", u.RawQuery)
		// 进一步拆解查询参数，展示解码后的键值对。
		if q, qerr := url.ParseQuery(u.RawQuery); qerr == nil {
			keys := make([]string, 0, len(q))
			for k := range q {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				for _, v := range q[k] {
					parts = append(parts, component{"  ├─", k + " = " + v})
				}
			}
		}
	}
	if u.Fragment != "" {
		add("fragment", u.Fragment)
	}
	nameWidth, valueWidth := 0, 0
	for _, part := range parts {
		nameWidth = max(nameWidth, ui.Width(part.name))
		valueWidth = max(valueWidth, ui.Width(part.value))
	}
	table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{{Width: nameWidth}, {Width: valueWidth}}}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		name := structKeyStyle.Render(part.name)
		if strings.HasPrefix(part.name, "  ├─") {
			name = structHintStye.Render(part.name)
		}
		_, line := table.Row(name, structValStyle.Render(part.value))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}
