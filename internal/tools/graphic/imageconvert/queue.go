package imageconvert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

const maxQueueImages = 16
const maxQueuePixels = 64_000_000

func (t *tool) barActions() []string {
	return []string{"＋ Add images"}
}
func (t *tool) batchActions() []string	{ return []string{"Save all", "Delete all"} }
func (t *tool) batchBarView() string {
	view := t.batchBar.View(t.batchActions()...)
	if len(t.items) == 0 {
		// 禁用态用 DisabledCompactActions 胶囊（命中宽度与可用态一致）。
		view = toolkit.DisabledCompactActions(t.batchActions()...)
	}
	return lipgloss.NewStyle().Width(max(1, t.width)).Align(lipgloss.Right).Render(view)
}
func (t *tool) batchBarRowOffset(row int) int {
	lines := strings.Split(t.batchBar.View(t.batchActions()...), "\n")
	if row < 0 || row >= len(lines) {
		return 0
	}
	return max(0, t.width-lipgloss.Width(lines[row]))
}
func (t *tool) itemAction(mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft || len(t.items) == 0 {
		return nil, false
	}
	localY := mouse.Y - t.workspaceTop()
	itemRow := localY + t.ws.Output.YOffset - 3
	if itemRow < 0 || itemRow%2 != 0 {
		return nil, false
	}
	itemIndex := itemRow / 2
	if itemIndex >= len(t.items) {
		return nil, false
	}
	local := mouse
	local.Y = 0
	local.X -= 1 + max(0, t.ws.Output.Width-lipgloss.Width(toolkit.CompactActions("󰆓", "󰆴")))
	switch toolkit.CompactActionAt(local, "󰆓", "󰆴") {
	case 0:
		t.selectItem(itemIndex)
		t.beginPathPrompt(false)
		return nil, true
	case 1:
		t.selectItem(itemIndex)
		t.removeSelected()
		return nil, true
	default:
		return nil, false
	}
}
func (t *tool) resize() tea.Cmd {
	size := tea.WindowSizeMsg{Width: max(1, t.width), Height: max(1, t.height)}
	t.headerHeight = t.header.SetSize(size, t.actions()...)
	t.barHeight = 0
	t.batchBarHeight = 0
	if t.height > 10 {
		t.barHeight = t.bar.SetSize(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-t.headerHeight-imageConverterSectionGap)}, t.barActions()...)
		t.batchBarHeight = t.batchBar.SetSize(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-t.headerHeight-t.barHeight-2*imageConverterSectionGap)}, t.batchActions()...)
	}
	gaps := imageConverterSectionGap
	if t.barHeight > 0 {
		gaps += imageConverterSectionGap
	}
	if t.batchBarHeight > 0 {
		gaps += imageConverterSectionGap
	}
	return t.ws.Update(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-t.headerHeight-t.barHeight-t.batchBarHeight-gaps)})
}
func (t *tool) storePath() {
	if t.selected >= 0 && t.selected < len(t.items) {
		t.items[t.selected].path = t.ws.Input.Value()
	}
}
func (t *tool) addImages(images []toolkit.ImageImportMsg) {
	if len(images) == 0 {
		return
	}
	if len(t.items)+len(images) > maxQueueImages {
		t.ws.SetError(fmt.Errorf("queue is limited to %d images", maxQueueImages))
		return
	}
	pixels := 0
	for _, item := range t.items {
		b := item.img.Bounds()
		pixels += b.Dx() * b.Dy()
	}
	for _, entry := range images {
		if entry.Image == nil {
			t.ws.SetError(fmt.Errorf("invalid image"))
			return
		}
		b := entry.Image.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx() > maxQueuePixels/b.Dy() {
			t.ws.SetError(fmt.Errorf("invalid image dimensions"))
			return
		}
		pixels += b.Dx() * b.Dy()
		if pixels > maxQueuePixels {
			t.ws.SetError(fmt.Errorf("queue is limited to 64 million pixels"))
			return
		}
	}
	t.storePath()
	first := len(t.items)
	for _, entry := range images {
		t.nextID++
		name := filepath.Base(safeText(entry.Name))
		if name == "." || name == "" {
			name = "image"
		}
		t.items = append(t.items, imageItem{id: t.nextID, img: entry.Image, name: name, path: suggestedPathFor(name, t.format), dataSize: len(entry.Data)})
	}
	t.selectItem(first)
}
func (t *tool) selectItem(index int) {
	if len(t.items) == 0 {
		return
	}
	t.storePath()
	t.selected = max(0, min(index, len(t.items)-1))
	item := t.items[t.selected]
	t.img, t.name, t.status = item.img, item.name, item.status
	t.ws.Input.SetValue(item.path)
	t.ws.SetError(nil)
	t.render()
}
func (t *tool) removeSelected() {
	if t.saving {
		t.ws.SetError(fmt.Errorf("wait for saving to finish"))
		return
	}
	if t.selected < 0 || t.selected >= len(t.items) {
		return
	}
	t.items = append(t.items[:t.selected], t.items[t.selected+1:]...)
	if len(t.items) == 0 {
		t.resetQueue()
		return
	}
	// Avoid storing the removed item's path on the item that moved into its slot.
	index := min(t.selected, len(t.items)-1)
	t.selected = -1
	t.selectItem(index)
}
func (t *tool) clearItems() {
	if t.saving {
		t.ws.SetError(fmt.Errorf("wait for saving to finish"))
		return
	}
	t.items = nil
	t.resetQueue()
}
func (t *tool) resetQueue() {
	t.selected = -1
	t.img = nil
	t.name = ""
	t.status = ""
	t.ws.Input.SetValue("")
	t.ws.SetError(nil)
	t.render()
}
func (t *tool) saveAll() tea.Cmd {
	if t.saving {
		return nil
	}
	if len(t.items) == 0 {
		t.ws.SetError(fmt.Errorf("open at least one image first"))
		return nil
	}
	if !t.pathPrompt {
		t.storePath()
	}
	path := strings.TrimSpace(t.ws.Input.Value())
	if path == "" {
		t.ws.SetError(fmt.Errorf("enter an output path or folder"))
		return nil
	}
	dir := filepath.Dir(path)
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		dir = path
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.ws.SetError(fmt.Errorf("output folder does not exist"))
		return nil
	}
	items := append([]imageItem(nil), t.items...)
	format := t.format
	t.saving = true
	t.status = "Saving all…"
	t.ws.SetError(nil)
	t.render()
	return func() tea.Msg {
		results := make([]batchResult, 0, len(items))
		for _, item := range items {
			base := strings.TrimSuffix(filepath.Base(item.name), filepath.Ext(item.name))
			if base == "" || base == "." {
				base = "image"
			}
			ext := strings.ToLower(formats[format])
			if ext == "jpeg" {
				ext = "jpg"
			}
			result := saveImage(filepath.Join(dir, base+"."+ext), item.img, format, jpegQuality)
			results = append(results, batchResult{id: item.id, result: result})
		}
		return toolkit.BackgroundToolMsg{Tool: t, Message: batchDone{results: results}}
	}
}
