// icon.go 是组件库侧的图标入口，委托给 theme 的图标表
// （Nerd Font 字符 + ASCII 回退，见 theme/icons.go）。
package ui

import (
	"github.com/mattn/go-runewidth"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Icon 按语义名称返回当前模式的图标字符。
func Icon(name theme.IconName) string	{ return theme.Icon(name) }

// IconWidth 用 go-runewidth 返回图标的显示宽度。
func IconWidth(name theme.IconName) int	{ return theme.IconWidth(name) }

// ToolIcon 按工具注册表 ID 返回图标。
func ToolIcon(id string) string	{ return theme.ToolIcon(id) }

// GroupIcon 按类别名返回图标。
func GroupIcon(category string) string	{ return theme.GroupIcon(category) }

// StatusIcon 按严重度（0=info 1=success 2=warning 3=error）返回图标。
func StatusIcon(severity uint8) string	{ return theme.StatusIcon(severity) }

// TextWidth 用 go-runewidth 计算纯文本显示宽度（不含 ANSI 处理）。
func TextWidth(s string) int	{ return runewidth.StringWidth(s) }
