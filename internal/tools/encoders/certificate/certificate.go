// Package certificate decodes X.509 certificates and PKCS#12 files locally.
package certificate

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

const maxInputBytes = 1 << 20

type tool struct {
	ws				toolkit.Workspace
	password			textinput.Model
	focus				int	// 0 input, 1 output, 2 password
	width, height			int
	beforeInput, beforePassword	string
	revision			uint64
	running, queued			bool
}
type due struct{ revision uint64 }
type decoded struct {
	revision	uint64
	output		string
	err		error
}

func newTool() *tool {
	secret := textinput.New()
	// 标签由 passwordRow 的标签列渲染（ui.FormRow），输入框不带提示符；占位符接 MutedStyle。
	secret.Prompt = ""
	secret.Placeholder = "Optional PKCS#12 password"
	secret.EchoMode = textinput.EchoPassword
	secret.EchoCharacter = '•'
	secret.CharLimit = 1024
	secret.Width = 35
	secret.PromptStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	secret.PlaceholderStyle = theme.MutedStyle()
	t := &tool{ws: toolkit.NewWorkspace("Paste a PEM or Base64 certificate, or open a DER/PFX file…", ""), password: secret}
	t.ws.SetOutputExtendable(true)
	return t
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "encoders.certificate" }
func (*tool) Title() string		{ return "Certificate Decoder" }
func (*tool) ShortTitle() string	{ return "Certificate" }
func (*tool) Category() string		{ return "encoders" }
func (*tool) Icon() string		{ return "󰈙" }
func (*tool) Description() string {
	return "Inspect X.509 certificates and password-protected PKCS#12 files"
}
func (*tool) AcceptsBinary() bool	{ return true }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) setFocus(i int) {
	t.focus = (i + 3) % 3
	t.password.Blur()
	t.ws.Input.Blur()
	switch t.focus {
	case 0:
		t.ws.Focus = toolkit.FocusInput
		t.ws.Input.FocusPreservingScroll()
	case 1:
		t.ws.Focus = toolkit.FocusOutput
	case 2:
		t.ws.Focus = toolkit.FocusInput
		t.password.Focus()
	}
}
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = event.Width, event.Height
		t.password.Width = max(1, event.Width-lipgloss.Width(t.password.Prompt))
		height := event.Height
		if !t.wideLayout() {
			height = max(1, height-t.narrowRows())
		}
		return t, t.ws.Update(tea.WindowSizeMsg{Width: event.Width, Height: height})
	case toolkit.BinaryImportMsg:
		t.setFocus(0)
		data := event.Data
		text := string(data)
		if !utf8.Valid(data) || !strings.Contains(text, "-----BEGIN ") {
			text = base64.StdEncoding.EncodeToString(data)
		}
		t.ws.Input.SetValue(text)
		return t, t.changed(nil)
	case due:
		if event.revision != t.revision {
			return t, nil
		}
		if t.running {
			t.queued = true
			return t, nil
		}
		t.running = true
		input, password, rev := t.ws.Input.Value(), t.password.Value(), t.revision
		return t, func() tea.Msg {
			out, err := decodeCertificate(input, password)
			return toolkit.BackgroundToolMsg{Tool: t, Message: decoded{revision: rev, output: out, err: err}}
		}
	case decoded:
		t.running = false
		if event.revision == t.revision {
			t.ws.SetError(event.err)
			t.ws.SetOutput(event.output)
		}
		if t.queued {
			t.queued = false
			rev := t.revision
			return t, func() tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: due{revision: rev}} }
		}
		return t, nil
	case tea.KeyMsg:
		switch event.String() {
		case "ctrl+o":
			return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case "tab":
			t.setFocus(t.focus + 1)
			return t, nil
		case "shift+tab":
			t.setFocus(t.focus + 2)
			return t, nil
		case "ctrl+s":
			t.setFocus(2)
			return t, nil
		case "ctrl+y":
			return t, t.ws.Update(event)
		}
		if t.focus == 2 {
			var cmd tea.Cmd
			t.password, cmd = t.password.Update(event)
			return t, t.changed(cmd)
		}
	case tea.MouseMsg:
		if t.wideLayout() {
			if event.X < t.ws.Input.Width()+2 {
				if event.Y == 0 && event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
					t.setFocus(2)
					if event.X >= lipgloss.Width(t.password.Prompt) {
						t.password.SetCursor(event.X - lipgloss.Width(t.password.Prompt))
					}
					return t, nil
				}
				if event.Y == 2 && event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
					if toolkit.ActionAt(event, "Select certificate file") == 0 {
						return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
					}
					return t, nil
				}
				if event.Y < 4 {
					return t, nil
				}
				if event.Y < t.height-1 {
					event.Y -= 4
				}
			}
		} else {
			rows := t.narrowRows()
			if event.Y == 0 && event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
				t.setFocus(2)
				if event.X >= lipgloss.Width(t.password.Prompt) {
					t.password.SetCursor(event.X - lipgloss.Width(t.password.Prompt))
				}
				return t, nil
			}
			if event.Y == rows-1 && event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
				if toolkit.ActionAt(event, "Select certificate file") == 0 {
					return t, func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
				}
				return t, nil
			}
			if event.Y < rows {
				return t, nil
			}
			event.Y -= rows
		}
		cmd := t.ws.Update(event)
		if event.Action == tea.MouseActionPress && event.Button == tea.MouseButtonLeft {
			if t.ws.Focus == toolkit.FocusOutput {
				t.setFocus(1)
			} else {
				t.setFocus(0)
			}
		}
		return t, t.changed(cmd)
	}
	if t.focus == 2 {
		var cmd tea.Cmd
		t.password, cmd = t.password.Update(msg)
		return t, t.changed(cmd)
	}
	cmd := t.ws.Update(msg)
	return t, t.changed(cmd)
}
func (t *tool) changed(cmd tea.Cmd) tea.Cmd {
	input, password := t.ws.Input.Value(), t.password.Value()
	if input == t.beforeInput && password == t.beforePassword {
		return cmd
	}
	t.beforeInput, t.beforePassword = input, password
	t.revision++
	t.queued = false
	if strings.TrimSpace(input) == "" {
		t.ws.SetError(nil)
		t.ws.SetOutput("")
		return cmd
	}
	if len(input) > base64.StdEncoding.EncodedLen(maxInputBytes)+4096 {
		t.ws.SetError(fmt.Errorf("certificate exceeds 1 MiB"))
		t.ws.SetOutput("")
		return cmd
	}
	t.ws.SetError(nil)
	t.ws.SetOutput("Decoding…")
	rev := t.revision
	timer := tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: due{revision: rev}} })
	return tea.Batch(cmd, timer)
}

// passwordLabelWidth 是 PKCS#12 密码行的标签列宽（"Password" + 间距）。
const passwordLabelWidth = 10

// passwordRow 渲染密码行："Password" 标签 + 定宽输入框（ui.FormRow + ui.FieldView）。
func (t *tool) passwordRow() string {
	field := ui.FieldView(&t.password, max(1, t.width-passwordLabelWidth-ui.Space2), t.password.Focused())
	return ui.FormRow("Password", passwordLabelWidth, field, t.width)
}

func (t *tool) View() string {
	accessory := t.passwordRow() + "\n\n" + toolkit.Actions("Select certificate file")
	if t.wideLayout() {
		t.ws.SetInputAccessory(accessory)
		return t.ws.View()
	}
	t.ws.SetInputAccessory("")
	// 矮终端省掉密码行与按钮行之间的空行，保证工作区留有最小高度。
	separator := "\n\n"
	if t.narrowRows() == 2 {
		separator = "\n"
	}
	return ansi.Truncate(t.passwordRow(), max(1, t.width), "") + separator + ansi.Truncate(toolkit.Actions("Select certificate file"), max(1, t.width), "") + "\n" + t.ws.View()
}

func (t *tool) wideLayout() bool	{ return t.width >= 40 && t.height >= 11 }

// narrowRows 返回窄布局中工作区上方的行数：密码行 +（放得下时的）空行 + 按钮行。
// 极矮终端省掉空行，给工作区让出最小高度（toolkit.WorkspaceMinHeight）。
func (t *tool) narrowRows() int {
	if t.height-3 < toolkit.WorkspaceMinHeight {
		return 2
	}
	return 3
}
func decodeCertificate(input, password string) (output string, err error) {
	defer func() {
		if recover() != nil {
			output, err = "", fmt.Errorf("invalid certificate data")
		}
	}()
	input = strings.TrimSpace(input)
	var certs []*x509.Certificate
	rest := []byte(input)
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return "", fmt.Errorf("invalid certificate: %w", err)
			}
			certs = append(certs, cert)
		}
	}
	if len(certs) == 0 {
		compact := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, input)
		data, err := base64.StdEncoding.DecodeString(compact)
		if err != nil {
			return "", fmt.Errorf("invalid certificate encoding")
		}
		if len(data) > maxInputBytes {
			return "", fmt.Errorf("certificate exceeds 1 MiB")
		}
		certs, err = x509.ParseCertificates(data)
		if err != nil {
			_, leaf, chain, pfxErr := pkcs12.DecodeChain(data, password)
			if pfxErr == nil && leaf != nil {
				certs = append([]*x509.Certificate{leaf}, chain...)
			} else {
				certs, err = pkcs12.DecodeTrustStore(data, password)
				if err != nil {
					if pfxErr == nil {
						pfxErr = err
					}
					return "", fmt.Errorf("could not decode certificate or PKCS#12: %w", pfxErr)
				}
			}
		}
	}
	if len(certs) == 0 {
		return "", fmt.Errorf("no certificate found")
	}
	var b strings.Builder
	for i, cert := range certs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		writeCertificate(&b, cert, i+1)
	}
	return b.String(), nil
}

// writeCertificate 把证书元数据写成对齐的纯文本（可直接复制）：标签列取最长标签宽度，
// 冒号对齐；标题行为证书序号。
func writeCertificate(b *strings.Builder, cert *x509.Certificate, n int) {
	sum := sha256.Sum256(cert.Raw)
	parts := make([]string, len(sum))
	for i, v := range sum {
		parts[i] = strings.ToUpper(hex.EncodeToString([]byte{v}))
	}
	type field struct{ label, value string }
	fields := []field{
		{"Subject", safe(cert.Subject.String())},
		{"Issuer", safe(cert.Issuer.String())},
		{"Serial", cert.SerialNumber.Text(16)},
		{"Valid from", cert.NotBefore.Format(time.RFC3339)},
		{"Valid until", cert.NotAfter.Format(time.RFC3339)},
		{"Signature", fmt.Sprint(cert.SignatureAlgorithm)},
		{"Public key", fmt.Sprint(cert.PublicKeyAlgorithm)},
		{"CA", fmt.Sprint(cert.IsCA)},
		{"SHA-256", strings.Join(parts, ":")},
	}
	if len(cert.DNSNames) > 0 {
		fields = append(fields, field{"DNS names", safe(strings.Join(cert.DNSNames, ", "))})
	}
	if len(cert.EmailAddresses) > 0 {
		fields = append(fields, field{"Email addresses", safe(strings.Join(cert.EmailAddresses, ", "))})
	}
	for _, ip := range cert.IPAddresses {
		fields = append(fields, field{"IP address", ip.String()})
	}
	for _, extension := range cert.Extensions {
		fields = append(fields, field{"Extension", fmt.Sprintf("%s (critical: %t)", extension.Id, extension.Critical)})
	}
	labelWidth := 0
	for _, f := range fields {
		labelWidth = max(labelWidth, len(f.label))
	}
	fmt.Fprintf(b, "Certificate %d", n)
	for _, f := range fields {
		fmt.Fprintf(b, "\n%-*s : %s", labelWidth, f.label, f.value)
	}
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}
