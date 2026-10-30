# gurl

A curl-inspired HTTP and WebSocket client in Go, plus `gurl-server`, which hosts the docs site and httpbin-style test endpoints at https://gurl.somosingogo.dev. `PLAN.md` is the roadmap and the record of design decisions; read it before changing scope.

The owner is learning Go and Linux by reading this code. Keep commits small (one feature each) and comment the *why* of Go and Linux concepts where a learner would stumble. Do not comment the obvious.

## Commands

- `go test ./...`: all tests (CI also runs `go vet` and `-race`)
- `go run ./cmd/gurl <args>`: run the CLI (`go build -o bin/gurl ./cmd/gurl` for a binary)
- `go run ./cmd/gurl-server`: serve the site and endpoints on 127.0.0.1:8088

## Layout

- `internal/cli`: flag parser and URL normalization (pure, no I/O)
- `internal/request`: Options to `*http.Request`, client policy (redirects, timeouts, TLS), error to exit-code mapping
- `internal/output`: headers, pretty JSON, `-w` expansion
- `internal/app`: ties them together; `cmd/gurl` is a thin `main`

## Rules

- Standard library only, unless PLAN.md says otherwise. WebSocket is hand-written (RFC 6455).
- Flags keep curl's names and meaning. Raw output (piped, `-o`, `NO_COLOR`, `--raw`) must match curl byte for byte for bodies; pretty output is only for a TTY. Response headers are the exception: `net/http` keeps them in a map, so the server's order and original casing are lost and gurl prints them sorted.
- Exit codes follow curl (`man curl`, EXIT CODES).
- Table-driven tests for parsers and codecs.

## Website (`web/`)

Astro 7, static output, Tailwind v4, MDX docs, bun. Same repo, separate toolchain.

- `cd web && bun run dev` / `bun run build` (runs `astro check`, must pass cleanly).
- Docs are MDX files in `web/src/docs/` (frontmatter: title, description, section, order). The sidebar and ⌘K search are generated from them.
- System-specific docs: wrap content in `<Os for="macos linux">`. The picker state lives in `html[data-os]` and `localStorage["gurl-os"]`; see the script in `layouts/Base.astro`.
- Install commands for the home page terminal are in `components/InstallTerminal.astro`. Channels that don't exist yet are marked `soon`; remove the flag when the channel ships.
- Hosting: CI rsyncs `web/dist/` to `/var/www/gurl/site/`. Caddy serves it and proxies only the Go server's paths (list in `deploy/Caddyfile.snippet`). When you add a route to `internal/server`, add it to that `@api` matcher, install it by hand on the VPS and copy it to `somo-dev/deploy/Caddyfile`.
- The Go server's own `/` page (`site/` at the repo root) is no longer reachable through Caddy; it remains as a fallback for local runs.
