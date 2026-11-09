// Package app 实现 DevToys TUI 的主界面外壳：
// 左侧为工具类别导航（bubbles/list），右侧为当前类别下的工具列表
// （同样是 bubbles/list）或当前打开的工具，底部为状态栏。
// 几何见 shell.go，列表与行样式见 home.go，文件读取见 files.go。
package app

import (
	"os"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/toolkit/filepicker"
	"github.com/toy-forge/devtoys/internal/toolkit/imageview"
	"github.com/toy-forge/devtoys/internal/ui"
)

type mode int

const (
	modeBrowse	mode	= iota	// 浏览工具列表
	modeTool			// 工具打开中
)

// statusToken 是状态栏左侧的一个文本段；label 用于渲染，
// action 非 statusNone 时该段可点击。
type statusToken struct {
	label	string
	action	uint8
}

const (
	statusNone	uint8	= iota
	statusBack		// [Tools]：返回工具列表
	statusOpen		// [Open file]：打开文件选择器
)

// statusAction 是状态栏上一个可点击区域（全局坐标，最后一行）。
type statusAction struct {
	rect	ui.Rect
	action	uint8
}

// Model 是应用主模型。
type Model struct {
	mode	mode
	tools	list.Model	// 内容区工具列表（当前类别）
	nav	list.Model	// 侧栏类别导航
	active	toolkit.Tool
	width	int
	height	int
	picker	*filepicker.Picker
	preview	*imageview.View
	loading	bool
	notice	string
	// category 是当前类别；空串表示 All tools。
	category	string
	// navFocus 表示浏览模式下键盘焦点在侧栏（Tab 切换）。
	navFocus	bool

	// 指针追踪：motion 消息更新坐标，sync 据此换算 hover，不派生命令。
	pointerX, pointerY	int
	hasPointer		bool
}

// New 创建主模型，读取注册表中全部工具并填充侧栏与工具列表。
func New() Model {
	m := Model{mode: modeBrowse, tools: newToolList(), nav: newNavList()}
	m.nav.SetItems(navItems())
	m.tools.SetItems(m.toolItems())
	m.selectNav()
	m.sync()
	return m
}

func (m Model) Init() tea.Cmd	{ return nil }

func (m Model) acceptsFiles() bool {
	if m.active == nil {
		return false
	}
	if t, ok := m.active.(toolkit.FileInput); ok {
		return t.AcceptsFiles()
	}
	return true
}

func (m Model) openPicker() Model {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	m.picker = filepicker.New(dir)
	if receiver, ok := m.active.(toolkit.ImageBatchImporter); ok && receiver.AcceptsImageBatch() {
		m.picker.SetMultiple(true)
	}
	m.picker.SetSize(m.innerWidth(), m.bodyHeight())
	m.loading = false
	return m
}

// Update 先按消息更新模型，再把委托的焦点与 hover 同步到最新状态。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if model, ok := next.(Model); ok {
		model.sync()
		return model, cmd
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if request, ok := msg.(toolkit.ForegroundCommandMsg); ok {
		allowed := request.Tool != nil && m.active != nil && m.mode == modeTool && reflect.TypeOf(m.active).Comparable() && m.active == request.Tool && m.picker == nil && m.preview == nil
		if allowed && (request.Valid == nil || request.Valid()) {
			return m, request.Command
		}
		if request.OnDrop != nil {
			request.OnDrop()
		}
		return m, nil
	}
	if result, ok := msg.(toolkit.BackgroundToolMsg); ok && result.Tool != nil {
		if m.active != nil && m.active.ID() == result.Tool.ID() {
			return m.forwardToActive(result.Message)
		}
		_, cmd := result.Tool.Update(result.Message)
		return m, cmd
	}

	if _, ok := msg.(toolkit.OpenFileRequestMsg); ok && m.acceptsFiles() && m.picker == nil {
		return m.openPicker(), nil
	}
	if failure, ok := msg.(editor.PasteErrorMsg); ok {
		m.notice = failure.Err.Error()
		return m, nil
	}
	// 通知的清除语义：按键与鼠标按下清除，指针滑动（motion）不清除，
	// 避免划过屏幕就抹掉错误提示。
	switch event := msg.(type) {
	case tea.KeyMsg:
		m.notice = ""
	case tea.MouseMsg:
		if event.Action != tea.MouseActionMotion {
			m.notice = ""
		}
	}
	if result, ok := msg.(nativeImageResult); ok {
		if result.err != nil {
			m.notice = result.err.Error()
		}
		return m, nil
	}
	if result, ok := msg.(fileBatchResult); ok {
		if m.picker == nil || result.picker != m.picker {
			return m, nil
		}
		m.loading = false
		if result.err != nil {
			m.picker.SetError(result.err)
			return m, nil
		}
		if receiver, ok := m.active.(toolkit.ImageBatchImporter); !ok || !receiver.AcceptsImageBatch() {
			return m, nil
		}
		m.picker = nil
		return m.forwardToActive(toolkit.ImageBatchImportMsg{Images: result.images})
	}
	if result, ok := msg.(fileResult); ok {
		if m.picker == nil || result.picker != m.picker {
			return m, nil
		}
		m.loading = false
		if receiver, ok := m.active.(toolkit.BinaryImporter); ok && receiver.AcceptsBinary() && result.data != nil && result.image == nil {
			m.picker = nil
			return m.forwardToActive(toolkit.BinaryImportMsg{Data: result.data, Name: result.name})
		}
		if result.err != nil {
			m.picker.SetError(result.err)
			return m, nil
		}
		if result.image != nil {
			if receiver, ok := m.active.(toolkit.ImageImporter); ok && receiver.AcceptsImages() {
				m.picker = nil
				return m.forwardToActive(toolkit.ImageImportMsg{Image: result.image, Name: result.name, Data: result.data})
			}
			m.preview = imageview.New(result.image, result.name)
			m.preview.SetSize(m.innerWidth(), m.bodyHeight())
			return m, nil
		}
		m.picker = nil
		return m.forwardToActive(toolkit.FileImportMsg{Text: result.text})
	}
	if m.picker != nil {
		return m.updatePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		if m.mode == modeTool && m.active != nil {
			return m.forwardToActive(m.toolSize())
		}
		return m, nil

	case tea.MouseMsg:
		return m.mouse(msg)

	case tea.KeyMsg:
		if next, cmd, handled := m.key(msg); handled {
			return next, cmd
		}
	}

	if m.mode == modeTool && m.active != nil {
		return m.forwardToActive(msg)
	}
	// 浏览模式：按键与非按键消息交给当前焦点列表（侧栏或工具列表）。
	if m.navFocus {
		if _, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.nav, cmd = m.nav.Update(msg)
			m.syncCategoryFromNav()
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.tools, cmd = m.tools.Update(msg)
	return m, cmd
}

// updatePicker 在文件选择器打开期间接管输入：选择器是覆盖在内容区上的模态层。
func (m Model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(event.Width, event.Height)
		return m.forwardToActive(m.toolSize())
	case tea.KeyMsg:
		if event.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if event.String() == "i" && m.preview != nil && imageview.NativeAvailable() {
			return m, m.preview.Native(func(err error) tea.Msg { return nativeImageResult{err: err} })
		}
		if event.String() == "esc" {
			if m.preview != nil {
				m.preview = nil
				return m, nil
			}
			m.picker = nil
			m.loading = false
			return m, nil
		}
	case tea.MouseMsg:
		if event.X < m.contentX() {
			return m, nil
		}
		event.X -= m.contentX()
		if m.preview != nil {
			// 图片预览是居中 Modal：点击盒子外关闭，盒内不透传。
			if event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft && !m.preview.ModalRect().Contains(event.X, event.Y) {
				m.preview = nil
			}
			return m, nil
		}
		if m.picker.CancelAt(event) {
			m.picker = nil
			m.loading = false
			return m, nil
		}
		msg = event
	}
	if m.preview != nil {
		return m, nil
	}
	if !m.loading {
		path := m.picker.Update(msg)
		if paths := m.picker.TakeSelection(); len(paths) > 0 {
			m.loading = true
			return m, readSelectedImages(m.picker, paths)
		}
		if path != "" {
			if receiver, ok := m.active.(toolkit.FilePathImporter); ok && receiver.AcceptsFilePaths() {
				m.picker = nil
				return m.forwardToActive(toolkit.FilePathImportMsg{Path: path})
			}
			m.loading = true
			return m, readSelectedFile(m.picker, path)
		}
	}
	return m, nil
}

// key 处理外壳级快捷键。handled=false 时按键继续交给工具或焦点列表。
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true

	case "ctrl+f":
		if m.mode == modeTool && m.acceptsFiles() {
			return m.openPicker(), nil, true
		}

	case "/":
		// 搜索属于工具列表：焦点在侧栏时 "/" 先把焦点交还列表再开始过滤。
		if m.mode == modeBrowse && m.navFocus {
			m.navFocus = false
			var cmd tea.Cmd
			m.tools, cmd = m.tools.Update(msg)
			return m, cmd, true
		}

	case "esc":
		if m.mode == modeTool {
			if handler, ok := m.active.(toolkit.EscapeHandler); ok && handler.HandlesEscape() {
				return m, nil, false
			}
			m.mode, m.active = modeBrowse, nil
			return m, nil, true
		}
		if m.navFocus {
			m.navFocus = false
			return m, nil, true
		}

	case "tab":
		if m.mode == modeBrowse && m.tools.FilterState() != list.Filtering {
			m.navFocus = !m.navFocus
			return m, nil, true
		}

	case "left":
		// 工具列表无过滤时，左键把焦点移到侧栏（与 Tab 同义）。
		if m.mode == modeBrowse && !m.navFocus && m.tools.FilterState() == list.Unfiltered {
			m.navFocus = true
			return m, nil, true
		}

	case "right":
		if m.mode == modeBrowse && m.navFocus {
			m.navFocus = false
			return m, nil, true
		}

	case "enter":
		if m.mode == modeBrowse && m.tools.FilterState() != list.Filtering {
			if m.navFocus {
				m.navFocus = false
				return m, nil, true
			}
			if sel, ok := m.tools.SelectedItem().(listItem); ok && sel.kind == itemTool {
				m.selectTool(sel.tool)
				next, cmd := m.forwardToActive(m.toolSize())
				return next, cmd, true
			}
			return m, nil, true
		}
	}
	return m, nil, false
}

// mouse 把全局鼠标事件分派给状态栏、侧栏、内容区或工具。
func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// 指针追踪：每次鼠标消息刷新坐标；motion 只驱动 hover，不派生命令。
	m.pointerX, m.pointerY, m.hasPointer = msg.X, msg.Y, true
	if msg.Action == tea.MouseActionMotion {
		if m.mode == modeTool && m.active != nil && msg.X >= m.contentX() {
			local := msg
			local.X -= m.contentX()
			local.Y -= m.toolHeaderHeight()
			if local.Y >= 0 {
				return m.forwardToActive(local)
			}
		}
		return m, nil
	}
	// 状态栏（最后一行）：[Tools] / [Open file] 点击区。
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == m.height-1 {
		for _, action := range m.statusActions() {
			if !action.rect.Contains(msg.X, msg.Y) {
				continue
			}
			switch action.action {
			case statusBack:
				m.mode, m.active = modeBrowse, nil
				return m, nil
			case statusOpen:
				return m.openPicker(), nil
			}
		}
	}
	if msg.X < m.sidebarColumns() {
		return m.sidebarMouse(msg)
	}
	if msg.X < m.contentX() {
		return m, nil
	}
	if m.mode == modeTool && m.active != nil {
		msg.X -= m.contentX()
		msg.Y -= m.toolHeaderHeight()
		if msg.Y < 0 {
			return m, nil
		}
		return m.forwardToActive(msg)
	}
	return m.browseMouse(msg)
}

// sidebarMouse 处理侧栏（或图标轨）上的点击与滚轮。
func (m Model) sidebarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.railMode() {
		return m.railClick(msg)
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		if m.mode != modeBrowse {
			return m, nil
		}
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		m.navFocus = true
		if n := len(m.nav.VisibleItems()); n > 0 {
			m.nav.Select(max(0, min(n-1, m.nav.Index()+delta)))
		}
		m.syncCategoryFromNav()
	case tea.MouseButtonLeft:
		idx := m.navIndexAt(msg.X, msg.Y)
		if idx < 0 {
			return m, nil
		}
		// 点击类别即返回浏览页并切换到该类别（与 Finder 侧栏的语义一致）。
		m.mode, m.active = modeBrowse, nil
		m.navFocus = true
		m.nav.Select(idx)
		m.syncCategoryFromNav()
	}
	return m, nil
}

// railClick 处理图标轨上的左键点击：每个图标对应一个类别（logo = All tools）。
func (m Model) railClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	entries := navEntries()
	if msg.Y < 0 || msg.Y >= len(entries) {
		return m, nil
	}
	m.mode, m.active, m.navFocus = modeBrowse, nil, false
	m.setCategory(entries[msg.Y].category)
	return m, nil
}

// browseMouse 处理内容区（浏览模式）的点击与滚轮。
func (m Model) browseMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	x := msg.X - m.contentX()
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		m.navFocus = false
		if n := len(m.tools.VisibleItems()); n > 0 {
			m.tools.Select(max(0, min(n-1, m.tools.Index()+delta)))
		}
	case tea.MouseButtonLeft:
		// 搜索行：点击即开始过滤。
		if msg.Y == browseHeaderRows-3 && m.tools.FilterState() != list.Filtering {
			m.navFocus = false
			var cmd tea.Cmd
			m.tools, cmd = m.tools.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			return m, cmd
		}
		idx := m.toolIndexAt(x, msg.Y)
		if idx < 0 {
			return m, nil
		}
		m.tools.Select(idx)
		m.navFocus = false
		if sel, ok := m.tools.SelectedItem().(listItem); ok && sel.kind == itemTool {
			m.selectTool(sel.tool)
			return m.forwardToActive(m.toolSize())
		}
	}
	return m, nil
}

// forwardToActive 把消息转发给当前工具并回写其返回的模型。
func (m Model) forwardToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.active.Update(msg)
	if t, ok := next.(toolkit.Tool); ok {
		m.active = t
	}
	return m, cmd
}

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	body := m.contentView()
	// 遮罩开合：picker/preview 都是模态层，整窗变暗——
	// 内容区由 Modal.Render 压暗，侧栏与状态栏在下方补齐。
	masked := m.picker != nil || m.preview != nil
	if m.sidebarColumns() != 0 {
		sidebar := m.sidebarBox()
		if masked {
			sidebar = dimLines(sidebar)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, body)
	}
	lines := strings.Split(body, "\n")
	lines = lines[:min(len(lines), max(0, m.height-1))]
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusView())
	if masked {
		// 遮罩帧不走画布底色：canvas.Background 会盖住行首注入的 Overlay
		// 底。行宽由各区域铺满保证；这里只补最后一行状态栏。
		for i, line := range lines {
			line = ansi.Truncate(line, m.width, "")
			if i == len(lines)-1 {
				line = ui.DimLine(line)
			}
			lines[i] = line
		}
		return strings.Join(lines, "\n")
	}
	canvas := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background).Width(m.width)
	for i, line := range lines {
		lines[i] = canvas.Render(ansi.Truncate(line, m.width, ""))
	}
	return strings.Join(lines, "\n")
}

// dimLines 把多行渲染结果逐行压暗（遮罩铺满整窗的侧栏部分）。
func dimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = ui.DimLine(line)
	}
	return strings.Join(lines, "\n")
}

// statusLeftTokens 组成状态栏左侧 token 序列；渲染
// （statusView → ui.StatusBar.SetLeftTokens）与点击命中（statusActions）
// 共用同一份序列：action 非 statusNone 的 token 即可点击（Accent+bold）。
func (m Model) statusLeftTokens() []ui.StatusToken {
	tokens := make([]ui.StatusToken, 0, 3)
	for _, token := range m.statusTokenList() {
		tokens = append(tokens, ui.StatusToken{Label: token.label, Clickable: token.action != statusNone})
	}
	return tokens
}

// statusTokenList 是带点击动作的 token 源序列；statusLeftTokens 与
// statusActions 都从这里推导，禁止两套手算。
func (m Model) statusTokenList() []statusToken {
	if m.mode != modeTool || m.active == nil {
		return []statusToken{{theme.Icon(theme.IconLogo) + " " + m.browseTitle(), statusNone}}
	}
	var tokens []statusToken
	if m.sidebarColumns() == 0 {
		tokens = append(tokens, statusToken{"[Tools]", statusBack})
	}
	if m.acceptsFiles() {
		tokens = append(tokens, statusToken{"[Open file]", statusOpen})
	}
	// 页头已显示工具标题时状态栏不再重复；只有页头让位（矮屏）时才由状态栏承担。
	if m.toolHeaderHeight() == 0 {
		tokens = append(tokens, statusToken{theme.ToolIcon(m.active.ID()) + " " + m.active.Title(), statusNone})
	}
	return tokens
}

// statusActions 计算状态栏左侧可点击区域（全局坐标，最后一行）。
// 通知显示时整行被错误 Toast 占据（与 statusView 渲染一致），无可点击区域。
func (m Model) statusActions() []statusAction {
	if m.notice != "" {
		return nil
	}
	var actions []statusAction
	x := 1	// StatusBar 左内边距
	for _, token := range m.statusTokenList() {
		width := lipgloss.Width(token.label)
		if token.action != statusNone {
			actions = append(actions, statusAction{
				rect:	ui.Rect{X: x, Y: m.height - 1, W: width, H: 1},
				action:	token.action,
			})
		}
		x += width + ui.Space2	// token 间距 Space2
	}
	return actions
}

// statusHints 是状态栏右侧按键提示：每个焦点区域只列它真正响应的键。
func (m Model) statusHints() []ui.StatusHint {
	if m.mode == modeTool && m.active != nil {
		hints := []ui.StatusHint{{Key: "esc", Desc: "back"}}
		if m.acceptsFiles() {
			hints = append(hints, ui.StatusHint{Key: "ctrl+f", Desc: "open file"})
		}
		return append(hints, ui.StatusHint{Key: "tab", Desc: "focus"}, ui.StatusHint{Key: "ctrl+y", Desc: "copy"})
	}
	if m.navFocus {
		return []ui.StatusHint{{Key: "↑↓", Desc: "category"}, {Key: "tab", Desc: "tools"}, {Key: "ctrl+c", Desc: "quit"}}
	}
	return []ui.StatusHint{{Key: "/", Desc: "search"}, {Key: "enter", Desc: "open"}, {Key: "tab", Desc: "categories"}, {Key: "ctrl+c", Desc: "quit"}}
}

// statusView 渲染最后一行：ui.StatusBar，notice 以 ui.Toast 呈现。
func (m Model) statusView() string {
	bar := ui.NewStatusBar()
	bar.SetWidth(max(0, m.width))
	if m.notice != "" {
		toast, _ := ui.NewToast(ui.LevelError, m.notice, 0)
		bar.SetToast(toast)
		return bar.View()
	}
	bar.SetLeftTokens(m.statusLeftTokens())
	bar.SetHints(m.statusHints()...)
	return bar.View()
}
