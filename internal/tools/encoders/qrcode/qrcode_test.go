package qrcode

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/skip2/go-qrcode"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

// recalcOf 构造工具实例，以指定纠错等级计算，返回输出与错误。
func recalcOf(t *testing.T, in string, lv level) (string, error) {
	t.Helper()
	tl := newTool()
	tl.lv = lv
	tl.Input.SetValue(in)
	tl.recalc()
	return tl.OutputText(), tl.Err
}

// TestGenerateTypical 典型输入应生成非空二维码（半块字符渲染）。
func TestGenerateTypical(t *testing.T) {
	got, err := recalcOf(t, "https://example.com", levelMedium)
	if err != nil {
		t.Fatalf("生成二维码不应报错: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("二维码输出不应为空")
	}
	// ToSmallString 使用 ▀▄█ 与空格等半块字符。
	if !strings.ContainsAny(got, "▀▄█ ") {
		t.Fatalf("输出应包含半块字符渲染: %q", got)
	}
}

// TestAllLevels 四个纠错等级均应能生成二维码。
func TestAllLevels(t *testing.T) {
	for lv := levelLow; lv <= levelHighest; lv++ {
		got, err := recalcOf(t, "level test", lv)
		if err != nil {
			t.Fatalf("等级 %d 生成失败: %v", lv, err)
		}
		if strings.TrimSpace(got) == "" {
			t.Fatalf("等级 %d 输出不应为空", lv)
		}
	}
}

// TestChineseInput 中文内容应可生成二维码。
func TestChineseInput(t *testing.T) {
	got, err := recalcOf(t, "你好，DevToys", levelMedium)
	if err != nil {
		t.Fatalf("中文输入不应报错: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("输出不应为空")
	}
}

// TestEmptyInput 空输入不应报错：输出无内容，输出区展示引导提示。
func TestEmptyInput(t *testing.T) {
	got, err := recalcOf(t, "   \n", levelMedium)
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if got != "" {
		t.Fatalf("空输入应输出空串，得到 %q", got)
	}
	// OutputText 是可复制的正文（此处为空）；引导提示只出现在展示层，
	// 未初始化布局时按默认窄宽度裁剪为 …，但必须仍以文字开头而不是空白。
	tl := newTool()
	tl.Input.SetValue("   \n")
	tl.recalc()
	if shown := strings.TrimSpace(tl.Output.View()); !strings.HasPrefix(shown, "Enter text") {
		t.Fatalf("空闲态应给出引导提示，得到 %q", tl.Output.View())
	}
}

// TestEmptyMessageTruncatesWithEllipsis 窄输出区下空态提示以 … 结尾，
// 不在单词中间硬截断。
func TestEmptyMessageTruncatesWithEllipsis(t *testing.T) {
	tl := newTool()
	tl.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
	tl.Output.Width = 20
	msg := tl.emptyMessage()
	if !strings.HasSuffix(msg, "…") {
		t.Fatalf("窄输出区提示应以 … 结尾，得到 %q", msg)
	}
	if strings.Contains(msg, "QR c") && !strings.Contains(msg, "QR code") {
		t.Fatalf("提示不得在单词中间截断: %q", msg)
	}
}

// TestTooLongContent 超出二维码容量的输入应返回友好错误。
func TestTooLongContent(t *testing.T) {
	long := strings.Repeat("a", 10000)
	_, err := recalcOf(t, long, levelHighest)
	if err == nil {
		t.Fatal("超长内容应报错")
	}
	if !strings.Contains(err.Error(), "could not generate QR code") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
}

func TestScanImageAndIgnoreStaleResult(t *testing.T) {
	data, err := qrcode.Encode("hello from QR", qrcode.Medium, 256)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	text, err := scanImage(img)
	if err != nil || text != "hello from QR" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, cmd := tool.Update(toolkit.ImageImportMsg{Image: img})
	if cmd == nil {
		t.Fatal("no scan command")
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("new")})
	event := cmd().(toolkit.BackgroundToolMsg)
	tool.Update(event.Message)
	if tool.Input.Value() != "new" {
		t.Fatalf("stale scan replaced input: %q", tool.Input.Value())
	}
}
