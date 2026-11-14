package plugins

import (
	"net/http"
	"testing"
)

func TestInstalledUIFilesRevalidateBeforeReuse(t *testing.T) {
	router, svc := newTestRouter(t)
	if _, err := svc.Install(t.Context(), testPackage(t, "kandev-plugin-ui", "1.0.0", true)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	for _, path := range []string{
		"/api/plugins/kandev-plugin-ui/bundle",
		"/api/plugins/kandev-plugin-ui/ui/ui/style.css",
	} {
		first := doRequest(router, http.MethodGet, path, "", nil)
		if first.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200, body=%s", path, first.Code, first.Body.String())
		}
		if got := first.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s: Cache-Control = %q, want no-cache", path, got)
		}
		lastModified := first.Header().Get("Last-Modified")
		if lastModified == "" {
			t.Fatalf("%s: Last-Modified missing, browsers cannot revalidate", path)
		}

		again := doRequest(router, http.MethodGet, path, "", map[string]string{"If-Modified-Since": lastModified})
		if again.Code != http.StatusNotModified {
			t.Fatalf("%s: revalidation status = %d, want 304", path, again.Code)
		}
		if again.Body.Len() != 0 {
			t.Fatalf("%s: revalidation body = %q, want empty", path, again.Body.String())
		}
	}
}
