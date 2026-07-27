package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const reviewPromptFixture = codeReviewSentinel + `

## Changed files

apps/web/a.ts
apps/backend/b.go

## Diff

### apps/web/a.ts (modified)
@@ -1,3 +1,5 @@
 const existing = 1;
+const added = value;
+const other = 2;
 export {};

### apps/backend/b.go (added)
@@ -0,0 +1,2 @@
+package main
+func main() {}
`

type parsedReview struct {
	Summary  string `json:"summary"`
	Findings []struct {
		Repo     string `json:"repo"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Severity string `json:"severity"`
		Category string `json:"category"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	} `json:"findings"`
}

func parseMockReview(t *testing.T, response string) parsedReview {
	t.Helper()
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start < 0 || end <= start {
		t.Fatalf("no JSON object in response:\n%s", response)
	}
	var parsed parsedReview
	if err := json.Unmarshal([]byte(response[start:end+1]), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, response)
	}
	return parsed
}

func TestIsCodeReviewRequest(t *testing.T) {
	if !isCodeReviewRequest(reviewPromptFixture) {
		t.Fatal("expected the sentinel to be recognised")
	}
	if isCodeReviewRequest("please review my code") {
		t.Fatal("prose must not be mistaken for a review request")
	}
	if isCodeReviewRequest("") {
		t.Fatal("empty prompt must not be a review request")
	}
}

func TestCodeReviewResponse_AnchorsToRealFilesAndLines(t *testing.T) {
	parsed := parseMockReview(t, codeReviewResponse(reviewPromptFixture))

	if parsed.Summary == "" {
		t.Fatal("expected a summary")
	}
	if len(parsed.Findings) != 3 {
		t.Fatalf("expected 2 valid findings + 1 deliberately malformed, got %d", len(parsed.Findings))
	}

	first := parsed.Findings[0]
	if first.File != "apps/web/a.ts" {
		t.Fatalf("first finding should anchor to the first file in the prompt, got %q", first.File)
	}
	// The first added line of that hunk is new-side line 2.
	if first.Line != 2 {
		t.Fatalf("expected the first added line (2), got %d", first.Line)
	}
	if first.Severity != "blocker" {
		t.Fatalf("expected a blocker first, got %q", first.Severity)
	}

	second := parsed.Findings[1]
	if second.File != "apps/backend/b.go" {
		t.Fatalf("second finding should use the second file, got %q", second.File)
	}
	if second.Line != 1 {
		t.Fatalf("expected new-side line 1 for an added file, got %d", second.Line)
	}

	// The third entry has no file: it exercises the rejected-entry path, where
	// the run still completes and reports the discard.
	if parsed.Findings[2].File != "" {
		t.Fatalf("expected the third entry to be malformed, got %+v", parsed.Findings[2])
	}
}

func TestCodeReviewResponse_MultiRepoCarriesRepo(t *testing.T) {
	prompt := codeReviewSentinel + `
### repo=backend file=src/a.go (modified)
@@ -1,2 +1,3 @@
 package main
+var added = 1
`
	parsed := parseMockReview(t, codeReviewResponse(prompt))
	if len(parsed.Findings) == 0 {
		t.Fatal("expected findings")
	}
	if parsed.Findings[0].Repo != "backend" {
		t.Fatalf("expected the repo prefix carried through, got %q", parsed.Findings[0].Repo)
	}
	if parsed.Findings[0].File != "src/a.go" {
		t.Fatalf("expected the repo-relative path, got %q", parsed.Findings[0].File)
	}
}

func TestCodeReviewResponse_SingleFileReusesItForBothFindings(t *testing.T) {
	prompt := codeReviewSentinel + `
### only.go (modified)
@@ -1,1 +1,2 @@
 package main
+var x = 1
`
	parsed := parseMockReview(t, codeReviewResponse(prompt))
	if len(parsed.Findings) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(parsed.Findings))
	}
	if parsed.Findings[0].File != "only.go" || parsed.Findings[1].File != "only.go" {
		t.Fatalf("both valid findings should use the only available file: %+v", parsed.Findings)
	}
}

func TestCodeReviewResponse_NoFilesYieldsCleanReview(t *testing.T) {
	parsed := parseMockReview(t, codeReviewResponse(codeReviewSentinel+"\n\nno diff sections here"))
	if len(parsed.Findings) != 0 {
		t.Fatalf("expected an empty findings array, got %+v", parsed.Findings)
	}
}

func TestFirstAddedLine(t *testing.T) {
	tests := map[string]struct {
		section string
		want    int
	}{
		"added after context":     {"@@ -1,2 +1,3 @@\n keep\n+added\n", 2},
		"added first":             {"@@ -0,0 +1,1 @@\n+added\n", 1},
		"deletion does not count": {"@@ -1,2 +1,1 @@\n keep\n-gone\n+new\n", 2},
		"no hunk":                 {"nothing here", 0},
		"context only":            {"@@ -1,1 +5,1 @@\n keep\n", 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := firstAddedLine(tc.section); got != tc.want {
				t.Fatalf("firstAddedLine = %d, want %d", got, tc.want)
			}
		})
	}
}
