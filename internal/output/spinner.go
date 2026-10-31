package output

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Spinner is the "something is happening" line gurl shows on stderr while it waits
// for a response. It lives on stderr, not stdout, so it never ends up in a pipe or a
// file, and it erases itself before the real output starts.
//
// Go concept: a goroutine is a lightweight thread. This one redraws the line every
// 80ms until stop() closes the `done` channel; closing a channel wakes every reader
// at once, which makes it the idiomatic "please stop" signal.

// Each glow is a palette from deep to bright and back, as xterm-256 colour numbers.
// Moving a palette along the text one step per frame is what makes the word shimmer.
// Blue is the default; the verbs that change or destroy data get warmer colours.
var (
	glowBlue   = []int{24, 31, 38, 39, 45, 81, 117, 81, 45, 39, 38, 31}
	glowOrange = []int{94, 130, 166, 172, 208, 214, 215, 214, 208, 172, 166, 130}
	glowRed    = []int{52, 88, 124, 160, 196, 203, 210, 203, 196, 160, 124, 88}
)

// glowFor picks the palette for a verb: red for Deleting, orange for Posting and
// Putting, blue for everything else.
func glowFor(verb string) []int {
	switch verb {
	case "Deleting":
		return glowRed
	case "Posting", "Putting":
		return glowOrange
	}
	return glowBlue
}

var braille = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// VerbFor turns an HTTP method into the word shown while it is in flight.
func VerbFor(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet, "":
		return "Fetching"
	case http.MethodPost:
		return "Posting"
	case http.MethodPut:
		return "Putting"
	case http.MethodPatch:
		return "Patching"
	case http.MethodDelete:
		return "Deleting"
	case http.MethodHead:
		return "Checking"
	case http.MethodOptions:
		return "Asking"
	default:
		return "Sending"
	}
}

// Frame renders frame i of the spinner: a braille dot, the verb with a travelling
// glow, the target, and how long it has been going once that passes a second.
func Frame(i int, verb, target string, elapsed time.Duration) string {
	glow := glowFor(verb)
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[38;5;%dm%s\x1b[0m ", glow[(i/2)%len(glow)], braille[i%len(braille)])
	for k, r := range verb {
		fmt.Fprintf(&b, "\x1b[1;38;5;%dm%c", glow[(i+k*2)%len(glow)], r)
	}
	b.WriteString("\x1b[0m")
	b.WriteString(strings.Repeat(".", i/4%4))
	b.WriteString(strings.Repeat(" ", 3-i/4%4))
	fmt.Fprintf(&b, " \x1b[2m%s\x1b[0m", target)
	if elapsed >= time.Second {
		fmt.Fprintf(&b, " \x1b[2m· %.1fs\x1b[0m", elapsed.Seconds())
	}
	return b.String()
}

// StartSpinner begins drawing on w after `delay`, so quick requests never flash it,
// and returns a stop function that erases the line and restores the cursor. Call
// stop exactly once (defer is fine); it waits for the drawing goroutine to finish so
// nothing is written after it returns.
func StartSpinner(w io.Writer, verb, target string, delay time.Duration) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-time.After(delay):
		case <-done:
			return
		}
		start := time.Now()
		fmt.Fprint(w, "\x1b[?25l") // hide the cursor
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(w, "\r\x1b[2K%s", Frame(i, verb, target, time.Since(start)))
			select {
			case <-tick.C:
			case <-done:
				fmt.Fprint(w, "\r\x1b[2K\x1b[?25h") // erase the line, show the cursor
				return
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}
