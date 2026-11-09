// Package toolkit 提供 DevToys TUI 的工具接口、注册表与公共 UI 组件。
//
// 每个工具在各自包的 init() 中调用 Register 自注册，避免维护一个
// 需要频繁修改的中心化工具清单文件；程序入口只需空导入
// internal/tools 即可触发全部注册。
package toolkit

import (
	"fmt"
	"image"
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// BackgroundToolMsg delivers completion to the tool that started a command,
// even if the user has navigated elsewhere while it was running.
type BackgroundToolMsg struct {
	Tool	Tool
	Message	tea.Msg
}

// ForegroundCommandMsg runs a prepared terminal command only while its tool
// remains active and no file picker is covering it.
type ForegroundCommandMsg struct {
	Tool	Tool
	Command	tea.Cmd
	Valid	func() bool
	OnDrop	func()
}

// ImageImportMsg delivers an image selected in the application file picker.
type ImageImportMsg struct {
	Image	image.Image
	Name	string
	Data	[]byte
}

// ImageBatchImportMsg delivers images selected together in the file picker.
type ImageBatchImportMsg struct{ Images []ImageImportMsg }

// ImageBatchImporter opts an image tool into multiple file selection.
type ImageBatchImporter interface{ AcceptsImageBatch() bool }

// BinaryImportMsg delivers a selected binary file to a compatible tool.
type BinaryImportMsg struct {
	Data	[]byte
	Name	string
}

// BinaryImporter opts a tool into receiving bounded binary files.
type BinaryImporter interface{ AcceptsBinary() bool }

// FilePathImportMsg delivers a user-selected path for streaming file tools.
type FilePathImportMsg struct{ Path string }

// FilePathImporter opts into receiving selected paths instead of buffered data.
type FilePathImporter interface{ AcceptsFilePaths() bool }

// ImageImporter marks a tool that consumes selected images directly.
type ImageImporter interface{ AcceptsImages() bool }

// TextInput identifies tools without a text editor.
type TextInput interface{ AcceptsText() bool }

// FileInput describes whether a tool accepts files through the picker.
type FileInput interface{ AcceptsFiles() bool }

// Tool 是所有 DevToys 工具必须实现的接口。
// 除元信息方法外，Tool 同时是 bubbletea 模型（Init/Update/View），
// 工具必须响应 tea.WindowSizeMsg 完成自身布局的自适应。
type Tool interface {
	// ID 返回全局唯一标识，形如 "encoders.base64"。
	ID() string
	// Title 返回工具显示名。
	Title() string
	// Category 返回类别标识，见 categoryTitles。
	Category() string
	// Icon 返回简短的文本标识，用于侧边栏展示。
	Icon() string
	// Description 返回一句话描述。
	Description() string

	Init() tea.Cmd
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() string
}

// ShortTitleProvider lets a tool use a concise name in lists while keeping its
// full title in the detail view.
type ShortTitleProvider interface {
	ShortTitle() string
}

// EscapeHandler marks a tool with a modal state that must receive Escape before
// the application shell uses it to leave the tool.
type EscapeHandler interface {
	HandlesEscape() bool
}

// ListTitle returns the tool's short title when it provides one.
func ListTitle(tool Tool) string {
	if titled, ok := tool.(ShortTitleProvider); ok && titled.ShortTitle() != "" {
		return titled.ShortTitle()
	}
	return tool.Title()
}

var (
	mu	sync.RWMutex
	byID	= map[string]Tool{}
)

// Register 注册一个工具。在包的 init() 中调用；ID 重复视为编程错误。
func Register(t Tool) {
	if t == nil {
		panic("toolkit: nil tool registered")
	}
	if t.ID() == "" {
		panic("toolkit: tool ID cannot be empty")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := byID[t.ID()]; dup {
		panic(fmt.Sprintf("toolkit: duplicate tool ID: %s", t.ID()))
	}
	byID[t.ID()] = t
}

// Get 按 ID 查找工具。
func Get(id string) (Tool, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := byID[id]
	return t, ok
}

// All 返回全部已注册工具，按类别顺序、再按标题排序，保证界面稳定。
func All() []Tool {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Tool, 0, len(byID))
	for _, t := range byID {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		oi, oj := CategoryOrder(out[i].Category()), CategoryOrder(out[j].Category())
		if oi != oj {
			return oi < oj
		}
		return ListTitle(out[i]) < ListTitle(out[j])
	})
	return out
}

// Count 返回已注册工具数量。
func Count() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(byID)
}
