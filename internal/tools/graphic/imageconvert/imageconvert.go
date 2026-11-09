// Package imageconvert converts selected images to PNG, JPEG, or GIF files.
package imageconvert

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/HugoSmits86/nativewebp"
	"github.com/spakin/netpbm"
	"github.com/woozymasta/tga"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/imageview"
)

type tool struct {
	ws			toolkit.Workspace
	header			toolkit.SettingHeader
	bar, batchBar		toolkit.ActionHeader
	headerHeight, barHeight	int
	batchBarHeight		int
	items			[]imageItem
	selected		int
	nextID			uint64
	img			image.Image
	name			string
	format			int
	status			string
	width, height		int
	saving			bool
	pathPrompt		bool
	promptAll		bool
}

var formats = []string{"PNG", "JPEG", "GIF", "BMP", "TIFF", "WebP", "PBM", "TGA"}
var formatOptions = []int{0, 1, 6, 3, 7, 4, 5}

const jpegQuality = 75
const imageConverterSectionGap = 1

type imageItem struct {
	id			uint64
	img			image.Image
	name, path, status	string
	dataSize		int
}
type batchResult struct {
	id	uint64
	result	saveDone
}
type batchDone struct{ results []batchResult }
type previewDone struct{ err error }
type saveDone struct {
	path	string
	size	int
	err	error
	id	uint64
}

func newTool() *tool {
	t := &tool{
		ws:	toolkit.NewWorkspace("Enter a save path…", "Add images to start a conversion."),
		header: toolkit.SettingHeader{
			Names:		[]string{"Conversion"},
			Descriptions:	[]string{"Select the image format to convert into"},
			Icons:		[]string{"󰦝"},
			Options:	[][]string{formatLabels()},
		},
		selected:	-1, width: 80, height: 24,
	}
	t.ws.SetStacked(true)
	t.ws.SetStackedInputHeight(1)
	t.ws.SetPaneTitles("Save as", "Image conversion tasks")
	t.ws.SetOutputControls("Preview")
	t.ws.SetInputControls("Cancel", "Save")
	t.ws.SetOutputOnly(true)
	t.resize()
	return t
}
func init()			{ toolkit.Register(newTool()) }
func (*tool) ID() string	{ return "graphic.imageconvert" }
func (*tool) Title() string	{ return "Image Converter" }
func (*tool) Category() string	{ return "graphic" }
func (*tool) Icon() string	{ return "󰋩" }
func (*tool) Description() string {
	return "Convert BMP, JPEG, PBM, PNG, TGA, TIFF and WebP images"
}
func (*tool) AcceptsImages() bool	{ return true }

// 保存路径输入框仅在开始保存时出现，平时页面没有常驻文本输入区，
// 与 graphic.colorblind 一样声明为无文本输入工具。
func (*tool) AcceptsText() bool		{ return false }
func (*tool) AcceptsImageBatch() bool	{ return true }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) HandlesEscape() bool	{ return t.pathPrompt || t.header.MenuOpen() }

// setFormat 切换输出格式，并把仍使用默认命名的路径同步到新扩展名。
func (t *tool) setFormat(format int) {
	t.storePath()
	oldFormat := t.format
	t.format = format
	for i := range t.items {
		t.items[i].status = ""
		if t.items[i].path == suggestedPathFor(t.items[i].name, oldFormat) {
			t.items[i].path = suggestedPathFor(t.items[i].name, t.format)
		}
	}
	if t.selected >= 0 {
		t.ws.Input.SetValue(t.items[t.selected].path)
	}
	t.status = ""
	t.render()
}

func (t *tool) actions() []string {
	return []string{formats[t.format]}
}
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case toolkit.WorkspaceActionMsg:
		switch event.Action {
		case "Preview":
			return t, t.preview()
		case "Input:Cancel":
			t.cancelPathPrompt()
			return t, nil
		case "Input:Save":
			return t, t.commitPathPrompt()
		}
		return t, nil
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		cmd := t.resize()
		t.render()
		return t, cmd
	case toolkit.ImageImportMsg:
		t.addImages([]toolkit.ImageImportMsg{event})
		return t, nil
	case toolkit.ImageBatchImportMsg:
		t.addImages(event.Images)
		return t, nil
	case batchDone:
		t.saving = false
		success := 0
		var failed []string
		for _, r := range event.results {
			for i := range t.items {
				if t.items[i].id == r.id {
					if r.result.err != nil {
						t.items[i].status = "Failed: " + r.result.err.Error()
					} else {
						t.items[i].status = "Saved " + safeText(r.result.path)
					}
					break
				}
			}
			if r.result.err != nil {
				failed = append(failed, filepath.Base(r.result.path)+": "+r.result.err.Error())
			} else {
				success++
			}
		}
		t.status = fmt.Sprintf("Saved %d of %d images", success, len(event.results))
		if len(failed) > 0 {
			t.ws.SetError(fmt.Errorf("%s; %s", t.status, strings.Join(failed, "; ")))
		} else {
			t.ws.SetError(nil)
		}
		t.render()
		return t, nil
	case saveDone:
		t.saving = false
		for i := range t.items {
			if t.items[i].id == event.id {
				if event.err != nil {
					t.items[i].status = "Failed: " + event.err.Error()
				} else {
					t.items[i].status = "Saved " + safeText(event.path)
				}
				break
			}
		}
		if event.err != nil {
			t.status = ""
			t.ws.SetError(event.err)
			t.render()
		} else {
			t.ws.SetError(nil)
			t.status = fmt.Sprintf("Saved %s (%d bytes)", safeText(event.path), event.size)
			t.render()
		}
		return t, nil
	case previewDone:
		if event.err != nil {
			t.ws.SetError(event.err)
		}
		return t, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if t.pathPrompt {
			return t, t.ws.UpdateBelow(msg, t.workspaceTop())
		}
		if cmd, handled := t.itemAction(mouse); handled {
			return t, cmd
		}
	}
	barStart := t.headerHeight + imageConverterSectionGap
	if mouse, ok := msg.(tea.MouseMsg); ok && t.barHeight > 0 && mouse.Y >= barStart && mouse.Y < barStart+t.barHeight {
		mouse.Y -= barStart
		if t.bar.Index(mouse, t.barActions()...) == 0 {
			return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		}
		return t, nil
	}
	batchStart := barStart + t.barHeight + imageConverterSectionGap
	if mouse, ok := msg.(tea.MouseMsg); ok && t.batchBarHeight > 0 && mouse.Y >= batchStart && mouse.Y < batchStart+t.batchBarHeight {
		if len(t.items) == 0 {
			return t, nil
		}
		mouse.Y -= batchStart
		mouse.X -= t.batchBarRowOffset(mouse.Y)
		switch t.batchBar.Index(mouse, t.batchActions()...) {
		case 0:
			t.beginPathPrompt(true)
			return t, nil
		case 1:
			t.clearItems()
		}
		return t, nil
	}
	setting, choice, handled := t.header.Pick(msg, t.actions()...)
	if handled && setting < 0 {
		// 格式下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if t.pathPrompt {
			switch key.String() {
			case "esc":
				t.cancelPathPrompt()
				return t, nil
			case "enter", "ctrl+s":
				return t, t.commitPathPrompt()
			}
			return t, t.ws.UpdateBelow(msg, t.workspaceTop())
		}
		switch key.String() {
		case "ctrl+t":
			setting, choice, handled = t.header.Shortcut(0, t.actions()...)
		case "alt+i":
			return t, t.preview()
		case "ctrl+s":
			t.beginPathPrompt(false)
			return t, nil
		case "alt+[":
			t.selectItem(t.selected - 1)
			return t, nil
		case "alt+]":
			t.selectItem(t.selected + 1)
			return t, nil
		case "alt+d":
			t.removeSelected()
			return t, nil
		case "alt+x":
			t.clearItems()
			return t, nil
		case "alt+s":
			t.beginPathPrompt(true)
			return t, nil
		}
	}
	if handled && setting < 0 {
		// ctrl+t 只是打开了格式下拉框，等待键盘或鼠标选择。
		return t, nil
	}
	if handled && setting == 0 {
		if choice >= 0 {
			t.setFormat(formatOptions[choice])
		} else {
			// 紧凑布局的回退按钮条与 ctrl+t：轮换输出格式。
			t.setFormat(nextFormat(t.format))
		}
		return t, nil
	}
	cmd := t.ws.UpdateBelow(msg, t.workspaceTop())
	return t, cmd
}

// View 渲染设置条与工作区；格式下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.body())
}

func (t *tool) body() string {
	prefix := t.header.View(t.actions()...) + "\n\n"
	if t.barHeight > 0 {
		prefix += t.bar.View(t.barActions()...) + "\n\n"
	}
	if t.batchBarHeight > 0 {
		prefix += t.batchBarView() + "\n\n"
	}
	return prefix + t.ws.View()
}

func (t *tool) workspaceTop() int {
	top := t.headerHeight + imageConverterSectionGap
	if t.barHeight > 0 {
		top += t.barHeight + imageConverterSectionGap
	}
	if t.batchBarHeight > 0 {
		top += t.batchBarHeight + imageConverterSectionGap
	}
	return top
}

func (t *tool) beginPathPrompt(all bool) {
	if t.saving {
		return
	}
	if len(t.items) == 0 {
		t.ws.SetError(fmt.Errorf("open at least one image first"))
		return
	}
	if !all && (t.selected < 0 || t.selected >= len(t.items)) {
		t.selectItem(0)
	}
	t.storePath()
	t.pathPrompt, t.promptAll = true, all
	if all {
		t.ws.Input.SetValue(".")
		t.ws.SetPaneTitles("Destination folder", "Image conversion tasks")
	} else {
		t.ws.Input.SetValue(t.items[t.selected].path)
		t.ws.SetPaneTitles("Save as", "Image conversion tasks")
	}
	t.ws.SetOutputOnly(false)
	t.resize()
}

func (t *tool) cancelPathPrompt() {
	t.pathPrompt, t.promptAll = false, false
	t.restoreSelectedPath()
	t.ws.SetOutputOnly(true)
	t.resize()
}

func (t *tool) commitPathPrompt() tea.Cmd {
	if strings.TrimSpace(t.ws.Input.Value()) == "" {
		t.ws.SetError(fmt.Errorf("enter an output path or folder"))
		return nil
	}
	var cmd tea.Cmd
	if t.promptAll {
		cmd = t.saveAll()
	} else {
		cmd = t.save()
	}
	if cmd != nil {
		t.pathPrompt, t.promptAll = false, false
		t.restoreSelectedPath()
		t.ws.SetOutputOnly(true)
		t.resize()
	}
	return cmd
}

func (t *tool) restoreSelectedPath() {
	if t.selected >= 0 && t.selected < len(t.items) {
		t.ws.Input.SetValue(t.items[t.selected].path)
	}
}

func (t *tool) preview() tea.Cmd {
	if t.img == nil {
		t.ws.SetError(fmt.Errorf("add an image first"))
		return nil
	}
	if !imageview.NativeAvailable() {
		t.ws.SetError(fmt.Errorf("native preview requires Ghostty or Kitty outside tmux"))
		return nil
	}
	preview := imageview.New(t.img, t.name)
	preview.SetSize(max(1, t.width), max(4, t.height))
	return preview.Native(func(err error) tea.Msg { return previewDone{err} })
}
func (t *tool) render() {
	if t.img == nil {
		t.ws.SetOutput("󰋩  No images yet\n\nUse Add to select one or more image files.")
		return
	}
	info := fmt.Sprintf("󰋩  Images  ·  %d files", len(t.items))
	if t.status != "" {
		info += "  ·  " + safeText(t.status)
	}
	info += "\n\n"
	rowActions := toolkit.CompactActions("󰆓", "󰆴")
	rowActionWidth := lipgloss.Width(rowActions)
	rowWidth := max(1, t.ws.Output.Width-rowActionWidth-1)
	for i, item := range t.items {
		mark := "  "
		if i == t.selected {
			mark = "󰄾 "
		} else {
			mark = "󰄱 "
		}
		title := fmt.Sprintf("%s󰈔  %s", mark, safeText(item.name))
		description := "   " + humanSize(item.dataSize)
		if item.status != "" {
			description += "  ·  " + safeText(item.status)
		}
		title = ansi.Truncate(title, rowWidth, "…")
		info += title + strings.Repeat(" ", max(1, t.ws.Output.Width-lipgloss.Width(title)-rowActionWidth)) + rowActions + "\n"
		info += ansi.Truncate(description, max(1, t.ws.Output.Width-2), "…") + "\n"
	}
	info += "\nCtrl+S save  ·  Alt+D remove  ·  Alt+I preview"
	t.ws.SetOutput(info)
}

func humanSize(size int) string {
	units := [...]string{"B", "KB", "MB", "GB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", size, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
func suggestedPathFor(name string, format int) string {
	ext := strings.ToLower(formats[format])
	if ext == "jpeg" {
		ext = "jpg"
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if base == "" || base == "." {
		base = "image"
	}
	return base + "-converted." + ext
}

// formatLabels 返回可输出格式的下拉选项（顺序与 formatOptions 一致，GIF 不可编码故不列出）。
func formatLabels() []string {
	labels := make([]string, 0, len(formatOptions))
	for _, format := range formatOptions {
		labels = append(labels, formats[format])
	}
	return labels
}

func nextFormat(format int) int {
	for i, option := range formatOptions {
		if option == format {
			return formatOptions[(i+1)%len(formatOptions)]
		}
	}
	return formatOptions[0]
}

func (t *tool) save() tea.Cmd {
	if t.saving {
		return nil
	}
	if t.img == nil {
		t.ws.SetError(fmt.Errorf("open an image first"))
		return nil
	}
	path := strings.TrimSpace(t.ws.Input.Value())
	if path == "" {
		t.ws.SetError(fmt.Errorf("enter an output path"))
		return nil
	}
	t.storePath()
	img, format := t.img, t.format
	id := t.items[t.selected].id
	t.saving = true
	t.status = "Saving…"
	t.ws.SetError(nil)
	t.render()
	return func() tea.Msg {
		result := saveImage(path, img, format, jpegQuality)
		result.id = id
		return toolkit.BackgroundToolMsg{Tool: t, Message: result}
	}
}
func saveImage(path string, img image.Image, format, quality int) saveDone {
	result := saveDone{path: path}
	var data bytes.Buffer
	if err := encode(&data, img, format, quality); err != nil {
		result.err = fmt.Errorf("could not encode image: %w", err)
		return result
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		result.err = fmt.Errorf("could not create output: %w", err)
		return result
	}
	n, writeErr := f.Write(data.Bytes())
	if writeErr == nil && n != data.Len() {
		writeErr = io.ErrShortWrite
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr == nil {
			writeErr = closeErr
		}
		result.err = fmt.Errorf("could not write output: %w", writeErr)
		return result
	}
	result.size = data.Len()
	return result
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}

func encode(dst *bytes.Buffer, img image.Image, format, quality int) error {
	switch format {
	case 0:
		return png.Encode(dst, img)
	case 1:
		// JPEG has no alpha channel; composite over white like a typical image editor.
		b := img.Bounds()
		opaque := image.NewRGBA(b)
		draw.Draw(opaque, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(opaque, b, img, b.Min, draw.Over)
		return jpeg.Encode(dst, opaque, &jpeg.Options{Quality: quality})
	case 2:
		return gif.Encode(dst, img, nil)
	case 3:
		return bmp.Encode(dst, img)
	case 4:
		return tiff.Encode(dst, img, nil)
	case 5:
		b := img.Bounds()
		if b.Dx() > 4_000_000/max(1, b.Dy()) {
			return fmt.Errorf("WebP encoding is limited to 4 million pixels")
		}
		return nativewebp.Encode(dst, img, nil)
	case 6:
		return netpbm.Encode(dst, img, &netpbm.EncodeOptions{Format: netpbm.PBM})
	case 7:
		return tga.Encode(dst, img)
	default:
		return fmt.Errorf("unknown output format")
	}
}
