package colorblind

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestSimulationPreservesAlphaAndChangesColors(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 123})
	for i := 1; i < 4; i++ {
		got := simulated{source: img, kind: i}.At(0, 0).(color.NRGBA)
		if got.A != 123 || got.R == 255 && got.G == 0 && got.B == 0 {
			t.Fatalf("mode %s: %+v", names[i], got)
		}
	}
	if got := (simulated{source: img, kind: 0}).At(0, 0).(color.NRGBA); got != (color.NRGBA{R: 255, A: 123}) {
		t.Fatalf("original %+v", got)
	}
}
func TestGridAndCompactLayout(t *testing.T) {
	m := newTool()
	m.Update(toolkit.ImageImportMsg{Image: image.NewRGBA(image.Rect(0, 0, 4, 4)), Name: "test.png"})
	for _, sz := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 20, Height: 7}} {
		m.Update(sz)
		view := m.View()
		if lipgloss.Width(view) > sz.Width || lipgloss.Height(view) != sz.Height {
			t.Fatalf("size=%+v view=%dx%d", sz, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.selected != 2 || !strings.Contains(m.View(), "Tritanopia") {
		t.Fatal("mode did not change")
	}
}
func TestOpenButtonRequestsPicker(t *testing.T) {
	m := newTool()
	_, cmd := m.Update(tea.MouseMsg{X: 2, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd == nil {
		t.Fatal("missing open command")
	}
	if _, ok := cmd().(toolkit.OpenFileRequestMsg); !ok {
		t.Fatal("unexpected command")
	}
}

func TestBrettelReferenceRed(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	want := []color.NRGBA{{R: 255, A: 255}, {R: 107, G: 92, B: 12, A: 255}, {R: 255, G: 0, B: 77, A: 255}, {R: 164, G: 139, B: 0, A: 255}}
	for i, w := range want {
		if got := (simulated{source: img, kind: i}).At(0, 0).(color.NRGBA); got != w {
			t.Errorf("%s=%+v want %+v", names[i], got, w)
		}
	}
}

func TestExportSelectedSimulationAndNoOverwrite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 123})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	m := newTool()
	m.Update(toolkit.ImageImportMsg{Image: img, Name: "sample.jpg"})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.editing || m.path.Value() != "sample-protanopia.png" {
		t.Fatalf("save dialog: %v %q", m.editing, m.path.Value())
	}
	path := filepath.Join(t.TempDir(), "result.png")
	m.path.SetValue(path)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.saving || m.editing {
		t.Fatal("export not started")
	}
	msg := cmd().(toolkit.BackgroundToolMsg)
	m.Update(msg.Message)
	if m.err != nil || m.saving || !strings.Contains(m.status, "Saved") {
		t.Fatalf("save status %q %v", m.status, m.err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := simulated{source: img, kind: 1}.At(0, 0).(color.NRGBA)
	if pixel := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA); pixel != want {
		t.Fatalf("pixel=%+v want=%+v", pixel, want)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.path.Value() != path {
		t.Fatal("output path not retained")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd().(toolkit.BackgroundToolMsg).Message)
	if m.err == nil {
		t.Fatal("overwriting existing output was allowed")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing image changed")
	}
}
func TestExportFailureCleansPartialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.png")
	if err := writePNG(path, image.NewRGBA(image.Rect(0, 0, 0, 0))); err == nil {
		t.Fatal("empty image unexpectedly encoded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial output remained: %v", err)
	}
}
func TestPreviewPreparationAndStaleRequests(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TMUX", "")
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	m := newTool()
	m.Update(toolkit.ImageImportMsg{Image: img, Name: "test.png"})
	m.selected = 1
	cmd := m.native()
	if cmd == nil || !m.preparing {
		t.Fatal("preview did not prepare asynchronously")
	}
	message := cmd().(toolkit.BackgroundToolMsg)
	prepared := message.Message.(nativePrepared)
	pngData, err := base64.StdEncoding.DecodeString(prepared.payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got != (simulated{source: img, kind: 1}.At(0, 0).(color.NRGBA)) {
		t.Fatalf("preview pixel=%+v", got)
	}
	_, next := m.Update(prepared)
	if next == nil || !m.previewPending {
		t.Fatal("preview request missing")
	}
	fg := next().(toolkit.ForegroundCommandMsg)
	if !fg.Valid() {
		t.Fatal("valid preview request rejected")
	}
	m.selected = 2
	if fg.Valid() {
		t.Fatal("stale preview request still valid")
	}
	fg.OnDrop()
	if m.previewPending {
		t.Fatal("dropped preview remained pending")
	}
	m.selected = 1
	cmd = m.native()
	if cmd == nil {
		t.Fatal("cached preview cannot be reopened")
	}
	fg = cmd().(toolkit.ForegroundCommandMsg)
	fg.OnDrop()
	m.cachePayload = ""
	cmd = m.native()
	message = cmd().(toolkit.BackgroundToolMsg)
	m.Update(toolkit.ImageImportMsg{Image: img, Name: "new.png"})
	_, next = m.Update(message.Message)
	if next != nil || m.preparing || m.cachePayload != "" {
		t.Fatal("preview for old image was not discarded")
	}
}
func TestSaveDialogFitsSmallScreenAndCancels(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 20, Height: 7}} {
		m := newTool()
		m.Update(size)
		m.Update(toolkit.ImageImportMsg{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Name: "x.png"})
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		if w, h := lipgloss.Width(m.View()), lipgloss.Height(m.View()); w > size.Width || h != size.Height {
			t.Fatalf("%v dialog %dx%d", size, w, h)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
		if m.editing {
			t.Fatal("dialog did not cancel")
		}
	}
}

func TestSaveCompletionAfterChangingImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	m := newTool()
	m.Update(toolkit.ImageImportMsg{Image: img, Name: "old.png"})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.path.SetValue(filepath.Join(t.TempDir(), "old-result.png"))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(toolkit.ImageImportMsg{Image: img, Name: "new.png"})
	m.Update(cmd().(toolkit.BackgroundToolMsg).Message)
	if m.saving || m.err != nil || !strings.Contains(m.status, "Previous image:") {
		t.Fatalf("completion status %q %v", m.status, m.err)
	}
}
