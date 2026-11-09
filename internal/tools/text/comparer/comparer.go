// Package comparer 提供文本比较工具：
// 左侧上下两个输入框分别放原文本与修改后的文本，右侧实时显示行级 diff，
// 支持"双栏对照"与"统一 diff"两种视图（ctrl+d 切换），增删行按颜色区分。
package comparer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/toy-forge/devtoys/internal/theme"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sergi/go-diff/diffmatchpatch"

	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// 焦点区域：tab 在三个区域之间循环切换。
type focusArea int

const (
	focusOrig	focusArea	= iota	// 原文本输入框
	focusNew				// 修改后输入框
	focusDiff				// 右侧差异视图（可滚动）
)

// diffKind 描述一行差异的类型。
type diffKind int

const (
	kindEqual	diffKind	= iota	// 两栏相同的行
	kindDel					// 仅原文本有（删除）
	kindAdd					// 仅修改后有（新增）
	kindMod					// 原文本与修改后内容不同（替换）
)

// diffRow 是双栏对照中的一行：左右各带行号与内容。
type diffRow struct {
	leftNum, rightNum	string	// 左右行号（空串表示该侧无内容）
	left, right		string	// 左右行内容（空串表示该侧无内容）
	kind			diffKind
}

// segment 是一段连续同类行的 diff 片段。
type segment struct {
	kind	diffKind
	lines	[]string
}

var (
	// 面板语言与 ui.Panel 同构：圆角边框 + 内容左右各 1 列
	// 内边距；focused 边框 = BorderFocus 焦点环。
	paneStyle	= lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(0, ui.Space1)

	paneStyleFocused	= lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.Accent).
				Padding(0, ui.Space1)

	labelStyle	= lipgloss.NewStyle().
			Foreground(theme.Muted)

	focusedLabelStyle	= lipgloss.NewStyle().
				Foreground(theme.Accent)

	errorStyle	= lipgloss.NewStyle().
			Foreground(theme.Text).
			Background(theme.Red).
			Padding(0, 1)

	hintStyle	= lipgloss.NewStyle().
			Foreground(theme.Muted)

	copiedStyle	= lipgloss.NewStyle().
			Foreground(theme.Green)

	equalStyle	= lipgloss.NewStyle().
			Foreground(theme.Border)

	delStyle	= lipgloss.NewStyle().
			Foreground(theme.Red)

	addStyle	= lipgloss.NewStyle().
			Foreground(theme.Green)

	modStyle	= lipgloss.NewStyle().
			Foreground(theme.YellowBright)
)

type paneRect struct{ x, y, width, height int }

func (r paneRect) contains(x, y int) bool {
	return r.width > 0 && x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}
func (r paneRect) interior(x, y int) bool {
	return r.width > 2 && r.height > 2 && x > r.x && x < r.x+r.width-1 && y > r.y && y < r.y+r.height-1
}

// Model 是文本比较工具。
type Model struct {
	orig, mod	textarea.Model	// 原文本 / 修改后输入框
	diff		viewport.Model	// 差异输出视口

	focus	focusArea
	unified	bool	// true 显示统一 diff，false 显示双栏对照
	header	toolkit.SettingHeader

	width, height			int
	headerHeight			int
	stacked, compact		bool
	diffExpanded			bool
	origRect, modRect, diffRect	paneRect
	err				error
	copied				bool

	// 最近一次计算的输入，用于避免重复求 diff。
	lastOrig, lastMod	string

	// rows 是当前 diff 的结构化结果；added/removed 是增删行数。
	rows		[]diffRow
	added		int
	removed		int
	summary		string		// 输出区首行的统计说明
	plainUnified	[]string	// 统一格式纯文本 diff，用于 ctrl+y 复制

	// canonical 页脚：ui.StatusBar 左句式 + 右端按钮组，
	// 渲染与命中共用 footerBarRect。
	footer		*ui.StatusBar
	footerBar	*ui.Toolbar
}

func init()	{ toolkit.Register(newModel()) }

func newModel() *Model {
	orig := textarea.New()
	orig.Placeholder = "Enter original text…"
	orig.ShowLineNumbers = false
	orig.SetWidth(30)
	orig.SetHeight(6)
	orig.Focus()

	mod := textarea.New()
	mod.Placeholder = "Enter modified text…"
	mod.ShowLineNumbers = false
	mod.SetWidth(30)
	mod.SetHeight(6)

	vp := viewport.New(40, 10)

	return &Model{
		orig:	orig, mod: mod, diff: vp,
		footer:		ui.NewStatusBar(),
		footerBar:	ui.NewToolbar(ui.Button{Label: "Copy"}, ui.Button{Label: "<"}, ui.Button{Label: ">"}),
		header: toolkit.SettingHeader{
			Names:		[]string{"Inline mode"},
			Icons:		[]string{"󰕚"},
			Switches:	[]bool{true},
			HideTitle:	true,
		},
		width:	80, height: 24,
	}
}

func (m *Model) ID() string		{ return "text.comparer" }
func (m *Model) Title() string		{ return "Text Comparer" }
func (m *Model) ShortTitle() string	{ return "Compare" }
func (m *Model) Category() string	{ return "text" }
func (m *Model) Icon() string		{ return "󰕚" }
func (m *Model) Description() string	{ return "Compare two texts line by line" }

// Init 满足 bubbletea 模型接口。
func (m *Model) Init() tea.Cmd	{ return nil }

// Update 处理窗口尺寸、视图切换与按键，输入变化时即时重算 diff。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// 紧凑布局用标签页取代设置条，首行留给标签。
		if msg.Height < 13 {
			m.headerHeight = 0
		} else {
			m.headerHeight = m.header.SetSize(msg, m.headerActions()...)
		}
		m.layout()
		m.recompute(true)
		return m, nil

	case toolkit.FileImportMsg:
		if m.focus == focusDiff {
			m.cycleFocus(int(focusOrig - m.focus))
		}
		var err error
		if m.focus == focusOrig {
			err = toolkit.InsertFileText(&m.orig, msg.Text)
		} else {
			err = toolkit.InsertFileText(&m.mod, msg.Text)
		}
		if err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		m.recompute(false)
		return m, nil

	case tea.MouseMsg:
		if !m.compact && m.header.Index(msg, m.headerActions()...) == 0 {
			m.unified = !m.unified
			m.renderDiff()
			return m, nil
		}
		if m.compact && msg.Y == m.headerHeight {
			if i := toolkit.ActionAt(msg, m.tabs()...); i >= 0 {
				m.cycleFocus(i - int(m.focus))
				return m, nil
			}
		}
		if msg.Y == m.height-1 {
			// 页脚右端 Copy / < / > 按钮组，渲染与命中
			// 共用 footerBarRect；错误/已复制时按钮被 Toast 替换不命中
			//（与 workspace footer 一致）。
			if m.err == nil && !m.copied {
				if r := m.footerBarRect(); r.contains(msg.X, 0) {
					button := msg
					button.Y = 0
					button.X -= r.x
					switch m.footerBar.Index(button) {
					case 0:
						if err := m.copyDiff(); err != nil {
							m.err, m.copied = err, false
						} else {
							m.err, m.copied = nil, true
						}
						return m, nil
					case 1:
						m.diff.ScrollLeft(max(4, m.diff.Width/2))
						return m, nil
					case 2:
						m.diff.ScrollRight(max(4, m.diff.Width/2))
						return m, nil
					}
				}
			}
			return m, nil
		}
		target := -1
		var rect paneRect
		switch {
		case m.origRect.contains(msg.X, msg.Y):
			target = int(focusOrig)
			rect = m.origRect
		case m.modRect.contains(msg.X, msg.Y):
			target = int(focusNew)
			rect = m.modRect
		case m.diffRect.contains(msg.X, msg.Y):
			target = int(focusDiff)
			rect = m.diffRect
		}
		if target < 0 || msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			switch focusArea(target) {
			case focusOrig:
				m.orig.Scroll(delta)
			case focusNew:
				m.mod.Scroll(delta)
			case focusDiff:
				var cmd tea.Cmd
				m.diff, cmd = m.diff.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft {
			m.cycleFocus(target - int(m.focus))
			if rect.interior(msg.X, msg.Y) {
				// 面板内容 1 列内边距，文本起点 = 边框内 2 列。
				x, y := msg.X-rect.x-2, msg.Y-rect.y-1
				switch focusArea(target) {
				case focusOrig:
					return m, m.orig.PlaceCursor(x, y)
				case focusNew:
					return m, m.mod.PlaceCursor(x, y)
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		// 复制成功后按任意键清除"已复制"提示。
		if m.copied && msg.String() != "ctrl+y" {
			m.copied = false
		}
		switch msg.String() {
		case "alt+e":
			if m.focus == focusDiff {
				m.diffExpanded = !m.diffExpanded
				m.layout()
			}
			return m, nil
		case "ctrl+d":
			m.unified = !m.unified
			m.renderDiff()
			return m, nil
		case "tab":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil
		case "ctrl+y":
			if err := m.copyDiff(); err != nil {
				m.err = err
				m.copied = false
			} else {
				m.err = nil
				m.copied = true
			}
			return m, nil
		}

		// 其余按键交给当前持有焦点的组件；随后即时重算。
		var cmd tea.Cmd
		switch m.focus {
		case focusOrig:
			m.orig, cmd = m.orig.Update(msg)
			m.recompute(false)
		case focusNew:
			m.mod, cmd = m.mod.Update(msg)
			m.recompute(false)
		case focusDiff:
			m.diff, cmd = m.diff.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// cycleFocus 按 dir（+1/-1）在三个区域间循环切换焦点。
func (m *Model) cycleFocus(dir int) {
	m.focus = (m.focus + focusArea(dir) + 3) % 3
	if m.focus != focusDiff {
		m.diffExpanded = false
	}
	m.orig.Blur()
	m.mod.Blur()
	switch m.focus {
	case focusOrig:
		m.orig.FocusPreservingScroll()
	case focusNew:
		m.mod.FocusPreservingScroll()
	}
	if m.compact {
		m.layout()
	}
}

// layout 依据窗口尺寸重新分配左右布局。
func (m *Model) layout() {
	m.stacked = m.width < 46
	m.compact = m.height < 13
	m.origRect, m.modRect, m.diffRect = paneRect{}, paneRect{}, paneRect{}
	if m.diffExpanded {
		height := max(3, m.height-m.headerHeight-2)
		m.diff.Width, m.diff.Height = max(1, m.width-4), max(1, height-2)
		m.diffRect = paneRect{0, m.headerHeight + 1, m.width, height}
		m.diff.SetYOffset(m.diff.YOffset)
		return
	}
	if m.compact {
		paneH := max(3, m.height-m.headerHeight-2)
		innerW, innerH := max(1, m.width-4), max(1, paneH-2)
		m.orig.SetWidth(innerW)
		m.orig.SetHeight(innerH)
		m.mod.SetWidth(innerW)
		m.mod.SetHeight(innerH)
		m.diff.Width, m.diff.Height = innerW, innerH
		r := paneRect{0, m.headerHeight + 1, m.width, paneH}
		switch m.focus {
		case focusOrig:
			m.origRect = r
		case focusNew:
			m.modRect = r
		case focusDiff:
			m.diffRect = r
		}
	} else if m.stacked {
		// 三个窗格纵向排列（标题嵌在边框上），窗格间各留一个空行。
		contentH := max(3, m.height-m.headerHeight-9)
		origH := max(1, contentH/4)
		modH := max(1, contentH/4)
		diffH := max(1, contentH-origH-modH)
		innerW := max(1, m.width-4)
		m.orig.SetWidth(innerW)
		m.orig.SetHeight(origH)
		m.mod.SetWidth(innerW)
		m.mod.SetHeight(modH)
		m.diff.Width, m.diff.Height = innerW, diffH
		m.origRect = paneRect{0, m.headerHeight, m.width, origH + 2}
		m.modRect = paneRect{0, m.headerHeight + origH + 3, m.width, modH + 2}
		m.diffRect = paneRect{0, m.headerHeight + origH + modH + 6, m.width, diffH + 2}
	} else {
		bodyH := m.height - m.headerHeight - 1
		// 双栏缝 Space2：中缝 2 列；面板内容 1 列内边距，
		// 文本区宽 = 外宽-4。
		leftW := (m.width - ui.Space2) / 2
		rightW := m.width - leftW - ui.Space2
		paneH := max(3, (bodyH-1)/3)
		diffH := max(3, bodyH-1-paneH)
		m.orig.SetWidth(leftW - 4)
		m.orig.SetHeight(paneH - 2)
		m.mod.SetWidth(rightW - 4)
		m.mod.SetHeight(paneH - 2)
		m.diff.Width, m.diff.Height = m.width-4, diffH-2
		m.origRect = paneRect{0, m.headerHeight, leftW, paneH}
		m.modRect = paneRect{leftW + ui.Space2, m.headerHeight, rightW, paneH}
		m.diffRect = paneRect{0, m.headerHeight + paneH + 1, m.width, diffH}
	}
	m.diff.SetYOffset(m.diff.YOffset)
}

// recompute 重算行级 diff。force 为 true 时不做缓存判断。
func (m *Model) recompute(force bool) {
	a, b := m.orig.Value(), m.mod.Value()
	if !force && a == m.lastOrig && b == m.lastMod {
		return
	}
	m.lastOrig, m.lastMod = a, b
	m.rows, m.added, m.removed = buildRows(diffSegments(a, b))
	m.renderDiff()
}

// renderDiff 将当前 rows 渲染成视图文本并写入输出视口。
func (m *Model) renderDiff() {
	switch {
	case m.lastOrig == "" && m.lastMod == "":
		m.summary = "Enter two texts to see a live diff."
	case m.added == 0 && m.removed == 0:
		m.summary = fmt.Sprintf("Texts are identical (%d lines).", len(m.rows))
	default:
		m.summary = fmt.Sprintf("%d changes: %d lines added, %d lines removed.",
			countHunks(m.rows), m.added, m.removed)
	}

	m.plainUnified = unifiedPlainLines(m.rows)

	var b strings.Builder
	b.WriteString(labelStyle.Render(m.summary))
	b.WriteString("\n")
	if len(m.rows) == 0 {
		b.WriteString(equalStyle.Render("(empty)"))
		b.WriteString("\n")
	}
	for _, r := range m.rows {
		if m.unified {
			b.WriteString(renderUnifiedRow(r))
		} else {
			b.WriteString(r.view(m.cellWidth()))
		}
		b.WriteString("\n")
	}
	m.diff.SetContent(strings.TrimRight(b.String(), "\n"))
}

// cellWidth 计算双栏视图中单列的可用宽度。
func (m *Model) cellWidth() int {
	w := (m.diff.Width - 1) / 2	// 两列 + 1 列间隔
	if w < 8 {
		w = 8
	}
	return w
}

// copyDiff 将当前差异以统一 diff 纯文本格式写入剪贴板。
func (m *Model) copyDiff() error {
	if m.lastOrig == "" && m.lastMod == "" {
		return fmt.Errorf("no diff to copy")
	}
	text := m.summary + "\n" + strings.Join(m.plainUnified, "\n")
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("could not copy to clipboard: %w", err)
	}
	return nil
}

// headerActions 是设置条的动作序列（Inline mode 开关）。
func (m *Model) headerActions() []string {
	inline := "Off"
	if m.unified {
		inline = "On"
	}
	return []string{inline}
}

// tabs 渲染紧凑布局的页签标签。
func (m *Model) tabs() []string {
	labels := []string{"Original Text", "Modified Text", "Difference"}
	labels[m.focus] = "*" + labels[m.focus]
	return labels
}

// embedTitle 把窗格标题（与可选按钮）嵌进圆角边框的顶行。
func embedTitle(pane, title string, focused bool, actions ...string) string {
	lines := strings.Split(pane, "\n")
	width := lipgloss.Width(lines[0])
	color := theme.Border
	if focused {
		color = theme.BlueBright
	}
	label := " " + title + " "
	lines[0] = ansi.Cut(lines[0], 0, 2) + lipgloss.NewStyle().Foreground(color).Bold(true).Render(label) + ansi.Cut(lines[0], 2+lipgloss.Width(label), width)
	if len(actions) > 0 {
		buttons := toolkit.CompactActions(actions...)
		if start := width - 1 - lipgloss.Width(buttons); start >= 2+lipgloss.Width(label) {
			lines[0] = ansi.Cut(lines[0], 0, start) + buttons + ansi.Cut(lines[0], start+lipgloss.Width(buttons), width)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) View() string {
	config := ""
	if !m.compact {
		config = m.header.View(m.headerActions()...) + "\n"
	}
	origPane := embedTitle(paneStyle.Render(m.orig.View()), "Original Text", m.focus == focusOrig)
	newPane := embedTitle(paneStyle.Render(m.mod.View()), "Modified Text", m.focus == focusNew)
	diffPaneStyle := paneStyle
	if m.focus == focusDiff {
		diffPaneStyle = paneStyleFocused
	}
	diffPane := embedTitle(diffPaneStyle.Render(m.diff.View()), "Difference", m.focus == focusDiff)
	if m.diffExpanded {
		return config + diffPane + "\n" + ansi.Truncate(m.footerView(), max(1, m.width), "")
	}
	var body string
	switch {
	case m.compact:
		pane := origPane
		switch m.focus {
		case focusNew:
			pane = newPane
		case focusDiff:
			pane = diffPane
		}
		body = ansi.Truncate(toolkit.Actions(m.tabs()...), max(1, m.width), "") + "\n" + pane
	case m.stacked:
		body = lipgloss.JoinVertical(lipgloss.Left, origPane, "", newPane, "", diffPane)
	default:
		// 双栏缝 Space2：中缝 2 列。
		body = lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, origPane, strings.Repeat(" ", ui.Space2), newPane), "", diffPane)
	}
	if m.compact {
		return body + "\n" + ansi.Truncate(m.footerView(), max(1, m.width), "")
	}
	return config + body + "\n" + ansi.Truncate(m.footerView(), max(1, m.width), "")
}

// footerBarRect 返回页脚右端按钮组的矩形（页面局部坐标，最后一行）。
func (m *Model) footerBarRect() paneRect {
	if m.width <= 0 {
		return paneRect{}
	}
	total := 0
	for i, b := range m.footerBar.Buttons {
		if i > 0 {
			total += max(0, m.footerBar.Gap)
		}
		total += b.Width()
	}
	x := max(0, m.width-1-total)
	return paneRect{x, 0, total, 1}
}

// footerView 渲染 canonical 底行：ui.StatusBar——左侧
// canonical 英文句式（ctrl+d 保留切换入口），右端 Copy / < / > 按钮；
// 错误与已复制以 Toast 呈现（与 workspace footer 同语言）。
func (m *Model) footerView() string {
	m.footer.SetWidth(max(1, m.width))
	left := "click to edit · scroll to browse · ctrl+d two columns"
	if m.focus == focusDiff || m.diffExpanded {
		label := "Expand"
		if m.diffExpanded {
			label = "Restore"
		}
		left += " · alt+E " + label
	}
	// 极窄终端让位给右侧按钮组（水平控件常驻）。
	if m.width < 30 {
		left = ""
	}
	m.footer.SetLeftTokens([]ui.StatusToken{{Label: left}})
	m.footer.SetHints()
	switch {
	case m.err != nil:
		m.footer.SetToast(ui.Toast{Level: ui.LevelError, Message: "Error: " + m.err.Error()})
	case m.copied:
		m.footer.SetToast(ui.Toast{Level: ui.LevelSuccess, Message: "Copied to clipboard"})
	default:
		m.footer.ClearToast()
		m.footer.SetRight(m.footerBar.View())
	}
	return m.footer.View()
}

// diffSegments 用 sergi/go-diff 计算行级差异并拆成片段：
// 先把每行映射成一个 rune（DiffLinesToRunes），对 rune 序列求 diff，
// 再还原（DiffCharsToLines）并拆回行，保证 diff 的最小单位是整行。
func diffSegments(a, b string) []segment {
	dmp := diffmatchpatch.New()
	r1, r2, lineArray := dmp.DiffLinesToRunes(a, b)
	diffs := dmp.DiffMainRunes(r1, r2, false)
	hydrated := dmp.DiffCharsToLines(diffs, lineArray)

	segs := make([]segment, 0, len(hydrated))
	for _, d := range hydrated {
		kind := kindEqual
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			kind = kindDel
		case diffmatchpatch.DiffInsert:
			kind = kindAdd
		}
		if lines := splitKeepNewline(d.Text); len(lines) > 0 {
			segs = append(segs, segment{kind: kind, lines: lines})
		}
	}
	return segs
}

// buildRows 把 diff 片段拼装成带全局行号的双栏行：
// 连续的删除与新增被配对成"替换"行（kindMod），多出的一侧按纯增/删处理。
func buildRows(segs []segment) (rows []diffRow, added, removed int) {
	rows = make([]diffRow, 0, len(segs))
	ln, rn := 0, 0	// 左右两侧行号计数器
	var dels, inss []string

	// flush 把积攒的删除/新增行配对写入 rows。
	flush := func() {
		n := len(dels)
		if len(inss) > n {
			n = len(inss)
		}
		for i := 0; i < n; i++ {
			var r diffRow
			switch {
			case i < len(dels) && i < len(inss):
				ln++
				rn++
				r.kind = kindMod
				r.leftNum, r.left = fmt.Sprint(ln), dels[i]
				r.rightNum, r.right = fmt.Sprint(rn), inss[i]
				added++
				removed++
			case i < len(dels):
				ln++
				r.kind = kindDel
				r.leftNum, r.left = fmt.Sprint(ln), dels[i]
				removed++
			default:
				rn++
				r.kind = kindAdd
				r.rightNum, r.right = fmt.Sprint(rn), inss[i]
				added++
			}
			rows = append(rows, r)
		}
		dels, inss = dels[:0], inss[:0]
	}

	for _, seg := range segs {
		if seg.kind != kindEqual {
			if seg.kind == kindDel {
				dels = append(dels, seg.lines...)
			} else {
				inss = append(inss, seg.lines...)
			}
			continue
		}
		flush()
		for _, line := range seg.lines {
			ln++
			rn++
			rows = append(rows, diffRow{
				leftNum:	fmt.Sprint(ln), rightNum: fmt.Sprint(rn),
				left:	line, right: line, kind: kindEqual,
			})
		}
	}
	flush()
	return rows, added, removed
}

// splitKeepNewline 按行拆分文本，保留每行末尾的换行符（最后一行可能没有），
// 拆分结果重新拼接后与原文完全一致。
func splitKeepNewline(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if last := lines[len(lines)-1]; last == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// plain 去掉行尾换行，得到用于显示/复制的干净行文本。
func plain(s string) string	{ return strings.TrimSuffix(s, "\n") }

// view 渲染双栏对照中的一行：行号 + 内容，按差异类型着色，超宽截断。
func (r diffRow) view(cellW int) string {
	numW := numWidth(cellW)
	contentW := cellW - numW - 1
	if contentW < 1 {
		contentW = 1
	}
	st := lineStyle(r.kind)
	left := st.Render(numCell(r.leftNum, numW) + " " + textCell(plain(r.left), contentW))
	right := st.Render(numCell(r.rightNum, numW) + " " + textCell(plain(r.right), contentW))
	return left + " " + right
}

// renderUnifiedRow 渲染统一 diff 的一行：+ / - / 空格前缀着色。
func renderUnifiedRow(r diffRow) string {
	switch r.kind {
	case kindAdd:
		return addStyle.Render("+ " + plain(r.right))
	case kindDel:
		return delStyle.Render("- " + plain(r.left))
	case kindMod:
		return delStyle.Render("- "+plain(r.left)) + "\n" +
			addStyle.Render("+ "+plain(r.right))
	default:
		return equalStyle.Render("  " + plain(r.left))
	}
}

// unifiedPlainLines 生成统一格式纯文本 diff（与当前视图无关，供复制使用）。
func unifiedPlainLines(rows []diffRow) []string {
	out := make([]string, 0, len(rows)+2)
	for _, r := range rows {
		switch r.kind {
		case kindAdd:
			out = append(out, "+ "+plain(r.right))
		case kindDel:
			out = append(out, "- "+plain(r.left))
		case kindMod:
			out = append(out, "- "+plain(r.left), "+ "+plain(r.right))
		default:
			out = append(out, "  "+plain(r.left))
		}
	}
	return out
}

// countHunks 统计连续差异块的个数。
func countHunks(rows []diffRow) int {
	n, inHunk := 0, false
	for _, r := range rows {
		if r.kind != kindEqual {
			if !inHunk {
				n++
				inHunk = true
			}
			continue
		}
		inHunk = false
	}
	return n
}

// numWidth 依据列宽推算行号列宽度（3~5 位）。
func numWidth(cellW int) int {
	w := cellW / 8
	if w < 3 {
		w = 3
	}
	if w > 5 {
		w = 5
	}
	return w
}

// lineStyle 返回差异类型对应的着色样式。
func lineStyle(k diffKind) lipgloss.Style {
	switch k {
	case kindAdd:
		return addStyle
	case kindDel:
		return delStyle
	case kindMod:
		return modStyle
	default:
		return equalStyle
	}
}

// numCell 渲染行号单元格：右对齐，无行号时补空白。
func numCell(num string, w int) string {
	if num == "" {
		return strings.Repeat(" ", w)
	}
	pad := w - lipgloss.Width(num)
	if pad < 0 {
		num = "…"
		pad = w - 1
	}
	return strings.Repeat(" ", max(pad, 0)) + num
}

// textCell 渲染内容单元格：左对齐；超宽时截断并在末尾加省略号。
func textCell(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s + strings.Repeat(" ", w-lipgloss.Width(s))
	}
	if w == 1 {
		return "…"
	}
	for lipgloss.Width(s) > w-1 {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s + "…"
}
