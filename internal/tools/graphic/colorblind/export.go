package colorblind

import (
	"fmt"
	"image"
	"image/png"
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
	revision	uint64
	path		string
	kind		int
	err		error
}

func suggestedName(name string, kind int) string {
	name = filepath.Base(name)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" || base == "." {
		base = "image"
	}
	label := strings.TrimSuffix(strings.ToLower(names[kind]), " simulation")
	return base + "-" + label + ".png"
}
func (t *tool) editPath() tea.Cmd {
	if t.img == nil {
		t.err = fmt.Errorf("open an image first")
		return nil
	}
	if t.saving {
		t.err = fmt.Errorf("a save is already in progress")
		return nil
	}
	t.editing = true
	t.err = nil
	t.status = ""
	if t.pathRevision != t.revision || t.pathKind != t.selected || t.path.Value() == "" {
		t.path.SetValue(suggestedName(t.name, t.selected))
	}
	t.pathRevision, t.pathKind = t.revision, t.selected
	t.path.Width = max(1, t.width-2)
	t.path.CursorEnd()
	return t.path.Focus()
}
func (t *tool) cancelPath()	{ t.editing = false; t.path.Blur(); t.err = nil }
func (t *tool) startExport() tea.Cmd {
	if t.saving {
		return nil
	}
	if t.img == nil {
		t.err = fmt.Errorf("open an image first")
		return nil
	}
	path := strings.TrimSpace(t.path.Value())
	if path == "" {
		t.err = fmt.Errorf("enter an output file path")
		return nil
	}
	img, kind, revision := t.img, t.selected, t.revision
	t.saving = true
	t.editing = false
	t.path.Blur()
	t.err = nil
	t.status = "Saving PNG…"
	return func() tea.Msg {
		err := writePNG(path, simulated{source: img, kind: kind})
		return toolkit.BackgroundToolMsg{Tool: t, Message: exportDone{revision: revision, path: path, kind: kind, err: err}}
	}
}
func writePNG(path string, img image.Image) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return fmt.Errorf("could not create output: %w", err)
	}
	written := false
	defer func() {
		if !written {
			_ = os.Remove(path)
		}
	}()
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("could not write PNG: %w", err)
	}
	written = true
	return nil
}
func (t *tool) pathUpdate(msg tea.Msg) tea.Cmd {
	switch event := msg.(type) {
	case tea.KeyMsg:
		switch event.String() {
		case "ctrl+g":
			t.cancelPath()
			return nil
		case "enter", "ctrl+s":
			return t.startExport()
		}
	case tea.MouseMsg:
		if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
			return nil
		}
		if event.Y == 0 {
			switch toolkit.ActionAt(event, "Cancel", "Save") {
			case 0:
				t.cancelPath()
			case 1:
				return t.startExport()
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
func (t *tool) pathView() string {
	width, height := max(1, t.width), max(1, t.height)
	title := fmt.Sprintf("Save %s as PNG", names[t.selected])
	lines := []string{toolkit.Actions("Cancel", "Save"), title, t.path.View()}
	remaining := height - 4
	if remaining >= 3 {
		lines = append(lines, strings.Split(t.card(t.selected, width, remaining), "\n")...)
	}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	footer := "Enter save · Ctrl+G cancel"
	if t.err != nil {
		footer = "Error: " + safe(t.err.Error())
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
