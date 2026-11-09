// toast.go 是四严重级别的瞬时通知。
// 视觉为语义 pill（Padding 0,1）：Info 走中性海拔，其余走语义色填充
// （Accent 不进 toast）。
package ui

import (
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Level 是通知的严重级别。
type Level uint8

const (
	// LevelInfo 是普通信息。
	LevelInfo	Level	= iota
	// LevelSuccess 是成功反馈（如 Copied）。
	LevelSuccess
	// LevelWarning 是警告。
	LevelWarning
	// LevelError 是错误。
	LevelError
)

// ToastExpireMsg 由 NewToast 返回的 Cmd 在到期时发出；
// 宿主收到后调用 Toast.Expired 匹配并清除。
type ToastExpireMsg struct{ ID int }

// Toast 是一条瞬时通知。
type Toast struct {
	// ID 唯一标识一条通知（≤0 表示常驻通知，永不过期也不匹配过期消息）。
	ID	int
	// Level 决定图标与颜色。
	Level	Level
	// Message 是正文。
	Message	string
}

var lastToastID int

// NewToast 创建通知；ttl > 0 时返回到期 Cmd（tea.Tick）。
// ttl <= 0 表示常驻通知，不产生过期消息。
func NewToast(level Level, message string, ttl time.Duration) (Toast, tea.Cmd) {
	lastToastID++
	t := Toast{ID: lastToastID, Level: level, Message: message}
	if ttl <= 0 {
		return t, nil
	}
	id := t.ID
	return t, tea.Tick(ttl, func(time.Time) tea.Msg { return ToastExpireMsg{ID: id} })
}

// Expired 报告过期消息是否属于这条通知。
func (t Toast) Expired(msg ToastExpireMsg) bool {
	return t.ID > 0 && msg.ID == t.ID
}

// Icon 返回通知图标。
func (t Toast) Icon() string	{ return theme.StatusIcon(uint8(t.Level)) }

// View 渲染语义 pill：图标 + 空格 + 消息，左右各 1 列内边距（单行）。
// Info：bg=SurfaceHigh、正文 fg=Text、图标 fg=Blue（保留语义蓝）；
// Success/Warning/Error：语义色整颗填充、fg=Background（沿用
// KeyError/KeySuccess 的反色惯例）。
func (t Toast) View() string {
	icon := t.Icon()
	if t.Level == LevelInfo {
		// Info 是中性海拔 pill；分段各自携带底色，避免内层 reset 打断背景。
		seg := func(fg theme.Color, s string) string {
			return lipgloss.NewStyle().Foreground(fg).Background(theme.SurfaceHigh).Render(s)
		}
		return seg(theme.Text, " ") + seg(theme.Blue, icon) + seg(theme.Text, " "+t.Message) + seg(theme.Text, " ")
	}
	bg := theme.Success
	switch t.Level {
	case LevelWarning:
		bg = theme.Warning
	case LevelError:
		bg = theme.Danger
	}
	return lipgloss.NewStyle().
		Foreground(theme.Background).Background(bg).
		Padding(0, Space1).
		Render(icon + " " + t.Message)
}

// ViewTruncate 渲染并截断到 width（含省略号），供状态栏嵌入。
func (t Toast) ViewTruncate(width int) string {
	return lipgloss.NewStyle().MaxWidth(max(0, width)).Render(t.View())
}

// Width 返回渲染后的显示宽度（含 pill 左右内边距）。
func (t Toast) Width() int	{ return Width(t.View()) }
