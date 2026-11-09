// Package imaging decodes vector image formats used by image tools.
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"math"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const maxSVGImagePixels = 16_000_000
const maxSVGImageDimension = 4096

// DecodeSVG rasterizes an SVG into a bounded RGBA image for terminal previews.
func DecodeSVG(data []byte) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid SVG image: %w", err)
	}
	w, h := icon.ViewBox.W, icon.ViewBox.H
	if math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("SVG must have a valid viewBox or width and height")
	}
	scale := min(1, float64(maxSVGImageDimension)/w, float64(maxSVGImageDimension)/h)
	scale = min(scale, math.Sqrt(float64(maxSVGImagePixels)/(w*h)))
	w *= scale
	h *= scale
	width, height := max(1, int(math.Ceil(w))), max(1, int(math.Ceil(h)))
	if width > maxSVGImagePixels/height {
		return nil, fmt.Errorf("SVG exceeds %d pixels", maxSVGImagePixels)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	icon.SetTarget(0, 0, float64(width), float64(height))
	icon.Draw(rasterx.NewDasher(width, height, rasterx.NewScannerGV(width, height, canvas, canvas.Bounds())), 1)
	return canvas, nil
}
