package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run drives the whole CLI against args and returns stdout, stderr and the exit code.
func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") })
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"a":1}`)
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%s %s %s", r.Method, r.Header.Get("Content-Type"), b)
	})
	mux.HandleFunc("/status/{code}", func(w http.ResponseWriter, r *http.Request) {
		var c int
		fmt.Sscan(r.PathValue("code"), &c)
		w.WriteHeader(c)
		fmt.Fprint(w, "body-of-error")
	})
	mux.HandleFunc("/to-hello", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hello", http.StatusMovedPermanently)
	})
	return httptest.NewServer(mux)
}

func TestRun(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	dir := t.TempDir()

	t.Run("body to stdout, JSON stays raw when piped", func(t *testing.T) {
		out, _, code := run(t, "", srv.URL+"/json")
		if code != 0 || out != `{"a":1}` {
			t.Errorf("code=%d out=%q", code, out)
		}
	})

	t.Run("bare host:port gets http://", func(t *testing.T) {
		out, _, code := run(t, "", strings.TrimPrefix(srv.URL, "http://")+"/hello")
		if code != 0 || out != "hello" {
			t.Errorf("code=%d out=%q", code, out)
		}
	})

	t.Run("-i prints headers with CRLF before the body", func(t *testing.T) {
		out, _, _ := run(t, "", "-i", srv.URL+"/hello")
		if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") || !strings.HasSuffix(out, "\r\n\r\nhello") {
			t.Errorf("out = %q", out)
		}
	})

	t.Run("-I sends HEAD and prints only headers", func(t *testing.T) {
		out, _, code := run(t, "", "-I", srv.URL+"/hello")
		if code != 0 || !strings.HasPrefix(out, "HTTP/1.1 200 OK") || strings.Contains(out, "hello") {
			t.Errorf("code=%d out=%q", code, out)
		}
	})

	t.Run("-d with stdin and --json", func(t *testing.T) {
		out, _, _ := run(t, "from-stdin", "-d", "@-", srv.URL+"/echo")
		if out != "POST application/x-www-form-urlencoded from-stdin" {
			t.Errorf("out = %q", out)
		}
		out, _, _ = run(t, "", "--json", `{"x":1}`, srv.URL+"/echo")
		if out != `POST application/json {"x":1}` {
			t.Errorf("out = %q", out)
		}
	})

	t.Run("-L follows a redirect", func(t *testing.T) {
		out, _, _ := run(t, "", "-sL", srv.URL+"/to-hello")
		if out != "hello" {
			t.Errorf("out = %q", out)
		}
		out, _, _ = run(t, "", "-s", srv.URL+"/to-hello")
		if !strings.Contains(out, "Moved Permanently") {
			t.Errorf("without -L the redirect page should print, got %q", out)
		}
	})

	t.Run("-f turns an HTTP error into exit 22 and hides the body", func(t *testing.T) {
		out, errOut, code := run(t, "", "-f", srv.URL+"/status/404")
		if code != 22 || out != "" || !strings.Contains(errOut, "(22)") || !strings.Contains(errOut, "404") {
			t.Errorf("code=%d out=%q err=%q", code, out, errOut)
		}
	})

	t.Run("without -f an HTTP error still prints and exits 0", func(t *testing.T) {
		out, _, code := run(t, "", srv.URL+"/status/404")
		if code != 0 || out != "body-of-error" {
			t.Errorf("code=%d out=%q", code, out)
		}
	})

	t.Run("-s hides errors, -sS shows them", func(t *testing.T) {
		_, errOut, code := run(t, "", "-s", "http://127.0.0.1:1")
		if code != 7 || errOut != "" {
			t.Errorf("-s: code=%d err=%q", code, errOut)
		}
		_, errOut, code = run(t, "", "-sS", "http://127.0.0.1:1")
		if code != 7 || !strings.Contains(errOut, "(7)") {
			t.Errorf("-sS: code=%d err=%q", code, errOut)
		}
	})

	t.Run("-o writes a file and prints nothing", func(t *testing.T) {
		path := filepath.Join(dir, "out.txt")
		out, _, code := run(t, "", "-o", path, srv.URL+"/hello")
		b, _ := os.ReadFile(path)
		if code != 0 || out != "" || string(b) != "hello" {
			t.Errorf("code=%d out=%q file=%q", code, out, b)
		}
	})

	t.Run("-O names the file after the URL", func(t *testing.T) {
		cwd, _ := os.Getwd()
		defer os.Chdir(cwd)
		os.Chdir(dir)
		_, _, code := run(t, "", "-O", srv.URL+"/hello")
		b, _ := os.ReadFile(filepath.Join(dir, "hello"))
		if code != 0 || string(b) != "hello" {
			t.Errorf("code=%d file=%q", code, b)
		}
		_, errOut, code := run(t, "", "-O", srv.URL+"/")
		if code != 23 || !strings.Contains(errOut, "no length") {
			t.Errorf("code=%d err=%q", code, errOut)
		}
	})

	t.Run("-w prints after the body", func(t *testing.T) {
		out, _, _ := run(t, "", "-w", `[%{http_code} %{size_download} %{method}]\n`, srv.URL+"/hello")
		if out != "hello[200 5 GET]\n" {
			t.Errorf("out = %q", out)
		}
		out, _, _ = run(t, "", "-s", "-o", os.DevNull, "-w", "%{http_code}", srv.URL+"/status/418")
		if out != "418" {
			t.Errorf("out = %q", out)
		}
	})

	t.Run("-v writes the exchange to stderr, body stays on stdout", func(t *testing.T) {
		out, errOut, _ := run(t, "", "-v", srv.URL+"/hello")
		if out != "hello" {
			t.Errorf("stdout = %q", out)
		}
		for _, want := range []string{"* Connected to", "> User-Agent: gurl/", "< HTTP/1.1 200 OK", "< Content-Length: 5"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr missing %q:\n%s", want, errOut)
			}
		}
	})
}

func TestRunMetaAndUsage(t *testing.T) {
	out, _, code := run(t, "", "--version")
	if code != 0 || !strings.HasPrefix(out, "gurl ") {
		t.Errorf("version: code=%d out=%q", code, out)
	}
	out, _, code = run(t, "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: gurl") {
		t.Errorf("help: code=%d", code)
	}
	_, errOut, code := run(t, "")
	if code != 2 || !strings.Contains(errOut, "no URL specified") || !strings.Contains(errOut, "--help") {
		t.Errorf("no url: code=%d err=%q", code, errOut)
	}
	_, errOut, code = run(t, "", "ftp://example.com")
	if code != 1 || !strings.Contains(errOut, "not supported") {
		t.Errorf("ftp: code=%d err=%q", code, errOut)
	}
	_, errOut, code = run(t, "", "ws://example.com")
	if code != 1 || !strings.Contains(errOut, "v0.4") {
		t.Errorf("ws: code=%d err=%q", code, errOut)
	}
}
