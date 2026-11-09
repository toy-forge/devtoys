package qrcode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

type exportDone struct {
	path	string
	err	error
}

type previewDone struct{ err error }

func (t *tool) startExport() tea.Cmd {
	if t.qr == nil {
		t.SetError(fmt.Errorf("enter text to generate a QR code first"))
		return nil
	}
	if t.saving {
		return nil
	}
	t.exporting = true
	t.SetError(nil)
	t.status = ""
	if t.path.Value() == "" {
		t.path.SetValue("qrcode.svg")
	}
	t.path.Width = max(1, t.width-2)
	t.path.CursorEnd()
	return t.path.Focus()
}

func (t *tool) updateExport(msg tea.Msg) tea.Cmd {
	switch event := msg.(type) {
	case tea.KeyMsg:
		switch event.String() {
		case "esc", "ctrl+g":
			t.exporting = false
			t.path.Blur()
			t.SetError(nil)
			return nil
		case "enter", "ctrl+s":
			return t.saveSVG()
		}
	case tea.MouseMsg:
		if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
			return nil
		}
		if event.Y == 0 {
			switch toolkit.ActionAt(event, "Cancel", "Save SVG") {
			case 0:
				t.exporting = false
				t.path.Blur()
				t.SetError(nil)
			case 1:
				return t.saveSVG()
			}
		} else if event.Y == 2 && event.X >= 2 {
			t.path.SetCursor(event.X - 2)
		}
		return nil
	}
	var cmd tea.Cmd
	t.path, cmd = t.path.Update(msg)
	return cmd
}

func (t *tool) saveSVG() tea.Cmd {
	if t.saving {
		return nil
	}
	path := strings.TrimSpace(t.path.Value())
	if path == "" {
		t.SetError(fmt.Errorf("enter an output file path"))
		return nil
	}
	if !strings.EqualFold(filepath.Ext(path), ".svg") {
		path += ".svg"
	}
	content := svgDocument(t.qr.Bitmap())
	t.saving = true
	t.exporting = false
	t.path.Blur()
	t.SetError(nil)
	t.status = "Saving SVG…"
	return func() tea.Msg {
		err := writeSVG(path, content)
		return toolkit.BackgroundToolMsg{Tool: t, Message: exportDone{path: path, err: err}}
	}
}

func (t *tool) exportView() string {
	width, height := max(1, t.width), max(1, t.height)
	lines := []string{toolkit.Actions("Cancel", "Save SVG"), lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render("Export QR code as SVG"), t.path.View()}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	footer := "Enter save  ·  Esc cancel"
	if t.Err != nil {
		footer = "Error: " + t.Err.Error()
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render(footer))
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

func svgDocument(matrix [][]bool) string {
	size := len(matrix)
	var path strings.Builder
	for y, row := range matrix {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	return fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\"><path fill=\"#fff\" d=\"M0 0h%dv%dh-%dz\"/><path fill=\"#000\" d=\"%s\"/></svg>\n", size, size, size, size, size, size, size, path.String())
}

func writeSVG(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("could not create SVG: %w", err)
	}
	written := false
	defer func() {
		if !written {
			_ = os.Remove(path)
		}
	}()
	_, err = file.WriteString(content)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("could not write SVG: %w", err)
	}
	written = true
	return nil
}
