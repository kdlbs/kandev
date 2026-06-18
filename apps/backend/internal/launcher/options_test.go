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
