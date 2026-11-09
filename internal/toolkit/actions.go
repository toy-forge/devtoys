package toolkit

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/ui"
)

// actions.go 把 "[Label]" 形式的按钮统一到 ui.Button/ui.Toolbar：
// 渲染与命中共用同一份 Rect 布局，本文件只是一层薄适配。
// 按钮为 1 行内联胶囊（Inline），宽度是 label+2，
// 因此工具层可按 lipgloss.Width(toolkit.Actions(...)) 手算偏移。

// actionButtons 把纯文本标签转换成内联按钮。
func actionButtons(labels []string, disabled bool) []ui.Button {
	buttons := make([]ui.Button, len(labels))
	for i, label := range labels {
		buttons[i] = ui.Button{Label: label, Disabled: disabled}
	}
	return buttons
}

// actionToolbar 构建一个宽度恰好容纳全部按钮的单行按钮条，
// 保证渲染（View）与命中（Index）使用同一份布局。
func actionToolbar(gap int, labels []string, disabled bool) *ui.Toolbar {
	t := &ui.Toolbar{Gap: max(0, gap)}
	t.SetButtons(actionButtons(labels, disabled)...)
	width := 0
	for i, b := range t.Buttons {
		if i > 0 {
			width += t.Gap
		}
		width += b.Width()
	}
	t.SetSize(max(width, 1), 0)
	return t
}

// Actions renders a row of clickable labels in terminal cells.
func Actions(labels ...string) string	{ return actionToolbar(1, labels, false).View() }

// CompactActions removes gaps between buttons when space is limited.
func CompactActions(labels ...string) string	{ return actionToolbar(0, labels, false).View() }

// DisabledCompactActions renders inactive controls without changing their hit-box width.
func DisabledCompactActions(labels ...string) string	{ return actionToolbar(0, labels, true).View() }

// ActionAt returns the clicked label index, or -1 for another event/location.
func ActionAt(msg tea.Msg, labels ...string) int	{ return actionToolbar(1, labels, false).Index(msg) }

// CompactActionAt hit-tests buttons rendered by CompactActions.
func CompactActionAt(msg tea.Msg, labels ...string) int {
	return actionToolbar(0, labels, false).Index(msg)
}

// ActionHeader lays out clickable controls across the available width.
// 布局/翻页/命中全部委托 ui.Toolbar；本结构只记录尺寸并转发。
type ActionHeader struct {
	width, height, rows	int
	bar			*ui.Toolbar
}

// toolbar 返回按当前标签与尺寸布局好的按钮条。
func (h *ActionHeader) toolbar(labels []string) *ui.Toolbar {
	if h.bar == nil {
		h.bar = &ui.Toolbar{Gap: 1}
	}
	h.bar.SetButtons(actionButtons(labels, false)...)
	h.bar.SetSize(max(h.width, 1), max(h.height, 0))
	return h.bar
}

// toolbarFor 是值接收者安全版本：必要时在副本上初始化按钮条。
func (h ActionHeader) toolbarFor(labels []string) *ui.Toolbar {
	if h.bar == nil {
		header := h
		header.bar = nil
		return header.toolbar(labels)
	}
	return h.bar
}

// compact 报告按钮条是否进入了两行分页模式。
func (h ActionHeader) compact(labels []string) bool {
	if h.height < 2 {
		return false
	}
	t := h.toolbarFor(labels)
	if t.Height() != 2 {
		return false
	}
	fit := *t
	fit.SetSize(max(h.width, 1), 0)
	return fit.Height() > 1
}

// Height returns the number of lines occupied by the current labels.
func (h ActionHeader) Height(labels ...string) int	{ return h.toolbarFor(labels).Height() }

// View renders the action buttons, paging them on short screens.
func (h ActionHeader) View(labels ...string) string	{ return h.toolbarFor(labels).View() }

// Index returns the clicked button index, or -1 when no button was clicked.
// Paging (wheel, pager row) is handled internally and also returns -1.
func (h *ActionHeader) Index(msg tea.Msg, labels ...string) int {
	t := h.toolbar(labels)
	index := t.Index(msg)
	h.rows = t.Height()
	return index
}

// SetSize updates a standalone action bar and returns its row count.
func (h *ActionHeader) SetSize(size tea.WindowSizeMsg, labels ...string) int {
	h.width, h.height = size.Width, size.Height
	h.rows = h.toolbar(labels).Height()
	return h.rows
}

// Resize records the available area and resizes the workspace beneath it.
func (h *ActionHeader) Resize(size tea.WindowSizeMsg, w *Workspace, labels ...string) tea.Cmd {
	rows := h.SetSize(size, labels...)
	return w.Update(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-rows)})
}

// Relayout updates the workspace after labels change their width.
func (h *ActionHeader) Relayout(w *Workspace, labels ...string) {
	if h.width > 0 && h.height > 0 && h.rows != h.Height(labels...) {
		h.Resize(tea.WindowSizeMsg{Width: h.width, Height: h.height}, w, labels...)
	}
}
