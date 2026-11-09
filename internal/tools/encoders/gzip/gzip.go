// Package gzip 提供 文本 ⇄ GZip ⇄ Base64 转换工具：
// 压缩方向把文本经 GZip 压缩后输出 Base64；解压方向先做
// Base64 还原再解压，并对每一步失败给出友好提示。
package gzip

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

// tool 是 GZip 工具模型，复用 toolkit.Workspace 双栏布局。
type tool struct {
	toolkit.Workspace
	header	toolkit.SettingHeader

	compress	bool	// true=文本→GZip(Base64)，false=GZip(Base64)→文本
	ratio		float64
	width		int
	height		int
	// showRatio 报告压缩率行是否有落脚之处（内容区之外还有 ≥2 行余量）。
	showRatio	bool
}

// newTool 创建工具实例并在 init 中注册。
func newTool() *tool {
	t := &tool{
		header: toolkit.SettingHeader{
			ContentGap:	1,
			Names:		[]string{"Compression mode"},
			Descriptions:	[]string{"Select whether the input should be compressed or decompressed"},
			Icons:		[]string{"󰦝"},
			Switches:	[]bool{true},
		},
		Workspace:	toolkit.NewWorkspace("Enter text or Base64-encoded GZip…", ""),
		compress:	true,
	}
	t.SetStacked(true)
	t.Workspace.SetOutputExtendable(true)
	t.recalc()
	return t
}

func init()	{ toolkit.Register(newTool()) }

func (t *tool) ID() string		{ return "encoders.gzip" }
func (t *tool) Title() string		{ return "GZip Compress / Decompress" }
func (t *tool) Category() string	{ return "encoders" }
func (t *tool) Icon() string		{ return "󰀼" }
func (t *tool) Description() string	{ return "Convert text to and from GZip in Base64" }

func (t *tool) Init() tea.Cmd	{ return nil }

// Update 处理本工具的选项快捷键，其余按键转发给公共工作区。
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.header.Index(msg, t.action()) == 0 {
		msg = tea.KeyMsg{Type: tea.KeyCtrlT}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+t":	// 切换 压缩 ⇄ 解压
			t.compress = !t.compress
			t.Input.SetValue(t.OutputText())
			t.resize()
			t.recalc()
			return t, nil
		}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		t.width, t.height = sz.Width, sz.Height
		return t, t.resize()
	}
	cmd := t.Workspace.UpdateBelow(msg, t.header.Height(t.action()))
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.Workspace.Err != nil {
		return t, cmd
	}
	t.recalc()
	return t, cmd
}

// View 渲染顶部选项栏 + 双栏工作区。
func (t *tool) action() string {
	if t.compress {
		return "Compress"
	}
	return "Decompress"
}
func (t *tool) View() string {
	workspace := t.Workspace.View()
	if t.showRatio {
		ratio := lipgloss.NewStyle().Foreground(theme.BlueBright).Background(theme.Surface).Width(max(1, t.width-2)).Padding(0, 1).Render("Compression ratio  " + ratioPercent(t.ratio))
		if footer := strings.LastIndexByte(workspace, '\n'); footer >= 0 {
			workspace = workspace[:footer] + "\n\n" + ratio + "\n" + workspace[footer+1:]
		} else {
			workspace += "\n" + ratio
		}
	}
	return t.header.View(t.action()) + "\n" + workspace
}

func (t *tool) resize() tea.Cmd {
	rows := t.header.SetSize(tea.WindowSizeMsg{Width: t.width, Height: t.height}, t.action())
	// The ratio bar takes one row between the workspace panes and keyboard hints;
	// terminals too short to fit it keep the workspace at its full share.
	avail := max(1, t.height-rows)
	t.showRatio = avail >= toolkit.WorkspaceMinHeight+2
	if t.showRatio {
		avail -= 2
	}
	return t.Workspace.Update(tea.WindowSizeMsg{Width: t.width, Height: avail})
}

// recalc 依据当前输入与方向即时重算输出，错误写入信息条。
func (t *tool) recalc() {
	in := t.Input.Value()
	if strings.TrimSpace(in) == "" {
		t.ratio = 0
		t.SetError(nil)
		t.SetOutput("")
		return
	}

	if t.compress {
		t.SetError(nil)
		out := compressToBase64(in)
		t.ratio = compressionDifference(in, out)
		t.SetOutput(out)
		return
	}

	decoded, err := decompressFromBase64(in)
	if err != nil {
		t.ratio = 0
		t.SetError(err)
		t.SetOutput("")
		return
	}
	t.ratio = compressionDifference(string(decoded), in)
	t.SetError(nil)
	t.SetOutput(string(decoded))
}

func compressionDifference(original, converted string) float64 {
	originalLength := len(utf16.Encode([]rune(original)))
	if originalLength == 0 {
		return 0
	}
	convertedLength := len(utf16.Encode([]rune(converted)))
	return float64(originalLength-convertedLength) / float64(originalLength) * 100
}

func ratioPercent(ratio float64) string {
	value := strings.TrimSuffix(fmt.Sprintf("%.1f", ratio), ".0")
	return value + " %"
}

// compressToBase64 把文本 GZip 压缩后编码为标准 Base64。
func compressToBase64(text string) string {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// gzip.Writer 写入内存缓冲不会失败，错误可安全忽略。
	_, _ = w.Write([]byte(text))
	_ = w.Close()
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// decompressFromBase64 先做 Base64 还原再解压，逐步给出友好错误。
func decompressFromBase64(in string) ([]byte, error) {
	// 去掉空白（多行粘贴的 Base64 很常见）。
	compact := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, in)

	raw, err := decodeBase64(compact)
	if err != nil {
		return nil, fmt.Errorf("invalid Base64; check that the complete compressed output was copied")
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return nil, fmt.Errorf("not GZip data (missing 1f 8b signature)")
	}

	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid GZip header: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("corrupt or incomplete GZip data: %v", err)
	}
	return out, nil
}

// decodeBase64 依次尝试标准与 URL-safe、带/不带 padding 的变体。
func decodeBase64(s string) ([]byte, error) {
	encs := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encs {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("invalid Base64")
}
