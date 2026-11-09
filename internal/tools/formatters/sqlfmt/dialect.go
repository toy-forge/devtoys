package sqlfmt

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// The bundled MIT-licensed sql-formatter 4.0.2 supplies the same ten dialects
// used by the desktop implementation. Its comma handler is adapted to match
// DevToys' leading-comma option; see sqlformatter/NOTICE.
//
//go:embed sqlformatter/sql-formatter.js
var sqlFormatterJS string

var compiledFormatter = sync.OnceValues(func() (*goja.Program, error) { return goja.Compile("sql-formatter.js", sqlFormatterJS, false) })

type dialect struct{ name, id string }

const standardSQLDialectIndex = 8

var dialects = []dialect{{"DB2", "db2"}, {"MariaDB", "mariadb"}, {"MySQL", "mysql"}, {"N1QL", "n1ql"}, {"PL/SQL", "plsql"}, {"PostgreSQL", "postgresql"}, {"Redshift", "redshift"}, {"Spark SQL", "spark"}, {"Standard SQL", "sql"}, {"Transact-SQL", "tsql"}}

const maxSQLBytes = 1 << 20

func formatDialect(src string, dialectID string, indent string, leading bool) (string, error) {
	if strings.TrimSpace(src) == "" {
		return "", nil
	}
	if len(src) > maxSQLBytes {
		return "", fmt.Errorf("SQL input exceeds 1 MiB")
	}
	valid := false
	for _, d := range dialects {
		if d.id == dialectID {
			valid = true
			break
		}
	}
	if !valid {
		return "", fmt.Errorf("unsupported SQL dialect")
	}
	masked, literals, err := protectDialectLiterals(src, dialectID)
	if err != nil {
		return "", err
	}
	program, err := compiledFormatter()
	if err != nil {
		return "", fmt.Errorf("SQL formatter initialization failed: %w", err)
	}
	vm := goja.New()
	if err := vm.Set("window", vm.GlobalObject()); err != nil {
		return "", fmt.Errorf("SQL formatter initialization failed: %w", err)
	}
	timer := time.AfterFunc(2*time.Second, func() { vm.Interrupt("SQL formatting timed out") })
	defer timer.Stop()
	if _, err = vm.RunProgram(program); err != nil {
		return "", fmt.Errorf("SQL formatter initialization failed: %w", err)
	}
	obj := vm.Get("sqlFormatter")
	if obj == nil || goja.IsUndefined(obj) || goja.IsNull(obj) {
		return "", fmt.Errorf("SQL formatter initialization failed")
	}
	format, ok := goja.AssertFunction(obj.ToObject(vm).Get("format"))
	if !ok {
		return "", fmt.Errorf("SQL formatter initialization failed")
	}
	options := map[string]any{"language": dialectID, "indent": indent, "uppercase": true, "linesBetweenQueries": 2, "useLeadingComma": leading}
	result, err := format(goja.Undefined(), vm.ToValue(masked), vm.ToValue(options))
	if err != nil {
		return "", fmt.Errorf("SQL formatting failed: %w", err)
	}
	out := result.String()
	for _, literal := range literals {
		out = strings.ReplaceAll(out, literal.marker, literal.text)
	}
	return out, nil
}
