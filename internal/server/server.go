// Package server is gurl-server: the docs site plus an httpbin-style set of
// endpoints for trying gurl against. It uses only the standard library.
package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/CSingogo/gurl/internal/cli"
	"github.com/CSingogo/gurl/site"
)

// Config tunes a Server. Zero values get sensible defaults.
type Config struct {
	Logger    *slog.Logger
	RateLimit float64 // requests per second per client IP (default 10)
	Burst     int     // short-term burst allowance (default 30)
	Version   string  // shown on the docs page
}

// Server is an http.Handler. Call Close to stop its background cleanup.
type Server struct {
	handler http.Handler
	limiter *limiter
	stop    chan struct{}
	log     *slog.Logger
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// Close stops the limiter's sweeper goroutine.
func (s *Server) Close() { close(s.stop) }

// New wires the routes and middleware together.
func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10
	}
	if cfg.Burst == 0 {
		cfg.Burst = 30
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	page, err := renderDocs(cfg.Version)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(site.FS, "static")
	if err != nil {
		return nil, err
	}
	installScript, err := site.FS.ReadFile("install.sh")
	if err != nil {
		return nil, err
	}

	s := &Server{
		limiter: newLimiter(cfg.RateLimit, cfg.Burst),
		stop:    make(chan struct{}),
		log:     cfg.Logger,
	}
	go s.limiter.Run(time.Minute, 10*time.Minute, s.stop)

	// Go 1.22+ patterns: "GET /get" matches only GET (other methods get 405 for
	// free), and {name} segments are read with r.PathValue.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("GET /install.sh", func(w http.ResponseWriter, r *http.Request) {
		// text/plain so a browser shows the script instead of downloading it.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(installScript)
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(time.Hour, http.FileServerFS(static))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })

	mux.HandleFunc("GET /get", handleEcho)
	mux.HandleFunc("POST /post", handleEcho)
	mux.HandleFunc("PUT /put", handleEcho)
	mux.HandleFunc("PATCH /patch", handleEcho)
	mux.HandleFunc("DELETE /delete", handleEcho)
	mux.HandleFunc("/anything", handleEcho) // no method in the pattern: any method
	mux.HandleFunc("/anything/{path...}", handleEcho)
	mux.HandleFunc("GET /headers", handleHeaders)
	mux.HandleFunc("GET /ip", handleIP)
	mux.HandleFunc("GET /user-agent", handleUserAgent)
	mux.HandleFunc("/status/{code}", handleStatus)
	mux.HandleFunc("GET /delay/{n}", delayHandler(make(chan struct{}, 64)))
	mux.HandleFunc("GET /bytes/{n}", handleBytes)
	mux.HandleFunc("GET /redirect/{n}", handleRedirect)
	mux.HandleFunc("GET /basic-auth/{user}/{pass}", handleBasicAuth)
	mux.HandleFunc("GET /json", handleJSON)
	mux.HandleFunc("GET /gzip", handleGzip)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint: "+r.URL.Path)
	})

	s.handler = s.recoverer(s.logging(s.rateLimited(limitBody(securityHeaders(mux)))))
	return s, nil
}

func renderDocs(version string) ([]byte, error) {
	t, err := template.ParseFS(site.FS, "templates/index.html")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, map[string]string{"Help": cli.HelpText, "Version": version})
	return buf.Bytes(), err
}

func cacheFor(d time.Duration, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(d.Seconds())))
		h.ServeHTTP(w, r)
	})
}

// Middleware below follows one shape: take a handler, return a handler that
// does something before and/or after calling it.

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimited(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" { // monitoring must never be throttled
			if ok, wait := s.limiter.Allow(clientIP(r)); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status code so the logger can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int // 0 until the handler writes something
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

// Write is needed too: a handler that calls Write without WriteHeader sends an
// implicit 200 that would otherwise go unrecorded.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer underneath, which
// the WebSocket upgrade will need in order to hijack the connection.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		if rec.status == 0 {
			// The handler returned without answering: the client hung up first.
			// 499 is nginx's "client closed request"; Go has no constant for it.
			rec.status = 499
		}
		s.log.Info("request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"ms", time.Since(start).Milliseconds(), "ip", clientIP(r), "ua", r.UserAgent())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "value", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
