// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.

package dust

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Kind byte

const (
	Text Kind = iota
	Ref
	Section
	Exists
	NotExists
	Partial
	Block
	Inline
	Raw
	Special
	Pragma
)

type Expr struct {
	Path      []string
	Current   bool
	Literal   any
	IsLiteral bool
}
type Param struct {
	Name  string
	Value Expr
}
type Node struct {
	Kind      Kind
	Text      string
	Expr      Expr
	Filters   []string
	Params    []Param
	Context   *Expr
	Body      []*Node
	Else      map[string][]*Node
	Line, Col int
}
type Template struct{ Nodes []*Node }

func Parse(src string) (*Template, error) {
	p := parser{s: src, line: 1, col: 1}
	n, _, err := p.body("")
	if err != nil {
		return nil, err
	}
	return &Template{n}, nil
}

type parser struct {
	s            string
	i, line, col int
}

func (p *parser) pos() (int, int) { return p.line, p.col }
func (p *parser) take(n int) string {
	v := p.s[p.i : p.i+n]
	for _, r := range v {
		if r == '\n' {
			p.line++
			p.col = 1
		} else {
			p.col++
		}
	}
	p.i += n
	return v
}
func (p *parser) fail(msg string, l, c int) error {
	// renderSource compiles an anonymous template, so upstream reports the
	// literal name "undefined" rather than the CLI input filename.
	return fmt.Errorf("%s [undefined:%d:%d]", msg, l, c)
}
func (p *parser) body(end string) ([]*Node, string, error) {
	var out []*Node
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, &Node{Kind: Text, Text: text.String()})
			text.Reset()
		}
	}
	for p.i < len(p.s) {
		if strings.HasPrefix(p.s[p.i:], "{!") {
			flush()
			l, c := p.pos()
			j := strings.Index(p.s[p.i+2:], "!}")
			if j < 0 {
				return nil, "", p.fail("Expected comment end but end of input found.", l, c)
			}
			p.take(j + 4)
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "{`") {
			flush()
			l, c := p.pos()
			j := strings.Index(p.s[p.i+2:], "`}")
			if j < 0 {
				return nil, "", p.fail("Expected raw end but end of input found.", l, c)
			}
			p.take(2)
			v := p.take(j)
			p.take(2)
			out = append(out, &Node{Kind: Raw, Text: v, Line: l, Col: c})
			continue
		}
		if p.s[p.i] == '{' {
			flush()
			n, ctl, err := p.tag(end)
			if err != nil {
				return nil, "", err
			}
			if ctl != "" {
				return out, ctl, nil
			}
			if n != nil {
				out = append(out, n)
			}
			continue
		}
		if p.s[p.i] == '\r' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '\n' {
				p.take(2)
			} else {
				p.i++
				p.line++
				p.col = 1
			}
			continue
		}
		if p.s[p.i] == '\n' {
			// Compiler format nodes disappear when dust.config.whitespace is false;
			// the whole following Dust whitespace run is therefore removed too.
			p.take(1)
			for p.i < len(p.s) && strings.ContainsRune(" \t\v\f\r\n", rune(p.s[p.i])) {
				if p.s[p.i] == '\r' && (p.i+1 == len(p.s) || p.s[p.i+1] != '\n') {
					p.i++
					p.line++
					p.col = 1
				} else {
					p.take(1)
				}
			}
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "\u2028") || strings.HasPrefix(p.s[p.i:], "\u2029") {
			p.i += 3
			p.line++
			p.col = 1
			continue
		}
		text.WriteString(p.take(1))
	}
	flush()
	if end != "" {
		return nil, "", p.fail("Expected end tag for "+end+" but it was not found.", p.line, p.col)
	}
	return out, "", nil
}
func (p *parser) tag(end string) (*Node, string, error) {
	l, c := p.pos()
	start := p.i
	j := strings.IndexByte(p.s[p.i:], '}')
	if j < 0 {
		p.take(1)
		return &Node{Kind: Text, Text: "{"}, "", nil
	}
	j += p.i
	raw := p.s[p.i+1 : j]
	p.take(j - p.i + 1)
	trim := strings.TrimSpace(raw)
	if trim == "" {
		return &Node{Kind: Text, Text: p.s[start:p.i]}, "", nil
	}
	if trim[0] == '/' {
		name := strings.TrimSpace(trim[1:])
		if end == "" || name != end {
			return nil, "", p.fail("Expected end tag for "+end+" but it was not found.", p.line, p.col)
		}
		return nil, "end", nil
	}
	if trim[0] == ':' {
		return nil, strings.TrimSpace(trim[1:]), nil
	}
	if trim[0] == '~' {
		m := map[string]string{"s": " ", "n": "\n", "r": "\r", "lb": "{", "rb": "}"}
		return &Node{Kind: Special, Text: m[strings.TrimSpace(trim[1:])], Line: l, Col: c}, "", nil
	}
	op := trim[0]
	if strings.ContainsRune("#?^<+@%>", rune(op)) {
		self := strings.HasSuffix(trim, "/")
		inside := strings.TrimSpace(trim[1:])
		if self {
			inside = strings.TrimSpace(strings.TrimSuffix(inside, "/"))
		}
		name, ctx, params, err := parseHead(inside)
		if err != nil {
			return nil, "", p.fail(err.Error(), l, c)
		}
		k := Section
		switch op {
		case '?':
			k = Exists
		case '^':
			k = NotExists
		case '>':
			k = Partial
		case '+':
			k = Block
		case '<':
			k = Inline
		case '@':
			k = Section
		case '%':
			k = Pragma
		}
		if (op == '>' || op == '+' || op == '<') && !name.IsLiteral && len(name.Path) == 1 {
			name = Expr{Literal: name.Path[0], IsLiteral: true}
		}
		node := &Node{Kind: k, Expr: name, Params: params, Context: ctx, Else: map[string][]*Node{}, Line: l, Col: c}
		if op == '>' || self {
			return node, "", nil
		}
		body, ctl, err := p.body(exprName(name))
		if err != nil {
			return nil, "", err
		}
		node.Body = body
		for ctl != "end" {
			b, next, e := p.body(exprName(name))
			if e != nil {
				return nil, "", e
			}
			node.Else[ctl] = b
			ctl = next
		}
		return node, "", nil
	}
	expr, filters, err := parseRef(trim)
	if err != nil {
		return &Node{Kind: Text, Text: p.s[start:p.i]}, "", nil
	}
	return &Node{Kind: Ref, Expr: expr, Filters: filters, Line: l, Col: c}, "", nil
}
func exprName(e Expr) string {
	if e.IsLiteral {
		return fmt.Sprint(e.Literal)
	}
	return strings.Join(e.Path, ".")
}
func fields(s string) []string {
	var r []string
	var b strings.Builder
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			b.WriteByte(ch)
			if ch == quote && (i == 0 || s[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			b.WriteByte(ch)
		} else if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if b.Len() > 0 {
				r = append(r, b.String())
				b.Reset()
			}
		} else {
			b.WriteByte(ch)
		}
	}
	if b.Len() > 0 {
		r = append(r, b.String())
	}
	return r
}
func parseHead(s string) (Expr, *Expr, []Param, error) {
	f := fields(s)
	if len(f) == 0 {
		return Expr{}, nil, nil, errors.New("Expected identifier")
	}
	e := parseExpr(f[0])
	var ctx *Expr
	var ps []Param
	for _, v := range f[1:] {
		if strings.HasPrefix(v, ":") {
			x := parseExpr(v[1:])
			ctx = &x
			continue
		}
		q := strings.IndexByte(v, '=')
		if q < 1 {
			continue
		}
		ps = append(ps, Param{v[:q], parseExpr(v[q+1:])})
	}
	return e, ctx, ps, nil
}
func parseRef(s string) (Expr, []string, error) {
	a := strings.Split(s, "|")
	if a[0] == "" {
		return Expr{}, nil, errors.New("bad reference")
	}
	return parseExpr(strings.TrimSpace(a[0])), a[1:], nil
}
func parseExpr(s string) Expr {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		u, e := strconv.Unquote(s)
		if e == nil {
			return Expr{Literal: u, IsLiteral: true}
		}
	}
	if s == "true" {
		return Expr{Literal: true, IsLiteral: true}
	}
	if s == "false" {
		return Expr{Literal: false, IsLiteral: true}
	}
	if s == "null" {
		return Expr{Literal: nil, IsLiteral: true}
	}
	if n, e := strconv.ParseFloat(s, 64); e == nil {
		return Expr{Literal: n, IsLiteral: true}
	}
	// Dust's bracket grammar treats identifier members as dynamic path pieces;
	// the JSON-only CLI has no identifier-valued bracket expression.
	for rest := s; strings.Contains(rest, "["); {
		open := strings.IndexByte(rest, '[')
		close := strings.IndexByte(rest[open+1:], ']')
		if close < 0 {
			break
		}
		member := rest[open+1 : open+1+close]
		if _, err := strconv.Atoi(member); err != nil {
			return Expr{Path: []string{"__dust_missing_bracket__"}}
		}
		rest = rest[open+close+2:]
	}
	cur := strings.HasPrefix(s, ".")
	s = strings.TrimPrefix(s, ".")
	s = strings.ReplaceAll(s, "[", ".")
	s = strings.ReplaceAll(s, "]", "")
	var path []string
	for _, x := range strings.Split(s, ".") {
		if x != "" {
			path = append(path, x)
		}
	}
	return Expr{Path: path, Current: cur}
}
