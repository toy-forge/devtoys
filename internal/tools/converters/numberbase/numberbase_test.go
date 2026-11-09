package numberbase

import (
	"math/big"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// TestValidateDigits 验证进制数字校验的合法与非法路径。
func TestValidateDigits(t *testing.T) {
	legal := []struct {
		s	string
		base	int
	}{
		{"1010", 2},
		{"-777", 8},
		{"+255", 10},
		{"deadBEEF", 16},
		{"zz", 36},
		{"1000", 10},
		{"-", 10},	// 只有符号：字符校验通过，交给 big.Int 报错
	}
	for _, c := range legal {
		if err := validateDigits(c.s, c.base); err != nil {
			t.Fatalf("validateDigits(%q, %d) 不应报错: %v", c.s, c.base, err)
		}
	}

	illegal := []struct {
		s	string
		base	int
		hint	string
	}{
		{"", 10, "empty input"},
		{"12a3", 10, "outside"},
		{"2", 2, "outside"},
		{"g", 16, "outside"},
		{"1#0", 10, "is not a digit"},
		{"１２３", 10, "is not a digit"},	// 全角数字
	}
	for _, c := range illegal {
		err := validateDigits(c.s, c.base)
		if err == nil {
			t.Fatalf("validateDigits(%q, %d) 应报错", c.s, c.base)
		}
		if !strings.Contains(err.Error(), c.hint) {
			t.Fatalf("validateDigits(%q, %d) 错误信息应包含 %q，得到: %v", c.s, c.base, c.hint, err)
		}
	}
}

// TestDigitValueAndChar 验证字符与数值的互转。
func TestDigitValueAndChar(t *testing.T) {
	cases := map[rune]int{'0': 0, '9': 9, 'a': 10, 'f': 15, 'A': 10, 'Z': 35, '_': -1, '+': -1}
	for r, want := range cases {
		if got := digitValue(r); got != want {
			t.Fatalf("digitValue(%q) = %d, want %d", r, got, want)
		}
	}
	for v := 0; v <= 35; v++ {
		if got := digitValue(digitChar(v)); got != v {
			t.Fatalf("digitChar(%d) 往返不一致: %d", v, got)
		}
	}
	if digitChar(15) != 'f' {
		t.Fatalf("digitChar(15) = %q, want 'f'", digitChar(15))
	}
}

// TestCycleBase 验证进制循环切换与兜底逻辑。
func TestCycleBase(t *testing.T) {
	if got := cycleBase(2, 1); got != 8 {
		t.Fatalf("cycleBase(2, 1) = %d, want 8", got)
	}
	if got := cycleBase(8, -1); got != 2 {
		t.Fatalf("cycleBase(8, -1) = %d, want 2", got)
	}
	if got := cycleBase(36, 1); got != 2 {	// 循环回绕
		t.Fatalf("cycleBase(36, 1) = %d, want 2", got)
	}
	if got := cycleBase(2, -1); got != 36 {	// 向前回绕
		t.Fatalf("cycleBase(2, -1) = %d, want 36", got)
	}
	if got := cycleBase(7, 1); got != 9 {	// 跳过 8
		t.Fatalf("cycleBase(7, 1) = %d, want 9", got)
	}
	if got := cycleBase(999, 1); got != 10 {	// 不在表中的进制兜底为 10
		t.Fatalf("cycleBase(999, 1) = %d, want 10", got)
	}
}

// TestBaseName 验证进制中文名。
func TestBaseName(t *testing.T) {
	cases := map[int]string{2: "Binary", 8: "Octal", 10: "Decimal", 16: "Hexadecimal", 36: "Base 36"}
	for b, want := range cases {
		if got := baseName(b); got != want {
			t.Fatalf("baseName(%d) = %q, want %q", b, got, want)
		}
	}
}

// recomputeOf 构造模型并计算，返回输出与错误。
func recomputeOf(t *testing.T, in string, from, to int) (string, error) {
	t.Helper()
	m := &model{ws: toolkit.NewWorkspace("test", ""), from: from, to: to}
	m.recompute(in)
	return m.ws.OutputText(), m.ws.Err
}

// TestRecomputeTypical 验证十进制到十六进制及大整数转换。
func TestRecomputeTypical(t *testing.T) {
	got, err := recomputeOf(t, "255", 10, 16)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	if !strings.Contains(got, "ff") || !strings.Contains(got, "377") || !strings.Contains(got, "11111111") {
		t.Fatalf("常用进制参考缺失:\n%s", got)
	}
	if !strings.Contains(got, "Hexadecimal (target) : ff") {
		t.Fatalf("目标进制行不正确:\n%s", got)
	}
}

// TestRecomputeBigNumber math/big 支持的超大整数应正确转换。
func TestRecomputeBigNumber(t *testing.T) {
	n, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	got, err := recomputeOf(t, "123456789012345678901234567890", 10, 16)
	if err != nil {
		t.Fatalf("大整数转换不应报错: %v", err)
	}
	if !strings.Contains(got, n.Text(16)) {
		t.Fatalf("大整数十六进制结果缺失:\n%s", got)
	}
}

// TestRecomputeNegative 负数与下划线分隔应被支持。
func TestRecomputeNegative(t *testing.T) {
	got, err := recomputeOf(t, "-255", 10, 16)
	if err != nil {
		t.Fatalf("负数转换不应报错: %v", err)
	}
	if !strings.Contains(got, "-ff") {
		t.Fatalf("负数结果缺失:\n%s", got)
	}

	got, err = recomputeOf(t, "1010_0001", 2, 10)
	if err != nil {
		t.Fatalf("带下划线输入不应报错: %v", err)
	}
	if !strings.Contains(got, "161") {
		t.Fatalf("下划线应被忽略（10100001₂=161）:\n%s", got)
	}
}

// TestRecomputeInvalid 非法输入应报错且不输出结果。
func TestRecomputeInvalid(t *testing.T) {
	got, err := recomputeOf(t, "12a3", 10, 16)
	if err == nil {
		t.Fatal("非法数字应报错")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Fatalf("错误信息应友好，得到: %v", err)
	}
	if strings.Contains(got, "常用进制参考") {
		t.Fatalf("出错时不应输出转换结果:\n%s", got)
	}
}

// TestRecomputeEmpty 空输入应输出等待提示且不报错。
func TestRecomputeEmpty(t *testing.T) {
	got, err := recomputeOf(t, "  ", 10, 16)
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if got != "Waiting for input…" {
		t.Fatalf("空输入应提示等待，得到 %q", got)
	}
}

func TestMouseOptionsAndLiteralPunctuation(t *testing.T) {
	m := &model{ws: toolkit.NewWorkspace("", ""), from: 10, to: 16}
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
	if m.ws.Input.Value() != "." || m.to != 16 {
		t.Fatalf("input=%q to=%d", m.ws.Input.Value(), m.to)
	}
	m.Update(tea.MouseMsg{X: 1, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.from == 10 {
		t.Fatal("source base did not change")
	}
}
