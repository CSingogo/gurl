package cli

import (
	"net/url"
	"strings"
)

// NormalizeURL applies curl's "bare host" rule: with no scheme, assume http://.
//
// A plain url.Parse("localhost:2015") would read "localhost" as the scheme and
// "2015" as an opaque path, so the scheme check is done on the "://" separator
// first.
func NormalizeURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, &ExitError{Code: ExitMalformedURL, Msg: "URL rejected: " + err.Error()}
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return nil, &ExitError{Code: ExitUnsupportedProtocol, Msg: "Protocol \"" + u.Scheme + "\" not supported"}
	}
	if u.Hostname() == "" {
		return nil, &ExitError{Code: ExitMalformedURL, Msg: "URL rejected: no host in " + raw}
	}
	return u, nil
}
