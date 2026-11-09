package jsonfmt

// 回归测试：高亮走真彩 theme 色、无 256 色残留、无背景序列。

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

func TestHighlightUsesThemeTruecolor(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	out := highlight(`{"name": "devtoys", "n": 1, "ok": true}`)
	if out == "" {
		t.Fatal("highlight returned empty")
	}
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Fatalf("highlight must emit truecolor fg: %q", out)
	}
	if strings.Contains(out, "\x1b[38;5;") || strings.Contains(out, "\x1b[48;5;") {
		t.Fatalf("highlight must not emit 256-color sequences: %q", out)
	}
	if strings.Contains(out, "\x1b[48;2;") {
		t.Fatalf("highlight must not paint backgrounds (panel Surface owns bg): %q", out)
	}
	// 蓝色系（键/数字 BlueBright #79C0FF、字符串 Blue #58A6FF）必须出现。
	if !strings.Contains(out, "38;2;121;192;255") && !strings.Contains(out, "38;2;88;166;255") {
		t.Fatalf("highlight must use theme blues: %q", out)
	}
	// 可读性：剥离 ANSI 后仍是完整 JSON 文本。
	if got := ansi.Strip(out); !strings.Contains(got, `"name"`) {
		t.Fatalf("stripped text lost content: %q", got)
	}
}
