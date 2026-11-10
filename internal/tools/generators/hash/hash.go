// Package hash implements the desktop hash and checksum generator.
package hash

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

var algorithms = []string{"MD5", "SHA-1", "SHA-256", "SHA-384", "SHA-512", "CRC32"}
var algorithmLabels = []string{"MD5", "SHA1", "SHA256", "SHA384", "SHA512"}

const settingsContentGap = 1

type Model struct {
	ws						toolkit.Workspace
	header						toolkit.SettingHeader
	key, expected					textinput.Model
	algorithm					int
	uppercase					bool
	path						string
	focus						int	// 0 workspace, 1 key, 2 checksum
	width, height, headerHeight, workspaceHeight	int
	gap						int	// 设置条与内容区之间的空行数
	revision					uint64
	cancel						context.CancelFunc
	running						bool
}

type hashResult struct {
	revision	uint64
	digest		string
	err		error
}

func New() *Model {
	key, expected := textinput.New(), textinput.New()
	// 提示符「┃ 」与 password/certificate 的内联输入同构。
	key.Prompt, key.Placeholder, key.CharLimit = "┃ ", "Optional HMAC secret", 4096
	expected.Prompt, expected.Placeholder, expected.CharLimit = "", "Paste checksum to compare", 4096
	// 占位符接 theme token：bubbles textinput 的默认
	// PlaceholderStyle 是 256 色 "240"，明显暗于 theme.Muted。
	key.PlaceholderStyle = theme.MutedStyle()
	expected.PlaceholderStyle = theme.MutedStyle()
	m := &Model{
		ws:		toolkit.NewWorkspace("Enter text to hash…", ""),
		key:		key,
		expected:	expected,
		header: toolkit.SettingHeader{
			Names:		[]string{"Hashing Algorithm", "Uppercase"},
			Descriptions:	[]string{"Select which hashing algorithm you want to use"},
			Switches:	[]bool{false, true},
			Options:	[][]string{algorithmLabels, nil},
			Groups:		[]toolkit.SettingGroup{{Title: "Hashing Algorithm", Icon: "󰌆", Primary: 0, Children: []int{1}}},
		},
		width:	80, height: 24,
	}
	m.ws.SetStacked(true)
	m.recomputeText()
	m.resize()
	return m
}

func init()				{ toolkit.Register(New()) }
func (m *Model) ID() string		{ return "generators.hash" }
func (m *Model) Title() string		{ return "Hash and Checksum Generator" }
func (m *Model) ShortTitle() string	{ return "Hash / Checksum" }
func (m *Model) Category() string	{ return "generators" }
func (m *Model) Icon() string		{ return "󰌆" }
func (m *Model) Description() string	{ return "Generate and verify text or file hashes" }

// PaneStatusHints 供外壳状态栏合并展示本工具独有的键。
func (m *Model) PaneStatusHints() []ui.StatusHint {
	hints := m.ws.StatusHints()
	hints = append(hints, ui.StatusHint{Key: "ctrl+k", Desc: "key"}, ui.StatusHint{Key: "alt+v", Desc: "compare"})
	if m.running {
		hints = append(hints, ui.StatusHint{Key: "alt+x", Desc: "cancel"})
	}
	return hints
}
func (m *Model) Init() tea.Cmd		{ return nil }
func (m *Model) AcceptsFilePaths() bool	{ return true }

func (m *Model) buttons() []string {
	caseLabel := "Off"
	if m.uppercase {
		caseLabel = "On"
	}
	algorithm := algorithms[m.algorithm]
	if m.algorithm < len(algorithmLabels) {
		algorithm = algorithmLabels[m.algorithm]
	}
	return []string{algorithm, caseLabel}
}

// contentGap 返回设置条与内容区之间的空行数；极矮终端与桌面双栏布局
// 放弃空行，保证 View 总行数等于终端高度。
func (m *Model) contentGap() int	{ return m.gap }
func (m *Model) extraRows() (top, bottom int) {
	if m.desktopLayout() {
		// 桌面布局的 Compare 输入位于页脚行，不独占一行；
		// 比较结论按需占一行。
		if !m.header.Expanded(m.buttons()...) && m.height-m.headerHeight >= 10 {
			top = 1
		}
		if m.compare() != "" {
			return top, 1
		}
		return top, 0
	}
	bottom = 1
	if m.compare() != "" {
		bottom = 2
	}
	if m.header.Expanded(m.buttons()...) {
		if m.height-m.headerHeight >= 10 {
			return 0, bottom
		}
		if m.focus == 2 {
			return 0, 1
		}
		return 0, 0
	}
	if m.height-m.headerHeight >= 10 {
		return 1, bottom
	}
	if m.focus == 1 {
		return 1, 0
	}
	if m.focus == 2 {
		return 0, 1
	}
	return 0, 0
}
func (m *Model) resize() tea.Cmd {
	m.headerHeight = m.header.SetSize(tea.WindowSizeMsg{Width: m.width, Height: m.height}, m.buttons()...)
	if m.header.Expanded(m.buttons()...) {
		m.headerHeight += 2
	}
	m.gap = settingsContentGap
	if m.width >= 48 || m.height-m.headerHeight <= 5 {
		m.gap = 0
	}
	top, bottom := m.extraRows()
	m.workspaceHeight = max(1, m.height-m.headerHeight-m.gap-top-bottom)
	// key.View() 含「┃ 」提示符两列，宽度预算同步扣减，保证字段行
	// 总宽恰好铺满且右缘边框不被截断。
	m.key.Width = max(1, m.width-lipgloss.Width("  HMAC secret  ")-lipgloss.Width("┃ "))
	if m.desktopLayout() {
		// Compare 输入位于页脚行右段，宽度让出左侧提示。
		m.expected.Width = max(1, m.width-lipgloss.Width(hashCompareLabel)-2*ui.Space1-lipgloss.Width(hashFooterLeft(false))-2)
	} else {
		m.expected.Width = max(1, m.width-lipgloss.Width(hashCompareLabel)-2*ui.Space1)
	}
	cmd := m.ws.Update(tea.WindowSizeMsg{Width: m.width, Height: m.workspaceHeight})
	if m.desktopLayout() {
		m.resizeDesktop()
	}
	return cmd
}
func (m *Model) setFocus(focus int) {
	m.focus = focus
	m.key.Blur()
	m.expected.Blur()
	if focus != 0 {
		m.ws.Input.Blur()
	} else if m.ws.Focus == toolkit.FocusInput {
		m.ws.Input.FocusPreservingScroll()
	}
	if focus == 1 {
		m.key.Focus()
	}
	if focus == 2 {
		m.expected.Focus()
	}
	m.resize()
}
func (m *Model) invalidate() {
	m.revision++
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.running = false
}
func newHash(algorithm int, key string) hash.Hash {
	factories := []func() hash.Hash{md5.New, sha1.New, sha256.New, sha512.New384, sha512.New, func() hash.Hash { return crc32.NewIEEE() }}
	if key != "" {
		return hmac.New(factories[algorithm], []byte(key))
	}
	return factories[algorithm]()
}
func (m *Model) formatDigest(raw string) string {
	if m.uppercase {
		return strings.ToUpper(raw)
	}
	return strings.ToLower(raw)
}
func (m *Model) recomputeText() {
	m.invalidate()
	h := newHash(m.algorithm, m.key.Value())
	_, _ = io.WriteString(h, m.ws.Input.Value())
	m.ws.SetOutput(m.formatDigest(hex.EncodeToString(h.Sum(nil))))
	m.ws.SetError(nil)
}
func (m *Model) computeFile() tea.Cmd {
	m.invalidate()
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.ws.SetError(nil)
	m.ws.SetOutput("")
	m.ws.Output.SetContent("Calculating…")
	m.resize()
	revision, path, algorithm, key := m.revision, m.path, m.algorithm, m.key.Value()
	return func() tea.Msg {
		result := hashResult{revision: revision}
		info, err := os.Stat(path)
		if err == nil && !info.Mode().IsRegular() {
			err = fmt.Errorf("only regular files can be hashed")
		}
		var f *os.File
		if err == nil {
			f, err = os.Open(path)
		}
		if err == nil {
			defer f.Close()
			info, statErr := f.Stat()
			if statErr != nil {
				err = statErr
			} else if !info.Mode().IsRegular() {
				err = fmt.Errorf("only regular files can be hashed")
			}
		}
		if err == nil {
			h := newHash(algorithm, key)
			buf := make([]byte, 128*1024)
			for {
				if err = ctx.Err(); err != nil {
					break
				}
				n, readErr := f.Read(buf)
				if n > 0 {
					_, _ = h.Write(buf[:n])
				}
				if readErr == io.EOF {
					result.digest = hex.EncodeToString(h.Sum(nil))
					break
				}
				if readErr != nil {
					err = readErr
					break
				}
			}
		}
		result.err = err
		return toolkit.BackgroundToolMsg{Tool: m, Message: result}
	}
}
func (m *Model) recompute() tea.Cmd {
	if m.path != "" {
		return m.computeFile()
	}
	m.recomputeText()
	return m.resize()
}
func (m *Model) compare() string {
	if m.running || m.ws.OutputText() == "" || m.expected.Value() == "" {
		return ""
	}
	if strings.EqualFold(m.ws.OutputText(), m.expected.Value()) {
		return "✓ Checksums match"
	}
	return "✗ Checksums do not match"
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case hashResult:
		if event.revision == m.revision {
			m.running = false
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.ws.SetError(event.err)
			m.ws.SetOutput(m.formatDigest(event.digest))
			m.resize()
		}
		return m, nil
	case toolkit.FilePathImportMsg:
		m.path = event.Path
		m.ws.Input.SetValue("")
		return m, m.computeFile()
	case tea.WindowSizeMsg:
		m.width, m.height = event.Width, event.Height
		return m, m.resize()
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && m.header.Expanded(m.buttons()...) && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y == m.headerHeight-2 {
		m.setFocus(1)
		return m, nil
	}
	setting, choice, handled := m.header.Pick(msg, m.buttons()...)
	if handled && setting < 0 {
		// 算法下拉框展开/收起/选择期间，事件全部由浮层消费。
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+a":
			setting, choice, handled = m.header.Shortcut(0, m.buttons()...)
		case "alt+u":
			setting, choice, handled = 1, -1, true
		case "alt+x":
			if m.running {
				m.invalidate()
				m.ws.SetOutput("")
				m.ws.Output.SetContent("Cancelled")
				return m, m.resize()
			}
		case "ctrl+o":
			return m, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case "ctrl+k":
			m.setFocus(1)
			return m, nil
		case "ctrl+y":
			return m, m.ws.Update(key)
		case "alt+v":
			m.setFocus(2)
			return m, nil
		case "esc":
			if m.focus != 0 {
				m.setFocus(0)
				return m, nil
			}
		case "tab":
			if m.focus == 1 {
				m.setFocus(0)
				return m, nil
			}
			if m.focus == 2 {
				m.setFocus(1)
				return m, nil
			}
		}
	}
	if handled && setting < 0 {
		// alt+a 只是打开了算法下拉框，等待键盘或鼠标选择。
		return m, nil
	}
	if handled {
		switch {
		case setting == 0 && choice >= 0:
			m.algorithm = choice
			m.resize()
			return m, m.recompute()
		case setting == 0:
			// 紧凑布局的回退按钮条：点击即轮换算法。
			m.algorithm = (m.algorithm + 1) % len(algorithmLabels)
			m.resize()
			return m, m.recompute()
		case setting == 1:
			m.uppercase = !m.uppercase
			if !m.running {
				m.ws.SetOutput(m.formatDigest(m.ws.OutputText()))
			}
			m.resize()
			return m, nil
		}
	}
	top, bottom := m.extraRows()
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < m.headerHeight {
			return m, nil
		}
		y := mouse.Y - m.headerHeight - m.contentGap()
		if y < 0 {
			return m, nil
		}
		if mouse.Button == tea.MouseButtonLeft && mouse.Action == tea.MouseActionPress {
			if top > 0 && y == 0 {
				m.setFocus(1)
				return m, nil
			}
			if !m.desktopLayout() && bottom > 0 && y == top+m.workspaceHeight {
				m.setFocus(2)
				return m, nil
			}
			if y >= top && y < top+m.workspaceHeight {
				m.setFocus(0)
			}
		}
		if y < top || y >= top+m.workspaceHeight {
			return m, nil
		}
		mouse.Y -= m.headerHeight + m.contentGap() + top
		if m.desktopLayout() {
			return m, m.desktopMouse(mouse)
		}
		before := m.ws.Input.Value()
		cmd := m.ws.Update(mouse)
		if before != m.ws.Input.Value() {
			m.path = ""
			return m, tea.Batch(cmd, m.recompute())
		}
		return m, cmd
	}
	if m.focus != 0 {
		var cmd tea.Cmd
		if m.focus == 1 {
			before := m.key.Value()
			m.key, cmd = m.key.Update(msg)
			if before != m.key.Value() {
				cmd = tea.Batch(cmd, m.recompute())
			}
		} else {
			before := m.expected.Value()
			m.expected, cmd = m.expected.Update(msg)
			if before != m.expected.Value() {
				cmd = tea.Batch(cmd, m.resize())
			}
		}
		return m, cmd
	}
	before := m.ws.Input.Value()
	cmd := m.ws.Update(msg)
	if before != m.ws.Input.Value() {
		m.path = ""
		cmd = tea.Batch(cmd, m.recompute())
	}
	return m, cmd
}

// View 渲染设置条与工作区；算法下拉浮层叠加在完整视图之上。
func (m *Model) View() string {
	return m.header.Overlay(m.bodyView())
}

// HandlesEscape 算法下拉浮层展开时 esc 只收起浮层。
func (m *Model) HandlesEscape() bool	{ return m.header.MenuOpen() }

func (m *Model) bodyView() string {
	top, bottom := m.extraRows()
	header := m.header.View(m.buttons()...)
	if m.header.Expanded(m.buttons()...) {
		lines := strings.Split(header, "\n")
		if len(lines) > 0 {
			// 密钥字段并入设置列表：与上方设置行同一版式（左标签 + 右侧输入），
			// 描述行缩进对齐；不手绘边框（边框与内容宽度不同源会错位）。
			// 整行铺 Surface 底——textinput 分段 reset 会取消外层背景。
			border := lines[len(lines)-1]
			field := hashFieldRow("  HMAC secret", &m.key, m.focus == 1, m.width)
			lines[len(lines)-1] = field
			descText := "  Secret key to use to generate a HMAC hash"
			lines = append(lines, hashSurfaceRow(theme.MutedStyle().Render(descText), m.width))
			lines = append(lines, border)
			header = strings.Join(lines, "\n")
		}
	}
	view := header + strings.Repeat("\n", 1+m.contentGap())
	if m.focus != 0 && m.height-m.headerHeight < 6 {
		field := hashFieldRow("HMAC key", &m.key, m.focus == 1, m.width)
		if m.focus == 2 {
			field = hashFieldRow("Compare", &m.expected, true, m.width)
		}
		// 矮终端回退：字段行与余下空行整块铺 Surface 底，行数合同不变。
		rows := max(1, m.height-m.headerHeight)
		block := field
		for i := 1; i < rows; i++ {
			block += "\n" + hashSurfaceRow("", m.width)
		}
		return view + block
	}
	if top > 0 {
		view += hashFieldRow("HMAC key", &m.key, m.focus == 1, m.width) + "\n"
	}
	if m.desktopLayout() {
		view += m.desktopView()
	} else {
		view += m.ws.View()
	}
	if m.desktopLayout() {
		// Compare 输入在页脚行；这里只补比较结论行。
		if bottom > 0 {
			line := m.compare()
			if m.path != "" {
				line = "File: " + safeFileName(m.path) + "  " + line
			}
			view += "\n" + hashSurfaceRow(theme.LabelStyle().Render(line), m.width)
		}
		return view
	}
	if bottom > 0 {
		view += "\n"
		view += hashFieldRow("Compare", &m.expected, m.focus == 2, m.width)
	}
	if bottom > 1 {
		line := m.compare()
		if m.path != "" {
			line = "File: " + safeFileName(m.path) + "  " + line
		}
		view += "\n"
		view += hashSurfaceRow(theme.LabelStyle().Render(line), m.width)
	}
	return view
}

// hashLabelWidth 是哈希工具字段行的标签列宽：HMAC key / HMAC secret / Compare 共用，
// 保证字段输入框在不同的行里左缘对齐。
const hashLabelWidth = 16

// hashCompareLabel 是页脚比较输入的标签（"Compare" + 两列间距）；输入框宽度预算据此扣减。
const hashCompareLabel = "Compare  "

// compareSegment 渲染页脚右段的校验和输入：标签 + 定宽输入框（FieldView 左右各留 1 列）。
func (m *Model) compareSegment() string {
	return theme.LabelStyle().Render("Compare") + "  " + ui.FieldView(&m.expected, m.expected.Width+2*ui.Space1, m.focus == 2)
}

// hashFieldRow 渲染"标签 + 定宽输入框"的字段行（ui.FormRow + ui.FieldView），整行铺 Surface 底。
func hashFieldRow(label string, field *textinput.Model, focused bool, width int) string {
	control := ui.FieldView(field, max(1, width-hashLabelWidth-ui.Space2), focused)
	return hashSurfaceRow(ui.FormRow(label, hashLabelWidth, control, width), width)
}
