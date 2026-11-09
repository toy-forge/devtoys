package escape

import "testing"

func TestEscapeRoundTrip(t *testing.T) {
	input := "one\r\ntwo\t\\\"文"
	out := escaper.Replace(input)
	if got := unescaper.Replace(out); got != input {
		t.Fatalf("escaped %q decoded %q", out, got)
	}
	if got := unescaper.Replace(`unknown\q`); got != `unknown\q` {
		t.Fatalf("unknown escape=%q", got)
	}
}
func TestUnescapedOutputDisplaysControlsSafely(t *testing.T) {
	tool := newTool()
	tool.encode = false
	tool.ws.Input.SetValue(`a\rb\tc\b`)
	tool.recalc()
	if got := tool.ws.OutputText(); got != "a\rb\tc\b" {
		t.Fatalf("output=%q", got)
	}
}
