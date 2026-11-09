package urlenc

import (
	"strings"
	"testing"
)

// recalcOf 构造工具实例并按给定方向计算，返回输出与错误。
func recalcOf(t *testing.T, in string, encode bool, sc scope) (string, error) {
	t.Helper()
	tl := newTool()
	tl.encode = encode
	tl.scope = sc
	tl.Input.SetValue(in)
	tl.recalc()
	return tl.OutputText(), tl.Err
}

// TestComponentEncode 验证组件模式按 QueryEscape 编码。
func TestComponentEncode(t *testing.T) {
	got, err := recalcOf(t, "a b&c=中文", true, scopeComponent)
	if err != nil {
		t.Fatalf("编码不应报错: %v", err)
	}
	if got != "a+b%26c%3D%E4%B8%AD%E6%96%87" {
		t.Fatalf("got %q", got)
	}
}

// TestComponentDecode 验证组件模式解码，非法序列报友好错误。
func TestComponentDecode(t *testing.T) {
	got, err := recalcOf(t, "a+b%26c%3D%E4%B8%AD%E6%96%87", false, scopeComponent)
	if err != nil {
		t.Fatalf("解码不应报错: %v", err)
	}
	if got != "a b&c=中文" {
		t.Fatalf("got %q want %q", got, "a b&c=中文")
	}

	got, err = recalcOf(t, "%zz", false, scopeComponent)
	if err == nil {
		t.Fatalf("非法百分号序列应报错，输出 %q", got)
	}
	if !strings.Contains(err.Error(), "invalid URL encoding") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
}

// TestEncodeFullURL 验证完整 URL 分段转义：路径空格与查询参数。
func TestEncodeFullURL(t *testing.T) {
	got, err := encodeFullURL("https://example.com/a b/c d?q=go 语言&id=2")
	if err != nil {
		t.Fatalf("编码不应报错: %v", err)
	}
	if !strings.HasPrefix(got, "https://example.com/a%20b/c%20d?") {
		t.Fatalf("路径应被逐段转义，得到 %q", got)
	}
	if !strings.Contains(got, "q=go+%E8%AF%AD%E8%A8%80") || !strings.Contains(got, "id=2") {
		t.Fatalf("查询参数应被编码，得到 %q", got)
	}
}

// TestEncodeFullURLUserInfo 验证 URL 中的用户信息（凭证）不被丢弃。
func TestEncodeFullURLUserInfo(t *testing.T) {
	got, err := encodeFullURL("https://user:pass@example.com/path?a=1")
	if err != nil {
		t.Fatalf("编码不应报错: %v", err)
	}
	if want := "https://user:pass@example.com/path?a=1"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestEncodeFullURLErrors 缺少协议或解析失败时应返回可读错误。
func TestEncodeFullURLErrors(t *testing.T) {
	for _, in := range []string{"example.com/a", "::bad::"} {
		_, err := encodeFullURL(in)
		if err == nil {
			t.Fatalf("encodeFullURL(%q) 应报错", in)
		}
		if !strings.Contains(err.Error(), "component mode") {
			t.Fatalf("错误应提示切换组件模式，得到: %v", err)
		}
	}
}

// TestDecodeFullURL 验证完整 URL 解码输出结构化组件。
func TestDecodeFullURL(t *testing.T) {
	got, err := decodeFullURL("https://example.com/p?q=%E4%BD%A0%E5%A5%BD#top")
	if err != nil {
		t.Fatalf("解码不应报错: %v", err)
	}
	for _, want := range []string{"scheme", "example.com", "/p", "q", "你好", "fragment", "top"} {
		if !strings.Contains(got, want) {
			t.Fatalf("输出缺少 %q: %q", want, got)
		}
	}
}

// TestDecodeFullURLErrors 非法完整 URL 应报错。
func TestDecodeFullURLErrors(t *testing.T) {
	if _, err := decodeFullURL("no-scheme"); err == nil {
		t.Fatal("缺少协议前缀应报错")
	}
}

// TestEmptyInput 空输入应清空输出且不报错。
func TestEmptyInput(t *testing.T) {
	for _, sc := range []scope{scopeComponent, scopeFullURL} {
		got, err := recalcOf(t, "  ", true, sc)
		if err != nil {
			t.Fatalf("空输入不应报错: %v", err)
		}
		if got != "" {
			t.Fatalf("空输入应输出空串，得到 %q", got)
		}
	}
}

// TestOpaqueURL 不透明 URL（mailto:）在编码方向原样保留。
func TestOpaqueURL(t *testing.T) {
	got, err := encodeFullURL("mailto:someone@example.com")
	if err != nil {
		t.Fatalf("mailto 不应报错: %v", err)
	}
	if got != "mailto:someone@example.com" {
		t.Fatalf("got %q", got)
	}
}
