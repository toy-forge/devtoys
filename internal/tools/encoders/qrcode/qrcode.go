// Package qrcode 提供文本生成二维码工具：
// 使用 skip2/go-qrcode 的 ToSmallString 在终端以半块字符渲染，
// 纠错等级 L/M/Q/H 可切换。
package qrcode

import (
	"fmt"
	"image"
	"strings"

	"github.com/makiuchi-d/gozxing"
	zxing "github.com/makiuchi-d/gozxing/qrcode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/skip2/go-qrcode"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/toolkit/imageview"
)

// level 表示二维码纠错等级，从低到高。
type level int

const (
	levelLow	level	= iota	// L：约 7% 可恢复
	levelMedium			// M：约 15% 可恢复
	levelHigh			// Q：约 25% 可恢复
	levelHighest			// H：约 30% 可恢复
)

// levelInfo 是每个等级的 go-qrcode 值与中文说明。
var levelInfo = [...]struct {
	value	qrcode.RecoveryLevel	// go-qrcode 的纠错等级
	short	string			// 等级缩写 L/M/Q/H
	abstain	string			// 可恢复比例说明
}{
	{qrcode.Low, "L", "about 7% recovery"},
	{qrcode.Medium, "M", "about 15% recovery"},
	{qrcode.High, "Q", "about 25% recovery"},
	{qrcode.Highest, "H", "about 30% recovery"},
}

// tool 是二维码生成工具模型，复用 toolkit.Workspace 双栏布局。
type tool struct {
	toolkit.Workspace
	header	toolkit.SettingHeader

	lv		level	// current error correction level
	qr		*qrcode.QRCode
	img		image.Image
	path		textinput.Model
	revision	uint64
	before		string
	width		int
	height		int
	exporting	bool
	saving		bool
	status		string
}

// newTool 创建工具实例并在 init 中注册，默认纠错等级 M。
func newTool() *tool {
	t := &tool{
		Workspace:	toolkit.NewWorkspace("Enter text or a URL for the QR code…", ""),
		header: toolkit.SettingHeader{
			Names:		[]string{"Error correction level"},
			Descriptions:	[]string{"Select how much of the code can be recovered if damaged"},
			Options:	[][]string{levelLabels()},
			Icons:		[]string{"󰄀"},
			Switches:	[]bool{false},
		},
		lv:	levelMedium,
		width:	80,
		height:	24,
	}
	t.SetPaneTitles("Text", "QR Code")
	t.SetSplitRatio(2, 1)
	t.SetInputControls("Copy")
	t.updateOutputControls()
	t.path = textinput.New()
	t.path.Prompt = "┃ "
	t.path.CharLimit = 4096
	t.path.Width = 78
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newTool()) }

func (t *tool) ID() string		{ return "encoders.qrcode" }
func (t *tool) Title() string		{ return "QR Code Encoder / Decoder" }
func (t *tool) ShortTitle() string	{ return "QR Code" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰄀" }
func (t *tool) Description() string	{ return "Generate a QR code or decode one from an image" }

func (t *tool) Init() tea.Cmd		{ return nil }
func (*tool) AcceptsImages() bool	{ return true }

// values 返回设置条展示的纠错等级标签。
func (t *tool) values() []string	{ return []string{levelLabel(t.lv)} }

// levelLabels 返回全部纠错等级的下拉选项（顺序与 levelInfo 一致）。
func levelLabels() []string {
	labels := make([]string, 0, len(levelInfo))
	for i := range levelInfo {
		labels = append(labels, levelLabel(level(i)))
	}
	return labels
}

// levelLabel 返回等级的显示文本，如 "Medium · 15%"：名称 + 可恢复比例。
func levelLabel(lv level) string {
	return levelNames[lv] + " · " + levelPercent[lv]
}

var (
	levelNames	= [...]string{"Low", "Medium", "Quartile", "High"}
	levelPercent	= [...]string{"7%", "15%", "25%", "30%"}
)

// relayoutHeader 在选项值变化后刷新设置条布局（与 encoders.base64 相同的
// 两步刷新：无副作用命中查询更新共享按钮条，再重排工作区）。
func (t *tool) relayoutHeader() {
	t.header.Index(tea.MouseMsg{Action: tea.MouseActionRelease}, t.values()...)
	t.header.Relayout(&t.Workspace, t.values()...)
}

// cycleLevel 切换纠错等级并重绘二维码。
func (t *tool) cycleLevel() {
	t.setLevel(level((int(t.lv) + 1) % len(levelInfo)))
}

// setLevel 设置纠错等级并重绘二维码。
func (t *tool) setLevel(lv level) {
	t.lv = lv
	t.relayoutHeader()
	t.recalc()
}

// HandlesEscape 纠错等级下拉浮层展开时 esc 只收起浮层。
func (t *tool) HandlesEscape() bool	{ return t.header.MenuOpen() }

type decodedImage struct {
	revision	uint64
	text		string
	err		error
}

// Update 处理本工具的选项快捷键，其余按键转发给公共工作区。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		if t.exporting {
			t.path.Width = max(1, event.Width-2)
			break
		}
		cmd := t.header.Resize(tea.WindowSizeMsg{Width: event.Width, Height: event.Height}, &t.Workspace, t.values()...)
		t.updateOutputControls()
		t.render()
		return t, cmd
	case exportDone:
		t.saving = false
		if event.err != nil {
			t.status = ""
			t.SetError(event.err)
		} else {
			t.status = "Saved " + event.path
			t.SetError(nil)
		}
		return t, nil
	case previewDone:
		if event.err != nil {
			t.SetError(event.err)
		}
		return t, nil
	case toolkit.InputClearedMsg:
		t.revision++
		t.before = t.Input.Value()
		t.recalc()
		return t, nil
	case toolkit.WorkspaceActionMsg:
		switch event.Action {
		case "Output:＋ Image", "Output:＋ Select image file":
			return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case "󰆾", "󰆾 Save SVG":
			return t, t.startExport()
		default:
			return t, nil
		}
	case toolkit.ImageImportMsg:
		if event.Image == nil {
			t.SetError(fmt.Errorf("invalid image"))
			return t, nil
		}
		t.revision++
		t.qr, t.img = nil, nil
		rev := t.revision
		t.SetError(nil)
		t.SetOutput("Decoding QR code…")
		return t, func() tea.Msg {
			text, err := scanImage(event.Image)
			return toolkit.BackgroundToolMsg{Tool: t, Message: decodedImage{revision: rev, text: text, err: err}}
		}
	case decodedImage:
		if event.revision != t.revision {
			return t, nil
		}
		if event.err != nil {
			t.SetError(event.err)
			t.SetOutput("")
			t.qr, t.img = nil, nil
			return t, nil
		}
		fresh := editor.New()
		if err := fresh.CheckInsert(event.text); err != nil {
			t.SetError(fmt.Errorf("QR content cannot be edited: %w", err))
			return t, nil
		}
		t.Input.SetValue(event.text)
		t.before = event.text
		t.recalc()
		return t, nil
	}

	if t.exporting {
		return t, t.updateExport(msg)
	}
	// 纠错等级下拉框：展开时浮层消费全部事件；选中即设置等级。
	setting, choice, handled := t.header.Pick(msg, t.values()...)
	if handled && setting < 0 {
		return t, nil
	}
	if handled && setting == 0 {
		if choice >= 0 {
			t.setLevel(level(choice))
		} else {
			// 紧凑布局的回退按钮条：点击即轮换 L→M→Q→H。
			t.cycleLevel()
		}
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+o":
			// 打开纠错等级下拉框；设置条不可展开（紧凑布局）时退回轮换。
			if s, _, _ := t.header.Shortcut(0, t.values()...); s == 0 {
				t.cycleLevel()
			}
			return t, nil
		case "ctrl+s":
			return t, t.startExport()
		case "alt+i":
			return t, t.nativePreview()
		}
	}
	cmd := t.Workspace.Update(msg)
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.Workspace.Err != nil {
		return t, cmd
	}
	if input := t.Input.Value(); input != t.before {
		t.before = input
		t.revision++
	}
	t.recalc()
	return t, cmd
}

// View 渲染顶部选项栏 + 双栏工作区。
func (t *tool) updateOutputControls() {
	selectImage := "＋ Select image file"
	if t.Output.Width < 30 {
		selectImage = "＋ Image"
		t.SetOutputControls("󰆾")
	} else {
		t.SetOutputControls("󰆾 Save SVG")
	}
	t.SetOutputAccessory(toolkit.Actions(selectImage), selectImage)
}
func (t *tool) View() string {
	if t.exporting {
		return t.exportView()
	}
	if t.status != "" {
		return toolkit.Actions(t.status) + "\n" + t.Workspace.View()
	}
	return t.header.Overlay(t.header.View(t.values()...) + "\n" + t.Workspace.View())
}

// recalc 依据当前输入即时重绘二维码，错误写入信息条。
func (t *tool) recalc() {
	in := t.Input.Value()
	if strings.TrimSpace(in) == "" {
		t.SetError(nil)
		t.qr, t.img = nil, nil
		t.SetOutputDisplay("", t.emptyMessage())
		return
	}
	t.qr, t.img = nil, nil

	code, err := qrcode.New(in, levelInfo[t.lv].value)
	if err != nil {
		t.SetError(fmt.Errorf("could not generate QR code: %v", err))
		t.SetOutput("")
		return
	}
	t.SetError(nil)
	t.qr = code
	t.img = code.Image(256)
	t.render()
}

// emptyMessage 返回按输出区宽度裁剪的空态提示：窄窗格以 … 结尾，
// 不留半截单词。文案本身取短句，常见宽度下能完整显示。
func (t *tool) emptyMessage() string {
	const message = "Enter text to generate a QR code."
	return ansi.Truncate(message, max(1, t.Output.Width), "…")
}

func (t *tool) render() {
	if t.qr == nil {
		t.SetOutputDisplay("", t.emptyMessage())
		return
	}
	preview := imageview.New(t.img, "QR code")
	preview.SetSize(max(1, t.Output.Width), max(3, t.Output.Height+2))
	lines := strings.Split(preview.Render(), "\n")
	display := ""
	if len(lines) > 2 {
		display = strings.Join(lines[1:len(lines)-1], "\n")
	}
	t.SetOutputDisplay("QR code", display)
}

func (t *tool) nativePreview() tea.Cmd {
	if t.img == nil {
		t.SetError(fmt.Errorf("enter text to generate a QR code first"))
		return nil
	}
	if !imageview.NativeAvailable() {
		t.SetError(fmt.Errorf("native preview requires Ghostty or Kitty outside tmux"))
		return nil
	}
	preview := imageview.New(t.img, "QR code")
	preview.SetSize(max(1, t.width), max(4, t.height))
	return preview.Native(func(err error) tea.Msg { return previewDone{err} })
}

func scanImage(img image.Image) (string, error) {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", fmt.Errorf("could not read image: %w", err)
	}
	result, err := zxing.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil {
		return "", fmt.Errorf("no readable QR code found: %w", err)
	}
	return result.GetText(), nil
}
