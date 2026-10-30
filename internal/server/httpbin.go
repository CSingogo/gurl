package server

import (
	"bytes"
	"compress/gzip"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxBody     = 1 << 20 // request bodies and /bytes responses are capped at 1 MB
	maxDelay    = 10      // seconds
	maxRedirect = 20
)

// writeJSON replies with indented JSON: this server exists for people to read
// with their eyes at a terminal, so pretty beats compact.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// flatten turns url.Values into httpbin's shape: a string when a key has one
// value, a list when it has several.
func flatten(v url.Values) map[string]any {
	out := make(map[string]any, len(v))
	for k, vals := range v {
		if len(vals) == 1 {
			out[k] = vals[0]
		} else {
			out[k] = vals
		}
	}
	return out
}

// clientIP is the address of whoever is really calling.
//
// In production every request arrives from Caddy on loopback, and Cloudflare's
// edge puts the visitor's address in CF-Connecting-IP. That header is only
// believed when the TCP peer is loopback; otherwise anyone could spoof it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if v := r.Header.Get("CF-Connecting-IP"); v != "" {
			return v
		}
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			first, _, _ := strings.Cut(v, ",")
			return strings.TrimSpace(first)
		}
	}
	return host
}

func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if p := r.Header.Get("X-Forwarded-Proto"); p != "" && isLoopback(r) {
		scheme = p
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

func isLoopback(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// echoed is what /get, /post, /anything and friends reply with.
type echoed struct {
	Args    map[string]any    `json:"args"`
	Data    string            `json:"data"`
	Files   map[string]string `json:"files"`
	Form    map[string]any    `json:"form"`
	Headers map[string]string `json:"headers"`
	JSON    any               `json:"json"`
	Method  string            `json:"method"`
	Origin  string            `json:"origin"`
	URL     string            `json:"url"`
}

// echo describes the request back to the caller, parsing the body according to
// its Content-Type (form, multipart or JSON). It returns false if the reply has
// already been written because the body was too large.
func echo(w http.ResponseWriter, r *http.Request) (echoed, bool) {
	e := echoed{
		Args:    flatten(r.URL.Query()),
		Files:   map[string]string{},
		Form:    map[string]any{},
		Headers: map[string]string{"Host": r.Host},
		Method:  r.Method,
		Origin:  clientIP(r),
		URL:     requestURL(r),
	}
	for k, v := range r.Header {
		e.Headers[k] = strings.Join(v, ",")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body larger than %d bytes", maxBody))
		} else {
			writeError(w, http.StatusBadRequest, "could not read request body")
		}
		return e, false
	}
	e.Data = string(body)

	mt, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case mt == "application/x-www-form-urlencoded":
		if v, err := url.ParseQuery(string(body)); err == nil {
			e.Form = flatten(v)
		}
	case mt == "multipart/form-data" && params["boundary"] != "":
		if f, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(maxBody); err == nil {
			e.Form = flatten(url.Values(f.Value))
			for field, headers := range f.File {
				if len(headers) == 0 {
					continue
				}
				if fh, err := headers[0].Open(); err == nil {
					content, _ := io.ReadAll(fh)
					fh.Close()
					e.Files[field] = string(content)
				}
			}
			f.RemoveAll()
		}
	case mt == "application/json" || strings.HasSuffix(mt, "+json"):
		var v any
		if json.Unmarshal(body, &v) == nil {
			e.JSON = v
		}
	}
	return e, true
}

// handleEcho serves any endpoint whose whole job is to echo the request.
func handleEcho(w http.ResponseWriter, r *http.Request) {
	if e, ok := echo(w, r); ok {
		writeJSON(w, http.StatusOK, e)
	}
}

func handleHeaders(w http.ResponseWriter, r *http.Request) {
	h := map[string]string{"Host": r.Host}
	for k, v := range r.Header {
		h[k] = strings.Join(v, ",")
	}
	writeJSON(w, http.StatusOK, map[string]any{"headers": h})
}

func handleIP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"origin": clientIP(r)})
}

func handleUserAgent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"user-agent": r.UserAgent()})
}

const teapot = `    .------.
   (  ~~~~  )___
    '------'    )
   i'm a teapot
`

func handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 100 || code > 599 {
		writeError(w, http.StatusBadRequest, "status must be a number from 100 to 599")
		return
	}
	switch code {
	case http.StatusTeapot:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(code)
		io.WriteString(w, teapot)
	case http.StatusNoContent, http.StatusNotModified:
		w.WriteHeader(code) // these statuses must not have a body
	default:
		if code >= 100 && code < 200 {
			// 1xx are interim responses; net/http only lets handlers send them
			// as headers-only, so there is nothing more to say.
			w.WriteHeader(code)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(code)
		fmt.Fprintf(w, "%d %s\n", code, http.StatusText(code))
	}
}

// delayHandler answers after n seconds, but stops waiting the moment the client
// hangs up. The semaphore bounds how many sleepers can exist at once.
func delayHandler(slots chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil || n < 0 || n > maxDelay {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("delay must be a number from 0 to %d", maxDelay))
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "too many delayed requests in flight, try again")
			return
		}
		select {
		case <-time.After(time.Duration(n) * time.Second):
			handleEcho(w, r)
		case <-r.Context().Done():
		}
	}
}

func handleBytes(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 || n > maxBody {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("size must be a number from 0 to %d", maxBody))
		return
	}
	// ?seed=N makes the bytes repeatable, handy for checking downloads.
	var rng *rand.ChaCha8
	if s, err := strconv.ParseUint(r.URL.Query().Get("seed"), 10, 64); err == nil {
		var seed [32]byte
		for i := range 8 {
			seed[i] = byte(s >> (8 * i))
		}
		rng = rand.NewChaCha8(seed)
	} else {
		var seed [32]byte
		for i := range seed {
			seed[i] = byte(rand.Uint32())
		}
		rng = rand.NewChaCha8(seed)
	}
	buf := make([]byte, n)
	rng.Read(buf)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(n))
	w.Write(buf)
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 || n > maxRedirect {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("redirect count must be a number from 1 to %d", maxRedirect))
		return
	}
	// Relative Location headers are valid and keep this correct behind any proxy.
	target := "/get"
	if n > 1 {
		target = "/redirect/" + strconv.Itoa(n-1)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func handleBasicAuth(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	// ConstantTimeCompare takes the same time whether the first or last byte
	// differs, so response timing leaks nothing about the expected password.
	good := ok &&
		subtle.ConstantTimeCompare([]byte(user), []byte(r.PathValue("user"))) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(r.PathValue("pass"))) == 1
	if !good {
		w.Header().Set("WWW-Authenticate", `Basic realm="gurl test server"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": user})
}

func handleJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"project":  "gurl",
		"tagline":  "a curl-inspired HTTP client in Go",
		"features": []string{"curl flags", "pretty output on a terminal", "websockets"},
		"nested":   map[string]any{"number": 42, "float": 3.14, "bool": true, "nothing": nil},
	})
}

func handleGzip(w http.ResponseWriter, r *http.Request) {
	e, ok := echo(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	enc := json.NewEncoder(zw)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"gzipped": true, "method": e.Method, "origin": e.Origin, "headers": e.Headers})
	zw.Close()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}
