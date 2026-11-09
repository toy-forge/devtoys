package numberbase

import (
	"math/big"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestBasicRepresentations(t *testing.T) {
	cases := []struct {
		base				int
		in, decimal, hex, binary	string
	}{
		{10, "255", "255", "FF", "1111 1111"},
		{10, "-1", "-1", "FFFF FFFF FFFF FFFF", strings.Repeat("1111 ", 15) + "1111"},
		{16, "FFFF FFFF FFFF FFFF", "-1", "FFFF FFFF FFFF FFFF", strings.Repeat("1111 ", 15) + "1111"},
		{2, "0000 1010", "10", "A", "1010"},
		{10, "123456789012345678901234567890", "123,456,789,012,345,678,901,234,567,890", "1 8EE9 0FF6 C373 E0EE 4E3F 0AD2", ""},
	}
	for _, tc := range cases {
		n, err := parseBasic(tc.in, tc.base)
		if err != nil {
			t.Fatal(err)
		}
		if got := basicValue(n, 10, true); got != tc.decimal {
			t.Fatalf("%q decimal %q", tc.in, got)
		}
		if got := basicValue(n, 16, true); got != tc.hex {
			t.Fatalf("%q hex %q expected %q", tc.in, got, tc.hex)
		}
		if tc.binary != "" {
			if got := basicValue(n, 2, true); got != tc.binary {
				t.Fatalf("%q binary %q", tc.in, got)
			}
		}
	}
}
func TestAlphabetRoundTrips(t *testing.T) {
	numbers := []string{"0", "1", "64", "18446744073709551615", "123456789012345678901234567890"}
	for _, a := range append([]alphabet{}, alphabets[:5]...) {
		t.Run(a.name, func(t *testing.T) {
			for _, input := range numbers {
				n, _ := new(big.Int).SetString(input, 10)
				for _, formatted := range []bool{false, true} {
					text, err := formatAlphabet(n, a, formatted)
					if err != nil {
						t.Fatal(err)
					}
					got, err := parseAlphabet(text, a)
					if err != nil || got.Cmp(n) != 0 {
						t.Fatalf("%s -> %q -> %v (%v)", input, text, got, err)
					}
				}
			}
		})
	}
	for _, dict := range []string{"-+", "🙂🙃🚀", "ABC,DEF"} {
		a := alphabet{name: "custom", chars: dict}
		n := big.NewInt(666)
		encoded, err := formatAlphabet(n, a, true)
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseAlphabet(encoded, a)
		if err != nil || got.Cmp(n) != 0 {
			t.Fatalf("%q encoded %q: %v %v", dict, encoded, got, err)
		}
	}
}
func TestAlphabetValidationAndSize(t *testing.T) {
	for _, a := range []string{"", "a", "aab", "a b", "a\nb"} {
		if _, err := validateAlphabet(a); err == nil {
			t.Fatalf("accepted invalid alphabet %q", a)
		}
	}
	if _, err := parseAlphabet("a", alphabets[1]); err == nil {
		t.Fatal("base32 incorrectly ignored case")
	}
	n, err := parseAlphabet("ff", alphabets[0])
	if err != nil || n.Int64() != 255 {
		t.Fatalf("base16 case: %v %v", n, err)
	}
	if _, err := parseAlphabet(strings.Repeat("A", maxDigits+1), alphabets[1]); err == nil {
		t.Fatal("oversized input accepted")
	}
}
func TestBasicSynchronizationAndErrors(t *testing.T) {
	c := newConverter()
	c.inputs[decimalField].SetValue("255")
	c.refresh(decimalField)
	if c.err != nil || c.inputs[hexField].Value() != "FF" || c.inputs[binaryField].Value() != "1111 1111" {
		t.Fatalf("bad fields %q %q %v", c.inputs[hexField].Value(), c.inputs[binaryField].Value(), c.err)
	}
	c.inputs[hexField].SetValue("10")
	c.refresh(hexField)
	if c.err != nil || c.inputs[decimalField].Value() != "16" {
		t.Fatalf("hex edit: %q %v", c.inputs[decimalField].Value(), c.err)
	}
	c.inputs[hexField].SetValue("XYZ")
	c.refresh(hexField)
	if c.err == nil || c.inputs[decimalField].Value() != "" {
		t.Fatal("invalid source left a stale output")
	}
	c.toggle(0)
	if c.inputs[hexField].Value() != "XYZ" {
		t.Fatal("format change discarded invalid source")
	}
	c.toggle(0)
	c.inputs[hexField].SetValue("FFFF")
	c.refresh(hexField)
	c.toggle(0)
	if c.err != nil || c.inputs[decimalField].Value() != "65535" || c.inputs[binaryField].Value() != "1111111111111111" {
		t.Fatalf("format off: %s %s %v", c.inputs[decimalField].Value(), c.inputs[binaryField].Value(), c.err)
	}
}
func TestAdvancedCustomAndPresets(t *testing.T) {
	c := newConverter()
	c.toggle(1)
	c.inputAlphabet = customAlphabet
	c.inputs[inputAlphabetField].SetValue("abc")
	c.outputAlphabet = customAlphabet
	c.inputs[outputAlphabetField].SetValue("01")
	c.inputs[inputField].SetValue("bca")
	c.refreshAdvanced()
	if c.err != nil || c.inputs[outputField].Value() != "1111" {
		t.Fatalf("custom conversion: %s %v", c.inputs[outputField].Value(), c.err)
	}
	c.inputs[outputAlphabetField].SetValue("00")
	c.refreshAdvanced()
	if c.err == nil || c.inputs[outputField].Value() != "" {
		t.Fatal("invalid alphabet left stale output")
	}
	c.inputAlphabet = 0
	c.outputAlphabet = 3
	c.inputs[inputField].SetValue("ff")
	c.refreshAdvanced()
	if c.err != nil || c.inputs[outputField].Value() != "D/" {
		t.Fatalf("RFC alphabet: %s %v", c.inputs[outputField].Value(), c.err)
	}
}
func TestLayoutAndMouseNavigation(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 30, Height: 7}, {Width: 20, Height: 7}} {
		c := newConverter()
		c.Update(size)
		for _, mode := range []bool{false, true} {
			if mode {
				c.toggle(1)
				c.inputAlphabet = customAlphabet
				c.outputAlphabet = customAlphabet
				c.resize()
			}
			for _, f := range c.fields() {
				c.setFocus(f)
				view := c.View()
				if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size.Width || h != size.Height {
					t.Fatalf("%v %v: %dx%d", size, f, w, h)
				}
				// 字段行以标签开头（定宽输入框无提示符）：标签可见即字段可见。
				if !strings.Contains(ansi.Strip(view), titles[f]) {
					t.Fatalf("%v field %v hidden", size, f)
				}
				for i, r := range c.rows() {
					if r.isInput && r.field == f {
						y := c.headerHeight + i - c.offset
						c.Update(tea.MouseMsg{X: 16, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
						if c.focus != f {
							t.Fatalf("%v clicked %v, got %v", size, f, c.focus)
						}
					}
				}
			}
		}
	}
}
func TestFileImportAndLongConversion(t *testing.T) {
	c := newConverter()
	c.setFocus(decimalField)
	c.inputs[decimalField].SetValue("")
	c.Update(toolkit.FileImportMsg{Text: "255\n"})
	if c.err != nil || c.inputs[hexField].Value() != "FF" {
		t.Fatalf("file import: %s %v", c.inputs[hexField].Value(), c.err)
	}
	input := strings.Repeat("9", maxDigits)
	c.inputs[decimalField].SetValue(input)
	c.refresh(decimalField)
	if c.err != nil || len(c.inputs[binaryField].Value()) < maxDigits {
		t.Fatalf("large result truncated or rejected: %d %v", len(c.inputs[binaryField].Value()), c.err)
	}
}

func TestCompactDictionaryButtons(t *testing.T) {
	c := newConverter()
	c.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	c.toggle(1)
	rows := c.rows()
	// 字典切换是内联按钮胶囊（无方括号字面量）。
	if !strings.Contains(ansi.Truncate(ansi.Strip(rows[0].text), 20, ""), ">") {
		t.Fatal("dictionary controls hidden on narrow screens")
	}
	c.Update(tea.MouseMsg{X: rows[0].actionsX + 5, Y: c.headerHeight, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if c.inputAlphabet != 1 {
		t.Fatalf("input dictionary selection=%d", c.inputAlphabet)
	}
}
