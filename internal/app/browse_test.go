package app

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// press 把一条消息交给模型并返回新模型（测试用的值进值出辅助）。
func press(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestHomeCategoryNavigationAndMouse(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30})
	if view := m.View(); !strings.Contains(view, "All tools") || !strings.Contains(view, "Date & Time") {
		t.Fatalf("missing home content: %s", view)
	}
	// 侧栏：品牌行 + 空行之后，导航从 Y=2 开始；Converters 是第二项（Y=3）。
	m = press(t, m, tea.MouseMsg{X: 4, Y: sidebarHeaderRows + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.category != "converters" || m.nav.Index() != 1 {
		t.Fatalf("category=%q nav=%d, want converters/1", m.category, m.nav.Index())
	}
	if !strings.Contains(m.View(), "Number Base Converter") || strings.Contains(m.View(), "Certificate") {
		t.Fatal("content did not switch to the converters category")
	}
	// 内容区第一行（Y=browseHeaderRows）即该类别的第一个工具：点击直接打开。
	first := m.tools.Items()[0].(listItem).tool
	m = press(t, m, tea.MouseMsg{X: m.contentX() + 10, Y: browseHeaderRows, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.mode != modeTool || m.active == nil || m.active.ID() != first.ID() {
		t.Fatalf("row click did not open %s", first.ID())
	}
}

func TestKeyboardCategoryFocusAndSearch(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30})
	// Tab 把焦点移到侧栏，↓ 切换类别，内容区随之变化。
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.category != "converters" || !m.navFocus {
		t.Fatalf("category=%q navFocus=%v, want converters with sidebar focus", m.category, m.navFocus)
	}
	// Tab 回到工具列表，"/" 进入过滤，输入 json 后只剩匹配项。
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, runes("/"))
	m = press(t, m, runes("json"))
	// bubbles 的过滤结果经 Cmd 异步回灌；这里同步执行一次同样的过滤计算。
	m.tools.SetFilterText("json")
	m.tools.SetFilterState(list.Filtering)
	if m.tools.FilterState() != list.Filtering {
		t.Fatalf("filter state = %v, want Filtering", m.tools.FilterState())
	}
	if len(m.tools.VisibleItems()) == 0 {
		t.Fatal("json filter matched nothing")
	}
	// 模糊匹配会命中描述里的散落字符，因此只断言排名第一的结果是 JSON 工具。
	if top := m.tools.VisibleItems()[0].(listItem); !strings.Contains(top.title, "JSON") {
		t.Fatalf("top filter result = %q, want a JSON tool", top.title)
	}
	// esc 清除过滤。
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tools.FilterState() != list.Unfiltered {
		t.Fatalf("esc did not clear filter: %v", m.tools.FilterState())
	}
}

func TestAllToolsClustersByCategory(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30})
	// All tools 视图：每个类别前插一条不可选中的分组标题，类别不再占列。
	sections := 0
	for _, item := range m.tools.Items() {
		if isSectionItem(item) {
			sections++
		}
	}
	if sections != len(toolkit.Categories()) {
		t.Fatalf("sections=%d, want %d", sections, len(toolkit.Categories()))
	}
	if strings.Contains(m.columnHeader(m.innerWidth()), "CATEGORY") {
		t.Fatal("category is carried by section headers, not a column")
	}
	// 初始游标落在工具上，不停在分组标题。
	if isSectionItem(m.tools.SelectedItem()) {
		t.Fatal("cursor started on a section header")
	}
	// 单类别视图没有分组标题。
	m = press(t, m, tea.MouseMsg{X: 4, Y: sidebarHeaderRows + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	for _, item := range m.tools.Items() {
		if isSectionItem(item) {
			t.Fatal("a category page should not repeat a section header")
		}
	}
}

// 过滤中搜索行不得出现孤立的 …：输入框自带尾部填充，按整行宽渲染再截断
// 会把填充当成溢出，在提示前留下一个省略号。
func TestSearchRowHasNoStrayEllipsis(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 20}, {Width: 100, Height: 20}, {Width: 80, Height: 20}, {Width: 60, Height: 20}, {Width: 44, Height: 20}} {
		m := press(t, New(), size)
		m.tools.SetFilterText("json")
		m.tools.SetFilterState(list.Filtering)
		row := ansi.Strip(m.searchRow(m.innerWidth()))
		// 查询词很短，除 34 列以下的极端宽度外不应触发任何截断。
		if strings.Contains(row, "…") {
			t.Fatalf("%dx%d: stray ellipsis in search row: %q", size.Width, size.Height, row)
		}
		if !strings.Contains(row, "enter apply") {
			t.Fatalf("%dx%d: filter hint missing: %q", size.Width, size.Height, row)
		}
	}
}

func TestFilterFeedbackAndRecoverableEmptyState(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 100, Height: 20})
	m.tools.SetFilterText("json")
	want := fmt.Sprintf("%d of %d tools", m.visibleToolCount(), m.toolCount())
	if header := ansi.Strip(strings.Join(m.browseHeader(), "\n")); !strings.Contains(header, want) {
		t.Fatalf("filtered header = %q, want %q", header, want)
	}
	m.tools.SetFilterText("zzz")
	body := ansi.Strip(m.listBody())
	if !strings.Contains(body, `No tools match "zzz".`) || !strings.Contains(body, "esc clears the search.") {
		t.Fatalf("empty filter state is not recoverable: %q", body)
	}
}

func TestListOverflowCue(t *testing.T) {
	short := press(t, New(), tea.WindowSizeMsg{Width: 100, Height: 12})
	if header := ansi.Strip(strings.Join(short.browseHeader(), "\n")); !strings.Contains(header, "↓ more") {
		t.Fatalf("overflow cue missing: %q", header)
	}
	tall := press(t, New(), tea.WindowSizeMsg{Width: 100, Height: 70})
	if header := ansi.Strip(strings.Join(tall.browseHeader(), "\n")); strings.Contains(header, "more") {
		t.Fatalf("overflow cue shown when everything fits: %q", header)
	}
}

func TestSectionHeadersAreNotSelectable(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	items := m.tools.VisibleItems()
	// 一路向下：游标只会停在工具行，不会停在分组标题。
	for i := 0; i < len(items)+2; i++ {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
		if isSectionItem(m.tools.SelectedItem()) {
			t.Fatalf("cursor stopped on a section at step %d", i)
		}
	}
	// 从 ENCODERS 的第一个工具向上：跳到 CONVERTERS 的最后一个工具，跳过标题。
	encoders := 0
	for i, item := range items {
		if entry, ok := item.(listItem); ok && entry.kind == itemSection && entry.category == "encoders" {
			encoders = i
			break
		}
	}
	m.tools.Select(encoders + 1)
	before := m.tools.SelectedItem().(listItem)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyUp})
	after := m.tools.SelectedItem().(listItem)
	if isSectionItem(after) || after.category != "converters" || after.title == before.title {
		t.Fatalf("up from first encoder = %q (%s)", after.title, after.category)
	}
}

func TestSectionHeaderClickIsInert(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	// 第 0 行是 CONVERTERS 分组标题；点击它不打开任何工具。
	m = press(t, m, tea.MouseMsg{X: m.contentX() + 10, Y: browseHeaderRows, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.mode != modeBrowse || m.active != nil {
		t.Fatalf("section header opened a tool: mode=%v active=%v", m.mode, m.active)
	}
	// 第 1 行是第一个工具：点击照常打开。
	m = press(t, m, tea.MouseMsg{X: m.contentX() + 10, Y: browseHeaderRows + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.mode != modeTool || m.active == nil {
		t.Fatal("first tool row did not open")
	}
}

func TestToolRowsFillExactWidth(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: 80, Height: 24}, {Width: 100, Height: 20}} {
		m := press(t, New(), size)
		for i := range m.tools.VisibleItems() {
			var buf bytes.Buffer
			toolDelegate{hover: -1, focused: true}.Render(&buf, m.tools, i, m.tools.VisibleItems()[i])
			if got := lipgloss.Width(buf.String()); got != m.tools.Width() {
				t.Fatalf("%dx%d row %d width=%d, want %d", size.Width, size.Height, i, got, m.tools.Width())
			}
		}
	}
}

func TestBrowseLayoutFitsTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: 80, Height: 24}, {Width: 60, Height: 14}, {Width: 40, Height: 12}} {
		m := press(t, New(), size)
		view := m.View()
		if lipgloss.Height(view) != size.Height {
			t.Fatalf("%dx%d: height=%d", size.Width, size.Height, lipgloss.Height(view))
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.Width {
				t.Fatalf("%dx%d: line width %d", size.Width, size.Height, lipgloss.Width(line))
			}
		}
	}
}

func TestToolRowPrefixMatchesSpacingScale(t *testing.T) {
	if toolRowPrefix != 1+2+ui.Space1 {
		t.Fatalf("toolRowPrefix = %d, want 1+iconSlot+Space1 = %d", toolRowPrefix, 1+2+ui.Space1)
	}
	if columnGap != ui.Space2 {
		t.Fatalf("columnGap = %d, want Space2", columnGap)
	}
}

func TestToolColumnsAccountForEveryCell(t *testing.T) {
	for width := 30; width <= 140; width++ {
		cols := newToolColumns(width)
		total := toolRowPrefix + cols.title
		if cols.showDesc {
			total += columnGap + cols.desc
		}
		if total != width {
			t.Fatalf("width=%d: columns sum to %d", width, total)
		}
	}
}

func TestSidebarRowsCollapseToRailOnMediumScreens(t *testing.T) {
	m := press(t, New(), tea.WindowSizeMsg{Width: 60, Height: 20})
	rail := m.railView()
	if got := lipgloss.Width(strings.Split(rail, "\n")[0]); got != railWidth {
		t.Fatalf("rail width=%d, want %d", got, railWidth)
	}
	// 图标轨第二行是 Converters（第一行为 All tools），点击切换类别。
	m = press(t, m, tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.category != "converters" {
		t.Fatalf("rail click category=%q, want converters", m.category)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Converters") {
		t.Fatal("rail selection did not update the page title")
	}
}

func TestToolPageHeaderAndPadding(t *testing.T) {
	tool := &testTool{}
	m := New()
	m.tools.SetItems([]list.Item{listItem{kind: itemTool, tool: tool, title: "Test"}})
	m = press(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(t, m, tea.MouseMsg{X: m.contentX() + 1, Y: browseHeaderRows, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	lines := strings.Split(m.View(), "\n")
	// 页头第一行：工具标题；第二行必须是空白（页头与内容之间的一行留白）。
	if !strings.Contains(ansi.Strip(lines[0]), "Test") {
		t.Fatalf("tool header missing title: %q", ansi.Strip(lines[0]))
	}
	if rest := ansi.TruncateLeft(ansi.Strip(lines[1]), sidebarWidth, ""); strings.TrimSpace(rest) != "" {
		t.Fatalf("header should be followed by a blank row: %q", rest)
	}
	// 工具内容左侧留白 contentPad 列（按显示列计算，避免多字节图标干扰字节下标）。
	plain := ansi.Strip(lines[2])
	idx := strings.Index(plain, "test")
	if idx < 0 {
		t.Fatalf("tool content missing: %q", plain)
	}
	if col := lipgloss.Width(plain[:idx]); col != sidebarWidth+contentPad {
		t.Fatalf("tool content column = %d, want %d", col, sidebarWidth+contentPad)
	}
}

// 任意终端尺寸与各交互状态下都不得 panic，且画面恰为终端高度、不超宽。
func TestViewFillsAnySizeInEveryState(t *testing.T) {
	states := map[string]func(Model) Model{
		"browse":	func(m Model) Model { return m },
		"sidebar": func(m Model) Model {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
			return press(t, m, tea.KeyMsg{Type: tea.KeyDown})
		},
		"filter": func(m Model) Model {
			m = press(t, m, runes("/"))
			return press(t, m, runes("ju"))
		},
		"tool": func(m Model) Model {
			m.active, m.mode = &testTool{}, modeTool
			return m
		},
	}
	for name, setup := range states {
		for width := 1; width <= 124; width += 3 {
			for height := 1; height <= 42; height += 3 {
				m := setup(press(t, New(), tea.WindowSizeMsg{Width: width, Height: height}))
				view := m.View()
				if got := lipgloss.Height(view); got != height {
					t.Fatalf("%s %dx%d: height=%d", name, width, height, got)
				}
				for _, line := range strings.Split(view, "\n") {
					if lipgloss.Width(line) > width {
						t.Fatalf("%s %dx%d: line width %d", name, width, height, lipgloss.Width(line))
					}
				}
			}
		}
	}
}
