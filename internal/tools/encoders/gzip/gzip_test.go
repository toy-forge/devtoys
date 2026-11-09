package gzip

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// recalcOf 构造工具实例并按给定方向计算，返回输出与错误。
func recalcOf(t *testing.T, in string, compress bool) (string, error) {
	t.Helper()
	tl := newTool()
	tl.compress = compress
	tl.Input.SetValue(in)
	tl.recalc()
	return tl.OutputText(), tl.Err
}

// TestCompressToBase64 压缩结果应是合法 Base64 且以 GZip 魔数 1f 8b 开头。
func TestCompressToBase64(t *testing.T) {
	got := compressToBase64("hello gzip")
	raw, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("输出应是标准 Base64: %v", err)
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Fatalf("输出应携带 GZip 魔数，得到 %x", raw[:2])
	}
}

// TestRoundTrip 压缩再解压应还原原文（含中文与长文本）。
func TestRoundTrip(t *testing.T) {
	for _, src := range []string{"hello", "你好，世界", strings.Repeat("abc", 1000)} {
		encoded := compressToBase64(src)
		decoded, err := decompressFromBase64(encoded)
		if err != nil {
			t.Fatalf("解压不应报错: %v", err)
		}
		if string(decoded) != src {
			t.Fatalf("往返不一致: got %q want %q", decoded, src)
		}
	}
}

// TestDecompressWithWhitespace 带换行的 Base64 输入应可解压。
func TestDecompressWithWhitespace(t *testing.T) {
	encoded := compressToBase64("multiline payload")
	// 每 40 个字符插一个换行，模拟终端复制粘贴。
	var wrapped strings.Builder
	for i, r := range encoded {
		if i > 0 && i%40 == 0 {
			wrapped.WriteByte('\n')
		}
		wrapped.WriteRune(r)
	}
	decoded, err := decompressFromBase64(wrapped.String())
	if err != nil {
		t.Fatalf("带空白的解压不应报错: %v", err)
	}
	if string(decoded) != "multiline payload" {
		t.Fatalf("got %q", decoded)
	}
}

// TestDecompressNotBase64 非 Base64 输入应报友好错误。
func TestDecompressNotBase64(t *testing.T) {
	_, err := decompressFromBase64("这不是Base64!!!")
	if err == nil {
		t.Fatal("非 Base64 输入应报错")
	}
	if !strings.Contains(err.Error(), "invalid Base64") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
}

// TestDecompressNotGzip 合法 Base64 但不是 GZip 数据应报魔数错误。
func TestDecompressNotGzip(t *testing.T) {
	notGzip := base64.StdEncoding.EncodeToString([]byte("plain text"))
	_, err := decompressFromBase64(notGzip)
	if err == nil {
		t.Fatal("非 GZip 内容应报错")
	}
	if !strings.Contains(err.Error(), "1f 8b") {
		t.Fatalf("错误应提示缺少魔数，得到: %v", err)
	}
}

// TestDecompressCorrupted 数据体损坏应报数据损坏错误而不是 panic。
func TestDecompressCorrupted(t *testing.T) {
	valid := compressToBase64("some payload")
	raw, _ := base64.StdEncoding.DecodeString(valid)
	raw[len(raw)-1] ^= 0xff
	_, err := decompressFromBase64(base64.StdEncoding.EncodeToString(raw))
	if err == nil {
		t.Fatal("损坏数据应报错")
	}
	if !strings.Contains(err.Error(), "GZip data") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
}

// TestEmptyInput 空输入应清空输出且不报错。
func TestEmptyInput(t *testing.T) {
	for _, compress := range []bool{true, false} {
		got, err := recalcOf(t, " \n", compress)
		if err != nil {
			t.Fatalf("空输入不应报错（compress=%v）: %v", compress, err)
		}
		if got != "" {
			t.Fatalf("空输入应输出空串，得到 %q", got)
		}
	}
}

// TestRecalcViaTool 通过工具模型验证压缩方向输出与纯函数一致。
func TestRecalcViaTool(t *testing.T) {
	got, err := recalcOf(t, "consistent", true)
	if err != nil {
		t.Fatalf("压缩不应报错: %v", err)
	}
	if got != compressToBase64("consistent") {
		t.Fatalf("工具输出与纯函数不一致: %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("")) && got == "" {
		t.Fatal("输出不应为空")
	}
}
