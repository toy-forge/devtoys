package imageconvert

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/woozymasta/tga"
	_ "golang.org/x/image/webp"
)

func TestEncodeFormatsAndJPEGTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 0})
	for i, name := range []string{"png", "jpeg", "gif", "bmp", "tiff", "webp", "pbm", "tga"} {
		var buf bytes.Buffer
		if err := encode(&buf, img, i, 90); err != nil {
			t.Fatal(err)
		}
		var got image.Image
		var format string
		var err error
		if name == "tga" {
			got, err = tga.Decode(bytes.NewReader(buf.Bytes()))
			format = "tga"
		} else {
			got, format, err = image.Decode(bytes.NewReader(buf.Bytes()))
		}
		if err != nil || format != name || got.Bounds().Dx() != 2 {
			t.Fatalf("format=%s err=%v", format, err)
		}
		if name == "jpeg" {
			r, g, b, _ := got.At(0, 0).RGBA()
			if r < 60000 || g < 60000 || b < 60000 {
				t.Fatalf("transparent pixel is not white: %d %d %d", r, g, b)
			}
		}
	}
}
func TestSaveDoesNotOverwrite(t *testing.T) {
	t1 := newTool()
	t1.Update(toolkit.ImageImportMsg{Image: image.NewRGBA(image.Rect(0, 0, 2, 2)), Name: "x.png"})
	path := filepath.Join(t.TempDir(), "image.png")
	t1.ws.Input.SetValue(path)
	cmd := t1.save()
	if cmd == nil {
		t.Fatal("save command missing")
	}
	event := cmd().(toolkit.BackgroundToolMsg)
	t1.Update(event.Message)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd = t1.save()
	if cmd == nil {
		t.Fatal("second save command missing")
	}
	event = cmd().(toolkit.BackgroundToolMsg)
	t1.Update(event.Message)
	if t1.ws.Err == nil {
		t.Fatal("overwrite should fail")
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("existing file changed")
	}
}

func TestFormatChangeUpdatesOnlySuggestedPath(t *testing.T) {
	tool := newTool()
	tool.Update(toolkit.ImageImportMsg{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Name: "x.png"})
	chooseFormat(tool, 1)	// PNG → JPEG
	if got := tool.ws.Input.Value(); got != "x-converted.jpg" {
		t.Fatalf("suggested path=%q", got)
	}
	tool.ws.Input.SetValue("my-choice.dat")
	chooseFormat(tool, 1)
	if got := tool.ws.Input.Value(); got != "my-choice.dat" {
		t.Fatalf("custom path changed: %q", got)
	}
}

func TestNewImageClearsPriorErrorAndFailedSaveClearsProgress(t *testing.T) {
	tool := newTool()
	tool.ws.SetError(fmt.Errorf("previous error"))
	tool.Update(toolkit.ImageImportMsg{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Name: "new.png"})
	if tool.ws.Err != nil {
		t.Fatal("old error remained after loading an image")
	}
	tool.ws.Input.SetValue(filepath.Join(t.TempDir(), "missing", "file.png"))
	cmd := tool.save()
	event := cmd().(toolkit.BackgroundToolMsg)
	tool.Update(event.Message)
	if tool.ws.Err == nil || tool.saving || strings.Contains(tool.status, "Saving") {
		t.Fatalf("err=%v saving=%v status=%q", tool.ws.Err, tool.saving, tool.status)
	}
}

func TestQueueSelectionPathsAndRemoval(t *testing.T) {
	tool := newTool()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tool.Update(toolkit.ImageBatchImportMsg{Images: []toolkit.ImageImportMsg{{Image: img, Name: "one.png"}, {Image: img, Name: "two.png"}}})
	if len(tool.items) != 2 || tool.name != "one.png" || tool.selected != 0 {
		t.Fatalf("queue=%d selected=%d name=%s", len(tool.items), tool.selected, tool.name)
	}
	tool.ws.Input.SetValue("custom-one.png")
	tool.selectItem(1)
	if tool.name != "two.png" || tool.ws.Input.Value() != "two-converted.png" {
		t.Fatalf("second: %s %s", tool.name, tool.ws.Input.Value())
	}
	chooseFormat(tool, 1)
	if tool.ws.Input.Value() != "two-converted.jpg" {
		t.Fatalf("format path %q", tool.ws.Input.Value())
	}
	tool.selectItem(0)
	if tool.ws.Input.Value() != "custom-one.png" {
		t.Fatalf("lost custom path: %q", tool.ws.Input.Value())
	}
	tool.removeSelected()
	if len(tool.items) != 1 || tool.name != "two.png" || tool.ws.Input.Value() != "two-converted.jpg" {
		t.Fatalf("remove queue=%+v path=%q", tool.items, tool.ws.Input.Value())
	}
	tool.clearItems()
	if len(tool.items) != 0 || tool.img != nil || tool.selected != -1 {
		t.Fatal("queue not cleared")
	}
}
func TestBatchSavePartialFailureAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	tool := newTool()
	tool.Update(toolkit.ImageBatchImportMsg{Images: []toolkit.ImageImportMsg{{Image: img, Name: "one.jpg"}, {Image: img, Name: "two.jpg"}}})
	existing := filepath.Join(dir, "one.png")
	if err := os.WriteFile(existing, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	tool.ws.Input.SetValue(dir)
	cmd := tool.saveAll()
	if cmd == nil {
		t.Fatal("missing batch save")
	}
	tool.removeSelected()
	if len(tool.items) != 2 {
		t.Fatal("queue changed during save")
	}
	event := cmd().(toolkit.BackgroundToolMsg)
	tool.Update(event.Message)
	if tool.saving || tool.ws.Err == nil || !strings.Contains(tool.status, "1 of 2") {
		t.Fatalf("batch result: %q %v", tool.status, tool.ws.Err)
	}
	if data, err := os.ReadFile(existing); err != nil || string(data) != "existing" {
		t.Fatal("existing file changed")
	}
	f, err := os.Open(filepath.Join(dir, "two.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, _, err = image.Decode(f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tool.items[0].status, "Failed") || !strings.Contains(tool.items[1].status, "Saved") {
		t.Fatalf("item statuses: %+v", tool.items)
	}
}
func TestBatchImportIsAtomicAtQueueLimit(t *testing.T) {
	tool := newTool()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	entries := make([]toolkit.ImageImportMsg, maxQueueImages+1)
	for i := range entries {
		entries[i] = toolkit.ImageImportMsg{Image: img, Name: fmt.Sprintf("%d.png", i)}
	}
	tool.Update(toolkit.ImageBatchImportMsg{Images: entries})
	if len(tool.items) != 0 || tool.ws.Err == nil {
		t.Fatal("oversized batch was partially added")
	}
}

// chooseFormat 用键盘完成格式下拉框的选择：ctrl+t 打开列表，↓ 移动 steps 次，enter 确认。
func chooseFormat(tool *tool, steps int) {
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	for range steps {
		tool.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyEnter})
}
