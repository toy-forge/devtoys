package base64

import (
	"encoding/base64"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// recalcOf 构造工具实例，按给定方向计算输入并返回输出与错误。
func recalcOf(t *testing.T, in string, encode, urlSafe, pad bool) (string, error) {
	t.Helper()
	tl := newTool()
	tl.encode = encode
	tl.urlSafe = urlSafe
	tl.pad = pad
	tl.Input.SetValue(in)
	tl.recalc()
	return tl.OutputText(), tl.Err
}

// TestEncodeTypical 用典型文本验证四种编码选项组合。
func TestEncodeTypical(t *testing.T) {
	const in = "Hello, 世界"
	cases := []struct {
		name	string
		urlSafe	bool
		pad	bool
		want	string
	}{
		{"标准+padding", false, true, base64.StdEncoding.EncodeToString([]byte(in))},
		{"标准+无padding", false, false, base64.RawStdEncoding.EncodeToString([]byte(in))},
		{"URLsafe+padding", true, true, base64.URLEncoding.EncodeToString([]byte(in))},
		{"URLsafe+无padding", true, false, base64.RawURLEncoding.EncodeToString([]byte(in))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := recalcOf(t, in, true, c.urlSafe, c.pad)
			if err != nil {
				t.Fatalf("编码不应报错，得到: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestDecodeTypical 验证标准与 URL-safe 字母表均可解码。
func TestDecodeTypical(t *testing.T) {
	got, err := recalcOf(t, "SGVsbG8=", false, false, true)
	if err != nil {
		t.Fatalf("解码不应报错: %v", err)
	}
	if got != "Hello" {
		t.Fatalf("got %q want %q", got, "Hello")
	}
	// URL-safe 字母表（含 - _）且无 padding 也能被宽容解码。
	raw := base64.RawURLEncoding.EncodeToString([]byte("??~~"))
	got, err = recalcOf(t, raw, false, false, true)
	if err != nil {
		t.Fatalf("URL-safe 解码不应报错: %v", err)
	}
	if got != "??~~" {
		t.Fatalf("got %q want %q", got, "??~~")
	}
}

// TestDecodeWithWhitespace 多行粘贴的 Base64（夹带空白）应可解码。
func TestDecodeWithWhitespace(t *testing.T) {
	got, err := recalcOf(t, "SGVs\nbG8=", false, false, true)
	if err != nil {
		t.Fatalf("带空白的解码不应报错: %v", err)
	}
	if got != "Hello" {
		t.Fatalf("got %q want %q", got, "Hello")
	}
}

// TestEmptyInput 空输入应清空输出且不报错。
func TestEmptyInput(t *testing.T) {
	for _, encode := range []bool{true, false} {
		got, err := recalcOf(t, "", encode, false, true)
		if err != nil {
			t.Fatalf("空输入不应报错（encode=%v）: %v", encode, err)
		}
		if got != "" {
			t.Fatalf("空输入应输出空串，得到 %q", got)
		}
	}
}

// TestDecodeInvalid 非法输入应返回友好的中文错误。
func TestDecodeInvalid(t *testing.T) {
	got, err := recalcOf(t, "这不是Base64!!!", false, false, true)
	if err == nil {
		t.Fatalf("非法输入应报错，输出为 %q", got)
	}
	if !strings.Contains(err.Error(), "invalid Base64") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
	if got != "" {
		t.Fatalf("出错时输出应为空，得到 %q", got)
	}
}

// TestDecodeBase64Variants 直接验证 decodeBase64 对四种变体的尝试顺序。
func TestDecodeBase64Variants(t *testing.T) {
	for _, s := range []string{
		base64.StdEncoding.EncodeToString([]byte("x")),
		base64.URLEncoding.EncodeToString([]byte("x")),
		base64.RawStdEncoding.EncodeToString([]byte("x")),
		base64.RawURLEncoding.EncodeToString([]byte("x")),
	} {
		if _, err := decodeBase64(s); err != nil {
			t.Fatalf("decodeBase64(%q) 不应报错: %v", s, err)
		}
	}
	if _, err := decodeBase64("@@@"); err == nil {
		t.Fatal("decodeBase64(@@@) 应报错")
	}
}

// TestStripSpace 验证空白字符被全部移除。
func TestStripSpace(t *testing.T) {
	if got := stripSpace("a b\tc\nd\r\n e"); got != "abcde" {
		t.Fatalf("got %q want %q", got, "abcde")
	}
}

func TestMouseDirectionSwitch(t *testing.T) {
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	tool.Input.SetValue("YQ==")
	tool.Update(tea.MouseMsg{X: 1, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if tool.encode || tool.OutputText() != "a" {
		t.Fatalf("encode=%v output=%q", tool.encode, tool.OutputText())
	}
}

func TestImportRejectsChangedContentWithoutClearingError(t *testing.T) {
	tool := newTool()
	tool.Update(toolkit.FileImportMsg{Text: "a\x1bb"})
	if tool.Err == nil || tool.Input.Value() != "" {
		t.Fatalf("error=%v input=%q", tool.Err, tool.Input.Value())
	}
}

func TestWrappedSettingsRemainClickableAfterRelayout(t *testing.T) {
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 15, Height: 16})
	// 设置值渲染为内联胶囊（无方括号），点击当前值 chip 完成切换。
	click := func(label string) {
		t.Helper()
		lines := strings.Split(ansi.Strip(tool.View()), "\n")
		for y, line := range lines {
			if x := strings.Index(line, label); x >= 0 {
				tool.Update(tea.MouseMsg{X: lipgloss.Width(line[:x]) + 1, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
				return
			}
		}
		t.Fatalf("missing button %q in %q", label, lines)
	}
	click("UTF-8")
	if !tool.ascii {
		t.Fatal("text encoding did not change")
	}
	if got := lipgloss.Height(tool.View()); got != 16 {
		t.Fatalf("height=%d", got)
	}
	click("ASCII")
	if tool.ascii {
		t.Fatal("text encoding did not change back")
	}
}

func TestMultilineAndASCII(t *testing.T) {
	tool := newTool()
	tool.multiline = true
	tool.Input.SetValue("a\r\nb\n")
	tool.recalc()
	if got := tool.OutputText(); got != "YQ==\r\nYg==\n" {
		t.Fatalf("encoded=%q", got)
	}
	tool.encode = false
	tool.Input.SetValue(tool.OutputText())
	tool.recalc()
	if got := tool.OutputText(); got != "a\r\nb\n" {
		t.Fatalf("decoded=%q", got)
	}
	tool.encode = true
	tool.multiline = false
	tool.ascii = true
	tool.Input.SetValue("aé")
	tool.recalc()
	if got := tool.OutputText(); got != "YT8=" {
		t.Fatalf("ASCII=%q", got)
	}
	tool.ascii = false
	tool.Input.SetValue("  \n")
	tool.recalc()
	if got := tool.OutputText(); got != "ICAK" {
		t.Fatalf("whitespace=%q", got)
	}
}
