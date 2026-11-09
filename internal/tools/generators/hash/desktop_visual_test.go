package hash

// 回归测试：面板矩阵与内边距、占位符、footer 铺底。

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/toy-forge/devtoys/internal/theme"
)

const (
	accentSGR	= "38;2;47;129;247"	// Accent #2F81F7
	borderSGR	= "38;2;48;54;60"	// Border #30363D（termenv 渲染实际值，蓝通道 -1）
	textSGR		= "38;2;230;237;243"	// Text #E6EDF3
	mutedSGR	= "38;2;139;147;158"	// Muted #8B949E（termenv 渲染实际值，绿通道 -1）
	surfaceBGSGR	= "48;2;22;27;34"	// Surface #161B22
)

func forceTrueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func paneTitleRow(t *testing.T, lines []string, marker string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), marker) {
			return line
		}
	}
	t.Fatalf("pane title %q not found", marker)
	return ""
}

func TestHashDesktopVisualPolish(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	m := New()
	m.width, m.height = 100, 30
	m.resize()

	// 占位符 = theme.Muted。
	if got := m.key.PlaceholderStyle.GetForeground(); got != theme.Muted {
		t.Fatalf("key placeholder fg = %v, want %v", got, theme.Muted)
	}
	if got := m.expected.PlaceholderStyle.GetForeground(); got != theme.Muted {
		t.Fatalf("expected placeholder fg = %v, want %v", got, theme.Muted)
	}

	if !m.desktopLayout() {
		t.Skipf("desktop layout inactive at %dx%d", m.width, m.height)
	}
	// 初始状态：workspace 焦点在 Input（零值），hash focus == 0。
	if m.ws.Focus != 0 || m.focus != 0 {
		t.Fatalf("initial focus: ws=%v hash=%v", m.ws.Focus, m.focus)
	}
	lines := strings.Split(m.View(), "\n")

	// 标题矩阵（直接驱动 hashPane，避免 JoinHorizontal 拼行互相污染；
	// 内容需足够宽才能嵌入标题）：
	// focused = BorderFocus 环 + Text bold 标题；rest = Border 发丝线 +
	// Muted bold 标题。
	focused := hashPane(strings.Repeat("x", 20), "Input text", true)
	if !strings.Contains(focused, accentSGR) || !strings.Contains(focused, textSGR) || strings.Contains(focused, mutedSGR) {
		t.Fatalf("focused pane must ring Accent + Text bold title: %q", focused)
	}
	for _, rest := range []string{hashPane(strings.Repeat("x", 20), "File", false), hashPane(strings.Repeat("x", 20), "Output", false)} {
		if strings.Contains(rest, accentSGR) || !strings.Contains(rest, mutedSGR) || !strings.Contains(rest, borderSGR) {
			t.Fatalf("rest pane must be hairline Border + Muted bold title: %q", rest)
		}
	}

	// 内边距：内容行 = 边框 1 列 + 内边距 1 列 + 文本，窗格总宽 =
	// 内容宽 + 4（与 ui.Panel 同一算术）。
	padded := hashPane(strings.Repeat("a", 20), "File", false)
	pLines := strings.Split(padded, "\n")
	if got := lipgloss.Width(pLines[1]); got != 24 {
		t.Fatalf("pane row width = %d, want content+4 = 24", got)
	}
	if got := ansi.Strip(pLines[1]); !strings.HasPrefix(got, "│ aaa") {
		t.Fatalf("content must start at border+1 padding column: %q", got)
	}
	// 桌面视图里 FILE 正文同样紧贴「边框+1 内边距」。
	fileRow := -1
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "Select a file to hash") {
			fileRow = i
			break
		}
	}
	if fileRow < 0 {
		t.Fatal("file pane content not found")
	}
	if got := ansi.Strip(lines[fileRow]); !strings.Contains(got, "│ Select a file to hash") {
		t.Fatalf("file label must sit one padding column inside the border: %q", got)
	}
	if got := ansi.Strip(lines[7]); !strings.Contains(got, "│ ┃ Enter text to hash") {
		t.Fatalf("input prompt must sit one padding column inside the border: %q", got)
	}

	// 面板行整行 Surface 底。
	for _, marker := range []string{"Input text", "File", "Output"} {
		row := paneTitleRow(t, lines, marker)
		if !strings.Contains(row, surfaceBGSGR) {
			t.Fatalf("%s pane row missing Surface bg: %q", marker, row)
		}
	}

	// footer 行整行 Surface 底 + Text 前景。
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, surfaceBGSGR) || !strings.Contains(footer, textSGR) {
		t.Fatalf("footer must be Text on Surface: %q", footer)
	}
	if got := lipgloss.Width(footer); got != m.width {
		t.Fatalf("footer width = %d, want %d", got, m.width)
	}
}
