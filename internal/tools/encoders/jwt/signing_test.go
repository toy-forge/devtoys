package jwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func completeEncoder(t *testing.T, tool *jwtTool) {
	t.Helper()
	_, cmd := tool.Update(encodeDue{revision: tool.encodeRevision})
	if cmd == nil {
		t.Fatal("encoding job missing")
	}
	result := cmd().(toolkit.BackgroundToolMsg)
	tool.Update(result.Message)
}

func keys(t *testing.T, key any) (string, string) {
	t.Helper()
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	var public any
	switch k := key.(type) {
	case *rsa.PrivateKey:
		public = &k.PublicKey
	case *ecdsa.PrivateKey:
		public = &k.PublicKey
	}
	pub, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))
}
func TestSigningAlgorithms(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPrivate, rsaPublic := keys(t, rsaKey)
	ecKeys := map[string]elliptic.Curve{"ES256": elliptic.P256(), "ES384": elliptic.P384(), "ES512": elliptic.P521()}
	for _, alg := range algorithms {
		t.Run(alg, func(t *testing.T) {
			private, public := " secret with spaces ", " secret with spaces "
			if strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS") {
				private, public = rsaPrivate, rsaPublic
			}
			if curve := ecKeys[alg]; curve != nil {
				k, err := ecdsa.GenerateKey(curve, rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				private, public = keys(t, k)
			}
			token, err := signToken(`{"typ":"JWT","alg":"wrong"}`, `{"sub":"123","exp":42}`, alg, private, false)
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(token, ".")
			if err = verifySignature(parts, alg, public, false); err != nil {
				t.Fatalf("verify: %v", err)
			}
			out, err := renderToken(token, public)
			if err != nil || !strings.Contains(out, "Signature verified") {
				t.Fatalf("decode: %s %v", out, err)
			}
			parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"tampered"}`))
			if verifySignature(parts, alg, public, false) == nil {
				t.Fatal("modified payload verified")
			}
		})
	}
}
func TestKeyFormatsAndValidation(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("secret"))
	token, err := signToken(`{}`, `{}`, "HS384", encoded, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySignature(strings.Split(token, "."), "HS384", encoded, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ header, payload, alg, key string }{{`[]`, `{}`, "HS256", "k"}, {`{}`, `[]`, "HS256", "k"}, {`{}`, `{}`, "none", "k"}, {`{}`, `{}`, "HS256", ""}, {`{}`, `{}`, "RS256", "secret"}} {
		if _, err := signToken(tc.header, tc.payload, tc.alg, tc.key, false); err == nil {
			t.Fatalf("expected failure: %+v", tc)
		}
	}
	if _, err := signToken(`{}`, `{}`, "HS256", "not-base64!", true); err == nil {
		t.Fatal("invalid key accepted")
	}
}
func TestModeLayoutAndInput(t *testing.T) {
	tool := newJWTTool()
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 30, Height: 12}, {Width: 45, Height: 8}, {Width: 20, Height: 7}} {
		tool.Update(size)
		if got := lipgloss.Height(tool.View()); got != size.Height {
			t.Fatalf("decode %v height %d", size, got)
		}
		tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
		if got := lipgloss.Height(tool.View()); got != size.Height {
			t.Fatalf("encode %v height %d header=%d key=%d view=%q", size, got, tool.headerHeight, tool.keyHeight, tool.View())
		}
		tool.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
		completeEncoder(t, tool)
		if tool.encoder.OutputText() == "" {
			t.Fatalf("no token: %v", tool.encoder.Err)
		}
		tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	}
}

func TestEncodeDoesNotResignOnNavigation(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	private, _ := keys(t, key)
	tool := newJWTTool()
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	tool.algorithm = 9	// ES256 uses a random nonce.
	tool.key.SetValue(private)
	tool.scheduleEncoder()
	completeEncoder(t, tool)
	original := tool.encoder.OutputText()
	if original == "" {
		t.Fatalf("signing failed: %v", tool.encoder.Err)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRight})
	tool.Update(tea.MouseMsg{X: 0, Y: tool.headerHeight + tool.keyHeight, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	tool.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := tool.encoder.OutputText(); got != original {
		t.Fatal("navigation regenerated a signed token")
	}
}
