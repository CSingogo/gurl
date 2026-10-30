// Package output decides how responses are shown: raw like curl, or colored and
// pretty-printed when a human is looking at a terminal.
package output

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Style turns text into colored text. The zero value (no color) returns it unchanged.
type Style struct{ Color bool }

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
)

func (s Style) paint(code, text string) string {
	if !s.Color {
		return text
	}
	return code + text + reset
}

// StatusLine renders "HTTP/1.1 200 OK". curl prints HTTP/2 responses as
// "HTTP/2 200" with no reason phrase, so gurl does too.
func StatusLine(resp *http.Response) string {
	if resp.ProtoMajor >= 2 {
		return fmt.Sprintf("HTTP/%d %d", resp.ProtoMajor, resp.StatusCode)
	}
	return fmt.Sprintf("HTTP/%d.%d %s", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
}

func (s Style) status(resp *http.Response) string {
	code := green
	switch {
	case resp.StatusCode >= 500:
		code = red
	case resp.StatusCode >= 400:
		code = yellow
	case resp.StatusCode >= 300:
		code = cyan
	}
	return s.paint(bold+code, StatusLine(resp))
}

// WriteHeaders prints the status line and headers followed by a blank line.
//
// net/http stores headers in a map, so the order the server sent them in is
// gone; they are printed sorted so output is at least stable. eol is "\r\n"
// for raw output (what is on the wire and what curl -i prints) and "\n" when a
// human is reading.
func WriteHeaders(w io.Writer, resp *http.Response, s Style, eol string) {
	fmt.Fprint(w, s.status(resp), eol)
	names := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, v := range resp.Header[name] {
			fmt.Fprint(w, s.paint(cyan, name), ": ", v, eol)
		}
	}
	fmt.Fprint(w, eol)
}

// WriteVerboseResponse prints the response side of -v: "< " before each line.
func WriteVerboseResponse(w io.Writer, resp *http.Response) {
	fmt.Fprintln(w, "< "+StatusLine(resp))
	names := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, v := range resp.Header[name] {
			fmt.Fprintf(w, "< %s: %s\n", name, v)
		}
	}
	fmt.Fprintln(w, "<")
}

// FileNameFromURL is what -O uses: the last path segment, or "" if there is none.
func FileNameFromURL(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	return path
}
