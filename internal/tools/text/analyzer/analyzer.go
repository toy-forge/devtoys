// Package analyzer 提供文本统计工具：
// 对输入文本即时统计字符数、不含空格字符数、词数、行数、段落数、
// 句子数与预估阅读时长，并列出 Top 10 高频词。
package analyzer

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/toy-forge/devtoys/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// 阅读速度假设：英文按词、中文按字分别估算。
const (
	latinWPM	= 200	// 每分钟阅读的英文词数
	cjkCPM		= 300	// 每分钟阅读的中文字数
)

// emptyHint 输入为空时展示在输出区的引导文案。
const emptyHint = "Enter text to see live statistics."

var (
	labelStyle	= lipgloss.NewStyle().Foreground(theme.Text)
	numberStyle	= lipgloss.NewStyle().Foreground(theme.Green)
	// 分组标题走标题样式（Text 粗体）：Accent 只留给可交互控件的选中/焦点态。
	titleStyle	= theme.TitleStyle()
)

// Model 是文本统计工具，包装公共双栏工作区。
type Model struct {
	ws		toolkit.Workspace
	headers		[3]toolkit.ActionHeader
	all		toolkit.ActionHeader	// 紧凑布局：全部操作合并为一组分页按钮
	height		int
	last		string
	original	string
}

type operationSection struct {
	title		string
	first, end	int
}

var operationSections = [...]operationSection{
	{title: "Convert line break", first: 0, end: 2},
	{title: "Convert case", first: 2, end: 16},
	{title: "Sort lines", first: 16, end: 22},
}

// init 注册文本统计工具。
func init() {
	m := &Model{
		ws: toolkit.NewWorkspace("Enter text to analyze…", emptyHint),
	}
	m.ws.SetPaneTitles("Text input", "Statistics")
	m.ws.SetInputControls("Original")
	m.ws.SetOutputExtendable(true)
	// The C# page fixes the statistics pane at 250 px; 31 terminal columns
	// approximate that width while leaving the remaining space for the editor.
	m.ws.SetFixedOutputWidth(31)
	m.report(analyze(""))
	toolkit.Register(m)
}

func (m *Model) ID() string		{ return "text.analyzer" }
func (m *Model) Title() string		{ return "Text Analyser and Utilities" }
func (m *Model) ShortTitle() string	{ return "Analyser & Utilities" }
func (m *Model) Category() string	{ return "text" }
func (m *Model) Icon() string		{ return "󰈙" }
func (m *Model) Description() string	{ return "Count characters, words and lines" }

func (m *Model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

// Init 满足 bubbletea 模型接口。
func (m *Model) Init() tea.Cmd	{ return nil }

// Update 处理窗口尺寸与按键：输入区每有按键都即时重算统计。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = size.Height
		for i, section := range operationSections {
			m.headers[i].SetSize(size, operationLabels[section.first:section.end]...)
		}
		m.all.SetSize(size, operationLabels...)
		m.ws.SetFixedOutputWidth(31)
		return m, m.ws.Update(tea.WindowSizeMsg{Width: size.Width, Height: max(1, size.Height-m.controlsHeight()-m.controlsGap())})
	}
	if event, ok := msg.(toolkit.WorkspaceActionMsg); ok && event.Action == "Input:Original" {
		m.applyOperation(len(operationLabels) - 1)
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && mouse.Y >= 0 && mouse.Y < m.controlsHeight() {
		if action := m.operationAt(mouse); action >= 0 {
			m.applyOperation(action)
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "alt+o" {
		m.applyOperation(len(operationLabels) - 1)
		return m, nil
	}
	cmd := m.ws.UpdateBelow(msg, m.controlsHeight()+m.controlsGap())
	text := m.ws.Input.Value()
	if text != m.last {
		m.last = text
		m.original = text
		m.report(analyze(text))
	}
	return m, cmd
}

// View renders the C# operation groups above the input and statistics panes.
func (m *Model) View() string {
	return m.controlsView() + strings.Repeat("\n", m.controlsGap()+1) + m.ws.View()
}

// controlsGap 是操作区与工作区之间的空行数：工作区放得下最小高度时才留白，
// 极矮终端取消留白，保证"总行数 == 终端高度"的合同。
func (m *Model) controlsGap() int {
	if m.height-m.controlsHeight()-ui.Space1 >= toolkit.WorkspaceMinHeight {
		return ui.Space1
	}
	return 0
}

// fullControlsHeight 是分组布局下操作区占用的高度：
// 每组 1 行标题 + 组间空行 + 各组按钮行数。
func (m *Model) fullControlsHeight() int {
	height := 2*len(operationSections) - 1
	for i, section := range operationSections {
		height += m.headers[i].Height(operationLabels[section.first:section.end]...)
	}
	return height
}

// compactControls 报告分组操作区放不下、需要合并为单组分页按钮。
func (m *Model) compactControls() bool {
	return m.fullControlsHeight()+3 > m.height
}

func (m *Model) controlsHeight() int {
	if m.compactControls() {
		return m.all.Height(operationLabels...)
	}
	return m.fullControlsHeight()
}

func (m *Model) controlsView() string {
	if m.compactControls() {
		// 极矮终端：合并全部操作（含 Original）为一组分页按钮。
		return m.all.View(operationLabels...)
	}
	var lines []string
	for i, section := range operationSections {
		lines = append(lines, titleStyle.Render(section.title))
		lines = append(lines, m.headers[i].View(operationLabels[section.first:section.end]...))
		if i < len(operationSections)-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) operationAt(mouse tea.MouseMsg) int {
	if m.compactControls() {
		if index := m.all.Index(mouse, operationLabels...); index >= 0 {
			return index
		}
		return -1
	}
	row := 0
	for i, section := range operationSections {
		row++	// section label
		height := m.headers[i].Height(operationLabels[section.first:section.end]...)
		if mouse.Y >= row && mouse.Y < row+height {
			mouse.Y -= row
			if index := m.headers[i].Index(mouse, operationLabels[section.first:section.end]...); index >= 0 {
				return section.first + index
			}
			return -1
		}
		row += height
		if i < len(operationSections)-1 {
			row++
		}
	}
	return -1
}

// report 把统计结果渲染进输出区：统计表（标题 / 数值，数值右对齐）、词频表、字符频次表。
func (m *Model) report(s stats) {
	var b strings.Builder

	section := titleStyle
	num := numberStyle

	labels := []string{"Bytes", "Characters", "Words", "Sentences", "Paragraphs", "Lines", "Line break"}
	if m.ws.Output.Width < 28 {
		labels = []string{"Bytes", "Chars", "Words", "Sents", "Paras", "Lines", "Line break"}
	}
	values := []string{
		strconv.Itoa(s.bytes),
		strconv.Itoa(s.chars),
		fmt.Sprint(s.words),
		fmt.Sprint(s.sentences),
		fmt.Sprint(s.paragraphs),
		fmt.Sprint(s.lines),
		s.lineEndings,
	}
	width := max(1, m.ws.Output.Width)

	// 统计表：标签列取最长标签宽度，数值列占满剩余宽度并右对齐。
	labelWidth := ui.LabelWidth(0, labels...)
	stats := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
		{Title: "Title", Width: labelWidth},
		{Title: "Value", Width: max(1, width-labelWidth-ui.Space2), Right: true},
	}}
	b.WriteString(section.Render("Text") + "\n")
	b.WriteString(stats.Header() + "\n")
	for i, label := range labels {
		// 零值用 Muted 与非零值区分。
		valueStyle := num
		if values[i] == "0" {
			valueStyle = labelStyle
		}
		_, line := stats.Row(labelStyle.Render(label), valueStyle.Render(values[i]))
		b.WriteString(line + "\n")
	}

	if len(s.top) > 0 {
		b.WriteString("\n" + section.Render("Word frequency") + "\n")
		// 词频表：序号（右对齐）/ 词 / 次数（右对齐）。
		count := 6
		freq := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
			{Width: 3, Right: true},
			{Width: max(1, width-3-count-2*ui.Space2)},
			{Width: count, Right: true},
		}}
		for i, e := range s.top {
			_, line := freq.Row(fmt.Sprintf("%d.", i+1), e.word, fmt.Sprintf("×%d", e.count))
			b.WriteString(line + "\n")
		}
	}
	if len(s.characters) > 0 {
		characterTitle := "Character frequency"
		if m.ws.Output.Width < 28 {
			characterTitle = "Char frequency"
		}
		b.WriteString("\n" + section.Render(characterTitle) + "\n")
		chars := ui.Table{Gap: ui.Space2, Columns: []ui.Column{{Width: 6}, {Width: 6, Right: true}}}
		for _, c := range s.characters {
			label := string(c.char)
			if c.char == ' ' {
				label = "⎵"
			} else if unicode.IsControl(c.char) {
				label = strconv.QuoteRune(c.char)
			}
			_, line := chars.Row(label, fmt.Sprintf("×%d", c.count))
			b.WriteString(line + "\n")
		}
	}

	m.ws.SetError(nil)
	m.ws.SetOutput(strings.TrimRight(b.String(), "\n"))
}

// stats 是一次统计的结果集合。
type stats struct {
	empty		bool
	bytes		int
	lineEndings	string
	chars		int	// UTF-16 code units, as in the desktop application
	charsNoSpace	int	// UTF-16 code units excluding whitespace
	words		int	// 词数（英文按词，中日韩按字）
	lines		int	// 行数
	paragraphs	int	// 段落数（以空行分隔的文本块）
	sentences	int	// 句子数
	readingTime	string	// 预估阅读时长
	top		[]wordCount
	characters	[]charCount
}

type charCount struct {
	char	rune
	count	int
}

// wordCount 是高频词榜单条目。
type wordCount struct {
	word	string
	count	int
}

// analyze 对文本执行全部统计。
func analyze(text string) stats {
	s := stats{empty: text == "", lines: 1, paragraphs: 1, lineEndings: "Unknown", readingTime: "—"}
	if s.empty {
		return s
	}
	s.bytes = len(text)
	s.lineEndings = detectLineEndings(text)

	// CR, LF, and CRLF each separate lines, including a final empty line.
	s.lines = 1 + strings.Count(normalizeLines(text), "\n")

	// 段落数：以空行（仅含空白字符的行）分隔的非空文本块数。
	s.paragraphs = countParagraphs(text)

	s.chars = 0
	s.charsNoSpace = 0
	s.words = 0
	s.sentences = 0
	latinWords := 0
	cjkChars := 0
	freq := map[string]int{}
	charFreq := map[rune]int{}

	var buf strings.Builder	// 当前正在累积的英文/数字词
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		latinWords++
		freq[buf.String()]++
		buf.Reset()
	}

	var sentenceBuf strings.Builder
	endSentence := func() {
		if strings.TrimSpace(sentenceBuf.String()) != "" {
			s.sentences++
		}
		sentenceBuf.Reset()
	}

	for _, r := range text {
		charFreq[r]++
		s.chars += utf16.RuneLen(r)
		if !unicode.IsSpace(r) {
			s.charsNoSpace += utf16.RuneLen(r)
		}

		// 分词：中日韩字符按字计词，字母/数字累积成词，其余字符断词。
		switch {
		case isCJK(r):
			flush()
			s.words++
			cjkChars++
			w := string(unicode.ToLower(r))
			freq[w]++
			sentenceBuf.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			buf.WriteRune(unicode.ToLower(r))
			sentenceBuf.WriteRune(r)
		default:
			flush()
			if isSentenceEnd(r) {
				endSentence()
			}
		}
	}
	flush()
	// 末尾没有断句符的残余内容也算一句。
	endSentence()

	s.words += latinWords

	// 预估阅读时长：英文与中文分别按速度折算后求和。
	minutes := float64(latinWords)/latinWPM + float64(cjkChars)/cjkCPM
	switch {
	case minutes <= 0:
		s.readingTime = "—"
	case minutes < 1:
		s.readingTime = "Under 1 minute"
	default:
		s.readingTime = fmt.Sprintf("About %d minutes", int64(minutes+0.5))
	}

	s.top = topWords(freq, len(freq))
	for r, count := range charFreq {
		s.characters = append(s.characters, charCount{r, count})
	}
	slices.SortFunc(s.characters, func(a, b charCount) int {
		if c := cmp.Compare(b.count, a.count); c != 0 {
			return c
		}
		return cmp.Compare(a.char, b.char)
	})
	return s
}

// isCJK 判断是否为中日韩表意文字（按字计词）。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isSentenceEnd 判断 rune 是否为句末标点。
func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？', '…':
		return true
	}
	return false
}

// countParagraphs 统计以空行分隔的非空文本块数。
func countParagraphs(text string) int {
	count, inPara := 0, false
	for _, line := range strings.Split(normalizeLines(text), "\n") {
		blank := strings.TrimSpace(line) == ""
		if !blank && !inPara {
			count++
		}
		inPara = !blank
	}
	return count
}

// topWords 从词频表中取出现次数最多的 n 个词，
// 按次数降序排列，同频按字典序排列。
func topWords(freq map[string]int, n int) []wordCount {
	list := make([]wordCount, 0, len(freq))
	for w, c := range freq {
		list = append(list, wordCount{word: w, count: c})
	}
	slices.SortFunc(list, func(a, b wordCount) int {
		if c := cmp.Compare(b.count, a.count); c != 0 {
			return c
		}
		return cmp.Compare(a.word, b.word)
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}
