package xpath

import (
	"strings"
	"testing"
)

const sampleXML = `<?xml version="1.0"?>
<store>
  <book price="30"><title>Go 语言</title></book>
  <book price="45"><title>Rust 语言</title></book>
</store>`

// newTestModel 构造一个已填入 XML 文档的测试模型。
func newTestModel(t *testing.T) *model {
	t.Helper()
	m := New()
	m.ws.Input.SetValue(sampleXML)
	return m
}

func outputOf(t *testing.T, expr string) (string, error) {
	t.Helper()
	m := newTestModel(t)
	m.expr.SetValue(expr)
	m.recompute()
	return m.ws.OutputText(), m.ws.Err
}

func TestElementNodeQuery(t *testing.T) {
	out, err := outputOf(t, "//book[@price > 30]/title")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "Rust 语言") || strings.Contains(out, "Go 语言") {
		t.Fatalf("意外输出: %q", out)
	}
	if !strings.Contains(out, "<title>Rust 语言</title>") {
		t.Fatalf("期望输出节点 XML: %q", out)
	}
}

func TestAttributeNodeQuery(t *testing.T) {
	out, err := outputOf(t, "//book/@price")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "@price = 30") || !strings.Contains(out, "@price = 45") {
		t.Fatalf("意外输出: %q", out)
	}
}

func TestScalarQuery(t *testing.T) {
	out, err := outputOf(t, "count(//book)")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "Scalar result: 2") {
		t.Fatalf("期望标量结果 2，得到: %q", out)
	}
}

func TestInvalidExpression(t *testing.T) {
	_, err := outputOf(t, "//book[[")
	if err == nil || !strings.Contains(err.Error(), "invalid XPath expression") {
		t.Fatalf("期望表达式无效错误，得到: %v", err)
	}
}

func TestInvalidXML(t *testing.T) {
	m := New()
	m.ws.Input.SetValue("<a><b></a>")
	m.expr.SetValue("//b")
	m.recompute()
	if m.ws.Err == nil || !strings.Contains(m.ws.Err.Error(), "XML parse failed") {
		t.Fatalf("期望 XML 解析失败错误，得到: %v", m.ws.Err)
	}
}

func TestNoMatch(t *testing.T) {
	out, err := outputOf(t, "//nomatch")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "No results") {
		t.Fatalf("期望无匹配提示，得到: %q", out)
	}
}
