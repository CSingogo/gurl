# gurl

A curl-inspired HTTP client written in Go. It uses curl's flags, so muscle memory works, and adds friendlier output in a terminal.

```sh
gurl localhost:2015
gurl -sSL -H 'Accept: application/json' gurl.somosingogo.dev/get
gurl -X POST --json '{"name":"gurl"}' gurl.somosingogo.dev/post
```

When stdout is a terminal, gurl colors the status line and headers and pretty-prints JSON. When output is piped, written with `-o`, or `NO_COLOR` / `--raw` is set, it prints exactly what curl would.

Status: early development. See [PLAN.md](PLAN.md) for the roadmap.

## License

MIT
