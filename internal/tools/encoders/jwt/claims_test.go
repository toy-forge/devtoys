package jwt

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestClaimChecks(t *testing.T) {
	now := time.Unix(1700000000, 0)
	all := claimOptions{lifetime: true, issuer: true, audience: true, issuers: "other, https://issuer", audiences: "client-a, client-b"}
	cases := []struct {
		name, payload	string
		options		claimOptions
		want		[]string
	}{
		{"valid", `{"iss":"https://issuer","aud":["client-b","client-c"],"exp":1700000300,"nbf":1699999999}`, all, []string{"Lifetime: PASS", "Issuer: PASS", "Audience: PASS"}},
		{"expired", `{"iss":"bad","aud":"wrong","exp":1699999500}`, all, []string{"Lifetime: FAIL", "Issuer: FAIL", "Audience: FAIL"}},
		{"not before", `{"iss":"https://issuer","aud":"client-a","exp":1700000900,"nbf":1700000601}`, all, []string{"Lifetime: FAIL", "Issuer: PASS", "Audience: PASS"}},
		{"clock skew", `{"iss":"https://issuer","aud":"client-a","exp":1699999999}`, all, []string{"Lifetime: PASS", "Clock skew: 5 minutes"}},
		{"inverted lifetime", `{"iss":"https://issuer","aud":"client-a","exp":1700000100,"nbf":1700000200}`, all, []string{"Lifetime: FAIL"}},
		{"missing", `{}`, all, []string{"Lifetime: FAIL", "Issuer: FAIL", "Audience: FAIL"}},
		{"invalid types", `{"iss":3,"aud":[1],"exp":"tomorrow"}`, all, []string{"Lifetime: FAIL", "Issuer: FAIL", "Audience: FAIL"}},
		{"empty config", `{"iss":"","aud":"","exp":1700000300}`, all, []string{"Issuer: FAIL", "Audience: FAIL"}},
		{"disabled", `{}`, claimOptions{}, []string{"No claim checks enabled"}},
		{"nonobject", `[]`, all, []string{"payload must be a JSON object"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkClaims([]byte(tc.payload), tc.options, now)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
		})
	}
}

func TestClaimConfigurationAndSignatureSeparation(t *testing.T) {
	tool := newJWTTool()
	tool.Input.SetValue(base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"expected","aud":["app"],"exp":4102444800}`)) + ".c2ln")
	tool.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	if !tool.configuring {
		t.Fatal("claim settings not opened")
	}
	tool.claimsForm.A.SetValue("expected,other")
	tool.claimsForm.B.SetValue("app")
	for _, k := range []rune{'l', 'i', 'a'} {
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}, Alt: true})
	}
	got := tool.claimsForm.OutputText()
	for _, want := range []string{"Lifetime: PASS", "Issuer: PASS", "Audience: PASS"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	got = tool.OutputText()
	if !strings.Contains(got, "signature not verified") || !strings.Contains(got, "Issuer: PASS") {
		t.Fatalf("signature status and claims not both visible: %s", got)
	}
}
func TestClaimSettingsFitSmallTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 7}, {Width: 30, Height: 12}, {Width: 80, Height: 24}} {
		tool := newJWTTool()
		tool.Update(size)
		tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
		if w, h := lipgloss.Width(tool.View()), lipgloss.Height(tool.View()); w > size.Width || h != size.Height {
			t.Fatalf("%dx%d: rendered %dx%d", size.Width, size.Height, w, h)
		}
	}
}
