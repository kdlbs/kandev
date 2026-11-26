package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPackRejectsThemeWithUndeclaredExecutable(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"manifest.yaml": `id: sample-themes
api_version: 2
version: 1.0.0
display_name: Sample
distribution: {schema_version: 1, kind: theme}
themes: [{key: dark, label: Dark, base: dark, path: dark.json}]
`, "dark.json": `{"schema_version":1}`, "evil.js": "alert(1)"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := Pack(dir, &out, PackOptions{}); err == nil {
		t.Fatal("unsafe theme packaged")
	}
	if err := os.Remove(filepath.Join(dir, "evil.js")); err != nil {
		t.Fatal(err)
	}
	if err := Pack(dir, &out, PackOptions{}); err != nil {
		t.Fatal(err)
	}
}
