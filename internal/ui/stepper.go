// stepper.go 是 "[-] value [+]" 三连真实按钮（含中间只读值）。
package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Stepper 是数值步进控件：左右是真实按钮，中间是只读值。
type Stepper struct {
	// Label 是左侧说明文字（可为空）。
	Label	string
	// Value 是当前值，Dec/Inc 自动钳制在 [Min, Max]。
	Value	int
	Min	int
	Max	int
	// Disabled 变暗并忽略命中。
	Disabled	bool
	// ValueWidth 是中间值格的最小宽度；默认按 Min/Max 与当前值自适应。
	ValueWidth	int
}

// Dec 值减一并返回是否发生变化。
func (s *Stepper) Dec() bool {
	if s.Disabled || s.Value <= s.Min {
		return false
	}
	s.Value--
	return true
}

// Inc 值加一并返回是否发生变化。
func (s *Stepper) Inc() bool {
	if s.Disabled || s.Value >= s.Max {
		return false
	}
	s.Value++
	return true
}

// SetValue 直接设置值并钳制范围。
func (s *Stepper) SetValue(v int)	{ s.Value = max(s.Min, min(s.Max, v)) }

// valueCell 渲染只读值格：bg=Control、左右各 1 列内边距（宽 = valueWidth+2），
// fg=Text bold，disabled 走 DisabledStyle。渲染与 Rects 共用，
// 几何自洽（无外部手算依赖）。
func (s *Stepper) valueCell() string {
	text := strconv.Itoa(s.Value)
	width := s.valueWidth()
	if pad := width - Width(text); pad > 0 {
		text = strings.Repeat(" ", pad) + text
	}
	if s.Disabled {
		return theme.DisabledStyle().Background(theme.Surface).Padding(0, Space1).Render(text)
	}
	return lipgloss.NewStyle().
		Foreground(theme.Text).Bold(true).
		Background(theme.Control).
		Padding(0, Space1).
		Render(text)
}

func (s *Stepper) valueWidth() int {
	width := s.ValueWidth
	for _, v := range []int{s.Value, s.Min, s.Max} {
		if w := Width(strconv.Itoa(v)); w > width {
			width = w
		}
	}
	return max(1, width)
}

func (s *Stepper) stepButtons() (dec, inc Button) {
	dec = Button{Label: "-", Inline: true, Disabled: s.Disabled}
	inc = Button{Label: "+", Inline: true, Disabled: s.Disabled}
	if !s.Disabled {
		dec.Disabled = s.Value <= s.Min
		inc.Disabled = s.Value >= s.Max
	}
	return dec, inc
}

// View 渲染 "[Label] [-] value [+]"（单行；Label 为空时省略）。
// 三格 gap = Space1（组内缝，一个控件不拆散）。
func (s *Stepper) View() string {
	dec, inc := s.stepButtons()
	cells := make([]string, 0, 5)
	if s.Label != "" {
		cells = append(cells, lipgloss.NewStyle().Foreground(theme.Text).Render(s.Label))
	}
	cells = append(cells, dec.View(), s.valueCell(), inc.View())
	_, out := PlaceRow(cells, Space1)
	return out
}

// Width 返回渲染后的显示宽度。
func (s *Stepper) Width() int	{ return Width(s.View()) }

// Rects 返回三个格子绘制在 (offsetX, offsetY) 时的矩形：
// 依次为 [-]、值、[+]。
func (s *Stepper) Rects(offsetX, offsetY int) (dec, val, inc Rect) {
	decButton, incButton := s.stepButtons()
	cells := make([]string, 0, 5)
	if s.Label != "" {
		cells = append(cells, s.Label)
	}
	cells = append(cells, decButton.View(), s.valueCell(), incButton.View())
	rects, _ := PlaceRow(cells, Space1)
	base := len(rects) - 3
	if base < 0 {
		return Rect{}, Rect{}, Rect{}
	}
	shift := func(r Rect) Rect { r.X += offsetX; r.Y += offsetY; return r }
	return shift(rects[base]), shift(rects[base+1]), shift(rects[base+2])
}

// Hit 对一次点击返回步进方向：-1（减）、+1（加）；未点中时 hit=false。
func (s *Stepper) Hit(msg tea.Msg, offsetX, offsetY int) (delta int, hit bool) {
	mouse, press := isPressLeft(msg)
	if !press || s.Disabled {
		return 0, false
	}
	dec, val, inc := s.Rects(offsetX, offsetY)
	switch {
	case dec.Contains(mouse.X, mouse.Y):
		if s.Dec() {
			return -1, true
		}
	case inc.Contains(mouse.X, mouse.Y):
		if s.Inc() {
			return 1, true
		}
	case val.Contains(mouse.X, mouse.Y):
		return 0, true
	}
	return 0, false
}
