package output

import (
	"fmt"
	"strings"
)

// Expand implements -w / --write-out: %{name} is replaced by lookup(name), %%
// is a literal percent, and \n \r \t \\ are the usual escapes. Unknown names
// expand to nothing, like curl (which also warns; gurl keeps quiet).
func Expand(format string, lookup func(name string) (string, bool)) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		switch c := format[i]; {
		case c == '%' && i+1 < len(format) && format[i+1] == '%':
			out.WriteByte('%')
			i++
		case c == '%' && i+1 < len(format) && format[i+1] == '{':
			end := strings.IndexByte(format[i:], '}')
			if end < 0 {
				out.WriteString(format[i:])
				return out.String()
			}
			name := format[i+2 : i+end]
			if v, ok := lookup(name); ok {
				out.WriteString(v)
			}
			i += end
		case c == '\\' && i+1 < len(format):
			i++
			switch format[i] {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\':
				out.WriteByte('\\')
			default:
				out.WriteByte('\\')
				out.WriteByte(format[i])
			}
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// Seconds formats a duration the way curl's time_* variables do: seconds with six decimals.
func Seconds(d interface{ Seconds() float64 }) string {
	return fmt.Sprintf("%.6f", d.Seconds())
}
