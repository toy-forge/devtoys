// DevToys TUI：基于 bubbletea 的开发者小工具箱。
//
// 组装流程：空导入 internal/tools 触发全部工具自注册，
// 然后把 app 主模型交给 bubbletea 以全屏模式运行。
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/app"

	// 触发所有工具包的 init() 自注册。
	_ "github.com/toy-forge/devtoys/internal/tools"
)

func main() {
	program := tea.NewProgram(app.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "devtoys:", err)
		os.Exit(1)
	}
}
