// Package filepicker provides a terminal file browser for selecting local files.
package filepicker

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type entry struct {
	name	string
	dir	bool
}

// pickerLayout 是一次布局的产物：渲染（View）与命中（Update/CancelAt）
// 共用同一份矩形，禁止两套手算算术。所有矩形均为 Picker 局部坐标。
type pickerLayout struct {
	box	ui.Rect	// 居中弹窗盒（含边框）
	innerW	int	// 盒内内容宽度（不含左右边框）
	listTop	int	// 第一条目行在盒内的行号（含上边框/标题栏/面包屑）
	rows	int	// 可见条目行数
	cancel	ui.Rect	// [Cancel] 按钮；W==0 表示放不下
	open	ui.Rect	// [Open selected] 按钮；W==0 表示不存在或放不下
}

// Picker displays the contents of a directory in a centered modal dialog and
// returns a selected file path. Coordinates passed to Update/CancelAt are
// relative to the area given to SetSize.
type Picker struct {
	dir				string
	entries				[]entry
	cursor, offset, width, height	int
	err				error
	loading				string
	multiple, confirmed		bool
	selected			map[string]bool
	selection			[]string
	modal				ui.Modal
}

// New opens the requested directory. Errors remain visible in the picker.
func New(dir string) *Picker {
	p := &Picker{}
	p.SetSize(80, 24)
	p.open(dir)
	return p
}

func (p *Picker) open(dir string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		p.err = err
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		p.err = err
		return
	}
	entries := []entry{{name: "..", dir: true}}
	for _, f := range files {
		isDir := f.IsDir()
		if f.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, f.Name())); err == nil {
				isDir = info.IsDir()
			}
		}
		entries = append(entries, entry{name: f.Name(), dir: isDir})
	}
	slices.SortFunc(entries[1:], func(a, b entry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	p.dir, p.entries, p.cursor, p.offset, p.err = dir, entries, 0, 0, nil
}

// SetMultiple enables selecting files across directories before opening them.
func (p *Picker) SetMultiple(enabled bool) {
	p.multiple = enabled
	if enabled && p.selected == nil {
		p.selected = make(map[string]bool)
	}
}
func (p *Picker) toggleSelection() {
	if len(p.entries) == 0 || p.entries[p.cursor].dir {
		return
	}
	path := filepath.Join(p.dir, p.entries[p.cursor].name)
	if p.selected[path] {
		delete(p.selected, path)
		p.selection = slices.DeleteFunc(p.selection, func(s string) bool { return s == path })
		p.err = nil
		return
	}
	if len(p.selection) >= 16 {
		p.err = fmt.Errorf("select at most 16 images at once")
		return
	}
	p.selected[path] = true
	p.selection = append(p.selection, path)
	p.err = nil
}

// TakeSelection returns confirmed paths once; a nil result means no batch is ready.
func (p *Picker) TakeSelection() []string {
	if !p.confirmed {
		return nil
	}
	p.confirmed = false
	return slices.Clone(p.selection)
}
func (p *Picker) confirm() {
	if len(p.selection) > 0 {
		p.confirmed = true
	}
}

// SetSize sets the available dimensions in terminal cells.
func (p *Picker) SetSize(width, height int) {
	p.width, p.height = max(1, width), max(1, height)
	p.modal.SetSize(p.width, p.height)
	p.reveal()
}
func (p *Picker) rows() int	{ return max(1, min(24, p.height-5)) }
func (p *Picker) reveal() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.rows() {
		p.offset = p.cursor - p.rows() + 1
	}
}
func (p *Picker) move(delta int) {
	if len(p.entries) == 0 {
		return
	}
	p.cursor = max(0, min(len(p.entries)-1, p.cursor+delta))
	p.reveal()
}
func (p *Picker) selectEntry() string {
	if len(p.entries) == 0 {
		return ""
	}
	e := p.entries[p.cursor]
	path := filepath.Join(p.dir, e.name)
	if e.dir {
		p.open(path)
		return ""
	}
	return path
}

// barButtons 是标题栏右侧的真实按钮：[Cancel] 恒在，
// 多选模式追加 [Open selected]（没有选中项时禁用）。
func (p *Picker) barButtons() []ui.Button {
	buttons := []ui.Button{{Label: "Cancel", Variant: ui.VariantGhost, Inline: true}}
	if p.multiple {
		buttons = append(buttons, ui.Button{
			Label:		"Open selected",
			Variant:	ui.VariantPrimary,
			Inline:		true,
			Disabled:	len(p.selection) == 0,
		})
	}
	return buttons
}

// layout 计算居中弹窗的盒矩形与按钮/列表矩形，并把盒子内容写入 p.modal。
// Modal 会给宿主正文加左右各 Space2 内边距（盒内宽 = 内容 + 4），
// 因此宿主内容预算 = 目标盒内宽 - 2*Space2。
func (p *Picker) layout() pickerLayout {
	areaW := p.width

	buttons := p.barButtons()
	views := make([]string, len(buttons))
	for i, b := range buttons {
		views[i] = b.View()
	}
	// 按钮间隔 Space2（弹窗按钮间隔刻度，与 modal.go 自带按钮行一致）；
	// 命中矩形 btnRects 与渲染共用同一份 PlaceRow 推导。
	btnRects, btnRow := ui.PlaceRow(views, ui.Space2)
	btnW := ui.Width(btnRow)

	// 盒内宽度：窄区域几乎占满，宽区域钳在 64 列；保证盒宽 ≤ 区域宽-2。
	inner := min(64, max(26, areaW-8))
	inner = min(inner, max(2, areaW-4))
	inner = max(2, inner)
	// 宿主内容宽（Modal 正文换行预算）：盒内宽 - 左右内边距。
	content := max(2, inner-2*ui.Space2)
	// 按钮行放不下时丢弃 [Open selected]（键盘 Ctrl+O 仍可确认），保证 [Cancel] 完整可见。
	if btnW > content && len(buttons) > 1 {
		buttons, views = buttons[:1], views[:1]
		btnRects, btnRow = ui.PlaceRow(views, ui.Space2)
		btnW = ui.Width(btnRow)
	}

	// 标题栏：bold 正文色标题 + 右对齐按钮（弹窗标题条属 chrome，
	// 不做 Accent 满宽反色）；放不下标题时按钮靠左。
	// [Cancel]/[Open selected] 的命中坐标依赖 titleW 的算式。
	titleW := max(0, content-btnW)
	titleText := ansi.Truncate("Open a file", max(1, titleW-2), "…")
	bar := lipgloss.NewStyle().
		Foreground(theme.Text).Bold(true).
		Width(titleW).
		Render(" " + titleText + " ")
	if titleW > 0 {
		bar += btnRow
	} else {
		bar = btnRow
	}

	lines := []string{bar, p.crumbLine(content)}
	for i := 0; i < p.rows(); i++ {
		lines = append(lines, p.entryLine(p.offset+i, content))
	}
	lines = append(lines, p.hintLine(content))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, content, "")
	}

	p.modal.Kind = ui.KindCustom
	p.modal.Title = ""
	p.modal.Width = content
	p.modal.Body = strings.Join(lines, "\n")
	box := p.modal.Rect()

	l := pickerLayout{box: box, innerW: content, listTop: 3, rows: p.rows()}
	// 正文首列 = 左边框 1 列 + Modal 正文内边距 Space2。
	btnX := box.X + 1 + ui.Space2
	if titleW > 0 {
		btnX += titleW
	}
	for i, br := range btnRects {
		r := ui.Rect{X: btnX + br.X, Y: box.Y + 1, W: br.W, H: 1}
		switch i {
		case 0:
			l.cancel = r
		case 1:
			l.open = r
		}
	}
	return l
}

// checkbox 渲染多选复选标记（选中=Success 图标，未选中=圆圈），光标行反色。
func (p *Picker) checkbox(checked, cursor bool) string {
	if cursor {
		return lipgloss.NewStyle().Foreground(theme.CursorText).Background(theme.Accent).Bold(true).
			Render(theme.Icon(theme.IconSuccess))
	}
	if checked {
		return theme.SuccessStyle().Render(theme.Icon(theme.IconSuccess))
	}
	return theme.MutedStyle().Render("○")
}

func (p *Picker) entryIcon(e entry, cursor bool) string {
	name := theme.IconFile
	style := theme.MutedStyle()
	if e.dir {
		name, style = theme.IconFolder, lipgloss.NewStyle().Foreground(theme.Accent)
	}
	if cursor {
		style = lipgloss.NewStyle().Foreground(theme.CursorText).Background(theme.Accent).Bold(true)
	}
	return style.Render(theme.Icon(name))
}

func (p *Picker) entryLine(i, innerW int) string {
	if i < 0 || i >= len(p.entries) {
		return ""
	}
	e := p.entries[i]
	cursor := i == p.cursor
	// 前缀槽位（多选文件行 = 复选框 + 空格，其余 = 两个空格）与图标后的
	// 间隔空格——宽度算式与渲染共用这一份槽位定义。
	var checkbox string
	if p.multiple && !e.dir {
		checkbox = p.checkbox(p.selected[filepath.Join(p.dir, e.name)], cursor)
	}
	prefixW := 2
	if checkbox != "" {
		prefixW = ui.Width(checkbox) + 1
	}
	icon := p.entryIcon(e, cursor)
	iconW := ui.Width(icon) + 1
	budget := max(0, innerW-prefixW-iconW)
	name := ansi.Truncate(displayName(e.name), budget, "…")
	if !cursor {
		line := "  "
		if checkbox != "" {
			line = checkbox + " "
		}
		line += icon + " " + name
		return ansi.Truncate(line, innerW, "")
	}
	// 光标行 = 整行 Accent 反色填充：checkbox/icon
	// 两段在构造处已带 cursor 样式，其余段（前缀空格、名段、行尾 pad）逐一
	// 补上同一样式——组合串整体包裹会在内层 reset 后失去前景/背景
	//（Modal 的 Surface 重注入会接管裸段），
	// 填充死于第一个 reset，选中行退化成 4 列图标 chip。
	cursorStyle := lipgloss.NewStyle().Foreground(theme.CursorText).Background(theme.Accent).Bold(true)
	pad := max(0, innerW-prefixW-iconW-ui.Width(name))
	var b strings.Builder
	if checkbox != "" {
		b.WriteString(checkbox)
		b.WriteString(cursorStyle.Render(" "))
	} else {
		b.WriteString(cursorStyle.Render("  "))
	}
	b.WriteString(icon)
	b.WriteString(cursorStyle.Render(" "))
	b.WriteString(cursorStyle.Render(name))
	b.WriteString(cursorStyle.Render(strings.Repeat(" ", pad)))
	return b.String()
}

func (p *Picker) crumbLine(innerW int) string {
	icon := theme.MutedStyle().Render(theme.Icon(theme.IconFolder)) + " "
	budget := max(0, innerW-ui.Width(icon)-2)
	text := ansi.Truncate(displayName(p.dir), budget, "…")
	return " " + icon + theme.MutedStyle().Render(text)
}

func (p *Picker) hintLine(innerW int) string {
	hint := "Click or enter to open  ·  scroll to browse  ·  esc cancel"
	style := theme.MutedStyle()
	if p.multiple {
		hint = fmt.Sprintf("Space/click select · Ctrl+O open · %d selected", len(p.selection))
	}
	switch {
	case p.err != nil:
		hint, style = "Error: "+displayName(p.err.Error()), theme.ErrorStyle()
	case p.loading != "":
		// 读取可能是 10 MiB 图片解码，期间给出可见进度与取消路径。
		hint = "Opening " + displayName(p.loading) + "…  ·  esc cancels"
		style = lipgloss.NewStyle().Foreground(theme.Blue)
	}
	return " " + style.Render(ansi.Truncate(hint, max(0, innerW-2), "…"))
}

// CancelAt reports a click on the modal's cancel button.
func (p *Picker) CancelAt(msg tea.MouseMsg) bool {
	return ui.HitIn(msg, p.layout().cancel)
}

// Update handles keyboard and mouse input and returns a selected path, if any.
func (p *Picker) Update(msg tea.Msg) string {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			p.move(-1)
		case "down", "j":
			p.move(1)
		case "pgup":
			p.move(-p.rows())
		case "pgdown":
			p.move(p.rows())
		case "backspace":
			p.open(filepath.Dir(p.dir))
		case " ":
			if p.multiple {
				p.toggleSelection()
			}
		case "ctrl+o":
			if p.multiple {
				p.confirm()
			}
		case "enter":
			if p.multiple && len(p.entries) > 0 && !p.entries[p.cursor].dir && len(p.selection) > 0 {
				p.confirm()
				break
			}
			return p.selectEntry()
		}
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			break
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			p.move(-3)
		case tea.MouseButtonWheelDown:
			p.move(3)
		case tea.MouseButtonLeft:
			l := p.layout()
			if !l.box.Contains(msg.X, msg.Y) {
				break
			}
			if l.open.W > 0 && ui.HitIn(msg, l.open) {
				if len(p.selection) > 0 {
					p.confirm()
				}
				break
			}
			row := msg.Y - l.box.Y - l.listTop
			// 条目行占 Modal 正文内容区：左边框 1 列 + 内边距 Space2 起。
			contentLeft := l.box.X + 1 + ui.Space2
			if row >= 0 && row < l.rows && msg.X >= contentLeft && msg.X < contentLeft+l.innerW && p.offset+row < len(p.entries) {
				p.cursor = p.offset + row
				if p.multiple && !p.entries[p.cursor].dir {
					p.toggleSelection()
					break
				}
				return p.selectEntry()
			}
		}
	}
	return ""
}

// SetError reports an operation error without leaving the picker.
func (p *Picker) SetError(err error)	{ p.err = err }

// SetLoading shows a progress hint for the file being read; esc still cancels.
func (p *Picker) SetLoading(name string)	{ p.loading = name }

// ClearLoading removes the progress hint.
func (p *Picker) ClearLoading()	{ p.loading = "" }

func displayName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, s)
}

// View renders the centered modal dialog over the whole area given to
// SetSize; the output is exactly p.height lines of p.width cells.
func (p *Picker) View() string {
	p.layout()
	return p.modal.Render("")
}
