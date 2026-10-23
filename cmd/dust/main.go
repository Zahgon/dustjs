// Copyright (c) 2011 LinkedIn
// Licensed under the MIT License. See LICENSE in the project root.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	dust "github.com/Zahgon/dustjs"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var tf, cf, pd string
	flags := flag.NewFlagSet("dust", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&tf, "template", "", "template file")
	flags.StringVar(&cf, "context", "", "JSON context file")
	flags.StringVar(&pd, "partials", "", "directory of .dust partials")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if tf == "" || cf == "" {
		fmt.Fprintln(stderr, "usage: dust --template FILE --context FILE [--partials DIR]")
		return 2
	}
	tb, e := os.ReadFile(tf)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	cb, e := os.ReadFile(cf)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	var ctx any
	d := json.NewDecoder(strings.NewReader(string(cb)))
	d.UseNumber()
	if e = d.Decode(&ctx); e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	var trailing any
	if e = d.Decode(&trailing); e != io.EOF {
		if e == nil {
			e = fmt.Errorf("unexpected data after JSON value")
		}
		fmt.Fprintln(stderr, e)
		return 1
	}
	parts := map[string]string{}
	if pd != "" {
		ents, e := os.ReadDir(pd)
		if e != nil {
			fmt.Fprintln(stderr, e)
			return 1
		}
		for _, x := range ents {
			if x.IsDir() || filepath.Ext(x.Name()) != ".dust" {
				continue
			}
			b, e := os.ReadFile(filepath.Join(pd, x.Name()))
			if e != nil {
				fmt.Fprintln(stderr, e)
				return 1
			}
			parts[strings.TrimSuffix(x.Name(), ".dust")] = string(b)
		}
	}
	out, e := dust.Render(string(tb), ctx, parts)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	fmt.Fprint(stdout, out)
	return 0
}
