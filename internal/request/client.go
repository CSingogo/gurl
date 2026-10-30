package request

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"syscall"
	"time"

	"github.com/CSingogo/gurl/internal/cli"
)

// Timings are measured from the moment Do was called, like curl's -w time_* variables.
type Timings struct {
	NameLookup    time.Duration
	Connect       time.Duration
	AppConnect    time.Duration // TLS handshake finished; zero for plain HTTP
	StartTransfer time.Duration // first response byte
	Total         time.Duration // set by Result.Done once the body is read
}

// Result is a finished exchange. The caller owns Resp.Body and must close it.
type Result struct {
	Resp         *http.Response
	Timings      Timings
	Redirects    int
	EffectiveURL string
	RemoteIP     string
	RemotePort   string

	start time.Time
}

// Done records the total time. Call it after the body has been read.
func (r *Result) Done() { r.Timings.Total = time.Since(r.start) }

// errTooManyRedirects is a sentinel so errors.Is can find it through the
// *url.Error that net/http wraps around anything CheckRedirect returns.
var errTooManyRedirects = errors.New("maximum redirects followed")

// Do builds the request from o, sends it and returns the first non-followed
// response. If verbose is non-nil, curl-style "*" and ">" lines are written to it.
func Do(ctx context.Context, o *cli.Options, stdin io.Reader, verbose io.Writer) (*Result, error) {
	req, err := Build(o, stdin)
	if err != nil {
		return nil, err
	}

	res := &Result{start: time.Now(), EffectiveURL: req.URL.String()}
	req = req.WithContext(httptrace.WithClientTrace(ctx, tracer(res, verbose)))

	resp, err := newClient(o, res).Do(req)
	if err != nil {
		return nil, classify(err, req)
	}
	res.Resp = resp
	res.EffectiveURL = resp.Request.URL.String()
	return res, nil
}

func newClient(o *cli.Options, res *Result) *http.Client {
	// Clone keeps the stdlib defaults (HTTP/2, proxy from environment, keep-alives)
	// so only the differences from curl need spelling out.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// curl does not ask for or decode compressed bodies unless --compressed is
	// given; the stdlib would do both silently, which breaks byte-for-byte output.
	tr.DisableCompression = true
	if o.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // that is what -k means
	}
	if o.ConnectTimeout > 0 {
		tr.DialContext = (&net.Dialer{Timeout: o.ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext
	}

	return &http.Client{
		Transport: tr,
		Timeout:   o.MaxTime, // 0 means no limit, same as curl's default
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !o.Location {
				return http.ErrUseLastResponse // hand the 3xx back to the caller untouched
			}
			if o.MaxRedirs >= 0 && len(via) > o.MaxRedirs {
				return errTooManyRedirects
			}
			res.Redirects = len(via)
			// net/http adds a Referer on redirects; curl only sends one if asked.
			if o.Referer == "" {
				req.Header.Del("Referer")
			}
			return nil
		},
	}
}

func tracer(res *Result, verbose io.Writer) *httptrace.ClientTrace {
	since := func() time.Duration { return time.Since(res.start) }
	logf := func(format string, a ...any) {
		if verbose != nil {
			fmt.Fprintf(verbose, format+"\n", a...)
		}
	}
	return &httptrace.ClientTrace{
		DNSDone: func(httptrace.DNSDoneInfo) { res.Timings.NameLookup = since() },
		ConnectStart: func(network, addr string) {
			logf("*   Trying %s...", addr)
		},
		ConnectDone: func(network, addr string, err error) {
			if err != nil {
				return
			}
			res.Timings.Connect = since()
			res.RemoteIP, res.RemotePort, _ = net.SplitHostPort(addr)
			logf("* Connected to %s", addr)
		},
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			if err != nil {
				return
			}
			res.Timings.AppConnect = since()
			logf("* TLS connected: %s, %s", tls.VersionName(cs.Version), tls.CipherSuiteName(cs.CipherSuite))
		},
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				logf("* Re-using existing connection")
			}
		},
		WroteHeaderField: func(key string, value []string) {
			for _, v := range value {
				logf("> %s: %s", key, v)
			}
		},
		WroteHeaders:         func() { logf(">") },
		GotFirstResponseByte: func() { res.Timings.StartTransfer = since() },
	}
}

// classify maps a transport error to the exit code and message curl would give.
func classify(err error, req *http.Request) error {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[req.URL.Scheme]
	}

	var dnsErr *net.DNSError
	var opErr *net.OpError
	var certErr interface{ Error() string }
	switch {
	case errors.Is(err, errTooManyRedirects):
		return &cli.ExitError{Code: cli.ExitTooManyRedirects, Msg: "Maximum (redirects) followed"}
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return &cli.ExitError{Code: cli.ExitTimeout, Msg: "Operation timed out"}
	case errors.As(err, &dnsErr):
		return &cli.ExitError{Code: cli.ExitCouldntResolveHost, Msg: fmt.Sprintf("Could not resolve host: %s", host)}
	case errors.Is(err, syscall.ECONNREFUSED):
		return &cli.ExitError{Code: cli.ExitCouldntConnect, Msg: fmt.Sprintf("Failed to connect to %s port %s: Connection refused", host, port)}
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return &cli.ExitError{Code: cli.ExitCouldntConnect, Msg: fmt.Sprintf("Failed to connect to %s port %s: %v", host, port, opErr.Err)}
	case isCertError(err, &certErr):
		return &cli.ExitError{Code: cli.ExitSSL, Msg: "SSL certificate problem: " + certErr.Error() + "\nUse -k to skip verification."}
	}
	return &cli.ExitError{Code: 56, Msg: "Failure when receiving data: " + err.Error()}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
