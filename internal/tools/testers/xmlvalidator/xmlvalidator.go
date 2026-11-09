// Package xmlvalidator validates XML documents against XSD schemas.
package xmlvalidator

import (
	"context"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xsd"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

const maxDocumentBytes = 1 << 20

type tool struct {
	panes			*toolkit.ThreePane
	beforeSchema, beforeXML	string
	revision		uint64
	cancel			context.CancelFunc
}
type due struct{ revision uint64 }
type validationResult struct {
	revision	uint64
	message		string
}
type diagnostics struct{ messages []string }

func (d *diagnostics) Handle(_ context.Context, err error) {
	if len(d.messages) < 3 {
		d.messages = append(d.messages, err.Error())
	}
}
func newTool() *tool {
	p := toolkit.NewThreePane("XSD", "XML")
	p.SetExtendable(0, 1)
	p.SetResultBar(true)
	p.SetStatus("XML and XSD content must be set in order to perform a validation.", toolkit.StatusInfo)
	return &tool{panes: p}
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "testers.xmlvalidator" }
func (*tool) Title() string		{ return "XML / XSD Tester" }
func (*tool) ShortTitle() string	{ return "XML" }
func (*tool) Category() string		{ return "testers" }
func (*tool) Icon() string		{ return "󰗀" }
func (*tool) Description() string	{ return "Validate an XML document against an XSD schema" }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case due:
		if event.revision != t.revision {
			return t, nil
		}
		schema, doc := t.panes.A.Value(), t.panes.B.Value()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		t.cancel = cancel
		return t, func() tea.Msg {
			message := validate(ctx, schema, doc)
			return toolkit.BackgroundToolMsg{Tool: t, Message: validationResult{revision: event.revision, message: message}}
		}
	case validationResult:
		if event.revision == t.revision {
			if t.cancel != nil {
				t.cancel()
				t.cancel = nil
			}
			t.panes.SetStatus(event.message, severityFor(event.message))
		}
		return t, nil
	}
	cmd := t.panes.Update(msg)
	schema, doc := t.panes.A.Value(), t.panes.B.Value()
	if schema != t.beforeSchema || doc != t.beforeXML {
		t.beforeSchema, t.beforeXML = schema, doc
		t.revision++
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		if strings.TrimSpace(schema) == "" || strings.TrimSpace(doc) == "" {
			t.panes.SetStatus("XML and XSD content must be set in order to perform a validation.", toolkit.StatusInfo)
			return t, cmd
		}
		if len(schema) > maxDocumentBytes || len(doc) > maxDocumentBytes {
			t.panes.SetStatus("Input exceeds the 1 MiB validation limit.", toolkit.StatusError)
			return t, cmd
		}
		t.panes.SetStatus("Validating…", toolkit.StatusInfo)
		rev := t.revision
		pending := tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toolkit.BackgroundToolMsg{Tool: t, Message: due{revision: rev}} })
		return t, tea.Batch(cmd, pending)
	}
	return t, cmd
}
func (t *tool) View() string	{ return t.panes.View() }

func severityFor(message string) toolkit.StatusSeverity {
	switch {
	case strings.HasPrefix(message, "XML is compliant"):
		return toolkit.StatusSuccess
	case strings.HasPrefix(message, "Enter "), strings.HasPrefix(message, "Validating"):
		return toolkit.StatusInfo
	default:
		return toolkit.StatusError
	}
}

func validate(ctx context.Context, schemaText, xmlText string) string {
	parser := helium.NewParser()
	schemaDoc, err := parser.Parse(ctx, []byte(schemaText))
	if err != nil {
		return "Invalid XSD: " + safe(err.Error())
	}
	collector := &diagnostics{}
	schema, err := xsd.NewCompiler().ErrorHandler(collector).Compile(ctx, schemaDoc)
	if err != nil {
		return message("Invalid XSD", err, collector)
	}
	document, err := parser.Parse(ctx, []byte(xmlText))
	if err != nil {
		return "Invalid XML: " + safe(err.Error())
	}
	collector.messages = nil
	if err = xsd.NewValidator(schema).ErrorHandler(collector).Validate(ctx, document); err != nil {
		return message("Validation failed", err, collector)
	}
	return "XML is valid"
}
func message(prefix string, err error, d *diagnostics) string {
	if len(d.messages) > 0 {
		return prefix + ": " + safe(strings.Join(d.messages, "; "))
	}
	return prefix + ": " + safe(err.Error())
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
