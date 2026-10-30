// Package request turns parsed Options into an HTTP request and runs it with
// the redirect, timeout and TLS behaviour curl users expect.
package request

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/CSingogo/gurl/internal/cli"
)

// Version is reported in the default User-Agent. Release builds override it
// with: go build -ldflags "-X github.com/CSingogo/gurl/internal/request.Version=1.2.3"
var Version = "dev"

// Build creates the *http.Request described by o. stdin is read only for "-d @-".
func Build(o *cli.Options, stdin io.Reader) (*http.Request, error) {
	u, err := cli.NormalizeURL(o.URL)
	if err != nil {
		return nil, err
	}

	body, err := buildBody(o.Data, stdin)
	if err != nil {
		return nil, err
	}

	// -G moves the data from the body into the query string.
	if o.GetMode && body != nil {
		if u.RawQuery != "" {
			u.RawQuery += "&"
		}
		u.RawQuery += string(body)
		body = nil
	}

	method := o.Method
	switch {
	case method != "":
	case o.Head:
		method = http.MethodHead
	case body != nil:
		method = http.MethodPost
	default:
		method = http.MethodGet
	}

	// http.NewRequest sets GetBody for *bytes.Reader, which lets the client
	// replay the body on 307/308 redirects.
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u.String(), reader)
	if err != nil {
		return nil, &cli.ExitError{Code: cli.ExitMalformedURL, Msg: "URL rejected: " + err.Error()}
	}

	applyDefaults(req, o, body != nil)
	if err := applyHeaders(req, o.Headers); err != nil {
		return nil, err
	}
	if o.User != "" {
		user, pass, _ := strings.Cut(o.User, ":")
		req.SetBasicAuth(user, pass)
	}
	return req, nil
}

// buildBody joins every -d style argument with "&", as curl does.
func buildBody(items []cli.DataItem, stdin io.Reader) ([]byte, error) {
	if len(items) == 0 {
		return nil, nil
	}
	parts := make([][]byte, 0, len(items))
	for _, it := range items {
		part, err := dataBytes(it, stdin)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return bytes.Join(parts, []byte("&")), nil
}

func dataBytes(it cli.DataItem, stdin io.Reader) ([]byte, error) {
	if it.Kind == cli.DataRaw || !strings.HasPrefix(it.Value, "@") {
		return []byte(it.Value), nil
	}
	name := it.Value[1:]
	var b []byte
	var err error
	if name == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(name)
	}
	if err != nil {
		return nil, &cli.ExitError{Code: cli.ExitReadError, Msg: "Failed to read data from " + name + ": " + err.Error()}
	}
	// Plain -d strips line breaks from files (so multi-line files become one
	// form value); --data-binary and --json keep every byte.
	if it.Kind == cli.DataText {
		b = []byte(strings.NewReplacer("\r", "", "\n", "").Replace(string(b)))
	}
	return b, nil
}

func applyDefaults(req *http.Request, o *cli.Options, hasBody bool) {
	ua := o.UserAgent
	if ua == "" {
		ua = "gurl/" + Version
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	if o.Referer != "" {
		req.Header.Set("Referer", o.Referer)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, d := range o.Data {
		if d.Kind == cli.DataJSON {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
		}
	}
}

// applyHeaders handles -H the way curl does:
//
//	"Name: value"  set the header (replacing a default, adding to earlier -H of the same name)
//	"Name:"        remove the header, including defaults
//	"Name;"        send the header with an empty value
func applyHeaders(req *http.Request, headers []string) error {
	replaced := map[string]bool{}
	for _, h := range headers {
		name, value, found := strings.Cut(h, ":")
		if !found {
			// "Name;" form.
			if n, ok := strings.CutSuffix(h, ";"); ok && n != "" {
				name, value = n, ""
			} else {
				return &cli.ExitError{Code: cli.ExitBadUsage, Msg: "invalid header (want \"Name: value\"): " + h}
			}
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		canon := http.CanonicalHeaderKey(name)
		if canon == "Host" {
			req.Host = value
			continue
		}
		if found && value == "" {
			req.Header.Del(canon)
			replaced[canon] = true
			continue
		}
		if !replaced[canon] {
			req.Header.Del(canon) // first -H for this name replaces our default
			replaced[canon] = true
		}
		req.Header.Add(canon, value)
	}
	return nil
}
