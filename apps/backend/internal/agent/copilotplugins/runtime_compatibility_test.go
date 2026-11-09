package copilotplugins

import "testing"

func TestHelpAdvertisesPluginDirOption(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "separate argument", text: "Options:\n  --plugin-dir <path>  Load plugin directories\n", want: true},
		{name: "equals argument", text: "Options:\n  --plugin-dir=<path>  Load plugin directories\n", want: true},
		{name: "prose token", text: "Use --plugin-dir to load one or more directories.", want: false},
		{name: "option after prose", text: "Options include --plugin-dir <path>.", want: false},
		{name: "partial token", text: "  --plugin-directory <path>  Load plugin directories\n", want: false},
		{name: "missing argument", text: "  --plugin-dir  Load plugin directories\n", want: false},
		{name: "incomplete equals token", text: "  --plugin-dir=\n", want: false},
		{name: "invalid utf8", text: string([]byte{'-', '-', 'p', 'l', 'u', 'g', 'i', 'n', '-', 'd', 'i', 'r', 0xff}), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := helpAdvertisesPluginDir([]byte(test.text)); got != test.want {
				t.Fatalf("helpAdvertisesPluginDir(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}
