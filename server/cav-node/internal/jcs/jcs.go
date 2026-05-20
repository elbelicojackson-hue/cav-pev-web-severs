// Package jcs implements RFC 8785 JSON Canonicalization Scheme.
// This is a minimal implementation sufficient for Praxon signing.
package jcs

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Canonicalize takes a JSON byte slice and returns its RFC 8785 canonical form.
func Canonicalize(input []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(input, &v); err != nil {
		return nil, fmt.Errorf("jcs: unmarshal failed: %w", err)
	}
	var b strings.Builder
	if err := serialize(&b, v); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func serialize(b *strings.Builder, v interface{}) error {
	switch val := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if val {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		b.WriteString(serializeNumber(val))
	case string:
		b.WriteString(serializeString(val))
	case []interface{}:
		b.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := serialize(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]interface{}:
		// RFC 8785: sort keys by UTF-16 code unit order
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return compareUTF16(keys[i], keys[j]) < 0
		})
		b.WriteByte('{')
		first := true
		for _, k := range keys {
			if first {
				first = false
			} else {
				b.WriteByte(',')
			}
			b.WriteString(serializeString(k))
			b.WriteByte(':')
			if err := serialize(b, val[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("jcs: unsupported type %T", v)
	}
	return nil
}

func serializeNumber(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "null"
	}
	if f == 0 {
		return "0"
	}
	// Use ES6 number serialization (shortest representation)
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func serializeString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// compareUTF16 compares two strings by UTF-16 code unit order (RFC 8785 §3.2.3).
func compareUTF16(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	minLen := len(ra)
	if len(rb) < minLen {
		minLen = len(rb)
	}
	for i := 0; i < minLen; i++ {
		if ra[i] != rb[i] {
			// Compare as UTF-16 code units
			au := utf16Units(ra[i])
			bu := utf16Units(rb[i])
			for j := 0; j < len(au) && j < len(bu); j++ {
				if au[j] != bu[j] {
					return int(au[j]) - int(bu[j])
				}
			}
			return len(au) - len(bu)
		}
	}
	return len(ra) - len(rb)
}

func utf16Units(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)}
	}
	r -= 0x10000
	return []uint16{uint16(r>>10) + 0xD800, uint16(r&0x3FF) + 0xDC00}
}
