package jsonfmt

import (
	"bytes"
	"cmp"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"slices"
	"strings"
)

type property struct {
	name, encodedName	string
	value			[]byte
}

// sortJSON retains the first duplicate property, as the desktop formatter
// does. Raw string and numeric literals are retained while object keys sort.
func sortJSON(src string) (string, error) {
	if !json.Valid([]byte(src)) {
		_, err := FormatJSON(src, 2, false)
		return "", err
	}
	decoder := jsontext.NewDecoder(strings.NewReader(src), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	out, err := sortedValue(decoder)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func sortedValue(decoder *jsontext.Decoder) ([]byte, error) {
	switch decoder.PeekKind() {
	case '{':
		if _, err := decoder.ReadToken(); err != nil {
			return nil, err
		}
		var fields []property
		seen := map[string]bool{}
		for decoder.PeekKind() != '}' {
			raw, err := decoder.ReadValue()
			if err != nil {
				return nil, err
			}
			encoded := string(raw)
			var name string
			if err := jsonv2.Unmarshal([]byte(encoded), &name, jsontext.AllowInvalidUTF8(true)); err != nil {
				return nil, err
			}
			value, err := sortedValue(decoder)
			if err != nil {
				return nil, err
			}
			if !seen[name] {
				seen[name] = true
				fields = append(fields, property{name, encoded, value})
			}
		}
		if _, err := decoder.ReadToken(); err != nil {
			return nil, err
		}
		slices.SortStableFunc(fields, func(a, b property) int { return cmp.Compare(a.name, b.name) })
		var out bytes.Buffer
		out.WriteByte('{')
		for i, f := range fields {
			if i > 0 {
				out.WriteByte(',')
			}
			out.WriteString(f.encodedName)
			out.WriteByte(':')
			out.Write(f.value)
		}
		out.WriteByte('}')
		return out.Bytes(), nil
	case '[':
		if _, err := decoder.ReadToken(); err != nil {
			return nil, err
		}
		var out bytes.Buffer
		out.WriteByte('[')
		first := true
		for decoder.PeekKind() != ']' {
			value, err := sortedValue(decoder)
			if err != nil {
				return nil, err
			}
			if !first {
				out.WriteByte(',')
			}
			first = false
			out.Write(value)
		}
		if _, err := decoder.ReadToken(); err != nil {
			return nil, err
		}
		out.WriteByte(']')
		return out.Bytes(), nil
	default:
		value, err := decoder.ReadValue()
		return bytes.Clone(value), err
	}
}
