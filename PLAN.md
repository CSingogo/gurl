# Plan: gurl, a curl-inspired HTTP client in Go

## Context
The user is learning Go and Linux and wants a real project to build and host on their VPS (`somo@31.97.114.157`, Ubuntu 24.04, Caddy, behind Cloudflare). gurl is a CLI that uses curl's flags (`gurl localhost:2015`, `gurl -sSL -H ... url`) with nicer output in a terminal. It has its own site and a test server on `gurl.somosingogo.dev`.

The user chose to have Claude build it while they read along. Commits stay small, one feature each, with comments that explain the Go and Linux concepts, so the history reads like a tutorial.

## Decisions (settled in the grilling session)
- **Product:** curl-inspired with its own UX. curl flag names and semantics are kept. Pretty output (colors, pretty JSON) is used only when writing to a TTY. Raw curl-identical output is used when piped, with `-o`, with `NO_COLOR`, or with `--raw`.
- **Protocols:** HTTP/HTTPS (1.1 and 2 via stdlib), plus WebSocket (`ws://`, `wss://`) implemented from scratch per RFC 6455, using no library.
- **curl source:** reference only (man pages, `tests/` for expected behaviour). Fresh Go code, no fork and no cgo.
- **Arg parsing:** hand-written parser that supports combined short flags (`-sSL`), repeated flags, `--flag=value`, and `-dvalue`.
- **Repo:** one public repo, `github.com/CSingogo/gurl`, local at `~/Repos/gurl`. MIT license. Go 1.26 (installed locally).
- **Hosting:** a single host, `gurl.somosingogo.dev`. The Go server renders the docs site (`html/template` + `embed`), serves `install.sh`, and serves httpbin-style endpoints on the same host. The apt repo is static under `/apt`. Run as a systemd service behind a Caddy `reverse_proxy`.
- **Distribution:** GitHub Releases via GoReleaser (linux, darwin and windows, each amd64 and arm64), `go install`, a Homebrew tap (`CSingogo/homebrew-tap`), a signed apt repo at `gurl.somosingogo.dev/apt`, and the AUR package `gurl-bin`. The name `gurl` is already taken on the AUR, so the package uses `conflicts=(gurl)`.
- **Testing:** table-driven unit tests, integration tests against `httptest` servers, and a CI job that runs the same requests through real curl and `gurl --raw` and diffs the results.
- **Releases:** staged.
  - v0.1: core HTTP + site + install.sh + echo server
  - v0.2: uploads and forms
  - v0.3: cookies, proxy, compression, HTTP version flags
  - v0.4: WebSocket
  - v0.5: downloads (progress, ranges, resume, retry, rate limit)
  - v1.0: hardening

## Repo layout
```
cmd/gurl/            CLI entry point
cmd/gurl-server/     site + echo server entry point
internal/cli/        arg parser -> Options struct (table-driven tests)
internal/request/    Options -> *http.Request / http.Client (redirects, timeouts, TLS -k, auth)
internal/output/     raw vs pretty writer, -v/-i/-I formatting, -w write-out variables
internal/ws/         RFC 6455 handshake + frame codec + interactive session (v0.4)
internal/server/     handlers, rate limiting, embedded site templates/static
site/                templates + CSS (embedded into gurl-server)
scripts/install.sh   detects OS/arch, downloads from GitHub Releases, verifies checksum
deploy/              gurl.service (systemd), Caddyfile snippet, server setup notes
.goreleaser.yaml     builds, archives, checksums, brew tap, nfpm .deb, AUR gurl-bin
.github/workflows/   ci.yml (vet, test, curl-compare), release.yml (tag -> goreleaser), deploy.yml (main -> server)
AGENTS.md, README.md, LICENSE
```

## v0.1 scope (first build)
0. **Location.** Create `~/Repos/gurl` (next to somo-dev). Copy this plan into it as `PLAN.md` and commit it as the project roadmap. Update PLAN.md as releases ship.
1. **Scaffold.** `go mod init github.com/CSingogo/gurl`, MIT license, README, AGENTS.md, and `ci.yml` running `go vet` and `go test ./...`.
2. **Parser** (`internal/cli`). Positional URL (a bare `host:port` gets `http://`, like curl). Flags:
   - Request: `-X -H -d --data-raw --data-binary --json -G -A -e -u`
   - Output: `-i -I -v -s -S -f -o -O -w`
   - Connection: `-L --max-redirs -k -m --connect-timeout`
   - gurl's own: `--raw`
   - `-d @file` and `-d @-` (stdin).
   - Errors use curl's exit codes (e.g. 6 can't resolve, 7 can't connect, 22 with `-f`, 28 timeout).
3. **Request** (`internal/request`). Build on `net/http`. Redirect policy matches `-L`/`--max-redirs` (curl's method-change rules for 301/302/303). Use `httptrace` timings for `-w` (`%{http_code}`, `%{time_total}`, `%{size_download}`, etc.).
4. **Output** (`internal/output`).
   - TTY detection via `os.Stdout.Stat()` and `NO_COLOR`.
   - Pretty mode: colored status line and headers, indented JSON when the Content-Type is JSON.
   - Raw mode: exact bytes.
   - `-v` prints `>`/`<`/`*` lines to stderr like curl.
5. **gurl-server** (`internal/server`), stdlib `net/http` only, listening on `127.0.0.1:8088`.
   - Endpoints: `/get /post /put /patch /delete /anything /headers /ip /user-agent /status/{code} /delay/{n} /bytes/{n} /redirect/{n} /basic-auth/{u}/{p} /json /gzip` (`/ws` echo is added in v0.4).
   - Limits: max body 1 MB, `/delay` up to 10 s, `/bytes` up to 1 MB, and a per-IP token-bucket rate limit keyed on `CF-Connecting-IP`.
   - Docs pages at `/` (what it is, install, flag reference, try-it examples using its own endpoints) and `/install.sh`.
   - Graceful shutdown on SIGTERM, structured logs via `log/slog` (read with `journalctl -u gurl`).
6. **install.sh.** POSIX sh. Detects `uname -s`/`-m`, fetches the latest release asset and `checksums.txt`, verifies sha256, and installs to `/usr/local/bin` (or `~/.local/bin` without sudo).
7. **Release pipeline.** `.goreleaser.yaml` (6 targets, tar.gz/zip, checksums). `release.yml` runs on tag `v*`. Homebrew, apt and AUR blocks are added once their secrets exist (step 9).
8. **Deploy.** `deploy.yml` on push to main:
   - Build `gurl-server` for linux/amd64.
   - rsync it to `/opt/gurl/` and run `sudo systemctl restart gurl`. This uses a narrow sudoers rule for that one command only.
   - Reuse the existing `SSH_*` secrets pattern from somo-dev's deploy.yml.
9. **Distribution follow-up (v0.1.x).** Turn on the Homebrew tap, the signed apt repo and the AUR `gurl-bin` publishing as each secret is provided.

## One-time server and account setup
Claude runs the server-side setup over SSH, but only after the user approves the exact commands at that point.
- **Cloudflare DNS (user):** A record `gurl` → `31.97.114.157`, proxied. The free universal certificate covers one subdomain level, so `gurl.` is fine. Cloudflare proxies WebSockets on the free plan.
- **VPS:**
  - Create a system user `gurl` and `/opt/gurl`.
  - Install `deploy/gurl.service` (`User=gurl`, `Restart=on-failure`, hardening options such as `NoNewPrivileges` and `ProtectSystem=strict`).
  - Add a sudoers drop-in allowing `somo` to run only `systemctl restart gurl`.
  - Add a `gurl.somosingogo.dev` block to `/etc/caddy/Caddyfile` with `reverse_proxy 127.0.0.1:8088` and `/apt/*` served from `/var/www/gurl/apt`. Also add it to `somo-dev/deploy/Caddyfile`, the source of truth.
- **GitHub (user, or Claude via `gh` with approval):**
  - Create the `CSingogo/gurl` and `CSingogo/homebrew-tap` repos.
  - Secrets: `SSH_*` (same values as somo-dev), `HOMEBREW_TAP_TOKEN` (fine-grained PAT), `APT_GPG_PRIVATE_KEY` + passphrase (user generates with `brew install gnupg`; Claude never handles the key), `AUR_SSH_KEY` (user creates an AUR account and adds the key).

## Verification
- `go vet ./... && go test ./...` pass locally and in CI.
- The curl-compare CI job shows identical status, headers (minus Date) and bodies for a request matrix against a local gurl-server.
- Manual tests:
  - `gurl localhost:8088/get`, `gurl -sSL -o out.json localhost:8088/redirect/3`
  - `gurl -v -X POST --json '{"a":1}' localhost:8088/post`
  - `gurl -w '%{http_code}' -s -o /dev/null localhost:8088/status/418`
  - piped vs TTY output
- After deploy:
  - `curl -I https://gurl.somosingogo.dev/` returns 200.
  - `systemctl status gurl` shows active.
  - `journalctl -u gurl` shows request logs.
  - `curl -fsSL https://gurl.somosingogo.dev/install.sh | sh` installs v0.1.0, then `gurl gurl.somosingogo.dev/get` works.
- Release: tagging `v0.1.0` produces 6 archives + checksums on GitHub Releases, and `go install github.com/CSingogo/gurl/cmd/gurl@latest` works.
