package request

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CSingogo/gurl/internal/cli"
)

func parse(t *testing.T, args ...string) *cli.Options {
	t.Helper()
	o, err := cli.Parse(args)
	if err != nil {
		t.Fatalf("Parse(%q): %v", args, err)
	}
	return o
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "body.txt")
	if err := os.WriteFile(file, []byte("line1\nline2\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantMethod string
		wantURL    string
		wantBody   string
		wantHeader map[string]string // "" means must be absent
	}{
		{
			name: "plain get", args: []string{"localhost:2015"},
			wantMethod: "GET", wantURL: "http://localhost:2015",
			wantHeader: map[string]string{"Accept": "*/*", "User-Agent": "gurl/dev", "Content-Type": ""},
		},
		{
			name: "data implies POST and form content type", args: []string{"-d", "a=1", "-d", "b=2", "x.test"},
			wantMethod: "POST", wantURL: "http://x.test", wantBody: "a=1&b=2",
			wantHeader: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		},
		{
			name: "explicit method wins", args: []string{"-X", "PUT", "-d", "a", "x.test"},
			wantMethod: "PUT", wantURL: "http://x.test", wantBody: "a",
		},
		{
			name: "-I means HEAD", args: []string{"-I", "x.test"},
			wantMethod: "HEAD", wantURL: "http://x.test",
		},
		{
			name: "-G moves data to the query", args: []string{"-G", "-d", "a=1", "-d", "b=2", "x.test/p?z=0"},
			wantMethod: "GET", wantURL: "http://x.test/p?z=0&a=1&b=2",
		},
		{
			name: "json sets both headers", args: []string{"--json", `{"a":1}`, "x.test"},
			wantMethod: "POST", wantURL: "http://x.test", wantBody: `{"a":1}`,
			wantHeader: map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		},
		{
			name: "-d @file strips newlines", args: []string{"-d", "@" + file, "x.test"},
			wantMethod: "POST", wantURL: "http://x.test", wantBody: "line1line2",
		},
		{
			name: "--data-binary @file keeps bytes", args: []string{"--data-binary", "@" + file, "x.test"},
			wantMethod: "POST", wantURL: "http://x.test", wantBody: "line1\nline2\r\n",
		},
		{
			name: "--data-raw keeps the @", args: []string{"--data-raw", "@notafile", "x.test"},
			wantMethod: "POST", wantURL: "http://x.test", wantBody: "@notafile",
		},
		{
			name: "-d @- reads stdin", args: []string{"-d", "@-", "x.test"}, stdin: "from-stdin",
			wantMethod: "POST", wantURL: "http://x.test", wantBody: "from-stdin",
		},
		{
			name:       "-H overrides defaults, removes, and empties",
			args:       []string{"-H", "User-Agent: custom", "-H", "Accept:", "-H", "X-Empty;", "-H", "X-Two: 1", "-H", "X-Two: 2", "x.test"},
			wantMethod: "GET", wantURL: "http://x.test",
			wantHeader: map[string]string{"User-Agent": "custom", "Accept": "", "X-Empty": ""},
		},
		{
			name: "basic auth and referer", args: []string{"-u", "me:pw", "-e", "http://from.test", "x.test"},
			wantMethod: "GET", wantURL: "http://x.test",
			wantHeader: map[string]string{"Authorization": "Basic bWU6cHc=", "Referer": "http://from.test"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := Build(parse(t, tt.args...), strings.NewReader(tt.stdin))
			if err != nil {
				t.Fatal(err)
			}
			if req.Method != tt.wantMethod {
				t.Errorf("method = %s, want %s", req.Method, tt.wantMethod)
			}
			if req.URL.String() != tt.wantURL {
				t.Errorf("url = %s, want %s", req.URL, tt.wantURL)
			}
			var gotBody string
			if req.Body != nil {
				b, _ := io.ReadAll(req.Body)
				gotBody = string(b)
			}
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
			for k, want := range tt.wantHeader {
				got, present := req.Header[k]
				switch {
				case want == "" && k != "X-Empty" && present:
					t.Errorf("header %s present (%q), want absent", k, got)
				case k == "X-Empty":
					if !present || len(got) != 1 || got[0] != "" {
						t.Errorf("X-Empty = %q (present=%v), want one empty value", got, present)
					}
				case want != "" && req.Header.Get(k) != want:
					t.Errorf("header %s = %q, want %q", k, req.Header.Get(k), want)
				}
			}
		})
	}

	t.Run("repeated -H keeps both values", func(t *testing.T) {
		req, _ := Build(parse(t, "-H", "X-Two: 1", "-H", "X-Two: 2", "x.test"), nil)
		if got := req.Header.Values("X-Two"); len(got) != 2 || got[0] != "1" || got[1] != "2" {
			t.Errorf("X-Two = %q", got)
		}
	})

	t.Run("Host header sets req.Host", func(t *testing.T) {
		req, _ := Build(parse(t, "-H", "Host: other.test", "x.test"), nil)
		if req.Host != "other.test" {
			t.Errorf("Host = %q", req.Host)
		}
	})

	t.Run("missing data file is exit 26", func(t *testing.T) {
		_, err := Build(parse(t, "-d", "@/no/such/file", "x.test"), nil)
		var ee *cli.ExitError
		if !errors.As(err, &ee) || ee.Code != cli.ExitReadError {
			t.Fatalf("err = %v, want exit 26", err)
		}
	})

	t.Run("bad header is a usage error", func(t *testing.T) {
		_, err := Build(parse(t, "-H", "nocolon", "x.test"), nil)
		var ee *cli.ExitError
		if !errors.As(err, &ee) || ee.Code != cli.ExitBadUsage {
			t.Fatalf("err = %v, want exit 2", err)
		}
	})
}

func do(t *testing.T, args ...string) (*Result, string, error) {
	t.Helper()
	res, err := Do(context.Background(), parse(t, args...), nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Resp.Body.Close()
	b, _ := io.ReadAll(res.Resp.Body)
	res.Done()
	return res, string(b), nil
}

func exitCode(err error) int {
	var ee *cli.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return -1
}

func TestDoRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hop/{n}", func(w http.ResponseWriter, r *http.Request) {
		var n int
		fmt.Sscan(r.PathValue("n"), &n)
		if n == 0 {
			fmt.Fprintf(w, "%s %q referer=%q", r.Method, mustBody(r), r.Header.Get("Referer"))
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
	})
	mux.HandleFunc("/keep", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop/0", http.StatusTemporaryRedirect)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Run("without -L the 3xx is returned as is", func(t *testing.T) {
		res, _, err := do(t, srv.URL+"/hop/2")
		if err != nil {
			t.Fatal(err)
		}
		if res.Resp.StatusCode != 302 || res.Resp.Header.Get("Location") != "/hop/1" {
			t.Errorf("got %d Location=%q", res.Resp.StatusCode, res.Resp.Header.Get("Location"))
		}
	})

	t.Run("-L follows and counts", func(t *testing.T) {
		res, body, err := do(t, "-L", srv.URL+"/hop/3")
		if err != nil {
			t.Fatal(err)
		}
		if res.Redirects != 3 || !strings.HasSuffix(res.EffectiveURL, "/hop/0") || !strings.HasPrefix(body, "GET") {
			t.Errorf("redirects=%d url=%s body=%q", res.Redirects, res.EffectiveURL, body)
		}
		if strings.Contains(body, `referer="http`) {
			t.Errorf("net/http leaked a Referer curl would not send: %s", body)
		}
	})

	t.Run("302 turns POST into GET and drops the body", func(t *testing.T) {
		_, body, err := do(t, "-L", "-d", "x=1", srv.URL+"/hop/1")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(body, `GET ""`) {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("307 keeps method and body", func(t *testing.T) {
		_, body, err := do(t, "-L", "-d", "x=1", srv.URL+"/keep")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(body, `POST "x=1"`) {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("--max-redirs limit is exit 47", func(t *testing.T) {
		_, _, err := do(t, "-L", "--max-redirs", "2", srv.URL+"/hop/5")
		if exitCode(err) != cli.ExitTooManyRedirects {
			t.Fatalf("err = %v, want exit 47", err)
		}
	})

	t.Run("--max-redirs 2 allows exactly 2", func(t *testing.T) {
		if _, _, err := do(t, "-L", "--max-redirs", "2", srv.URL+"/hop/2"); err != nil {
			t.Fatal(err)
		}
	})
}

func mustBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func TestDoErrors(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "secure") }))
	defer tlsSrv.Close()

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"connection refused is 7", []string{closedURL}, cli.ExitCouldntConnect},
		{"timeout is 28", []string{"-m", "0.05", slow.URL}, cli.ExitTimeout},
		{"unknown host is 6", []string{"-m", "5", "http://no-such-host.invalid"}, cli.ExitCouldntResolveHost},
		{"untrusted certificate is 60", []string{tlsSrv.URL}, cli.ExitSSL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := do(t, tt.args...)
			if exitCode(err) != tt.want {
				t.Fatalf("err = %v (code %d), want %d", err, exitCode(err), tt.want)
			}
		})
	}

	t.Run("-k accepts the untrusted certificate", func(t *testing.T) {
		_, body, err := do(t, "-k", tlsSrv.URL)
		if err != nil || body != "secure" {
			t.Fatalf("body=%q err=%v", body, err)
		}
	})
}

func TestDoDoesNotDecompress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ae := r.Header.Get("Accept-Encoding"); ae != "" {
			t.Errorf("sent Accept-Encoding %q; curl sends none without --compressed", ae)
		}
		fmt.Fprint(w, "plain")
	}))
	defer srv.Close()
	if _, body, err := do(t, srv.URL); err != nil || body != "plain" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestTimingsAreOrdered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "x") }))
	defer srv.Close()
	res, _, err := do(t, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tm := res.Timings
	if !(tm.Connect > 0 && tm.StartTransfer >= tm.Connect && tm.Total >= tm.StartTransfer) {
		t.Errorf("timings out of order: %+v", tm)
	}
	if res.RemoteIP != "127.0.0.1" {
		t.Errorf("RemoteIP = %q", res.RemoteIP)
	}
}
