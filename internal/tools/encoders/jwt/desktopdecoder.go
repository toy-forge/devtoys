package jwt

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// decoderDesktop places the token and verification key above separate, read-only
// header and payload panes, following the desktop decoder's visual order.
type decoderDesktop struct {
	active						bool
	width, height, tokenHeight, keyHeight, resultY	int
	leftWidth					int	// 双栏左列宽（中缝 Space2）
	focus						int	// token, key, header, payload
	header, payload					viewport.Model
	status						string
	decoded						bool	// 已解码出有效令牌（状态行仅此时展示结论）
	showClaims					bool
	expanded					bool
	footer						*ui.StatusBar
}

func newDecoderDesktop() *decoderDesktop {
	return &decoderDesktop{header: viewport.New(1, 1), payload: viewport.New(1, 1), footer: ui.NewStatusBar()}
}

func (d *decoderDesktop) resize(t *tool, width, height int) {
	wasActive := d.active
	d.width, d.height = width, height
	d.active = width >= 46 && height >= 14
	if !d.active {
		if wasActive {
			t.cur = focusToken
			t.applyFocus()
		}
		return
	}
	d.tokenHeight = max(4, min(7, height/3))
	// 面板内容 1 列内边距：文本区 = 外宽-4。
	t.Input.SetWidth(width - 4)
	t.Input.SetHeight(d.tokenHeight - 2)
	t.secret.SetWidth(width - 4)
	t.secret.SetHeight(1)
	d.layoutResults(t)
	if !wasActive {
		d.focus = 0
	}
	d.applyFocus(t)
	d.refresh(t)
}

func (d *decoderDesktop) layoutResults(t *tool) {
	d.keyHeight = 0
	if t.verificationKeyVisible() {
		d.keyHeight = 3
	}
	// 双栏缝 Space2、纵向缝 1 空行：令牌与密钥、密钥与结果面板
	// 之间各留 1 行；左右结果窗格中缝 2 列。
	gapBeforeKey := 0
	if d.keyHeight > 0 {
		gapBeforeKey = 1
	}
	d.resultY = d.tokenHeight + gapBeforeKey + d.keyHeight + 1
	resultHeight := max(1, d.height-d.resultY-1)
	d.leftWidth = (d.width - ui.Space2) / 2
	if d.expanded {
		d.payload.Width, d.payload.Height = d.width-4, resultHeight-2
	} else {
		d.header.Width, d.header.Height = d.leftWidth-4, resultHeight-2
		d.payload.Width, d.payload.Height = d.width-d.leftWidth-ui.Space2-4, resultHeight-2
	}
	d.header.SetYOffset(d.header.YOffset)
	d.payload.SetYOffset(d.payload.YOffset)
}

func (d *decoderDesktop) applyFocus(t *tool) {
	t.Input.Blur()
	t.secret.Blur()
	switch d.focus {
	case 0:
		t.cur = focusToken
		t.Input.FocusPreservingScroll()
	case 1:
		t.cur = focusSecret
		t.secret.FocusPreservingScroll()
	default:
		t.cur = focusOutput
	}
}

func (d *decoderDesktop) refresh(t *tool) {
	d.layoutResults(t)
	d.header.SetContent("")
	d.payload.SetContent("")
	d.status = "Paste a JWT to decode"
	d.decoded = false
	token := strings.Join(strings.Fields(t.Input.Value()), "")
	parts := strings.Split(token, ".")
	if len(parts) != 3 || t.Err != nil {
		return
	}
	header, e1 := decodeSegment(parts[0])
	payload, e2 := decodeSegment(parts[1])
	if e1 != nil || e2 != nil {
		return
	}
	d.header.SetContent(colorizeJSON(header))
	content := colorizeJSON(payload)
	if d.showClaims {
		content = d.claimsTable(payload, d.payload.Width)
	}
	if times := renderTimeFields(payload); times != "" {
		content += "\n\n" + sectionStyle.Render("Time claims") + "\n" + times
	}
	if t.validateToken && t.claims.enabled() {
		content += "\n" + checkClaims(payload, t.claims, time.Now())
	}
	d.payload.SetContent(content)
	d.status = strings.SplitN(t.OutputText(), "\n", 2)[0]
	d.decoded = true
}

// claimsTable 把载荷的声明渲染为两列表格：声明名（定宽 20 列）与取值（占满剩余宽度）。
func (d *decoderDesktop) claimsTable(payload []byte, width int) string {
	var claims map[string]jsontext.Value
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return "Payload is not a JSON object"
	}
	const nameWidth = 20
	table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
		{Title: "Claim", Width: nameWidth},
		{Title: "Value", Width: max(1, width-nameWidth-ui.Space2)},
	}}
	var b strings.Builder
	b.WriteString(table.Header())
	for _, name := range slices.Sorted(maps.Keys(claims)) {
		b.WriteByte('\n')
		value := strings.ReplaceAll(string(claims[name]), "\n", " ")
		label := strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return '�'
			}
			return r
		}, name)
		_, line := table.Row(label, value)
		b.WriteString(line)
	}
	return b.String()
}

func decoderActions(index int) []string {
	switch index {
	case 0, 1:
		return []string{"Paste", "Open", "Clear", "Copy"}
	case 2:
		return []string{"Copy"}
	default:
		return []string{"Claims", "Copy"}
	}
}

func (d *decoderDesktop) title(t *tool, index int) string {
	if index == 1 {
		return t.keyTitle()
	}
	return []string{"Token", "", "Header", "Payload"}[index]
}

func decoderToolbarStart(width, index int, title string) int {
	label := " " + title + " "
	start := width - 1 - lipgloss.Width(toolkit.CompactActions(decoderActions(index)...))
	if start < 3+lipgloss.Width(label) {
		return -1
	}
	return start
}

func (d *decoderDesktop) copy(t *tool, index int) {
	text := ansi.Strip(t.OutputText())
	switch index {
	case 0:
		text = t.Input.Value()
	case 1:
		text = t.secret.Value()
	case 2, 3:
		parts := strings.Split(strings.Join(strings.Fields(t.Input.Value()), ""), ".")
		if len(parts) != 3 {
			return
		}
		decoded, err := decodeSegment(parts[index-2])
		if err != nil {
			return
		}
		text = string(decoded)
	}
	if err := clipboard.WriteAll(text); err != nil {
		t.SetError(fmt.Errorf("could not copy: %w", err))
	} else {
		t.Copied = true
	}
}

func (d *decoderDesktop) toggleClaims(t *tool) {
	d.showClaims = !d.showClaims
	d.payload.SetYOffset(0)
	d.refresh(t)
}

func (d *decoderDesktop) toolbar(t *tool, index, action int) tea.Cmd {
	d.focus = index
	d.applyFocus(t)
	if index >= 2 {
		if index == 3 && action == 0 {
			d.toggleClaims(t)
		} else {
			d.copy(t, index)
		}
		return nil
	}
	target := &t.Input
	if index == 1 {
		target = &t.secret
	}
	switch action {
	case 0:
		before := target.Value()
		updated, cmd := target.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
		*target = updated
		if target.Value() != before {
			t.Copied = false
			t.recalc()
		}
		return cmd
	case 1:
		return func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
	case 2:
		target.SetValue("")
		t.Copied = false
		t.recalc()
	case 3:
		d.copy(t, index)
	}
	return nil
}

func (d *decoderDesktop) update(t *tool, msg tea.Msg) tea.Cmd {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		return nil
	case toolkit.FileImportMsg:
		target := &t.Input
		if d.focus == 1 {
			target = &t.secret
		} else {
			d.focus = 0
			d.applyFocus(t)
		}
		if err := toolkit.InsertFileText(target, event.Text); err != nil {
			t.SetError(err)
			return nil
		}
		t.recalc()
		return nil
	case toolkit.InputClearedMsg:
		t.recalc()
		return nil
	case tea.KeyMsg:
		switch event.String() {
		case "ctrl+e":
			if d.focus == 3 || d.expanded {
				d.expanded = !d.expanded
				d.layoutResults(t)
			}
			return nil
		case "alt+c":
			d.toggleClaims(t)
			return nil
		case "tab":
			d.focus = (d.focus + 1) % 4
			d.applyFocus(t)
			return nil
		case "shift+tab":
			d.focus = (d.focus + 3) % 4
			d.applyFocus(t)
			return nil
		case "ctrl+s":
			d.focus = 1
			d.applyFocus(t)
			return nil
		case "ctrl+y":
			index := d.focus
			if index < 2 {
				index = 4
			}
			d.copy(t, index)
			return nil
		}
		beforeToken, beforeKey := t.Input.Value(), t.secret.Value()
		var cmd tea.Cmd
		switch d.focus {
		case 0:
			t.Input, cmd = t.Input.Update(event)
		case 1:
			t.secret, cmd = t.secret.Update(event)
		case 2:
			d.header, cmd = d.header.Update(event)
		case 3:
			d.payload, cmd = d.payload.Update(event)
		}
		if beforeToken != t.Input.Value() || beforeKey != t.secret.Value() {
			t.Copied = false
			t.recalc()
		}
		return cmd
	case tea.MouseMsg:
		if event.Action != tea.MouseActionPress {
			return nil
		}
		focus := 0
		switch {
		case event.Y >= d.resultY:
			focus = 2
			if d.expanded || event.X >= d.leftWidth+ui.Space2 {
				focus = 3
			}
		case d.keyHeight > 0 && event.Y >= d.tokenHeight+1:
			focus = 1
		}
		if event.Button == tea.MouseButtonWheelUp || event.Button == tea.MouseButtonWheelDown {
			delta := 3
			if event.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			switch focus {
			case 0:
				t.Input.Scroll(delta)
			case 1:
				t.secret.Scroll(delta)
			case 2:
				d.header, _ = d.header.Update(event)
			case 3:
				d.payload, _ = d.payload.Update(event)
			}
			return nil
		}
		if event.Button != tea.MouseButtonLeft || event.Y >= d.height-1 {
			return nil
		}
		paneX, paneY, paneWidth := 0, 0, d.width
		switch focus {
		case 1:
			paneY = d.tokenHeight + 1
		case 2, 3:
			paneY = d.resultY
			paneWidth = d.leftWidth
			if d.expanded {
				paneWidth = d.width
			} else if focus == 3 {
				paneX = d.leftWidth + ui.Space2
				paneWidth = d.width - paneX
			}
		}
		if event.Y == paneY {
			if start := decoderToolbarStart(paneWidth, focus, d.title(t, focus)); start >= 0 {
				local := event
				local.X -= paneX + start
				local.Y = 0
				if action := toolkit.CompactActionAt(local, decoderActions(focus)...); action >= 0 {
					return d.toolbar(t, focus, action)
				}
			}
		}
		d.focus = focus
		d.applyFocus(t)
		if focus == 0 && event.X > 1 && event.X < d.width-2 && event.Y > 0 && event.Y < d.tokenHeight-1 {
			return t.Input.PlaceCursor(event.X-2, event.Y-1)
		}
		if focus == 1 && event.X > 1 && event.X < d.width-2 && event.Y == d.tokenHeight+2 {
			return t.secret.PlaceCursor(event.X-2, 0)
		}
	}
	return nil
}

func (d *decoderDesktop) pane(t *tool, content string, index int, selected bool) string {
	style := secretPaneStyle
	if selected {
		style = secretPaneFocusedStyle
	}
	lines := strings.Split(style.Render(content), "\n")
	label := " " + d.title(t, index) + " "
	width := lipgloss.Width(lines[0])
	if width > lipgloss.Width(label)+3 {
		color := theme.Muted
		if selected {
			color = theme.BlueBright
		}
		lines[0] = ansi.Cut(lines[0], 0, 2) + lipgloss.NewStyle().Foreground(color).Bold(true).Render(label) + ansi.Cut(lines[0], 2+lipgloss.Width(label), width)
		if start := decoderToolbarStart(width, index, d.title(t, index)); start >= 0 {
			buttons := toolkit.CompactActions(decoderActions(index)...)
			lines[0] = ansi.Cut(lines[0], 0, start) + buttons + ansi.Cut(lines[0], start+lipgloss.Width(buttons), width)
		}
	}
	return strings.Join(lines, "\n")
}

func (d *decoderDesktop) view(t *tool) string {
	token := d.pane(t, t.Input.View(), 0, d.focus == 0)
	header := d.pane(t, d.header.View(), 2, d.focus == 2)
	payload := d.pane(t, d.payload.View(), 3, d.focus == 3)
	// 纵向缝 1 空行：令牌/密钥、密钥/结果面板之间各留 1 行，
	// 与 layoutResults 的 resultY 预算同源。
	inputs := token
	if d.keyHeight > 0 {
		inputs += "\n\n" + d.pane(t, t.secret.View(), 1, d.focus == 1)
	}
	// 双栏缝 Space2：结果双窗格中缝 2 列。
	results := lipgloss.JoinHorizontal(lipgloss.Top, header, strings.Repeat(" ", ui.Space2), payload)
	if d.expanded {
		results = payload
	}
	return inputs + "\n\n" + results + "\n" + d.footerView(t)
}

// footerView 渲染底行：ui.StatusBar——左句式（解码出结论时以第二个
// token 附带验签结论），右端按键提示；错误与已复制以 Toast 呈现。
func (d *decoderDesktop) footerView(t *tool) string {
	d.footer.SetWidth(max(1, d.width))
	left := "click to edit · scroll to browse"
	if d.expanded {
		left += " · ctrl+E Restore"
	} else if d.focus == 3 {
		left += " · ctrl+E Expand"
	}
	tokens := []ui.StatusToken{{Label: left}}
	if d.decoded && d.status != "" && t.Err == nil {
		tokens = append(tokens, ui.StatusToken{Label: d.status})
	}
	d.footer.SetLeftTokens(tokens)
	d.footer.SetHints(ui.StatusHint{Key: "alt+C", Desc: "claims"}, ui.StatusHint{Key: "ctrl+S", Desc: "key"})
	switch {
	case t.Err != nil:
		d.footer.SetToast(ui.Toast{Level: ui.LevelError, Message: "Error: " + t.Err.Error()})
	case t.Copied:
		d.footer.SetToast(ui.Toast{Level: ui.LevelSuccess, Message: "Copied"})
	default:
		d.footer.ClearToast()
	}
	return d.footer.View()
}
