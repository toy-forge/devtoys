package app

import (
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

func paneHintsFromActive(active toolkit.Tool) []ui.StatusHint {
	return toolkit.PaneStatusHints(active)
}
