// Package xpath 实现 XPath 查询测试器（基于 antchfx/xmlquery）：
// 输入 XML 文档与 XPath 表达式，输出区实时列出全部匹配节点。
package xpath

import (
	"fmt"
	"strings"

	"github.com/toy-forge/devtoys/internal/theme"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// focusArea 表示工具内部焦点所在的区域（表达式 / XML 输入 / 输出）。
type focusArea int

const (
	focusExpr	focusArea	= iota
	focusXML
	focusOutput
)

var labelStyle = lipgloss.NewStyle().
	Foreground(theme.Muted)

type model struct {
	id, title, icon, desc	string

	ws	toolkit.Workspace	// 公共双栏工作区：XML 输入 + 输出
	expr	textinput.Model		// XPath 表达式输入（单行）
	focus	focusArea

	width	int
	height	int
}

// New 创建 XPath 测试器模型。
func New() *model {
	ws := toolkit.NewWorkspace(
		"Enter an XML document…",
		"Enter an XPath expression to search.",
	)
	expr := textinput.New()
	expr.Placeholder = `Example: //book[@price > 30]/title`
	expr.Focus()
	// 初始焦点在表达式输入框，取消 NewWorkspace 默认给主输入区的焦点，
	// 避免两个输入框同时持焦、显示双光标。
	ws.Input.Blur()

	return &model{
		id:	"testers.xpath",
		title:	"XPath",
		icon:	"󰍉",
		desc:	"Query XML with XPath",
		ws:	ws,
		expr:	expr,
		focus:	focusExpr,
		width:	80,
		height:	24,
	}
}

func (m *model) ID() string		{ return m.id }
func (m *model) Title() string		{ return m.title }
func (m *model) Category() string	{ return "testers" }
func (m *model) Icon() string		{ return m.icon }
func (m *model) Description() string	{ return m.desc }

func (m *model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

func (m *model) Init() tea.Cmd	{ return nil }

func (m *model) headerLines() int {
	if m.height <= 6 {
		return 1
	}
	return 2
}
func (m *model) expressionPrefix() int {
	if m.headerLines() != 1 {
		return 0
	}
	return exprLabelWidth + ui.Space2
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(toolkit.InputClearedMsg); ok {
		m.recompute()
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case toolkit.FileImportMsg:
		if m.focus == focusExpr {
			if err := toolkit.InsertLine(&m.expr, msg.Text); err != nil {
				m.ws.SetError(err)
				return m, nil
			}
		} else {
			m.focus = focusXML
			m.syncFocus()
			m.ws.Update(msg)
			if m.ws.Err != nil {
				return m, nil
			}
		}
		m.recompute()
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= 0 && msg.Y < m.headerLines() {
			m.focus = focusExpr
			m.syncFocus()
			if msg.Y == m.headerLines()-1 && msg.X >= m.expressionPrefix() {
				// 字段内容从 expressionPrefix 之后的 Space1 列开始。
				m.expr.SetCursor(msg.X - m.expressionPrefix() - ui.Space1)
			}
			return m, nil
		}
		if msg.Y >= m.headerLines() {
			cmd := m.ws.UpdateBelow(msg, m.headerLines())
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y < m.height-1 {
				m.focus = focusXML
				if m.ws.Focus == toolkit.FocusOutput {
					m.focus = focusOutput
				}
				m.syncFocus()
			}
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			// 焦点在三个区域之间循环：表达式 → XML → 输出。
			if msg.String() == "shift+tab" {
				m.focus = (m.focus + 2) % 3
			} else {
				m.focus = (m.focus + 1) % 3
			}
			m.syncFocus()
			return m, nil

		case "ctrl+y":
			m.ws.Update(msg)
			return m, nil
		}

		// 非公共按键：交给焦点所在区域的组件处理，随后即时重算。
		var cmd tea.Cmd
		switch m.focus {
		case focusExpr:
			m.expr, cmd = m.expr.Update(msg)
			m.recompute()
		case focusXML:
			cmd = m.ws.Update(msg)
			m.recompute()
		case focusOutput:
			cmd = m.ws.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// exprLabelWidth 是单行布局中 "Expression" 标签列的宽度。
const exprLabelWidth = 10

// expressionField 渲染表达式输入框：单行布局为"标签 + 定宽字段"（ui.FormRow），
// 两行布局为整行定宽字段（标签在上一行）。字段底色标明可编辑。
func (m *model) expressionField() string {
	focused := m.focus == focusExpr
	if m.headerLines() == 1 {
		return ui.FormRow("Expression", exprLabelWidth, ui.FieldView(&m.expr, max(1, m.width-m.expressionPrefix()), focused), m.width)
	}
	return ui.FieldView(&m.expr, max(1, m.width), focused)
}

func (m *model) View() string {
	if m.headerLines() == 1 {
		return m.expressionField() + "\n" + m.ws.View()
	}
	header := labelStyle.Render("XPath expression") + "\n" +
		m.expressionField() + "\n"
	return header + m.ws.View()
}

// syncFocus 让各输入组件的焦点状态与当前焦点区域一致，
// 同时把公共工作区的焦点同步到 XML 输入或输出。
func (m *model) syncFocus() {
	m.expr.Blur()
	m.ws.Input.Blur()
	switch m.focus {
	case focusExpr:
		m.expr.Focus()
		m.ws.Focus = toolkit.FocusInput
	case focusXML:
		m.ws.Input.FocusPreservingScroll()
		m.ws.Focus = toolkit.FocusInput
	case focusOutput:
		m.ws.Focus = toolkit.FocusOutput
	}
	if m.focus != focusOutput {
		m.ws.Copied = false
	}
}

func (m *model) resize() {
	m.expr.Width = max(1, m.width-m.expressionPrefix()-2*ui.Space1)
	// 剩余高度交给工作区。
	m.ws.Update(tea.WindowSizeMsg{
		Width:	m.width,
		Height:	max(1, m.height-m.headerLines()),
	})
}

// recompute 依据当前表达式与 XML 文档重算查询结果。
// 节点结果输出其 XML 片段；属性节点输出"名称=值"；
// count()/string() 等标量结果通过导航器求值后直接输出。
func (m *model) recompute() {
	src := strings.TrimSpace(m.expr.Value())
	doc := m.ws.Input.Value()

	m.ws.SetError(nil)
	if src == "" {
		m.ws.SetOutput("Enter an XPath expression to search.")
		return
	}
	if strings.TrimSpace(doc) == "" {
		m.ws.SetOutput("Enter an XML document.")
		return
	}

	root, err := xmlquery.Parse(strings.NewReader(doc))
	if err != nil {
		m.ws.SetError(fmt.Errorf("XML parse failed: %v", err))
		return
	}

	exp, err := xpath.Compile(src)
	if err != nil {
		m.ws.SetError(fmt.Errorf("invalid XPath expression: %v", err))
		return
	}

	var b strings.Builder
	switch res := exp.Evaluate(xmlquery.CreateXPathNavigator(root)).(type) {
	case *xpath.NodeIterator:
		// 节点集结果：逐个输出匹配节点。
		nodes := xmlquery.QuerySelectorAll(root, exp)
		if len(nodes) == 0 {
			b.WriteString("No results.")
			break
		}
		fmt.Fprintf(&b, "%d matching nodes:\n", len(nodes))
		for i, n := range nodes {
			fmt.Fprintf(&b, "\n[%d] ", i)
			switch n.Type {
			case xmlquery.AttributeNode:
				// 属性节点：输出 名称=值，避免把父元素整体展开。
				fmt.Fprintf(&b, "@%s = %s", n.Data, n.InnerText())
			default:
				b.WriteString(n.OutputXML(true))
			}
			b.WriteString("\n")
		}
	default:
		// 标量结果：count()/string()/boolean 等表达式。
		fmt.Fprintf(&b, "Scalar result: %v", res)
	}
	m.ws.SetOutput(b.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	toolkit.Register(New())
}
