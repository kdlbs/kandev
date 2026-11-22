package manifest

import (
	"strings"
	"testing"
)

func TestFilePreviewManifestRejectsUnsafeExtensions(t *testing.T) {
	for _, extension := range []string{"*", "../stl", ".stl", "STL", "", strings.Repeat("x", 33)} {
		text := strings.Replace(validManifestYAML, "ui:\n", "ui:\n  bundle: /bundle.js\n  file_viewers:\n    - id: stl\n      title: STL viewer\n      extensions: [\""+extension+"\"]\n", 1)
		m, err := Parse([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Validate(); err == nil {
			t.Errorf("unsafe extension accepted: %q", extension)
		}
	}
}

func TestFilePreviewManifestValidDeclaration(t *testing.T) {
	text := strings.Replace(validManifestYAML, "ui:\n", "ui:\n  bundle: /bundle.js\n  file_viewers:\n    - id: stl\n      title: STL viewer\n      extensions: [stl]\n", 1)
	m, err := Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}
