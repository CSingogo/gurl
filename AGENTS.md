# gurl

A curl-inspired HTTP and WebSocket client in Go, plus `gurl-server`, which hosts the docs site and httpbin-style test endpoints at https://gurl.somosingogo.dev. `PLAN.md` is the roadmap and the record of design decisions; read it before changing scope.

The owner is learning Go and Linux by reading this code. Keep commits small (one feature each) and comment the *why* of Go and Linux concepts where a learner would stumble. Do not comment the obvious.

## Commands

- `go test ./...`: all tests (CI also runs `go vet` and `-race`)
- `go run ./cmd/gurl <args>`: run the CLI
- `go run ./cmd/gurl-server`: serve the site and endpoints on 127.0.0.1:8088

## Rules

- Standard library only, unless PLAN.md says otherwise. WebSocket is hand-written (RFC 6455).
- Flags keep curl's names and meaning. Raw output (piped, `-o`, `NO_COLOR`, `--raw`) must match curl byte for byte; pretty output is only for a TTY.
- Exit codes follow curl (`man curl`, EXIT CODES).
- Table-driven tests for parsers and codecs.
