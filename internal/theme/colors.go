// Package theme 定义共享的语义角色调色板。
//
// 调色板按"语义角色"组织（背景、表面、边框、文本、强调色、状态色……），
// 同一个角色在 dark 与 light 两套终端背景下各有对应色值。
// dark 采用 GitHub Dark 十六进制色，light 采用 GitHub Light 对应色。
//
// 启动时经 termenv.HasDarkBackground 探测终端底色并缓存为导出变量；
// 所有导出色值同时是组件库（internal/ui）与各工具的唯一样式来源。
// 导出色值是 var 而非 const，以支持明暗切换。
package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Color 是调色板色值类型，与 lipgloss.Color 完全等价。
type Color = lipgloss.Color

// Palette 是一套完整的语义角色色板。
type Palette struct {
	Background	Color	// 应用最底层背景（E0 画布）
	Surface		Color	// 面板/卡片/状态栏表面（E1）
	SurfaceHigh	Color	// 悬浮层、下拉列表、hover 填充等更高一级表面（E2）
	Border		Color	// 静止边框（发丝线）
	BorderStrong	Color	// 重边框：hover 时的盒边框、E2 浮层（下拉菜单、弹窗）边框
	BorderFocus	Color	// 焦点边框（键盘焦点环；语义上等于 Accent）
	Text		Color	// 正文
	CursorText	Color	// 反色填充（选中/聚焦填充）上的文字
	Muted		Color	// 弱化文本、提示
	Accent		Color	// 品牌强调色（仅用于：主按钮、选中态、菜单光标、焦点环、开关 on、链接）
	AccentHover	Color	// Primary 按钮与开关 on 的 hover 色（dark 提亮 / light 加深）
	Control		Color	// 可交互控件的静止填充（按钮、下拉 chip、开关 off、步进格）：比 Surface 高一档，保证在任何表面上都可辨认为控件
	ControlHover	Color	// 控件 hover 填充：在 Control 之上再抬一档
	Blue		Color	// 语义蓝
	BlueBright	Color	// 亮蓝（可点击/取值文本）
	Green		Color	// 语义绿
	Red		Color	// 语义红
	Yellow		Color	// 语义黄
	YellowBright	Color	// 亮黄
	Magenta		Color	// 语义紫红
	Success		Color	// 成功状态
	Warning		Color	// 警告状态
	Danger		Color	// 错误/危险操作
	Overlay		Color	// 弹窗遮罩底色（激活，配合 DimLine 的 faint 使用）
}

// darkPalette 是 GitHub Dark 基调的暗色板。
var darkPalette = Palette{
	Background:	"#0D1117",
	Surface:	"#161B22",
	SurfaceHigh:	"#21262D",
	Border:		"#30363D",	// GitHub Dark 默认发丝线
	BorderStrong:	"#484F58",
	BorderFocus:	"#2F81F7",
	Text:		"#E6EDF3",
	CursorText:	"#EFF8FF",
	Muted:		"#8B949E",
	Accent:		"#2F81F7",
	AccentHover:	"#58A6FF",
	Control:	"#30363D",
	ControlHover:	"#484F58",
	Blue:		"#58A6FF",
	BlueBright:	"#79C0FF",
	Green:		"#3FB950",
	Red:		"#FF7B72",
	Yellow:		"#D29922",
	YellowBright:	"#E3B341",
	Magenta:	"#BC8CFF",
	Success:	"#3FB950",
	Warning:	"#D29922",
	Danger:		"#FF7B72",
	Overlay:	"#010409",
}

// lightPalette 是 GitHub Light 基调的亮色板。
var lightPalette = Palette{
	Background:	"#FFFFFF",
	Surface:	"#F6F8FA",
	SurfaceHigh:	"#EAEEF2",
	Border:		"#D0D7DE",
	BorderStrong:	"#8C959F",
	BorderFocus:	"#0969DA",
	Text:		"#1F2328",
	CursorText:	"#FFFFFF",
	Muted:		"#59636E",
	Accent:		"#0969DA",
	AccentHover:	"#0550AE",
	Control:	"#D0D7DE",
	ControlHover:	"#B1BAC4",
	Blue:		"#0969DA",
	BlueBright:	"#0550AE",
	Green:		"#1A7F37",
	Red:		"#CF222E",
	Yellow:		"#9A6700",
	YellowBright:	"#BF8700",
	Magenta:	"#8250DF",
	Success:	"#1A7F37",
	Warning:	"#9A6700",
	Danger:		"#CF222E",
	Overlay:	"#1F2328",
}

// 语义角色色值。默认取暗色板，init 时按终端底色（或 DEVTOYS_THEME）确定。
var (
	Background	Color
	Surface		Color
	SurfaceHigh	Color
	Border		Color
	BorderStrong	Color
	BorderFocus	Color
	Text		Color
	CursorText	Color
	Muted		Color
	Accent		Color
	AccentHover	Color
	Control		Color
	ControlHover	Color
	Blue		Color
	BlueBright	Color
	Green		Color
	Red		Color
	Yellow		Color
	YellowBright	Color
	Magenta		Color
	Success		Color
	Warning		Color
	Danger		Color
	Overlay		Color

	dark	= true
)

func init() {
	switch strings.ToLower(os.Getenv("DEVTOYS_THEME")) {
	case "light":
		SetDark(false)
	case "dark":
		SetDark(true)
	default:
		SetDark(termenv.DefaultOutput().HasDarkBackground())
	}
}

// SetDark 切换明暗调色板并刷新全部导出色值与样式缓存。
func SetDark(d bool) {
	dark = d
	if d {
		apply(darkPalette)
	} else {
		apply(lightPalette)
	}
	resetStyleCache()
}

func apply(p Palette) {
	Background = p.Background
	Surface = p.Surface
	SurfaceHigh = p.SurfaceHigh
	Border = p.Border
	BorderStrong = p.BorderStrong
	BorderFocus = p.BorderFocus
	Text = p.Text
	CursorText = p.CursorText
	Muted = p.Muted
	Accent = p.Accent
	AccentHover = p.AccentHover
	Control = p.Control
	ControlHover = p.ControlHover
	Blue = p.Blue
	BlueBright = p.BlueBright
	Green = p.Green
	Red = p.Red
	Yellow = p.Yellow
	YellowBright = p.YellowBright
	Magenta = p.Magenta
	Success = p.Success
	Warning = p.Warning
	Danger = p.Danger
	Overlay = p.Overlay
}

// Dark 报告当前是否使用暗色板。
func Dark() bool	{ return dark }

// Current 返回当前语义角色的副本，便于一次性取整板配色。
func Current() Palette {
	return Palette{Background: Background, Surface: Surface, SurfaceHigh: SurfaceHigh, Border: Border, BorderStrong: BorderStrong, BorderFocus: BorderFocus, Text: Text, CursorText: CursorText, Muted: Muted, Accent: Accent, AccentHover: AccentHover, Control: Control, ControlHover: ControlHover, Blue: Blue, BlueBright: BlueBright, Green: Green, Red: Red, Yellow: Yellow, YellowBright: YellowBright, Magenta: Magenta, Success: Success, Warning: Warning, Danger: Danger, Overlay: Overlay}
}

// hexRGB 解析 "#RRGGBB" 形式的颜色；解析失败返回 ok=false。
func hexRGB(c Color) (r, g, b int, ok bool) {
	s := strings.TrimPrefix(string(c), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF), true
}

// OverlaySGR 返回 Overlay 遮罩底色的真彩背景序列 "\x1b[48;2;r;g;bm"，
// 供 ui.DimLine 在压暗背景行时注入。Overlay 非十六进制色时返回空串
// （此时遮罩只保留 faint，不注入底色）。
func OverlaySGR() string {
	r, g, b, ok := hexRGB(Overlay)
	if !ok {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}
