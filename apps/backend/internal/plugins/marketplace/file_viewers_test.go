package marketplace

import "testing"

func TestFileViewerExtensionQuery(t *testing.T) {
	entries := []CatalogEntry{{IndexEntry: IndexEntry{ID: "stl", FileExtensions: []string{"stl"}}}, {IndexEntry: IndexEntry{ID: "obj", Description: "STL viewer", FileExtensions: []string{"obj"}}}, {IndexEntry: IndexEntry{ID: "unknown"}}}
	for _, query := range []string{"stl", "STL", ".stl"} {
		got := ApplyQuery(entries, Query{Extension: query})
		if len(got) != 1 || got[0].ID != "stl" {
			t.Fatalf("exact extension query %q: %+v", query, got)
		}
	}
	if got := ApplyQuery(entries, Query{Extension: "../stl"}); len(got) != 0 {
		t.Fatal("invalid extension matched")
	}
}
