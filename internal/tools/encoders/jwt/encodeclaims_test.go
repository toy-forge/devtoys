package jwt

import (
	"encoding/base64"
	json "encoding/json/v2"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func decodedClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("bad JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}
func TestEncodingClaimsAndDefaults(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	opts := encoderClaims{issuerEnabled: true, audienceEnabled: true, expirationEnabled: true, defaultTimes: true, issuer: " issuer ", audience: "application", expiration: now.Add(12 * time.Hour), expirationSet: true}
	token, err := signTokenWithClaims(`{"typ":"JWT"}`, `{"iss":"old","aud":"old","exp":1,"iat":11,"custom":"kept"}`, "HS256", "secret", false, opts, now)
	if err != nil {
		t.Fatal(err)
	}
	claims := decodedClaims(t, token)
	for key, want := range map[string]any{"iss": "issuer", "aud": "application", "custom": "kept", "iat": float64(11), "nbf": float64(now.Unix()), "exp": float64(now.Add(12 * time.Hour).Unix())} {
		if claims[key] != want {
			t.Errorf("%s=%v want %v", key, claims[key], want)
		}
	}
	if err := verifySignature(strings.Split(token, "."), "HS256", "secret", false); err != nil {
		t.Fatal(err)
	}
	token, err = signTokenWithClaims(`{}`, `{"sub":"x"}`, "HS256", "secret", false, encoderClaims{defaultTimes: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	claims = decodedClaims(t, token)
	if claims["iat"] != float64(now.Unix()) || claims["nbf"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(time.Hour).Unix()) {
		t.Fatalf("default times %+v", claims)
	}
	token, err = signTokenWithClaims(`{}`, `{"iss":"original","exp":0}`, "HS256", "secret", false, encoderClaims{}, now)
	if err != nil {
		t.Fatal(err)
	}
	claims = decodedClaims(t, token)
	if claims["iss"] != "original" || claims["exp"] != float64(0) {
		t.Fatalf("payload changed %+v", claims)
	}
}
func TestEncodingClaimValidation(t *testing.T) {
	for _, opts := range []encoderClaims{{issuerEnabled: true}, {audienceEnabled: true}, {expirationEnabled: true}} {
		if _, err := signTokenWithClaims(`{}`, `{}`, "HS256", "secret", false, opts, time.Now()); err == nil {
			t.Fatalf("accepted missing claim %+v", opts)
		}
	}
	f := newEncodeClaimForm()
	f.inputs[encodeYear].SetValue("2023")
	f.inputs[encodeMonth].SetValue("02")
	f.inputs[encodeDay].SetValue("29")
	if _, err := f.options(encoderClaims{expirationEnabled: true}); err == nil {
		t.Fatal("accepted invalid calendar date")
	}
	f.inputs[encodeYear].SetValue("2024")
	opt, err := f.options(encoderClaims{expirationEnabled: true})
	if err != nil || opt.expiration.Year() != 2024 || opt.expiration.Month() != 2 || opt.expiration.Day() != 29 {
		t.Fatalf("leap date: %+v %v", opt, err)
	}
	f.inputs[encodeYear].SetValue("0001")
	f.inputs[encodeMonth].SetValue("01")
	f.inputs[encodeDay].SetValue("01")
	f.inputs[encodeHour].SetValue("00")
	f.inputs[encodeMinute].SetValue("00")
	opt, err = f.options(encoderClaims{expirationEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = signTokenWithClaims(`{}`, `{}`, "HS256", "secret", false, opt, time.Now()); err != nil {
		t.Fatalf("year 1 rejected: %v", err)
	}
}
func TestEncodeClaimOptionsLayout(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 30}, {Width: 30, Height: 12}, {Width: 20, Height: 7}} {
		tool := newJWTTool()
		tool.Update(size)
		tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
		if !tool.encodeConfiguring {
			t.Fatal("claim editor not opened")
		}
		for i := range encodeFieldCount {
			tool.encodeForm.setFocus(encodeField(i))
			if w, h := lipgloss.Width(tool.View()), lipgloss.Height(tool.View()); w > size.Width || h != size.Height {
				t.Fatalf("%v field %d rendered %dx%d", size, i, w, h)
			}
		}
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
		if !tool.encClaims.issuerEnabled {
			t.Fatal("issuer not enabled")
		}
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
		if tool.encodeConfiguring {
			t.Fatal("claim editor not closed")
		}
	}
}

func TestEncoderClaimFormUpdatesSignedToken(t *testing.T) {
	tool := newJWTTool()
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	tool.key.SetValue("secret")
	tool.encoder.B.SetValue(`{"iss":"original","sub":"u"}`)
	tool.scheduleEncoder()
	completeEncoder(t, tool)
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
	if !tool.encodeConfiguring || tool.encodeForm.focus != encodeIssuer {
		t.Fatal("issuer editor not focused")
	}
	tool.Update(toolkit.FileImportMsg{Text: "example-issuer"})
	completeEncoder(t, tool)
	claims := decodedClaims(t, tool.encoder.OutputText())
	if claims["iss"] != "example-issuer" || claims["sub"] != "u" {
		t.Fatalf("issuer %+v", claims)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}, Alt: true})
	tool.Update(toolkit.FileImportMsg{Text: "example-audience"})
	completeEncoder(t, tool)
	claims = decodedClaims(t, tool.encoder.OutputText())
	if claims["aud"] != "example-audience" {
		t.Fatalf("audience %+v", claims)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}, Alt: true})
	for f, value := range map[encodeField]string{encodeYear: "2030", encodeMonth: "06", encodeDay: "15", encodeHour: "10", encodeMinute: "30"} {
		tool.encodeForm.inputs[f].SetValue(value)
	}
	tool.scheduleEncoder()
	completeEncoder(t, tool)
	claims = decodedClaims(t, tool.encoder.OutputText())
	want := time.Date(2030, 6, 15, 10, 30, 0, 0, time.UTC).Unix()
	if claims["exp"] != float64(want) {
		t.Fatalf("expiration %+v", claims)
	}
	if err := verifySignature(strings.Split(tool.encoder.OutputText(), "."), "HS256", "secret", false); err != nil {
		t.Fatal(err)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
	completeEncoder(t, tool)
	claims = decodedClaims(t, tool.encoder.OutputText())
	if claims["iss"] != "original" {
		t.Fatalf("disabled option did not preserve payload: %+v", claims)
	}
}
func TestEncoderDiscardsStaleAsyncResults(t *testing.T) {
	tool := newJWTTool()
	tool.key.SetValue("secret")
	tool.encoder.B.SetValue(`{"sub":"old"}`)
	tool.scheduleEncoder()
	first := tool.encodeRevision
	_, oldJob := tool.Update(encodeDue{revision: first})
	if oldJob == nil {
		t.Fatal("old job missing")
	}
	tool.encoder.B.SetValue(`{"sub":"new"}`)
	tool.scheduleEncoder()
	latest := tool.encodeRevision
	tool.Update(encodeDue{revision: latest})
	oldResult := oldJob().(toolkit.BackgroundToolMsg)
	_, newJob := tool.Update(oldResult.Message)
	if newJob == nil {
		t.Fatal("latest job not queued")
	}
	if tool.encoder.OutputText() != "" {
		t.Fatal("stale token was displayed")
	}
	tool.Update(newJob().(toolkit.BackgroundToolMsg).Message)
	if got := decodedClaims(t, tool.encoder.OutputText())["sub"]; got != "new" {
		t.Fatalf("subject=%v", got)
	}
}
func TestEncoderInputSizeLimit(t *testing.T) {
	if _, err := signToken(`{}`, `{"value":"`+strings.Repeat("x", 1<<20)+`"}`, "HS256", "secret", false); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
func TestEncodeClaimMouseFocus(t *testing.T) {
	tool := newJWTTool()
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	for row, layout := range tool.encodeForm.rows() {
		for _, cell := range layout.cells {
			y := tool.headerHeight + 1 + row - tool.encodeForm.offset
			tool.Update(tea.MouseMsg{X: cell.inputStart, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if tool.encodeForm.focus != cell.field {
				t.Fatalf("clicked field %d, focus %d", cell.field, tool.encodeForm.focus)
			}
		}
	}
}

func TestEncoderDesktopPaneOrderAndCompactKey(t *testing.T) {
	tool := newJWTTool()
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	view := ansi.Strip(tool.View())
	token, header, payload, key := strings.Index(view, "Token"), strings.Index(view, "Header"), strings.Index(view, "Payload"), strings.Index(view, "Signing key")
	if token < 0 || header < token || payload < token || key < header || key < payload {
		t.Fatalf("encoder pane order incorrect:\n%s", view)
	}
	tool.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
	view = ansi.Strip(tool.View())
	if !strings.Contains(view, "secret") {
		t.Fatalf("compact key input is hidden:\n%s", view)
	}
	if h := lipgloss.Height(tool.View()); h != 7 {
		t.Fatalf("compact height=%d", h)
	}
}

func TestExpirationControlsIncrementAndBounds(t *testing.T) {
	f := newEncodeClaimForm()
	f.setFocus(encodeMonth)
	f.inputs[encodeMonth].SetValue("11")
	changed, _ := f.Update(tea.KeyMsg{Type: tea.KeyUp})
	if !changed || f.inputs[encodeMonth].Value() != "12" {
		t.Fatal("month increment failed")
	}
	changed, _ = f.Update(tea.KeyMsg{Type: tea.KeyUp})
	if changed || f.err == nil || f.inputs[encodeMonth].Value() != "12" {
		t.Fatal("month overflow accepted")
	}
	f.setFocus(encodeYear)
	f.inputs[encodeYear].SetValue("9999")
	changed, _ = f.Update(tea.KeyMsg{Type: tea.KeyUp})
	if changed || f.err == nil || f.inputs[encodeYear].Value() != "9999" {
		t.Fatal("year overflow accepted")
	}
}
