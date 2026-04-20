package cliflags

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestTokenise(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "whitespace only", input: "   \t  ", want: nil},
		{name: "single flag", input: "--allow-all-tools", want: []string{"--allow-all-tools"}},
		{name: "flag with value", input: "--add-dir /shared", want: []string{"--add-dir", "/shared"}},
		{name: "flag eq value", input: "--foo=bar", want: []string{"--foo=bar"}},
		{name: "double quoted value", input: `--msg "hello world"`, want: []string{"--msg", "hello world"}},
		{name: "single quoted value", input: `--msg 'hello world'`, want: []string{"--msg", "hello world"}},
		{name: "escaped space", input: `--path /tmp/my\ dir`, want: []string{"--path", "/tmp/my dir"}},
		{name: "escaped quote inside dq", input: `--json "{\"a\":1}"`, want: []string{"--json", `{"a":1}`}},
		{name: "multiple tokens", input: "--a 1 --b 2", want: []string{"--a", "1", "--b", "2"}},
		{name: "tabs between tokens", input: "--a\t1", want: []string{"--a", "1"}},
		{name: "unterminated double quote", input: `--msg "hi`, wantErr: true},
		{name: "unterminated single quote", input: `--msg 'hi`, wantErr: true},
		{name: "trailing backslash unquoted", input: `--flag foo\`, wantErr: true},
		{name: "trailing backslash inside dq", input: `--msg "hi\`, wantErr: true},
		{name: "empty quoted string yields empty token", input: `--empty ""`, want: []string{"--empty", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Tokenise(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got tokens %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tokens mismatch\n  got:  %#v\n  want: %#v", got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	flags := []models.CLIFlag{
		{Flag: "--allow-all-tools", Enabled: true},
		{Flag: "--allow-all-paths", Enabled: false}, // should be skipped
		{Flag: "--add-dir /shared", Enabled: true},
		{Flag: "--max-continues 5", Enabled: true},
	}
	got, err := Resolve(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--allow-all-tools", "--add-dir", "/shared", "--max-continues", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolved tokens mismatch\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestResolve_ErrorPointsToOffendingIndex(t *testing.T) {
	flags := []models.CLIFlag{
		{Flag: "--ok", Enabled: true},
		{Flag: `--broken "unterminated`, Enabled: true},
	}
	_, err := Resolve(flags)
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
	if !strings.Contains(err.Error(), "cli_flags[1]") {
		t.Errorf("error should identify offending index, got: %v", err)
	}
}

// TestTokenise_ErrorsOmitRawInput confirms the error messages surface the
// error kind and position without embedding the user-authored flag string.
// Launch-time fallback logs these errors; raw flag content could carry
// paths or tokens that shouldn't end up in structured logs.
func TestTokenise_ErrorsOmitRawInput(t *testing.T) {
	sensitive := `--config "/home/u/.secret/token`
	_, err := Tokenise(sensitive)
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
	if strings.Contains(err.Error(), "/home/u/.secret/token") {
		t.Errorf("error should not echo raw flag contents, got: %v", err)
	}
}

func TestResolve_AllDisabled_ReturnsNil(t *testing.T) {
	flags := []models.CLIFlag{
		{Flag: "--a", Enabled: false},
		{Flag: "--b", Enabled: false},
	}
	got, err := Resolve(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil tokens, got %#v", got)
	}
}
