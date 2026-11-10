// Package uuid 实现批量 UUIDv4 生成工具：
// 数量用 +/- 调整，大写与去连字符为开关；增减数量时保留已生成的
// 结果，只追加或截断；大写/去连字符仅做格式转换，不重新生成。
package uuid

import (
	"fmt"
	"strings"

	googleuuid "github.com/google/uuid"
	"uuid"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
	"github.com/toy-forge/devtoys/internal/tools/generators/internal/optbar"
)

// Model 是 UUID 生成工具的 bubbletea 模型。
type Model struct {
	ws		toolkit.Workspace
	bar		optbar.Bar
	width, height	int

	// uuids 保存已生成的原始（小写、带连字符）UUID。
	uuids			[]string
	generatedVersion	int
}

// New 创建模型并生成初始数量的 UUID。
func New() *Model {
	m := &Model{
		ws:	toolkit.NewWorkspace("No input required", ""),
		bar: optbar.Bar{
			Numbers:	[]*optbar.Number{{Label: "UUID Generation Count", Icon: "󰔟", Description: "Number of UUIDs to generate", Value: 5, Min: 1, Max: 10000}},
			Choices:	[]*optbar.Choice{{Label: "UUID version", Icon: "󰒓", Description: "Choose the version of UUID to generate", Options: []string{"1", "4 (GUID)", "7"}, Index: 1}},
			Toggles: []*optbar.Toggle{
				{Label: "Uppercase", Icon: "󰬴"},
				{Label: "Hyphens", Icon: "󰌾", On: true},
			},
			Order: []optbar.OptionRef{
				{Kind: optbar.ToggleOption, Index: 0},
				{Kind: optbar.ToggleOption, Index: 1},
				{Kind: optbar.ChoiceOption, Index: 0},
				{Kind: optbar.NumberOption, Index: 0},
			},
		},
	}
	m.ws.SetPaneTitles("", "Output")
	m.ws.SetOutputControls("Copy", "Regenerate")
	m.ws.SetOutputOnly(true)
	m.syncCount()
	return m
}

func init()	{ toolkit.Register(New()) }

func (m *Model) ID() string		{ return "generators.uuid" }
func (m *Model) Title() string		{ return "UUID Generator" }
func (m *Model) ShortTitle() string	{ return "UUID" }
func (m *Model) Category() string	{ return "generators" }
func (m *Model) Icon() string		{ return "󰌆" }
func (m *Model) Description() string	{ return "Generate UUIDs in batches" }

func (m *Model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

func (m *Model) Init() tea.Cmd	{ return nil }

func (*Model) AcceptsFiles() bool	{ return false }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toolkit.WorkspaceActionMsg:
		switch msg.Action {
		case "Regenerate":
			m.uuids = nil
			m.syncCount()
		case "Copy":
			if err := m.ws.CopyOutput(); err != nil {
				m.ws.SetError(err)
			} else {
				m.ws.SetError(nil)
				m.ws.Copied = true
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.bar.SetSize(msg.Width, msg.Height)
		m.resizeWorkspace()
		return m, nil

	case toolkit.FileImportMsg:
		m.bar.Focused = false
		m.ws.Update(msg)
		return m, nil

	case tea.MouseMsg:
		if m.bar.OverlayActive() || (msg.Y >= 0 && msg.Y < m.bar.Height()) {
			oldVersion := m.bar.Choices[0].Index
			changed, regenerate := m.bar.HandleMouse(msg)
			if regenerate || oldVersion != m.bar.Choices[0].Index {
				m.uuids = nil
			}
			if changed || regenerate {
				m.syncCount()
				m.resizeWorkspace()
			}
			return m, nil
		}
		if m.bar.Focused && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.bar.Focused = false
			if m.ws.Focus == toolkit.FocusInput {
				m.ws.Input.Focus()
			}
		}

	case tea.KeyMsg:
		key := msg.String()
		// ctrl+y 交给工作区复制完整输出。
		if key != "ctrl+y" {
			// alt+r 在任何状态下（包括选项栏聚焦时）都重新生成，
			// 必须先于 handleBarKey 判断，否则聚焦时按键被选项栏吞掉。
			if key == "alt+r" {
				m.uuids = nil
				m.syncCount()
				return m, nil
			}
			if handled, changed := m.handleBarKey(key); handled {
				if changed {
					if m.generatedVersion != m.bar.Choices[0].Index {
						m.uuids = nil
					}
					m.syncCount()
					m.resizeWorkspace()
				}
				return m, nil
			}
		}
	}

	cmd := m.ws.UpdateBelow(msg, m.bar.Height()+1)
	return m, cmd
}

func (m *Model) resizeWorkspace() {
	if m.width > 0 {
		m.ws.Update(tea.WindowSizeMsg{Width: m.width, Height: max(1, m.height-m.bar.Height()-1)})
	}
}

func (m *Model) View() string {
	return m.bar.OverlayOn(m.bar.View() + "\n\n" + m.ws.View())
}

// handleBarKey 处理选项栏聚焦/取值按键；handled 表示按键已被
// 选项栏消费，不再进入工作区。进入聚焦时失焦输入区，退出时恢复。
func (m *Model) handleBarKey(key string) (handled, changed bool) {
	was := m.bar.Focused
	changed = m.bar.HandleKey(key)
	switch {
	case !was && m.bar.Focused:
		m.ws.Input.Blur()
	case was && !m.bar.Focused:
		m.ws.Focus = toolkit.FocusOutput
	}
	return was || m.bar.HandlesKeys(), changed
}

// syncCount 按"Count"选项补足或截断已生成的 UUID 并刷新输出。
func (m *Model) syncCount() {
	n := m.bar.Numbers[0].Value
	for len(m.uuids) < n {
		value, err := m.newUUID()
		if err != nil {
			m.ws.SetError(err)
			return
		}
		m.uuids = append(m.uuids, value)
	}
	m.uuids = m.uuids[:n]
	m.generatedVersion = m.bar.Choices[0].Index
	m.ws.SetError(nil)
	m.render()
}

func (m *Model) newUUID() (string, error) {
	switch m.bar.Choices[0].Index {
	case 0:
		value, err := googleuuid.NewUUID()
		if err != nil {
			return "", fmt.Errorf("generate UUID v1: %w", err)
		}
		return value.String(), nil
	case 1:
		return uuid.NewV4().String(), nil
	case 2:
		return uuid.NewV7().String(), nil
	default:
		return "", fmt.Errorf("unsupported UUID version")
	}
}

// render 按大写/去连字符开关格式化并展示全部 UUID。
func (m *Model) render() {
	upper := m.bar.Toggles[0].On
	noHyphen := !m.bar.Toggles[1].On
	out := make([]string, len(m.uuids))
	for i, u := range m.uuids {
		s := u
		if noHyphen {
			s = strings.ReplaceAll(s, "-", "")
		}
		if upper {
			s = strings.ToUpper(s)
		}
		out[i] = s
	}
	m.ws.SetOutput(strings.Join(out, "\n"))
}
