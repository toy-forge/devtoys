// Package optbar 为生成器类工具提供选项栏组件。所有控件均由
// internal/ui 的真实控件组合而成：
//
//   - 数值选项（Number）→ ui.Stepper（[-] 值 [+] 三连按钮）；
//   - 枚举选项（Choice）→ ui.Select（chip + 浮层下拉列表）；
//   - 开关选项（Toggle）→ ui.Switch（pill 开关）；
//   - Hint 追加一个 ui.Button 的 Regenerate 按钮。
//
// 约定按键：ctrl+o 进入/退出选项栏聚焦。聚焦时 ↑↓ 移动活动选项，
// ←→ / +/- 调整活动选项，enter/space 切换开关或展开下拉，tab/esc 退出。
// 未聚焦时选项栏仅作展示，按键不经过本组件，不影响输入区打字。
// 宽度不足时选项栏收缩为两行分页条（[<] n/m [>] + 当前页选项）。
package optbar

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

// Number 是数值选项，范围 [Min, Max]。
type Number struct {
	Label		string
	Icon		string
	Description	string
	Value		int
	Min		int
	Max		int
}

// Choice 是枚举选项，在 Options 中选中一项。
type Choice struct {
	Label		string
	Icon		string
	Description	string
	Options		[]string
	Index		int
}

// Toggle 是开关选项。
type Toggle struct {
	Label		string
	Icon		string
	Description	string
	On		bool
}

// OptionKind identifies one option category in an explicit display order.
type OptionKind uint8

const (
	NumberOption	OptionKind	= iota
	ChoiceOption
	ToggleOption
)

// OptionRef selects an item from one of the Bar option slices.
type OptionRef struct {
	Kind	OptionKind
	Index	int
}

// Bar 是选项栏。Numbers/Choices/Toggles 持有指针，
// 调用方通过指针字段读取当前值。
type Bar struct {
	Numbers	[]*Number
	Choices	[]*Choice
	Toggles	[]*Toggle
	Order	[]OptionRef
	// Hint 非空时在选项栏末尾追加 Regenerate 按钮。
	Hint	string
	Focused	bool

	width	int
	height	int
	page	int
	// cursor 是聚焦时的活动选项（optionOrder 的下标）。
	cursor	int
	// selects 按 Choice 下标缓存下拉控件，保持 Open 状态跨帧。
	selects	map[int]*ui.Select
}

// HandleKey 处理一个按键（key 为 tea.KeyMsg.String()）。
// 返回值 changed 表示选项值发生了变化（聚焦状态变化不算）。
// ctrl+o 在任意状态下切换聚焦；下拉展开时按键先交给下拉框；
// 仅在聚焦时消费其余按键。
func (b *Bar) HandleKey(key string) (changed bool) {
	if key == "ctrl+o" {
		if b.anySelectOpen() {
			b.closeSelects()
			return false
		}
		b.Focused = !b.Focused
		if !b.Focused {
			b.closeSelects()
		}
		return false
	}
	if sel, choice := b.openSelect(); sel != nil {
		ch, _ := sel.HandleKey(key)
		if ch {
			choice.Index = sel.Index
		}
		return ch
	}
	if !b.Focused {
		return false
	}
	order := b.optionOrder()
	b.cursor = max(0, min(b.cursor, len(order)-1))
	switch key {
	case "tab", "esc":
		b.Focused = false
	case "up", "k":
		b.cursor = max(0, b.cursor-1)
	case "down", "j":
		b.cursor = min(len(order)-1, b.cursor+1)
	case "left", "h", "-", "_":
		if len(order) > 0 {
			changed = b.adjust(order[b.cursor], -1)
		}
	case "right", "l", "+", "=":
		if len(order) > 0 {
			changed = b.adjust(order[b.cursor], 1)
		}
	case "enter", " ":
		if len(order) > 0 {
			ref := order[b.cursor]
			switch ref.Kind {
			case ToggleOption:
				changed = b.adjust(ref, 1)
			case ChoiceOption:
				if sel := b.selectFor(ref.Index); len(sel.Options) > 0 {
					sel.Cursor = sel.Index
					sel.Open = true
				}
			}
		}
	default:
		// 数字键 1-9 按声明顺序切换开关选项。
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if i := int(key[0] - '1'); i < len(b.Toggles) {
				b.Toggles[i].On = !b.Toggles[i].On
				changed = true
			}
		}
	}
	return changed
}

// HandlesKeys 报告当前按键是否应交给本组件（聚焦中或下拉展开）。
func (b *Bar) HandlesKeys() bool	{ return b.Focused || b.anySelectOpen() }

// adjust 按方向调整一个选项，返回是否发生变化。
func (b *Bar) adjust(ref OptionRef, delta int) bool {
	switch ref.Kind {
	case NumberOption:
		if ref.Index < 0 || ref.Index >= len(b.Numbers) {
			return false
		}
		n := b.Numbers[ref.Index]
		if v := n.Value + delta; v >= n.Min && v <= n.Max {
			n.Value = v
			return true
		}
	case ChoiceOption:
		if ref.Index < 0 || ref.Index >= len(b.Choices) || len(b.Choices[ref.Index].Options) == 0 {
			return false
		}
		c := b.Choices[ref.Index]
		c.Index = (c.Index + delta + len(c.Options)) % len(c.Options)
		if sel := b.selectFor(ref.Index); sel.Index != c.Index {
			sel.Index, sel.Cursor = c.Index, c.Index
		}
		return true
	case ToggleOption:
		if ref.Index < 0 || ref.Index >= len(b.Toggles) {
			return false
		}
		t := b.Toggles[ref.Index]
		t.On = !t.On
		return true
	}
	return false
}

// SetWidth 设置选项栏可用宽度。选项按单元换行，点击区域与显示保持一致。
func (b *Bar) SetWidth(width int)	{ b.width = max(1, width) }

// SetSize 同时设置可用宽高；高度用于在桌面式行与紧凑分页条之间取舍。
func (b *Bar) SetSize(width, height int) {
	b.width, b.height = max(1, width), max(1, height)
}

// Height 返回选项栏当前占用的行数（与 View 的行数一致）。
func (b *Bar) Height() int {
	if b.expanded() {
		return b.expandedRowCount()
	}
	l := b.compactLayout()
	if b.isCompact(l) {
		return 2
	}
	if b.height > 0 {
		return len(l.lines) + 1
	}
	return len(l.lines)
}

// OverlayActive 报告是否有下拉浮层展开（宿主应把鼠标事件先交给本组件）。
func (b *Bar) OverlayActive() bool	{ return b.anySelectOpen() }

// HandleMouse 处理选项与分页控件的鼠标事件（工具局部坐标）。
// 返回值 changed 表示选项值变化，regenerate 表示点了 Regenerate。
func (b *Bar) HandleMouse(msg tea.MouseMsg) (changed, regenerate bool) {
	if msg.Action != tea.MouseActionPress {
		return false, false
	}
	if b.anySelectOpen() {
		l := b.layout()
		areaW, areaH := max(1, b.width), max(1, b.height)
		for _, ch := range l.chips {
			if !ch.sel.Open {
				continue
			}
			before := ch.c.Index
			if ch.sel.HandleMouse(msg, ch.x, ch.y, areaW, areaH) {
				if ch.sel.Index != before {
					ch.c.Index = ch.sel.Index
					return true, false
				}
				return false, false
			}
		}
		// 点击浮层以外：收起下拉，不作用于下层。
		b.closeSelects()
		return false, false
	}
	l := b.layout()
	if b.isCompact(l) {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			b.clampPage(len(l.lines))
			b.page = max(0, b.page-1)
			return false, false
		case tea.MouseButtonWheelDown:
			b.clampPage(len(l.lines))
			b.page = min(len(l.lines)-1, b.page+1)
			return false, false
		case tea.MouseButtonLeft:
		default:
			return false, false
		}
		if msg.Y == 0 {
			line := b.pageLine(len(l.lines))
			if msg.X >= 0 && msg.X < 3 {
				b.page = max(0, b.page-1)
			}
			if start := lipgloss.Width(line) - 3; msg.X >= start && msg.X < start+3 {
				b.page = min(len(l.lines)-1, b.page+1)
			}
			return false, false
		}
		if msg.Y != 1 {
			return false, false
		}
		msg.Y = b.page
	} else {
		if msg.Button != tea.MouseButtonLeft {
			return false, false
		}
		if !b.expanded() && b.height > 0 {
			// 非紧凑的换行视图在宿主里带一行 Configuration 标题。
			if msg.Y < 1 {
				return false, false
			}
			msg.Y--
		}
	}
	for _, h := range l.hits {
		if h.r.Contains(msg.X, msg.Y) {
			return b.applyHit(h)
		}
	}
	return false, false
}

// applyHit 执行一次命中的动作。
func (b *Bar) applyHit(h hitCell) (changed, regenerate bool) {
	switch h.kind {
	case hitStep:
		n := b.Numbers[h.opt]
		if v := n.Value + h.delta; v >= n.Min && v <= n.Max {
			n.Value = v
			return true, false
		}
	case hitToggle:
		t := b.Toggles[h.opt]
		t.On = !t.On
		return true, false
	case hitChip:
		sel := b.selectFor(h.opt)
		if sel.Open {
			sel.Close()
			return false, false
		}
		b.closeSelects()
		sel.Cursor = sel.Index
		sel.Open = true
	case hitRegenerate:
		return false, true
	}
	return false, false
}

// View 渲染选项栏：宽裕时为桌面式行（标题 + 名称/控件 + 描述），
// 紧凑时为两行分页条，介于两者之间时为换行的多行控件视图。
func (b *Bar) View() string {
	if b.expanded() {
		return strings.Join(b.expandedLayout().lines, "\n")
	}
	l := b.compactLayout()
	b.clampPage(len(l.lines))
	if b.isCompact(l) {
		pageLine := b.pageLine(len(l.lines))
		title := "Configuration"
		if b.width < 30 {
			title = "Config"
		}
		if b.width >= lipgloss.Width(pageLine)+lipgloss.Width(title)+2 {
			pageLine += "  " + theme.Eyebrow(title)
		}
		return pageLine + "\n" + l.lines[min(b.page, len(l.lines)-1)]
	}
	view := strings.Join(l.lines, "\n")
	if b.height > 0 {
		heading := theme.Eyebrow("Configuration")
		view = heading + "\n" + view
	}
	return view
}

// OverlayOn 把展开中的下拉浮层叠加到宿主完整视图上。
// 选项栏必须渲染在宿主视图的左上角（本包三个工具均满足）。
func (b *Bar) OverlayOn(view string) string {
	if !b.anySelectOpen() {
		return view
	}
	l := b.layout()
	areaW, areaH := max(1, b.width), max(1, lipgloss.Height(view))
	for _, ch := range l.chips {
		if !ch.sel.Open {
			continue
		}
		r := ch.sel.OverlayRect(ch.x, ch.y, areaW, areaH)
		view = ui.Overlay(view, ch.sel.OpenView(), r)
	}
	return view
}

// ControlRect 返回指定选项的主控件在布局坐标中的矩形：
// Number 为 [-] 按钮（配合 StepRects 取 [+]），Choice 为下拉 chip，
// Toggle 为开关 pill。用于测试与宿主定位点击。
func (b *Bar) ControlRect(kind OptionKind, index int) (ui.Rect, bool) {
	l := b.layout()
	for _, h := range l.hits {
		if h.opt != index {
			continue
		}
		switch {
		case kind == NumberOption && h.kind == hitStep && h.delta < 0,
			kind == ChoiceOption && h.kind == hitChip,
			kind == ToggleOption && h.kind == hitToggle:
			return h.r, true
		}
	}
	return ui.Rect{}, false
}

// StepRects 返回数值选项 [-]/[+] 两个按钮的布局矩形。
func (b *Bar) StepRects(index int) (dec, inc ui.Rect, ok bool) {
	l := b.layout()
	for _, h := range l.hits {
		if h.kind == hitStep && h.opt == index {
			if h.delta < 0 {
				dec = h.r
			} else {
				inc = h.r
			}
		}
	}
	return dec, inc, dec.W > 0 && inc.W > 0
}

// ---------------------------------------------------------------------------
// 布局

type hitKind uint8

const (
	hitStep	hitKind	= iota
	hitToggle
	hitChip
	hitRegenerate
)

// hitCell 是一个可点击控件在布局坐标中的矩形与动作。
type hitCell struct {
	r	ui.Rect
	kind	hitKind
	opt	int	// 在 Numbers/Choices/Toggles 中的下标
	delta	int	// hitStep 的步进方向
}

// chipPlacement 记录一个下拉 chip 的放置位置，供浮层命中与叠加使用。
// x/y 是 chip 在 Bar.View() 中的屏幕坐标：x 含单元格内 chip 前缀
// （Choice 为 label+2 列）的偏移，y 已换算标题行与分页偏移；
// 与 HandleMouse 的命中矩形、OverlayOn 的叠加位置共用同一份坐标。
type chipPlacement struct {
	x, y	int
	// dx 是 chip 在单元格视图内的列偏移（place() 记录 x 时加上）。
	dx	int
	sel	*ui.Select
	c	*Choice
}

type barLayout struct {
	lines	[]string
	hits	[]hitCell
	chips	[]chipPlacement
}

func (b *Bar) optionOrder() []OptionRef {
	if len(b.Order) > 0 {
		return b.Order
	}
	order := make([]OptionRef, 0, len(b.Numbers)+len(b.Choices)+len(b.Toggles))
	for i := range b.Numbers {
		order = append(order, OptionRef{Kind: NumberOption, Index: i})
	}
	for i := range b.Choices {
		order = append(order, OptionRef{Kind: ChoiceOption, Index: i})
	}
	for i := range b.Toggles {
		order = append(order, OptionRef{Kind: ToggleOption, Index: i})
	}
	return order
}

func (b *Bar) selectFor(index int) *ui.Select {
	if b.selects == nil {
		b.selects = map[int]*ui.Select{}
	}
	sel := b.selects[index]
	if sel == nil {
		sel = &ui.Select{}
		b.selects[index] = sel
	}
	c := b.Choices[index]
	sel.Options = c.Options
	if sel.Index != c.Index {
		sel.Index, sel.Cursor = c.Index, c.Index
	}
	return sel
}

func (b *Bar) anySelectOpen() bool {
	for _, sel := range b.selects {
		if sel.Open {
			return true
		}
	}
	return false
}

func (b *Bar) openSelect() (*ui.Select, *Choice) {
	for i, sel := range b.selects {
		if sel.Open && i < len(b.Choices) {
			return sel, b.Choices[i]
		}
	}
	return nil, nil
}

func (b *Bar) closeSelects() {
	for _, sel := range b.selects {
		sel.Close()
	}
}

// expanded 报告是否采用桌面式布局（标题 + 每选项一行 + 描述）。
func (b *Bar) expanded() bool {
	return b.width >= 40 && b.height >= b.expandedRowCount()+7
}

func (b *Bar) expandedRowCount() int {
	rows := 1	// 标题行
	for _, ref := range b.optionOrder() {
		if !b.optionVisible(ref) {
			continue
		}
		rows++	// 说明与名称同行（见 expandedLayout），不占用额外行
	}
	if b.Hint != "" {
		rows++
	}
	return rows
}

func (b *Bar) optionVisible(ref OptionRef) bool {
	switch ref.Kind {
	case NumberOption:
		return ref.Index >= 0 && ref.Index < len(b.Numbers)
	case ChoiceOption:
		return ref.Index >= 0 && ref.Index < len(b.Choices) && len(b.Choices[ref.Index].Options) > 0
	case ToggleOption:
		return ref.Index >= 0 && ref.Index < len(b.Toggles)
	}
	return false
}

// isCompact 报告是否收缩为两行分页条（行数会挤占工作区时）。
func (b *Bar) isCompact(l barLayout) bool {
	return b.height > 0 && len(l.lines) > 1 && len(l.lines)+5 > b.height
}

func (b *Bar) layout() barLayout {
	if b.expanded() {
		return b.expandedLayout()
	}
	return b.compactLayout()
}

func (b *Bar) clampPage(rows int)	{ b.page = max(0, min(b.page, rows-1)) }

// pageLine 渲染分页条：两端是内联按钮 < 与 >（各 3 列，命中区与 Update 的
// 左右 3 列判定一致），中间是 n/m 页码。太窄时只保留两端按钮。
func (b *Bar) pageLine(rows int) string {
	prev := ui.Button{Label: "<", Inline: true, Disabled: b.page <= 0}.View()
	next := ui.Button{Label: ">", Inline: true, Disabled: b.page >= rows-1}.View()
	line := prev + " " +
		lipgloss.NewStyle().Foreground(theme.Text).Render(strconv.Itoa(b.page+1)+"/"+strconv.Itoa(rows)) +
		" " + next
	if lipgloss.Width(line) > b.width {
		return prev + " " + next
	}
	return line
}

// compactLayout 把每个选项渲染成一行控件单元，按宽度换行。
func (b *Bar) compactLayout() barLayout {
	width := max(1, b.width)
	l := barLayout{}
	var line strings.Builder
	x, y := 0, 0
	place := func(view string, cellHits []hitCell, cp *chipPlacement) {
		w := lipgloss.Width(view)
		if x > 0 && x+1+min(w, width) > width {
			l.lines = append(l.lines, line.String())
			line.Reset()
			x, y = 0, y+1
		}
		if x > 0 {
			line.WriteByte(' ')
			x++
		}
		tw := min(w, max(1, width-x))
		for i := range cellHits {
			cellHits[i].r.X += x
			cellHits[i].r.Y += y
			if cellHits[i].r.X+cellHits[i].r.W > x+tw {
				cellHits[i].r.W = max(1, x+tw-cellHits[i].r.X)
			}
			l.hits = append(l.hits, cellHits[i])
		}
		if cp != nil {
			// 记录 chip 的真实位置：单元格起点 x 加上 chip 在视图内的偏移。
			cp.x, cp.y = x+cp.dx, y
			l.chips = append(l.chips, *cp)
		}
		line.WriteString(ansi.Truncate(view, tw, ""))
		x += tw
	}
	for _, ref := range b.optionOrder() {
		if !b.optionVisible(ref) {
			continue
		}
		switch ref.Kind {
		case NumberOption:
			n := b.Numbers[ref.Index]
			st := ui.Stepper{Label: n.Label, Value: n.Value, Min: n.Min, Max: n.Max}
			dec, _, inc := st.Rects(0, 0)
			place(st.View(), []hitCell{
				{r: dec, kind: hitStep, opt: ref.Index, delta: -1},
				{r: inc, kind: hitStep, opt: ref.Index, delta: 1},
			}, nil)
		case ChoiceOption:
			c := b.Choices[ref.Index]
			sel := b.selectFor(ref.Index)
			sel.ListHeight = max(3, min(8, b.height-y-2))
			chip := sel.View()
			view := c.Label + "  " + chip
			chipDx := lipgloss.Width(c.Label) + 2
			r := sel.ChipRect(chipDx, 0)
			place(view, []hitCell{{r: r, kind: hitChip, opt: ref.Index}},
				&chipPlacement{dx: chipDx, sel: sel, c: c})
		case ToggleOption:
			t := b.Toggles[ref.Index]
			sw := ui.Switch{On: t.On}
			view := t.Label + " " + sw.View()
			r := sw.Rect(lipgloss.Width(t.Label)+1, 0)
			place(view, []hitCell{{r: r, kind: hitToggle, opt: ref.Index}}, nil)
		}
	}
	if b.Hint != "" {
		btn := ui.Button{Label: "Regenerate", Inline: true, Variant: ui.VariantPrimary}
		place(btn.View(), []hitCell{{r: btn.Rect(0, 0), kind: hitRegenerate}}, nil)
	}
	l.lines = append(l.lines, line.String())
	// 把 chip 的 y 从布局行号换算成 Bar.View() 中的屏幕行：
	// 分页视图固定渲染在第 2 行；带标题的换行视图整体下移一行。
	switch {
	case b.isCompact(l):
		for i := range l.chips {
			l.chips[i].y = 1
		}
	case b.height > 0:
		for i := range l.chips {
			l.chips[i].y++
		}
	}
	return l
}

// expandedLayout 渲染标题行 + 每选项一行（名称居左、控件贴右）+ 描述行。
func (b *Bar) expandedLayout() barLayout {
	l := barLayout{}
	l.lines = append(l.lines, theme.Eyebrow("Configuration"))
	nameStyle := lipgloss.NewStyle().Foreground(theme.Text)
	for orderIndex, ref := range b.optionOrder() {
		if !b.optionVisible(ref) {
			continue
		}
		var label, icon, right string
		var hits []hitCell
		var cp *chipPlacement
		switch ref.Kind {
		case NumberOption:
			n := b.Numbers[ref.Index]
			label, icon = n.Label, n.Icon
			st := ui.Stepper{Value: n.Value, Min: n.Min, Max: n.Max}
			dec, _, inc := st.Rects(0, 0)
			right = st.View()
			hits = []hitCell{
				{r: dec, kind: hitStep, opt: ref.Index, delta: -1},
				{r: inc, kind: hitStep, opt: ref.Index, delta: 1},
			}
		case ChoiceOption:
			c := b.Choices[ref.Index]
			label, icon = c.Label, c.Icon
			sel := b.selectFor(ref.Index)
			// 列表高度不超过选项数（短列表不占空行），长列表按可用高度收缩，至少 3 行可滚动。
			sel.ListHeight = min(len(c.Options), max(3, min(8, b.height-len(l.lines)-2)))
			right = sel.View()
			hits = []hitCell{{r: sel.ChipRect(0, 0), kind: hitChip, opt: ref.Index}}
			cp = &chipPlacement{sel: sel, c: c}
		case ToggleOption:
			t := b.Toggles[ref.Index]
			label, icon = t.Label, t.Icon
			sw := ui.Switch{On: t.On}
			right = sw.View()
			hits = []hitCell{{r: sw.Rect(0, 0), kind: hitToggle, opt: ref.Index}}
		}
		left := "  "
		if b.Focused && orderIndex == b.cursor {
			left = "▌ "
		}
		if icon != "" {
			left += lipgloss.NewStyle().Foreground(theme.Accent).Render(icon) + "  "
		}
		nameWidth := b.width - lipgloss.Width(right) - lipgloss.Width(left) - 3
		name := ansi.Truncate(label, max(1, nameWidth), "…")
		if b.Focused && orderIndex == b.cursor {
			name = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render(name)
		} else {
			name = nameStyle.Render(name)
		}
		left += name
		// 说明文字与名称同行（弱化色），放得下才显示；与设置条的版式一致。
		if desc := b.description(ref); desc != "" {
			room := b.width - lipgloss.Width(left) - lipgloss.Width(right) - 2*ui.Space2 - 1
			if room >= minInlineDescWidth {
				left += strings.Repeat(" ", ui.Space2) + lipgloss.NewStyle().Foreground(theme.Muted).Render(ansi.Truncate(desc, room, "…"))
			}
		}
		gap := max(1, b.width-lipgloss.Width(left)-lipgloss.Width(right)-1)
		x := lipgloss.Width(left) + gap
		y := len(l.lines)
		for i := range hits {
			hits[i].r.X += x
			hits[i].r.Y += y
			l.hits = append(l.hits, hits[i])
		}
		if cp != nil {
			cp.x, cp.y = x, y
			l.chips = append(l.chips, *cp)
		}
		l.lines = append(l.lines, left+strings.Repeat(" ", gap)+right)
	}
	if b.Hint != "" {
		btn := ui.Button{Label: "Regenerate", Inline: true, Variant: ui.VariantPrimary}
		right := btn.View()
		left := "  Generate"
		gap := max(1, b.width-lipgloss.Width(left)-lipgloss.Width(right)-1)
		l.hits = append(l.hits, hitCell{
			r:	btn.Rect(lipgloss.Width(left)+gap, len(l.lines)),
			kind:	hitRegenerate,
		})
		l.lines = append(l.lines, left+strings.Repeat(" ", gap)+right)
	}
	return l
}

// minInlineDescWidth 是说明文字与名称同行显示的最小宽度；更窄时整段说明省略。
const minInlineDescWidth = 12

// description 返回选项的说明文字（空串表示没有）。
func (b *Bar) description(ref OptionRef) string {
	switch ref.Kind {
	case NumberOption:
		if ref.Index >= 0 && ref.Index < len(b.Numbers) {
			return b.Numbers[ref.Index].Description
		}
	case ChoiceOption:
		if ref.Index >= 0 && ref.Index < len(b.Choices) {
			return b.Choices[ref.Index].Description
		}
	case ToggleOption:
		if ref.Index >= 0 && ref.Index < len(b.Toggles) {
			return b.Toggles[ref.Index].Description
		}
	}
	return ""
}
