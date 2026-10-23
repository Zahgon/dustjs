// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRendersFiles(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "main.dust")
	context := filepath.Join(dir, "context.json")
	if err := os.WriteFile(template, []byte("Hi {name}!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(context, []byte(`{"name":"<Ada>"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"--template", template, "--context", context}, &stdout, &stderr); status != 0 {
		t.Fatalf("run status=%d stderr=%q", status, stderr.String())
	}
	if got := stdout.String(); got != "Hi &lt;Ada&gt;!" {
		t.Fatalf("stdout=%q", got)
	}
}

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run(nil, &stdout, &stderr); status != 2 {
		t.Fatalf("status=%d", status)
	}
	if stderr.String() != "usage: dust --template FILE --context FILE [--partials DIR]\n" {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
