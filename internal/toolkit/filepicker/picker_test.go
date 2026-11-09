package filepicker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/toy-forge/devtoys/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

var press = tea.MouseActionPress

func TestNavigateAndSelectWithMouse(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "folder")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "example.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	p := New(dir)
	p.SetSize(30, 8)
	// 30 列区域 → 盒宽 28、居中左偏移 1；Modal 正文左边框 1 列 +
	// 内边距 2 列，内容首列在 X=4；标题栏 + 面包屑各占 1 行，
	// 第一条目行在盒内第 3 行（picker 局部 Y=3+box.Y=3），第 2 行是 folder。
	if got := p.Update(tea.MouseMsg{X: 4, Y: 4, Button: tea.MouseButtonLeft, Action: press}); got != "" || p.dir != sub {
		t.Fatalf("open directory: %q %q", got, p.dir)
	}
	if got := p.Update(tea.MouseMsg{X: 4, Y: 4, Button: tea.MouseButtonLeft, Action: press}); got != path {
		t.Fatalf("selected %q", got)
	}
}

func TestViewIsCenteredModalOfFullArea(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	p.SetSize(30, 8)
	view := p.View()
	if lines := strings.Count(view, "\n") + 1; lines != 8 {
		t.Fatalf("view height = %d, want 8", lines)
	}
	if !strings.Contains(view, "Open a file") || !strings.Contains(view, "Cancel") {
		t.Fatalf("modal chrome missing:\n%s", view)
	}
	// 盒宽 28 → 左右各留 1 列：顶行边框落在第 1 列（先剥掉压暗用 ANSI 前缀）。
	var ansiCode = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if top := ansiCode.ReplaceAllString(strings.Split(view, "\n")[0], ""); !strings.HasPrefix(top, " ╭") {
		t.Fatalf("modal not centered at 30 columns: %q", top)
	}
}

func TestCancelHitUsesModalRect(t *testing.T) {
	p := New(t.TempDir())
	p.SetSize(30, 8)
	// 30x8 时盒宽 28（X=1 起）；正文首列 = 边框 1 + 内边距 2（X=4），
	// [Cancel] 位于标题栏（Y=1）右端 8 列（X∈[18,26)）。
	if !p.CancelAt(tea.MouseMsg{X: 18, Y: 1, Button: tea.MouseButtonLeft, Action: press}) {
		t.Fatal("cancel button not hit")
	}
	if !p.CancelAt(tea.MouseMsg{X: 25, Y: 1, Button: tea.MouseButtonLeft, Action: press}) {
		t.Fatal("cancel button right edge not hit")
	}
	if p.CancelAt(tea.MouseMsg{X: 17, Y: 1, Button: tea.MouseButtonLeft, Action: press}) {
		t.Fatal("false positive left of cancel button")
	}
	if p.CancelAt(tea.MouseMsg{X: 18, Y: 3, Button: tea.MouseButtonLeft, Action: press}) {
		t.Fatal("false positive outside title bar")
	}
	if p.CancelAt(tea.MouseMsg{X: 18, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}) {
		t.Fatal("non-press event must not cancel")
	}
}

func TestOpenSelectedButton(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.png")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	picker := New(root)
	picker.SetMultiple(true)
	picker.SetSize(40, 10)
	// 无选中项时按钮禁用：点击 [Open selected]（X∈[21,36), Y=1）无效。
	picker.Update(tea.MouseMsg{X: 25, Y: 1, Button: tea.MouseButtonLeft, Action: press})
	if picker.TakeSelection() != nil {
		t.Fatal("disabled open button confirmed selection")
	}
	picker.cursor = 1	// a.png
	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	picker.Update(tea.MouseMsg{X: 25, Y: 1, Button: tea.MouseButtonLeft, Action: press})
	paths := picker.TakeSelection()
	if len(paths) != 1 || paths[0] != path {
		t.Fatalf("selected %#v", paths)
	}
	if picker.TakeSelection() != nil {
		t.Fatal("selection delivered twice")
	}
}

func TestUnreadableDirectoryLeavesCurrentLocation(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	p.open(filepath.Join(dir, "missing"))
	if p.dir != dir || p.err == nil || !strings.Contains(p.View(), "Error") {
		t.Fatalf("dir=%q err=%v", p.dir, p.err)
	}
}

func TestNavigateSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, filepath.Join(root, "shortcut")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	p := New(root)
	for i, e := range p.entries {
		if e.name == "shortcut" {
			p.cursor = i
			if !e.dir {
				t.Fatal("symlink directory not recognized")
			}
			p.selectEntry()
			if p.dir != filepath.Join(root, "shortcut") {
				t.Fatalf("opened %s", p.dir)
			}
			return
		}
	}
	t.Fatal("shortcut not listed")
}

func TestMultipleSelectionAcrossDirectories(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "dir")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a.png")
	b := filepath.Join(sub, "b.png")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	picker := New(root)
	picker.SetMultiple(true)
	picker.SetSize(40, 10)
	find := func(name string) {
		t.Helper()
		for i, e := range picker.entries {
			if e.name == name {
				picker.cursor = i
				return
			}
		}
		t.Fatalf("missing %s", name)
	}
	find("a.png")
	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	view := picker.View()
	if !strings.Contains(view, "a.png") || !strings.Contains(view, theme.Icon(theme.IconSuccess)) {
		t.Fatalf("selection not visible:\n%s", view)
	}
	find("dir")
	picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	find("b.png")
	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	picker.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	paths := picker.TakeSelection()
	if len(paths) != 2 || paths[0] != a || paths[1] != b {
		t.Fatalf("selected %#v", paths)
	}
	if picker.TakeSelection() != nil {
		t.Fatal("selection delivered twice")
	}
}
func TestMultipleSelectionLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 17; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%02d.png", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	picker := New(root)
	picker.SetMultiple(true)
	for i := 1; i < len(picker.entries); i++ {
		picker.cursor = i
		picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	}
	if len(picker.selection) != 16 || picker.err == nil {
		t.Fatalf("selection count=%d error=%v", len(picker.selection), picker.err)
	}
}
