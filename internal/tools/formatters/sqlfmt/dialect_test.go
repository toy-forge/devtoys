package sqlfmt

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestAllDesktopDialects(t *testing.T) {
	for _, d := range dialects {
		t.Run(d.id, func(t *testing.T) {
			got, err := formatDialect("select a,b from things where a=1", d.id, "  ", false)
			if err != nil || !strings.Contains(got, "SELECT") || !strings.Contains(got, "FROM") {
				t.Fatalf("%s: %q %v", d.id, got, err)
			}
		})
	}
}

func TestDialectSpecificSyntaxAndLiterals(t *testing.T) {
	cases := []struct {
		dialect, sql	string
		keep		[]string
	}{
		{"postgresql", `select $tag$a, 'quoted'; and $other$$tag$ as x, E'it\'s', U&'d\0061t' from t`, []string{`$tag$a, 'quoted'; and $other$$tag$`, `E'it\'s'`, `U&'d\0061t'`}},
		{"postgresql", `select /* outer /* nested */ tail */ 1`, []string{`/* outer /* nested */ tail */`}},
		{"postgresql", `select array[$中文$a,b$中文$, E'it\'s']`, []string{`$中文$a,b$中文$`, `E'it\'s'`}},
		{"postgresql", `select 'backslash\' as x, '😀'`, []string{`'backslash\'`, `'😀'`}},
		{"mysql", `select 'it\'s', "escaped\"text"`, []string{`'it\'s'`, `"escaped\"text"`}},
		{"mysql", "select 1 # it's a comment\nfrom t", []string{"# it's a comment", "FROM"}},
		{"plsql", `select q'[don't, split]' as x, nq'!a,b!' from dual`, []string{`q'[don't, split]'`, `nq'!a,b!'`}},
		{"mysql", "select _utf8mb4'你好', x'FF', b'01', N'text', `a,b` from t", []string{`_utf8mb4'你好'`, `x'FF'`, `b'01'`, `N'text'`, "`a,b`"}},
		{"tsql", `select top 1 [first name], N'姓名' from [my table] where id=@id`, []string{`[first name]`, `N'姓名'`, `[my table]`, `@id`}},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+tc.sql, func(t *testing.T) {
			got, err := formatDialect(tc.sql, tc.dialect, "  ", false)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.keep {
				if !strings.Contains(got, want) {
					t.Fatalf("literal %q lost in %q", want, got)
				}
			}
		})
	}
}
func TestProtectedLiteralsDoNotCaptureCommentsOrExistingMarkers(t *testing.T) {
	src := "select '__DEVTOYS_SQL_LITERAL_0__', '$tag$not a literal', $body$SELECT a, b$body$ -- $missing$\nfrom t"
	got, err := formatDialect(src, "postgresql", "  ", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'__DEVTOYS_SQL_LITERAL_0__'", "'$tag$not a literal'", "$body$SELECT a, b$body$", "-- $missing$"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, sql := range []string{"select $tag$missing", "select E'unfinished\\'", "select /* a /* b */"} {
		if _, err := formatDialect(sql, "postgresql", "  ", false); err == nil {
			t.Fatalf("accepted unterminated %q", sql)
		}
	}
	if _, err := formatDialect("select q'[missing'", "plsql", "  ", false); err == nil {
		t.Fatal("accepted unterminated Oracle literal")
	}
}
func TestDesktopSettingsAndDisplay(t *testing.T) {
	sql := "select a,b from t; select c,d from u"
	out, err := formatDialect(sql, "sql", "\t", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\n\t, b") || !strings.Contains(out, ";\n\nSELECT") {
		t.Fatalf("settings ignored: %q", out)
	}
	if display := sqlDisplay(out); strings.Contains(display, "\t") || !strings.Contains(out, "\t") {
		t.Fatalf("tab output/display mismatch: %q", display)
	}
}

func TestAsyncFormatterKeepsLatestInput(t *testing.T) {
	tool := newTool()
	tool.ws.Input.SetValue("select first_column from old_table")
	tool.schedule()
	firstRevision := tool.revision
	_, first := tool.Update(sqlDue{revision: firstRevision})
	if first == nil {
		t.Fatal("first job missing")
	}
	tool.ws.Input.SetValue("select second_column from new_table")
	tool.dialect = 5
	tool.schedule()
	latest := tool.revision
	tool.Update(sqlDue{revision: latest})
	done := first().(toolkit.BackgroundToolMsg)
	if done.Tool != tool {
		t.Fatal("result routed to wrong tool")
	}
	_, next := tool.Update(done.Message)
	if next == nil {
		t.Fatal("latest input was not queued")
	}
	if strings.Contains(tool.ws.OutputText(), "old_table") {
		t.Fatal("stale output applied")
	}
	done = next().(toolkit.BackgroundToolMsg)
	tool.Update(done.Message)
	if tool.ws.Err != nil || !strings.Contains(tool.ws.OutputText(), "new_table") || strings.Contains(tool.ws.OutputText(), "old_table") {
		t.Fatalf("wrong output %q: %v", tool.ws.OutputText(), tool.ws.Err)
	}
}
func TestDialectControlsAndNarrowLayout(t *testing.T) {
	tool := newTool()
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 30, Height: 7}, {Width: 20, Height: 7}} {
		tool.Update(size)
		if w, h := lipgloss.Width(tool.View()), lipgloss.Height(tool.View()); w > size.Width || h != size.Height {
			t.Fatalf("%v rendered %dx%d", size, w, h)
		}
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true})
	if tool.dialect != 9 {
		t.Fatal("dialect shortcut did not work")
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}, Alt: true})
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true})
	if tool.indent != 4 || !tool.leadingComma {
		t.Fatal("formatting options did not work")
	}
}
func TestSQLInputLimit(t *testing.T) {
	if _, err := formatDialect(strings.Repeat("x", maxSQLBytes+1), "sql", "  ", false); err == nil {
		t.Fatal("oversized input accepted")
	}
	if _, err := formatDialect("select 1", "invalid", "  ", false); err == nil {
		t.Fatal("invalid dialect accepted")
	}
}

func TestDialectChangesKeywordRules(t *testing.T) {
	input := "select returning,ilike from t"
	standard, err := formatDialect(input, "sql", "  ", false)
	if err != nil {
		t.Fatal(err)
	}
	postgres, err := formatDialect(input, "postgresql", "  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(standard, "returning") || !strings.Contains(postgres, "RETURNING") || !strings.Contains(postgres, "ILIKE") {
		t.Fatalf("dialect rules not applied:\nstandard %s\npostgres %s", standard, postgres)
	}
}
