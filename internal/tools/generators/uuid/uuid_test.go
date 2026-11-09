package uuid

import (
	"strings"
	"testing"
)

// TestSyncCountAndFormat 验证数量同步与大写/去连字符格式化。
func TestSyncCountAndFormat(t *testing.T) {
	m := New()
	if got := len(strings.Split(m.ws.OutputText(), "\n")); got != 5 {
		t.Fatalf("初始数量应为 5，实际 %d", got)
	}

	// 调整数量：增补与截断。
	m.bar.Numbers[0].Value = 8
	m.syncCount()
	if got := len(strings.Split(m.ws.OutputText(), "\n")); got != 8 {
		t.Fatalf("数量应为 8，实际 %d", got)
	}
	first := m.ws.OutputText()
	m.bar.Toggles[0].On = true	// 大写
	m.bar.Toggles[1].On = false	// 去连字符（关闭 Hyphens 开关）
	m.render()
	if strings.ContainsAny(m.ws.OutputText(), "-") {
		t.Fatalf("去连字符后仍包含 '-': %s", m.ws.OutputText())
	}
	if m.ws.OutputText() != strings.ToUpper(strings.ReplaceAll(first, "-", "")) {
		t.Fatalf("大写+去连字符转换不正确")
	}

	m.bar.Numbers[0].Value = 2
	m.syncCount()
	if got := len(strings.Split(m.ws.OutputText(), "\n")); got != 2 {
		t.Fatalf("数量应为 2，实际 %d", got)
	}
}
