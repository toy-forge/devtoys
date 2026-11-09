// Package numberbase 实现数字进制转换工具。
// 支持 2/8/10/16 以及任意 2-36 进制互转，内部使用 math/big
// 支持任意大整数；输入即时重算，出错显示在信息条。
//
// 附加按键：alt+[ / alt+] 切换源进制，alt+, / alt+. 切换目标进制。
package numberbase

import (
	"fmt"
	"math/big"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// baseOrder 是按键循环切换进制时的顺序：常用四进制在前，其余升序。
var baseOrder = func() []int {
	out := []int{2, 8, 10, 16}
	for b := 3; b <= 36; b++ {
		if b == 8 || b == 10 || b == 16 {
			continue
		}
		out = append(out, b)
	}
	return out
}()

// baseName 返回进制的中文显示名。
func baseName(b int) string {
	switch b {
	case 2:
		return "Binary"
	case 8:
		return "Octal"
	case 10:
		return "Decimal"
	case 16:
		return "Hexadecimal"
	default:
		return fmt.Sprintf("Base %d", b)
	}
}

// model 持有公共工作区、进制选择与上一次的输入内容。
type model struct {
	header	toolkit.ActionHeader
	ws	toolkit.Workspace
	from	int	// 源进制
	to	int	// 目标进制
	before	string	// 上一次触发重算时的输入内容
}

func init()	{ toolkit.Register(newConverter()) }

func (m *model) ID() string		{ return "converters.numberbase" }
func (m *model) Title() string		{ return "Number Base Converter" }
func (m *model) ShortTitle() string	{ return "Number Base" }
func (m *model) Category() string	{ return "converters" }
func (m *model) Icon() string		{ return "[N]" }
func (m *model) Description() string {
	return "Convert large integers between bases 2–36"
}

func (m *model) Init() tea.Cmd	{ return nil }

// Update 拦截进制切换按键，其余转发给工作区；输入变化时即时重算。
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.header.Index(msg, m.actions()...) {
	case 0:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}, Alt: true}
	case 1:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true}
	case 2:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}, Alt: true}
	case 3:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}, Alt: true}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return m, m.header.Resize(sz, &m.ws, m.actions()...)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+[":
			m.from = cycleBase(m.from, -1)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		case "alt+]":
			m.from = cycleBase(m.from, 1)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		case "alt+,":
			m.to = cycleBase(m.to, -1)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		case "alt+.":
			m.to = cycleBase(m.to, 1)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		}
	}

	cmd := m.ws.UpdateBelow(msg, m.header.Height(m.actions()...))
	if cur := m.ws.Input.Value(); cur != m.before {
		m.before = cur
		m.recompute(cur)
	}
	return m, cmd
}

// View 渲染进制选择头部与工作区。
func (m *model) actions() []string {
	return []string{"Source <", fmt.Sprintf("%d >", m.from), "Target <", fmt.Sprintf("%d >", m.to)}
}
func (m *model) View() string	{ return m.header.View(m.actions()...) + "\n" + m.ws.View() }

// cycleBase 在 baseOrder 中按方向 step（+1/-1）循环切换当前进制。
func cycleBase(cur, step int) int {
	for i, b := range baseOrder {
		if b == cur {
			j := (i + step + len(baseOrder)) % len(baseOrder)
			return baseOrder[j]
		}
	}
	return 10
}

// recompute 解析输入并转换到目标进制，同时给出常用进制参考。
func (m *model) recompute(input string) {
	m.ws.SetError(nil)

	raw := strings.TrimSpace(input)
	if raw == "" {
		m.ws.SetOutput("Waiting for input…")
		return
	}

	// 去除数字中允许的分隔下划线后再解析。
	clean := strings.ReplaceAll(raw, "_", "")
	if err := validateDigits(clean, m.from); err != nil {
		m.ws.SetError(err)
		return
	}

	n, ok := new(big.Int).SetString(clean, m.from)
	if !ok {
		m.ws.SetError(fmt.Errorf("cannot parse input as %s", baseName(m.from)))
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s → %s：\n%s", baseName(m.from), baseName(m.to), n.Text(m.to))
	// 常用进制表：进制名（取最长名称宽）/ 数值（右对齐的纯文本，可直接复制）。
	type commonBase struct{ label, value string }
	var common []commonBase
	for _, base := range []int{2, 8, 10, 16} {
		label := baseName(base)
		if base == m.to {
			label += " (target)"
		}
		common = append(common, commonBase{label, n.Text(base)})
	}
	labelWidth := 0
	for _, c := range common {
		labelWidth = max(labelWidth, len(c.label))
	}
	b.WriteString("\n\nCommon bases:")
	for _, c := range common {
		fmt.Fprintf(&b, "\n%-*s : %s", labelWidth, c.label, c.value)
	}
	m.ws.SetOutput(b.String())
}

// validateDigits 检查字符串是否为 base 进制的合法数字，
// 允许前置正负号；出错时返回带具体字符位置的信息。
func validateDigits(s string, base int) error {
	if s == "" {
		return fmt.Errorf("empty input cannot be parsed as %s", baseName(base))
	}
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		v := digitValue(r)
		if v < 0 {
			return fmt.Errorf("character %d (%q) is not a digit", i+1, string(r))
		}
		if v >= base {
			return fmt.Errorf("character %d (%q) is outside %s (maximum %c)",
				i+1, string(r), baseName(base), digitChar(base-1))
		}
	}
	return nil
}

// digitValue 返回字符的数值；非法字符返回 -1。
func digitValue(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'z':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'Z':
		return int(r-'A') + 10
	default:
		return -1
	}
}

// digitChar 返回数值对应的数字字符（数值须在 0-35 之间）。
func digitChar(v int) rune {
	return rune("0123456789abcdefghijklmnopqrstuvwxyz"[v])
}
