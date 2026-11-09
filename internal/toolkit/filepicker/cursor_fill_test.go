package filepicker

// 回归测试：光标行的整行 Accent 反色填充必须贯穿到行尾。
// 若把 marker/icon/name 拼成组合串再整体包裹，图标段的内层 reset 之后
// 只剩 Modal 重注入的 Surface 底——`\x1b[1;38;2;239;248;255;
// 48;2;47;129;247m<icon>\x1b[0m\x1b[48;2;22;27;34m`，选中行会退化成 4 列
// 图标 chip，与未选中行同色。因此 entryLine 的每一段（前缀空格、
// 复选框、图标、名段、行尾 pad）独立携带 cursor 样式，本测试以字节级
// 断言锁死。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/toy-forge/devtoys/internal/theme"
)

// cursorSGR = bold + CursorText fg(#EFF8FF) + Accent bg(#2F81F7)。
const cursorSGR = "\x1b[1;38;2;239;248;255;48;2;47;129;247m"

func forceTrueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// assertCursorFill 锁死光标行的字节级合同：
//  1. cursor 样式必须在多个分段上重新建立（组合串包裹只会在行首与
//     图标 chip 出现，名段/行尾全部落回 Surface 底）；
//  2. 名段必须直接开在 cursor 样式之后（名段自身带 Accent 填充）；
//  3. 名段之后必须还有 cursor 段（行尾 pad 同样式补满到整行宽）；
//  4. 整行显示宽度 == 内容区宽。
func assertCursorFill(t *testing.T, row string, innerW int, name string) {
	t.Helper()
	if got := strings.Count(row, cursorSGR); got < 4 {
		t.Fatalf("cursor style must re-establish on every segment, got %d: %q", got, row)
	}
	i := strings.Index(row, name)
	if i < 0 || !strings.HasSuffix(row[:i], cursorSGR) {
		t.Fatalf("name %q must open a cursor-styled segment: %q", name, row)
	}
	if strings.LastIndex(row, cursorSGR) <= i {
		t.Fatalf("cursor fill must continue after the name with a styled pad: %q", row)
	}
	if got := lipgloss.Width(row); got != innerW {
		t.Fatalf("cursor row width = %d, want %d", got, innerW)
	}
	if rest := ansi.Strip(row[strings.LastIndex(row, cursorSGR):]); strings.TrimSpace(rest) != "" {
		t.Fatalf("visible content after the last cursor segment must be pad spaces only: %q", rest)
	}
}

func TestCursorRowFillSpansFullRow(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)

	p := New(writeFixture(t))
	p.SetSize(80, 24)
	p.move(1)	// 光标移到 alpha.txt（文件行，单选无复选框）
	l := p.layout()
	assertCursorFill(t, p.entryLine(p.cursor, l.innerW), l.innerW, "alpha.txt")

	// 组合视图里同样成立：Modal 正文拼装（Space2 前缀 + Surface 重注入）
	// 不得吞掉名段/尾衬的填充。
	for _, line := range strings.Split(p.View(), "\n") {
		if !strings.Contains(line, cursorSGR) {
			continue
		}
		i := strings.Index(line, "alpha.txt")
		if i < 0 || !strings.HasSuffix(line[:i], cursorSGR) {
			t.Fatalf("composed cursor row lost the styled name segment: %q", line)
		}
		if strings.Count(line, cursorSGR) < 4 {
			t.Fatalf("composed cursor row lost segment styling: %q", line)
		}
		return
	}
	t.Fatal("cursor row not found in picker view")
}

func TestCursorRowFillWithCheckboxAndDirectory(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)

	p := New(writeFixture(t))
	p.SetSize(80, 24)
	p.SetMultiple(true)
	l := p.layout()
	// 光标在 ".."（目录行，无复选框）：前缀两个空格也必须进填充。
	assertCursorFill(t, p.entryLine(p.cursor, l.innerW), l.innerW, "..")
	// 光标移到文件行（带复选框）：checkbox/icon 段 + 名段 + 尾衬都带样式。
	p.move(1)
	assertCursorFill(t, p.entryLine(p.cursor, l.innerW), l.innerW, "alpha.txt")

	// 未选中行不得携带 cursor 样式（对比度由整行填充唯一承担）。
	for i := range p.entries {
		if i == p.cursor {
			continue
		}
		if row := p.entryLine(i, l.innerW); strings.Contains(row, cursorSGR) {
			t.Fatalf("unselected row must not carry the cursor style: %q", row)
		}
	}
}
