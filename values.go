// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.

package dust

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf16"
)

func resolve(e Expr, c *frame) any {
	if e.IsLiteral {
		return e.Literal
	}
	if len(e.Path) == 0 {
		return c.v
	}
	if len(e.Path) == 1 {
		if e.Path[0] == "$idx" {
			return c.index
		}
		if e.Path[0] == "$len" {
			return c.length
		}
	}
	start := c
	if e.Current {
		if v, ok := down(start.v, e.Path); ok {
			return v
		}
		return nil
	}
	for f := start; f != nil; f = f.parent {
		if v, ok := down(f.v, e.Path); ok {
			return v
		}
	}
	return nil
}
func down(v any, path []string) (any, bool) {
	for _, k := range path {
		if v == nil {
			return nil, false
		}
		switch x := v.(type) {
		case map[string]any:
			y, ok := x[k]
			if !ok {
				return nil, false
			}
			v = y
		case []any:
			i, e := strconv.Atoi(k)
			if e != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			v = x[i]
		default:
			rv := reflect.ValueOf(v)
			// JSON objects are string-keyed. Guard the generic map path as well so
			// library callers cannot trigger reflect.MapIndex with a wrong key type.
			if rv.IsValid() && rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
				key := reflect.ValueOf(k).Convert(rv.Type().Key())
				z := rv.MapIndex(key)
				if !z.IsValid() {
					return nil, false
				}
				v = z.Interface()
			} else {
				return nil, false
			}
		}
	}
	return v, true
}
func truthy(v any) bool {
	// A Dust section is deliberately not Go truthiness: numeric zero renders,
	// empty objects render, but false, nil, empty strings and empty arrays do not.
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	}
	rv := reflect.ValueOf(v)
	return !(rv.IsValid() && rv.Kind() == reflect.Slice && rv.Len() == 0)
}
func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		f, err := x.Float64()
		if err == nil {
			return jsNumber(f)
		}
		return x.String()
	}
	if reflect.TypeOf(v).Kind() == reflect.Map {
		return "[object Object]"
	}
	if reflect.TypeOf(v).Kind() == reflect.Slice {
		rv := reflect.ValueOf(v)
		parts := make([]string, rv.Len())
		for i := range parts {
			parts[i] = stringify(rv.Index(i).Interface())
		}
		return strings.Join(parts, ",")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func jsNumber(f float64) string {
	// JavaScript selects fixed notation through [1e-6, 1e21), unlike Go's
	// default %g thresholds, and prints negative zero as zero.
	if f == 0 {
		return "0"
	}
	a := f
	if a < 0 {
		a = -a
	}
	if a >= 1e-6 && a < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	s = strings.Replace(s, "e+0", "e+", 1)
	s = strings.Replace(s, "e-0", "e-", 1)
	return s
}

func isEmpty(v any) bool {
	// exists/not-exists use dust.isEmpty rather than section truthiness. In
	// particular, zero exists while false and an empty array do not.
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}
func applyFilters(v any, fs []string) string {
	// Dust applies HTML escaping after the explicit chain unless |s disables
	// it. Keeping this final step separate is important for |j and |js.
	s := stringify(v)
	auto := true
	for _, f := range fs {
		switch f {
		case "s":
			auto = false
		case "h":
			s = escapeHTML(s)
			auto = false
		case "u":
			s = encodeURI(s)
		case "uc":
			s = url.QueryEscape(s)
			s = strings.ReplaceAll(s, "+", "%20")
		case "j":
			s = escapeJS(s)
		case "js":
			b, _ := json.Marshal(v)
			s = string(b)
			s = strings.ReplaceAll(s, "<", "\\u003c")
			s = strings.ReplaceAll(s, "\u2028", "\\u2028")
			s = strings.ReplaceAll(s, "\u2029", "\\u2029")
		case "jp":
			var parsed any
			if json.Unmarshal([]byte(s), &parsed) == nil {
				s = stringify(parsed)
			}
		}
	}
	if auto {
		s = escapeHTML(s)
	}
	return s
}

func escapeHTML(s string) string {
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "&#34;", "&quot;")
	return strings.ReplaceAll(s, "&#39;", "&#39;")
}

func escapeJS(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '/':
			b.WriteString("\\/")
		case '\r':
			b.WriteString("\\r")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '"':
			b.WriteString("\\\"")
		case '\'':
			b.WriteString("\\'")
		default:
			if r == 0x2028 || r == 0x2029 || r < 0x20 {
				for _, u := range utf16.Encode([]rune{r}) {
					fmt.Fprintf(&b, "\\u%04x", u)
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func encodeURI(s string) string {
	q := url.QueryEscape(s)
	q = strings.ReplaceAll(q, "+", "%20")
	for code, ch := range map[string]string{"%2F": "/", "%3F": "?", "%3A": ":", "%40": "@", "%26": "&", "%3D": "=", "%2B": "+", "%24": "$", "%2C": ",", "%23": "#"} {
		q = strings.ReplaceAll(q, code, ch)
	}
	return q
}
