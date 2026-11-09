package imageview

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderFitsAndSanitizesFilename(t *testing.T) {
	img := image.NewNRGBA(image.Rect(5, 7, 25, 47))
	for y := 7; y < 47; y++ {
		for x := 5; x < 25; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	v := New(img, "evil\x1b[31m.png")
	for _, size := range [][2]int{{10, 8}, {1, 1}, {60, 20}} {
		v.SetSize(size[0], size[1])
		out := v.Render()
		if lipgloss.Height(out) > size[1] {
			t.Fatalf("height %d > %d", lipgloss.Height(out), size[1])
		}
		for _, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > size[0] {
				t.Fatalf("width %d > %d", lipgloss.Width(line), size[0])
			}
		}
		if strings.Contains(out, "evil\x1b") {
			t.Fatal("filename control sequence was retained")
		}
	}
}

func TestSampleCompositesTransparentPixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 0})
	img.SetNRGBA(0, 1, color.NRGBA{G: 255, A: 255})
	if got := sample(img, 0, 0, 1, 2); got != (color.NRGBA{R: 13, G: 17, B: 23, A: 255}) {
		t.Fatalf("transparent=%v", got)
	}
	if got := sample(img, 0, 1, 1, 2); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("opaque=%v", got)
	}
}

func TestMonochromeFallbackDistinguishesBrightness(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(previous)
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		img.SetNRGBA(0, y, color.NRGBA{A: 255})
		img.SetNRGBA(1, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	v := New(img, "test.png")
	v.SetSize(20, 5)
	lines := strings.Split(v.Render(), "\n")
	if len(lines) < 2 || lines[1] != " @" {
		t.Fatalf("monochrome pixels: %q", lines)
	}
}

func TestOverlayRendersCenteredModal(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	v := New(img, "test.png")
	const areaW, areaH = 60, 20
	v.SetSize(areaW, areaH)
	background := strings.Repeat(strings.Repeat("~", areaW)+"\n", areaH)
	out := v.Overlay(strings.TrimSuffix(background, "\n"))
	if lipgloss.Height(out) != areaH {
		t.Fatalf("overlay height %d, want %d", lipgloss.Height(out), areaH)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > areaW {
			t.Fatalf("overlay line width %d > %d", lipgloss.Width(line), areaW)
		}
	}
	if !strings.Contains(out, "Image preview") {
		t.Fatal("modal title missing")
	}
	// 渲染与命中共用同一份 Rect：盒子必须落在区域内且不贴边。
	r := v.ModalRect()
	if r.X <= 0 || r.Y <= 0 || r.Right() > areaW || r.Bottom() > areaH {
		t.Fatalf("modal rect %+v outside %dx%d area", r, areaW, areaH)
	}
	// 盒子居中：左右留白接近。
	if left, right := r.X, areaW-r.Right(); left-right > 1 || right-left > 1 {
		t.Fatalf("modal not centered: left=%d right=%d", left, right)
	}
}

func TestOverlayConsistentWithModalRect(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	v := New(img, "tiny.png")
	v.SetSize(40, 12)
	r := v.ModalRect()
	background := strings.Repeat(strings.Repeat(".", 40)+"\n", 12)
	out := v.Overlay(strings.TrimSuffix(background, "\n"))
	lines := strings.Split(out, "\n")
	// ModalRect 覆盖的行里必须出现弹窗内容（标题或图片）。
	for y := r.Y; y < r.Bottom() && y < len(lines); y++ {
		if strings.Contains(lines[y], "Image preview") {
			return
		}
	}
	t.Fatalf("modal rect %+v does not match render:\n%s", r, out)
}

func TestNativeCommandWritesKittyImageAndCleansUp(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	var out strings.Builder
	cmd := &nativeCommand{image: img, width: 20, height: 10}
	cmd.SetStdin(strings.NewReader("\n"))
	cmd.SetStdout(&out)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b_Ga=T,f=100,i=") || !strings.Contains(got, "c=20,r=7,q=2,m=0;") || !strings.Contains(got, "\x1b_Ga=d,d=i,i=") || !strings.HasSuffix(got, "\x1b[?1049l") {
		t.Fatalf("unexpected protocol output: %q", got)
	}
}

func TestNativeAvailability(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TMUX", "")
	if !NativeAvailable() {
		t.Fatal("Ghostty should enable native preview")
	}
	t.Setenv("TMUX", "session")
	if NativeAvailable() {
		t.Fatal("tmux should use the fallback")
	}
}

func TestPreparedNativeDataRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 100})
	payload, err := PrepareNativeData(img)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 20, G: 40, B: 60, A: 100}) {
		t.Fatalf("decoded %+v", got)
	}
	var output strings.Builder
	command := &nativeCommand{payload: payload, width: 10, height: 6}
	command.SetStdin(strings.NewReader("\n"))
	command.SetStdout(&output)
	if err = command.Run(); err != nil || !strings.Contains(output.String(), payload) {
		t.Fatalf("prepared protocol: %v", err)
	}
}
