package clarification

import "testing"

func TestNormalizeContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "actual newlines", input: "first\n\nsecond", want: "first\n\nsecond"},
		{name: "escaped line feeds", input: `first\n\nsecond`, want: "first\n\nsecond"},
		{name: "escaped CRLF", input: `first\r\n\r\nsecond`, want: "first\n\nsecond"},
		{name: "escaped carriage returns", input: `first\r\rsecond`, want: "first\n\nsecond"},
		{name: "single escape syntax", input: `Use \n or open C:\new\agent`, want: `Use \n or open C:\new\agent`},
		{name: "unrelated backslashes", input: `C:\tools\agent and \\server\share`, want: `C:\tools\agent and \\server\share`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeContext(tt.input); got != tt.want {
				t.Fatalf("NormalizeContext(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
