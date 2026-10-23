// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.

package dust

import (
	"fmt"
	"reflect"
	"strings"
)

type frame struct {
	v             any
	parent        *frame
	index, length int
}
type renderer struct {
	partials map[string]*Template
	blocks   []map[string][]*Node
}

func Render(source string, context any, partials map[string]string) (string, error) {
	t, e := Parse(source)
	if e != nil {
		return "", e
	}
	r := renderer{partials: map[string]*Template{}}
	for k, v := range partials {
		x, er := Parse(v)
		if er != nil {
			return "", er
		}
		r.partials[k] = x
	}
	return r.nodes(t.Nodes, &frame{v: context, index: -1, length: -1})
}
func (r *renderer) nodes(ns []*Node, c *frame) (string, error) {
	var b strings.Builder
	for _, n := range ns {
		s, e := r.node(n, c)
		if e != nil {
			return "", e
		}
		b.WriteString(s)
	}
	return b.String(), nil
}
func (r *renderer) node(n *Node, c *frame) (string, error) {
	switch n.Kind {
	case Text, Raw, Special:
		return n.Text, nil
	case Ref:
		v := resolve(n.Expr, c)
		if isEmpty(v) {
			return "", nil
		}
		return applyFilters(v, n.Filters), nil
	case Section, Exists, NotExists:
		v := resolve(n.Expr, c)
		// Dust's three section forms share parsing but not emptiness rules. The
		// helpers below preserve that runtime distinction rather than using Go.
		ok := truthy(v)
		if n.Kind == NotExists {
			ok = !ok
		}
		if !ok {
			return r.nodes(n.Else["else"], c)
		}
		if n.Kind == Exists || n.Kind == NotExists {
			return r.nodes(n.Body, c)
		}
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.Kind() == reflect.Slice {
			if rv.Len() == 0 {
				return r.nodes(n.Else["else"], c)
			}
			var b strings.Builder
			for i := 0; i < rv.Len(); i++ {
				f := &frame{v: rv.Index(i).Interface(), parent: c, index: i, length: rv.Len()}
				s, e := r.nodes(n.Body, f)
				if e != nil {
					return "", e
				}
				b.WriteString(s)
			}
			return b.String(), nil
		}
		return r.nodes(n.Body, &frame{v: v, parent: c, index: -1, length: -1})
	case Partial:
		name := stringify(resolve(n.Expr, c))
		t := r.partials[name]
		if t == nil {
			return "", fmt.Errorf("Template Not Found: %s", name)
		}
		base := c
		if n.Context != nil {
			base = &frame{v: resolve(*n.Context, c), parent: c, index: -1, length: -1}
		}
		if len(n.Params) > 0 {
			m := map[string]any{}
			for _, p := range n.Params {
				m[p.Name] = resolve(p.Value, c)
			}
			base = &frame{v: m, parent: base, index: -1, length: -1}
		}
		return r.nodes(t.Nodes, base)
	case Inline:
		name := exprName(n.Expr)
		if len(r.blocks) == 0 {
			r.blocks = append(r.blocks, map[string][]*Node{})
		}
		r.blocks[len(r.blocks)-1][name] = n.Body
		return "", nil
	case Block:
		name := exprName(n.Expr)
		for i := len(r.blocks) - 1; i >= 0; i-- {
			if x := r.blocks[i][name]; x != nil {
				return r.nodes(x, c)
			}
		}
		return r.nodes(n.Body, c)
	case Pragma:
		return r.nodes(n.Body, c)
	}
	return "", nil
}
