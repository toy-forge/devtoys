// Package escape provides string escaping and unescaping.
package escape

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

var escaper = strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t", "\b", "\\b", "\f", "\\f", "\"", "\\\"")
var unescaper = strings.NewReplacer("\\\\", "\\", "\\n", "\n", "\\r", "\r", "\\t", "\t", "\\b", "\b", "\\f", "\f", "\\\"", "\"")

type tool struct {
	ws	toolkit.Workspace
	header	toolkit.SettingHeader
	encode	bool
}

func newTool() *tool {
	t := &tool{
		ws:	toolkit.NewWorkspace("Enter text to escape or unescape…", ""),
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Conversion"},
			Descriptions:	[]string{"Select which conversion mode you want to use"},
			Icons:		[]string{"󰦝"},
			Switches:	[]bool{true},
		},
		encode:	true,
	}
	t.ws.SetStacked(true)
	t.ws.SetOutputExtendable(true)
	return t
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "text.escape" }
func (*tool) Title() string		{ return "Text Escape / Unescape" }
func (*tool) ShortTitle() string	{ return "Escape / Unescape" }
func (*tool) Category() string		{ return "text" }
func (*tool) Icon() string		{ return "󰘦" }
func (*tool) Description() string	{ return "Escape or unescape special characters in text" }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) action() string {
	if t.encode {
		return "Escape"
	}
	return "Unescape"
}
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return t, t.header.Resize(sz, &t.ws, t.action())
	}
	if t.header.Index(msg, t.action()) == 0 {
		t.encode = !t.encode
		t.recalc()
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+t" {
		t.encode = !t.encode
		t.recalc()
		return t, nil
	}
	cmd := t.ws.UpdateBelow(msg, t.header.Height(t.action()))
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.ws.Err != nil {
		return t, cmd
	}
	t.recalc()
	return t, cmd
}
func (t *tool) View() string	{ return t.header.View(t.action()) + "\n" + t.ws.View() }
func (t *tool) recalc() {
	out := t.ws.Input.Value()
	if t.encode {
		out = escaper.Replace(out)
	} else {
		out = unescaper.Replace(out)
	}
	display := strings.Map(func(r rune) rune {
		switch r {
		case '\n':
			return r
		case '\r':
			return '␍'
		case '\t':
			return '␉'
		case '\b':
			return '␈'
		case '\f':
			return '␌'
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, out)
	t.ws.SetOutputDisplay(out, display)
}
