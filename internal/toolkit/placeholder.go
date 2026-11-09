package toolkit

import tea "github.com/charmbracelet/bubbletea"

// Placeholder 是占位工具：带完整的元信息与公共双栏工作区，
// 输出区固定显示"Not implemented"。后续实现真实逻辑时，在对应工具包内
// 用具体实现替换 NewPlaceholder 即可，注册方式不变。
type Placeholder struct {
	id, title, category, icon, desc	string
	ws				Workspace
}

// NewPlaceholder 创建一个占位工具，供各工具包在 init() 中注册。
func NewPlaceholder(id, title, category, icon, desc string) *Placeholder {
	ws := NewWorkspace("Enter text here…", "Not implemented")
	return &Placeholder{
		id:		id,
		title:		title,
		category:	category,
		icon:		icon,
		desc:		desc,
		ws:		ws,
	}
}

func (p *Placeholder) ID() string		{ return p.id }
func (p *Placeholder) Title() string		{ return p.title }
func (p *Placeholder) Category() string		{ return p.category }
func (p *Placeholder) Icon() string		{ return p.icon }
func (p *Placeholder) Description() string	{ return p.desc }

func (p *Placeholder) Init() tea.Cmd	{ return nil }

func (p *Placeholder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := p.ws.Update(msg)
	return p, cmd
}

func (p *Placeholder) View() string	{ return p.ws.View() }
