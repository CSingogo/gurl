package output

import (
	"net/http"
	"strings"
	"testing"
)

func TestIsJSON(t *testing.T) {
	tests := map[string]bool{
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"application/vnd.api+json":        true,
		"text/html":                       false,
		"application/jsonp":               false,
		"":                                false,
	}
	for in, want := range tests {
		if got := IsJSON(in); got != want {
			t.Errorf("IsJSON(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	in := []byte(`{"a":1,"b":[true,null,"x\"y"],"c":{"d":-2.5e3}}`)

	t.Run("plain indent", func(t *testing.T) {
		got, ok := PrettyJSON(in, Style{})
		if !ok {
			t.Fatal("not ok")
		}
		want := "{\n  \"a\": 1,\n  \"b\": [\n    true,\n    null,\n    \"x\\\"y\"\n  ],\n  \"c\": {\n    \"d\": -2.5e3\n  }\n}\n"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("color keeps the text and adds codes", func(t *testing.T) {
		got, _ := PrettyJSON(in, Style{Color: true})
		plain, _ := PrettyJSON(in, Style{})
		stripped := stripANSI(string(got))
		if stripped != string(plain) {
			t.Errorf("stripping color changed the text:\n%s", stripped)
		}
		if !strings.Contains(string(got), blue+`"a"`+reset) {
			t.Error("object key not colored as a key")
		}
		if !strings.Contains(string(got), green+`"x\"y"`+reset) {
			t.Error("string with escaped quote not colored as one token")
		}
		if !strings.Contains(string(got), yellow+"-2.5e3"+reset) || !strings.Contains(string(got), magenta+"null"+reset) {
			t.Error("number or null not colored")
		}
	})

	t.Run("invalid JSON is declined", func(t *testing.T) {
		if _, ok := PrettyJSON([]byte(`{"a":`), Style{}); ok {
			t.Error("ok for invalid JSON")
		}
	})
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestExpand(t *testing.T) {
	vars := map[string]string{"http_code": "200", "size_download": "12"}
	lookup := func(n string) (string, bool) { v, ok := vars[n]; return v, ok }
	tests := []struct{ in, want string }{
		{`%{http_code}`, "200"},
		{`code=%{http_code} size=%{size_download}\n`, "code=200 size=12\n"},
		{`100%% sure`, "100% sure"},
		{`%{nope}|`, "|"},
		{`a\tb\\c`, "a\tb\\c"},
		{`unterminated %{http_code`, "unterminated %{http_code"},
		{`\q`, `\q`},
	}
	for _, tt := range tests {
		if got := Expand(tt.in, lookup); got != tt.want {
			t.Errorf("Expand(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStatusLine(t *testing.T) {
	h1 := &http.Response{ProtoMajor: 1, ProtoMinor: 1, Status: "404 Not Found", StatusCode: 404}
	h2 := &http.Response{ProtoMajor: 2, StatusCode: 200, Status: "200 OK"}
	if got := StatusLine(h1); got != "HTTP/1.1 404 Not Found" {
		t.Errorf("h1: %q", got)
	}
	if got := StatusLine(h2); got != "HTTP/2 200" {
		t.Errorf("h2: %q", got)
	}
}

func TestFileNameFromURL(t *testing.T) {
	tests := map[string]string{"/a/b/file.zip": "file.zip", "/file": "file", "/dir/": "dir", "/": "", "": ""}
	for in, want := range tests {
		if got := FileNameFromURL(in); got != want {
			t.Errorf("FileNameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
