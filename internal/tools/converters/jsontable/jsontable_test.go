package jsontable

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTableFlattenAndOrder(t *testing.T) {
	h, r, e := parseTable(`[{"b":1,"a":{"x":"hello, world"},"arr":[1,2]},{"a":{"x":"line\n2"},"c":true}]`)
	if e != nil || !reflect.DeepEqual(h, []string{"b", "a_x", "c"}) || !reflect.DeepEqual(r, [][]string{{"1", "hello, world", ""}, {"", "line\n2", "true"}}) {
		t.Fatalf("headings=%v rows=%v err=%v", h, r, e)
	}
	out, e := delimited(h, r, ',')
	if e != nil || !strings.Contains(out, `"hello, world"`) || !strings.Contains(out, "\"line\n2\"") {
		t.Fatalf("CSV=%q err=%v", out, e)
	}
}
func TestRejectInvalidInput(t *testing.T) {
	for _, s := range []string{`{}`, `[1]`, `[]`, `[{"a":1}] {}`, `[{"x":`} {
		if _, _, err := parseTable(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
