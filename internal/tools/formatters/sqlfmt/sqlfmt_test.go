package sqlfmt

import (
	"strings"
	"testing"
)

func TestStandardSQLFormatting(t *testing.T) {
	input := "select a,b from t where x=1 and y=2; select 'where' from \"table name\""
	got, err := formatDialect(input, "sql", "    ", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SELECT", "FROM", "WHERE", "AND", "'where'", `"table name"`, ";\n\nSELECT"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}
func TestEmptyAndInvalidSQL(t *testing.T) {
	for _, s := range []string{"", "  \n\t"} {
		out, err := formatDialect(s, "sql", "  ", false)
		if err != nil || out != "" {
			t.Fatalf("empty SQL: %q %v", out, err)
		}
	}
	if _, err := formatDialect("select 'unfinished", "sql", "  ", false); err == nil {
		t.Fatal("unclosed quoted text accepted")
	}
}
