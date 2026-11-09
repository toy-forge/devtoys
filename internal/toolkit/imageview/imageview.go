// Package imageview renders bounded terminal previews without requiring a graphics protocol.
package imageview

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

// View holds an image and its current display size in terminal cells.
type View struct {
	image		image.Image
	name		string
	width, height	int
	cached		string
	overlayCached	string
}

// New constructs a preview. The image is not modified.
func New(img image.Image, name string) *View {
	return &View{image: img, name: name, width: 80, height: 24}
}

// SetSize sets the available width and height in terminal cells.
func (v *View) SetSize(width, height int) {
	width, height = max(1, width), max(1, height)
	if v.width != width || v.height != height {
		v.cached, v.overlayCached = "", ""
	}
	v.width, v.height = width, height
}

// Render returns a fitted image using two vertical pixels per cell. On a
// monochrome terminal it uses luminance characters instead of color.
func (v *View) Render() string {
	if v.cached != "" {
		return v.cached
	}
	titleText := "[Back]  Image preview"
	if NativeAvailable() {
		titleText += "  ·  i native"
	}
	title := lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render(titleText)
	if v.height < 3 || v.image == nil || v.image.Bounds().Empty() {
		return ansi.Truncate(title, v.width, "")
	}
	b := v.image.Bounds()
	width, height := fit(b.Dx(), b.Dy(), v.width, v.height-2)
	rows := append([]string{title}, v.imageRows(width, height)...)
	rows = append(rows, lipgloss.NewStyle().Foreground(theme.Muted).Render(v.hintLine(b, "esc back")))
	for i, line := range rows {
		rows[i] = ansi.Truncate(line, v.width, "")
	}
	v.cached = strings.Join(rows, "\n")
	return v.cached
}

// Overlay renders the preview as a centered ui.Modal dimming and covering
// the given background (expected to span v.width × v.height cells).
func (v *View) Overlay(background string) string {
	return v.previewModal().Render(background)
}

// ModalRect returns the modal box rectangle within the content area so the
// host can translate mouse hits (clicks outside the box close the preview).
func (v *View) ModalRect() ui.Rect	{ return v.previewModal().Rect() }

// previewModal composes the centered modal; render and hit testing share it.
func (v *View) previewModal() ui.Modal {
	modal := ui.Modal{Kind: ui.KindCustom, Title: "Image preview"}
	modal.SetSize(v.width, v.height)
	if body := v.overlayBody(); body != "" {
		modal.Width = ui.Width(strings.Split(body, "\n")[0]) + 2
		modal.Body = body
	}
	return modal
}

// overlayBody is the modal body: fitted image rows plus a hint line.
func (v *View) overlayBody() string {
	if v.overlayCached != "" {
		return v.overlayCached
	}
	if v.image == nil || v.image.Bounds().Empty() || v.height < 8 || v.width < 12 {
		return ""
	}
	b := v.image.Bounds()
	width, height := fit(b.Dx(), b.Dy(), max(1, v.width-8), max(1, v.height-8))
	rows := v.imageRows(width, height)
	rows = append(rows, lipgloss.NewStyle().Foreground(theme.Muted).Render(v.hintLine(b, "esc close")))
	v.overlayCached = strings.Join(rows, "\n")
	return v.overlayCached
}

func (v *View) hintLine(b image.Rectangle, back string) string {
	return fmt.Sprintf("%s  ·  %d × %d  ·  %s", v.sanitizedName(), b.Dx(), b.Dy(), back)
}

func (v *View) sanitizedName() string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, v.name)
}

// imageRows renders width×height cells using two vertical pixels per cell.
func (v *View) imageRows(width, height int) []string {
	colorful := lipgloss.ColorProfile() != termenv.Ascii
	const shades = " .:-=+*#%@"
	rows := make([]string, 0, height)
	for y := 0; y < height; y++ {
		var line strings.Builder
		for x := 0; x < width; x++ {
			top := sample(v.image, x, y*2, width, height*2)
			bottom := sample(v.image, x, y*2+1, width, height*2)
			if colorful {
				fg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", top.R, top.G, top.B))
				bg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", bottom.R, bottom.G, bottom.B))
				line.WriteString(lipgloss.NewStyle().Foreground(fg).Background(bg).Render("▀"))
			} else {
				lum := (int(top.R)+int(bottom.R))*299 + (int(top.G)+int(bottom.G))*587 + (int(top.B)+int(bottom.B))*114
				line.WriteByte(shades[lum*(len(shades)-1)/(2*255*1000)])
			}
		}
		rows = append(rows, line.String())
	}
	return rows
}

func fit(srcW, srcH, maxW, maxH int) (int, int) {
	scale := math.Min(float64(maxW)/float64(srcW), float64(maxH*2)/float64(srcH))
	// Avoid enlarging a small source beyond one source pixel per terminal half-cell.
	scale = math.Min(scale, 1)
	return min(maxW, max(1, int(math.Round(float64(srcW)*scale)))), min(maxH, max(1, int(math.Ceil(float64(srcH)*scale/2))))
}

func sample(img image.Image, x, y, outW, outH int) color.NRGBA {
	b := img.Bounds()
	sx := b.Min.X + x*b.Dx()/outW
	sy := b.Min.Y + y*b.Dy()/outH
	// Composite alpha over the palette background rather than printing controls.
	c := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
	a := uint32(c.A)
	return color.NRGBA{R: uint8((uint32(c.R)*a + 13*(255-a)) / 255), G: uint8((uint32(c.G)*a + 17*(255-a)) / 255), B: uint8((uint32(c.B)*a + 23*(255-a)) / 255), A: 255}
}
