package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// home.go 是浏览模式的两个列表：侧栏的类别导航（nav）与内容区的工具列表
// （tools）。两者都是 bubbles/list，共用同一套单行行样式——选中整行填充、
// hover 抬一层、焦点只决定选中行是否用 Accent。列宽推导只在
// newToolColumns 一处，工具行与列标题共用。

const (
	// toolRowPrefix 是工具行的前导宽度：1 留白 + 2 格图标 + 2 格间距。
	toolRowPrefix	= 5
	// toolCatWidth 是类别列宽（"Encoders / Decoders" 截断到此宽）。
	toolCatWidth	= 14
	// columnGap 是列与列之间的间距。
	columnGap	= 2
	// minDescWidth 低于此宽时隐藏描述列，只保留标题与类别。
	minDescWidth	= 8
)

type itemKind int

const (
	itemTool	itemKind	= iota	// 工具行（内容区）
	itemHeader				// 类别导航项（侧栏）
	itemAll					// All tools 导航项（侧栏）
)

// listItem 是两个列表共用的条目：工具（内容区）或类别（侧栏）。
type listItem struct {
	kind		itemKind
	tool		toolkit.Tool
	title		string
	desc		string
	category	string
	count		int
}

// FilterValue 参与模糊搜索：工具按"标题 + 类别 + 描述"，类别按名称。
// 标题在前缀位置，命中下标落在标题段时才用于高亮标题。
func (i listItem) FilterValue() string {
	if i.kind != itemTool {
		return i.title
	}
	return i.title + " " + groupTitle(i.category) + " " + i.desc
}

func groupTitle(category string) string {
	if category == "encoders" {
		return "Encoders / Decoders"
	}
	return toolkit.CategoryTitle(category)
}

// navEntries 是侧栏与图标轨共用的类别条目：All tools 在前，随后各类别。
func navEntries() []listItem {
	counts := map[string]int{}
	for _, tool := range toolkit.All() {
		counts[tool.Category()]++
	}
	entries := []listItem{{kind: itemAll, title: "All tools", count: toolkit.Count()}}
	for _, category := range toolkit.Categories() {
		entries = append(entries, listItem{kind: itemHeader, category: category, title: groupTitle(category), count: counts[category]})
	}
	return entries
}

func navItems() []list.Item {
	entries := navEntries()
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entry)
	}
	return items
}

// toolItems 返回当前类别下的工具条目（空类别 = 全部，按类别再按标题排序）。
func (m Model) toolItems() []list.Item {
	var items []list.Item
	for _, tool := range toolkit.All() {
		if m.category != "" && tool.Category() != m.category {
			continue
		}
		items = append(items, listItem{
			kind:		itemTool,
			tool:		tool,
			title:		toolkit.ListTitle(tool),
			desc:		tool.Description(),
			category:	tool.Category(),
		})
	}
	return items
}

// newToolList 创建内容区工具列表。搜索行由外壳渲染（SetShowFilter(false)），
// 过滤逻辑仍由 list 负责；过滤命中按字符下标高亮（见 newToolList 的 Filter）。
func newToolList() list.Model {
	l := newBaseList(toolDelegate{})
	l.Filter = runeFilter
	l.FilterInput.Prompt = " 󰍉  "
	l.FilterInput.PromptStyle = theme.MutedStyle()
	l.FilterInput.TextStyle = theme.LabelStyle()
	l.FilterInput.Cursor.Style = lipgloss.NewStyle().Foreground(theme.CursorText).Background(theme.Accent)
	l.FilterInput.Cursor.TextStyle = theme.LabelStyle()
	return l
}

// newNavList 创建侧栏类别导航：不做过滤，选中即切换内容区类别。
func newNavList() list.Model {
	l := newBaseList(navDelegate{})
	l.SetFilteringEnabled(false)
	return l
}

func newBaseList(delegate list.ItemDelegate) list.Model {
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.DisableQuitKeybindings()
	return l
}

// runeFilter 把 bubbles 默认过滤的字节下标换算成字符下标：sahilm/fuzzy 返回
// 字节下标，而委托按字符取样式；标题含多字节 Nerd 图标时二者会错位。
func runeFilter(term string, targets []string) []list.Rank {
	ranks := list.DefaultFilter(term, targets)
	for i, r := range ranks {
		matched := make([]int, 0, len(r.MatchedIndexes))
		for _, b := range r.MatchedIndexes {
			if b >= 0 && b <= len(targets[r.Index]) {
				matched = append(matched, len([]rune(targets[r.Index][:b])))
			}
		}
		r.MatchedIndexes = matched
		ranks[i] = r
	}
	return ranks
}

// toolColumns 是工具行与列标题共用的列宽推导（唯一源头）。
// 总宽 = 前导 + title + [类别列：gap+14] + [描述列：gap+desc]，描述隐藏时
// 标题吃掉剩余宽度，保证任何宽度下行宽都精确等于 width。
type toolColumns struct {
	title, desc	int
	showCategory	bool
	showDesc	bool
}

func newToolColumns(width int, showCategory bool) toolColumns {
	avail := max(0, width-toolRowPrefix)
	c := toolColumns{showCategory: showCategory}
	if showCategory {
		avail = max(0, avail-columnGap-toolCatWidth)
	}
	title := max(min(12, avail), min(32, avail*2/5))
	if rest := avail - title - columnGap; rest >= minDescWidth {
		c.title, c.desc, c.showDesc = title, rest, true
	} else {
		c.title = avail
	}
	return c
}

// rowLook 是一行的配色：由「选中 / 焦点 / hover / 底色」决定。
type rowLook struct {
	bg, fg, muted, icon, hit	theme.Color
	bold				bool
}

// lookFor 按行状态取配色。选中且有焦点 = Accent 整行填充（唯一的强调）；
// 选中但焦点在别处 = SurfaceHigh + 粗体；hover = SurfaceHigh；其余 = 底色。
func lookFor(base theme.Color, selected, focused, hover bool) rowLook {
	switch {
	case selected && focused:
		return rowLook{bg: theme.Accent, fg: theme.CursorText, muted: theme.CursorText, icon: theme.CursorText, hit: theme.CursorText, bold: true}
	case selected:
		return rowLook{bg: theme.SurfaceHigh, fg: theme.Text, muted: theme.Muted, icon: theme.BlueBright, hit: theme.Accent, bold: true}
	case hover:
		return rowLook{bg: theme.SurfaceHigh, fg: theme.Text, muted: theme.Muted, icon: theme.BlueBright, hit: theme.Accent}
	default:
		return rowLook{bg: base, fg: theme.Text, muted: theme.Muted, icon: theme.BlueBright, hit: theme.Accent}
	}
}

func (l rowLook) style(fg theme.Color, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Background(l.bg).Foreground(fg)
	if bold {
		s = s.Bold(true)
	}
	return s
}

// cell 把文本截断或补齐到 width 列，并整段套上样式（段内自带底色，
// 不会被相邻段的 reset 取消）。
func cell(text string, width int, style lipgloss.Style) string {
	text = ansi.Truncate(text, width, "…")
	if pad := width - ui.Width(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	return style.Render(text)
}

// toolDelegate 渲染内容区的工具行：前导图标 + 标题 + [类别] + [描述]。
type toolDelegate struct {
	hover		int
	focused		bool
	showCategory	bool
}

func (d toolDelegate) Height() int				{ return 1 }
func (d toolDelegate) Spacing() int				{ return 0 }
func (d toolDelegate) Update(tea.Msg, *list.Model) tea.Cmd	{ return nil }
func (d toolDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(listItem)
	if !ok {
		return
	}
	width := m.Width()
	cols := newToolColumns(width, d.showCategory)
	look := lookFor(theme.Background, index == m.Index(), d.focused, index == d.hover)

	var b strings.Builder
	b.WriteString(look.style(look.fg, false).Render(" "))
	b.WriteString(cell(theme.ToolIcon(entry.tool.ID()), 2, look.style(look.icon, true)))
	b.WriteString(look.style(look.fg, false).Render("  "))
	b.WriteString(titleCell(entry.title, cols.title, m.MatchesForItem(index), look))
	if cols.showCategory {
		b.WriteString(look.style(look.fg, false).Render(strings.Repeat(" ", columnGap)))
		b.WriteString(cell(toolkit.CategoryTitle(entry.category), toolCatWidth, look.style(look.muted, false)))
	}
	if cols.showDesc {
		b.WriteString(look.style(look.fg, false).Render(strings.Repeat(" ", columnGap)))
		b.WriteString(cell(entry.desc, cols.desc, look.style(look.muted, false)))
	}
	line := b.String()
	if pad := width - ui.Width(line); pad > 0 {
		line += look.style(look.fg, false).Render(strings.Repeat(" ", pad))
	}
	fmt.Fprint(w, line)
}

// titleCell 渲染标题段：过滤命中的字符用 hit 色加粗，其余为行正文色。
// matches 是 FilterValue 的字符下标，只取落在标题段内的部分。
func titleCell(title string, width int, matches []int, look rowLook) string {
	title = ansi.Truncate(title, width, "…")
	titleRunes := len([]rune(title))
	inTitle := make([]int, 0, len(matches))
	for _, i := range matches {
		if i >= 0 && i < titleRunes {
			inTitle = append(inTitle, i)
		}
	}
	plain := look.style(look.fg, look.bold)
	rendered := lipgloss.StyleRunes(title, inTitle, look.style(look.hit, true), plain)
	if pad := width - ui.Width(title); pad > 0 {
		rendered += plain.Render(strings.Repeat(" ", pad))
	}
	return rendered
}

// navDelegate 渲染侧栏的类别行：图标 + 名称 + 右对齐数量。
type navDelegate struct {
	hover	int
	focused	bool
}

func (d navDelegate) Height() int				{ return 1 }
func (d navDelegate) Spacing() int				{ return 0 }
func (d navDelegate) Update(tea.Msg, *list.Model) tea.Cmd	{ return nil }
func (d navDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(listItem)
	if !ok {
		return
	}
	width := m.Width()
	look := lookFor(theme.Surface, index == m.Index(), d.focused, index == d.hover)
	icon := theme.GroupIcon(entry.category)
	if entry.kind == itemAll {
		icon = theme.Icon(theme.IconLogo)
	}
	// 布局：" " + 图标(2) + " " + 名称 + " " + 数量 + " "，名称吃掉剩余宽度。
	count := fmt.Sprint(entry.count)
	nameWidth := max(1, width-6-ui.Width(count))

	base := look.style(look.fg, false)
	var b strings.Builder
	b.WriteString(base.Render(" "))
	b.WriteString(cell(icon, 2, look.style(look.icon, true)))
	b.WriteString(base.Render(" "))
	b.WriteString(cell(entry.title, nameWidth, look.style(look.fg, look.bold)))
	b.WriteString(base.Render(" "))
	b.WriteString(look.style(look.muted, false).Render(count))
	b.WriteString(base.Render(" "))
	fmt.Fprint(w, b.String())
}

// syncDelegates 把焦点与指针 hover 写进两个列表的委托。委托只读状态、
// 不派生命令，因此在每次 Update 之后整体同步一次即可。
func (m *Model) sync() {
	hoverTool, hoverNav := -1, -1
	if m.hasPointer && m.mode == modeBrowse && m.picker == nil && m.preview == nil {
		hoverTool = m.toolIndexAt(m.pointerX-m.contentX(), m.pointerY)
		hoverNav = m.navIndexAt(m.pointerX, m.pointerY)
	}
	m.tools.SetDelegate(toolDelegate{
		hover:		hoverTool,
		focused:	!m.navFocus,
		showCategory:	m.category == "",
	})
	m.nav.SetDelegate(navDelegate{
		hover:		hoverNav,
		focused:	m.navFocus,
	})
}

// browseTitle 返回内容区标题：All tools 或当前类别名。
func (m Model) browseTitle() string {
	if m.category == "" {
		return "All tools"
	}
	return groupTitle(m.category)
}

// setCategory 切换内容区类别：清空搜索、重建工具条目并选中第一项。
func (m *Model) setCategory(category string) {
	if category == m.category {
		return
	}
	m.category = category
	m.tools.ResetFilter()
	m.tools.SetItems(m.toolItems())
	m.tools.Select(0)
	m.selectNav()
}

// selectNav 让侧栏高亮与当前类别一致。
func (m *Model) selectNav() {
	for i, item := range m.nav.Items() {
		if entry, ok := item.(listItem); ok && entry.category == m.category {
			m.nav.Select(i)
			return
		}
	}
}

// syncCategoryFromNav 把侧栏的选中项落到内容区类别上（键盘与鼠标共用）。
func (m *Model) syncCategoryFromNav() {
	entry, ok := m.nav.SelectedItem().(listItem)
	if !ok {
		return
	}
	m.setCategory(entry.category)
}

func (m *Model) selectTool(tool toolkit.Tool) {
	m.active, m.mode, m.navFocus = tool, modeTool, false
}

// visibleIndex 把列表内的行号换算成可见条目下标（考虑当前分页）；越界返回 -1。
func visibleIndex(l list.Model, row int) int {
	if row < 0 || row >= l.Paginator.PerPage {
		return -1
	}
	i := l.Paginator.Page*l.Paginator.PerPage + row
	if i >= len(l.VisibleItems()) {
		return -1
	}
	return i
}

// toolIndexAt 命中内容区工具行，x/y 为内容区局部坐标；未命中返回 -1。
func (m Model) toolIndexAt(x, y int) int {
	if x < 0 || x >= m.innerWidth() {
		return -1
	}
	return visibleIndex(m.tools, y-browseHeaderRows)
}

// navIndexAt 命中侧栏类别行（全局坐标）；图标轨与未显示侧栏时返回 -1。
func (m Model) navIndexAt(x, y int) int {
	if m.sidebarColumns() != sidebarWidth || x < 0 || x >= sidebarWidth {
		return -1
	}
	return visibleIndex(m.nav, y-sidebarHeaderRows)
}
