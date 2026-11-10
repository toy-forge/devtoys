package toolkit

import "github.com/toy-forge/devtoys/internal/ui"

// PaneHintsProvider supplies pane-specific快捷键提示，由外壳状态栏合并展示。
type PaneHintsProvider interface {
	PaneStatusHints() []ui.StatusHint
}

// OutputOnlyProvider 报告工具只有结果窗格，没有可切换焦点的输入窗格。
type OutputOnlyProvider interface {
	OutputOnly() bool
}

var paneHintsByID = map[string]func(Tool) []ui.StatusHint{}

// RegisterPaneHints 在工具 init 中注册；fn 接收同一包内的具体 Tool 类型。
func RegisterPaneHints(id string, fn func(Tool) []ui.StatusHint) {
	if id == "" || fn == nil {
		panic("toolkit: invalid pane hints registration")
	}
	paneHintsByID[id] = fn
}

// PaneStatusHints 返回工具的窗格提示（无注册则尝试 PaneHintsProvider）。
func PaneStatusHints(t Tool) []ui.StatusHint {
	if t == nil {
		return nil
	}
	if p, ok := t.(PaneHintsProvider); ok {
		return p.PaneStatusHints()
	}
	if fn, ok := paneHintsByID[t.ID()]; ok {
		return fn(t)
	}
	return nil
}

// OutputOnly 报告工具是否只有结果窗格（此时 tab 焦点切换无意义）。
func OutputOnly(t Tool) bool {
	if provider, ok := t.(OutputOnlyProvider); ok {
		return provider.OutputOnly()
	}
	return false
}
