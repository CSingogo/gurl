// Package app is the whole CLI minus os.Args/os.Exit, so tests can drive it
// with plain readers and writers.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/CSingogo/gurl/internal/cli"
	"github.com/CSingogo/gurl/internal/output"
	"github.com/CSingogo/gurl/internal/request"
)

// Run executes gurl with args (without the program name) and returns the exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, err := cli.Parse(args)
	if err != nil {
		return fail(stderr, false, err, true)
	}
	switch {
	case o.Help:
		fmt.Fprint(stdout, helpText)
		return 0
	case o.Version:
		fmt.Fprintf(stdout, "gurl %s\n", request.Version)
		return 0
	}

	// Silent mode hides errors too, unless -S asks for them back.
	quiet := o.Silent && !o.ShowError

	u, err := cli.NormalizeURL(o.URL)
	if err != nil {
		return fail(stderr, quiet, err, false)
	}
	if u.Scheme == "ws" || u.Scheme == "wss" {
		return fail(stderr, quiet, &cli.ExitError{Code: cli.ExitUnsupportedProtocol, Msg: "WebSocket support is coming in v0.4"}, false)
	}

	var verbose io.Writer
	if o.Verbose {
		verbose = stderr
	}
	res, err := request.Do(ctx, o, stdin, verbose)
	if err != nil {
		return fail(stderr, quiet, err, false)
	}
	defer res.Resp.Body.Close()

	return show(o, u, res, stdout, stderr, quiet)
}

// show writes the response according to the output flags and returns the exit code.
func show(o *cli.Options, u *url.URL, res *request.Result, stdout, stderr io.Writer, quiet bool) int {
	resp := res.Resp

	// -f: HTTP errors become exit 22 and the body is not printed.
	if o.Fail && resp.StatusCode >= 400 {
		res.Done()
		writeOut(o, res, 0, stdout)
		msg := fmt.Sprintf("The requested URL returned error: %d", resp.StatusCode)
		return fail(stderr, quiet, &cli.ExitError{Code: cli.ExitHTTPError, Msg: msg}, false)
	}

	dest, closeDest, err := destination(o, u, stdout)
	if err != nil {
		return fail(stderr, quiet, err, false)
	}
	toTerminal := dest == stdout && output.IsTerminal(stdout) && !o.Raw
	style := output.Style{Color: toTerminal}

	if o.Verbose {
		output.WriteVerboseResponse(stderr, resp)
	}
	if o.Include || o.Head {
		eol := "\r\n"
		if toTerminal {
			eol = "\n"
		}
		output.WriteHeaders(dest, resp, style, eol)
	}

	var n int64
	if !o.Head {
		n, err = copyBody(dest, resp, style, toTerminal)
		if err != nil {
			closeDest()
			return fail(stderr, quiet, &cli.ExitError{Code: 56, Msg: "Failure when receiving data: " + err.Error()}, false)
		}
	}
	if err := closeDest(); err != nil {
		return fail(stderr, quiet, &cli.ExitError{Code: cli.ExitWriteError, Msg: err.Error()}, false)
	}
	res.Done()
	writeOut(o, res, n, stdout)
	return 0
}

// copyBody streams the body to w. On a terminal, JSON bodies are buffered (up
// to a cap) so they can be indented and colored; everything else streams as is.
func copyBody(w io.Writer, resp *http.Response, s output.Style, pretty bool) (int64, error) {
	const maxPretty = 8 << 20
	if pretty && output.IsJSON(resp.Header.Get("Content-Type")) {
		buf, err := io.ReadAll(io.LimitReader(resp.Body, maxPretty+1))
		if err != nil {
			return int64(len(buf)), err
		}
		if len(buf) <= maxPretty {
			if out, ok := output.PrettyJSON(buf, s); ok {
				_, err := w.Write(out)
				return int64(len(buf)), err
			}
		}
		// Too big or not valid JSON: print what was read, then stream the rest.
		if _, err := w.Write(buf); err != nil {
			return 0, err
		}
		rest, err := io.Copy(w, resp.Body)
		return int64(len(buf)) + rest, err
	}
	return io.Copy(w, resp.Body)
}

// destination picks where the body goes: stdout, or a file for -o / -O.
func destination(o *cli.Options, u *url.URL, stdout io.Writer) (io.Writer, func() error, error) {
	name := o.Output
	if o.RemoteName {
		name = output.FileNameFromURL(u.Path)
		if name == "" {
			return nil, nil, &cli.ExitError{Code: cli.ExitWriteError, Msg: "Remote file name has no length!"}
		}
	}
	if name == "" || name == "-" {
		return stdout, func() error { return nil }, nil
	}
	f, err := os.Create(name)
	if err != nil {
		return nil, nil, &cli.ExitError{Code: cli.ExitWriteError, Msg: "Failure writing output to destination: " + err.Error()}
	}
	return f, f.Close, nil
}

// writeOut prints the -w format after the body.
func writeOut(o *cli.Options, res *request.Result, size int64, stdout io.Writer) {
	if o.WriteOut == "" {
		return
	}
	resp := res.Resp
	vars := map[string]string{
		"http_code":          strconv.Itoa(resp.StatusCode),
		"response_code":      strconv.Itoa(resp.StatusCode),
		"url_effective":      res.EffectiveURL,
		"content_type":       resp.Header.Get("Content-Type"),
		"size_download":      strconv.FormatInt(size, 10),
		"num_redirects":      strconv.Itoa(res.Redirects),
		"redirect_url":       resp.Header.Get("Location"),
		"http_version":       httpVersion(resp),
		"remote_ip":          res.RemoteIP,
		"remote_port":        res.RemotePort,
		"method":             resp.Request.Method,
		"time_namelookup":    output.Seconds(res.Timings.NameLookup),
		"time_connect":       output.Seconds(res.Timings.Connect),
		"time_appconnect":    output.Seconds(res.Timings.AppConnect),
		"time_starttransfer": output.Seconds(res.Timings.StartTransfer),
		"time_total":         output.Seconds(res.Timings.Total),
	}
	io.WriteString(stdout, output.Expand(o.WriteOut, func(n string) (string, bool) { v, ok := vars[n]; return v, ok }))
}

func httpVersion(resp *http.Response) string {
	if resp.ProtoMajor >= 2 {
		return strconv.Itoa(resp.ProtoMajor)
	}
	return fmt.Sprintf("%d.%d", resp.ProtoMajor, resp.ProtoMinor)
}

// fail prints "gurl: (code) message" and returns the exit code. Usage errors
// also point at --help.
func fail(stderr io.Writer, quiet bool, err error, usage bool) int {
	var ee *cli.ExitError
	if !errors.As(err, &ee) {
		ee = &cli.ExitError{Code: 1, Msg: err.Error()}
	}
	if !quiet {
		fmt.Fprintf(stderr, "gurl: (%d) %s\n", ee.Code, ee.Msg)
		if usage {
			fmt.Fprintln(stderr, "Try 'gurl --help' for more information.")
		}
	}
	return ee.Code
}

const helpText = `gurl: a curl-inspired HTTP client

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
