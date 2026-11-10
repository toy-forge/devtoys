// Package lorem 实现 Lorem Ipsum 占位文本生成工具：
// 单位（词/句子/段落）用 ← / → 切换，数量用 +/- 调整，
// alt+r 重新生成；输出按输出区宽度自动换行，便于浏览。
package lorem

import (
	"embed"
	"encoding/xml"
	"io/fs"
	"math/rand/v2"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
	"github.com/toy-forge/devtoys/internal/tools/generators/internal/optbar"
)

// Corpus data and its MIT license are copied from the DevToys C# implementation,
// which forked the NLipsum corpus by Alex Pendleton.
//
//go:embed corpora/*.xml
var corpusFiles embed.FS

var corpusLabels = []string{"Child Harold", "Decameron", "Faust", "In der Fremde", "Le Bateau Ivre", "Le Masque", "Lorem Ipsum", "Nagyon Fáj", "Ómagyar", "Robinsono Kruso", "The Raven", "Tierra y Luna"}
var corpusFilesByLabel = []string{"childharold.xml", "decameron.xml", "faust.xml", "inderfremde.xml", "lebateauivre.xml", "lemasque.xml", "loremipsum.xml", "nagyonfaj.xml", "omagyar.xml", "robinsonokruso.xml", "theraven.xml", "tierrayluna.xml"}

type lipsumCorpus struct {
	Title	string	`xml:"title"`
	Text	string	`xml:"text"`
}

const settingsContentGap = 1

func corpusText(index int) string {
	index = min(max(index, 0), len(corpusFilesByLabel)-1)
	data, err := fs.ReadFile(corpusFiles, "corpora/"+corpusFilesByLabel[index])
	if err != nil {
		return "lorem ipsum dolor sit amet"
	}
	var corpus lipsumCorpus
	if err := xml.Unmarshal(data, &corpus); err != nil {
		return "lorem ipsum dolor sit amet"
	}
	return corpus.Text
}

func corpusWords(index int) []string	{ return strings.Fields(corpusText(index)) }

// 单位枚举在选项中的下标。
const (
	unitWord	= iota
	unitSentence
	unitParagraph
)

// Model 是 Lorem Ipsum 生成工具的 bubbletea 模型。
type Model struct {
	ws		toolkit.Workspace
	bar		optbar.Bar
	width, height	int
	text		string
}

// New 创建模型并生成初始内容。
func New() *Model {
	m := &Model{
		ws:	toolkit.NewWorkspace("No input required", ""),
		bar: optbar.Bar{
			Choices: []*optbar.Choice{
				{Label: "Text corpus", Icon: "󰂺", Options: corpusLabels, Index: 6},
				{Label: "Type", Icon: "󰉢", Description: "Generate words, sentences or paragraphs of Lorem Ipsum", Options: []string{"Words", "Sentences", "Paragraphs"}, Index: unitParagraph},
			},
			Numbers:	[]*optbar.Number{{Label: "Length", Icon: "󰔟", Description: "Number of words, sentences or paragraphs to generate", Value: 1, Min: 1, Max: 5000}},
			Order: []optbar.OptionRef{
				{Kind: optbar.ChoiceOption, Index: 0},
				{Kind: optbar.ChoiceOption, Index: 1},
				{Kind: optbar.NumberOption, Index: 0},
			},
		},
	}
	m.ws.SetOutputOnly(true)
	m.ws.SetPaneTitles("", "Output")
	m.ws.SetOutputControls("Copy", "Refresh")
	m.regenerate()
	return m
}

func init()	{ toolkit.Register(New()) }

func (m *Model) ID() string		{ return "generators.lorem" }
func (m *Model) Title() string		{ return "Lorem Ipsum Generator" }
func (m *Model) ShortTitle() string	{ return "Lorem Ipsum" }
func (m *Model) Category() string	{ return "generators" }
func (m *Model) Icon() string		{ return "󰉶" }
func (m *Model) Description() string	{ return "Generate Lorem Ipsum placeholder text" }

func (m *Model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

// OutputOnly 报告本工具只有结果窗格（无输入焦点可切换）。
func (m *Model) OutputOnly() bool { return true }

func (m *Model) Init() tea.Cmd	{ return nil }

func (*Model) AcceptsFiles() bool	{ return false }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toolkit.WorkspaceActionMsg:
		switch msg.Action {
		case "Refresh":
			m.regenerate()
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
		m.render()
		return m, nil

	case toolkit.FileImportMsg:
		m.bar.Focused = false
		m.ws.Update(msg)
		return m, nil

	case tea.MouseMsg:
		if m.bar.OverlayActive() || (msg.Y >= 0 && msg.Y < m.bar.Height()) {
			changed, regenerate := m.bar.HandleMouse(msg)
			if changed || regenerate {
				m.regenerate()
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
		// ctrl+y 交给工作区复制输出。
		if key != "ctrl+y" {
			// alt+r 在任何状态下（包括选项栏聚焦时）都重新生成，
			// 必须先于 handleBarKey 判断，否则聚焦时按键被选项栏吞掉。
			if key == "alt+r" {
				m.regenerate()
				return m, nil
			}
			if handled, changed := m.handleBarKey(key); handled {
				if changed {
					m.regenerate()
					m.resizeWorkspace()
				}
				return m, nil
			}
		}
	}

	cmd := m.ws.UpdateBelow(msg, m.bar.Height()+settingsContentGap)
	return m, cmd
}

func (m *Model) resizeWorkspace() {
	if m.width > 0 {
		m.ws.Update(tea.WindowSizeMsg{Width: m.width, Height: max(1, m.height-m.bar.Height()-settingsContentGap)})
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

// regenerate 按当前单位与数量生成文本并刷新输出区。
func (m *Model) regenerate() {
	count := m.bar.Numbers[0].Value
	corpusIndex := m.bar.Choices[0].Index
	unit := m.bar.Choices[1].Index
	pool := corpusWords(corpusIndex)
	var text string
	switch unit {
	case unitParagraph:
		paras := make([]string, count)
		for i := range paras {
			paras[i] = paragraph(pool)
		}
		text = strings.Join(paras, "\n\n")
	case unitSentence:
		ss := make([]string, count)
		for i := range ss {
			ss[i] = sentence(pool)
		}
		text = strings.Join(ss, " ")
	default:
		text = strings.Join(randomWords(pool, count), " ")
	}
	if corpusIndex == 6 && (unit == unitParagraph || unit == unitSentence) {
		text = applyStartWords("Lorem ipsum dolor sit amet", text)
	}
	m.text = text
	m.render()
}

func (m *Model) render()	{ m.ws.SetOutputDisplay(m.text, wrap(m.text, m.ws.Output.Width-2)) }

// sentence 生成一个句子：5-15 个词，首字母大写，句号结尾。
func sentence(pool []string) string {
	s := strings.Join(randomWords(pool, 3+rand.IntN(18)), " ")
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes) + "."
}

// paragraph 生成一个段落：3-6 个句子。
func paragraph(pool []string) string {
	n := 3 + rand.IntN(18)
	ss := make([]string, n)
	for i := range ss {
		ss[i] = sentence(pool)
	}
	return strings.Join(ss, " ")
}

// randomWords 随机抽取 n 个词（可重复）。
func randomWords(pool []string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = pool[rand.IntN(len(pool))]
	}
	return out
}

func applyStartWords(start, text string) string {
	startWords, textWords := strings.Fields(start), strings.Split(text, " ")
	for i := range min(len(startWords), len(textWords)) {
		textWords[i] = startWords[i]
	}
	return strings.Join(textWords, " ")
}

// wrap 按显示宽度对文本做逐词换行；段落之间保留一个空行。
func wrap(s string, width int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	for _, para := range strings.Split(s, "\n\n") {
		line := ""
		for _, w := range strings.Fields(para) {
			switch {
			case line == "":
				line = w
			case lipgloss.Width(line)+1+lipgloss.Width(w) <= width:
				line += " " + w
			default:
				lines = append(lines, line)
				line = w
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
		// 段落之间补一个空行，末尾统一裁掉。
		lines = append(lines, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
