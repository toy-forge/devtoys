package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

// shell.go 是外壳的几何与绘制：侧栏（类别导航）/图标轨、内容区留白、
// 浏览页与工具页的页头。所有尺寸只从这里推导，渲染与鼠标命中共用，
// 禁止在别处手算。
//
// 版式约定（全应用只此一套）：
//   - 侧栏与内容区之间不画边框，靠 Surface / Background 两层底色分界；
//   - 内容区左右各留 contentPad 列；
//   - 页内的纵向节奏：标题 → 空行 → 内容，区块之间恒为一行空白。

const (
	// sidebarWidth 是宽屏下左侧类别导航的宽度（Surface 底，无边框）。
	sidebarWidth	= 28
	// railWidth 是中屏下图标轨的宽度。
	railWidth	= 6
	// sidebarFullMin 及以上的宽度使用完整侧栏。
	sidebarFullMin	= 100
	// railMin 及以上的宽度收窄为图标轨，再窄则完全让位给内容区。
	railMin	= 48
	// contentPad 是内容区左右留白（Space2）。
	contentPad	= ui.Space2
	// browseHeaderRows 是浏览页页头行数：标题行、搜索行、列标题。
	browseHeaderRows	= 3
	// browseSearchRow 是搜索行在页头内的行号（点击即开始过滤）。
	browseSearchRow	= 1
	// toolHeaderRows 是工具页页头行数：标题行 + 空行。
	toolHeaderRows	= 2
	// sidebarHeaderRows 是侧栏品牌行数：品牌行 + 空行。
	sidebarHeaderRows	= 2
)

// sidebarColumns 是侧栏宽度换算的唯一源头：
// 宽屏（≥sidebarFullMin）为完整侧栏，中屏（≥railMin）收窄为图标轨，
// 更窄则完全让位给内容区。
func (m Model) sidebarColumns() int {
	switch {
	case m.width >= sidebarFullMin:
		return sidebarWidth
	case m.width >= railMin:
		return railWidth
	default:
		return 0
	}
}

// railMode 报告侧栏当前是否为图标轨形态。
func (m Model) railMode() bool	{ return m.sidebarColumns() == railWidth }

// bodyHeight 是除状态栏外的内容高度。
func (m Model) bodyHeight() int	{ return max(1, m.height-1) }

// contentWidth 是侧栏右侧的内容区总宽（含左右留白）。
func (m Model) contentWidth() int	{ return max(1, m.width-m.sidebarColumns()) }

// innerWidth 是去掉左右留白后的可用宽度：工具、选择器与列表都按它布局。
func (m Model) innerWidth() int	{ return max(1, m.contentWidth()-2*contentPad) }

// contentX 是内容区可用区域的起始列（侧栏宽 + 左留白），鼠标换算用。
func (m Model) contentX() int	{ return m.sidebarColumns() + contentPad }

// toolHeaderHeight 是工具页页头占用的行数；高度不足时让位给工具本身。
func (m Model) toolHeaderHeight() int {
	if m.height >= 12 {
		return toolHeaderRows
	}
	return 0
}

// toolSize 是工具页可用的内容尺寸（已扣除留白与页头）。
func (m Model) toolSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: m.innerWidth(), Height: max(1, m.bodyHeight()-m.toolHeaderHeight())}
}

// resize 记录窗口尺寸并同步两个列表与弹层的尺寸。
func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.nav.SetSize(max(1, sidebarWidth-2), max(1, m.bodyHeight()-sidebarHeaderRows))
	m.tools.SetSize(m.innerWidth(), max(1, m.bodyHeight()-browseHeaderRows))
	m.tools.FilterInput.Width = max(1, m.innerWidth()-6)
	if m.picker != nil {
		m.picker.SetSize(m.innerWidth(), m.bodyHeight())
	}
	if m.preview != nil {
		m.preview.SetSize(m.innerWidth(), m.bodyHeight())
	}
}

// activeCategory 返回侧栏应高亮的类别：工具打开时取工具所属类别。
func (m Model) activeCategory() string {
	if m.mode == modeTool && m.active != nil {
		return m.active.Category()
	}
	return m.category
}

// surfaceRow 把一行铺满 width 列的 Surface 底（侧栏是 E1 表面）。
func surfaceRow(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ui.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return ui.WithBackground(line, theme.Surface)
}

// fillRows 把行补齐到 height 行，缺失的行用 blank 填充。
func fillRows(rows []string, height int, blank string) []string {
	if len(rows) > height {
		return rows[:height]
	}
	for len(rows) < height {
		rows = append(rows, blank)
	}
	return rows
}

// sidebarView 渲染宽屏侧栏：品牌行、空行、类别导航（bubbles list）。
// 整块 Surface 底，行左右各留 1 列。
func (m Model) sidebarView() string {
	brand := theme.TitleStyle().Render(theme.Icon(theme.IconLogo) + "  DevToys")
	rows := []string{surfaceRow(" "+brand, sidebarWidth), surfaceRow("", sidebarWidth)}
	for _, line := range strings.Split(m.nav.View(), "\n") {
		rows = append(rows, surfaceRow(" "+line+" ", sidebarWidth))
	}
	blank := surfaceRow("", sidebarWidth)
	return strings.Join(fillRows(rows, m.bodyHeight(), blank), "\n")
}

// railView 渲染中屏图标轨：每行一个语义图标，活动类别反色整格高亮（Accent），
// 指针悬停的行抬一层（SurfaceHigh）。
func (m Model) railView() string {
	active := m.activeCategory()
	hover := -1
	if m.hasPointer && m.pointerX < railWidth && m.mode == modeBrowse {
		hover = m.pointerY
	}
	rows := make([]string, 0, len(navEntries()))
	for i, entry := range navEntries() {
		icon := theme.Icon(theme.IconLogo)
		if entry.kind == itemHeader {
			icon = theme.GroupIcon(entry.category)
		}
		style := lipgloss.NewStyle().Width(railWidth).AlignHorizontal(lipgloss.Center).Background(theme.Surface).Foreground(theme.Muted)
		switch {
		case entry.category == active:
			style = theme.Style(theme.KeyStatusAccent).Width(railWidth).AlignHorizontal(lipgloss.Center)
		case i == hover:
			style = style.Background(theme.SurfaceHigh)
		}
		rows = append(rows, style.Render(icon))
	}
	blank := surfaceRow("", railWidth)
	return strings.Join(fillRows(rows, m.bodyHeight(), blank), "\n")
}

// sidebarBox 返回侧栏（宽屏或图标轨）的整块渲染，高度恒为 bodyHeight。
func (m Model) sidebarBox() string {
	if m.railMode() {
		return m.railView()
	}
	return m.sidebarView()
}

// browseHeader 是浏览页页头的三行：标题（左）+ 数量（右）、搜索行、
// 列标题。行数必须等于 browseHeaderRows。
func (m Model) browseHeader() []string {
	iw := m.innerWidth()
	count := fmt.Sprintf("%d tools", m.toolCount())
	if m.tools.FilterState() != list.Unfiltered {
		// 过滤时给出实时反馈：命中的工具数而不是类别总数。
		count = fmt.Sprintf("%d of %d tools", m.visibleToolCount(), m.toolCount())
	}
	if cue := m.scrollCue(); cue != "" {
		count += " · " + cue
	}
	title := theme.TitleStyle().Render(" " + m.browseTitle())
	return []string{
		spread(iw, title, theme.MutedStyle().Render(count)),
		m.searchRow(iw),
		m.columnHeader(iw),
	}
}

// spread 把左右两段摆在 width 列的两端（中间补空格）；放不下时截断左段。
func spread(width int, left, right string) string {
	gap := width - ui.Width(left) - ui.Width(right)
	if gap < 1 {
		left = ansi.Truncate(left, max(1, width-ui.Width(right)-1), "…")
		gap = max(1, width-ui.Width(left)-ui.Width(right))
	}
	return left + strings.Repeat(" ", gap) + right
}

// searchRow 渲染搜索行：一个 Surface 底的输入框。
// 过滤中显示 bubbles 的输入框；已过滤显示查询词与清除提示。
//
// 过滤时输入框按剩余宽度渲染（而不是整行宽）：输入框自带尾部填充，
// 若按整行宽渲染再由 spread 截断，截掉的只是填充空白，
// 却会在提示前留下一个孤零零的 …。
func (m Model) searchRow(iw int) string {
	var line string
	switch m.tools.FilterState() {
	case list.Filtering:
		hint := theme.MutedStyle().Render("enter apply · esc clear")
		// textinput 的 Width 不含 Prompt 列；预算要再扣掉提示符宽度。
		budget := max(1, iw-ui.Width(hint)-2)
		m.tools.FilterInput.Width = max(1, budget-ui.Width(m.tools.FilterInput.Prompt))
		// textinput.View 只返回内容宽度，不铺到 Width；补到预算宽度后再交给
		// spread，否则左段偏短、spread 会把它当成溢出并塞进一个 …。
		input := m.tools.FilterInput.View()
		if pad := budget - ui.Width(input); pad > 0 {
			input += strings.Repeat(" ", pad)
		}
		line = spread(iw, input, hint)
	case list.FilterApplied:
		icon := ui.IconCell(theme.Icon(theme.IconSearch), 2)
		query := theme.LabelStyle().Render(" " + icon + strings.Repeat(" ", ui.Space1) + m.tools.FilterInput.Value())
		line = spread(iw, query, theme.MutedStyle().Render("esc clear"))
	default:
		icon := ui.IconCell(theme.Icon(theme.IconSearch), 2)
		search := theme.MutedStyle().Render(" " + icon + strings.Repeat(" ", ui.Space1) + "Search tools…")
		line = spread(iw, search, theme.MutedStyle().Render("/ "))
	}
	return surfaceRow(line, iw)
}

// columnHeader 渲染列标题，列宽与 toolDelegate 共用 newToolColumns。
// 列标题走眉标角色（大写、Muted 粗体），与类别带同一份字符形态。
func (m Model) columnHeader(iw int) string {
	cols := newToolColumns(iw)
	style := theme.EyebrowStyle()
	line := style.Render(strings.Repeat(" ", toolRowPrefix)) + cell("TOOL", cols.title, style)
	if cols.showDesc {
		gap := style.Render(strings.Repeat(" ", columnGap))
		line += gap + cell("DESCRIPTION", cols.desc, style)
	}
	return ansi.Truncate(line, iw, "")
}

// listBody 渲染工具列表；没有匹配项时给出可恢复的空状态。
func (m Model) listBody() string {
	if len(m.tools.VisibleItems()) == 0 {
		message, hint := "No tools available.", ""
		if m.tools.FilterState() != list.Unfiltered {
			message = fmt.Sprintf("No tools match %q.", m.tools.FilterInput.Value())
			hint = "esc clears the search."
		}
		lines := []string{theme.MutedStyle().Render(" " + message)}
		if hint != "" {
			lines = append(lines, theme.MutedStyle().Render(" "+hint))
		}
		return strings.Join(lines, "\n")
	}
	return m.tools.View()
}

// browseView 渲染浏览页：页头 + 工具列表。
func (m Model) browseView() string {
	return strings.Join(m.browseHeader(), "\n") + "\n" + m.listBody()
}

// toolHeader 渲染工具页页头：图标 + 标题 + 类别面包屑，之后跟一行空白。
func (m Model) toolHeader() string {
	chip := lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render(theme.ToolIcon(m.active.ID()))
	title := theme.TitleStyle().Render(m.active.Title())
	crumb := theme.MutedStyle().Render("· " + groupTitle(m.active.Category()))
	line := " " + chip + strings.Repeat(" ", ui.Space1) + title + strings.Repeat(" ", ui.Space2) + crumb
	return ansi.Truncate(line, m.innerWidth(), "…") + "\n\n"
}

// contentView 渲染内容区（侧栏右侧），输出恰为 contentWidth×bodyHeight。
func (m Model) contentView() string {
	var body string
	switch {
	case m.picker != nil:
		body = m.picker.View()
	case m.mode == modeTool && m.active != nil:
		header := ""
		if m.toolHeaderHeight() > 0 {
			header = m.toolHeader()
		}
		body = header + m.active.View()
	default:
		body = m.browseView()
	}
	if m.preview != nil {
		body = m.preview.Overlay(body)
	}
	return m.fitContent(body)
}

// fitContent 把正文铺到 innerWidth×bodyHeight，再左右各加 contentPad 列留白。
func (m Model) fitContent(body string) string {
	inner := ui.Fill(body, m.innerWidth(), m.bodyHeight())
	pad := lipgloss.NewStyle().Background(theme.Background).Render(strings.Repeat(" ", contentPad))
	lines := strings.Split(inner, "\n")
	for i, line := range lines {
		lines[i] = pad + line + pad
	}
	return strings.Join(lines, "\n")
}
