package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.1, AC-WORKSPACES-HIDDEN-FOLDERS-001.2,
// AC-WORKSPACES-HIDDEN-FOLDERS-001.6
func TestHTTPListDirectoryHiddenEntryVisibilityFollowsRequestValue(t *testing.T) {
	// The reveal is an explicit display decision. Only the exact value "true"
	// activates it, so a mis-spelled or truthy-looking value keeps the current
	// contract instead of erroring on a display-only concern.
	cases := []struct {
		name         string
		query        string
		wantRevealed bool
	}{
		{name: "exact true reveals", query: "&include_hidden=true", wantRevealed: true},
		{name: "absent keeps current contract", query: "", wantRevealed: false},
		{name: "uppercase is not the exact value", query: "&include_hidden=TRUE", wantRevealed: false},
		{name: "numeric one is not the exact value", query: "&include_hidden=1", wantRevealed: false},
		{name: "yes is not the exact value", query: "&include_hidden=yes", wantRevealed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, _ := newRepositoryHTTPTestRouter(t)
			root := t.TempDir()
			// Ordinary entries bracket the hidden one so a revealed listing can
			// prove that ordinary entries keep their existing relative order.
			for _, name := range []string{"alpha", "gamma"} {
				if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", name, err)
				}
			}
			if err := os.Mkdir(filepath.Join(root, ".hidden-dir"), 0o755); err != nil {
				t.Fatalf("mkdir hidden dir: %v", err)
			}
			// A hidden file is never a browsable entry, revealed or not.
			if err := os.WriteFile(filepath.Join(root, ".hidden-file"), []byte("x"), 0o644); err != nil {
				t.Fatalf("write hidden file: %v", err)
			}

			request := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/fs/list-dir?path="+url.QueryEscape(root)+tc.query,
				nil,
			)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			var body struct {
				Entries []listDirEntry `json:"entries"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			// Neither contract lists a file, hidden or not, so a revealed
			// listing must still contain directories only.
			for _, entry := range body.Entries {
				if entry.Name == HIDDEN_FILE_FIXTURE {
					t.Fatalf("hidden file listed as a browsable entry: %+v", body.Entries)
				}
			}
			names := entryNames(body.Entries)
			want := []string{"alpha", "gamma"}
			if tc.wantRevealed {
				// A revealed listing leads with the dot entry, and ordinary
				// entries keep the relative order they have without it.
				want = append([]string{".hidden-dir"}, want...)
			}
			if len(names) != len(want) {
				t.Fatalf("entries = %v, want %v", names, want)
			}
			for i := range want {
				if names[i] != want[i] {
					t.Fatalf("entries = %v, want %v", names, want)
				}
			}
		})
	}
}

// HIDDEN_FILE_FIXTURE is a hidden file, which no listing may contain.
const HIDDEN_FILE_FIXTURE = ".hidden-file"

// listDirEntry is one entry of a GET /api/v1/fs/list-dir response.
type listDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// entryNames reduces a listing to its entry names for order comparison.
func entryNames(entries []listDirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

// @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.11
func TestHTTPListDirectoryFailureIsIdenticalWithAndWithoutTheReveal(t *testing.T) {
	// Revealing hidden entries must not widen access or change what a failure
	// discloses, so an unlistable path answers the same either way and never
	// echoes the requested host path back to the client.
	router, _ := newRepositoryHTTPTestRouter(t)
	notADirectory := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "absent-directory")

	for _, target := range []struct {
		name string
		path string
	}{
		{name: "a path that is not a directory", path: notADirectory},
		{name: "a path that does not exist", path: missing},
	} {
		t.Run(target.name, func(t *testing.T) {
			statuses := make([]int, 0, 2)
			bodies := make([]string, 0, 2)
			for _, query := range []string{"", "&include_hidden=true"} {
				response := httptest.NewRequest(
					http.MethodGet,
					"/api/v1/fs/list-dir?path="+url.QueryEscape(target.path)+query,
					nil,
				)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, response)
				statuses = append(statuses, recorder.Code)
				bodies = append(bodies, recorder.Body.String())
			}
			if statuses[0] != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", statuses[0], http.StatusBadRequest, bodies[0])
			}
			if statuses[0] != statuses[1] || bodies[0] != bodies[1] {
				t.Fatalf("reveal changed the failure: default (%d, %s) vs revealed (%d, %s)",
					statuses[0], bodies[0], statuses[1], bodies[1])
			}
			if strings.Contains(bodies[0], target.path) {
				t.Fatalf("failure body discloses the requested host path %q: %s", target.path, bodies[0])
			}
		})
	}
}

// @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.7
func TestHTTPCreateDirectoryDotPrefixedChildIsEnterableAndRevealable(t *testing.T) {
	// create-dir returns a listing of the folder it just created, and the
	// browser navigates into that folder and re-lists it. The created name is
	// therefore only ever rendered by its parent's listing, which is where the
	// reveal has to make it appear.
	router, _ := newRepositoryHTTPTestRouter(t)
	parent := trustedRepositoryParent(t)
	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/fs/create-dir",
		strings.NewReader(`{"parent_path":`+strconv.Quote(parent)+`,"name":".hidden-child"}`),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	createResponse := httptest.NewRecorder()

	router.ServeHTTP(createResponse, createRequest)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body = %s",
			createResponse.Code, http.StatusCreated, createResponse.Body.String())
	}
	createdPath := filepath.Join(parent, ".hidden-child")
	if info, err := os.Stat(createdPath); err != nil || !info.IsDir() {
		t.Fatalf("created directory %q: info=%v error=%v", createdPath, info, err)
	}

	// A dot-prefixed name is accepted, so the new folder is enterable directly.
	enterRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/fs/list-dir?path="+url.QueryEscape(createdPath)+"&include_hidden=true",
		nil,
	)
	enterResponse := httptest.NewRecorder()
	router.ServeHTTP(enterResponse, enterRequest)
	if enterResponse.Code != http.StatusOK {
		t.Fatalf("entering created folder: status = %d, want %d; body = %s",
			enterResponse.Code, http.StatusOK, enterResponse.Body.String())
	}

	// Returning to the parent shows the new folder only when the reveal is on.
	for _, tc := range []struct {
		name     string
		query    string
		wantSeen bool
	}{
		{name: "revealed", query: "&include_hidden=true", wantSeen: true},
		{name: "default", query: "", wantSeen: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listRequest := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/fs/list-dir?path="+url.QueryEscape(parent)+tc.query,
				nil,
			)
			listResponse := httptest.NewRecorder()
			router.ServeHTTP(listResponse, listRequest)

			if listResponse.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s",
					listResponse.Code, http.StatusOK, listResponse.Body.String())
			}
			var body struct {
				Entries []listDirEntry `json:"entries"`
			}
			if err := json.Unmarshal(listResponse.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			seen := false
			for _, entry := range body.Entries {
				if entry.Name == ".hidden-child" {
					seen = true
				}
			}
			if seen != tc.wantSeen {
				t.Fatalf("created folder seen = %v, want %v; body = %s",
					seen, tc.wantSeen, listResponse.Body.String())
			}
		})
	}
}
