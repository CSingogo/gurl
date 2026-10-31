package output

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"os"
	"strings"
)

// IsTerminal reports whether w is a character device (a terminal) and the user
// has not opted out of color with NO_COLOR (https://no-color.org).
func IsTerminal(w io.Writer) bool {
	return IsTTY(w) && os.Getenv("NO_COLOR") == ""
}

// IsTTY reports whether w is a terminal, ignoring NO_COLOR. Use it for things
// that are about a human being present (hints), not about colour.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// IsJSON reports whether a Content-Type header describes JSON, including
// vendor types such as application/vnd.api+json.
func IsJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// PrettyJSON indents and colors body. ok is false when body is not valid JSON,
// in which case the caller should print the original bytes untouched.
func PrettyJSON(body []byte, s Style) (out []byte, ok bool) {
	if !json.Valid(body) {
		return nil, false
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, bytes.TrimSpace(body), "", "  "); err != nil {
		return nil, false
	}
	indented.WriteByte('\n')
	if !s.Color {
		return indented.Bytes(), true
	}
	return colorizeJSON(indented.Bytes(), s), true
}

// colorizeJSON walks already-indented, already-valid JSON and wraps each token
// in color codes. Because the input is known to be well formed, a small scanner
// is enough: no need to re-parse it.
func colorizeJSON(src []byte, s Style) []byte {
	var out bytes.Buffer
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"':
			j := i + 1
			for src[j] != '"' {
				if src[j] == '\\' {
					j++ // skip the escaped character, which may itself be a quote
				}
				j++
			}
			j++ // include the closing quote
			// A string followed by ':' is an object key.
			color := green
			if k := j; k < len(src) && src[k] == ':' {
				color = blue
			}
			out.WriteString(s.paint(color, string(src[i:j])))
			i = j
		case c == '-' || (c >= '0' && c <= '9'):
			j := i
			for j < len(src) && strings.IndexByte("+-.eE0123456789", src[j]) >= 0 {
				j++
			}
			out.WriteString(s.paint(yellow, string(src[i:j])))
			i = j
		case c == 't' || c == 'f' || c == 'n':
			j := i
			for j < len(src) && src[j] >= 'a' && src[j] <= 'z' {
				j++
			}
			out.WriteString(s.paint(magenta, string(src[i:j])))
			i = j
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.Bytes()
}
