package markdown

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// newTestModel 构造一个可直接测试的模型。
func newTestModel() *Model {
	return &Model{ws: toolkit.NewWorkspace("测试输入", previewHint)}
}

// TestRenderBasic 验证常见 Markdown 能渲染出内容且不报错。
func TestRenderBasic(t *testing.T) {
	m := newTestModel()
	m.rebuildRenderer(80)
	m.render("# 标题\n\n正文 **加粗** 与 `code`。\n\n- 列表项 1\n- 列表项 2\n", true)

	m.ws.Input.SetValue("# 标题")
	m.syncOutput(false)
	out := m.ws.OutputText()
	if m.ws.Err != nil {
		t.Fatalf("渲染出错: %v", m.ws.Err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("预览输出为空")
	}
}

// TestToggleSource 验证 ctrl+p 逻辑：原文视图输出与输入一致。
func TestToggleSource(t *testing.T) {
	m := newTestModel()
	m.rebuildRenderer(80)
	src := "# Hello\n\nWorld"
	m.ws.Input.SetValue(src)

	m.showSrc = true
	m.syncOutput(true)
	if m.ws.OutputText() != src {
		t.Fatalf("原文视图输出 = %q, want 输入原文", m.ws.OutputText())
	}

	m.showSrc = false
	m.syncOutput(true)
	if strings.TrimSpace(m.ws.OutputText()) == src {
		t.Fatal("预览视图不应原样输出源码")
	}
}

func TestMarkdownStyleUsesSharedPalette(t *testing.T) {
	style := markdownStyle()
	if style.Document.Color == nil || *style.Document.Color != string(theme.Text) {
		t.Fatal("document color does not match theme")
	}
	if style.H1.BackgroundColor == nil || *style.H1.BackgroundColor != string(theme.Accent) {
		t.Fatal("heading background does not match theme")
	}
	if style.CodeBlock.Theme != "github-dark" || style.CodeBlock.Chroma != nil {
		t.Fatal("code block theme is inconsistent")
	}
}

func TestThemeSwitchAndEmptyPreview(t *testing.T) {
	m := newTestModel()
	m.header.Names = []string{"Preview theme", "View"}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(toolkit.FileImportMsg{Text: "# Heading"})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true})
	if !m.light || m.tr == nil || m.ws.Err != nil {
		t.Fatalf("light=%v error=%v", m.light, m.ws.Err)
	}
	if lightMarkdownStyle().Document.BackgroundColor == nil {
		t.Fatal("light preview background missing")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Light") {
		t.Fatalf("theme not shown: %s", m.View())
	}
	m.ws.Input.SetValue("")
	m.Update(toolkit.InputClearedMsg{})
	if m.ws.OutputText() != previewHint {
		t.Fatalf("cleared preview: %q", m.ws.OutputText())
	}
}

func TestBackgroundRenderDiscardsStaleAndQueuesLatest(t *testing.T) {
	m := newTestModel()
	m.header.Names = []string{"Preview theme", "View"}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ws.Input.SetValue(strings.Repeat("hello world\n", 1500))
	m.scheduleOutput(false)
	if m.ws.OutputText() != "" {
		t.Fatal("stale output remained while rendering")
	}
	_, first := m.Update(renderDue{revision: m.revision})
	if first == nil || !m.running {
		t.Fatal("background render not started")
	}
	m.ws.Input.SetValue(strings.Repeat("new text\n", 2000))
	m.scheduleOutput(false)
	m.Update(renderDue{revision: m.revision})
	if !m.queued {
		t.Fatal("new render not queued")
	}
	_, next := m.Update(first().(toolkit.BackgroundToolMsg).Message)
	if next == nil || m.ws.OutputText() != "" {
		t.Fatal("stale result applied")
	}
	m.Update(next().(toolkit.BackgroundToolMsg).Message)
	if !strings.Contains(ansi.Strip(m.ws.OutputText()), "new text") {
		t.Fatal("latest render missing")
	}
	m.ws.Input.SetValue(strings.Repeat("old\n", 5000))
	m.scheduleOutput(false)
	_, worker := m.Update(renderDue{revision: m.revision})
	m.ws.Input.SetValue("# newest")
	m.scheduleOutput(false)
	m.Update(worker().(toolkit.BackgroundToolMsg).Message)
	if !strings.Contains(ansi.Strip(m.ws.OutputText()), "newest") {
		t.Fatal("old background result replaced small preview")
	}
}

func TestMarkdownLayoutFits(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 7}, {Width: 30, Height: 7}, {Width: 80, Height: 24}} {
		m := newTestModel()
		m.header.Names = []string{"Preview theme", "View"}
		m.Update(size)
		for i := 0; i < 2; i++ {
			if w, h := lipgloss.Width(m.View()), lipgloss.Height(m.View()); w > size.Width || h != size.Height {
				t.Fatalf("%v: got %dx%d", size, w, h)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true})
		}
	}
}

func TestMarkdownDesktopExtensions(t *testing.T) {
	r, err := newRenderer(100, false)
	if err != nil {
		t.Fatal(err)
	}
	source := "---\ntitle: Hidden metadata\n---\n\n# Document\n\n:smile: -- a footnote[^1].\n\n[^1]: Footnote content.\n\n| Name | Value |\n| --- | --- |\n| alpha | beta |\n\n- [x] Completed\n\n` :smile: `\n"
	output, err := r.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(output)
	for _, want := range []string{"Document", "😄", "Footnote content", "alpha", "beta", "Completed", ":smile:"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in preview:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Hidden metadata") || strings.Contains(plain, "[^1]:") {
		t.Errorf("metadata or footnote markup leaked:\n%s", plain)
	}
	if !strings.Contains(plain, "–") {
		t.Errorf("smart punctuation missing:\n%s", plain)
	}
}

func TestFrontMatterOnlyStripsCompleteOpeningBlock(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"---\ntitle: x\n---\n# Body", "# Body"},
		{"\ufeff---\r\ntitle: x\r\n...\r\nbody", "body"},
		{"---\rtitle: x\r---\rbody", "body"},
		{"---\ntext\n", "---\ntext\n"},
		{"# Heading\n---\ntext\n---\n", "# Heading\n---\ntext\n---\n"},
		{"---\ntitle: x\n---", ""},
	} {
		if got := withoutFrontMatter(test.source); got != test.want {
			t.Errorf("%q: %q, want %q", test.source, got, test.want)
		}
	}
}

func TestSmartPunctuationDoesNotDropFollowingText(t *testing.T) {
	r, err := newRenderer(80, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render(`"hello" -- world ... done, isn't it?`)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"“hello”", "–", "world", "…", "done", "isn’t it?"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q: %q", want, plain)
		}
	}
	out, err = r.Render("Second document")
	if err != nil || !strings.Contains(ansi.Strip(out), "Second document") || strings.Contains(ansi.Strip(out), "hello") {
		t.Fatalf("renderer reuse: %q, %v", ansi.Strip(out), err)
	}
}
