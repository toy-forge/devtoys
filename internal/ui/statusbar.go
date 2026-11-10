// statusbar.go 是单行状态/帮助条：左侧上下文 token、右侧按键提示，
// 可嵌入 Toast 显示位。整条 bg=Surface（E1 底座，与画布分层），
// 严格执行 1 行合同——它是 app.View "总行数 = 终端高度" 的最后一行。
// 渲染与命中共享同一 token 序列：token 间距固定 Space2、左侧前导 1 列
// 内边距，宿主（app.statusActions）按同一算术换算可点击区域。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// StatusHint 是一条右侧按键提示。
type StatusHint struct {
	Key	string	// 如 "ctrl+y"
	Desc	string	// 如 "copy"
}

// StatusToken 是状态栏左侧的一个结构化 token。
type StatusToken struct {
	Label		string
	Clickable	bool	// 可点击 token 渲染为 Accent+bold（链接语义）
}

// StatusBar 是单行状态条。
type StatusBar struct {
	// Left 是左侧上下文文本（自动加粗、前补一列内边距）。
	// SetLeftTokens 之后被忽略。
	Left	string
	// Hints 是右侧按键提示（与 Right 二选一，Hints 优先）。
	Hints	[]StatusHint
	// Right 是右侧原始文本（Hints 为空时使用，后补一列内边距）。
	Right	string

	tokens	[]StatusToken
	toast	*Toast
	width	int
}

// NewStatusBar 创建空状态条。
func NewStatusBar() *StatusBar	{ return &StatusBar{} }

// SetWidth 记录可用宽度（每行严格铺满）。
func (s *StatusBar) SetWidth(width int)	{ s.width = max(0, width) }

// SetLeft 更新左侧文本（等价于单个 Clickable=false 的 token）。
func (s *StatusBar) SetLeft(text string) {
	s.Left = text
	s.tokens = nil
}

// SetLeftTokens 更新左侧结构化 token 序列（渲染与命中共享同一序列）。
func (s *StatusBar) SetLeftTokens(tokens []StatusToken)	{ s.tokens = tokens }

// SetHints 更新右侧按键提示。
func (s *StatusBar) SetHints(hints ...StatusHint)	{ s.Hints = hints }

// SetRight 更新右侧原始文本并清空按键提示。
func (s *StatusBar) SetRight(text string) {
	s.Right = text
	s.Hints = nil
}

// SetToast 把通知嵌入状态栏（渲染期间替换右侧提示）。
func (s *StatusBar) SetToast(t Toast)	{ s.toast = &t }

// ClearToast 清除嵌入的通知。
func (s *StatusBar) ClearToast()	{ s.toast = nil }

// surfaceSeg 渲染一段带 Surface 底的文本。组合行的每一段都必须自带
// 底色：内层样式 reset 会取消外层背景（与 DimLine 同一问题域）。
func surfaceSeg(fg theme.Color, bold bool, text string) string {
	st := lipgloss.NewStyle().Foreground(fg).Background(theme.Surface)
	if bold {
		st = st.Bold(true)
	}
	return st.Render(text)
}

// leftView 渲染左侧 token 序列：" " + token0 + "  " + token1 + ...，
// 前导 1 列、token 间 Space2——宿主命中算术（x+=width+Space2）的依据。
func (s *StatusBar) leftView() string {
	tokens := s.tokens
	if tokens == nil && strings.TrimSpace(s.Left) != "" {
		tokens = []StatusToken{{Label: s.Left}}
	}
	if len(tokens) == 0 && strings.TrimSpace(s.Left) == "" {
		return surfaceSeg(theme.Text, false, " ")
	}
	parts := make([]string, 0, 2*len(tokens)+1)
	parts = append(parts, surfaceSeg(theme.Text, false, " "))
	for i, token := range tokens {
		if i > 0 {
			parts = append(parts, surfaceSeg(theme.Text, false, strings.Repeat(" ", Space2)))
		}
		if token.Clickable {
			// 可点击 token：Accent 粗体链式样式（无 Control 胶囊），与右侧 help 行高一致。
			parts = append(parts, lipgloss.NewStyle().Foreground(theme.Accent).Background(theme.Surface).Bold(true).Render(token.Label))
			continue
		}
		parts = append(parts, surfaceSeg(theme.Text, true, token.Label))
	}
	return strings.Join(parts, "")
}

// hintsView 用 bubbles/help 渲染右侧按键提示。宽度不够时从末尾逐条丢弃
// 整条提示，保证每条提示要么完整显示、要么不显示（不在词中间截断）。
func (s *StatusBar) hintsView(room int) string {
	h := help.New()
	h.ShortSeparator = " · "
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(theme.Text)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(theme.Muted)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(theme.Muted)
	bindings := make([]key.Binding, 0, len(s.Hints))
	for _, hint := range s.Hints {
		bindings = append(bindings, key.NewBinding(key.WithKeys(hint.Key), key.WithHelp(hint.Key, hint.Desc)))
	}
	for n := len(bindings); n > 0; n-- {
		if view := h.ShortHelpView(bindings[:n]); Width(view) <= room {
			return view
		}
	}
	return ""
}

// rightView 渲染右侧内容：Toast 优先，其次按键提示，最后是原始文本。
// room 是右侧可用列数，供提示按整条裁剪。
func (s *StatusBar) rightView(room int) string {
	if s.toast != nil {
		// Toast pill 自带语义底色；外层只补 Surface，pill 内部分段不受影响。
		return withBackground(s.toast.View(), theme.Surface)
	}
	if len(s.Hints) > 0 {
		// 段内的底色由 withBackground 注入（内层 reset 会取消外层背景）。
		return withBackground(s.hintsView(room), theme.Surface)
	}
	// Right 可能是宿主传入的富文本（如 Toolbar View，按钮间缝隙为透明
	// 空格），同样统一注入 Surface 底，避免缝隙露出画布。
	return withBackground(s.Right, theme.Surface)
}

// Height 返回占用行数（恒为 1）。
func (s *StatusBar) Height() int	{ return 1 }

// View 渲染状态条：严格 1 行、严格等于可用宽度（不足补空格，超出截断），
// 整条落 Surface 底（含中缝）。
func (s *StatusBar) View() string {
	width := s.width
	if width <= 0 {
		width = 80
	}
	left := s.leftView()
	room := max(0, width-Width(left)-1)
	right := s.rightView(room) + surfaceSeg(theme.Muted, false, " ")
	if lw, rw := Width(left), Width(right); lw+rw > width {
		if lw >= width {
			left = ansi.Truncate(left, width, "…")
			right = ""
		} else {
			right = ansi.Truncate(right, width-lw, "…")
		}
	}
	gap := max(1, width-Width(left)-Width(right))
	middle := surfaceSeg(theme.Text, false, strings.Repeat(" ", gap))
	line := left + middle + right
	return ansi.Truncate(line, width, "")
}
