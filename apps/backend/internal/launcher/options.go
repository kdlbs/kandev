package launcher

import (
	"fmt"
	"strconv"
	"strings"
)

type Command string

const (
	CommandRun   Command = "run"
	CommandDev   Command = "dev"
	CommandStart Command = "start"
)

type Options struct {
	Command        Command
	RuntimeVersion string
	BackendPort    int
	WebPort        int
	Verbose        bool
	Debug          bool
	ShowVersion    bool
	Headless       bool
	ShowHelp       bool
	Deprecated     []string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func parseArgs(argv []string) (Options, error) {
	opts := Options{Command: CommandRun}
	for i := 0; i < len(argv); i++ {
		next, err := parseArgAt(argv, i, &opts)
		if err != nil {
			return opts, err
		}
		i = next
	}
	return opts, nil
}

func parseArgAt(argv []string, i int, opts *Options) (int, error) {
	arg := argv[i]
	if parseSimpleArg(arg, opts) {
		return i, nil
	}
	if next, ok, err := parseRuntimeArg(argv, i, opts); ok || err != nil {
		return next, err
	}
	if next, ok, err := parsePortArg(argv, i, opts); ok || err != nil {
		return next, err
	}
	return i, ParseError{Message: fmt.Sprintf("unknown argument %q", arg)}
}

func parseSimpleArg(arg string, opts *Options) bool {
	return parseCommandArg(arg, opts) || parseBooleanFlag(arg, opts)
}

func parseCommandArg(arg string, opts *Options) bool {
	switch arg {
	case "run":
		opts.Command = CommandRun
	case "dev", "--dev":
		opts.Command = CommandDev
	case string(CommandStart):
		opts.Command = CommandStart
	default:
		return false
	}
	return true
}

func parseBooleanFlag(arg string, opts *Options) bool {
	switch arg {
	case "--help", "-h", "help":
		opts.ShowHelp = true
	case "--version", "-V":
		opts.ShowVersion = true
	case "--verbose", "-v":
		opts.Verbose = true
	case "--debug":
		opts.Debug = true
	case "--headless", "--no-browser":
		opts.Headless = true
	default:
		return false
	}
	return true
}

func parseRuntimeArg(argv []string, i int, opts *Options) (int, bool, error) {
	arg := argv[i]
	switch {
	case arg == "--runtime-version":
		next, err := parseRuntimeVersion(argv, i, opts)
		return next, true, err
	case strings.HasPrefix(arg, "--runtime-version="):
		return i, true, parseRuntimeVersionValue(arg, opts)
	default:
		return i, false, nil
	}
}

func parsePortArg(argv []string, i int, opts *Options) (int, bool, error) {
	arg := argv[i]
	switch {
	case arg == "--port" || arg == "--backend-port":
		next, err := parseBackendPort(argv, i, opts)
		return next, true, err
	case strings.HasPrefix(arg, "--port="):
		return i, true, parseBackendPortValue("--port", strings.TrimPrefix(arg, "--port="), opts)
	case strings.HasPrefix(arg, "--backend-port="):
		return i, true, parseBackendPortValue("--backend-port", strings.TrimPrefix(arg, "--backend-port="), opts)
	case arg == "--web-internal-port" || arg == "--web-port":
		next, err := parseWebPort(argv, i, opts)
		return next, true, err
	case strings.HasPrefix(arg, "--web-internal-port="):
		return i, true, parseWebPortValue("--web-internal-port", strings.TrimPrefix(arg, "--web-internal-port="), opts)
	case strings.HasPrefix(arg, "--web-port="):
		return i, true, parseWebPortValue("--web-port", strings.TrimPrefix(arg, "--web-port="), opts)
	default:
		return i, false, nil
	}
}

func parseRuntimeVersion(argv []string, i int, opts *Options) (int, error) {
	v, err := takeValue(argv, i, argv[i])
	if err != nil {
		return i, err
	}
	opts.RuntimeVersion = v
	return i + 1, nil
}

func parseRuntimeVersionValue(arg string, opts *Options) error {
	v := strings.TrimPrefix(arg, "--runtime-version=")
	if v == "" {
		return ParseError{Message: "--runtime-version requires a value"}
	}
	opts.RuntimeVersion = v
	return nil
}

func parseBackendPort(argv []string, i int, opts *Options) (int, error) {
	v, err := takeValue(argv, i, argv[i])
	if err != nil {
		return i, err
	}
	if err := parseBackendPortValue(argv[i], v, opts); err != nil {
		return i, err
	}
	return i + 1, nil
}

func parseBackendPortValue(flag, value string, opts *Options) error {
	port, err := parsePort(value, flag)
	if err != nil {
		return err
	}
	opts.BackendPort = port
	return nil
}

func parseWebPort(argv []string, i int, opts *Options) (int, error) {
	v, err := takeValue(argv, i, argv[i])
	if err != nil {
		return i, err
	}
	if err := parseWebPortValue(argv[i], v, opts); err != nil {
		return i, err
	}
	return i + 1, nil
}

func parseWebPortValue(flag, value string, opts *Options) error {
	port, err := parsePort(value, flag)
	if err != nil {
		return err
	}
	opts.WebPort = port
	if flag == "--web-port" {
		opts.Deprecated = appendDeprecated(opts.Deprecated, flag)
	}
	return nil
}

func takeValue(argv []string, i int, flag string) (string, error) {
	if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
		return "", ParseError{Message: fmt.Sprintf("%s requires a value", flag)}
	}
	return argv[i+1], nil
}

func parsePort(raw, flag string) (int, error) {
	n, err := strconv.Atoi(raw)
	if raw == "" || err != nil || n < 1 || n > 65535 {
		return 0, ParseError{Message: fmt.Sprintf("%s value must be an integer between 1 and 65535, got %q", flag, raw)}
	}
	return n, nil
}

func appendDeprecated(flags []string, flag string) []string {
	for _, existing := range flags {
		if existing == flag {
			return flags
		}
	}
	return append(flags, flag)
}
