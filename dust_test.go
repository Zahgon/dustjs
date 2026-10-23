// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.
package dust

import "testing"

func TestRenderCore(t *testing.T) {
	out, err := Render("{#xs}{$idx}:{.};{:else}empty{/xs}|{name}", map[string]any{
		"name": "<Ada>", "xs": []any{"a", "b"},
	}, nil)
	if err != nil || out != "0:a;1:b;|&lt;Ada&gt;" {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestFiltersAndPartials(t *testing.T) {
	out, err := Render("{>card who=name/}", map[string]any{"name": "a b"}, map[string]string{
		"card": "[{who|u}]",
	})
	if err != nil || out != "[a%20b]" {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestParseError(t *testing.T) {
	if _, err := Parse("{#x}missing"); err == nil {
		t.Fatal("Parse accepted an unterminated section")
	}
}
