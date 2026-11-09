// styles.go 集中常用的样式构造与缓存。
//
// 调色板明暗切换时（SetDark）缓存整体失效，下一次取值时按新色板重建。
// 各工具与组件库应优先使用这里的构造器，避免各自手写重复样式。
package theme

import (
	"sync"

	"github.com/charmbracelet/lipgloss"
)

type (
	// StyleKey 唯一标识一个缓存样式。
	StyleKey	uint8

	// styleEntry 是缓存键：样式种类 + 当前明暗模式。
	styleEntry	struct {
		key	StyleKey
		dark	bool
	}
)

const (
	// KeyTitle 是标题样式（加粗正文色）。
	KeyTitle	StyleKey	= iota
	// KeyMuted 是弱化文本样式。
	KeyMuted
	// KeyValue 是取值文本样式（正文色；可交互性由控件自身的填充/hover 表达）。
	KeyValue
	// KeyLabel 是普通标签文本样式。
	KeyLabel
	// KeySurface 是表面填充样式（Surface 背景）。
	KeySurface
	// KeySurfaceHigh 是高一级表面填充样式。
	KeySurfaceHigh
	// KeyPanel 是静止面板边框样式。
	KeyPanel
	// KeyPanelFocus 是焦点面板边框样式（焦点环）。
	KeyPanelFocus
	// KeyError 是错误信息条（红底）。
	KeyError
	// KeySuccess 是成功信息条（绿底）。
	KeySuccess
	// KeyWarning 是警告信息条（黄底）。
	KeyWarning
	// KeyOverlayDim 是弹窗压暗层。
	KeyOverlayDim
	// KeyStatusLeft 是状态栏左侧文本。
	KeyStatusLeft
	// KeyStatusRight 是状态栏右侧提示。
	KeyStatusRight
	// KeyStatusAccent 是状态栏强调文本。
	KeyStatusAccent
	// KeyDisabled 是禁用控件/文本样式（Muted + Faint）。
	KeyDisabled
)

var (
	styleMu		sync.RWMutex
	styleCache	= map[styleEntry]lipgloss.Style{}
	lastDarkSet	= true
)

// resetStyleCache 在 SetDark 后由 colors.go 调用。
func resetStyleCache() {
	styleMu.Lock()
	defer styleMu.Unlock()
	styleCache = map[styleEntry]lipgloss.Style{}
	lastDarkSet = dark
}

func cached(key StyleKey, build func() lipgloss.Style) lipgloss.Style {
	styleMu.RLock()
	d := lastDarkSet
	s, ok := styleCache[styleEntry{key, d}]
	styleMu.RUnlock()
	if ok {
		return s
	}
	s = build()
	styleMu.Lock()
	styleCache[styleEntry{key, d}] = s
	styleMu.Unlock()
	return s
}

// Style 按键返回缓存的公共样式。
func Style(key StyleKey) lipgloss.Style {
	return cached(key, func() lipgloss.Style {
		switch key {
		case KeyTitle:
			return lipgloss.NewStyle().Foreground(Text).Bold(true)
		case KeyMuted:
			return lipgloss.NewStyle().Foreground(Muted)
		case KeyValue:
			return lipgloss.NewStyle().Foreground(Text)
		case KeyLabel:
			return lipgloss.NewStyle().Foreground(Text)
		case KeySurface:
			return lipgloss.NewStyle().Background(Surface)
		case KeySurfaceHigh:
			return lipgloss.NewStyle().Background(SurfaceHigh)
		case KeyError:
			return lipgloss.NewStyle().Foreground(Background).Background(Red).Padding(0, 1)
		case KeySuccess:
			return lipgloss.NewStyle().Foreground(Background).Background(Success).Padding(0, 1)
		case KeyWarning:
			return lipgloss.NewStyle().Foreground(Background).Background(Warning).Padding(0, 1)
		case KeyStatusLeft:
			return lipgloss.NewStyle().Foreground(Text).Bold(true)
		case KeyStatusRight:
			return lipgloss.NewStyle().Foreground(Muted)
		case KeyStatusAccent:
			return lipgloss.NewStyle().Foreground(CursorText).Background(Accent).Bold(true)
		case KeyDisabled:
			return lipgloss.NewStyle().Foreground(Muted).Faint(true)
		default:
			return lipgloss.NewStyle()
		}
	})
}

// TitleStyle 标题（加粗正文色）。
func TitleStyle() lipgloss.Style	{ return Style(KeyTitle) }

// MutedStyle 弱化文本。
func MutedStyle() lipgloss.Style	{ return Style(KeyMuted) }

// ValueStyle 取值文本（正文色）。
func ValueStyle() lipgloss.Style	{ return Style(KeyValue) }

// DisabledStyle 禁用控件/文本（Muted + Faint）。
func DisabledStyle() lipgloss.Style	{ return Style(KeyDisabled) }

// LabelStyle 普通标签文本。
func LabelStyle() lipgloss.Style	{ return Style(KeyLabel) }

// SurfaceStyle Surface 背景。
func SurfaceStyle() lipgloss.Style	{ return Style(KeySurface) }

// PanelBorderStyle 圆角面板边框；focused 时使用焦点色（焦点环）。
func PanelBorderStyle(focused bool) lipgloss.Style {
	color := Border
	if focused {
		color = BorderFocus
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color)
}

// ErrorStyle 错误信息条（红底 + 图标位）。
func ErrorStyle() lipgloss.Style	{ return Style(KeyError) }

// SuccessStyle 成功信息条。
func SuccessStyle() lipgloss.Style	{ return Style(KeySuccess) }

// WarningStyle 警告信息条。
func WarningStyle() lipgloss.Style	{ return Style(KeyWarning) }
