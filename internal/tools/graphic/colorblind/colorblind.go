// Package colorblind previews Brettel 1997 dichromacy simulations.
package colorblind

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/imageview"
	"github.com/toy-forge/devtoys/internal/ui"
)

// Brettel coefficients are from libDaltonLens (Unlicense), also used by
// DevToys.Tools. See DALTONLENS-LICENSE in this directory.
type parameters struct {
	first, second	[9]float64
	normal		[3]float64
}

var transforms = []parameters{
	{},
	{first: [9]float64{.14980, 1.19548, -.34528, .10764, .84864, .04372, .00384, -.00540, 1.00156}, second: [9]float64{.14570, 1.16172, -.30742, .10816, .85291, .03892, .00386, -.00524, 1.00139}, normal: [3]float64{.00048, .00393, -.00441}},
	{first: [9]float64{1.01277, .13548, -.14826, -.01243, .86812, .14431, .07589, .80500, .11911}, second: [9]float64{.93678, .18979, -.12657, .06154, .81526, .12320, -.37562, 1.12767, .24796}, normal: [3]float64{.03901, -.02788, -.01113}},
	{first: [9]float64{.36477, .86381, -.22858, .26294, .64245, .09462, -.02006, .02728, .99278}, second: [9]float64{.37298, .88166, -.25464, .25954, .63506, .10540, -.01980, .02784, .99196}, normal: [3]float64{-.00281, -.00611, .00892}},
}
var names = []string{"Original", "Protanopia simulation", "Tritanopia simulation", "Deuteranopia simulation"}

type simulated struct {
	source	image.Image
	kind	int
}

func (s simulated) ColorModel() color.Model	{ return color.NRGBAModel }
func (s simulated) Bounds() image.Rectangle	{ return s.source.Bounds() }
func (s simulated) At(x, y int) color.Color {
	c := color.NRGBAModel.Convert(s.source.At(x, y)).(color.NRGBA)
	if s.kind == 0 {
		return c
	}
	p := transforms[s.kind]
	rgb := [3]float64{linearTable[c.R], linearTable[c.G], linearTable[c.B]}
	m := p.first
	if rgb[0]*p.normal[0]+rgb[1]*p.normal[1]+rgb[2]*p.normal[2] < 0 {
		m = p.second
	}
	return color.NRGBA{R: srgb(m[0]*rgb[0] + m[1]*rgb[1] + m[2]*rgb[2]), G: srgb(m[3]*rgb[0] + m[4]*rgb[1] + m[5]*rgb[2]), B: srgb(m[6]*rgb[0] + m[7]*rgb[1] + m[8]*rgb[2]), A: c.A}
}

var linearTable = func() [256]float64 {
	var table [256]float64
	for i := range table {
		table[i] = linear(uint8(i))
	}
	return table
}()

func linear(v uint8) float64 {
	x := float64(v) / 255
	if x < .04045 {
		return x / 12.92
	}
	return math.Pow((x+.055)/1.055, 2.4)
}
func srgb(x float64) uint8 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 255
	}
	if x < .0031308 {
		return uint8(.5 + x*12.92*255)
	}
	return uint8(255 * (math.Pow(x, 1/2.4)*1.055 - .055))
}

type previewDone struct{ err error }
type gridPrepared struct {
	revision	uint64
	payloads	[]string
	err		error
}
type gridDone struct{ err error }
type nativePrepared struct {
	revision	uint64
	kind		int
	payload		string
	err		error
}
type tool struct {
	img						image.Image
	name						string
	width, height, selected				int
	err						error
	status						string
	revision					uint64
	preparing, gridPreparing, saving, editing	bool
	previewPending, gridPending			bool
	pathRevision					uint64
	pathKind					int
	path						textinput.Model
	cacheRevision					uint64
	cacheKind					int
	cachePayload					string
	gridRevision					uint64
	gridPayloads					[]string
}

func newTool() *tool {
	path := textinput.New()
	path.Prompt = "┃ "
	path.CharLimit = 4096
	path.Width = 78
	return &tool{width: 80, height: 24, path: path}
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "graphic.colorblind" }
func (*tool) Title() string		{ return "Color Blindness Simulator" }
func (*tool) ShortTitle() string	{ return "Color Blindness Simulator" }
func (*tool) Category() string		{ return "graphic" }
func (*tool) Icon() string		{ return "󰈈" }
func (*tool) Description() string	{ return "Compare an image with three color vision simulations" }
func (*tool) AcceptsImages() bool	{ return true }
func (*tool) AcceptsText() bool		{ return false }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) compact() bool		{ return t.width < 40 || t.height < 10 }
func (*tool) actions() []string		{ return []string{"＋ Open image"} }
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		t.path.Width = max(1, t.width-2)
		return t, nil
	case toolkit.ImageImportMsg:
		if event.Image == nil {
			t.err = fmt.Errorf("invalid image")
			return t, nil
		}
		b := event.Image.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx() > 16_000_000/b.Dy() {
			t.err = fmt.Errorf("image exceeds 16 million pixels")
			return t, nil
		}
		t.img = event.Image
		t.name = safe(event.Name)
		t.revision++
		t.cachePayload = ""
		t.gridPayloads = nil
		t.editing = false
		t.path.Blur()
		t.err = nil
		t.status = ""
		return t, nil
	case exportDone:
		t.saving = false
		prefix := ""
		if event.revision != t.revision {
			prefix = "Previous image: "
		}
		if event.err != nil {
			t.err = fmt.Errorf("%s%w", prefix, event.err)
			t.status = ""
		} else {
			t.err = nil
			t.status = fmt.Sprintf("%sSaved %s · %s", prefix, safe(event.path), names[event.kind])
		}
		return t, nil
	case nativePrepared:
		t.preparing = false
		if event.revision != t.revision || event.kind != t.selected {
			if t.status == "Preparing native preview…" {
				t.status = ""
			}
			return t, nil
		}
		if event.err != nil {
			t.err = fmt.Errorf("could not prepare preview: %w", event.err)
			t.status = ""
			return t, nil
		}
		t.cacheRevision, t.cacheKind, t.cachePayload = event.revision, event.kind, event.payload
		t.status = ""
		if t.editing {
			return t, nil
		}
		return t, t.foregroundPreview(event.payload)
	case previewDone:
		t.previewPending = false
		if t.status == "Opening native preview…" {
			t.status = ""
		}
		t.cachePayload = ""
		if event.err != nil {
			t.err = event.err
		}
		return t, nil
	case gridPrepared:
		t.gridPreparing = false
		if event.revision != t.revision {
			if t.status == "Preparing comparison grid…" {
				t.status = ""
			}
			return t, nil
		}
		if event.err != nil {
			t.err = fmt.Errorf("could not prepare comparison grid: %w", event.err)
			t.status = ""
			return t, nil
		}
		t.gridRevision, t.gridPayloads = event.revision, event.payloads
		t.status = ""
		if t.editing {
			return t, nil
		}
		return t, t.foregroundGrid(event.payloads)
	case gridDone:
		t.gridPending = false
		if t.status == "Opening comparison grid…" {
			t.status = ""
		}
		if event.err != nil {
			t.err = event.err
		}
		return t, nil
	case tea.MouseMsg:
		if t.editing {
			return t, t.pathUpdate(event)
		}
		if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
			return t, nil
		}
		if event.Y == 0 {
			switch toolkit.ActionAt(event, t.actions()...) {
			case 0:
				return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
			}
			return t, nil
		}
		if t.compact() && event.Y == 1 {
			if i := toolkit.ActionAt(event, t.tabs()...); i >= 0 {
				t.selected = i
			}
			return t, nil
		}
		if !t.compact() && event.Y >= 1 && event.Y < t.height-1 {
			// 空态面板：整块可点击 = 打开文件（与 hash FILE 面板同语义）。
			if t.img == nil {
				return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
			}
			row := 0
			topHeight := max(1, (t.height-3)/2)
			if event.Y >= topHeight+2 {
				row = 1
			}
			col := 0
			leftWidth := max(1, (t.width-1)/2)
			if event.X >= leftWidth+1 {
				col = 1
			}
			t.selected = row*2 + col
		}
		return t, nil
	case tea.KeyMsg:
		if t.editing {
			return t, t.pathUpdate(event)
		}
		switch event.String() {
		case "1", "2", "3", "4":
			t.selected = int(event.String()[0] - '1')
			return t, nil
		case "tab":
			t.selected = (t.selected + 1) % 4
			return t, nil
		case "shift+tab":
			t.selected = (t.selected + 3) % 4
			return t, nil
		case "enter":
			return t, t.nativeGrid()
		case "alt+i":
			return t, t.native()
		case "ctrl+s":
			return t, t.editPath()
		}
	}
	if t.editing {
		return t, t.pathUpdate(msg)
	}
	return t, nil
}
func (t *tool) nativeGrid() tea.Cmd {
	if t.img == nil {
		t.err = fmt.Errorf("open an image first")
		return nil
	}
	if !imageview.NativeAvailable() {
		t.err = fmt.Errorf("native preview requires Ghostty or Kitty outside tmux")
		return nil
	}
	if t.gridPending || t.gridPreparing {
		return nil
	}
	t.err = nil
	if t.gridRevision == t.revision && len(t.gridPayloads) == 4 {
		return t.foregroundGrid(t.gridPayloads)
	}
	img, revision := t.img, t.revision
	t.gridPreparing = true
	t.status = "Preparing comparison grid…"
	return func() tea.Msg {
		payloads := make([]string, len(names))
		for i := range payloads {
			payload, err := imageview.PrepareNativeData(simulated{source: img, kind: i})
			if err != nil {
				return toolkit.BackgroundToolMsg{Tool: t, Message: gridPrepared{revision: revision, err: err}}
			}
			payloads[i] = payload
		}
		return toolkit.BackgroundToolMsg{Tool: t, Message: gridPrepared{revision: revision, payloads: payloads}}
	}
}
func (t *tool) foregroundGrid(payloads []string) tea.Cmd {
	cmd := imageview.NativeGridPrepared(payloads, names, t.width, t.height, func(err error) tea.Msg { return gridDone{err} })
	revision := t.revision
	t.gridPending = true
	t.status = "Opening comparison grid…"
	return func() tea.Msg {
		return toolkit.ForegroundCommandMsg{
			Tool:	t, Command: cmd,
			Valid:	func() bool { return t.revision == revision && !t.editing },
			OnDrop: func() {
				t.gridPending = false
				if t.status == "Opening comparison grid…" {
					t.status = ""
				}
			},
		}
	}
}
func (t *tool) tabs() []string {
	labels := []string{"1", "2", "3", "4"}
	labels[t.selected] = "*" + labels[t.selected]
	return labels
}
func (t *tool) native() tea.Cmd {
	if t.img == nil {
		t.err = fmt.Errorf("open an image first")
		return nil
	}
	if !imageview.NativeAvailable() {
		t.err = fmt.Errorf("native preview requires Ghostty or Kitty outside tmux")
		return nil
	}
	if t.previewPending {
		t.status = "Opening native preview…"
		return nil
	}
	if t.preparing {
		t.status = "Preparing native preview…"
		return nil
	}
	t.err = nil
	if t.cachePayload != "" && t.cacheRevision == t.revision && t.cacheKind == t.selected {
		return t.foregroundPreview(t.cachePayload)
	}
	img, kind, rev := t.img, t.selected, t.revision
	t.preparing = true
	t.status = "Preparing native preview…"
	return func() tea.Msg {
		payload, err := imageview.PrepareNativeData(simulated{source: img, kind: kind})
		return toolkit.BackgroundToolMsg{Tool: t, Message: nativePrepared{revision: rev, kind: kind, payload: payload, err: err}}
	}
}
func (t *tool) foregroundPreview(payload string) tea.Cmd {
	v := imageview.New(t.img, names[t.selected])
	v.SetSize(max(1, t.width), max(4, t.height))
	cmd := v.NativePrepared(payload, func(err error) tea.Msg { return previewDone{err} })
	rev, kind := t.revision, t.selected
	t.previewPending = true
	t.status = "Opening native preview…"
	return func() tea.Msg {
		return toolkit.ForegroundCommandMsg{
			Tool:	t, Command: cmd,
			Valid:	func() bool { return t.revision == rev && t.selected == kind && !t.editing },
			OnDrop: func() {
				t.previewPending = false
				if t.status == "Opening native preview…" {
					t.status = ""
				}
			},
		}
	}
}

func (t *tool) View() string {
	if t.editing {
		return t.pathView()
	}
	width, height := max(1, t.width), max(1, t.height)
	top := ansi.Truncate(toolkit.Actions(t.actions()...), width, "")
	footer := ""
	if t.err != nil {
		footer = "Error: " + safe(t.err.Error())
	} else if t.status != "" {
		footer = t.status
	} else if t.img != nil {
		footer = t.name + " · Enter native grid · Alt+I selected preview · 1–4 select · Ctrl+S save PNG"
	} else {
		// 空态页脚提示（ctrl+f 由 shell 处理），
		// 前导空格与 StatusBar 页脚语言一致。
		footer = " ctrl+f open file · tab focus"
	}
	footer = lipgloss.NewStyle().Foreground(theme.Muted).Render(ansi.Truncate(footer, width, ""))
	var body string
	if t.img == nil {
		// 空态按 hash FILE 面板模式给出带边框的
		// drop-zone 面板（标题行嵌 ＋ Open 按钮），占满内容区。
		body = t.emptyState(width, max(3, height-2))
	} else if t.compact() {
		h := max(3, height-3)
		body = ansi.Truncate(toolkit.Actions(t.tabs()...), width, "") + "\n" + t.card(t.selected, width, h)
	} else {
		avail := height - 2
		topH := max(1, (avail-1)/2)
		bottomH := max(1, avail-topH-1)
		left := max(1, (width-1)/2)
		right := max(1, width-left-1)
		firstRow := lipgloss.JoinHorizontal(lipgloss.Top, t.card(0, left, topH), " ", t.card(1, right, topH))
		secondRow := lipgloss.JoinHorizontal(lipgloss.Top, t.card(2, left, bottomH), " ", t.card(3, right, bottomH))
		body = lipgloss.JoinVertical(lipgloss.Left, firstRow, "", secondRow)
	}
	lines := strings.Split(top+"\n"+body+"\n"+footer, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

// emptyState 渲染无图时的空状态面板：圆角边框 + 标题行嵌
// ＋ Open 按钮 + 引导文案，结构与 hash 的 FILE 面板、imageconvert 的
// "Image conversion tasks" 空态一致。
func (t *tool) emptyState(width, height int) string {
	innerW, innerH := max(1, width-4), max(1, height-2)
	hint := "󰈈  No image yet"
	use := "Use ＋ Open to select an image to compare."
	lines := []string{hint, "", use, "", "ctrl+f open file  ·  enter opens the comparison grid"}
	content := lipgloss.NewStyle().Foreground(theme.Muted).Render(ansi.Truncate(hint, innerW, ""))
	rest := make([]string, 0, innerH-1)
	for _, line := range lines[1:] {
		rest = append(rest, theme.MutedStyle().Render(ansi.Truncate(line, innerW, "")))
	}
	content += "\n" + strings.Join(rest, "\n")
	pane := lipgloss.NewStyle().Width(innerW).Height(innerH).
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border).
		Padding(0, ui.Space1).
		Render(content)
	out := strings.Split(pane, "\n")
	w := lipgloss.Width(out[0])
	label := " IMAGE "
	out[0] = ansi.Cut(out[0], 0, 2) + theme.MutedStyle().Bold(true).Render(label) + ansi.Cut(out[0], 2+lipgloss.Width(label), w)
	if start := w - 1 - lipgloss.Width(toolkit.CompactActions(t.actions()...)); start >= 2+lipgloss.Width(label) {
		buttons := toolkit.CompactActions(t.actions()...)
		out[0] = ansi.Cut(out[0], 0, start) + buttons + ansi.Cut(out[0], start+lipgloss.Width(buttons), w)
	}
	return strings.Join(out, "\n")
}

func (t *tool) card(i, w, h int) string {
	innerW, innerH := max(1, w-2), max(1, h-2)
	label := names[i]
	if i == t.selected {
		label = "▸ " + label
	}
	content := lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render(ansi.Truncate(label, innerW, ""))
	if innerH > 1 {
		v := imageview.New(simulated{source: t.img, kind: i}, names[i])
		v.SetSize(innerW, innerH+1)
		lines := strings.Split(v.Render(), "\n")
		if len(lines) > 2 {
			content += "\n" + strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	border := theme.Border
	if i == t.selected {
		border = theme.Accent
	}
	return lipgloss.NewStyle().Width(innerW).Height(innerH).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(content)
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}
