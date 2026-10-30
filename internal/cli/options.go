// Package cli turns command-line arguments into an Options value.
//
// The parser is a pure function: arguments in, Options (or an error) out. It
// never touches the network, the filesystem or os.Args, which is what makes it
// easy to cover with table-driven tests.
package cli

import "time"

// DataKind says how a -d style argument must be treated when the body is built.
type DataKind int

const (
	// DataText is -d / --data: a leading @ means "read this file", and
	// newlines and carriage returns in file contents are stripped (curl does this).
	DataText DataKind = iota
	// DataRaw is --data-raw: sent exactly as given, @ has no special meaning.
	DataRaw
	// DataBinary is --data-binary: like DataText but file contents are untouched.
	DataBinary
	// DataJSON is --json: like DataBinary, and also sets the JSON Content-Type
	// and Accept headers.
	DataJSON
)

// DataItem is one -d / --data-raw / --data-binary / --json argument.
type DataItem struct {
	Kind  DataKind
	Value string
}

// Options is everything the command line asked for. Zero values mean "not set",
// except MaxRedirs, whose default of 50 matches curl.
type Options struct {
	URL string

	Method    string   // -X
	Headers   []string // -H, in order; "Name: value" or "Name:" to remove or "Name;" to send empty
	Data      []DataItem
	GetMode   bool   // -G: put the data in the query string instead of the body
	UserAgent string // -A
	Referer   string // -e
	User      string // -u user:password

	Include    bool   // -i: print response headers before the body
	Head       bool   // -I: HEAD request, print headers only
	Verbose    bool   // -v
	Silent     bool   // -s
	ShowError  bool   // -S: with -s, still print errors
	Fail       bool   // -f: exit 22 on HTTP status >= 400 without printing the body
	Output     string // -o file
	RemoteName bool   // -O: name the file after the URL's last path segment
	WriteOut   string // -w format

	Location       bool          // -L
	MaxRedirs      int           // --max-redirs
	Insecure       bool          // -k
	MaxTime        time.Duration // -m
	ConnectTimeout time.Duration // --connect-timeout

	Raw     bool // --raw: never colorize or pretty-print, even on a terminal
	Help    bool // -h / --help
	Version bool // -V / --version
}

// ExitError is returned for problems the user can fix on the command line. It
// carries the exit code curl would use, so scripts that check $? keep working.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

// Exit codes taken from curl's man page (EXIT CODES section).
const (
	ExitUnsupportedProtocol = 1
	ExitBadUsage            = 2
	ExitMalformedURL        = 3
	ExitCouldntResolveHost  = 6
	ExitCouldntConnect      = 7
	ExitHTTPError           = 22 // -f with a 4xx/5xx response
	ExitWriteError          = 23
	ExitTimeout             = 28
	ExitTooManyRedirects    = 47
	ExitSSL                 = 60
)
