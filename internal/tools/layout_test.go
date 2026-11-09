package tools

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestToolsFitShortViewport(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 30, Height: 7}, {Width: 20, Height: 7}} {
		for _, tool := range toolkit.All() {
			tool.Update(size)
			v := tool.View()
			if width, height := lipgloss.Width(v), lipgloss.Height(v); width > size.Width || height != size.Height {
				t.Errorf("%s: view %dx%d, want at most %dx%d", tool.ID(), width, height, size.Width, size.Height)
			}
		}
	}
}
