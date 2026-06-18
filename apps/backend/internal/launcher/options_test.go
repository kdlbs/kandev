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
	_, err := parseArgs([]string{"--web-port", "12345"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseArgsRejectsUnsupportedDevMode(t *testing.T) {
	for _, argv := range [][]string{{"dev"}, {"--dev"}} {
		_, err := parseArgs(argv)
		if err == nil {
			t.Fatalf("parseArgs(%v) returned nil error", argv)
		}
		if _, ok := err.(ParseError); !ok {
			t.Fatalf("parseArgs(%v) error = %T, want ParseError", argv, err)
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
