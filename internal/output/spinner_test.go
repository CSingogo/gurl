package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// plain strips ANSI colour codes so tests can read the words.
func plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' && s[i] != 'l' && s[i] != 'h' && s[i] != 'K' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestVerbFor(t *testing.T) {
	tests := map[string]string{
		"GET": "Fetching", "": "Fetching", "post": "Posting", "PUT": "Putting",
		"PATCH": "Patching", "DELETE": "Deleting", "HEAD": "Checking", "PROPFIND": "Sending",
	}
	for method, want := range tests {
		if got := VerbFor(method); got != want {
			t.Errorf("VerbFor(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestFrameShowsVerbTargetAndElapsed(t *testing.T) {
	fast := plain(Frame(0, "Posting", "x.dev/post", 200*time.Millisecond))
	if !strings.Contains(fast, "Posting") || !strings.Contains(fast, "x.dev/post") || strings.Contains(fast, "·") {
		t.Errorf("fast frame = %q", fast)
	}
	slow := plain(Frame(3, "Posting", "x.dev/post", 2500*time.Millisecond))
	if !strings.Contains(slow, "· 2.5s") {
		t.Errorf("slow frame = %q, want elapsed time", slow)
	}
}

func TestSpinnerDelayAndCleanup(t *testing.T) {
	// A request that finishes before the delay must leave no trace at all.
	var quick bytes.Buffer
	stop := StartSpinner(&quick, "Fetching", "x.dev", time.Hour)
	stop()
	if quick.Len() != 0 {
		t.Errorf("quick request wrote %q", quick.String())
	}

	// A slow one draws, then erases its line and restores the cursor.
	var slow bytes.Buffer
	stop = StartSpinner(&slow, "Fetching", "x.dev", 0)
	time.Sleep(200 * time.Millisecond)
	stop()
	out := slow.String()
	if !strings.Contains(plain(out), "Fetching") {
		t.Errorf("no spinner drawn: %q", out)
	}
	if !strings.HasSuffix(out, "\r\x1b[2K\x1b[?25h") {
		t.Errorf("did not clean up: %q", out[max(0, len(out)-30):])
	}
	stop() // calling stop twice must be safe
}

func TestGlowColoursByVerb(t *testing.T) {
	// 196 is bright red, 208 is orange, 45 is cyan-blue; each appears in its own palette only.
	has := func(frames []string, code string) bool {
		for _, f := range frames {
			if strings.Contains(f, "38;5;"+code+"m") {
				return true
			}
		}
		return false
	}
	frames := func(verb string) []string {
		var out []string
		for i := 0; i < 24; i++ {
			out = append(out, Frame(i, verb, "x.dev", 0))
		}
		return out
	}
	tests := []struct {
		verb, want string
		notWant    []string
	}{
		{"Deleting", "196", []string{"208", "45"}},
		{"Posting", "208", []string{"196", "45"}},
		{"Putting", "208", []string{"196", "45"}},
		{"Fetching", "45", []string{"196", "208"}},
		{"Patching", "45", []string{"196", "208"}},
	}
	for _, tt := range tests {
		f := frames(tt.verb)
		if !has(f, tt.want) {
			t.Errorf("%s: missing colour %s", tt.verb, tt.want)
		}
		for _, n := range tt.notWant {
			if has(f, n) {
				t.Errorf("%s: unexpected colour %s", tt.verb, n)
			}
		}
	}
}
