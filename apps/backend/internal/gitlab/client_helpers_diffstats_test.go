package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestPATClient_ListMRFiles_CountsDashAndPlusPrefixedContent walks the live path
// so shared diff counting is verified through the MR file statistics response.
func TestPATClient_ListMRFiles_CountsDashAndPlusPrefixedContent(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"changes": []map[string]any{{
				"old_path": "seed.sql",
				"new_path": "seed.sql",
				"diff": "--- a/seed.sql\n" +
					"+++ b/seed.sql\n" +
					"@@ -1,2 +1,2 @@\n" +
					"--- seed data\n" +
					" counter;\n" +
					"+++counter;\n",
			}},
		})
	}))
	defer stop()

	files, err := NewPATClient(host, "tok").ListMRFiles(context.Background(), "group/project", 7)
	if err != nil {
		t.Fatalf("ListMRFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", files[0].Additions, files[0].Deletions)
	}
}
