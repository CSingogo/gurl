<p align="center">
  <img src="assets/banner.svg" alt="gurl: curl, but written in Go" width="720">
</p>

<p align="center">
  <a href="https://gurl.somosingogo.dev">Website</a> ·
  <a href="https://gurl.somosingogo.dev/docs/getting-started">Docs</a> ·
  <a href="https://github.com/CSingogo/gurl/releases">Releases</a> ·
  <a href="PLAN.md">Roadmap</a>
</p>

<p align="center">
  <a href="https://github.com/CSingogo/gurl/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/CSingogo/gurl/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/CSingogo/gurl/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/CSingogo/gurl?color=05acd7"></a>
  <a href="LICENSE"><img alt="MIT license" src="https://img.shields.io/badge/license-MIT-05acd7"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-05acd7">
</p>

# gurl

**gurl** is a curl-compatible HTTP and WebSocket client written in Go. It speaks curl's flags, so your muscle memory works, and it prints readable output when a person is looking at it. One static binary, standard library only.

- [x] curl's flags: `-X -H -d --json -G -u -A -e -i -I -v -s -S -f -o -O -w -L -k -m` and more
- [x] Pretty in a terminal (colour, indented JSON), byte-exact in a pipe
- [x] curl's exit codes, so `if gurl -f …; then` works
- [x] Redirects with curl's rules (`-L`, `--max-redirs`, 302 turns POST into GET)
- [x] A public test server with httpbin-style endpoints at [gurl.somosingogo.dev](https://gurl.somosingogo.dev)
- [ ] Uploads and forms: `-F`, `-T`, `--data-urlencode` (v0.2)
- [ ] Cookies, proxies, compression, HTTP version flags (v0.3)
- [ ] WebSocket, written from scratch against RFC 6455 (v0.4)
- [ ] Downloads: progress, resume, retries, rate limits (v0.5)

## Install

```sh
curl -fsSL https://gurl.somosingogo.dev/install.sh | sh
```

The script verifies the download's sha256 checksum and installs to `/usr/local/bin` (it asks for `sudo`), or to `~/.local/bin` if you set `GURL_INSTALL_DIR=$HOME/.local/bin`. With Go installed:

```sh
go install github.com/CSingogo/gurl/cmd/gurl@latest
```

| System | Status |
| --- | --- |
| macOS, Linux, Windows (amd64, arm64) | Binaries on the [releases page](https://github.com/CSingogo/gurl/releases) |
| Debian and Ubuntu | `.deb` on the releases page; apt repo planned |
| Fedora, Alpine | `.rpm` and `.apk` on the releases page |
| Homebrew, AUR (`gurl-bin`) | Planned |

## Usage

```sh
gurl https://gurl.somosingogo.dev/get
gurl -sSL -H 'Accept: application/json' https://gurl.somosingogo.dev/get
gurl --json '{"name":"gopher"}' https://gurl.somosingogo.dev/post
gurl -s -o /dev/null -w '%{http_code}\n' https://gurl.somosingogo.dev/status/418
```

A bare host gets `http://` added, like curl. If a server answers with an empty redirect, gurl tells you so on a terminal; add `-L` to follow it. Full flag list: `gurl --help` or the [docs](https://gurl.somosingogo.dev/docs/flags).

## Develop

| Task | Command |
| --- | --- |
| Run the tests | `go test -race ./...` |
| Build the CLI | `go build -o gurl ./cmd/gurl` |
| Run the site's API locally | `go run ./cmd/gurl-server` (listens on `127.0.0.1:8088`) |
| Work on the website | `cd web && bun install && bun run dev` |

The code is small on purpose and commented to explain the Go and Linux ideas behind it. Layout, conventions and deploy notes are in [AGENTS.md](AGENTS.md).

## License

[MIT](LICENSE)
