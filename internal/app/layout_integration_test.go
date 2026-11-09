package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/filepicker"
	_ "github.com/toy-forge/devtoys/internal/tools"
)

func TestAllToolsFitScreen(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 60, Height: 16}} {
		for _, tool := range toolkit.All() {
			t.Run(tool.ID(), func(t *testing.T) {
				m := New()
				m.mode, m.active = modeTool, tool
				next, _ := m.Update(size)
				view := next.(Model).View()
				if lipgloss.Height(view) != size.Height {
					t.Fatalf("height=%d, want %d", lipgloss.Height(view), size.Height)
				}
				for _, line := range strings.Split(view, "\n") {
					if lipgloss.Width(line) > size.Width {
						t.Fatalf("width=%d, want <=%d", lipgloss.Width(line), size.Width)
					}
				}
			})
		}
	}
}

func TestFileImportRecomputesTool(t *testing.T) {
	tool, ok := toolkit.Get("converters.jsonyaml")
	if !ok {
		t.Fatal("JSON/YAML tool not registered")
	}
	m := New()
	m.active = tool
	m.mode = modeTool
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	m.picker = filepicker.New(t.TempDir())
	next, _ = m.Update(fileResult{picker: m.picker, text: `{"answer":42}`})
	m = next.(Model)
	// jsonyaml 输出被 256 色转义序列切开（如 "answer: 42" 中间含 ANSI），
	// 断言前必须剥离 ANSI。
	if !strings.Contains(ansi.Strip(m.active.View()), "answer: 42") {
		t.Fatalf("file text did not reach tool: %s", m.active.View())
	}
}

func TestFileImportPreservesCRLFAndTabs(t *testing.T) {
	tool, ok := toolkit.Get("encoders.base64")
	if !ok {
		t.Fatal("Base64 tool not registered")
	}
	m := New()
	m.active = tool
	m.mode = modeTool
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m.picker = filepicker.New(t.TempDir())
	next, _ = m.Update(fileResult{picker: m.picker, text: "a\tb\r\nc"})
	m = next.(Model)
	// Base64 must encode the exact bytes, not expanded tabs or normalized line endings.
	if !strings.Contains(m.active.View(), "YQliDQpj") {
		t.Fatalf("wrong encoded output: %s", m.active.View())
	}
}

func TestSelectedImageGoesDirectlyToImageConverter(t *testing.T) {
	tool, ok := toolkit.Get("graphic.imageconvert")
	if !ok {
		t.Fatal("image converter missing")
	}
	m := New()
	m.active = tool
	m.mode = modeTool
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m.picker = filepicker.New(t.TempDir())
	next, _ = m.Update(fileResult{picker: m.picker, image: image.NewRGBA(image.Rect(0, 0, 3, 2)), name: "sample.png"})
	m = next.(Model)
	if m.picker != nil || m.preview != nil || !strings.Contains(m.active.View(), "sample.png") {
		t.Fatalf("image not routed to converter: %s", m.active.View())
	}
}

type backgroundTestTool struct {
	testTool
	received	bool
}

func (t *backgroundTestTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(backgroundTestEvent); ok {
		t.received = true
	}
	return t, nil
}

type backgroundTestEvent struct{}

func TestBackgroundCompletionReachesInactiveTool(t *testing.T) {
	tool := &backgroundTestTool{}
	m := New()
	m.mode = modeBrowse
	_, _ = m.Update(toolkit.BackgroundToolMsg{Tool: tool, Message: backgroundTestEvent{}})
	if !tool.received {
		t.Fatal("inactive tool did not receive completion")
	}
}

func TestOutputOnlyGeneratorDoesNotOpenFilePicker(t *testing.T) {
	tool, ok := toolkit.Get("generators.lorem")
	if !ok {
		t.Fatal("missing generator")
	}
	m := New()
	m.active = tool
	m.mode = modeTool
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(Model)
	if strings.Contains(m.View(), "No input required") || strings.Contains(m.statusView(), "Open file") {
		t.Fatalf("generator still exposes input: %s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if next.(Model).picker != nil {
		t.Fatal("output-only tool opened file picker")
	}
}

func TestBase64ImageFileImportPreservesBytes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var bytesOut bytes.Buffer
	if err := png.Encode(&bytesOut, img); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, bytesOut.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	registered, ok := toolkit.Get("encoders.base64image")
	if !ok {
		t.Fatal("missing Base64 image tool")
	}
	m := New()
	m.mode, m.active = modeTool, registered
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	m.picker = filepicker.New(dir)
	result := readSelectedFile(m.picker, path)()
	next, _ = m.Update(result)
	m = next.(Model)
	if !strings.Contains(ansi.Strip(m.active.View()), "data:image/png;base64,") {
		t.Fatalf("missing data URI: %s", m.active.View())
	}
	if !strings.Contains(ansi.Strip(m.active.View()), "image.png") {
		t.Fatal("preview missing selected filename")
	}
}

func TestCertificateToolReceivesBinaryFile(t *testing.T) {
	registered, ok := toolkit.Get("encoders.certificate")
	if !ok {
		t.Fatal("certificate tool missing")
	}
	m := New()
	m.mode, m.active = modeTool, registered
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	dir := t.TempDir()
	path := filepath.Join(dir, "input.cer")
	raw := []byte{0x30, 0x03, 0x02, 0x01, 0x01}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	m.picker = filepicker.New(dir)
	result := readSelectedFile(m.picker, path)()
	next, _ = m.Update(result)
	m = next.(Model)
	if m.picker != nil || !strings.Contains(ansi.Strip(m.active.View()), base64.StdEncoding.EncodeToString(raw)) {
		t.Fatal("binary input was not routed to certificate tool")
	}
}
