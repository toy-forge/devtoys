// modal.go 是自研弹窗：全区域压暗背景 + 居中带边框圆角盒子。
// filepicker、图片预览、删除/清空确认共用；负责自身 Esc 消费与焦点。
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Kind 是弹窗的形态。
type Kind uint8

const (
	// KindInfo 是纯信息弹窗（只有确认按钮）。
	KindInfo	Kind	= iota
	// KindConfirm 是确认弹窗（OK/Cancel 按钮 + enter/esc）。
	KindConfirm
	// KindCustom 是宿主自绘内容的弹窗（按钮由宿主渲染与命中）。
	KindCustom
)

// Action 是 HandleKey 的处理结果。
type Action uint8

const (
	// ActionNone 表示未产生动作。
	ActionNone	Action	= iota
	// ActionConfirm 表示确认（enter 或 OK 按钮）。
	ActionConfirm
	// ActionCancel 表示取消（esc 或 Cancel 按钮）。
	ActionCancel
)

// Modal 是居中弹窗。
type Modal struct {
	// Kind 决定自带按钮组。
	Kind	Kind
	// Title 是弹窗标题（可空）。
	Title	string
	// Body 是正文（按盒子宽度自动换行）。
	Body	string
	// ConfirmLabel/CancelLabel 定制按钮文案，默认 "OK"/"Cancel"。
	ConfirmLabel	string
	CancelLabel	string
	// Width 是盒子内容最小宽度；0 按内容自适应，最终钳制在区域内。
	Width	int
	// MaxWidth 是盒子最大总宽度；0 表示区域宽度。
	MaxWidth	int
	// Danger 把弹窗标记为危险级：标题前缀错误图标（fg=Danger）、
	// 确认按钮变 Danger 变体（视觉分级，键盘语义不变）。
	Danger	bool
	// FocusConfirm 在确认弹窗中让焦点初始落在 OK 上（enter 语义相同）。
	FocusConfirm	bool
	// focusCancel 标记焦点当前在 Cancel 上（tab 切换）。
	focusCancel	bool

	areaW, areaH	int
}

// ConfirmLabel/CancelLabel 的兜底取值。
func (m Modal) labels() (confirm, cancel string) {
	confirm, cancel = m.ConfirmLabel, m.CancelLabel
	if confirm == "" {
		confirm = "OK"
	}
	if cancel == "" {
		cancel = "Cancel"
	}
	return confirm, cancel
}

// SetSize 记录弹窗可用的区域尺寸。
func (m *Modal) SetSize(areaW, areaH int) {
	m.areaW, m.areaH = max(0, areaW), max(0, areaH)
}

// Open 把弹窗置为可见并复位焦点。
func (m *Modal) Open() {
	m.focusCancel = false
}

// boxInner 计算盒子内容宽度（不含边框）。
func (m Modal) boxInner() int {
	width := m.Width
	if w := Width(m.Body); w > width {
		width = w
	}
	if m.Title != "" && Width(m.Title)+2 > width {
		width = Width(m.Title) + 2
	}
	if buttons := m.Buttons(); len(buttons) > 0 {
		row := 0
		for i, b := range buttons {
			if i > 0 {
				row += Space2
			}
			row += b.Width()
		}
		if row > width {
			width = row
		}
	}
	// total = 内容宽 + 左右边框 2 + 正文左右内边距各 Space2（共加 4 列，
	// 宽自适应自洽；正文换行预算 = inner-Space2*2）。
	total := width + 2 + 2*Space2
	maxWidth := m.MaxWidth
	if maxWidth <= 0 {
		maxWidth = m.areaW
	}
	if maxWidth > 0 && total > maxWidth {
		total = max(4, maxWidth)
	}
	if m.areaW > 0 && total > m.areaW {
		total = max(4, m.areaW)
	}
	return max(1, total-2)
}

// Buttons 返回弹窗自带的按钮（KindCustom 返回 nil）。
// 顺序固定：确认在前，取消在后（KindInfo 只有确认）。
// Danger 弹窗的确认按钮为 Danger 变体（红 icon + Danger 按钮）。
func (m Modal) Buttons() []Button {
	if m.Kind == KindCustom {
		return nil
	}
	confirm, cancel := m.labels()
	variant := VariantPrimary
	if m.Danger {
		variant = VariantDanger
	}
	confirmButton := Button{Label: confirm, Variant: variant, Inline: true}
	cancelButton := Button{Label: cancel, Variant: VariantGhost, Inline: true}
	confirmButton.Focused = m.Kind != KindConfirm || !m.focusCancel
	cancelButton.Focused = m.Kind == KindConfirm && m.focusCancel
	if m.Kind == KindInfo {
		return []Button{confirmButton}
	}
	return []Button{confirmButton, cancelButton}
}

// BoxSize 返回盒子总尺寸（含边框）。
func (m Modal) BoxSize() (w, h int) {
	inner := m.boxInner()
	h = len(m.bodyLines(inner)) + 2
	if m.Title != "" {
		h++
	}
	if len(m.Buttons()) > 0 {
		h += 2	// 空行 + 按钮行
	}
	return inner + 2, h
}

// Rect 返回盒子在区域中的居中矩形。
func (m Modal) Rect() Rect {
	w, h := m.BoxSize()
	x, y := Center(w, h, m.areaW, m.areaH)
	return Rect{X: x, Y: y, W: w, H: h}
}

func (m Modal) bodyLines(inner int) []string {
	if m.Body == "" {
		return nil
	}
	// 正文每行左右各 Space2 列内边距，换行预算同步收窄。
	return strings.Split(ansi.Wrap(m.Body, max(1, inner-2*Space2), " "), "\n")
}

// titleRow 渲染标题行：左对齐的 " Title "（bold 正文色）；Danger 弹窗标题前缀
// 错误图标（fg=Danger）。分段各自携带 Surface 底，避免内层 reset
// 打断盒子底色。
func (m Modal) titleRow(inner int) string {
	seg := func(fg theme.Color, bold bool, s string) string {
		st := lipgloss.NewStyle().Foreground(fg).Background(theme.Surface)
		if bold {
			st = st.Bold(true)
		}
		return st.Render(s)
	}
	if m.Danger {
		label := ansi.Truncate(m.Title, max(1, inner-4), "…")
		return seg(theme.Danger, false, " "+theme.StatusIcon(3)) + seg(theme.Text, true, " "+label+" ")
	}
	label := ansi.Truncate(m.Title, max(1, inner-2), "…")
	return seg(theme.Text, true, " "+label+" ")
}

// buttonRowOffset 返回按钮行在盒子内的起始列（含左边框 1 列）：
// 右对齐贴内容区右缘（桌面对话框惯例）。渲染（View）与命中（ButtonHit）
// 共用本函数，禁止第二套算术。
func buttonRowOffset(inner, rowWidth int) int {
	return 1 + max(0, inner-rowWidth)
}

// buttonRowWidth 返回按钮行的显示宽度（按钮 + Space2 间隔）。
func buttonRowWidth(views []string) int {
	rects, _ := PlaceRow(views, Space2)
	if len(rects) == 0 {
		return 0
	}
	last := rects[len(rects)-1]
	return last.X + last.W
}

// modalButtonRow 拼接按钮行；按钮间 Space2 缝隙自带 Surface 底，
// 否则缝隙会露出弹窗外的遮罩（内层按钮样式的 reset 会取消外层背景）。
func modalButtonRow(views []string) string {
	var b strings.Builder
	for i, v := range views {
		if i > 0 {
			b.WriteString(lipgloss.NewStyle().Background(theme.Surface).Render(strings.Repeat(" ", Space2)))
		}
		b.WriteString(v)
	}
	return b.String()
}

// View 渲染盒子本体（不含背景压暗与定位）。盒 = 圆角 + BorderStrong +
// Surface 底（浮起靠海拔与遮罩表达，不用彩色描边）。
func (m Modal) View() string {
	w, h := m.BoxSize()
	inner := w - 2
	rows := make([]string, 0, h)
	if m.Title != "" {
		rows = append(rows, m.titleRow(inner))
	}
	for _, line := range m.bodyLines(inner) {
		rows = append(rows, lipgloss.NewStyle().Foreground(theme.Text).Width(inner).Render(strings.Repeat(" ", Space2)+line))
	}
	buttons := m.Buttons()
	if len(buttons) > 0 {
		views := buttonViews(buttons)
		pad := strings.Repeat(" ", max(0, buttonRowOffset(inner, buttonRowWidth(views))-1))
		rows = append(rows, "", pad+modalButtonRow(views))
	}
	for len(rows) < h-2 {
		rows = append(rows, "")
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderStrong).
		Background(theme.Surface).
		Render(strings.Join(rows, "\n"))
	// E1 海拔：盒逐行铺 Surface 底。标题分段、正文与按钮的 reset
	// 会取消外层 Background，标题行尾/正文右半/按钮缝隙会露出遮罩底——
	// 与 statusbar 同一问题域，逐行重注入（已知限制：
	// 行内自带背景的分段不被 Overlay 覆盖）。
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		lines[i] = withBackground(line, theme.Surface)
	}
	return strings.Join(lines, "\n")
}

// Render 把弹窗叠加到已渲染的背景上：先压暗背景，再在居中位置放盒子。
// background 应当是整块区域的渲染结果（高度即区域高度）。
func (m Modal) Render(background string) string {
	if m.areaW <= 0 || m.areaH <= 0 {
		return background
	}
	lines := strings.Split(background, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.areaW, "")
		if pad := m.areaW - Width(lines[i]); pad > 0 {
			lines[i] += strings.Repeat(" ", pad)
		}
	}
	for len(lines) < m.areaH {
		lines = append(lines, strings.Repeat(" ", m.areaW))
	}
	dimmed := make([]string, len(lines))
	for i, line := range lines {
		dimmed[i] = DimLine(line)
	}
	r := m.Rect()
	box := strings.Split(m.View(), "\n")
	for y := 0; y < len(box) && r.Y+y < len(dimmed); y++ {
		left := ansi.Truncate(dimmed[r.Y+y], r.X, "")
		left += strings.Repeat(" ", max(0, r.X-Width(left)))
		piece := ansi.Truncate(box[y], r.W, "")
		if pad := r.W - Width(piece); pad > 0 {
			piece += strings.Repeat(" ", pad)
		}
		right := skipCells(dimmed[r.Y+y], r.X+r.W)
		dimmed[r.Y+y] = left + piece + right
	}
	return strings.Join(dimmed, "\n")
}

// HandleKey 处理一个按键（key 为 tea.KeyMsg.String()）。
// 返回动作与是否消费：确认弹窗 enter→ActionConfirm、esc→ActionCancel，
// tab 切换焦点；动作非 None 时按键同样被消费。
func (m *Modal) HandleKey(key string) (Action, bool) {
	switch m.Kind {
	case KindCustom:
		return ActionNone, false
	case KindInfo:
		switch key {
		case "enter", "esc":
			return ActionConfirm, true
		}
		return ActionNone, true	// 信息弹窗吞掉全部按键
	case KindConfirm:
		switch key {
		case "enter":
			if m.focusCancel {
				return ActionCancel, true
			}
			return ActionConfirm, true
		case "esc":
			return ActionCancel, true
		case "tab", "shift+tab", "left", "right", "h", "l":
			m.focusCancel = !m.focusCancel
			return ActionNone, true
		}
		return ActionNone, true	// 确认弹窗吞掉其余按键
	}
	return ActionNone, false
}

// ButtonHit 判定一次点击命中了哪个自带按钮：0=确认、1=取消（KindInfo 恒为 0）。
// inside 表示点击落在盒子内但不在按钮上；完全在盒子外时 ok=false。
func (m Modal) ButtonHit(msg tea.Msg) (button int, inside, ok bool) {
	mouse, press := isPressLeft(msg)
	if !press {
		return -1, false, false
	}
	r := m.Rect()
	if !r.Contains(mouse.X, mouse.Y) {
		return -1, false, false
	}
	buttons := m.Buttons()
	if len(buttons) == 0 {
		return -1, true, true
	}
	// 按钮行在盒子内倒数第 2 行（上边框占 1 行）。
	buttonRow := r.Y + r.H - 2
	if mouse.Y != buttonRow {
		return -1, true, true
	}
	rects, _ := PlaceRow(buttonViews(buttons), Space2)
	off := buttonRowOffset(r.W-2, buttonRowWidth(buttonViews(buttons)))
	for i, br := range rects {
		local := Rect{X: r.X + off + br.X, Y: buttonRow, W: br.W, H: 1}
		if local.Contains(mouse.X, mouse.Y) {
			return i, true, true
		}
	}
	return -1, true, true
}

func buttonViews(buttons []Button) []string {
	out := make([]string, len(buttons))
	for i, b := range buttons {
		out[i] = b.View()
	}
	return out
}
