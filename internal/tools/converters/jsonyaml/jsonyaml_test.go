package jsonyaml

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// recomputeOf 构造模型并计算，返回输出与错误。
func recomputeOf(t *testing.T, in string) (string, error) {
	t.Helper()
	m := newTool()
	m.recompute(in)
	return m.ws.OutputText(), m.ws.Err
}

// TestJSONToYAML 合法 JSON 应转为 YAML。
func TestJSONToYAML(t *testing.T) {
	got, err := recomputeOf(t, `{"name":"devtoys","count":3,"nested":{"ok":true}}`)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	for _, want := range []string{"name: devtoys", "count: 3", "ok: true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("YAML 输出缺少 %q:\n%s", want, got)
		}
	}
}

// TestYAMLToJSON 合法 YAML 应转为缩进的 JSON。
func TestYAMLToJSON(t *testing.T) {
	got, err := recomputeOf(t, "name: devtoys\ncount: 3\n")
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	if !strings.Contains(got, `"name": "devtoys"`) || !strings.Contains(got, `"count": 3`) {
		t.Fatalf("JSON 输出不正确:\n%s", got)
	}
	// 应按两空格缩进格式化。
	if !strings.Contains(got, "\n  \"count\": 3") {
		t.Fatalf("JSON 应缩进格式化:\n%s", got)
	}
}

// TestRoundTrip JSON → YAML → JSON 应语义一致。
func TestRoundTrip(t *testing.T) {
	const src = `{"list":[1,2,3],"obj":{"a":"b"}}`
	yamlOut, err := recomputeOf(t, src)
	if err != nil {
		t.Fatalf("JSON → YAML 不应报错: %v", err)
	}
	jsonOut, err := recomputeOf(t, yamlOut)
	if err != nil {
		t.Fatalf("YAML → JSON 不应报错: %v", err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(src), &want); err != nil {
		t.Fatalf("测试用例 JSON 不合法: %v", err)
	}
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("往返 JSON 不合法: %v\n%s", err, jsonOut)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("往返结果不一致: got %s want %s", gotJSON, wantJSON)
	}
}

// TestEmptyInput 空输入应输出等待提示且不报错。
func TestEmptyInput(t *testing.T) {
	got, err := recomputeOf(t, "  \n")
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if got != "Waiting for input…" {
		t.Fatalf("空输入应提示等待，得到 %q", got)
	}
}

// TestBothInvalid 两种格式都解析失败时应在信息条给出双方错误。
func TestBothInvalid(t *testing.T) {
	got, err := recomputeOf(t, "{oops: [")
	if err == nil {
		t.Fatal("非法输入应报错")
	}
	if !strings.Contains(err.Error(), "could not detect format") {
		t.Fatalf("错误应说明无法识别方向，得到: %v", err)
	}
	if !strings.Contains(err.Error(), "JSON") || !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("错误应同时包含两个方向的解析错误，得到: %v", err)
	}
	if strings.Contains(got, "list") {
		t.Fatalf("出错时不应输出转换结果: %q", got)
	}
}

// TestLineColAt 验证字节偏移到行列号的换算。
func TestLineColAt(t *testing.T) {
	cases := []struct {
		s		string
		offset		int
		line, col	int
	}{
		{"abc", 0, 1, 1},
		{"abc", 2, 1, 3},
		{"a\nb\nc", 2, 2, 1},	// offset 2 是 'b'
		{"a\nb\nc", 4, 3, 1},	// offset 4 是 'c'
		{"a\nb", 99, 2, 2},	// 超界按文本长度截断
	}
	for _, c := range cases {
		line, col := lineColAt(c.s, c.offset)
		if line != c.line || col != c.col {
			t.Fatalf("lineColAt(%q, %d) = (%d, %d), want (%d, %d)",
				c.s, c.offset, line, col, c.line, c.col)
		}
	}
}

// TestBothErrorsJSONSyntaxError 验证 JSON 语法错误带行列位置。
func TestBothErrorsJSONSyntaxError(t *testing.T) {
	const src = "{\n  oops: [\n}"	// JSON 与 YAML 均无法解析
	var jv any
	jsonErr := json.Unmarshal([]byte(src), &jv)
	if jsonErr == nil {
		t.Fatal("测试用例应无法按 JSON 解析")
	}
	var yv any
	yamlErr := yaml.Unmarshal([]byte(src), &yv)
	if yamlErr == nil {
		t.Fatal("测试用例应无法按 YAML 解析")
	}
	msg := bothErrors(src, jsonErr, yamlErr)
	if !strings.Contains(msg, "JSON syntax error") || !strings.Contains(msg, "line 2") {
		t.Fatalf("JSON 错误应带行号，得到: %s", msg)
	}
	if !strings.Contains(msg, "YAML parse error") {
		t.Fatalf("应附 YAML 错误，得到: %s", msg)
	}
}

func TestExplicitConversionAndIndentation(t *testing.T) {
	m := newTool()
	m.mode = 2
	m.indent = 4
	m.recompute("nested:\n  value: 1\n")
	if m.ws.Err != nil || !strings.Contains(m.ws.OutputText(), "\n    \"nested\"") {
		t.Fatalf("JSON result: %q, %v", m.ws.OutputText(), m.ws.Err)
	}
	m.mode = 1
	m.recompute("not JSON")
	if m.ws.Err == nil {
		t.Fatal("explicit JSON mode accepted invalid JSON")
	}
	m.recompute(`{"nested":{"value":1}}`)
	if m.ws.Err != nil || !strings.Contains(m.ws.OutputText(), "\n    value: 1") {
		t.Fatalf("YAML result: %q, %v", m.ws.OutputText(), m.ws.Err)
	}
}
