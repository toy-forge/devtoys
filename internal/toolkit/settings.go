package toolkit

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

// SettingHeader presents configuration settings as desktop-style rows when
// space permits, while retaining pageable actions in short terminals.
type SettingHeader struct {
	Names		[]string
	Descriptions	[]string
	Icons		[]string
	Switches	[]bool
	// Options[i] 非空时第 i 项是下拉框（ui.Select）：chip 列出当前值，点击展开
	// 浮层列出全部选项，选中后 Pick 返回选项下标，由工具据此设置状态。
	Options	[][]string
	// Actions[i] 为 true 时第 i 项的取值是一个动作按钮（如"Configure"），渲染为
	// 内联按钮而不是 ▾ 下拉框，点击照常交给工具处理。
	Actions	[]bool
	// ReadOnly[i] 为 true 时第 i 项只展示取值（弱化文字，无 chip），用于当前状态下
	// 不可交互的设置，避免它看起来像可点击的下拉框。
	ReadOnly		[]bool
	Groups			[]SettingGroup
	Accessory		string
	HideTitle		bool
	ContentGap		int
	width, height, rows	int
	fallback		ActionHeader
	// menu 是展开中的下拉框（nil 表示收起）；menuFor 为其设置下标，
	// menuX/menuY 为 chip 在设置条中的位置（浮层据此定位）。
	menu		*ui.Select
	menuFor		int
	menuX, menuY	int
}

// SettingGroup renders one primary setting with child settings beneath it.
// Indexes refer to the matching Names and values positions.
type SettingGroup struct {
	Title		string
	Icon		string
	Primary		int
	Children	[]int
	HeaderOnly	bool
}

type settingHeaderRow struct {
	index		int
	group		int
	start		bool
	end		bool
	child		bool
	description	bool
	text		string
}

func (h SettingHeader) settingRows(values []string) []settingHeaderRow {
	rows := make([]settingHeaderRow, 0, len(values)+len(h.Groups)*2)
	covered := make(map[int]bool, len(values))
	appendSetting := func(row settingHeaderRow) {
		rows = append(rows, row)
	}
	for groupIndex, group := range h.Groups {
		if group.HeaderOnly {
			rows = append(rows, settingHeaderRow{index: -1, group: groupIndex, start: true})
			for _, child := range group.Children {
				if child < 0 || child >= len(values) || covered[child] {
					continue
				}
				appendSetting(settingHeaderRow{index: child, group: groupIndex, child: true})
				covered[child] = true
			}
			rows = append(rows, settingHeaderRow{group: groupIndex, end: true})
			continue
		}
		if group.Primary < 0 || group.Primary >= len(values) || covered[group.Primary] {
			continue
		}
		appendSetting(settingHeaderRow{index: group.Primary, group: groupIndex, start: true})
		covered[group.Primary] = true
		for _, child := range group.Children {
			if child < 0 || child >= len(values) || covered[child] {
				continue
			}
			appendSetting(settingHeaderRow{index: child, group: groupIndex, child: true})
			covered[child] = true
		}
		rows = append(rows, settingHeaderRow{group: groupIndex, end: true})
	}
	for index := range values {
		if !covered[index] {
			appendSetting(settingHeaderRow{index: index})
		}
	}
	return rows
}

// contentGap 返回当前尺寸下设置条与内容区之间实际使用的空行数。
// 极矮终端放不下"设置条 + 空行 + 最小工作区"时收缩空行，
// 保证 header.View 行数与 Height 一致且工作区不会被挤出最小高度。
func (h SettingHeader) contentGap(values []string) int {
	gap := max(0, h.ContentGap)
	if h.width <= 0 || h.height <= 0 {
		return gap
	}
	avail := h.height - h.baseHeight(values...) - lipgloss.Height(h.Accessory)
	if avail-gap < WorkspaceMinHeight {
		gap = max(0, avail-WorkspaceMinHeight)
	}
	return gap
}

func (h SettingHeader) expanded(values []string) bool {
	rows := h.settingRows(values)
	requiredHeight := max(len(values)+9, len(rows)+9)
	if h.HideTitle {
		requiredHeight--
	}
	requiredHeight += max(0, h.ContentGap)
	return h.width >= 35 && h.height >= requiredHeight
}

func (h SettingHeader) titleHeight() int {
	if h.HideTitle {
		return 0
	}
	return 1
}

// Expanded reports whether the desktop-style settings rows fit the current size.
func (h SettingHeader) Expanded(values ...string) bool	{ return h.expanded(values) }

func (h SettingHeader) baseHeight(values ...string) int {
	if h.expanded(values) {
		return len(h.settingRows(values)) + h.titleHeight()
	}
	if h.fallback.compact(values) {
		return h.fallback.Height(values...)
	}
	return h.fallback.Height(values...) + h.titleHeight()
}

// BaseHeight reports the height used by the settings controls, excluding Accessory.
func (h SettingHeader) BaseHeight(values ...string) int	{ return h.baseHeight(values...) }

func (h SettingHeader) Height(values ...string) int {
	height := h.baseHeight(values...)
	if h.Accessory != "" {
		height += lipgloss.Height(h.Accessory)
	}
	return height + h.contentGap(values)
}

func (h SettingHeader) View(values ...string) string {
	view := h.baseView(values...)
	if h.Accessory != "" {
		view += "\n" + h.accessoryView()
	}
	return view + strings.Repeat("\n", h.contentGap(values))
}

// accessoryView 把附件行按当前宽度截断，避免长描述撑破终端宽度。
func (h SettingHeader) accessoryView() string {
	lines := strings.Split(h.Accessory, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, h.width), "")
	}
	return strings.Join(lines, "\n")
}

func (h SettingHeader) baseView(values ...string) string {
	if !h.expanded(values) {
		if h.HideTitle {
			return h.fallback.View(values...)
		}
		if h.fallback.compact(values) {
			lines := strings.Split(h.fallback.View(values...), "\n")
			title := "Configuration"
			if h.width < 30 {
				title = "Config"
			}
			if len(h.Groups) > 0 && h.Groups[0].Title != "" {
				title += " · " + h.Groups[0].Title
			}
			title = ansi.Truncate(title, max(1, h.width-lipgloss.Width(lines[0])-2), "…")
			heading := lipgloss.NewStyle().Foreground(theme.Text).Bold(true).Render(title)
			if h.width >= lipgloss.Width(lines[0])+lipgloss.Width(title)+2 {
				lines[0] += "  " + heading
			}
			return strings.Join(lines, "\n")
		}
		title := "Configuration"
		if len(h.Groups) > 0 && h.Groups[0].Title != "" {
			title += " · " + h.Groups[0].Title
		}
		heading := lipgloss.NewStyle().Foreground(theme.Text).Bold(true).Render(ansi.Truncate(title, h.width, "…"))
		return heading + "\n" + h.fallback.View(values...)
	}
	heading := theme.TitleStyle().Render("Configuration")
	label := theme.LabelStyle()
	lines := make([]string, 0, len(h.settingRows(values))+1)
	if !h.HideTitle {
		lines = append(lines, heading)
	}
	for _, row := range h.settingRows(values) {
		if row.end {
			// 组末尾保留一行空白分隔。
			lines = append(lines, "")
			continue
		}
		if row.description {
			// 描述行：已内联到设置行，跳过独立描述行。
			continue
		}
		name := "Option"
		if row.index >= 0 && row.index < len(h.Names) {
			name = h.Names[row.index]
		}
		right := ""
		if row.index >= 0 && row.index < len(values) {
			right = h.controlLabel(row.index, values[row.index])
		}
		prefix := "  "
		if row.child {
			prefix = "    "
		}
		if row.start {
			group := h.Groups[row.group]
			title := group.Title
			if title == "" {
				title = name
			}
			icon := ""
			if group.Icon != "" {
				icon = group.Icon + "  "
			}
			name = icon + title
		} else {
			name = h.settingIcon(row.index) + name
		}
		nameWidth := max(1, h.width-lipgloss.Width(prefix)-lipgloss.Width(right)-2)
		name = ansi.Truncate(name, nameWidth, "…")
		left := label.Render(prefix + name)
		// 说明文字（设置项的副标题）：名称之后、控件之前，弱化色；放不下则省略。
		if desc := h.descriptionFor(row.index); desc != "" {
			room := h.width - lipgloss.Width(prefix+name) - lipgloss.Width(right) - 2*settingDescGap
			if room >= minSettingDescWidth {
				left += strings.Repeat(" ", settingDescGap) + theme.MutedStyle().Render(ansi.Truncate(desc, room, "…"))
			}
		}
		gap := max(1, h.width-lipgloss.Width(left)-lipgloss.Width(right))
		// 设置行整行铺 Surface 底，控件 chip 压在表面上。
		lines = append(lines, h.surfaceRow(left+strings.Repeat(" ", gap)+right))
	}
	return strings.Join(lines, "\n")
}

// groupTitleRow 渲染组标题行：前两列并入 Border 发丝线色段，其余
// 标签（图标 + 标题）走 TitleStyle。
func groupTitleRow(left string) string {
	corner := min(2, lipgloss.Width(left))
	border := lipgloss.NewStyle().Foreground(theme.Border)
	return border.Render(ansi.Cut(left, 0, corner)) + theme.TitleStyle().Render(ansi.Cut(left, corner, lipgloss.Width(left)))
}

// surfaceRow 把一行设置内容右补到整行宽并铺 Surface 底：组合行
// 的内层分段（控件 chip、Border 色段）的 reset 会取消外层背景，
// 与 ui.WithBackground 同一问题域。
func (h SettingHeader) surfaceRow(line string) string {
	line = ansi.Truncate(line, max(1, h.width), "")
	if pad := h.width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return ui.WithBackground(line, theme.Surface)
}

func (h SettingHeader) settingIcon(index int) string {
	if index >= 0 && index < len(h.Icons) && h.Icons[index] != "" {
		return h.Icons[index] + "  "
	}
	return ""
}

// controlLabel 用真实控件渲染设置值：开关类走 ui.Switch 药丸，
// 其余走 ui.Select 关闭态 chip。渲染宽度为 value+4 列，
// 保证行布局命中不受影响。
func (h SettingHeader) controlLabel(index int, value string) string {
	if index >= 0 && index < len(h.ReadOnly) && h.ReadOnly[index] {
		return theme.MutedStyle().Render(value)
	}
	if index >= 0 && index < len(h.Actions) && h.Actions[index] {
		return ui.Button{Label: value, Inline: true}.View()
	}
	if options := h.optionsFor(index); len(options) > 0 {
		sel := h.dropdown(index, value)
		return sel.View()
	}
	isSwitch := index < len(h.Switches) && h.Switches[index]
	isSwitch = isSwitch || value == "On" || value == "Off" || value == "Yes" || value == "No"
	if !isSwitch {
		dropdown := ui.Select{Options: []string{value}, Width: lipgloss.Width(value) + 1}
		return dropdown.View()
	}
	on := !(value == "Off" || value == "No" || value == "Decode" || strings.HasSuffix(value, " Off"))
	sw := ui.Switch{On: on, OnLabel: value, OffLabel: value}
	return sw.View()
}

// settingDescGap 是名称与说明、说明与控件之间的间距；minSettingDescWidth 是
// 说明文字可见的最小宽度，低于它时整段说明省略，保证控件与名称不被挤压。
const (
	settingDescGap		= 2
	minSettingDescWidth	= 12
)

// descriptionFor 返回第 index 项的说明文字（空串表示没有）。
func (h SettingHeader) descriptionFor(index int) string {
	if index >= 0 && index < len(h.Descriptions) {
		return h.Descriptions[index]
	}
	return ""
}

// optionsFor 返回第 index 项的下拉选项；非下拉框返回 nil。
func (h SettingHeader) optionsFor(index int) []string {
	if index >= 0 && index < len(h.Options) {
		return h.Options[index]
	}
	return nil
}

// dropdown 构造第 index 项的下拉框（关闭态）：选中项与 value 对齐，chip 宽度
// 取最长选项，切换取值时宽度不跳动。
func (h SettingHeader) dropdown(index int, value string) ui.Select {
	sel := ui.Select{Options: h.optionsFor(index)}
	sel.SetValue(value)
	return sel
}

// Index 返回被点击的设置下标（开关、回退按钮条）；下拉框的点击不在这里处理，
// 由 Pick 负责——Index 对下拉框行返回 -1，避免工具把它当作开关切换。
func (h *SettingHeader) Index(msg tea.Msg, values ...string) int {
	if !h.expanded(values) {
		if mouse, ok := msg.(tea.MouseMsg); ok {
			if !h.HideTitle && !h.fallback.compact(values) {
				if mouse.Y < 1 {
					return -1
				}
				mouse.Y--
				msg = mouse
			}
		}
		return h.fallback.Index(msg, values...)
	}
	mouse, ok := msg.(tea.MouseMsg)
	rows := h.settingRows(values)
	titleRows := h.titleHeight()
	if !ok || mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft || mouse.X < 0 || mouse.X >= h.width || mouse.Y < titleRows || mouse.Y >= len(rows)+titleRows {
		return -1
	}
	row := mouse.Y - titleRows
	if row < 0 || row >= len(rows) || rows[row].index < 0 || rows[row].description {
		return -1
	}
	if len(h.optionsFor(rows[row].index)) > 0 {
		return -1
	}
	return rows[row].index
}

// Pick 处理设置条上的一次交互，是 Index 的超集：
//   - 下拉框展开时，浮层消费全部按键与鼠标事件（工具不得再处理）：
//     选中返回 (设置下标, 选项下标)；收起（esc/enter 不变/点击外部）返回 (-1, -1)；
//   - 点击下拉框 chip 展开浮层，返回 (-1, -1)；
//   - 其余点击（开关、回退按钮条）返回 (设置下标, -1)，与 Index 语义一致。
//
// handled 报告事件是否已被设置条处理；为 false 时工具照常处理该消息。
// 工具需在 handled 时调用 Overlay 叠加浮层（View 中），并用 MenuOpen 让 esc 先收起浮层。
func (h *SettingHeader) Pick(msg tea.Msg, values ...string) (setting, option int, handled bool) {
	// 只消费输入事件（按键、鼠标）：窗口尺寸、异步结果等其它消息即使在浮层展开时
	// 也必须照常交给工具，否则 resize 会被吞掉。
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
	default:
		return -1, -1, false
	}
	if h.menu != nil {
		return h.pickInMenu(msg)
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && h.expanded(values) {
		if index, x, y, hit := h.dropdownAt(mouse, values...); hit {
			sel := h.dropdown(index, values[index])
			sel.Cursor, sel.Open = sel.Index, true
			h.menu, h.menuFor, h.menuX, h.menuY = &sel, index, x, y
			return -1, -1, true
		}
	}
	setting = h.Index(msg, values...)
	return setting, -1, setting >= 0
}

// pickInMenu 处理展开中的下拉框的事件：浮层是模态的，全部事件都被消费。
func (h *SettingHeader) pickInMenu(msg tea.Msg) (setting, option int, handled bool) {
	sel, index := h.menu, h.menuFor
	switch event := msg.(type) {
	case tea.KeyMsg:
		changed, _ := sel.HandleKey(event.String())
		if sel.Open {
			return -1, -1, true
		}
		h.menu = nil
		if changed {
			return index, sel.Index, true
		}
		return -1, -1, true
	case tea.MouseMsg:
		if event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
			if option, hit := sel.ListHit(event, h.menuX, h.menuY, h.width, h.height); hit {
				h.menu = nil
				return index, option, true
			}
			// 点击 chip 或浮层外部：收起，不穿透到工具。
			h.menu = nil
			return -1, -1, true
		}
		sel.HandleMouse(event, h.menuX, h.menuY, h.width, h.height)
	}
	return -1, -1, true
}

// dropdownAt 命中设置条上的下拉框行（tool 局部坐标）。整行都是可点目标——
// 右端 chip 很小，名称区点击同样展开。设置条每行恰好占一行，第 r 行位于
// titleRows+r；chip 右对齐到设置条宽度，浮层锚定在 chip 上。
func (h SettingHeader) dropdownAt(mouse tea.MouseMsg, values ...string) (index, x, y int, ok bool) {
	titleRows := h.titleHeight()
	for r, row := range h.settingRows(values) {
		if row.end || row.index < 0 || row.index >= len(values) || len(h.optionsFor(row.index)) == 0 {
			continue
		}
		cy := titleRows + r
		if mouse.Y != cy || mouse.X < 0 || mouse.X >= h.width {
			continue
		}
		sel := h.dropdown(row.index, values[row.index])
		return row.index, max(0, h.width-sel.ChipRect(0, 0).W), cy, true
	}
	return -1, 0, 0, false
}

// Shortcut 处理第 index 项的键盘快捷键（如 alt+d）：下拉框在设置条展开时打开
// 浮层，供键盘选择（与点击等效）；其余情况返回 (index, -1, true)，工具照旧轮换取值。
// 返回值语义与 Pick 一致：setting<0 表示浮层已打开并消费了该快捷键。
func (h *SettingHeader) Shortcut(index int, values ...string) (setting, option int, handled bool) {
	if len(h.optionsFor(index)) == 0 || index < 0 || index >= len(values) || !h.expanded(values) || h.menu != nil {
		return index, -1, true
	}
	titleRows := h.titleHeight()
	for r, row := range h.settingRows(values) {
		if row.end || row.index != index {
			continue
		}
		sel := h.dropdown(index, values[index])
		sel.Cursor, sel.Open = sel.Index, true
		x := max(0, h.width-sel.ChipRect(0, 0).W)
		h.menu, h.menuFor, h.menuX, h.menuY = &sel, index, x, titleRows+r
		return -1, -1, true
	}
	return index, -1, true
}

// MenuOpen 报告下拉浮层是否展开；工具的 HandlesEscape 用它让 esc 先收起浮层。
func (h SettingHeader) MenuOpen() bool	{ return h.menu != nil }

// Overlay 把展开中的下拉浮层叠加到工具的完整视图（页头 + 工作区）上；
// 设置条位于视图顶部时使用，等价于 OverlayAt(view, 0)。
func (h SettingHeader) Overlay(view string) string {
	return h.OverlayAt(view, 0)
}

// OverlayAt 同 Overlay，但设置条的第 0 行位于完整视图的第 top 行（工具在设置条
// 上方还有状态行或"Configuration"标题时使用）。浮层收起时原样返回；
// 浮层不改变布局，因此不需要 Relayout。
func (h SettingHeader) OverlayAt(view string, top int) string {
	if h.menu == nil {
		return view
	}
	r := h.menu.OverlayRect(h.menuX, h.menuY, h.width, h.height)
	r.Y += top
	return ui.Overlay(view, h.menu.OpenView(), r)
}

// SetSize updates the header for layouts that do not use Workspace.
func (h *SettingHeader) SetSize(size tea.WindowSizeMsg, values ...string) int {
	h.width, h.height = size.Width, size.Height
	fallbackHeight := max(1, size.Height-h.titleHeight())
	h.fallback.width, h.fallback.height = size.Width, fallbackHeight
	// 立即用新尺寸刷新回退按钮条：ActionHeader 的按钮条只在指针方法里
	// 重排，若留到 View 时才按旧尺寸布局，紧凑判定与换行会滞留旧宽度。
	h.fallback.SetSize(tea.WindowSizeMsg{Width: size.Width, Height: fallbackHeight}, values...)
	h.rows = h.Height(values...)
	return h.rows
}
func (h *SettingHeader) Resize(size tea.WindowSizeMsg, w *Workspace, values ...string) tea.Cmd {
	rows := h.SetSize(size, values...)
	return w.Update(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-rows)})
}
func (h *SettingHeader) Relayout(w *Workspace, values ...string) {
	if h.width > 0 && h.height > 0 && h.rows != h.Height(values...) {
		h.Resize(tea.WindowSizeMsg{Width: h.width, Height: h.height}, w, values...)
	}
}
