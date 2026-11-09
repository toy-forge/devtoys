package imageview

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// NativeAvailable reports terminals known to implement the Kitty graphics protocol.
func NativeAvailable() bool {
	return os.Getenv("TMUX") == "" && (os.Getenv("TERM_PROGRAM") == "ghostty" || strings.HasPrefix(os.Getenv("TERM"), "xterm-kitty"))
}

// Native opens the image on a temporary terminal screen and returns on Enter.
func (v *View) Native(done func(error) tea.Msg) tea.Cmd {
	return tea.Exec(&nativeCommand{image: v.image, width: v.width, height: v.height}, done)
}

// PrepareNativeData encodes an image for the Kitty graphics protocol. It can
// run in a background command before the terminal is suspended.
func PrepareNativeData(img image.Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("no image to preview")
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data.Bytes()), nil
}

// NativePrepared opens a preview whose image data was encoded in the background.
func (v *View) NativePrepared(payload string, done func(error) tea.Msg) tea.Cmd {
	return tea.Exec(&nativeCommand{payload: payload, width: v.width, height: v.height}, done)
}

// NativeGridPrepared opens four pre-encoded images in a two-by-two terminal grid.
func NativeGridPrepared(payloads, titles []string, width, height int, done func(error) tea.Msg) tea.Cmd {
	return tea.Exec(&nativeGridCommand{payloads: payloads, titles: titles, width: width, height: height}, done)
}

type nativeGridCommand struct {
	payloads	[]string
	titles		[]string
	width		int
	height		int
	in		io.Reader
	out		io.Writer
}

func (c *nativeGridCommand) SetStdin(r io.Reader)	{ c.in = r }
func (c *nativeGridCommand) SetStdout(w io.Writer)	{ c.out = w }
func (c *nativeGridCommand) SetStderr(io.Writer)	{}
func (c *nativeGridCommand) Run() error {
	if len(c.payloads) != 4 || len(c.titles) != 4 {
		return fmt.Errorf("native image grid requires four images and titles")
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return err
	}
	ids := [4]uint32{}
	for i := range ids {
		ids[i] = binary.BigEndian.Uint32(idBytes[i*4:i*4+4]) | 1
	}
	if _, err := fmt.Fprint(c.out, "\x1b[?1049h\x1b[2J\x1b[HColor vision comparison  ·  press Enter to return\r\n"); err != nil {
		return err
	}
	defer func() {
		for _, id := range ids {
			fmt.Fprintf(c.out, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", id)
		}
		fmt.Fprint(c.out, "\x1b[?1049l")
	}()
	cols, rows := max(1, (c.width-2)/2), max(1, (c.height-4)/2)
	for i, payload := range c.payloads {
		col, row := i%2*cols+1, i/2*rows+3
		if _, err := fmt.Fprintf(c.out, "\x1b[%d;%dH%s", row-1, col, c.titles[i]); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.out, "\x1b[%d;%dH", row, col); err != nil {
			return err
		}
		for offset := 0; offset < len(payload); offset += 4096 {
			end := min(offset+4096, len(payload))
			more := 0
			if end < len(payload) {
				more = 1
			}
			header := fmt.Sprintf("m=%d", more)
			if offset == 0 {
				header = fmt.Sprintf("a=T,f=100,i=%d,c=%d,r=%d,q=2,m=%d", ids[i], max(1, cols-1), max(1, rows-2), more)
			}
			if _, err := fmt.Fprintf(c.out, "\x1b_G%s;%s\x1b\\", header, payload[offset:end]); err != nil {
				return err
			}
		}
	}
	_, err := bufio.NewReader(c.in).ReadString('\n')
	return err
}

type nativeCommand struct {
	image		image.Image
	payload		string
	width, height	int
	in		io.Reader
	out		io.Writer
}

func (c *nativeCommand) SetStdin(r io.Reader)	{ c.in = r }
func (c *nativeCommand) SetStdout(w io.Writer)	{ c.out = w }
func (c *nativeCommand) SetStderr(io.Writer)	{}
func (c *nativeCommand) Run() error {
	var idBytes [4]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return err
	}
	id := binary.BigEndian.Uint32(idBytes[:]) | 1
	payload := c.payload
	if payload == "" {
		var err error
		payload, err = PrepareNativeData(c.image)
		if err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(c.out, "\x1b[?1049h\x1b[2J\x1b[HNative image preview  ·  press Enter to return\r\n\r\n"); err != nil {
		return err
	}
	defer fmt.Fprintf(c.out, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\\x1b[?1049l", id)
	for i := 0; i < len(payload); i += 4096 {
		end := min(i+4096, len(payload))
		more := 0
		if end < len(payload) {
			more = 1
		}
		header := fmt.Sprintf("m=%d", more)
		if i == 0 {
			header = fmt.Sprintf("a=T,f=100,i=%d,c=%d,r=%d,q=2,m=%d", id, max(1, c.width), max(1, c.height-3), more)
		}
		if _, err := fmt.Fprintf(c.out, "\x1b_G%s;%s\x1b\\", header, payload[i:end]); err != nil {
			return err
		}
	}
	_, err := bufio.NewReader(c.in).ReadString('\n')
	return err
}
