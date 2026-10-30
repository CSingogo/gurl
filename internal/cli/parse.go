package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// spec describes one flag. takesArg flags consume a value; the rest are switches.
type spec struct {
	short    byte // 0 when the flag has no short form
	long     string
	takesArg bool
	apply    func(o *Options, v string) error
}

func on(set func(*Options)) func(*Options, string) error {
	return func(o *Options, _ string) error { set(o); return nil }
}

// seconds parses curl's duration arguments. Like curl, it accepts fractions ("1.5").
func seconds(name string, set func(*Options, time.Duration)) func(*Options, string) error {
	return func(o *Options, v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return usage("option %s: expected a non-negative number of seconds, got %q", name, v)
		}
		set(o, time.Duration(f*float64(time.Second)))
		return nil
	}
}

func data(kind DataKind) func(*Options, string) error {
	return func(o *Options, v string) error {
		o.Data = append(o.Data, DataItem{Kind: kind, Value: v})
		return nil
	}
}

// specs is the whole flag table. Order is irrelevant; lookups build maps below.
var specs = []spec{
	{short: 'X', long: "request", takesArg: true, apply: func(o *Options, v string) error { o.Method = strings.ToUpper(v); return nil }},
	{short: 'H', long: "header", takesArg: true, apply: func(o *Options, v string) error { o.Headers = append(o.Headers, v); return nil }},
	{short: 'd', long: "data", takesArg: true, apply: data(DataText)},
	{long: "data-raw", takesArg: true, apply: data(DataRaw)},
	{long: "data-binary", takesArg: true, apply: data(DataBinary)},
	{long: "json", takesArg: true, apply: data(DataJSON)},
	{short: 'G', long: "get", apply: on(func(o *Options) { o.GetMode = true })},
	{short: 'A', long: "user-agent", takesArg: true, apply: func(o *Options, v string) error { o.UserAgent = v; return nil }},
	{short: 'e', long: "referer", takesArg: true, apply: func(o *Options, v string) error { o.Referer = v; return nil }},
	{short: 'u', long: "user", takesArg: true, apply: func(o *Options, v string) error { o.User = v; return nil }},

	{short: 'i', long: "include", apply: on(func(o *Options) { o.Include = true })},
	{short: 'I', long: "head", apply: on(func(o *Options) { o.Head = true })},
	{short: 'v', long: "verbose", apply: on(func(o *Options) { o.Verbose = true })},
	{short: 's', long: "silent", apply: on(func(o *Options) { o.Silent = true })},
	{short: 'S', long: "show-error", apply: on(func(o *Options) { o.ShowError = true })},
	{short: 'f', long: "fail", apply: on(func(o *Options) { o.Fail = true })},
	{short: 'o', long: "output", takesArg: true, apply: func(o *Options, v string) error { o.Output = v; return nil }},
	{short: 'O', long: "remote-name", apply: on(func(o *Options) { o.RemoteName = true })},
	{short: 'w', long: "write-out", takesArg: true, apply: func(o *Options, v string) error { o.WriteOut = v; return nil }},

	{short: 'L', long: "location", apply: on(func(o *Options) { o.Location = true })},
	{long: "max-redirs", takesArg: true, apply: func(o *Options, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < -1 {
			return usage("option --max-redirs: expected a number (or -1 for unlimited), got %q", v)
		}
		o.MaxRedirs = n
		return nil
	}},
	{short: 'k', long: "insecure", apply: on(func(o *Options) { o.Insecure = true })},
	{short: 'm', long: "max-time", takesArg: true, apply: seconds("--max-time", func(o *Options, d time.Duration) { o.MaxTime = d })},
	{long: "connect-timeout", takesArg: true, apply: seconds("--connect-timeout", func(o *Options, d time.Duration) { o.ConnectTimeout = d })},

	{long: "raw", apply: on(func(o *Options) { o.Raw = true })},
	{short: 'h', long: "help", apply: on(func(o *Options) { o.Help = true })},
	{short: 'V', long: "version", apply: on(func(o *Options) { o.Version = true })},
}

var (
	byShort = map[byte]*spec{}
	byLong  = map[string]*spec{}
)

// Go runs package-level init() functions once, before main. Here it indexes the
// flag table so Parse can look flags up in O(1).
func init() {
	for i := range specs {
		s := &specs[i]
		if s.short != 0 {
			byShort[s.short] = s
		}
		byLong[s.long] = s
	}
}

func usage(format string, args ...any) *ExitError {
	return &ExitError{Code: ExitBadUsage, Msg: fmt.Sprintf(format, args...)}
}

// Parse reads args (without the program name) the way curl does:
//
//   - short flags combine: -sSL is -s -S -L
//   - a short flag's value may be attached (-XPOST) or separate (-X POST)
//   - long flags take --name value or --name=value
//   - flags may appear before or after the URL
//   - "--" ends flag parsing; everything after it is a URL
func Parse(args []string) (*Options, error) {
	o := &Options{MaxRedirs: 50}
	var urls []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			urls = append(urls, args[i+1:]...)
			i = len(args)

		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			s, ok := byLong[name]
			if !ok {
				return nil, usage("option --%s: is unknown", name)
			}
			switch {
			case s.takesArg && !hasValue:
				if i+1 >= len(args) {
					return nil, usage("option --%s: requires parameter", name)
				}
				i++
				value = args[i]
			case !s.takesArg && hasValue:
				return nil, usage("option --%s: is a switch and takes no value", name)
			}
			if err := s.apply(o, value); err != nil {
				return nil, err
			}

		// A lone "-" is not a flag; by convention it means stdin/stdout.
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			for j := 1; j < len(arg); j++ {
				s, ok := byShort[arg[j]]
				if !ok {
					return nil, usage("option -%c: is unknown", arg[j])
				}
				if !s.takesArg {
					if err := s.apply(o, ""); err != nil {
						return nil, err
					}
					continue
				}
				// The rest of this word is the value (-XPOST); if there is none, take the next word.
				value := arg[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						return nil, usage("option -%c: requires parameter", arg[j])
					}
					i++
					value = args[i]
				}
				if err := s.apply(o, value); err != nil {
					return nil, err
				}
				break // the value consumed the rest of the word
			}

		default:
			urls = append(urls, arg)
		}
	}

	if o.Help || o.Version {
		return o, nil
	}
	switch len(urls) {
	case 0:
		return nil, usage("no URL specified")
	case 1:
		o.URL = urls[0]
	default:
		return nil, usage("only one URL is supported for now, got %d", len(urls))
	}
	return o, nil
}
