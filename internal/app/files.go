package app

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "github.com/fyne-io/image/ico"
	_ "github.com/spakin/netpbm"
	"github.com/woozymasta/tga"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/imaging"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/toolkit/filepicker"
)

// files.go 负责文件选择器返回的读取工作（在 Cmd 中异步执行）：
// 文本、二进制、图片与批量图片的大小与格式校验。

type nativeImageResult struct{ err error }

type fileResult struct {
	picker	*filepicker.Picker
	text	string
	image	image.Image
	data	[]byte
	name	string
	err	error
}

const (
	maxFileBytes	= 1 << 20
	maxImageBytes	= 10 << 20
	maxImagePixels	= 16_000_000
)

type fileBatchResult struct {
	picker	*filepicker.Picker
	images	[]toolkit.ImageImportMsg
	err	error
}

func readSelectedImages(picker *filepicker.Picker, paths []string) tea.Cmd {
	return func() tea.Msg {
		result := fileBatchResult{picker: picker}
		total := 0
		for _, path := range paths {
			file := readSelectedFile(picker, path)().(fileResult)
			if file.err != nil {
				result.err = fmt.Errorf("%s: %w", filepath.Base(path), file.err)
				result.images = nil
				return result
			}
			if file.image == nil {
				result.err = fmt.Errorf("%s is not a supported image", filepath.Base(path))
				result.images = nil
				return result
			}
			b := file.image.Bounds()
			total += b.Dx() * b.Dy()
			if total > 64_000_000 {
				result.err = fmt.Errorf("selected images exceed 64 million pixels")
				result.images = nil
				return result
			}
			result.images = append(result.images, toolkit.ImageImportMsg{Image: file.image, Name: file.name, Data: file.data})
		}
		return result
	}
}

func readSelectedFile(picker *filepicker.Picker, path string) tea.Cmd {
	return func() tea.Msg {
		result := fileResult{picker: picker, name: filepath.Base(path)}
		info, err := os.Stat(path)
		if err == nil && !info.Mode().IsRegular() {
			err = fmt.Errorf("only regular files can be opened")
		}
		if err == nil && info.Size() > maxImageBytes {
			err = fmt.Errorf("file exceeds 10 MiB")
		}
		if err == nil {
			var f *os.File
			f, err = os.Open(path)
			if err == nil {
				var data []byte
				data, err = io.ReadAll(io.LimitReader(f, maxImageBytes+1))
				_ = f.Close()
				if err == nil && len(data) > maxImageBytes {
					err = fmt.Errorf("file exceeds 10 MiB")
				}
				if err == nil {
					if len(data) <= maxFileBytes {
						result.data = data
					}
					var cfg image.Config
					var imageErr error
					isTGA := false
					switch strings.ToLower(filepath.Ext(path)) {
					case ".svg":
						result.image, imageErr = imaging.DecodeSVG(data)
						if imageErr == nil {
							bounds := result.image.Bounds()
							cfg = image.Config{ColorModel: result.image.ColorModel(), Width: bounds.Dx(), Height: bounds.Dy()}
						}
					case ".tga":
						cfg, imageErr = tga.DecodeConfig(bytes.NewReader(data))
						isTGA = imageErr == nil
					default:
						cfg, _, imageErr = image.DecodeConfig(bytes.NewReader(data))
					}
					if imageErr == nil {
						if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImagePixels/cfg.Height {
							err = fmt.Errorf("image exceeds %d pixels", maxImagePixels)
						} else {
							if result.image != nil {
								// SVG was rasterized above.
							} else if isTGA {
								result.image, err = tga.Decode(bytes.NewReader(data))
							} else {
								result.image, _, err = image.Decode(bytes.NewReader(data))
							}
							if err == nil {
								result.data = data
							}
						}
					} else if strings.EqualFold(filepath.Ext(path), ".svg") {
						err = fmt.Errorf("could not decode SVG: %w", imageErr)
					} else if len(data) > maxFileBytes {
						err = fmt.Errorf("text file exceeds 1 MiB")
					} else if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
						err = fmt.Errorf("choose UTF-8 text or a supported image")
					} else {
						result.text = string(data)
					}
				}
			}
		}
		result.err = err
		return result
	}
}
