// theme_test.go 锁死色板合同：全部语义角色在明暗两板下非零、
// BorderStrong/AccentHover 与 Border/Accent 区分明确、
// 明暗 Accent 必须不同、OverlaySGR 输出格式正确。
package theme

import (
	"fmt"
	"testing"
)

func TestPaletteCompleteness(t *testing.T) {
	previous := Dark()
	defer SetDark(previous)
	// 每个语义角色在两套板下都必须有值。
	for _, dark := range []bool{true, false} {
		SetDark(dark)
		p := Current()
		roles := map[string]Color{
			"Background":	p.Background,
			"Surface":	p.Surface,
			"SurfaceHigh":	p.SurfaceHigh,
			"Border":	p.Border,
			"BorderStrong":	p.BorderStrong,
			"BorderFocus":	p.BorderFocus,
			"Text":		p.Text,
			"CursorText":	p.CursorText,
			"Muted":	p.Muted,
			"Accent":	p.Accent,
			"AccentHover":	p.AccentHover,
			"Control":	p.Control,
			"ControlHover":	p.ControlHover,
			"Blue":		p.Blue,
			"BlueBright":	p.BlueBright,
			"Green":	p.Green,
			"Red":		p.Red,
			"Yellow":	p.Yellow,
			"YellowBright":	p.YellowBright,
			"Magenta":	p.Magenta,
			"Success":	p.Success,
			"Warning":	p.Warning,
			"Danger":	p.Danger,
			"Overlay":	p.Overlay,
		}
		for name, c := range roles {
			if c == "" {
				t.Fatalf("dark=%v role %s is empty", dark, name)
			}
		}
		// Current() 必须与导出 var 同步。
		if p.Control != Control || p.ControlHover != ControlHover {
			t.Fatalf("Current() out of sync with control tokens")
		}
		// 控件填充必须比画布和表面都更亮（可辨认），hover 再抬一档。
		if Control == Surface || Control == SurfaceHigh || ControlHover == Control {
			t.Fatalf("dark=%v control fills must be distinct from Surface and each other", dark)
		}
		if p.BorderStrong != BorderStrong || p.AccentHover != AccentHover {
			t.Fatalf("Current() out of sync with exported vars")
		}
		// 海拔与边框档位必须彼此可区分。
		if BorderStrong == Border {
			t.Fatalf("dark=%v BorderStrong must differ from Border", dark)
		}
		if AccentHover == Accent {
			t.Fatalf("dark=%v AccentHover must differ from Accent", dark)
		}
		if SurfaceHigh == Surface || Surface == Background {
			t.Fatalf("dark=%v elevation colors must differ", dark)
		}
	}
	// 明暗 Accent 必须不同（切板必须可感知）。
	SetDark(true)
	darkAccent := Accent
	SetDark(false)
	if Accent == darkAccent {
		t.Fatal("light and dark accents must differ")
	}
}

func TestOverlaySGR(t *testing.T) {
	previous := Dark()
	defer SetDark(previous)
	for _, dark := range []bool{true, false} {
		SetDark(dark)
		r, g, b, ok := hexRGB(Overlay)
		if !ok {
			t.Fatalf("dark=%v Overlay %q is not a hex color", dark, Overlay)
		}
		want := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
		if got := OverlaySGR(); got != want {
			t.Fatalf("dark=%v OverlaySGR = %q, want %q", dark, got, want)
		}
	}
}

func TestStyleConstructors(t *testing.T) {
	previous := Dark()
	defer SetDark(previous)
	SetDark(true)
	// KeyValue 是正文色（可交互性由控件表达）。
	if got := ValueStyle().GetForeground(); got != Text {
		t.Fatalf("ValueStyle foreground = %v, want Text %v", got, Text)
	}
	// DisabledStyle = Muted + Faint。
	ds := DisabledStyle()
	if got := ds.GetForeground(); got != Muted {
		t.Fatalf("DisabledStyle foreground = %v, want Muted", got)
	}
	if !ds.GetFaint() {
		t.Fatal("DisabledStyle must be faint")
	}
}
