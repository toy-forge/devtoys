package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"math/big"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) ([]byte, *ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "example.test"}, NotBefore: time.Unix(0, 0), NotAfter: time.Unix(86400, 0), DNSNames: []string{"example.test"}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return der, key, cert
}
func TestDecodePEMDERAndPKCS12(t *testing.T) {
	der, key, cert := fixture(t)
	pfx, err := pkcs12.Modern2023.Encode(key, cert, nil, "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ input, password string }{{string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), ""}, {base64.StdEncoding.EncodeToString(der), ""}, {base64.StdEncoding.EncodeToString(pfx), "secret"}} {
		out, err := decodeCertificate(tc.input, tc.password)
		if err != nil || !strings.Contains(out, "example.test") || !strings.Contains(out, "SHA-256") {
			t.Fatalf("out=%q err=%v", out, err)
		}
	}
	if _, err := decodeCertificate(base64.StdEncoding.EncodeToString(pfx), "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
}
func TestBinaryImportAndPasswordMask(t *testing.T) {
	der, _, _ := fixture(t)
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.Update(toolkit.BinaryImportMsg{Data: der, Name: "cert.der"})
	out, err := decodeCertificate(m.ws.Input.Value(), "")
	if err != nil || !strings.Contains(out, "example.test") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	m.setFocus(2)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret-password")})
	if strings.Contains(m.View(), "secret-password") {
		t.Fatal("password visible")
	}
}
