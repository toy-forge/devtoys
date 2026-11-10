// Package markdown 提供 Markdown 渲染预览工具。
package markdown

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// Model 是 Markdown 预览工具，包装公共双栏工作区。
type Model struct {
	ws		toolkit.Workspace
	tr		*termRenderer	// 与当前渲染宽度绑定的渲染器
	wrapW		int		// 当前的 word wrap 宽度
	lastSrc		string		// 上次已渲染的原文，避免重复渲染
	light		bool
	showSrc		bool	// true 时输出区显示原文而非渲染结果
	header		toolkit.SettingHeader
	width, height	int
	revision	uint64
	running, queued	bool
}

// previewHint 是空输入时输出区展示的占位文案。
const previewHint = "Enter Markdown to see the live preview"

type renderDue struct{ revision uint64 }
type renderDone struct {
	revision	uint64
	output		string
	err		error
}

const backgroundRenderThreshold = 16 << 10

// init 注册 Markdown 预览工具。
func init() {
	m := &Model{
		ws:	toolkit.NewWorkspace("Enter Markdown…", previewHint),
		header: toolkit.SettingHeader{
			Names:		[]string{"Theme"},
			Descriptions:	[]string{"Select which theme to use to preview the Markdown"},
			Icons:		[]string{"󰏘"},
			Options:	[][]string{{"Dark", "Light"}},
		},
	}
	m.ws.SetPaneTitles("Markdown", "Preview")
	m.ws.SetInputControls("Copy")
	toolkit.Register(m)
}

func (m *Model) ID() string		{ return "text.markdown" }
func (m *Model) Title() string		{ return "Markdown Preview" }
func (m *Model) Category() string	{ return "text" }
func (m *Model) Icon() string		{ return "󰍔" }
func (m *Model) Description() string	{ return "Render a Markdown preview" }

func (m *Model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

// Init 满足 bubbletea 模型接口；无定时任务需要启动。
func (m *Model) Init() tea.Cmd	{ return nil }

func markdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	style.Document.Color = new(string(theme.Text))
	style.Document.Margin = new(uint(0))
	style.Heading.Color = new(string(theme.Blue))
	style.H1.Color = new(string(theme.CursorText))
	style.H1.BackgroundColor = new(string(theme.Accent))
	style.H6.Color = new(string(theme.BlueBright))
	style.HorizontalRule.Color = new(string(theme.Border))
	style.Link.Color = new(string(theme.Blue))
	style.LinkText.Color = new(string(theme.BlueBright))
	style.Image.Color = new(string(theme.BlueBright))
	style.ImageText.Color = new(string(theme.Muted))
	style.Code.Color = new(string(theme.Text))
	style.Code.BackgroundColor = new(string(theme.Surface))
	style.CodeBlock.Color = new(string(theme.Text))
	style.CodeBlock.Theme = "github-dark"
	style.CodeBlock.Chroma = nil
	return style
}

func lightMarkdownStyle() ansi.StyleConfig {
	style := styles.LightStyleConfig
	style.Document.Margin = new(uint(0))
	style.Document.BackgroundColor = new("#ffffff")
	style.CodeBlock.Theme = "github"
	style.CodeBlock.Chroma = nil
	return style
}

func previewDisplay(content string, light bool, width int) string {
	if !light {
		return content
	}
	return lipgloss.NewStyle().Background(lipgloss.Color("#ffffff")).Width(width).Render(content)
}

// rebuildRenderer 依据输出区宽度重建 glamour 渲染器。
// glamour 的 word wrap 与样式边距都在渲染器创建时固定，宽度变化必须重建。
func (m *Model) rebuildRenderer(w int) {
	if w <= 0 {
		w = 80
	}
	if m.tr != nil && w == m.wrapW {
		return
	}
	tr, err := newRenderer(w, m.light)
	if err != nil {
		m.tr = nil
		m.ws.SetError(fmt.Errorf("could not initialize renderer: %w", err))
		return
	}
	m.tr = tr
	m.wrapW = w
}

// render 渲染原文到输出区。force 为 true 时即使原文未变化也重新渲染
// （用于从"原文"视图切回"预览"视图）。
func (m *Model) render(src string, force bool) {
	if !force && src == m.lastSrc {
		return
	}
	m.lastSrc = src
	if src == "" {
		m.ws.SetError(nil)
		m.ws.SetOutput(previewHint)
		return
	}
	if m.showSrc {
		m.ws.SetError(nil)
		m.ws.SetOutput(src)
		return
	}
	if m.tr == nil {
		m.ws.SetError(errors.New("renderer is not ready"))
		return
	}
	out, err := m.tr.Render(src)
	if err != nil {
		m.ws.SetError(fmt.Errorf("markdown rendering failed: %w", err))
		m.ws.SetOutput("")
		return
	}
	m.ws.SetError(nil)
	out = strings.TrimRight(out, "\n")
	m.ws.SetOutputDisplay(out, previewDisplay(out, m.light, m.ws.Output.Width))
}

// Update 处理窗口尺寸、按键与输入变化：每次输入都即时重渲染。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case renderDue:
		if event.revision != m.revision {
			return m, nil
		}
		if m.running {
			m.queued = true
			return m, nil
		}
		return m, m.startRender(event.revision)
	case renderDone:
		m.running = false
		if event.revision == m.revision {
			m.ws.SetError(event.err)
			m.ws.SetOutputDisplay(event.output, previewDisplay(event.output, m.light, m.ws.Output.Width))
		}
		if m.queued {
			m.queued = false
			return m, m.startRender(m.revision)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = event.Width, event.Height
		cmd := m.header.Resize(event, &m.ws, m.values()...)
		m.rebuildRenderer(m.ws.Output.Width)
		return m, tea.Batch(cmd, m.scheduleOutput(true))
	}
	setting, choice, handled := m.header.Pick(msg, m.values()...)
	if handled && setting < 0 {
		// 主题下拉框展开/收起/选择期间，事件全部由浮层消费。
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+t":
			setting, choice, handled = m.header.Shortcut(0, m.values()...)
		case "ctrl+p":
			m.showSrc = !m.showSrc
			m.lastSrc = ""	// 强制重渲染
			return m, m.scheduleOutput(true)
		}
	}
	if handled && setting < 0 {
		// alt+t 只是打开了主题下拉框，等待键盘或鼠标选择。
		return m, nil
	}
	if handled && setting == 0 {
		// 选中下拉项设置主题；紧凑布局的回退按钮条与 alt+t 则切换。
		if choice >= 0 {
			m.light = choice == 1
		} else {
			m.light = !m.light
		}
		m.tr = nil
		m.rebuildRenderer(m.ws.Output.Width)
		if m.width > 0 {
			m.header.Relayout(&m.ws, m.values()...)
			m.rebuildRenderer(m.ws.Output.Width)
		}
		return m, m.scheduleOutput(true)
	}
	cmd := m.ws.UpdateBelow(msg, m.header.Height(m.values()...))
	return m, tea.Batch(cmd, m.scheduleOutput(false))
}

func (m *Model) scheduleOutput(force bool) tea.Cmd {
	src := m.ws.Input.Value()
	if !force && src == m.lastSrc {
		return nil
	}
	m.revision++
	m.queued = false
	if len(src) < backgroundRenderThreshold {
		m.syncOutput(force)
		return nil
	}
	m.lastSrc = src
	m.ws.SetError(nil)
	m.ws.SetOutput("")
	m.ws.Output.SetContent("Rendering preview…")
	revision := m.revision
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return toolkit.BackgroundToolMsg{Tool: m, Message: renderDue{revision: revision}}
	})
}

func (m *Model) startRender(revision uint64) tea.Cmd {
	m.running = true
	src, width, light := m.ws.Input.Value(), m.ws.Output.Width, m.light
	return func() tea.Msg {
		result := renderDone{revision: revision}
		renderer, err := newRenderer(max(1, width), light)
		if err == nil {
			result.output, err = renderer.Render(src)
		}
		result.output = strings.TrimRight(result.output, "\n")
		if err != nil {
			result.err = fmt.Errorf("markdown rendering failed: %w", err)
		}
		return toolkit.BackgroundToolMsg{Tool: m, Message: result}
	}
}

// syncOutput 根据当前视图模式刷新输出区内容。
func (m *Model) syncOutput(force bool) {
	src := m.ws.Input.Value()
	m.render(src, force)
}

// View 渲染公共双栏工作区。
func (m *Model) values() []string {
	previewTheme := "Dark"
	if m.light {
		previewTheme = "Light"
	}
	return []string{previewTheme}
}

// HandlesEscape 主题下拉浮层展开时 esc 只收起浮层。
func (m *Model) HandlesEscape() bool	{ return m.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (m *Model) View() string {
	return m.header.Overlay(m.bodyView())
}

func (m *Model) bodyView() string {
	return m.header.View(m.values()...) + "\n" + m.ws.View()
}
