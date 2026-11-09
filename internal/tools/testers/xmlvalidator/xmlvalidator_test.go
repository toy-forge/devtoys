package xmlvalidator

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

const schema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="count" type="xs:int"/></xs:schema>`

func TestValidateSchemaAndDocument(t *testing.T) {
	for _, tc := range []struct{ schema, xml, want string }{{schema, `<count>12</count>`, "XML is valid"}, {schema, `<count>no</count>`, "Validation failed"}, {"<not-schema>", "<x/>", "Invalid XSD"}, {schema, "<count>", "Invalid XML"}} {
		if got := validate(context.Background(), tc.schema, tc.xml); !strings.Contains(got, tc.want) {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}
func TestRejectExternalResources(t *testing.T) {
	x := `<!DOCTYPE count [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><count>&xxe;</count>`
	if got := validate(context.Background(), schema, x); strings.Contains(got, "root:") || strings.HasPrefix(got, "XML is valid") {
		t.Fatalf("external entity accepted: %s", got)
	}
}
func TestInputsAndStaleResult(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(toolkit.FileImportMsg{Text: schema})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(toolkit.FileImportMsg{Text: `<count>1</count>`})
	if m.revision != 2 || !strings.Contains(m.panes.OutputText(), "Validating") {
		t.Fatalf("revision=%d output=%q", m.revision, m.panes.OutputText())
	}
	m.Update(validationResult{revision: 1, message: "stale"})
	if m.panes.OutputText() == "stale" {
		t.Fatal("stale validation replaced current state")
	}
	m.Update(validationResult{revision: 2, message: "valid"})
	if m.panes.OutputText() != "valid" {
		t.Fatal("current validation result missing")
	}
	if got := lipgloss.Height(m.View()); got != 24 {
		t.Fatalf("height=%d", got)
	}
}
