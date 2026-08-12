package launcher

import "testing"

func TestParseArgsStartPortsAndHeadless(t *testing.T) {
	opts, err := parseArgs([]string{"start", "--port", "1234", "--headless"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Command != CommandStart {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandStart)
	}
	if opts.BackendPort != 1234 || !opts.Headless {
		t.Fatalf("parsed options = %+v", opts)
	}
}

func TestParseArgsRejectsInvalidPort(t *testing.T) {
	_, err := parseArgs([]string{"--port", "70000"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseArgsRejectsRemovedWebPort(t *testing.T) {
	for _, argv := range [][]string{
		{"--web-port", "12345"},
		{"--web-port=12345"},
	} {
		_, err := parseArgs(argv)
		if err == nil {
			t.Fatalf("parseArgs(%v) returned nil error", argv)
		}
		if err.Error() != "--web-port has been removed; use --web-internal-port for dev mode" {
			t.Fatalf("parseArgs(%v) error = %q", argv, err)
		}
	}
}

func TestParseArgsAcceptsDevCommand(t *testing.T) {
	for _, argv := range [][]string{{"dev"}, {"--dev"}} {
		opts, err := parseArgs(argv)
		if err != nil {
			t.Fatalf("parseArgs(%v) = %v", argv, err)
		}
		if opts.Command != CommandDev {
			t.Fatalf("parseArgs(%v) Command = %q, want %q", argv, opts.Command, CommandDev)
		}
	}
}

func TestParseArgsAcceptsWebInternalPort(t *testing.T) {
	for _, argv := range [][]string{
		{"dev", "--web-internal-port", "37430"},
		{"dev", "--web-internal-port=37430"},
	} {
		opts, err := parseArgs(argv)
		if err != nil {
			t.Fatalf("parseArgs(%v) = %v", argv, err)
		}
		if opts.Command != CommandDev || opts.WebPort != 37430 {
			t.Fatalf("parseArgs(%v) = %+v", argv, opts)
		}
	}
}

func TestParseArgsRejectsWebInternalPortOutsideDevMode(t *testing.T) {
	for _, argv := range [][]string{
		{"--web-internal-port", "37430"},
		{"run", "--web-internal-port=37430"},
		{"start", "--web-internal-port", "37430"},
	} {
		_, err := parseArgs(argv)
		if err == nil {
			t.Fatalf("parseArgs(%v) returned nil error", argv)
		}
		if err.Error() != "--web-internal-port only applies to dev mode" {
			t.Fatalf("parseArgs(%v) error = %q", argv, err)
		}
	}
}

func TestParseArgsRejectsInvalidWebPort(t *testing.T) {
	for _, argv := range [][]string{
		{"dev", "--web-internal-port", "70000"},
		{"dev", "--web-internal-port=0"},
		{"dev", "--web-internal-port"},
	} {
		if _, err := parseArgs(argv); err == nil {
			t.Fatalf("parseArgs(%v) returned nil error", argv)
		}
	}
}

func TestParseArgsRejectsUnsupportedRuntimeVersion(t *testing.T) {
	for _, argv := range [][]string{
		{"--runtime-version", "v1.2.3"},
		{"--runtime-version=v1.2.3"},
		{"--runtime-version"},
		{"--runtime-version="},
	} {
		_, err := parseArgs(argv)
		if err == nil {
			t.Fatalf("parseArgs(%v) returned nil error", argv)
		}
		if err.Error() != "--runtime-version is not supported by the native launcher" {
			t.Fatalf("parseArgs(%v) error = %q", argv, err)
		}
	}
}
