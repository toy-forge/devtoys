package toolkit

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/ui"
)

// FileImportMsg inserts text from the file picker into an editable field.
type FileImportMsg struct{ Text string }

// OpenFileRequestMsg asks the application shell to open its file picker.
type OpenFileRequestMsg struct{}

// InputClearedMsg tells a tool to refresh after the clear button.
type InputClearedMsg struct{}

// WorkspaceActionMsg reports a tool-specific action from the output pane toolbar.
type WorkspaceActionMsg struct{ Action string }

// Focus 表示 Workspace 中当前持有焦点的区域。
type Focus int

const (
	FocusInput	Focus	= iota	// 输入区
	FocusOutput			// 输出区
)

// 间距一律引用刻度：两栏之间 = Space3（章节缝，两块面板不贴边）、
// 行缝 = Space1（组内缝）、窗格与底部条之间 = Space1（底部条不贴面板边框）。
const workspaceColumnGap = ui.Space3
const workspaceRowGap = ui.Space1
const workspaceFooterGap = ui.Space1

// workspaceFooterMinHeight 是保留底部空行所需的最小总高度；更矮时取消空行，
// 保证极限尺寸下的行数合同不变。
const workspaceFooterMinHeight = 8

// WorkspaceMinHeight 是 Workspace.View 在任意分配高度下实际渲染的最小行数
// （紧凑页签布局：页签行 + 3 行窗格 + 底部条）。头部组件为内容区让行时
// 以此为保留下限，保证"总行数 == 终端高度"的布局合同成立。
const WorkspaceMinHeight = 5

// copiedToastTTL 是"已复制"通知的停留时长。
const copiedToastTTL = 2 * time.Second

// Workspace 是所有具体工具共用的双栏工作区组件：
// 左侧输入区（textarea）、右侧输出区（viewport，可滚动），
// 底部一行 ui.StatusBar（右侧 Copy/横向滚动按钮 + Toast）。
// 窗格使用 ui.Panel（焦点环 + 标题栏按钮），渲染与鼠标命中
// 共用 resize() 计算出的同一份矩形布局。
//
// 约定按键：tab/shift+tab 切换焦点，ctrl+y 复制输出到剪贴板，
// esc 返回工具列表（由上层 shell 处理，Workspace 不拦截）。
type Workspace struct {
	// Input 是输入区文本框。
	Input	textarea.Model
	// Output 是输出区视口（可滚动浏览长内容）。
	Output	viewport.Model

	outputText		string
	Focus			Focus
	Err			error
	Copied			bool
	inputTitle		string
	outputTitle		string
	InputControls		[]string
	OutputControls		[]string
	OutputControlsDisabled	bool
	inputAccessory		string
	outputAccessory		string
	outputAccessoryActions	[]string

	width			int
	height			int
	stacked			bool
	compact			bool
	forceStacked		bool
	outputOnly		bool
	outputExtendable	bool
	outputExpanded		bool
	preExpandFocus		Focus
	leftRatio		int
	rightRatio		int
	fixedOutputWidth	int
	stackedInputHeight	int
	stackedDetailsTitle	string
	stackedDetails		string
	stackedStatus		string
	stackedStatusLevel	StatusSeverity

	// ui 组件：窗格、底部状态条、状态条右端按钮组、紧凑模式页签。
	inputPane	*ui.Panel
	outputPane	*ui.Panel
	footer		*ui.StatusBar
	footerBar	*ui.Toolbar
	tabsBar		*ui.Tabs
	toast		ui.Toast
	hasToast	bool

	// 布局矩形（Workspace 局部坐标），渲染与命中共用。
	inputRect		ui.Rect
	outputRect		ui.Rect
	statusRect		ui.Rect
	detailsRect		ui.Rect
	inputAccessoryRect	ui.Rect
	outputAccessoryRect	ui.Rect
	tabsRect		ui.Rect
	footerRect		ui.Rect
	// footerGap 是正文与底部条之间的空行数（矮屏为 0）。
	footerGap	int
}

// NewWorkspace 创建工作区。inputPlaceholder 为输入区占位文案，
// output 为输出区初始内容。
func NewWorkspace(inputPlaceholder, output string) Workspace {
	in := textarea.New()
	in.Placeholder = inputPlaceholder
	in.ShowLineNumbers = false
	in.SetWidth(36)
	in.SetHeight(8)
	in.Focus()

	out := viewport.New(36, 8)
	out.SetContent(output)

	w := Workspace{
		Input:		in,
		Output:		out,
		outputText:	output,
		Focus:		FocusInput,
		inputTitle:	"Input",
		outputTitle:	"Output",
		width:		80,
		height:		24,
	}
	w.inputPane = ui.NewPanel(w.inputTitle)
	w.outputPane = ui.NewPanel(w.outputTitle)
	w.footer = ui.NewStatusBar()
	w.footerBar = ui.NewToolbar(
		ui.Button{Label: "Copy"},
		ui.Button{Label: "<"},
		ui.Button{Label: ">"},
	)
	// 紧凑模式页签：激活项由 ui.Tabs 的填充态表达。
	w.tabsBar = ui.NewTabs(w.inputTitle, w.outputTitle)
	w.resize()
	return w
}

// SetPaneTitles gives tools content-specific titles for the input and result panes.
func (w *Workspace) SetPaneTitles(input, output string) {
	if input != "" {
		w.inputTitle = input
	}
	if output != "" {
		w.outputTitle = output
	}
}

// SetInputAccessory places content above the input pane while leaving the
// output pane aligned with the top of the workspace.
func (w *Workspace) SetInputAccessory(content string) {
	w.inputAccessory = content
	w.resize()
}

func (w Workspace) inputAccessoryHeight() int {
	if w.inputAccessory == "" {
		return 0
	}
	return lipgloss.Height(w.inputAccessory)
}

func (w Workspace) inputAccessoryView(width int) string {
	lines := strings.Split(w.inputAccessory, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

// SetOutputAccessory places a row above the output pane, with optional clickable actions.
func (w *Workspace) SetOutputAccessory(content string, actions ...string) {
	w.outputAccessory = content
	w.outputAccessoryActions = append(w.outputAccessoryActions[:0], actions...)
	w.resize()
}

// SetInputControls replaces the default input pane actions with tool-specific
// controls. Clicking a custom control reports its name through WorkspaceActionMsg
// using the "Input:" prefix.
func (w *Workspace) SetInputControls(actions ...string) {
	w.InputControls = append(w.InputControls[:0], actions...)
}

// SetOutputControls adds tool-specific actions to the output pane title bar.
func (w *Workspace) SetOutputControls(actions ...string) {
	w.OutputControls = append(w.OutputControls[:0], actions...)
}

// SetOutputControlsDisabled keeps custom output controls visible but inactive.
func (w *Workspace) SetOutputControlsDisabled(disabled bool) {
	w.OutputControlsDisabled = disabled
}

// SetStacked selects the desktop-style vertical arrangement for tools whose
// original layout places the output below the input.
func (w *Workspace) SetStacked(stacked bool) {
	w.forceStacked = stacked
	w.resize()
}

// SetStackedInputHeight sets the editor height in rows for a stacked workspace.
// A zero value keeps the default even split between input and output.
func (w *Workspace) SetStackedInputHeight(rows int) {
	w.stackedInputHeight = max(0, rows)
	w.resize()
}

// SetStackedDetails inserts a read-only, titled section between the input and
// output panes in a stacked workspace. Empty content hides the section.
func (w *Workspace) SetStackedDetails(title, content string) {
	w.stackedDetailsTitle = title
	w.stackedDetails = content
	w.resize()
}

// SetStackedStatus places a one-line status between the input and details
// sections in a stacked workspace. Empty text hides the status row.
func (w *Workspace) SetStackedStatus(text string, severity StatusSeverity) {
	w.stackedStatus = text
	w.stackedStatusLevel = severity
	w.resize()
}

// SetSplitRatio adjusts the input and output pane widths when they are side by side.
func (w *Workspace) SetSplitRatio(left, right int) {
	w.fixedOutputWidth = 0
	if left <= 0 || right <= 0 {
		w.leftRatio, w.rightRatio = 0, 0
	} else {
		w.leftRatio, w.rightRatio = left, right
	}
	w.resize()
}

// SetFixedOutputWidth keeps the output pane at a fixed inner width. A value of
// zero restores proportional sizing.
func (w *Workspace) SetFixedOutputWidth(width int) {
	w.fixedOutputWidth = max(0, width)
	if w.fixedOutputWidth > 0 {
		w.leftRatio, w.rightRatio = 0, 0
	}
	w.resize()
}

// SetOutputOnly shows only the result pane for tools with no input.
func (w *Workspace) SetOutputOnly(enabled bool) {
	w.outputOnly = enabled
	if enabled {
		w.Focus = FocusOutput
		w.Input.Blur()
	} else {
		w.Focus = FocusInput
		w.Input.Focus()
	}
	w.resize()
}

// SetOutputExtendable enables an expanded view for the output pane.
func (w *Workspace) SetOutputExtendable(enabled bool) {
	w.outputExtendable = enabled
	if !enabled && w.outputExpanded {
		w.outputExpanded = false
		w.resize()
	}
}

func (w *Workspace) toggleOutputExpanded() {
	if !w.outputExtendable {
		return
	}
	if !w.outputExpanded {
		w.preExpandFocus = w.Focus
		w.outputExpanded = true
		w.Input.Blur()
		w.Focus = FocusOutput
	} else {
		w.outputExpanded = false
		if w.preExpandFocus == FocusInput && !w.outputOnly {
			w.Input.FocusPreservingScroll()
			w.Focus = FocusInput
		}
	}
	w.resize()
}

// SetOutput 更新输出区内容（供具体工具在处理输入后调用）。
func (w *Workspace) SetOutput(s string)	{ w.SetOutputDisplay(s, s) }

// SetOutputDisplay separates the rendered output from the text copied by users.
func (w *Workspace) SetOutputDisplay(text, display string) {
	w.outputText = text
	w.Output.SetContent(display)
}

// OutputText 返回当前输出区内容。
func (w *Workspace) OutputText() string	{ return w.outputText }

// SetError 设置/清除错误信息（非 nil 时底部状态条显示 error toast）。
func (w *Workspace) SetError(err error) {
	w.Err = err
	if err != nil {
		w.hasToast = false
	}
}

// CopyOutput 把输出区内容写入系统剪贴板。
// 部分工具的输出带有 ANSI 转义序列（着色的高亮/摘要等），
// 复制前剥离，避免剪贴板内容是带颜色码的乱码。
func (w *Workspace) CopyOutput() error {
	if err := clipboard.WriteAll(stripANSI(w.outputText)); err != nil {
		return fmt.Errorf("could not copy to clipboard: %w", err)
	}
	return nil
}

// ansiSeq 匹配 ANSI 转义序列：CSI 序列（如 SGR 着色 \x1b[31m）与 OSC 序列。
var ansiSeq = regexp.MustCompile("\x1b(?:\\[[0-9;:]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

// stripANSI 移除字符串中的 ANSI 转义序列，返回纯文本。
func stripANSI(s string) string {
	return ansiSeq.ReplaceAllString(s, "")
}

// UpdateBelow 将位于顶部内容下方的工作区鼠标坐标转换为局部坐标。
// 窗口尺寸由调用方按布局分配。
func (w *Workspace) UpdateBelow(msg tea.Msg, top int) tea.Cmd {
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < top {
			return nil
		}
		mouse.Y -= top
		msg = mouse
	}
	return w.Update(msg)
}

// copyToClipboard 复制输出并给出 success toast（带到期 Cmd）。
func (w *Workspace) copyToClipboard() tea.Cmd {
	if err := w.CopyOutput(); err != nil {
		w.Err, w.Copied, w.hasToast = err, false, false
		return nil
	}
	w.Err = nil
	w.Copied = true
	toast, cmd := ui.NewToast(ui.LevelSuccess, "Copied to clipboard", copiedToastTTL)
	w.toast, w.hasToast = toast, true
	return cmd
}

// copyInput 复制输入框内容并反馈到错误/复制状态。
func (w *Workspace) copyInput() tea.Cmd {
	if err := clipboard.WriteAll(w.Input.Value()); err != nil {
		w.Err, w.Copied, w.hasToast = fmt.Errorf("could not copy input: %w", err), false, false
		return nil
	}
	w.Err = nil
	w.Copied = true
	toast, cmd := ui.NewToast(ui.LevelSuccess, "Copied to clipboard", copiedToastTTL)
	w.toast, w.hasToast = toast, true
	return cmd
}

func (w *Workspace) toggleFocus() {
	if w.Focus == FocusInput {
		w.Focus = FocusOutput
		w.Input.Blur()
	} else {
		w.Focus = FocusInput
		w.Input.Focus()
	}
	if w.compact {
		// 紧凑布局只渲染焦点窗格，焦点切换后矩形布局随之移动。
		w.resize()
	}
}

// resize 依据可用宽高重新分配两栏尺寸，并生成渲染与命中共用的矩形布局。
func (w *Workspace) resize() {
	width, total := max(1, w.width), max(1, w.height)
	// 底部条之上保留一行空白：布局按扣除空白后的高度计算，View 再补回空行。
	w.footerGap = 0
	if total >= workspaceFooterMinHeight {
		w.footerGap = workspaceFooterGap
	}
	height := max(1, total-w.footerGap)
	w.inputRect, w.outputRect = ui.Rect{}, ui.Rect{}
	w.statusRect, w.detailsRect = ui.Rect{}, ui.Rect{}
	w.inputAccessoryRect, w.outputAccessoryRect = ui.Rect{}, ui.Rect{}
	w.tabsRect = ui.Rect{}
	w.footerRect = ui.Rect{X: 0, Y: total - 1, W: width, H: 1}

	if w.outputExpanded || w.outputOnly {
		w.stacked, w.compact = false, false
		w.outputRect = ui.Rect{X: 0, Y: 0, W: width, H: max(3, height-1)}
		// 宽度从 Panel.ContentWidth() 推导：文本区 = 总宽-4。
		w.outputPane.SetSize(w.outputRect.W, w.outputRect.H)
		w.Output.Width = max(1, w.outputPane.ContentWidth())
		w.Output.Height = max(1, w.outputRect.H-2)
		w.Output.SetYOffset(w.Output.YOffset)
		w.layoutPanes()
		return
	}

	// Narrow terminals show the input above the output, keeping both reachable.
	// 高度 ≤ 7 时放不下"双窗格+间隔+底部条"（至少 8 行），退到紧凑页签布局，
	// 保证"View 总行数 == 分配高度"的合同在 30x7/20x7 极限尺寸下成立。
	w.stacked = w.forceStacked || width < 40
	w.compact = w.stacked && height <= 7

	switch {
	case w.compact:
		w.resizeCompact(width, height)
	case w.stacked:
		w.resizeStacked(width, height)
	default:
		w.resizeSideBySide(width, height)
	}
	w.layoutPanes()
}

// resizeCompact 紧凑布局：页签行 +（可选）附件/状态行 + 单个焦点窗格 + 底部条。
func (w *Workspace) resizeCompact(width, height int) {
	w.tabsRect = ui.Rect{X: 0, Y: 0, W: width, H: 1}
	y := 1
	if w.Focus == FocusInput && w.inputAccessory != "" {
		w.inputAccessoryRect = ui.Rect{X: 0, Y: y, W: width, H: lipgloss.Height(w.inputAccessory)}
		y += w.inputAccessoryRect.H + workspaceRowGap
	}
	// 状态行只在"状态 + 间隔 + 3 行窗格 + 底部条"都放得下时渲染，
	// 否则省略（状态是瞬时提示，不能挤破总行数合同）。
	if w.stackedStatus != "" && height-(y+2) >= 4 {
		w.statusRect = ui.Rect{X: 0, Y: y, W: width, H: 1}
		y += w.statusRect.H + workspaceRowGap
	}
	if w.Focus == FocusOutput && w.outputAccessory != "" {
		w.outputAccessoryRect = ui.Rect{X: 0, Y: y, W: width, H: lipgloss.Height(w.outputAccessory)}
		y += w.outputAccessoryRect.H + workspaceRowGap
	}
	paneH := max(3, height-1-y)
	paneRect := ui.Rect{X: 0, Y: y, W: width, H: paneH}
	// 紧凑布局只渲染焦点窗格；宽度从 Panel.ContentWidth() 推导，
	// 先 SetSize 再取值，保证读取的是本帧的文本区宽度。
	if w.Focus == FocusInput {
		w.inputRect = paneRect
		w.inputPane.SetSize(paneRect.W, paneRect.H)
		w.Input.SetWidth(max(1, w.inputPane.ContentWidth()))
		w.Input.SetHeight(max(1, paneH-2))
	} else {
		w.outputRect = paneRect
		w.outputPane.SetSize(paneRect.W, paneRect.H)
		w.Output.Width = max(1, w.outputPane.ContentWidth())
		w.Output.Height = max(1, paneH-2)
	}
	w.Output.SetYOffset(w.Output.YOffset)
}

// resizeStacked 纵向布局：输入窗格、状态、详情、输出附件、输出窗格依次排列。
func (w *Workspace) resizeStacked(width, height int) {
	accH := w.inputAccessoryHeight()
	outAccH := 0
	if w.outputAccessory != "" {
		outAccH = lipgloss.Height(w.outputAccessory)
	}
	statusH := 0
	if w.stackedStatus != "" {
		statusH = 1
	}
	detailsH := 0
	if w.stackedDetails != "" {
		detailsH = 2	// 标题 + 内容
	}
	rows := 2	// 输入 + 输出窗格
	if accH > 0 {
		rows++
	}
	if statusH > 0 {
		rows++
	}
	if detailsH > 0 {
		rows++
	}
	if outAccH > 0 {
		rows++
	}
	gaps := rows - 1
	inner := height - 1 - 4 - accH - outAccH - statusH - detailsH - gaps	// 减窗格边框与底部条
	inner = max(2, inner)
	inputInner := max(1, inner/2)
	if w.stackedInputHeight > 0 {
		inputInner = min(w.stackedInputHeight, max(1, inner-1))
	}
	outputInner := max(1, inner-inputInner)

	y := 0
	if accH > 0 {
		w.inputAccessoryRect = ui.Rect{X: 0, Y: y, W: width, H: accH}
		y += accH + workspaceRowGap
	}
	w.inputRect = ui.Rect{X: 0, Y: y, W: width, H: inputInner + 2}
	y += w.inputRect.H + workspaceRowGap
	if statusH > 0 {
		w.statusRect = ui.Rect{X: 0, Y: y, W: width, H: statusH}
		y += statusH + workspaceRowGap
	}
	if detailsH > 0 {
		w.detailsRect = ui.Rect{X: 0, Y: y, W: width, H: detailsH}
		y += detailsH + workspaceRowGap
	}
	if outAccH > 0 {
		w.outputAccessoryRect = ui.Rect{X: 0, Y: y, W: width, H: outAccH}
		y += outAccH + workspaceRowGap
	}
	w.outputRect = ui.Rect{X: 0, Y: y, W: width, H: outputInner + 2}

	// 宽度从 Panel.ContentWidth() 推导：先 SetSize 再取值。
	w.inputPane.SetSize(w.inputRect.W, w.inputRect.H)
	w.Input.SetWidth(max(1, w.inputPane.ContentWidth()))
	w.Input.SetHeight(inputInner)
	w.outputPane.SetSize(w.outputRect.W, w.outputRect.H)
	w.Output.Width = max(1, w.outputPane.ContentWidth())
	w.Output.Height = outputInner
	w.Output.SetYOffset(w.Output.YOffset)
}

// resizeSideBySide 横向布局：左右双栏 + 各自可选附件行 + 底部条。
func (w *Workspace) resizeSideBySide(width, height int) {
	bodyH := max(3, height-1)	// 窗格外框高度
	// Each pane takes 4 chrome columns (2 border + 2 horizontal padding);
	// leave an explicit two-column gutter.
	available := max(16, width-workspaceColumnGap-8)
	leftInner, rightInner := available/2, available/2
	if w.fixedOutputWidth > 0 {
		rightInner = min(w.fixedOutputWidth, max(8, available-8))
		leftInner = available - rightInner
	} else if w.leftRatio > 0 && w.rightRatio > 0 {
		leftInner = available * w.leftRatio / (w.leftRatio + w.rightRatio)
		rightInner = available - leftInner
	}
	if leftInner < 8 {
		leftInner, rightInner = 8, max(1, available-8)
	}
	if rightInner < 8 {
		rightInner, leftInner = 8, max(1, available-8)
	}
	leftW, rightW := leftInner+4, rightInner+4
	outputX := leftW + workspaceColumnGap

	inputPaneH := bodyH
	if w.inputAccessory != "" {
		accH := lipgloss.Height(w.inputAccessory)
		w.inputAccessoryRect = ui.Rect{X: 0, Y: 0, W: leftW, H: accH}
		inputPaneH = max(3, bodyH-accH-workspaceRowGap)
	}
	w.inputRect = ui.Rect{X: 0, Y: 0, W: leftW, H: inputPaneH}
	if w.inputAccessoryRect.H > 0 {
		w.inputRect.Y = w.inputAccessoryRect.H + workspaceRowGap
	}

	outputPaneH := bodyH
	if w.outputAccessory != "" {
		accH := lipgloss.Height(w.outputAccessory)
		w.outputAccessoryRect = ui.Rect{X: outputX, Y: 0, W: rightW, H: accH}
		outputPaneH = max(3, bodyH-accH-workspaceRowGap)
		w.outputRect = ui.Rect{X: outputX, Y: w.outputAccessoryRect.H + workspaceRowGap, W: rightW, H: outputPaneH}
	} else {
		w.outputRect = ui.Rect{X: outputX, Y: 0, W: rightW, H: outputPaneH}
	}

	// 宽度从 Panel.ContentWidth() 推导：先 SetSize 再取值，
	// 保证 ContentWidth 反映本帧矩形而不是上一帧。
	w.inputPane.SetSize(w.inputRect.W, w.inputRect.H)
	w.Input.SetWidth(max(1, w.inputPane.ContentWidth()))
	w.Input.SetHeight(max(1, w.inputRect.H-2))
	w.outputPane.SetSize(w.outputRect.W, w.outputRect.H)
	w.Output.Width = max(1, w.outputPane.ContentWidth())
	w.Output.Height = max(1, w.outputRect.H-2)
	w.Output.SetYOffset(w.Output.YOffset)
}

// layoutPanes 把尺寸、标题、焦点与按钮同步到 ui.Panel / 状态条 / 页签条，
// 保证鼠标命中（ToolbarRect/Index）与渲染使用同一份布局。
func (w *Workspace) layoutPanes() {
	width := max(1, w.width)
	w.inputPane.SetTitle(w.inputTitle)
	w.inputPane.SetFocused(w.Focus == FocusInput && !w.outputOnly && !w.outputExpanded)
	w.inputPane.SetToolbar(paneButtons(w.inputControls(), false)...)
	w.outputPane.SetTitle(w.outputLabel())
	w.outputPane.SetFocused(w.Focus == FocusOutput || w.outputOnly || w.outputExpanded)
	w.outputPane.SetToolbar(paneButtons(w.outputControls(), w.OutputControlsDisabled)...)
	// 标题行放不下按钮时使用短标题。
	if len(w.OutputControls) > 0 && w.Output.Width < 30 && w.outputTitle == "Output" && w.outputPane.ToolbarRect().W == 0 {
		w.outputPane.SetTitle("Out")
	}
	if w.inputRect.W > 0 {
		w.inputPane.SetSize(w.inputRect.W, w.inputRect.H)
	}
	if w.outputRect.W > 0 {
		w.outputPane.SetSize(w.outputRect.W, w.outputRect.H)
	}
	w.tabsBar.Items = w.tabs()
	w.tabsBar.SetIndex(int(w.Focus))
	w.tabsBar.SetWidth(width)
	w.footer.SetWidth(width)
}

// paneButtons 把控件标签转换成内联按钮。
func paneButtons(controls []string, disabled bool) []ui.Button {
	buttons := make([]ui.Button, len(controls))
	for i, control := range controls {
		buttons[i] = ui.Button{Label: control, Disabled: disabled}
	}
	return buttons
}

// tabs 返回紧凑页签标签：激活项由 ui.Tabs 的填充态表达，标签本身不加前缀。
func (w *Workspace) tabs() []string {
	return []string{w.inputTitle, w.outputTitle}
}

// Update 处理窗口尺寸变化与按键。返回的 cmd 需由外层模型继续上报。
func (w *Workspace) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "alt+e" && w.outputExtendable {
		w.toggleOutputExpanded()
		return nil
	}
	switch msg := msg.(type) {
	case ui.ToastExpireMsg:
		if w.hasToast && w.toast.Expired(msg) {
			w.hasToast = false
			w.Copied = false
		}
		return nil
	case FileImportMsg:
		if w.outputOnly {
			return nil
		}
		if w.Focus != FocusInput {
			w.toggleFocus()
		} else {
			w.Input.Focus()
		}
		if err := InsertFileText(&w.Input, msg.Text); err != nil {
			w.Err = err
			w.hasToast = false
		} else {
			w.Err = nil
		}
		return nil
	case tea.WindowSizeMsg:
		w.width, w.height = msg.Width, msg.Height
		w.resize()
		return nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionMotion {
			// 应用层 hover 管线：指针位置换算成悬停下标写入按钮组，
			// 不派生命令、不新增消息类型。
			w.updateHover(msg)
			return nil
		}
		if msg.Action == tea.MouseActionPress {
			// 按下同时刷新悬停，保证按下目标与高亮一致。
			w.updateHover(msg)
		}
		if w.outputExpanded || w.outputOnly {
			if cmd, handled := w.handleOutputOnlyMouse(msg); handled {
				return cmd
			}
			return nil
		}
		if cmd, handled := w.handlePaneToolbar(msg); handled {
			return cmd
		}
		if w.compact && w.tabsRect.H > 0 && msg.Y == 0 {
			local := msg
			local.Y = 0
			switch w.tabsBar.Hit(local) {
			case 0:
				if w.Focus != FocusInput {
					w.toggleFocus()
				}
				return nil
			case 1:
				if w.Focus != FocusOutput {
					w.toggleFocus()
				}
				return nil
			}
		}
		if w.Err == nil && !w.hasToast && w.footerRect.H > 0 && msg.Y == w.footerRect.Y {
			if cmd, handled := w.handleFooterMouse(msg); handled {
				return cmd
			}
		}
		if msg.Y < 0 || msg.Y >= w.height-1 || msg.X < 0 || msg.X >= w.width {
			return nil
		}
		// 状态/详情只读行不响应点击与滚轮。
		if (w.statusRect.H > 0 && w.statusRect.Contains(msg.X, msg.Y)) ||
			(w.detailsRect.H > 0 && w.detailsRect.Contains(msg.X, msg.Y)) {
			return nil
		}
		outputHit := w.outputRect.W > 0 && w.outputRect.Contains(msg.X, msg.Y)
		if w.compact {
			outputHit = w.Focus == FocusOutput
		}
		if msg.Button == tea.MouseButtonLeft {
			if !outputHit && w.Focus != FocusInput {
				w.Focus = FocusInput
				w.Input.FocusPreservingScroll()
			}
			if outputHit && w.Focus != FocusOutput {
				w.toggleFocus()
			}
			if !outputHit && w.inputRect.W > 0 &&
				msg.X > w.inputRect.X && msg.X < w.inputRect.X+w.inputRect.W-2 &&
				msg.Y > w.inputRect.Y && msg.Y < w.inputRect.Y+w.inputRect.H-1 {
				// Panel 水平内边距 1 列/侧，文本起点 = 边框内 2 列；
				// Y 行起点 = 边框内 1 行，改成 -2 会触发 editor/mouse.go 的负值保护变无操作。
				return w.Input.PlaceCursor(msg.X-w.inputRect.X-2, msg.Y-w.inputRect.Y-1)
			}
		}
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			if outputHit {
				var cmd tea.Cmd
				w.Output, cmd = w.Output.Update(msg)
				return cmd
			}
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			w.Input.Scroll(delta)
		}
		return nil

	case tea.KeyMsg:
		// 复制成功后按任意键清除"已复制"提示。
		if w.Copied && msg.String() != "ctrl+y" {
			w.Copied = false
			w.hasToast = false
		}
		switch msg.String() {
		case "tab", "shift+tab":
			if w.outputOnly {
				return nil
			}
			w.toggleFocus()
			return nil
		case "ctrl+y":
			return w.copyToClipboard()
		}
		switch w.Focus {
		case FocusInput:
			var cmd tea.Cmd
			w.Input, cmd = w.Input.Update(msg)
			return cmd
		case FocusOutput:
			var cmd tea.Cmd
			w.Output, cmd = w.Output.Update(msg)
			return cmd
		}
	}
	return nil
}

// updateHover 把指针位置换算成悬停下标，写入窗格标题条与底部按钮组
// （应用层负责指针追踪，组件只消费字段渲染）。
// 命中换算复用 ToolbarRect/footerBarRect 的同一份矩形，禁止第二套算术。
func (w *Workspace) updateHover(mouse tea.MouseMsg) {
	if bar := w.inputPane.Toolbar(); bar != nil {
		bar.SetHover(-1)
		if tr := paneToolbarRect(w.inputPane); tr.W > 0 && mouse.Y == w.inputRect.Y &&
			mouse.X >= w.inputRect.X+tr.X && mouse.X < w.inputRect.X+tr.Right() {
			bar.SetHover(bar.HoverAt(mouse.X-w.inputRect.X-tr.X, 0))
		}
	}
	if bar := w.outputPane.Toolbar(); bar != nil {
		bar.SetHover(-1)
		if tr := paneToolbarRect(w.outputPane); tr.W > 0 && mouse.Y == w.outputRect.Y &&
			mouse.X >= w.outputRect.X+tr.X && mouse.X < w.outputRect.X+tr.Right() {
			bar.SetHover(bar.HoverAt(mouse.X-w.outputRect.X-tr.X, 0))
		}
	}
	w.footerBar.SetHover(-1)
	if r := w.footerBarRect(); r.W > 0 && mouse.Y == r.Y &&
		mouse.X >= r.X && mouse.X < r.Right() {
		w.footerBar.SetHover(w.footerBar.HoverAt(mouse.X-r.X, 0))
	}
}

// handleFooterMouse 处理底部状态条右端按钮（Copy / < / >）的点击。
func (w *Workspace) handleFooterMouse(mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.Button != tea.MouseButtonLeft {
		return nil, false
	}
	r := w.footerBarRect()
	if r.W == 0 || mouse.X < r.X || mouse.X >= r.X+r.W {
		return nil, false
	}
	local := mouse
	local.X -= r.X
	local.Y = 0
	switch w.footerBar.Index(local) {
	case 0:
		return w.copyToClipboard(), true
	case 1:
		w.Output.ScrollLeft(max(4, w.Output.Width/2))
		return nil, true
	case 2:
		w.Output.ScrollRight(max(4, w.Output.Width/2))
		return nil, true
	}
	return nil, false
}

// footerBarRect 返回状态条右端按钮组的矩形（Workspace 局部坐标）。
func (w *Workspace) footerBarRect() ui.Rect {
	if w.footerBar == nil || len(w.footerBar.Buttons) == 0 || w.height <= 0 {
		return ui.Rect{}
	}
	total := 0
	for i, b := range w.footerBar.Buttons {
		if i > 0 {
			total += max(0, w.footerBar.Gap)
		}
		total += b.Width()
	}
	x := w.width - 1 - total	// 状态条右侧留 1 列内边距
	if x < 0 {
		return ui.Rect{}
	}
	return ui.Rect{X: x, Y: w.height - 1, W: total, H: 1}
}

// paneToolbarRect 返回窗格标题行按钮区（窗格局部坐标）；空矩形表示放不下。
func paneToolbarRect(p *ui.Panel) ui.Rect {
	if p == nil || p.Toolbar() == nil {
		return ui.Rect{}
	}
	r := p.ToolbarRect()
	if r.W > 0 {
		p.Toolbar().SetSize(r.W, 1)
	}
	return r
}

// handlePaneToolbar 处理双窗格标题行按钮与输出附件行动作的点击。
func (w *Workspace) handlePaneToolbar(mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft {
		return nil, false
	}
	if w.outputAccessoryRect.H > 0 && len(w.outputAccessoryActions) > 0 && mouse.Y == w.outputAccessoryRect.Y {
		local := mouse
		local.X -= w.outputAccessoryRect.X
		local.Y = 0
		if action := ActionAt(local, w.outputAccessoryActions...); action >= 0 {
			selected := w.outputAccessoryActions[action]
			return func() tea.Msg { return WorkspaceActionMsg{Action: "Output:" + selected} }, true
		}
	}
	if cmd, handled := w.paneToolbarClick(w.inputPane, w.inputRect, mouse, true); handled {
		return cmd, true
	}
	if cmd, handled := w.paneToolbarClick(w.outputPane, w.outputRect, mouse, false); handled {
		return cmd, true
	}
	return nil, false
}

// paneToolbarClick 命中一个窗格标题行按钮并分发动作。
func (w *Workspace) paneToolbarClick(p *ui.Panel, rect ui.Rect, mouse tea.MouseMsg, input bool) (tea.Cmd, bool) {
	if p == nil || rect.W == 0 || mouse.Y != rect.Y || mouse.X < rect.X || mouse.X >= rect.X+rect.W {
		return nil, false
	}
	tr := paneToolbarRect(p)
	if tr.W == 0 {
		return nil, false
	}
	local := mouse
	local.X -= rect.X + tr.X
	local.Y = 0
	action := p.Toolbar().Index(local)
	if action < 0 {
		return nil, false
	}
	if input {
		return w.dispatchInputAction(action)
	}
	return w.dispatchOutputAction(action)
}

func (w *Workspace) dispatchInputAction(action int) (tea.Cmd, bool) {
	controls := w.inputControls()
	if len(w.InputControls) > 0 {
		if action < 0 || action >= len(controls) {
			return nil, false
		}
		selected := controls[action]
		if selected == "↶" && len(w.InputControls) > 0 {
			selected = w.InputControls[0]
		}
		if selected == "Copy" {
			return w.copyInput(), true
		}
		return func() tea.Msg { return WorkspaceActionMsg{Action: "Input:" + selected} }, true
	}
	switch action {
	case 0:
		if w.Focus != FocusInput {
			w.toggleFocus()
		}
		var cmd tea.Cmd
		w.Input, cmd = w.Input.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
		return cmd, true
	case 1:
		return func() tea.Msg { return OpenFileRequestMsg{} }, true
	case 2:
		if w.Focus != FocusInput {
			w.toggleFocus()
		}
		w.Input.SetValue("")
		w.Err = nil
		w.hasToast = false
		return func() tea.Msg { return InputClearedMsg{} }, true
	case 3:
		return w.copyInput(), true
	}
	return nil, false
}

func (w *Workspace) dispatchOutputAction(action int) (tea.Cmd, bool) {
	controls := w.outputControls()
	if action < 0 || action >= len(controls) {
		return nil, false
	}
	selected := controls[action]
	if w.outputExtendable && selected == paneExpandControl(w.outputExpanded) {
		w.toggleOutputExpanded()
		return nil, true
	}
	if len(w.OutputControls) > 0 {
		if w.OutputControlsDisabled {
			return nil, true
		}
		return func() tea.Msg { return WorkspaceActionMsg{Action: selected} }, true
	}
	if action == 0 {
		return w.copyToClipboard(), true
	}
	return nil, false
}

func (w *Workspace) handleOutputOnlyMouse(mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.X < 0 || mouse.X >= w.width || mouse.Y < 0 || mouse.Y >= w.height {
		return nil, false
	}
	if cmd, handled := w.paneToolbarClick(w.outputPane, w.outputRect, mouse, false); handled {
		return cmd, true
	}
	if w.Err == nil && !w.hasToast && w.footerRect.H > 0 && mouse.Y == w.footerRect.Y {
		if cmd, handled := w.handleFooterMouse(mouse); handled {
			return cmd, true
		}
	}
	if mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown {
		var cmd tea.Cmd
		w.Output, cmd = w.Output.Update(mouse)
		return cmd, true
	}
	return nil, false
}

// View 渲染双栏工作区与底部状态条。总行数严格等于分配的高度。
// joinFooter 在正文与底部条之间补回 footerGap 行空白，并拼上底部条。
func (w *Workspace) joinFooter(body string) string {
	return body + strings.Repeat("\n", w.footerGap+1) + w.footerView()
}

func (w *Workspace) View() string {
	if w.outputOnly || w.outputExpanded {
		output := w.renderOutputPane()
		return w.joinFooter(output)
	}
	if w.compact {
		rows := []string{ui.WithBackground(w.tabsBar.View(), theme.Background)}
		if w.inputAccessoryRect.H > 0 {
			rows = append(rows, w.inputAccessoryView(max(1, w.width)), "")
		}
		if w.statusRect.H > 0 {
			rows = append(rows, w.stackedStatusPane(), "")
		}
		if w.outputAccessoryRect.H > 0 {
			rows = append(rows, ansi.Truncate(w.outputAccessory, max(1, w.width), ""), "")
		}
		rows = append(rows, w.focusedPaneView())
		return w.joinFooter(strings.Join(rows, "\n"))
	}
	if w.stacked {
		rows := make([]string, 0, 9)
		appendRow := func(row string) {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, row)
		}
		if w.inputAccessoryRect.H > 0 {
			appendRow(w.inputAccessoryView(max(1, w.width)))
		}
		appendRow(w.renderInputPane())
		if w.statusRect.H > 0 {
			appendRow(w.stackedStatusPane())
		}
		if w.detailsRect.H > 0 {
			appendRow(w.stackedDetailsPane())
		}
		if w.outputAccessoryRect.H > 0 {
			appendRow(ansi.Truncate(w.outputAccessory, max(1, w.width), ""))
		}
		appendRow(w.renderOutputPane())
		return w.joinFooter(strings.Join(rows, "\n"))
	}
	inputColumn := w.renderInputPane()
	if w.inputAccessoryRect.H > 0 {
		inputColumn = lipgloss.JoinVertical(lipgloss.Left,
			w.inputAccessoryView(lipgloss.Width(w.renderInputPane())),
			"",
			inputColumn,
		)
	}
	outputColumn := w.renderOutputPane()
	if w.outputAccessoryRect.H > 0 {
		outputColumn = lipgloss.JoinVertical(lipgloss.Left,
			ansi.Truncate(w.outputAccessory, lipgloss.Width(w.renderOutputPane()), ""),
			"",
			outputColumn,
		)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, inputColumn, ui.CanvasGutter(workspaceColumnGap), outputColumn)
	return w.joinFooter(body)
}

func (w *Workspace) renderInputPane() string {
	return w.renderPane(w.inputPane, w.inputRect, w.inputTitle, w.Focus == FocusInput, w.inputControls(), false, w.Input.View())
}

func (w *Workspace) renderOutputPane() string {
	// 空态教用途：输出区没有任何可见内容时给一行弱化占位，而不是留一块
	// 空白面板。直接写 viewport 的加载态（Formatting…）不算空，原样显示。
	content := w.Output.View()
	if w.Err == nil && strings.TrimSpace(ansi.Strip(content)) == "" {
		text := ansi.Truncate("Output appears here.", max(1, w.outputPane.ContentWidth()), "…")
		content = theme.MutedStyle().Render(text)
	}
	// 焦点表达式与 layoutPanes 的 SetFocused 同源：同一时刻至多一个
	// 容器持有焦点环。
	return w.renderPane(w.outputPane, w.outputRect, w.outputLabel(), w.Focus == FocusOutput || w.outputOnly || w.outputExpanded, w.outputControls(), w.OutputControlsDisabled, content)
}

func (w *Workspace) focusedPaneView() string {
	if w.Focus == FocusOutput {
		return w.renderOutputPane()
	}
	return w.renderInputPane()
}

// renderPane 用 ui.Panel 渲染一个带标题与按钮的窗格。
func (w *Workspace) renderPane(p *ui.Panel, rect ui.Rect, title string, focused bool, controls []string, disabled bool, content string) string {
	if rect.W <= 0 {
		return ""
	}
	p.SetTitle(title)
	p.SetFocused(focused)
	p.SetToolbar(paneButtons(controls, disabled)...)
	view, _ := p.Render(content)
	return view
}

func (w *Workspace) stackedDetailsPane() string {
	if w.stackedDetails == "" {
		return ""
	}
	width := max(1, w.width)
	title := lipgloss.NewStyle().Foreground(theme.Muted).Bold(true).Render(w.stackedDetailsTitle)
	content := lipgloss.NewStyle().Foreground(theme.Text).Render(ansi.Truncate(w.stackedDetails, width, "…"))
	return title + "\n" + content
}

func (w *Workspace) stackedStatusPane() string {
	if w.stackedStatus == "" {
		return ""
	}
	return statusLine(w.stackedStatusLevel, w.stackedStatus)
}

// statusLine 用语义色图标渲染一行状态（Severity 顺序见 StatusSeverity）。
func statusLine(level StatusSeverity, text string) string {
	color := theme.Blue
	switch level {
	case StatusSuccess:
		color = theme.Success
	case StatusWarning:
		color = theme.Warning
	case StatusError:
		color = theme.Danger
	}
	line := lipgloss.NewStyle().Foreground(color).Render(theme.StatusIcon(uint8(level)) + " " + text)
	return ansi.Truncate(line, max(1, lipgloss.Width(line)), "")
}

func (w *Workspace) inputControls() []string {
	if len(w.InputControls) > 0 {
		if w.Input.Width() < 32 {
			return []string{"↶"}
		}
		controls := []string{"Copy"}
		for _, action := range w.InputControls {
			if action != "Copy" {
				controls = append(controls, action)
			}
		}
		return controls
	}
	return []string{"Paste", "Open", "Clear", "Copy"}
}

func (w *Workspace) outputLabel() string	{ return w.outputTitle }

func (w *Workspace) outputControls() []string {
	controls := []string{"Copy"}
	if len(w.OutputControls) > 0 {
		controls = append([]string(nil), w.OutputControls...)
	}
	if w.outputExtendable {
		controls = append(controls, paneExpandControl(w.outputExpanded))
	}
	return controls
}

// paneExpandControl 返回输出/窗格展开钮的文案：文字 label，
// 与 footer 提示 "alt+E Expand/Restore" 同词，收起态用 Restore 表达「还原布局」。
// 命中分发（dispatchOutputAction/threepane）以字符串相等比较该返回值，
// 改文案在此一处同步。
func paneExpandControl(expanded bool) string {
	if expanded {
		return "Restore"
	}
	return "Expand"
}

// StatusHints 供外壳状态栏合并展示；Workspace 底栏只保留操作钮，避免双行快捷键。
func (w Workspace) StatusHints() []ui.StatusHint {
	if w.outputOnly {
		return []ui.StatusHint{{Key: "scroll", Desc: "output"}}
	}
	hints := []ui.StatusHint{
		{Key: "click", Desc: "edit"},
		{Key: "scroll", Desc: "output"},
	}
	if w.outputExtendable {
		desc := "expand"
		if w.outputExpanded {
			desc = "restore"
		}
		hints = append(hints, ui.StatusHint{Key: "alt+e", Desc: desc})
	}
	return hints
}

// OutputOnly 报告工作区是否只有输出窗格（无输入可聚焦）。
func (w Workspace) OutputOnly() bool { return w.outputOnly }

// footerView 渲染底部单行状态条：右侧 Copy / 翻页；快捷键由 app.statusHints 展示。
func (w *Workspace) footerView() string {
	w.footer.SetWidth(max(1, w.width))
	w.footer.SetLeft("")
	w.footer.SetLeftTokens(nil)
	w.footer.SetHints()
	switch {
	case w.Err != nil:
		w.footer.SetToast(ui.Toast{Level: ui.LevelError, Message: "Error: " + w.Err.Error()})
	case w.hasToast:
		w.footer.SetToast(w.toast)
	default:
		w.footer.ClearToast()
		w.footer.SetRight(w.footerBar.View())
	}
	return w.footer.View()
}
