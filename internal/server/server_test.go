package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit, cfg.Burst = 1000, 1000 // out of the way unless a test sets it
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s)
	t.Cleanup(func() { ts.Close(); s.Close() })
	return ts
}

// The helpers fail the test on a transport error, so call sites can use the
// response straight away (and go vet's httpresponse check stays happy).
func get(t *testing.T, url string) *http.Response { return getWith(t, http.DefaultClient, url) }

func getWith(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func post(t *testing.T, url, contentType string, body io.Reader) *http.Response {
	t.Helper()
	resp, err := http.Post(url, contentType, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func send(t *testing.T, c *http.Client, req *http.Request) *http.Response {
	t.Helper()
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decoding JSON reply: %v", err)
	}
	return m
}

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(2, 3) // 2 tokens/second, burst of 3
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d inside the burst was refused", i+1)
		}
	}
	ok, wait := l.Allow("a")
	if ok {
		t.Fatal("4th request should be refused")
	}
	if wait < 400*time.Millisecond || wait > 600*time.Millisecond {
		t.Errorf("retry-after = %v, want about 500ms (1 token at 2/s)", wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Error("a different key has its own bucket")
	}

	now = now.Add(time.Second) // two tokens earned
	if ok, _ := l.Allow("a"); !ok {
		t.Error("tokens should have refilled")
	}
	if ok, _ := l.Allow("a"); !ok {
		t.Error("second refilled token should work")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("only two tokens were earned")
	}

	now = now.Add(time.Hour)
	l.Sweep(10 * time.Minute)
	if len(l.buckets) != 0 {
		t.Errorf("idle buckets not swept: %d left", len(l.buckets))
	}
}

func TestRateLimitEndToEnd(t *testing.T) {
	ts := newTestServer(t, Config{RateLimit: 0.001, Burst: 2})
	for i := 0; i < 2; i++ {
		resp := get(t, ts.URL+"/get")
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("request %d: %d", i+1, resp.StatusCode)
		}
	}
	resp := get(t, ts.URL+"/get")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Errorf("status=%d Retry-After=%q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	hz := get(t, ts.URL+"/healthz")
	hz.Body.Close()
	if hz.StatusCode != 200 {
		t.Errorf("/healthz was throttled: %d", hz.StatusCode)
	}
}

func TestEcho(t *testing.T) {
	ts := newTestServer(t, Config{})

	t.Run("GET reports query, headers and method", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/get?a=1&b=2&b=3", nil)
		req.Header.Set("X-Test", "yes")
		resp := send(t, http.DefaultClient, req)
		m := getJSON(t, resp)
		args := m["args"].(map[string]any)
		if args["a"] != "1" || len(args["b"].([]any)) != 2 {
			t.Errorf("args = %v", args)
		}
		if m["headers"].(map[string]any)["X-Test"] != "yes" || m["method"] != "GET" {
			t.Errorf("reply = %v", m)
		}
	})

	t.Run("wrong method is not served", func(t *testing.T) {
		resp := post(t, ts.URL+"/get", "text/plain", nil)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Error("POST /get should not succeed")
		}
	})

	t.Run("POST parses a form body", func(t *testing.T) {
		resp := post(t, ts.URL+"/post", "application/x-www-form-urlencoded", strings.NewReader("name=gurl&n=1"))
		m := getJSON(t, resp)
		form := m["form"].(map[string]any)
		if form["name"] != "gurl" || m["data"] != "name=gurl&n=1" {
			t.Errorf("reply = %v", m)
		}
	})

	t.Run("POST parses JSON", func(t *testing.T) {
		resp := post(t, ts.URL+"/post", "application/json", strings.NewReader(`{"k":[1,2]}`))
		m := getJSON(t, resp)
		if m["json"].(map[string]any)["k"].([]any)[1] != float64(2) {
			t.Errorf("json = %v", m["json"])
		}
	})

	t.Run("POST parses multipart with a file", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		mw.WriteField("field", "value")
		fw, _ := mw.CreateFormFile("upload", "a.txt")
		fw.Write([]byte("file contents"))
		mw.Close()
		resp := post(t, ts.URL+"/post", mw.FormDataContentType(), &body)
		m := getJSON(t, resp)
		if m["form"].(map[string]any)["field"] != "value" || m["files"].(map[string]any)["upload"] != "file contents" {
			t.Errorf("reply = %v", m)
		}
	})

	t.Run("/anything takes any method and path", func(t *testing.T) {
		req, _ := http.NewRequest("PATCH", ts.URL+"/anything/deep/path", nil)
		resp := send(t, http.DefaultClient, req)
		m := getJSON(t, resp)
		if m["method"] != "PATCH" || !strings.HasSuffix(m["url"].(string), "/anything/deep/path") {
			t.Errorf("reply = %v", m)
		}
	})

	t.Run("oversized body is 413", func(t *testing.T) {
		resp := post(t, ts.URL+"/post", "text/plain", strings.NewReader(strings.Repeat("x", maxBody+1)))
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
}

func TestClientIPTrust(t *testing.T) {
	loop := httptest.NewRequest("GET", "/", nil)
	loop.RemoteAddr = "127.0.0.1:5555"
	loop.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if got := clientIP(loop); got != "203.0.113.9" {
		t.Errorf("behind the proxy: %q", got)
	}

	spoof := httptest.NewRequest("GET", "/", nil)
	spoof.RemoteAddr = "198.51.100.7:4444"
	spoof.Header.Set("CF-Connecting-IP", "203.0.113.9")
	spoof.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(spoof); got != "198.51.100.7" {
		t.Errorf("a direct caller must not be able to spoof its address, got %q", got)
	}
}

func TestEndpoints(t *testing.T) {
	ts := newTestServer(t, Config{Version: "9.9.9"})
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	t.Run("status", func(t *testing.T) {
		for path, want := range map[string]int{"/status/200": 200, "/status/404": 404, "/status/418": 418, "/status/204": 204, "/status/503": 503} {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("%s = %d, want %d", path, resp.StatusCode, want)
			}
		}
		for _, bad := range []string{"/status/99", "/status/600", "/status/abc"} {
			resp := get(t, ts.URL+bad)
			resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Errorf("%s = %d, want 400", bad, resp.StatusCode)
			}
		}
	})

	t.Run("teapot has a body", func(t *testing.T) {
		resp := get(t, ts.URL+"/status/418")
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), "teapot") {
			t.Errorf("body = %q", b)
		}
	})

	t.Run("delay waits, validates and gives up when the client leaves", func(t *testing.T) {
		start := time.Now()
		resp := get(t, ts.URL+"/delay/1")
		resp.Body.Close()
		if d := time.Since(start); d < 900*time.Millisecond || resp.StatusCode != 200 {
			t.Errorf("took %v, status %d", d, resp.StatusCode)
		}
		resp = get(t, ts.URL+"/delay/11")
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("/delay/11 = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("bytes honours size and seed", func(t *testing.T) {
		fetch := func(path string) []byte {
			resp := get(t, ts.URL+path)
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return b
		}
		a, b := fetch("/bytes/100?seed=7"), fetch("/bytes/100?seed=7")
		if len(a) != 100 || !bytes.Equal(a, b) {
			t.Errorf("same seed should repeat: %d bytes, equal=%v", len(a), bytes.Equal(a, b))
		}
		if bytes.Equal(a, fetch("/bytes/100?seed=8")) {
			t.Error("different seeds gave identical bytes")
		}
		if bytes.Equal(fetch("/bytes/100"), fetch("/bytes/100")) {
			t.Error("unseeded requests should differ")
		}
		resp := get(t, ts.URL+"/bytes/"+"2000000")
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("oversize = %d", resp.StatusCode)
		}
	})

	t.Run("redirect chain ends at /get", func(t *testing.T) {
		resp := getWith(t, noFollow, ts.URL+"/redirect/3")
		resp.Body.Close()
		if resp.StatusCode != 302 || resp.Header.Get("Location") != "/redirect/2" {
			t.Errorf("status=%d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
		}
		resp = getWith(t, noFollow, ts.URL+"/redirect/1")
		resp.Body.Close()
		if resp.Header.Get("Location") != "/get" {
			t.Errorf("last hop Location=%q", resp.Header.Get("Location"))
		}
		resp = get(t, ts.URL+"/redirect/3")
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Request.URL.Path != "/get" {
			t.Errorf("following ended at %s with %d", resp.Request.URL.Path, resp.StatusCode)
		}
	})

	t.Run("basic auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/basic-auth/me/secret", nil)
		resp := send(t, http.DefaultClient, req)
		resp.Body.Close()
		if resp.StatusCode != 401 || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Basic") {
			t.Errorf("no credentials: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}
		req.SetBasicAuth("me", "wrong")
		resp = send(t, http.DefaultClient, req)
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("wrong password: %d", resp.StatusCode)
		}
		req.SetBasicAuth("me", "secret")
		resp = send(t, http.DefaultClient, req)
		m := getJSON(t, resp)
		if resp.StatusCode != 200 || m["authenticated"] != true {
			t.Errorf("right password: %d %v", resp.StatusCode, m)
		}
	})

	t.Run("gzip", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/gzip", nil)
		req.Header.Set("Accept-Encoding", "gzip") // setting it ourselves stops net/http from decoding for us
		resp := send(t, http.DefaultClient, req)
		defer resp.Body.Close()
		if resp.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("Content-Encoding = %q", resp.Header.Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.NewDecoder(zr).Decode(&m); err != nil || m["gzipped"] != true {
			t.Errorf("m=%v err=%v", m, err)
		}
	})

	t.Run("headers, ip, user-agent, json", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/user-agent", nil)
		req.Header.Set("User-Agent", "gurl-test")
		resp := send(t, http.DefaultClient, req)
		if m := getJSON(t, resp); m["user-agent"] != "gurl-test" {
			t.Errorf("user-agent = %v", m)
		}
		resp = get(t, ts.URL+"/ip")
		if m := getJSON(t, resp); m["origin"] != "127.0.0.1" {
			t.Errorf("ip = %v", m)
		}
		resp = get(t, ts.URL+"/headers")
		if m := getJSON(t, resp); m["headers"].(map[string]any)["Host"] == "" {
			t.Errorf("headers = %v", m)
		}
		resp = get(t, ts.URL+"/json")
		if m := getJSON(t, resp); m["project"] != "gurl" {
			t.Errorf("json = %v", m)
		}
	})
}

func TestSite(t *testing.T) {
	ts := newTestServer(t, Config{Version: "9.9.9"})

	resp := get(t, ts.URL+"/")
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{"<title>gurl", "9.9.9", "Usage: gurl [options] &lt;url&gt;", "install.sh"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("docs page missing %q", want)
		}
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Error("security headers missing")
	}

	resp = get(t, ts.URL+"/install.sh")
	script, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(script), "#!/bin/sh") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("install.sh: ct=%q starts=%q", resp.Header.Get("Content-Type"), script[:min(20, len(script))])
	}

	resp = get(t, ts.URL+"/static/style.css")
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Cache-Control"), "max-age") {
		t.Errorf("css: %d cache=%q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}

	resp = get(t, ts.URL+"/nope")
	m := getJSON(t, resp)
	if resp.StatusCode != 404 || !strings.Contains(m["error"].(string), "/nope") {
		t.Errorf("404: %d %v", resp.StatusCode, m)
	}
}

// syncBuffer lets the test read log output while the server goroutine is still writing it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestClientHangupIsLogged499(t *testing.T) {
	logs := &syncBuffer{}
	ts := newTestServer(t, Config{Logger: slog.New(slog.NewTextHandler(logs, nil))})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/delay/5", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("expected the client to time out")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(logs.String(), "status=499") {
		if time.Now().After(deadline) {
			t.Fatalf("no 499 logged; logs:\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
