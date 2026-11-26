package themes

import (
	"strings"
	"testing"
)

// @covers AC-PLUGINS-THEMES-001.2
func TestCanonicalValidation(t *testing.T) {
	good := `{"schema_version":1,"ui":{"background":"#272822","foreground":"#FFF"},"syntax":{"keyword":{"foreground":"#f92672","fontStyle":"bold italic"}}}`
	d, err := Parse([]byte(good), "dark")
	if err != nil {
		t.Fatal(err)
	}
	if d.UI["foreground"] != "#ffffff" {
		t.Fatalf("not normalized: %v", d.UI)
	}
	for _, bad := range []string{
		`{"schema_version":2}`, `{"schema_version":1,"ui":{"made-up":"#fff"}}`,
		`{"schema_version":1,"ui":{"background":"#ffffff00"}}`,
		`{"schema_version":1,"ui":{"foreground":"url(evil)"}}`,
		`{"schema_version":1,"base":"light"}`, `{"schema_version":1,"schema_version":1}`,
		`{"schema_version":1,"__proto__":{}}`, `{"schema_version":1} {}`,
		`{"schema_version":1,"syntax":{"keyword":{"fontStyle":"display:none"}}}`,
		`{"schema_version":1,"textmate":[{"scope":"x","settings":{"foreground":"red"}}]}`,
		strings.Repeat(" ", 256*1024) + `{}`,
	} {
		if _, err := Parse([]byte(bad), "dark"); err == nil {
			t.Errorf("accepted %s", bad[:min(len(bad), 150)])
		}
	}
}
