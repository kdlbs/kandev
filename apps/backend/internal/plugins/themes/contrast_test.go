package themes

import "testing"

func TestContrastFindings(t *testing.T) {
	d := &Definition{Base: "dark", UI: map[string]string{"background": "#272822", "foreground": "#292a24"}}
	if got := ContrastFindings(d); len(got) != 1 {
		t.Fatalf("expected low-contrast finding, got %v", got)
	}
	d.UI["foreground"] = "#ffffff"
	if got := ContrastFindings(d); len(got) != 0 {
		t.Fatalf("unexpected finding: %v", got)
	}
}
