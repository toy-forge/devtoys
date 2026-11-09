// Package jsontable converts a JSON array of objects into tabular data.
package jsontable

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

type tool struct {
	ws		toolkit.Workspace
	format		int
	menu		int
	before		string
	headings	[]string
	rows		[][]string
	width		int
	height		int
	saving		bool
	path		textinput.Model
	pathWidth	int	// 保存路径字段的单元格宽度（随窗口尺寸）
}

func newTool() *tool {
	t := &tool{ws: toolkit.NewWorkspace("Paste a JSON array of objects…", ""), menu: -1}
	t.ws.SetPaneTitles("Input", "Output")
	t.ws.SetOutputExtendable(true)
	t.ws.SetOutputControlsDisabled(true)
	t.updateOutputControls()
	t.path = textinput.New()
	t.path.Placeholder = "table.csv"
	t.path.SetValue("table.csv")
	return t
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "converters.jsontable" }
func (*tool) Title() string		{ return "JSON Array to Table" }
func (*tool) ShortTitle() string	{ return "JSON > Table" }
func (*tool) Category() string		{ return "converters" }
func (*tool) Icon() string		{ return "󰓫" }
func (*tool) Description() string {
	return "Convert a JSON array to a table and copy as CSV, TSV or FSV"
}
func (*tool) AcceptsFiles() bool	{ return true }
func (*tool) Init() tea.Cmd		{ return nil }

var formatNames = []string{"CSV", "TSV", "FSV"}
var separators = []rune{',', '\t', ';'}

func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		if t.saving {
			t.pathWidth = max(1, event.Width-4)
			return t, nil
		}
		t.resize()
		if len(t.headings) > 0 {
			t.ws.SetOutputDisplay(t.ws.OutputText(), renderTable(t.headings, t.rows, max(1, t.ws.Output.Width)))
		}
		return t, nil
	case toolkit.WorkspaceActionMsg:
		switch event.Action {
		case "󰆏 Copy as", "󰆏":
			t.menu = 0
		case "󰆓 Save as", "󰆓":
			t.menu = 1
		default:
			return t, nil
		}
		t.resize()
		return t, nil
	}
	if t.saving {
		return t, t.updateSave(msg)
	}
	if t.menu >= 0 {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "esc" {
				t.menu = -1
				t.resize()
				return t, nil
			}
			if len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '3' {
				return t, t.chooseFormat(int(key.Runes[0] - '1'))
			}
		}
		if mouse, ok := msg.(tea.MouseMsg); ok && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
			if mouse.Y == 0 {
				local := mouse
				local.Y = 0
				if choice := toolkit.ActionAt(local, formatNames...); choice >= 0 {
					return t, t.chooseFormat(choice)
				}
			}
		}
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+y" {
		return t, t.copyAs(t.format)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+c":
			if len(t.headings) > 0 {
				t.menu = 0
				t.resize()
			}
			return t, nil
		case "alt+s":
			if len(t.headings) > 0 {
				t.menu = 1
				t.resize()
			}
			return t, nil
		}
	}
	cmd := t.ws.Update(msg)
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.ws.Err != nil {
		return t, cmd
	}
	if cur := t.ws.Input.Value(); cur != t.before {
		t.before = cur
		t.recalc()
	}
	return t, cmd
}

func (t *tool) resize() {
	height := t.height
	if t.menu >= 0 {
		height--
	}
	t.ws.Update(tea.WindowSizeMsg{Width: t.width, Height: max(1, height)})
	t.updateOutputControls()
}

func (t *tool) updateOutputControls() {
	if t.ws.Output.Width < 30 {
		t.ws.SetOutputControls("󰆏", "󰆓")
		return
	}
	t.ws.SetOutputControls("󰆏 Copy as", "󰆓 Save as")
}

func (t *tool) chooseFormat(format int) tea.Cmd {
	menu := t.menu
	t.menu, t.format = -1, format
	t.resize()
	if menu == 0 {
		return t.copyAs(format)
	}
	return t.beginSave()
}

func (t *tool) View() string {
	if t.saving {
		return t.saveView()
	}
	if t.menu >= 0 {
		return toolkit.Actions(formatNames...) + "\n" + t.ws.View()
	}
	return t.ws.View()
}
func (t *tool) recalc() {
	t.ws.SetOutputControlsDisabled(true)
	src := t.ws.Input.Value()
	if strings.TrimSpace(src) == "" {
		t.headings, t.rows = nil, nil
		t.ws.SetError(nil)
		t.ws.SetOutput("")
		return
	}
	headings, rows, err := parseTable(src)
	if err != nil {
		t.headings, t.rows = nil, nil
		t.ws.SetError(err)
		t.ws.SetOutput("")
		return
	}
	text, err := delimited(headings, rows, separators[t.format])
	if err != nil {
		t.headings, t.rows = nil, nil
		t.ws.SetError(err)
		t.ws.SetOutput("")
		return
	}
	t.ws.SetError(nil)
	t.headings, t.rows = headings, rows
	t.ws.SetOutputControlsDisabled(false)
	t.ws.SetOutputDisplay(text, renderTable(headings, rows, max(1, t.ws.Output.Width)))
}

func (t *tool) copyAs(format int) tea.Cmd {
	if len(t.headings) == 0 {
		return nil
	}
	text, err := delimited(t.headings, t.rows, separators[format])
	if err == nil {
		err = clipboard.WriteAll(text)
	}
	if err != nil {
		t.ws.SetError(fmt.Errorf("could not copy %s: %w", formatNames[format], err))
	} else {
		t.ws.SetError(nil)
	}
	return nil
}

func (t *tool) beginSave() tea.Cmd {
	if len(t.headings) == 0 {
		return nil
	}
	t.saving = true
	base := strings.TrimSuffix(t.path.Value(), filepath.Ext(t.path.Value()))
	if strings.TrimSpace(base) == "" || base == "." {
		base = "table"
	}
	t.path.SetValue(base + "." + []string{"csv", "tsv", "fsv"}[t.format])
	t.path.Focus()
	t.path.CursorEnd()
	t.ws.SetError(nil)
	return nil
}

func (t *tool) updateSave(msg tea.Msg) tea.Cmd {
	if mouse, ok := msg.(tea.MouseMsg); ok && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y == 0 {
		switch toolkit.ActionAt(mouse, "Cancel", "Save") {
		case 0:
			t.saving = false
			t.path.Blur()
		case 1:
			t.save()
		}
		return nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "ctrl+g":
			t.saving = false
			t.path.Blur()
			return nil
		case "enter", "ctrl+s":
			t.save()
			return nil
		}
	}
	var cmd tea.Cmd
	t.path, cmd = t.path.Update(msg)
	return cmd
}

func (t *tool) save() {
	path := strings.TrimSpace(t.path.Value())
	if path == "" {
		t.ws.SetError(fmt.Errorf("enter an output file path"))
		return
	}
	text, err := delimited(t.headings, t.rows, separators[t.format])
	if err == nil {
		var file *os.File
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, writeErr := file.WriteString(text)
			closeErr := file.Close()
			err = writeErr
			if err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(path)
			}
		}
	}
	if err != nil {
		t.ws.SetError(fmt.Errorf("could not save file: %w", err))
		return
	}
	t.saving = false
	t.path.Blur()
	t.ws.SetError(nil)
}

func (t *tool) saveView() string {
	width := max(1, t.width)
	field := ui.FieldView(&t.path, t.pathWidth, t.path.Focused())
	lines := []string{toolkit.Actions("Cancel", "Save"), "Save table as " + formatNames[t.format], field, "Enter save · Ctrl+G cancel"}
	if t.ws.Err != nil {
		lines[3] = "Error: " + t.ws.Err.Error()
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

// parseTable preserves first-seen property order and flattens nested objects.
// Array-valued properties are omitted, matching the desktop tool.
func parseTable(src string) ([]string, [][]string, error) {
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('[') {
		return nil, nil, fmt.Errorf("expected a JSON array of objects")
	}
	var headings []string
	var objects []map[string]string
	seen := map[string]bool{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil || tok != json.Delim('{') {
			return nil, nil, fmt.Errorf("expected an object in the JSON array")
		}
		row := map[string]string{}
		if err = readObject(dec, "", row, &headings, seen); err != nil {
			return nil, nil, err
		}
		objects = append(objects, row)
	}
	if _, err = dec.Token(); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, nil, fmt.Errorf("unexpected data after JSON array")
	}
	if len(headings) == 0 {
		return nil, nil, fmt.Errorf("array has no scalar properties")
	}
	rows := make([][]string, len(objects))
	for i, obj := range objects {
		rows[i] = make([]string, len(headings))
		for j, key := range headings {
			rows[i][j] = obj[key]
		}
	}
	return headings, rows, nil
}
func readObject(dec *json.Decoder, prefix string, row map[string]string, headings *[]string, seen map[string]bool) error {
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("invalid object key")
		}
		if prefix != "" {
			key = prefix + "_" + key
		}
		token, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		switch value := token.(type) {
		case json.Delim:
			if value == '{' {
				if err = readObject(dec, key, row, headings, seen); err != nil {
					return err
				}
			} else if value == '[' {
				if err = skipArray(dec); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("invalid value for %s", key)
			}
		default:
			if _, exists := row[key]; exists {
				return fmt.Errorf("duplicate flattened property %q", key)
			}
			text := ""
			if value != nil {
				text = fmt.Sprint(value)
			}
			row[key] = text
			if !seen[key] {
				seen[key] = true
				*headings = append(*headings, key)
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
func skipArray(dec *json.Decoder) error {
	for dec.More() {
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
func delimited(headings []string, rows [][]string, separator rune) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = separator
	if err := w.Write(headings); err != nil {
		return "", err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	return buf.String(), w.Error()
}

// renderTable 用 ui.Table 渲染 JSON 数组的表格：表头 Muted 粗体、表头下一条水平线、
// 列间距 Space2，没有竖线（与应用其它表格一致）。列宽取内容宽度，单列不超过可用宽度。
func renderTable(headings []string, rows [][]string, availableWidth int) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, s)
	}
	widths := make([]int, len(headings))
	for i, heading := range headings {
		widths[i] = lipgloss.Width(clean(heading))
	}
	for _, row := range rows {
		for i, value := range row {
			widths[i] = max(widths[i], lipgloss.Width(clean(value)))
		}
	}
	columns := make([]ui.Column, len(headings))
	for i, heading := range headings {
		columns[i] = ui.Column{Title: clean(heading), Width: min(max(4, widths[i]), max(4, availableWidth-3))}
	}
	table := ui.Table{Gap: ui.Space2, Columns: columns}
	lines := []string{table.Header(), table.Rule()}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, value := range row {
			cells[i] = clean(value)
		}
		_, line := table.Row(cells...)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
