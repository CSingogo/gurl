package cli

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Options
	}{
		{
			name: "bare url gets curl defaults",
			args: []string{"localhost:2015"},
			want: Options{URL: "localhost:2015", MaxRedirs: 50},
		},
		{
			name: "combined short switches",
			args: []string{"-sSL", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Silent: true, ShowError: true, Location: true},
		},
		{
			name: "short flag with separate value",
			args: []string{"-X", "post", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Method: "POST"},
		},
		{
			name: "short flag with attached value",
			args: []string{"-XPUT", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Method: "PUT"},
		},
		{
			name: "switch then valued flag in one word",
			args: []string{"-vo", "out.txt", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Verbose: true, Output: "out.txt"},
		},
		{
			name: "repeated headers keep order",
			args: []string{"-H", "A: 1", "--header", "B: 2", "-H=ignored-equals-is-part-of-value", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Headers: []string{"A: 1", "B: 2", "=ignored-equals-is-part-of-value"}},
		},
		{
			name: "long flag with equals",
			args: []string{"--max-time=2.5", "--user-agent=gurl/0", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, MaxTime: 2500 * time.Millisecond, UserAgent: "gurl/0"},
		},
		{
			name: "flags after the url",
			args: []string{"example.com", "-i", "--location"},
			want: Options{URL: "example.com", MaxRedirs: 50, Include: true, Location: true},
		},
		{
			name: "data kinds",
			args: []string{"-d", "a=1", "--data-raw", "@x", "--data-binary", "b", "--json", `{"k":1}`, "example.com"},
			want: Options{URL: "example.com", MaxRedirs: 50, Data: []DataItem{
				{DataText, "a=1"}, {DataRaw, "@x"}, {DataBinary, "b"}, {DataJSON, `{"k":1}`},
			}},
		},
		{
			name: "double dash ends flags",
			args: []string{"--", "-weird-host"},
			want: Options{URL: "-weird-host", MaxRedirs: 50},
		},
		{
			name: "lone dash is a url, not a flag",
			args: []string{"-"},
			want: Options{URL: "-", MaxRedirs: 50},
		},
		{
			name: "unlimited redirects",
			args: []string{"-L", "--max-redirs", "-1", "example.com"},
			want: Options{URL: "example.com", MaxRedirs: -1, Location: true},
		},
		{
			name: "help needs no url",
			args: []string{"--help"},
			want: Options{MaxRedirs: 50, Help: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.args, err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Parse(%q)\n got: %+v\nwant: %+v", tt.args, *got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no url", nil},
		{"two urls", []string{"a.com", "b.com"}},
		{"unknown long flag", []string{"--nope", "a.com"}},
		{"unknown short flag", []string{"-Z", "a.com"}},
		{"missing long value", []string{"a.com", "--header"}},
		{"missing short value", []string{"a.com", "-X"}},
		{"switch given a value", []string{"--silent=yes", "a.com"}},
		{"bad number", []string{"--max-time", "soon", "a.com"}},
		{"negative time", []string{"--max-time", "-3", "a.com"}},
		{"bad max-redirs", []string{"--max-redirs", "x", "a.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			var ee *ExitError
			if !errors.As(err, &ee) {
				t.Fatalf("Parse(%q) error = %v, want *ExitError", tt.args, err)
			}
			if ee.Code != ExitBadUsage {
				t.Errorf("exit code = %d, want %d", ee.Code, ExitBadUsage)
			}
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in       string
		want     string
		wantCode int // 0 means success
	}{
		{"localhost:2015", "http://localhost:2015", 0},
		{"example.com/path?q=1", "http://example.com/path?q=1", 0},
		{"https://example.com", "https://example.com", 0},
		{"ws://example.com/chat", "ws://example.com/chat", 0},
		{"[::1]:8080", "http://[::1]:8080", 0},
		{"ftp://example.com", "", ExitUnsupportedProtocol},
		{"http://", "", ExitMalformedURL},
		{"http://exa mple.com", "", ExitMalformedURL},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			u, err := NormalizeURL(tt.in)
			if tt.wantCode != 0 {
				var ee *ExitError
				if !errors.As(err, &ee) || ee.Code != tt.wantCode {
					t.Fatalf("NormalizeURL(%q) error = %v, want exit code %d", tt.in, err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeURL(%q) error: %v", tt.in, err)
			}
			if u.String() != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, u.String(), tt.want)
			}
		})
	}
}
