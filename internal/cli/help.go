package cli

// HelpText is printed by --help and rendered on the docs site, so the two never drift apart.
const HelpText = `gurl: a curl-inspired HTTP client

Usage: gurl [options] <url>

Request
  -X, --request <method>     HTTP method
  -H, --header <line>        extra header ("Name: value", "Name:" removes, "Name;" sends empty)
  -d, --data <data>          request body (@file reads a file, @- reads stdin)
      --data-raw <data>      body, sent as is
      --data-binary <data>   body from @file with no newline stripping
      --json <data>          body plus JSON Content-Type and Accept
  -G, --get                  put -d data in the query string
  -A, --user-agent <name>    User-Agent header
  -e, --referer <url>        Referer header
  -u, --user <user:pass>     basic authentication

Output
  -i, --include              show response headers
  -I, --head                 HEAD request, show headers only
  -v, --verbose              show the request and response exchange on stderr
  -s, --silent               no error messages (-S shows them again)
  -f, --fail                 exit 22 on HTTP errors instead of printing the body
  -o, --output <file>        write the body to a file
  -O, --remote-name          write to a file named after the URL
  -w, --write-out <format>   print variables after the transfer, e.g. '%{http_code}\n'
      --raw                  never colorize or pretty-print

Connection
  -L, --location             follow redirects
      --max-redirs <n>       redirect limit (default 50, -1 for unlimited)
  -k, --insecure             skip TLS certificate verification
  -m, --max-time <seconds>   whole-operation timeout
      --connect-timeout <s>  connection timeout

  -h, --help                 this text
  -V, --version              print the version
`
