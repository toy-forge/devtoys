package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestAlgorithmsAndHMAC(t *testing.T) {
	m := New()
	m.ws.Input.SetValue("abc")
	for i, want := range []string{
		"900150983cd24fb0d6963f7d28e17f72",
		"a9993e364706816aba3e25717850c26c9cd0d89d",
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",
		"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		"352441c2",
	} {
		m.algorithm = i
		m.recomputeText()
		if got := m.ws.OutputText(); got != want {
			t.Fatalf("%s: %s", algorithms[i], got)
		}
	}
	m.algorithm = 2
	m.key.SetValue("secret")
	m.recomputeText()
	h := hmac.New(sha256.New, []byte("secret"))
	h.Write([]byte("abc"))
	if got := m.ws.OutputText(); got != hex.EncodeToString(h.Sum(nil)) {
		t.Fatalf("HMAC: %s", got)
	}
	m.expected.SetValue(strings.ToUpper(m.ws.OutputText()))
	if !strings.Contains(m.compare(), "match") || strings.Contains(m.compare(), "not") {
		t.Fatal(m.compare())
	}
	m.expected.SetValue("wrong")
	if !strings.Contains(m.compare(), "not match") {
		t.Fatal(m.compare())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}, Alt: true})
	if m.ws.OutputText() != strings.ToUpper(m.ws.OutputText()) {
		t.Fatal("uppercase not applied")
	}
}

func TestFileHashAndStaleResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary.dat")
	data := []byte{0, 255, 1, 2, 3}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.algorithm = 2
	_, cmd := m.Update(toolkit.FilePathImportMsg{Path: path})
	if cmd == nil || !m.running {
		t.Fatal("file hash did not start")
	}
	message := cmd().(toolkit.BackgroundToolMsg)
	m.Update(message.Message)
	sum := sha256.Sum256(data)
	if got := m.ws.OutputText(); got != hex.EncodeToString(sum[:]) {
		t.Fatal(got)
	}
	_, old := m.Update(toolkit.FilePathImportMsg{Path: path})
	m.ws.Input.SetValue("new")
	m.path = ""
	m.recomputeText()
	m.Update(old().(toolkit.BackgroundToolMsg).Message)
	if got := m.ws.OutputText(); got == hex.EncodeToString(sum[:]) {
		t.Fatal("stale file result replaced text result")
	}
}

func TestHashLayoutAndFields(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 7}, {Width: 30, Height: 7}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
		m := New()
		m.Update(size)
		for _, focus := range []int{0, 1, 2} {
			m.setFocus(focus)
			if w, h := lipgloss.Width(m.View()), lipgloss.Height(m.View()); w > size.Width || h != size.Height {
				t.Fatalf("%v focus %d: %dx%d\n%s", size, focus, w, h, m.View())
			}
		}
	}
}

func TestFileCancellationAndError(t *testing.T) {
	m := New()
	_, old := m.Update(toolkit.FilePathImportMsg{Path: filepath.Join(t.TempDir(), "missing")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	if m.running {
		t.Fatal("cancel did not stop pending state")
	}
	m.Update(old().(toolkit.BackgroundToolMsg).Message)
	if m.ws.Err != nil {
		t.Fatal("cancelled result applied")
	}
	_, cmd := m.Update(toolkit.FilePathImportMsg{Path: filepath.Join(t.TempDir(), "missing")})
	m.Update(cmd().(toolkit.BackgroundToolMsg).Message)
	if m.ws.Err == nil {
		t.Fatal("missing file did not report error")
	}
}

func TestDesktopHashLayoutAndMouse(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	if !m.desktopLayout() {
		t.Fatal("desktop layout not active")
	}
	lines := strings.Split(m.View(), "\n")
	textRow, fileRow, outputRow, compareRow := -1, -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "Input text") && strings.Contains(line, "File"):
			textRow, fileRow = i, i
		case strings.Contains(line, "Output"):
			outputRow = i
		case strings.Contains(line, "Compare"):
			compareRow = i
		}
	}
	if textRow < 0 || fileRow != textRow || outputRow <= textRow || compareRow <= outputRow {
		t.Fatalf("unexpected layout:\n%s", m.View())
	}
	left, _, paneHeight := m.desktopGeometry()
	top, _ := m.extraRows()
	base := m.headerHeight + top
	_, cmd := m.Update(tea.MouseMsg{X: left + 6, Y: base + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil {
		t.Fatal("file panel did not open picker")
	}
	if _, ok := cmd().(toolkit.OpenFileRequestMsg); !ok {
		t.Fatal("file panel returned wrong command")
	}
	m.Update(tea.MouseMsg{X: 2, Y: base + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})
	if m.ws.OutputText() != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("typing did not update digest: %s", m.ws.OutputText())
	}
	m.Update(tea.MouseMsg{X: 3, Y: base + paneHeight + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.ws.Focus != toolkit.FocusOutput {
		t.Fatal("output click did not set focus")
	}
	// Compare 输入在页脚行右段（body 下空行缝+输出
	// 面板 3 行之后的最后一行右缘）。
	m.Update(tea.MouseMsg{X: m.width - 3, Y: base + paneHeight + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != 2 {
		t.Fatalf("compare field did not receive focus (focus=%d)", m.focus)
	}
}
