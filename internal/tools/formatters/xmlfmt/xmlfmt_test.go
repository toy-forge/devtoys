package xmlfmt

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

// TestFormatSimple 验证基础缩进格式化。
func TestFormatSimple(t *testing.T) {
	got, err := FormatXML("<a><b>text</b></a>", false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	want := strings.Join([]string{
		"<a>",
		"  <b>text</b>",
		"</a>",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNested 验证多层嵌套与属性、注释、声明的保留。
func TestFormatNested(t *testing.T) {
	src := `<?xml version="1.0"?><root id="1"><!-- comment --><item/><x>1</x></root>`
	got, err := FormatXML(src, false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	for _, want := range []string{
		`<?xml version="1.0"?>`,
		`<root id="1">`,
		"<!-- comment -->",
		"  <item/>",	// 自闭合标签原样保留
		"  <x>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("输出缺少 %q:\n%s", want, got)
		}
	}
}

// TestSelfCloseMerge 空元素 <a></a> 应合并为 <a />。
func TestSelfCloseMerge(t *testing.T) {
	got, err := FormatXML("<a></a>", false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	if got != "<a />" {
		t.Fatalf("got %q want %q", got, "<a />")
	}
}

// TestFormatCDATA 验证 CDATA 段作为字符数据原样保留（回归：缺少 i = end
// 推进会导致死循环）。
func TestFormatCDATA(t *testing.T) {
	got, err := FormatXML(`<root><![CDATA[hello]]></root>`, false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	want := "<root><![CDATA[hello]]></root>"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMinify 验证压缩模式去掉缩进与换行。
func TestMinify(t *testing.T) {
	got, err := FormatXML("<a>\n  <b>x</b>\n</a>", true)
	if err != nil {
		t.Fatalf("压缩不应报错: %v", err)
	}
	if got != "<a><b>x</b></a>" {
		t.Fatalf("got %q", got)
	}
	// 压缩模式下自闭合合并不带空格。
	got, err = FormatXML("<a></a>", true)
	if err != nil {
		t.Fatalf("压缩不应报错: %v", err)
	}
	if got != "<a/>" {
		t.Fatalf("got %q want %q", got, "<a/>")
	}
}

// TestErrors 验证配对校验的各种错误路径都带行号。
func TestErrors(t *testing.T) {
	cases := []struct {
		src	string
		hint	string
	}{
		{"</a>", "unexpected closing tag"},
		{"<a><b></a></b>", "does not match"},
		{"<a>", "unclosed"},
		{"<a></b>", "does not match"},
		{"text<a/>", "character data"},
		{"<a><x/></a><b>y</b>", "after root"},
		{"<!-- xxx", "unclosed comment"},
		{"<![CDATA[ xxx", "unclosed CDATA"},
		{"<?php echo", "unclosed processing instruction"},
		{"<1bad/>", "invalid '<'"},
		{"</>", "invalid end tag name"},
		{"<a b=", "unclosed start tag"},
		{"</a b>", "invalid end tag name"},
		{"<!DOCTYPE x", "unclosed declaration"},
	}
	for _, c := range cases {
		_, err := FormatXML(c.src, false)
		if err == nil {
			t.Fatalf("FormatXML(%q) 应报错", c.src)
		}
		if !strings.Contains(err.Error(), c.hint) {
			t.Fatalf("FormatXML(%q) 错误应包含 %q，得到: %v", c.src, c.hint, err)
		}
		if !strings.Contains(err.Error(), "line ") {
			t.Fatalf("错误应带行号，得到: %v", err)
		}
	}
}

// TestMultilineErrorLine 多层标签未闭合时报告最内层标签的起始行号。
func TestMultilineErrorLine(t *testing.T) {
	_, err := FormatXML("<a>\n<b>\ntext", false)
	if err == nil || !strings.Contains(err.Error(), "line 2") ||
		!strings.Contains(err.Error(), "<b>") {
		t.Fatalf("应报告第 2 行的 <b> 未闭合，得到: %v", err)
	}
}

// TestTokenizeXML 验证词法单元的行号与自闭合标记。
func TestTokenizeXML(t *testing.T) {
	toks, err := tokenizeXML("<a>\n  <b />\n  text\n</a>")
	if err != nil {
		t.Fatalf("词法切分不应报错: %v", err)
	}
	if len(toks) != 5 {	// open a、空白文本、open b（自闭合）、text、close a
		t.Fatalf("应有 5 个词法单元，得到 %d: %+v", len(toks), toks)
	}
	if toks[0].kind != tokOpen || toks[0].name != "a" || toks[0].line != 1 {
		t.Fatalf("第一个单元不正确: %+v", toks[0])
	}
	if toks[2].line != 2 || !toks[2].selfClosed {
		t.Fatalf("<b /> 应在第二行且为自闭合: %+v", toks[2])
	}
	if toks[4].kind != tokClose || toks[4].line != 4 {
		t.Fatalf("</a> 应在第四行: %+v", toks[4])
	}
}

// TestSelfCloseFn 直接验证 selfClose 的两种模式。
func TestSelfCloseFn(t *testing.T) {
	if got := selfClose("<a>", false); got != "<a />" {
		t.Fatalf("got %q", got)
	}
	if got := selfClose("<a>", true); got != "<a/>" {
		t.Fatalf("got %q", got)
	}
	if got := selfClose("<a x=\"1\" />", false); got != "<a x=\"1\" />" {
		t.Fatalf("已自闭合的标签应原样返回，得到 %q", got)
	}
}

func TestIndentationAndAttributes(t *testing.T) {
	src := `<root a="one two" b = 'x>y'><child c="v"/></root>`
	got, err := formatXML(src, false, "\t", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\n\ta=\"one two\"", "\n\tb = 'x>y'>", "\n\t<child", "\n\t\tc=\"v\"/>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestRejectMultipleEmptyRoots(t *testing.T) {
	for _, src := range []string{`<a/><b/>`, `<a></a><b/>`, `<a/><b></b>`} {
		if _, err := FormatXML(src, false); err == nil {
			t.Errorf("accepted multiple roots: %s", src)
		}
	}
}

func TestPreservesMixedContentAndSignificantWhitespace(t *testing.T) {
	for _, src := range []string{
		`<p>Hello <b>world</b> !</p>`,
		`<p><b>Hello</b> <i>world</i></p>`,
		`<a>  value  </a>`,
		"<a>\u00a0</a>",
		"<root xml:space=\"preserve\">\n  <x/>\n</root>",
		`<a><![CDATA[  content  ]]><b/> tail</a>`,
	} {
		for _, minify := range []bool{false, true} {
			got, err := FormatXML(src, minify)
			if err != nil || got != src {
				t.Errorf("minify %v: got %q, err %v, want %q", minify, got, err, src)
			}
		}
	}
}

func TestOnlyXMLWhitespaceIsIgnored(t *testing.T) {
	if _, err := FormatXML("\u00a0<root/>", false); err == nil {
		t.Fatal("accepted non-whitespace text outside root")
	}
	if _, err := FormatXML("<!-- comment -->", false); err == nil {
		t.Fatal("accepted document without root")
	}
}

func TestRejectInvalidAttributesAndEntities(t *testing.T) {
	for _, src := range []string{`<a x="1" x="2"/>`, `<a x=1/>`, `<a>&missing;</a>`, `<a x="<"/>`, `<a:b/>`} {
		if _, err := FormatXML(src, false); err == nil {
			t.Errorf("accepted invalid XML %q", src)
		}
	}
	for _, src := range []string{`<!DOCTYPE a [<!ENTITY test "hi">]><a>&test;</a>`, `<?xml version="1.0" encoding="test"?><a/>`} {
		if _, err := FormatXML(src, false); err != nil {
			t.Errorf("rejected valid XML %q: %v", src, err)
		}
	}
}

func TestLargeXMLBackgroundAndStaleResult(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ws.Input.SetValue("<root>" + strings.Repeat("<item/>", 5000) + "</root>")
	m.scheduleFormat()
	_, first := m.Update(formatDue{revision: m.revision})
	if first == nil || !m.running {
		t.Fatal("background format did not start")
	}
	m.minify = true
	m.scheduleFormat()
	m.Update(formatDue{revision: m.revision})
	if !m.queued {
		t.Fatal("new operation not queued")
	}
	_, latest := m.Update(first().(toolkit.BackgroundToolMsg).Message)
	if latest == nil || m.ws.OutputText() != "" {
		t.Fatal("stale output applied")
	}
	m.Update(latest().(toolkit.BackgroundToolMsg).Message)
	if got := m.ws.OutputText(); strings.Contains(got, "\n") || !strings.HasPrefix(got, "<root><item/>") {
		t.Fatal("latest minified output missing")
	}
	m.ws.Input.SetValue("<new/>")
	m.scheduleFormat()
	m.Update(formatDone{revision: m.revision - 1, output: "old"})
	if got := m.ws.OutputText(); got != "<new/>" {
		t.Fatalf("old output overwrote current: %q", got)
	}
}

func TestXMLFormattingRecoversAfterInvalidInput(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(toolkit.FileImportMsg{Text: "<a"})
	if m.ws.Err == nil {
		t.Fatal("missing error")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/>")})
	if m.ws.Err != nil || m.ws.OutputText() != "<a/>" {
		t.Fatalf("output=%q err=%v", m.ws.OutputText(), m.ws.Err)
	}
}

func TestDoctypeWithQuotedBracketsAndComment(t *testing.T) {
	src := `<!DOCTYPE root [<!-- ] > --><!ENTITY text "] > ok">]><root>&text;</root>`
	got, err := FormatXML(src, true)
	if err != nil || got != src {
		t.Fatalf("got %q error %v", got, err)
	}
}
