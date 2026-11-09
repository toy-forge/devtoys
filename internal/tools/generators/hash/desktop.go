package hash

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

func (m *Model) desktopLayout() bool	{ return m.width >= 48 && m.workspaceHeight >= 9 }
func (m *Model) desktopGeometry() (left, right, inputHeight int) {
	left = (m.width - 4) / 2
	// body 与 OUTPUT 面板之间留 1 空行（纵向缝）——非运行态为
	// 空行缝（1+输出 3+页脚 1），运行态由进度行充当分隔（1+3+1）。
	reserved := 5
	return left, m.width - 4 - left, m.workspaceHeight - reserved
}
func (m *Model) resizeDesktop() {
	// 面板内容宽 = 外宽 - 边框 2 - 水平内边距 2。
	left, _, height := m.desktopGeometry()
	m.ws.Input.SetWidth(left - 4)
	m.ws.Input.SetHeight(height - 2)
	m.ws.Output.Width = m.width - 4
	m.ws.Output.Height = 1
	m.ws.Output.SetYOffset(m.ws.Output.YOffset)
}
func hashPane(content, title string, selected bool, actions ...string) string {
	// 面板矩阵：rest=Border 发丝线 + Muted bold 标题，
	// focused=BorderFocus（键盘焦点环）+ Text bold 标题；内容左右各
	// Space1 列水平内边距，与 ui.Panel 的面板语言一致。
	color := theme.Border
	if selected {
		color = theme.BorderFocus
	}
	pane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, ui.Space1).
		Render(content)
	lines := strings.Split(pane, "\n")
	width := lipgloss.Width(lines[0])
	label := " " + title + " "
	titleStyle := theme.MutedStyle().Bold(true)
	if selected {
		titleStyle = theme.TitleStyle()
	}
	if width > lipgloss.Width(label)+3 {
		lines[0] = ansi.Cut(lines[0], 0, 2) + titleStyle.Render(label) + ansi.Cut(lines[0], 2+lipgloss.Width(label), width)
		if start := hashToolbarStart(width, label, actions); start >= 0 {
			bar := toolkit.CompactActions(actions...)
			lines[0] = ansi.Cut(lines[0], 0, start) + bar + ansi.Cut(lines[0], start+lipgloss.Width(bar), width)
		}
	}
	// 逐行铺 Surface 底——行内分段（textarea、标题、
	// 按钮）的 reset 会取消外层背景，与 ui.Panel.Render 同一处理。
	for i, line := range lines {
		lines[i] = ui.WithBackground(line, theme.Surface)
	}
	return strings.Join(lines, "\n")
}

// hashFooter 渲染桌面布局的页脚行：整行 Surface 底 + Text 前景（与
// workspace footer 同一语言）；组合行内
// 分段的 reset 由 ui.WithBackground 逐段重注入底色。
func hashFooter(line string, width int) string {
	return hashSurfaceRow(theme.LabelStyle().Render(line), width)
}

// hashFooterLeft 返回页脚左段提示。
func hashFooterLeft(running bool) string {
	left := toolkit.CompactActions("<", ">") + "  scroll output · Ctrl+K key · Alt+V compare"
	if running {
		left += " · Alt+X cancel"
	}
	return left
}

// hashFooterRow 渲染页脚行：左 = [</>] 按钮 + 提示，
// 右 = Compare 校验和输入；过窄时左段提示逐级缩短，输入区保留可打字。
func (m *Model) hashFooterRow() string {
	left := hashFooterLeft(m.running)
	right := m.compareSegment()
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > m.width {
		left = toolkit.CompactActions("<", ">") + "  scroll output"
	}
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > m.width {
		left = toolkit.CompactActions("<", ">")
	}
	pad := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return hashSurfaceRow(left+strings.Repeat(" ", pad)+right, m.width)
}

// hashSurfaceRow 把一行内容右补到整行宽并铺 Surface 底。
func hashSurfaceRow(line string, width int) string {
	line = ansi.Truncate(line, max(1, width), "")
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return ui.WithBackground(line, theme.Surface)
}
func hashToolbarStart(width int, label string, actions []string) int {
	if len(actions) == 0 {
		return -1
	}
	start := width - 1 - lipgloss.Width(toolkit.CompactActions(actions...))
	if start < 3+lipgloss.Width(label) {
		return -1
	}
	return start
}
func safeFileName(path string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '�'
		}
		return r
	}, filepath.Base(path))
}
func (m *Model) desktopView() string {
	_, right, height := m.desktopGeometry()
	input := hashPane(m.ws.Input.View(), "Input text", m.focus == 0 && m.ws.Focus == toolkit.FocusInput, "Paste", "Clear", "Copy")
	fileLabel := "Select a file to hash"
	if m.path != "" {
		fileLabel = safeFileName(m.path)
	}
	fileContent := ansi.Truncate(fileLabel, right-4, "…") + "\n" + toolkit.CompactActions("Choose file")
	fileContent = lipgloss.NewStyle().Width(right - 4).Height(height - 2).Render(fileContent)
	file := hashPane(fileContent, "File", false)
	middle := lipgloss.NewStyle().Width(4).Height(height).Align(lipgloss.Center).AlignVertical(lipgloss.Center).Foreground(theme.Muted).Render("OR")
	body := lipgloss.JoinHorizontal(lipgloss.Top, input, middle, file)
	output := hashPane(m.ws.Output.View(), "Output", m.focus == 0 && m.ws.Focus == toolkit.FocusOutput, "Copy")
	progress := ""
	if m.running {
		progress = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Foreground(theme.Accent).Render("󰔟  Hashing file…  Alt+X to cancel")
	}
	footer := m.hashFooterRow()
	if m.ws.Err != nil {
		footer = hashSurfaceRow(lipgloss.NewStyle().Foreground(theme.Danger).Render("Error: "+m.ws.Err.Error()), m.width)
	} else if m.ws.Copied {
		footer = hashSurfaceRow("Copied to clipboard", m.width)
	}
	if m.running {
		return body + "\n" + progress + "\n" + output + "\n" + footer
	}
	return body + "\n\n" + output + "\n" + footer
}
func (m *Model) desktopMouse(mouse tea.MouseMsg) tea.Cmd {
	if mouse.Action != tea.MouseActionPress {
		return nil
	}
	left, _, height := m.desktopGeometry()
	outputTop := height
	if m.running {
		outputTop++
	}
	if mouse.X < 0 || mouse.X >= m.width || mouse.Y < 0 || mouse.Y >= m.workspaceHeight {
		return nil
	}
	if mouse.Y < height && mouse.X >= left+4 {
		if mouse.Button == tea.MouseButtonLeft {
			return func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		}
		return nil
	}
	if mouse.Y == m.workspaceHeight-1 && mouse.Button == tea.MouseButtonLeft {
		local := mouse
		local.Y = 0
		switch toolkit.CompactActionAt(local, "<", ">") {
		case 0:
			m.ws.Output.ScrollLeft(max(4, m.ws.Output.Width/2))
		case 1:
			m.ws.Output.ScrollRight(max(4, m.ws.Output.Width/2))
		}
		// 点击页脚右段聚焦校验和输入。
		if right := lipgloss.Width(hashCompareLabel) + m.expected.Width + 2*ui.Space1; mouse.X >= m.width-1-right {
			m.setFocus(2)
		}
		return nil
	}
	if mouse.Y < height && mouse.X < left {
		if mouse.Button == tea.MouseButtonWheelUp {
			m.ws.Input.Scroll(-3)
			return nil
		}
		if mouse.Button == tea.MouseButtonWheelDown {
			m.ws.Input.Scroll(3)
			return nil
		}
		if mouse.Button != tea.MouseButtonLeft {
			return nil
		}
		m.ws.Focus = toolkit.FocusInput
		m.ws.Input.FocusPreservingScroll()
		if mouse.Y == 0 {
			actions := []string{"Paste", "Clear", "Copy"}
			if start := hashToolbarStart(left, " INPUT TEXT ", actions); start >= 0 {
				local := mouse
				local.X -= start
				switch toolkit.CompactActionAt(local, actions...) {
				case 0:
					before := m.ws.Input.Value()
					updated, cmd := m.ws.Input.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
					m.ws.Input = updated
					if before != updated.Value() {
						m.path = ""
						return tea.Batch(cmd, m.recompute())
					}
					return cmd
				case 1:
					m.ws.Input.SetValue("")
					m.path = ""
					return m.recompute()
				case 2:
					if err := clipboard.WriteAll(m.ws.Input.Value()); err != nil {
						m.ws.SetError(fmt.Errorf("could not copy: %w", err))
					} else {
						m.ws.SetError(nil)
						m.ws.Copied = true
					}
					return nil
				}
			}
		}
		// 面板内边距：文本起点 = 边框内 2 列，PlaceCursor 的 X
		// 偏移 -2（Y 偏移 -1，无垂直内边距）。
		if mouse.X > 1 && mouse.X < left-1 && mouse.Y > 0 && mouse.Y < height-1 {
			return m.ws.Input.PlaceCursor(mouse.X-2, mouse.Y-1)
		}
		return nil
	}
	if mouse.Y >= outputTop && mouse.Y < outputTop+3 {
		if mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown {
			var cmd tea.Cmd
			m.ws.Output, cmd = m.ws.Output.Update(mouse)
			return cmd
		}
		if mouse.Button != tea.MouseButtonLeft {
			return nil
		}
		m.ws.Focus = toolkit.FocusOutput
		m.ws.Input.Blur()
		if mouse.Y == outputTop {
			actions := []string{"Copy"}
			if m.running {
				actions = []string{"Cancel"}
			}
			if start := hashToolbarStart(m.width, " OUTPUT ", actions); start >= 0 {
				local := mouse
				local.X -= start
				local.Y = 0
				if toolkit.CompactActionAt(local, actions...) == 0 && m.running {
					m.invalidate()
					m.ws.SetOutput("")
					m.ws.Output.SetContent("Cancelled")
					return m.resize()
				}
				if toolkit.CompactActionAt(local, actions...) == 0 {
					if err := m.ws.CopyOutput(); err != nil {
						m.ws.SetError(err)
					} else {
						m.ws.SetError(nil)
						m.ws.Copied = true
					}
					return nil
				}
			}
		}
	}
	return nil
}
