package htmlenc

import (
	"testing"
)

// recalcOf 构造工具实例并按给定方向计算，返回输出与错误。
func recalcOf(t *testing.T, in string, encode bool) (string, error) {
	t.Helper()
	tl := newTool()
	tl.encode = encode
	tl.Input.SetValue(in)
	tl.recalc()
	return tl.OutputText(), tl.Err
}

// TestEncodeTypical 验证 <、&、"、' 等字符被正确转义。
func TestEncodeTypical(t *testing.T) {
	got, err := recalcOf(t, `<a href="x">&'`+"\n", true)
	if err != nil {
		t.Fatalf("转义不应报错: %v", err)
	}
	want := "&lt;a href=&#34;x&#34;&gt;&amp;&#39;\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestDecodeTypical 验证实体被正确还原。
func TestDecodeTypical(t *testing.T) {
	got, err := recalcOf(t, "&lt;div class=&quot;a&quot;&gt;&amp;nbsp;&lt;/div&gt;", false)
	if err != nil {
		t.Fatalf("还原不应报错: %v", err)
	}
	want := `<div class="a">&nbsp;</div>`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestRoundTrip 转义后再还原应回到原文。
func TestRoundTrip(t *testing.T) {
	const src = "a<b>&\"'c\"</b>"
	escaped, err := recalcOf(t, src, true)
	if err != nil {
		t.Fatalf("转义不应报错: %v", err)
	}
	back, err := recalcOf(t, escaped, false)
	if err != nil {
		t.Fatalf("还原不应报错: %v", err)
	}
	if back != src {
		t.Fatalf("往返不一致: got %q want %q", back, src)
	}
}

// TestEmptyInput 空输入应输出空串且不报错。
func TestEmptyInput(t *testing.T) {
	for _, encode := range []bool{true, false} {
		got, err := recalcOf(t, "", encode)
		if err != nil {
			t.Fatalf("空输入不应报错（encode=%v）: %v", encode, err)
		}
		if got != "" {
			t.Fatalf("空输入应输出空串，得到 %q", got)
		}
	}
}

// TestIllegalEntity 非法实体应被宽容处理（原样保留），不报错。
func TestIllegalEntity(t *testing.T) {
	got, err := recalcOf(t, "&nosuchentity;", false)
	if err != nil {
		t.Fatalf("非法实体不应报错: %v", err)
	}
	if got != "&nosuchentity;" {
		t.Fatalf("非法实体应原样保留，得到 %q", got)
	}
}
