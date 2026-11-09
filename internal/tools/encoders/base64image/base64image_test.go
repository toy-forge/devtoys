package base64image

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestDataURIRetainsOriginalBytesAndDecodes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	tool := newTool()
	tool.Update(toolkit.ImageImportMsg{Image: img, Name: "file.png", Data: data.Bytes()})
	text := tool.ws.Input.Value()
	payload := strings.TrimPrefix(text, "data:image/png;base64,")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || !bytes.Equal(raw, data.Bytes()) {
		t.Fatal("original image bytes were not preserved")
	}
	decoded, err := decode(context.Background(), text)
	if err != nil || decoded.Bounds().Dy() != 3 {
		t.Fatalf("decoded=%v err=%v", decoded, err)
	}
}
func TestRejectInvalidOrOversizedData(t *testing.T) {
	for _, s := range []string{"data:text/plain;base64,YQ==", "data:image/png;base64,!!!", strings.Repeat("A", base64.StdEncoding.EncodedLen(maxImageBytes)+2048)} {
		if _, err := decode(context.Background(), s); err == nil {
			t.Fatal("invalid data accepted")
		}
	}
}
func TestStaleResultIsIgnored(t *testing.T) {
	tool := newTool()
	tool.revision = 3
	tool.Update(decoded{revision: 2, err: context.Canceled})
	if tool.ws.Err != nil {
		t.Fatal("stale result changed current state")
	}
}

func TestMIMEUsesFileContent(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if got := imageMIME("misleading.jpg", data.Bytes()); got != "image/png" {
		t.Fatalf("MIME=%q", got)
	}
}
