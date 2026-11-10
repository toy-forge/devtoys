package app

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/toolkit/filepicker"
	"github.com/woozymasta/tga"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type testTool struct {
	size	tea.WindowSizeMsg
	pasted	string
}

func (*testTool) ID() string		{ return "test.tool" }
func (*testTool) Title() string		{ return "Test" }
func (*testTool) Category() string	{ return "test" }
func (*testTool) Icon() string		{ return "T" }
func (*testTool) Description() string	{ return "" }
func (*testTool) Init() tea.Cmd		{ return nil }
func (t *testTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		t.size = size
	}
	if file, ok := msg.(toolkit.FileImportMsg); ok {
		t.pasted = file.Text
	}
	return t, nil
}
func (*testTool) View() string	{ return "test" }

func TestMouseOpensToolAndResizeIsForwarded(t *testing.T) {
	tool := &testTool{}
	m := New()
	m.tools.SetItems([]list.Item{listItem{kind: itemTool, tool: tool, title: "Test"}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	// 宽屏：侧栏 28 列 + 左留白 2 列，内容区第一行在页头之后（Y=browseHeaderRows）。
	next, _ = m.Update(tea.MouseMsg{X: 31, Y: browseHeaderRows, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	// 宽 100 - 28 - 左右留白 4 = 68；高 30 - 1 状态栏 - 2 页头 = 27。
	if m.active != tool || tool.size.Width != 68 || tool.size.Height != 27 {
		t.Fatalf("click did not open and size tool: active=%v size=%+v", m.active, tool.size)
	}
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if tool.size.Width != 88 || tool.size.Height != 37 {
		t.Fatalf("resize not forwarded: %+v", tool.size)
	}
}

func TestFileImportAndCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0600); err != nil {
		t.Fatal(err)
	}
	tool := &testTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	m.width = 100
	m.height = 30
	m.picker = filepicker.New(dir)
	m.picker.SetSize(68, 29)
	// picker 弹窗盒内（局部 X≈4..63）第 2 个文件行（Y=4）是 input.txt。
	next, cmd := m.Update(tea.MouseMsg{X: 42, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected file read command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.picker != nil || tool.pasted != "hello\nworld" {
		t.Fatalf("picker=%v pasted=%q", m.picker, tool.pasted)
	}

	m.picker = filepicker.New(dir)
	old := m.picker
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	next, _ = m.Update(fileResult{picker: old, text: "stale"})
	m = next.(Model)
	if tool.pasted != "hello\nworld" || m.picker != nil {
		t.Fatal("cancelled read was applied")
	}
}

func TestFileReadShowsLoadingAndClearsOnResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	tool := &testTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	m.width = 100
	m.height = 30
	m.picker = filepicker.New(dir)
	m.picker.SetSize(68, 29)
	next, cmd := m.Update(tea.MouseMsg{X: 42, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd == nil || !m.loading {
		t.Fatalf("file read not started: cmd=%v loading=%v", cmd != nil, m.loading)
	}
	if view := m.picker.View(); !strings.Contains(view, "Opening input.txt") || !strings.Contains(view, "esc cancels") {
		t.Fatalf("loading state not visible in picker: %s", view)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.loading || m.picker != nil || tool.pasted != "hello" {
		t.Fatalf("loading did not resolve: loading=%v picker=%v pasted=%q", m.loading, m.picker != nil, tool.pasted)
	}
}

func TestToolStatusHintsHaveNoDuplicates(t *testing.T) {
	seenTools := 0
	for _, tool := range toolkit.All() {
		m := New()
		m.mode, m.active = modeTool, tool
		next, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
		m = next.(Model)
		seen := map[string]bool{}
		for _, hint := range m.statusHints() {
			if seen[hint.Key] {
				t.Fatalf("%s: duplicate hint key %q", tool.ID(), hint.Key)
			}
			seen[hint.Key] = true
		}
		seenTools++
	}
	if seenTools != toolkit.Count() {
		t.Fatalf("checked %d tools, want %d", seenTools, toolkit.Count())
	}
}

func TestOutputOnlyToolsDoNotAdvertiseFocusToggle(t *testing.T) {
	for _, id := range []string{"generators.lorem", "generators.uuid"} {
		tool, ok := toolkit.Get(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if !toolkit.OutputOnly(tool) {
			t.Fatalf("%s should report OutputOnly", id)
		}
		m := New()
		m.mode, m.active = modeTool, tool
		next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
		m = next.(Model)
		for _, hint := range m.statusHints() {
			if hint.Key == "tab" {
				t.Fatalf("%s advertises a focus toggle it does not have", id)
			}
		}
	}
}

func TestBespokeToolDrawsOnlyOneHintRow(t *testing.T) {
	// 工具自带页脚只保留按钮/结论；快捷键统一由外壳状态栏展示。
	for _, id := range []string{"text.comparer", "testers.regex", "converters.dateparser", "converters.numberbase"} {
		tool, ok := toolkit.Get(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		m := New()
		m.mode, m.active = modeTool, tool
		next, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
		m = next.(Model)
		view := ansi.Strip(m.View())
		if strings.Count(view, "click to edit") != 0 {
			t.Fatalf("%s still draws a duplicate hint sentence", id)
		}
		if strings.Count(view, "tab focus") > 1 {
			t.Fatalf("%s draws the focus hint more than once", id)
		}
	}
}

func TestReadTextFileRejectsBinaryAndLargeFiles(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name	string
		data	[]byte
	}{{"binary", []byte{'a', 0, 'b'}}, {"large", bytes.Repeat([]byte{'x'}, maxFileBytes+1)}} {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, tc.data, 0600); err != nil {
			t.Fatal(err)
		}
		result := readSelectedFile(filepicker.New(dir), path)().(fileResult)
		if result.err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

func TestCompactToolUsesFullWidthAndCanReturnToTools(t *testing.T) {
	tool := &testTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	// 40 列低于 railMin：侧栏完全隐藏，工具占满整行。
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = next.(Model)
	// 40 - 左右留白 4 = 36。
	if tool.size.Width != 36 || strings.Contains(m.View(), "DevToys") {
		t.Fatalf("tool width=%d or sidebar visible", tool.size.Width)
	}
	next, _ = m.Update(tea.MouseMsg{X: 2, Y: 19, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.mode != modeBrowse {
		t.Fatal("tools button did not return to browse")
	}
}

// 中屏收窄为图标轨：工具不再是整行宽，但侧栏依然可见。
func TestMediumScreenUsesIconRail(t *testing.T) {
	tool := &testTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	if cols := m.sidebarColumns(); cols != railWidth {
		t.Fatalf("sidebarColumns=%d, want rail %d", cols, railWidth)
	}
	if want := 60 - railWidth - 2*contentPad; tool.size.Width != want {
		t.Fatalf("tool width=%d, want %d", tool.size.Width, want)
	}
	// 图标轨第一行（无边框，Y=0）是 All tools，点击后回到浏览页。
	next, _ = m.Update(tea.MouseMsg{X: 2, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.mode != modeBrowse || m.category != "" {
		t.Fatal("rail logo did not return to browse")
	}
}

func TestPickerCanBeCancelledWithMouse(t *testing.T) {
	m := New()
	m.mode = modeTool
	m.active = &testTool{}
	m.width = 40
	m.height = 20
	m = m.openPicker()
	// picker 弹窗在 40×19 区域居中：[Cancel] 按钮在标题行右侧（X≈28..35，Y=1）。
	next, _ := m.Update(tea.MouseMsg{X: 30, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if next.(Model).picker != nil {
		t.Fatal("picker still open")
	}
}

func TestImagePreviewReturnsToPicker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "picture.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	tool := &testTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	m.width = 60
	m.height = 20
	m.picker = filepicker.New(dir)
	// 60 列为中屏：侧栏是 6 列图标轨，内容区 54 列。
	m.picker.SetSize(54, 19)
	// 点击 picker 弹窗盒内的第 2 个文件行（局部 (10,4)，全局 X = 10 + railWidth）。
	next, cmd := m.Update(tea.MouseMsg{X: 10 + railWidth, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected read command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.preview == nil || m.picker == nil || tool.pasted != "" {
		t.Fatal("image was not shown as preview")
	}
	if !strings.Contains(m.View(), "Image preview") {
		t.Fatal("preview title missing")
	}
	// 预览是居中 Modal：点击盒子外（内容区左上角）关闭并回到 picker。
	next, _ = m.Update(tea.MouseMsg{X: 2 + railWidth, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.preview != nil || m.picker == nil {
		t.Fatal("back did not return to picker")
	}
}

func TestImageDimensionsRejectedBeforeDecode(t *testing.T) {
	var data bytes.Buffer
	data.Write([]byte("\x89PNG\r\n\x1a\n"))
	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[:4], 20000)
	binary.BigEndian.PutUint32(ihdr[4:8], 1000)
	ihdr[8], ihdr[9] = 8, 2
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ihdr)))
	data.Write(length[:])
	chunk := append([]byte("IHDR"), ihdr[:]...)
	data.Write(chunk)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc32.ChecksumIEEE(chunk))
	data.Write(sum[:])
	path := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	result := readSelectedFile(filepicker.New(filepath.Dir(path)), path)().(fileResult)
	if result.err == nil || !strings.Contains(result.err.Error(), "image exceeds") {
		t.Fatalf("error=%v", result.err)
	}
}

func TestClipboardErrorIsVisible(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 20
	next, _ := m.Update(editor.PasteErrorMsg{Err: errors.New("clipboard unavailable")})
	m = next.(Model)
	if !strings.Contains(m.View(), "clipboard unavailable") {
		t.Fatal("clipboard error is not visible")
	}
}

func TestReadTGAFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.tga")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := tga.Encode(f, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	result := readSelectedFile(filepicker.New(dir), path)().(fileResult)
	if result.err != nil || result.image == nil || result.image.Bounds().Dx() != 3 {
		t.Fatalf("TGA result=%+v", result)
	}
}

func TestReadSelectedImagesBatchAndErrors(t *testing.T) {
	dir := t.TempDir()
	paths := []string{}
	for _, name := range []string{"one.png", "two.png"} {
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	picker := filepicker.New(dir)
	result := readSelectedImages(picker, paths)().(fileBatchResult)
	if result.err != nil || result.picker != picker || len(result.images) != 2 || result.images[1].Name != "two.png" {
		t.Fatalf("batch=%+v", result)
	}
	bad := filepath.Join(dir, "not-image.txt")
	if err := os.WriteFile(bad, []byte("plain text"), 0600); err != nil {
		t.Fatal(err)
	}
	result = readSelectedImages(picker, []string{paths[0], bad})().(fileBatchResult)
	if result.err == nil || len(result.images) != 0 {
		t.Fatalf("bad batch was partially delivered: %+v", result)
	}
}

type batchTestTool struct {
	testTool
	images	[]toolkit.ImageImportMsg
}

func (*batchTestTool) AcceptsImages() bool	{ return true }
func (*batchTestTool) AcceptsImageBatch() bool	{ return true }
func (t *batchTestTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if event, ok := msg.(toolkit.ImageBatchImportMsg); ok {
		t.images = event.Images
		return t, nil
	}
	_, cmd := t.testTool.Update(msg)
	return t, cmd
}
func TestBatchPickerDeliversImagesToTool(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.png", "b.png"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	tool := &batchTestTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	m.picker = filepicker.New(dir)
	m.picker.SetMultiple(true)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeySpace}, {Type: tea.KeyDown}, {Type: tea.KeySpace}} {
		next, _ := m.Update(key)
		m = next.(Model)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = next.(Model)
	if cmd == nil || !m.loading {
		t.Fatal("batch read not started")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.picker != nil || len(tool.images) != 2 || tool.images[0].Name != "a.png" || tool.images[1].Name != "b.png" {
		t.Fatalf("batch not delivered: picker=%v images=%+v", m.picker, tool.images)
	}
}

func TestForegroundCommandRunsOnlyForActiveVisibleTool(t *testing.T) {
	tool := &testTool{}
	other := &batchTestTool{}
	m := New()
	m.mode = modeTool
	m.active = tool
	ran, dropped := false, 0
	request := toolkit.ForegroundCommandMsg{Tool: tool, Command: func() tea.Msg { ran = true; return nil }, Valid: func() bool { return true }, OnDrop: func() { dropped++ }}
	_, cmd := m.Update(request)
	if cmd == nil {
		t.Fatal("active command rejected")
	}
	cmd()
	if !ran {
		t.Fatal("active command not returned")
	}
	m.active = other
	_, cmd = m.Update(request)
	if cmd != nil || dropped != 1 {
		t.Fatal("inactive tool opened a preview")
	}
	m.active = tool
	m.picker = filepicker.New(t.TempDir())
	_, cmd = m.Update(request)
	if cmd != nil || dropped != 2 {
		t.Fatal("preview opened over file picker")
	}
	m.picker = nil
	request.Valid = func() bool { return false }
	_, cmd = m.Update(request)
	if cmd != nil || dropped != 3 {
		t.Fatal("stale request not discarded")
	}
}

type pathTestTool struct {
	testTool
	path	string
}

func (*pathTestTool) AcceptsFilePaths() bool	{ return true }
func (t *pathTestTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if selected, ok := msg.(toolkit.FilePathImportMsg); ok {
		t.path = selected.Path
		return t, nil
	}
	_, cmd := t.testTool.Update(msg)
	return t, cmd
}
func TestPickerDeliversFilePathWithoutBuffering(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.dat")
	if err := os.WriteFile(path, []byte{0, 0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	tool := &pathTestTool{}
	m := New()
	m.active = tool
	m.mode = modeTool
	m.width = 100
	m.height = 30
	m.picker = filepicker.New(dir)
	m.picker.SetSize(68, 29)
	next, cmd := m.Update(tea.MouseMsg{X: 42, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd != nil || m.picker != nil || tool.path != path {
		t.Fatalf("path=%q picker=%v cmd=%v", tool.path, m.picker, cmd != nil)
	}
}
