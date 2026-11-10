package tools

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// Every registered tool exposes an input reachable by mouse at both sizes.
// Tool-specific options and output interactions are covered in their packages.
func TestRegisteredToolMouseInput(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 30, Height: 7}, {Width: 20, Height: 7}} {
		for _, registered := range toolkit.All() {
			t.Run(fmt.Sprintf("%s/%dx%d", registered.ID(), size.Width, size.Height), func(t *testing.T) {
				tool := registered
				if f, ok := tool.(toolkit.TextInput); ok && !f.AcceptsText() {
					return
				}
				if f, ok := tool.(toolkit.FileInput); ok && !f.AcceptsFiles() {
					return
				}
				next, _ := tool.Update(size)
				if updated, ok := next.(toolkit.Tool); ok {
					tool = updated
				}
				lines := strings.Split(ansi.Strip(tool.View()), "\n")
				// 取最后一个「┃ 」提示符：部分工具（certificate）的
				// 配置行内联输入也带 ┃，首个命中可能是回显密码框（键入
				// 不回显）；末个命中即主输入区。
				x, y := -1, -1
				for row, line := range lines {
					if pos := strings.LastIndex(line, "┃ "); pos >= 0 {
						x = lipgloss.Width(line[:pos]) + 2
						y = row
					}
				}
				if y < 0 || y >= size.Height {
					// 表格字段（ui.FieldView）不带提示符，无法用此通用方式定位：
					// 这些工具的字段点击与光标由各自包内的布局/鼠标测试覆盖。
					switch tool.ID() {
					case "converters.dateparser", "converters.numberbase":
						t.Skip("table-cell input: covered by the package's own layout and mouse tests")
					}
					t.Fatal("no visible input prompt")
				}
				next, _ = tool.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if updated, ok := next.(toolkit.Tool); ok {
					tool = updated
				}
				marker := fmt.Sprintf("m%d", size.Width)
				next, _ = tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(marker)})
				if updated, ok := next.(toolkit.Tool); ok {
					tool = updated
				}
				if !strings.Contains(ansi.Strip(tool.View()), marker) {
					t.Fatalf("mouse input %q not visible at (%d,%d)", marker, x, y)
				}
				for range marker {
					next, _ = tool.Update(tea.KeyMsg{Type: tea.KeyBackspace})
					if updated, ok := next.(toolkit.Tool); ok {
						tool = updated
					}
				}
			})
		}
	}
}

func TestToolOptionButtonsRespondToMouse(t *testing.T) {
	ids := []string{
		"converters.dateparser", "converters.numberbase",
		"encoders.base64", "encoders.gzip", "encoders.htmlenc", "encoders.urlenc", "encoders.qrcode",
		"formatters.xmlfmt", "text.markdown",
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			tool, ok := toolkit.Get(id)
			if !ok {
				t.Fatalf("tool not registered: %s", id)
			}
			next, _ := tool.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			if updated, ok := next.(toolkit.Tool); ok {
				tool = updated
			}
			lines := strings.Split(ansi.Strip(tool.View()), "\n")
			// 桌面式设置头可能不在首行渲染标题（字符形态为大写），
			// 定位标题行并点击其下第一行选项。
			row := 0
			for i, line := range lines {
				if strings.HasPrefix(strings.ToUpper(line), "CONFIGURATION") {
					row = i + 1
					break
				}
			}
			// 下拉框点击展开浮层（视图变化），开关与回退按钮则直接改变取值；
			// 两种情况下整个视图都必须有反应。
			before := tool.View()
			next, _ = tool.Update(tea.MouseMsg{X: 1, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if updated, ok := next.(toolkit.Tool); ok {
				tool = updated
			}
			if after := tool.View(); before == after {
				t.Fatalf("option did not respond to a click on row %d: %q", row, ansi.Strip(lines[row]))
			}
		})
	}
}

func TestGeneratorNumberButtonsRespondToMouse(t *testing.T) {
	labels := map[string]string{
		"generators.uuid":	"UUID Generation Count",
		"generators.lorem":	"Length",
		"generators.password":	"Length",
	}
	for _, id := range []string{"generators.uuid", "generators.lorem", "generators.password"} {
		t.Run(id, func(t *testing.T) {
			tool, ok := toolkit.Get(id)
			if !ok {
				t.Fatalf("tool not registered: %s", id)
			}
			next, _ := tool.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			if updated, ok := next.(toolkit.Tool); ok {
				tool = updated
			}
			// Stepper 控件渲染为 "[label] - value +"，[+] 贴行右缘；
			// 从行右端向左逐列点击，直到该行内容变化。
			sweep := func(label string, below int) (int, string, string) {
				t.Helper()
				lines := strings.Split(ansi.Strip(tool.View()), "\n")
				for y, line := range lines {
					pos := strings.Index(line, label)
					if pos < 0 {
						continue
					}
					for x := min(lipgloss.Width(line)-2, below-1); x > pos; x-- {
						next, _ := tool.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
						if updated, ok := next.(toolkit.Tool); ok {
							tool = updated
						}
						after := strings.Split(ansi.Strip(tool.View()), "\n")[y]
						if after != line {
							return x, line, after
						}
					}
					t.Fatalf("no clickable control on row %q", line)
				}
				t.Fatalf("stepper %q missing", label)
				return 0, "", ""
			}
			incX, _, _ := sweep(labels[id], 1<<30)
			// 从已命中的 [+] 再往左扫：跳过中间的值格后命中的是 [-]。
			_, before, after := sweep(labels[id], incX)
			if before == after {
				t.Fatalf("decrement button did not respond: %q", before)
			}
		})
	}
}
