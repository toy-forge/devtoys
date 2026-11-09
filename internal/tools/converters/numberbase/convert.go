package numberbase

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

const maxDigits = 4096

var two64 = new(big.Int).Lsh(big.NewInt(1), 64)
var two63 = new(big.Int).Lsh(big.NewInt(1), 63)
var basicBases = []int{16, 10, 8, 2}

type alphabet struct {
	name, chars	string
	insensitive	bool
}

var alphabets = func() []alphabet {
	a := []alphabet{
		{"RFC 4648 Base16", "0123456789ABCDEF", true},
		{"RFC 4648 Base32", "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", false},
		{"RFC 4648 Base32hex", "0123456789ABCDEFGHIJKLMNOPQRSTUV", false},
		{"RFC 4648 Base64", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", false},
		{"RFC 4648 Base64url", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", false},
		{"Custom", "", false},
	}
	for _, base := range baseOrder {
		a = append(a, alphabet{fmt.Sprintf("Base %d", base), "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"[:base], true})
	}
	return a
}()

const customAlphabet = 5

func cleanBasic(raw string, base int) string {
	var b strings.Builder
	for _, r := range raw {
		if !unicode.IsSpace(r) && r != ',' && r != '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if base == 16 {
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			s = s[2:]
		}
	}
	return s
}
func parseBasic(raw string, base int) (*big.Int, error) {
	s := cleanBasic(raw, base)
	if s == "" {
		return nil, nil
	}
	if len(s) > maxDigits {
		return nil, fmt.Errorf("number exceeds %d digits", maxDigits)
	}
	if err := validateDigits(s, base); err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(s, base)
	if !ok {
		return nil, fmt.Errorf("invalid %s number", baseName(base))
	}
	// Desktop non-decimal inputs interpret a 64-bit high bit as a signed value.
	if base != 10 && s[0] != '-' && s[0] != '+' && n.Cmp(two63) >= 0 && n.Cmp(two64) < 0 {
		n.Sub(n, two64)
	}
	return n, nil
}
func basicValue(n *big.Int, base int, formatted bool) string {
	if n == nil {
		return ""
	}
	v := new(big.Int).Set(n)
	if base != 10 && v.Sign() < 0 && v.IsInt64() {
		v.Add(v, two64)
	}
	s := v.Text(base)
	if base == 16 {
		s = strings.ToUpper(s)
	}
	if base == 2 {
		sign := ""
		if strings.HasPrefix(s, "-") {
			sign = "-"
			s = s[1:]
		}
		if rem := len(s) % 4; rem != 0 {
			s = strings.Repeat("0", 4-rem) + s
		}
		s = sign + s
	}
	if formatted {
		switch base {
		case 2, 16:
			s = groupDigits(s, 4, ' ')
		case 8:
			s = groupDigits(s, 3, ' ')
		case 10:
			s = groupDigits(s, 3, ',')
		}
	}
	return s
}
func groupDigits(s string, size int, sep rune) string {
	sign := ""
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		sign = s[:1]
		s = s[1:]
	}
	r := []rune(s)
	var b strings.Builder
	b.WriteString(sign)
	for i, ch := range r {
		if i > 0 && (len(r)-i)%size == 0 {
			b.WriteRune(sep)
		}
		b.WriteRune(ch)
	}
	return b.String()
}
func validateAlphabet(chars string) ([]rune, error) {
	runes := []rune(chars)
	if len(runes) < 2 || len(runes) > 256 {
		return nil, fmt.Errorf("alphabet must have 2–256 distinct characters")
	}
	seen := make(map[rune]bool, len(runes))
	for _, r := range runes {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, fmt.Errorf("alphabet cannot contain whitespace or control characters")
		}
		if seen[r] {
			return nil, fmt.Errorf("alphabet characters must be distinct")
		}
		seen[r] = true
	}
	return runes, nil
}
func parseAlphabet(raw string, a alphabet) (*big.Int, error) {
	digits, err := validateAlphabet(a.chars)
	if err != nil {
		return nil, err
	}
	lookup := make(map[rune]int, len(digits))
	for i, r := range digits {
		if a.insensitive {
			r = unicode.ToUpper(r)
		}
		lookup[r] = i
	}
	n := new(big.Int)
	base := big.NewInt(int64(len(digits)))
	count := 0
	for _, r := range raw {
		if unicode.IsSpace(r) {
			continue
		}
		count++
		if count > maxDigits {
			return nil, fmt.Errorf("number exceeds %d digits", maxDigits)
		}
		if a.insensitive {
			r = unicode.ToUpper(r)
		}
		digit, ok := lookup[r]
		if !ok {
			return nil, fmt.Errorf("character %q is not in the input alphabet", r)
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(int64(digit)))
	}
	if count == 0 {
		return nil, nil
	}
	return n, nil
}
func formatAlphabet(n *big.Int, a alphabet, formatted bool) (string, error) {
	digits, err := validateAlphabet(a.chars)
	if err != nil {
		return "", err
	}
	if n == nil {
		return "", nil
	}
	if n.Sign() < 0 {
		return "", fmt.Errorf("advanced mode requires a non-negative number")
	}
	if n.Sign() == 0 {
		return string(digits[0]), nil
	}
	v := new(big.Int).Set(n)
	base := big.NewInt(int64(len(digits)))
	remainder := new(big.Int)
	result := make([]rune, 0)
	for v.Sign() > 0 {
		v.QuoRem(v, base, remainder)
		result = append(result, digits[remainder.Int64()])
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	out := string(result)
	if formatted {
		r := []rune(out)
		var b strings.Builder
		for i, ch := range r {
			if i > 0 && (len(r)-i)%4 == 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(ch)
		}
		out = b.String()
	}
	return out, nil
}
