package toolkit

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"
)

// InsertFileText inserts a file only if the input can preserve it completely.
func InsertFileText(input *textarea.Model, text string) error {
	if err := input.CheckInsert(text); err != nil {
		return err
	}
	input.InsertString(text)
	return nil
}

// InsertLine 把导入的文本插入单行输入框（光标处）。单行字段不接受换行：
// 多行文本返回错误，宿主据此提示而不是静默截断。
func InsertLine(input *textinput.Model, text string) error {
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("file must contain one short line of text")
	}
	value := []rune(input.Value())
	pos := input.Position()
	inserted := []rune(text)
	next := append(append(append([]rune{}, value[:pos]...), inserted...), value[pos:]...)
	input.SetValue(string(next))
	input.SetCursor(pos + len(inserted))
	return nil
}

// PasteLineCmd 异步读取剪贴板，并以粘贴的按键消息回灌给单行输入框：换行压成空格，
// 与 editor 的 ctrl+v 处理同语义（读取失败以 PasteErrorMsg 上报）。
func PasteLineCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		if err != nil {
			return textarea.PasteErrorMsg{Err: fmt.Errorf("could not read clipboard: %w", err)}
		}
		if !utf8.ValidString(text) {
			return textarea.PasteErrorMsg{Err: fmt.Errorf("clipboard is not valid UTF-8")}
		}
		text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text)
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
	}
}
