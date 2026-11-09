// Package password 实现随机密码生成工具：长度由选项栏的 Stepper 调整，
// 字符类型（大写/小写/数字/符号）为 Switch 开关，输入区可填写排除字符，
// 生成数量用独立 Stepper 调整；使用 crypto/rand 保证随机性，
// 并保证每个启用的字符类型至少出现一次。错误显示在底部信息条。
// 短屏幕下排除字符/数量行自动隐藏，保证输出与选项仍可触达。
package password

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/tools/generators/internal/optbar"
	"github.com/toy-forge/devtoys/internal/ui"
)

// 各字符类型与易混淆字符集合。
const (
	upperChars	= "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerChars	= "abcdefghijklmnopqrstuvwxyz"
	digitChars	= "0123456789"
	symbolChars	= "!@#$%^&*()-_=+[]{};:,.<>?"
	ambiguous	= "Il1O0o"

	excludedLabel		= "  Exclude characters"
	excludedLabelWidth	= 22					// 标签列宽（与 excludedLabel 对齐）
	excludedFieldX		= excludedLabelWidth + ui.Space2	// 输入框起始列（鼠标光标换算用）
)

// excludedFieldWidth 返回排除字符输入框的单元格宽度（标签列与间距之外的剩余宽度）。
func (m *Model) excludedFieldWidth() int {
	return max(1, m.width-excludedFieldX)
}

// excludedRow 渲染排除字符行：标签列定宽 + 定宽输入框（ui.FormRow + ui.FieldView）。
func (m *Model) excludedRow() string {
	return ui.FormRow(excludedLabel, excludedLabelWidth, ui.FieldView(&m.excluded, m.excludedFieldWidth(), m.excluded.Focused()), m.width)
}

// Model 是密码生成工具的 bubbletea 模型。
type Model struct {
	ws		toolkit.Workspace
	bar		optbar.Bar
	excluded	textinput.Model
	count		ui.Stepper
	width, height	int
}

// New 创建模型并生成首个密码。
func New() *Model {
	m := &Model{
		ws:		toolkit.NewWorkspace("Input", "Output"),
		excluded:	textinput.New(),
		count:		ui.Stepper{Label: "Password Generation Count", Value: 1, Min: 1, Max: 10000},
		bar: optbar.Bar{
			Numbers:	[]*optbar.Number{{Label: "Length", Value: 30, Min: 5, Max: 128}},
			Toggles: []*optbar.Toggle{
				{Label: "Uppercase characters", Description: "Use uppercase characters (ABCDEFGHIJKLMNOPQRSTUVWXYZ)", On: true},
				{Label: "Lowercase characters", Description: "Use lowercase characters (abcdefghijklmnopqrstuvwxyz)", On: true},
				{Label: "Digit characters", Description: "Use digit characters (0123456789)", On: true},
				{Label: "Special characters", Description: "Use special characters (!\"#$%&')*+,-.:;=>?@]^_}~)", On: true},
			},
			Order: []optbar.OptionRef{
				{Kind: optbar.NumberOption, Index: 0},
				{Kind: optbar.ToggleOption, Index: 1},
				{Kind: optbar.ToggleOption, Index: 0},
				{Kind: optbar.ToggleOption, Index: 2},
				{Kind: optbar.ToggleOption, Index: 3},
			},
		},
	}
	m.excluded.Prompt = "┃ "
	m.excluded.Placeholder = "Optional"
	// 占位符接 theme token，不用 bubbles 默认的 256 色 "240"
	// （与 hash.go 同一处理）。
	m.excluded.PlaceholderStyle = theme.MutedStyle()
	m.excluded.CharLimit = 4096
	m.excluded.Width = 40
	m.ws.SetPaneTitles("Input", "Output")
	m.ws.SetOutputControls("Copy", "Regenerate")
	m.regenerate()
	return m
}

func init()	{ toolkit.Register(New()) }

// AcceptsFiles 与 lorem/uuid 一致：密码生成无文件输入，状态栏不出现
// [Open file] token。
func (*Model) AcceptsFiles() bool	{ return false }

func (m *Model) ID() string		{ return "generators.password" }
func (m *Model) Title() string		{ return "Password Generator" }
func (m *Model) ShortTitle() string	{ return "Password" }
func (m *Model) Category() string	{ return "generators" }
func (m *Model) Icon() string		{ return "󰌾" }
func (m *Model) Description() string	{ return "Generate a random password" }

func (m *Model) Init() tea.Cmd	{ return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toolkit.WorkspaceActionMsg:
		switch msg.Action {
		case "Regenerate":
			m.regenerate()
		case "Copy":
			if err := m.ws.CopyOutput(); err != nil {
				m.ws.SetError(err)
			} else {
				m.ws.SetError(nil)
				m.ws.Copied = true
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.bar.SetSize(msg.Width, msg.Height)
		m.excluded.Width = max(1, m.excludedFieldWidth()-ui.Space2)
		m.resizeWorkspace()
		return m, nil

	case tea.MouseMsg:
		press := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft
		if m.bar.OverlayActive() || (msg.Y >= 0 && msg.Y < m.bar.Height()) {
			m.excluded.Blur()
			changed, regenerate := m.bar.HandleMouse(msg)
			if changed || regenerate {
				m.regenerate()
				m.resizeWorkspace()
			}
			return m, nil
		}
		barH := m.bar.Height()
		if m.extrasVisible() {
			if press && msg.Y == barH {
				// 排除字符输入行。
				m.excluded.SetCursor(max(0, msg.X-excludedFieldX-ui.Space1))
				return m, m.excluded.Focus()
			}
			if press && msg.Y == barH+1 {
				// 生成数量 Stepper。
				m.excluded.Blur()
				// 数量行缩进 Space2。
				if delta, hit := m.count.Hit(msg, ui.Space2, barH+1); hit && delta != 0 {
					m.regenerate()
				}
				return m, nil
			}
		}
		if m.excluded.Focused() {
			m.excluded.Blur()
		}

	case tea.KeyMsg:
		key := msg.String()
		if key == "alt+r" {
			m.regenerate()
			return m, nil
		}
		if m.excluded.Focused() {
			switch key {
			case "esc", "enter", "tab":
				m.excluded.Blur()
				return m, nil
			}
			before := m.excluded.Value()
			var cmd tea.Cmd
			m.excluded, cmd = m.excluded.Update(msg)
			if before != m.excluded.Value() {
				m.regenerate()
			}
			return m, cmd
		}
		if handled, changed := m.handleBarKey(key); handled {
			if changed {
				m.regenerate()
				m.resizeWorkspace()
			}
			return m, nil
		}
	}

	return m, m.ws.UpdateBelow(msg, m.workspaceTop())
}

func (m *Model) resizeWorkspace() {
	if m.width <= 0 {
		return
	}
	top := m.workspaceTop()
	m.ws.SetOutputOnly(m.height-top >= 7)
	m.ws.Update(tea.WindowSizeMsg{Width: m.width, Height: max(1, m.height-top)})
}

// extrasVisible 报告排除字符/数量行是否有空间显示。
// 隐藏时视图只保留选项栏（2 行）与紧凑工作区（5 行）。
func (m *Model) extrasVisible() bool {
	return m.width > 0 && m.height >= m.bar.Height()+8
}

func (m *Model) workspaceTop() int {
	if !m.extrasVisible() {
		return m.bar.Height()
	}
	return m.bar.Height() + 3 + m.warningHeight()
}

func (m *Model) warningHeight() int {
	if m.hasCharacterSets() {
		return 0
	}
	return 1
}

func (m *Model) hasCharacterSets() bool {
	for _, toggle := range m.bar.Toggles {
		if toggle.On {
			return true
		}
	}
	return false
}

func (m *Model) View() string {
	truncate := func(s string) string { return ansi.Truncate(s, max(1, m.width), "") }
	view := m.bar.OverlayOn(m.bar.View())
	if m.extrasVisible() {
		count := "  " + m.count.View()
		view += "\n" + truncate(m.excludedRow())
		view += "\n" + truncate(count)
		if m.warningHeight() > 0 {
			view += "\n" + truncate(lipgloss.NewStyle().Foreground(theme.YellowBright).
				Render("󰀦  No password(s) can be generated because no options have been selected."))
		}
		view += "\n\n" + m.ws.View()
		return view
	}
	return view + "\n" + m.ws.View()
}

// handleBarKey 处理选项栏聚焦/取值按键；handled 表示按键已被
// 选项栏消费，不再进入工作区。
func (m *Model) handleBarKey(key string) (handled, changed bool) {
	was := m.bar.Focused
	changed = m.bar.HandleKey(key)
	return was || m.bar.HandlesKeys(), changed
}

// regenerate 按当前选项生成新密码；失败时把错误显示在信息条并
// 保留上一次的输出。
func (m *Model) regenerate() {
	if !m.hasCharacterSets() {
		m.ws.SetError(nil)
		m.ws.SetOutput("")
		return
	}
	length, count := m.bar.Numbers[0].Value, m.count.Value
	t := m.bar.Toggles
	passwords := make([]string, count)
	for i := range passwords {
		pw, err := generate(length, t[0].On, t[1].On, t[2].On, t[3].On, false, m.excluded.Value())
		if err != nil {
			m.ws.SetError(err)
			m.ws.SetOutput("")
			return
		}
		passwords[i] = pw
	}
	m.ws.SetError(nil)
	m.ws.SetOutput(strings.Join(passwords, "\n"))
}

// generate 生成密码：每个启用的字符类型先各取一个，再从合并字符
// 集中补足长度，最后用 Fisher-Yates 洗牌。
func generate(
	length int,
	upper, lower, digit, symbol, avoidAmbiguous bool,
	exclude string,
) (string, error) {
	excl := map[rune]bool{}
	for _, r := range exclude {
		excl[r] = true
	}

	enabled := 0
	var pools []string
	add := func(on bool, chars string) {
		if !on {
			return
		}
		enabled++
		var b strings.Builder
		for _, r := range chars {
			if excl[r] {
				continue
			}
			if avoidAmbiguous && strings.ContainsRune(ambiguous, r) {
				continue
			}
			b.WriteRune(r)
		}
		if s := b.String(); s != "" {
			pools = append(pools, s)
		}
	}
	add(upper, upperChars)
	add(lower, lowerChars)
	add(digit, digitChars)
	add(symbol, symbolChars)

	switch {
	case enabled == 0:
		return "", errors.New("enable at least one character type")
	case len(pools) == 0:
		return "", errors.New("no characters available; check exclusions")
	case length < len(pools):
		return "", fmt.Errorf("length %d cannot include every enabled character type (minimum %d)", length, len(pools))
	}

	all := strings.Join(pools, "")
	out := make([]byte, 0, length)
	for _, p := range pools {
		i, err := randInt(len(p))
		if err != nil {
			return "", fmt.Errorf("random generation failed: %w", err)
		}
		out = append(out, p[i])
	}
	for len(out) < length {
		i, err := randInt(len(all))
		if err != nil {
			return "", fmt.Errorf("random generation failed: %w", err)
		}
		out = append(out, all[i])
	}

	// Fisher-Yates 洗牌，避免每类字符固定出现在开头。
	for i := len(out) - 1; i > 0; i-- {
		j, err := randInt(i + 1)
		if err != nil {
			return "", fmt.Errorf("random generation failed: %w", err)
		}
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

// randInt 返回 [0, n) 内的密码学安全随机数。
func randInt(n int) (int, error) {
	if n <= 1 {
		return 0, nil
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}
