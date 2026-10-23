# Dust for Go

This repository contains a Go port of the Dust 3.0.1 core template engine.
It uses only the Go standard library.

Build the command offline:

```sh
CGO_ENABLED=0 go build -o dust ./cmd/dust
```

Render a UTF-8 template using a JSON context and an optional directory of
`.dust` partials:

```sh
./dust --template page.dust --context data.json --partials partials
```

Run the focused parser, runtime, filter, partial, and command package tests
with `go test ./...`.
