// Package base64image converts images to data URIs and previews encoded images.
package base64image

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "github.com/fyne-io/image/ico"
	_ "github.com/spakin/netpbm"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/imaging"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
	"github.com/toy-forge/devtoys/internal/toolkit/imageview"
	"github.com/woozymasta/tga"
)

const maxImageBytes = 10 << 20
const maxImagePixels = 16_000_000

type tool struct {
	ws		toolkit.Workspace
	img		image.Image
	name, before	string
	revision	uint64
	cancel		context.CancelFunc
	width, height	int
}
type due struct{ revision uint64 }
type decoded struct {
	revision	uint64
	img		image.Image
	err		error
}
type previewDone struct{ err error }

func newTool() *tool {
	t := &tool{ws: toolkit.NewWorkspace("Paste a Base64 image or open an image file…", "Paste a data URI or open an image file to preview it.")}
	t.ws.SetSplitRatio(2, 1)
	t.ws.SetPaneTitles("Base64 text", "Image preview")
	t.ws.SetInputControls("Copy")
	t.ws.SetOutputAccessory(
		toolkit.Actions("＋ Open", "󰈈 Preview"),
		"＋ Open",
		"󰈈 Preview",
	)
	return t
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "encoders.base64image" }
func (*tool) Title() string		{ return "Base64 Image Encoder / Decoder" }
func (*tool) ShortTitle() string	{ return "Base64 Image" }
func (*tool) Category() string		{ return "encoders" }
func (*tool) Icon() string		{ return "󰋩" }
func (*tool) Description() string	{ return "Encode an image as a data URI or preview a Base64 image" }

func (t *tool) PaneStatusHints() []ui.StatusHint { return t.ws.StatusHints() }
func (*tool) AcceptsImages() bool	{ return true }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		if event.Width < 64 {
			t.ws.SetSplitRatio(1, 1)
		} else {
			t.ws.SetSplitRatio(2, 1)
		}
		cmd := t.ws.Update(event)
		t.render()
		return t, cmd
	case toolkit.WorkspaceActionMsg:
		switch event.Action {
		case "Output:＋ Open":
			return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case "Output:󰈈 Preview":
			return t, t.nativePreview()
		}
	case toolkit.ImageImportMsg:
		if event.Image == nil {
			t.ws.SetError(fmt.Errorf("invalid image"))
			return t, nil
		}
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		t.revision++
		data := event.Data
		if len(data) == 0 {
			var b bytes.Buffer
			if err := png.Encode(&b, event.Image); err != nil {
				t.ws.SetError(fmt.Errorf("could not encode image: %w", err))
				return t, nil
			}
			data = b.Bytes()
		}
		if len(data) > maxImageBytes {
			t.ws.SetError(fmt.Errorf("image exceeds 10 MiB"))
			return t, nil
		}
		encoded := "data:" + imageMIME(event.Name, data) + ";base64," + base64.StdEncoding.EncodeToString(data)
		t.ws.Input.SetValue(encoded)
		t.before = encoded
		t.img = event.Image
		t.name = safe(event.Name)
		t.ws.SetError(nil)
		t.render()
		return t, nil
	case due:
		if event.revision != t.revision {
			return t, nil
		}
		text := t.ws.Input.Value()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.cancel = cancel
		return t, func() tea.Msg {
			img, err := decode(ctx, text)
			return toolkit.BackgroundToolMsg{Tool: t, Message: decoded{revision: event.revision, img: img, err: err}}
		}
	case decoded:
		if event.revision != t.revision {
			return t, nil
		}
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		t.img = event.img
		t.name = "Decoded image"
		t.ws.SetError(event.err)
		t.render()
		return t, nil
	case previewDone:
		if event.err != nil {
			t.ws.SetError(event.err)
		}
		return t, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "alt+i" {
		return t, t.nativePreview()
	}
	cmd := t.ws.Update(msg)
	if _, ok := msg.(toolkit.FileImportMsg); ok && t.ws.Err != nil {
		return t, cmd
	}
	if input := t.ws.Input.Value(); input != t.before {
		t.before = input
		t.revision++
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		if strings.TrimSpace(input) == "" {
			t.img = nil
			t.ws.SetError(nil)
			t.render()
			return t, cmd
		}
		t.img = nil
		t.ws.SetError(nil)
		t.ws.SetOutput("Decoding image…")
		rev := t.revision
		pending := tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: due{revision: rev}} })
		return t, tea.Batch(cmd, pending)
	}
	return t, cmd
}
func (t *tool) nativePreview() tea.Cmd {
	if t.img == nil {
		t.ws.SetError(fmt.Errorf("no image to preview"))
		return nil
	}
	if !imageview.NativeAvailable() {
		t.ws.SetError(fmt.Errorf("native preview requires Ghostty or Kitty outside tmux"))
		return nil
	}
	v := imageview.New(t.img, t.name)
	v.SetSize(max(1, t.width), max(4, t.height))
	return v.Native(func(err error) tea.Msg { return previewDone{err} })
}
func (t *tool) View() string	{ return t.ws.View() }
func (t *tool) render() {
	if t.img == nil {
		t.ws.SetOutputDisplay("", "")
		return
	}
	bounds := t.img.Bounds()
	summary := fmt.Sprintf("%s · %d × %d", t.name, bounds.Dx(), bounds.Dy())
	preview := imageview.New(t.img, t.name)
	preview.SetSize(max(1, t.ws.Output.Width), max(3, t.ws.Output.Height-1))
	lines := strings.Split(preview.Render(), "\n")
	display := summary
	if len(lines) > 2 {
		display += "\n" + strings.Join(lines[1:len(lines)-1], "\n")
	}
	t.ws.SetOutputDisplay(summary, display)
}
func imageMIME(name string, data []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		switch format {
		case "png":
			return "image/png"
		case "jpeg":
			return "image/jpeg"
		case "gif":
			return "image/gif"
		case "bmp":
			return "image/bmp"
		case "webp":
			return "image/webp"
		case "ico":
			return "image/x-icon"
		case "tiff":
			return "image/tiff"
		case "pbm":
			return "image/x-portable-bitmap"
		case "pgm":
			return "image/x-portable-graymap"
		case "ppm":
			return "image/x-portable-pixmap"
		case "pam":
			return "image/x-portable-arbitrarymap"
		}
	}
	if strings.EqualFold(filepath.Ext(name), ".tga") {
		return "image/x-tga"
	}
	return "application/octet-stream"
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}
func decode(ctx context.Context, text string) (image.Image, error) {
	if len(text) > base64.StdEncoding.EncodedLen(maxImageBytes)+1024 {
		return nil, fmt.Errorf("image exceeds 10 MiB")
	}
	text = strings.TrimSpace(text)
	isTGA := false
	isSVG := false
	if strings.HasPrefix(strings.ToLower(text), "data:") {
		header, body, ok := strings.Cut(text, ",")
		if !ok || !strings.HasPrefix(strings.ToLower(header), "data:image/") || !strings.HasSuffix(strings.ToLower(header), ";base64") {
			return nil, fmt.Errorf("expected a Base64 image data URI")
		}
		isTGA = strings.HasPrefix(strings.ToLower(header), "data:image/x-tga;")
		isSVG = strings.HasPrefix(strings.ToLower(header), "data:image/svg+xml;")
		text = body
	}
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("invalid Base64 data: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds 10 MiB")
	}
	if isSVG || looksLikeSVG(data) {
		img, err := imaging.DecodeSVG(data)
		if err != nil {
			return nil, fmt.Errorf("could not decode SVG: %w", err)
		}
		return img, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil && isTGA {
		cfg, err = tga.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil {
		return nil, fmt.Errorf("unsupported or invalid image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImagePixels/cfg.Height {
		return nil, fmt.Errorf("image exceeds %d pixels", maxImagePixels)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil && isTGA {
		img, err = tga.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, fmt.Errorf("could not decode image: %w", err)
	}
	return img, ctx.Err()
}

func looksLikeSVG(data []byte) bool {
	trimmed := strings.TrimSpace(strings.TrimPrefix(string(data), "\uFEFF"))
	trimmed = strings.TrimPrefix(trimmed, "<?xml")
	if i := strings.Index(trimmed, "<svg"); i >= 0 && i < 256 {
		return true
	}
	return false
}
