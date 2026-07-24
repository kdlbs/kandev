package review

import (
	"errors"
	"strings"
	"testing"
)

const validEnvelope = `{
  "summary": "Two issues found.",
  "findings": [
    {"file": "a.go", "line": 12, "line_end": 14, "severity": "blocker",
     "category": "correctness", "title": "Nil deref", "body": "x may be nil."},
    {"file": "b.ts", "line": 3, "severity": "nit",
     "category": "style", "title": "Naming", "body": "Prefer camelCase."}
  ]
}`

func TestParseFindings_FencedJSONBlock(t *testing.T) {
	response := "Here is my review:\n\n```json\n" + validEnvelope + "\n```\n\nHope that helps!"

	got, err := ParseFindings(response)
	if err != nil {
		t.Fatalf("ParseFindings: %v", err)
	}
	if got.Summary != "Two issues found." {
		t.Fatalf("summary mismatch: %q", got.Summary)
	}
	if len(got.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got.Findings))
	}
	if got.Findings[0].LineEnd != 14 || got.Findings[1].LineEnd != 3 {
		t.Fatalf("line_end defaulting wrong: %+v", got.Findings)
	}
	if got.Rejected != 0 {
		t.Fatalf("expected nothing rejected, got %d", got.Rejected)
	}
}

func TestParseFindings_UnlabelledFenceAndBareObject(t *testing.T) {
	for name, response := range map[string]string{
		"bare fence":  "```\n" + validEnvelope + "\n```",
		"no fence":    validEnvelope,
		"prose wrap":  "I reviewed the diff.\n" + validEnvelope + "\nThat is all.",
		"open fence":  "```json\n" + validEnvelope,
		"crlf output": strings.ReplaceAll("```json\n"+validEnvelope+"\n```", "\n", "\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseFindings(response)
			if err != nil {
				t.Fatalf("ParseFindings: %v", err)
			}
			if len(got.Findings) != 2 {
				t.Fatalf("expected 2 findings, got %d", len(got.Findings))
			}
		})
	}
}

func TestParseFindings_CleanReviewIsSuccess(t *testing.T) {
	got, err := ParseFindings("```json\n{\"summary\":\"Looks good.\",\"findings\":[]}\n```")
	if err != nil {
		t.Fatalf("an empty findings array is a valid clean review, got %v", err)
	}
	if len(got.Findings) != 0 || got.Summary != "Looks good." {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestParseFindings_MalformedEntryIsCountedNotFatal(t *testing.T) {
	response := `{"summary":"","findings":[
		{"file":"a.go","line":5,"severity":"major","category":"bug","title":"Real","body":"ok"},
		{"line":9,"severity":"major","category":"bug","title":"No file","body":"ok"},
		{"file":"c.go","line":0,"severity":"major","category":"bug","title":"Bad line","body":"ok"},
		{"file":"d.go","line":4,"severity":"catastrophic","category":"bug","title":"Bad sev","body":"ok"},
		{"file":"e.go","line":4,"severity":"minor","category":"bug","title":"","body":"ok"},
		{"file":"f.go","line":9,"line_end":2,"severity":"minor","category":"bug","title":"Backwards","body":"ok"}
	]}`

	got, err := ParseFindings(response)
	if err != nil {
		t.Fatalf("ParseFindings: %v", err)
	}
	if len(got.Findings) != 1 || got.Findings[0].File != "a.go" {
		t.Fatalf("expected only the valid entry, got %+v", got.Findings)
	}
	if got.Rejected != 5 {
		t.Fatalf("expected 5 rejected entries, got %d", got.Rejected)
	}
}

func TestParseFindings_UnparseableResponses(t *testing.T) {
	for name, response := range map[string]string{
		"empty":           "",
		"prose only":      "The code looks fine to me, no issues.",
		"broken json":     "```json\n{\"findings\": [ {\"file\": \n```",
		"unrelated json":  `{"result": "ok", "count": 3}`,
		"json array only": `[{"file":"a.go"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFindings(response); !errors.Is(err, ErrUnparseableResponse) {
				t.Fatalf("expected ErrUnparseableResponse, got %v", err)
			}
		})
	}
}

func TestParseFindings_PrefersFencedBlockOverSurroundingBraces(t *testing.T) {
	// A reply that mentions a JSON-looking example in prose before the real
	// block must still parse the real block.
	response := "Consider {not: json} then:\n```json\n" + validEnvelope + "\n```"

	got, err := ParseFindings(response)
	if err != nil {
		t.Fatalf("ParseFindings: %v", err)
	}
	if len(got.Findings) != 2 {
		t.Fatalf("expected the fenced block to win, got %d findings", len(got.Findings))
	}
}

func TestNormalizeFindingInput_Defaults(t *testing.T) {
	got, err := NormalizeFindingInput(FindingInput{
		File:     "  pkg/a.go  ",
		Line:     7,
		Severity: "  BLOCKER ",
		Category: "  Race Condition!! ",
		Title:    "  A very\nmulti-line   title  ",
		Body:     "  body  ",
	})
	if err != nil {
		t.Fatalf("NormalizeFindingInput: %v", err)
	}
	if got.File != "pkg/a.go" {
		t.Fatalf("file not trimmed: %q", got.File)
	}
	if got.LineEnd != 7 {
		t.Fatalf("line_end should default to line, got %d", got.LineEnd)
	}
	if got.Severity != "blocker" {
		t.Fatalf("severity should be lowercased, got %q", got.Severity)
	}
	if got.Category != "race-condition" {
		t.Fatalf("category should be kebab-cased, got %q", got.Category)
	}
	if got.Title != "A very multi-line title" {
		t.Fatalf("title should be flattened, got %q", got.Title)
	}
	if got.Side != "additions" {
		t.Fatalf("side should default to additions, got %q", got.Side)
	}
}

func TestNormalizeFindingInput_KeepsDeletionsSide(t *testing.T) {
	got, err := NormalizeFindingInput(FindingInput{
		File: "a.go", Line: 1, Severity: "minor", Title: "t", Body: "b", Side: "deletions",
	})
	if err != nil {
		t.Fatalf("NormalizeFindingInput: %v", err)
	}
	if got.Side != "deletions" {
		t.Fatalf("expected deletions preserved, got %q", got.Side)
	}
}

func TestNormalizeFindingInput_CapsLongFields(t *testing.T) {
	got, err := NormalizeFindingInput(FindingInput{
		File:     "a.go",
		Line:     1,
		Severity: "minor",
		Category: strings.Repeat("c", 80),
		Title:    strings.Repeat("t", 300),
		Body:     "b",
	})
	if err != nil {
		t.Fatalf("NormalizeFindingInput: %v", err)
	}
	if len([]rune(got.Title)) != maxTitleRunes {
		t.Fatalf("title should be capped at %d runes, got %d", maxTitleRunes, len([]rune(got.Title)))
	}
	if len([]rune(got.Category)) != maxCategoryRunes {
		t.Fatalf("category should be capped at %d runes, got %d", maxCategoryRunes, len([]rune(got.Category)))
	}
}

func TestNormalizeFindingInput_Rejections(t *testing.T) {
	base := FindingInput{File: "a.go", Line: 1, Severity: "minor", Title: "t", Body: "b"}
	mutate := map[string]func(FindingInput) FindingInput{
		"missing file":     func(f FindingInput) FindingInput { f.File = "  "; return f },
		"zero line":        func(f FindingInput) FindingInput { f.Line = 0; return f },
		"negative line":    func(f FindingInput) FindingInput { f.Line = -3; return f },
		"bad severity":     func(f FindingInput) FindingInput { f.Severity = "urgent"; return f },
		"missing severity": func(f FindingInput) FindingInput { f.Severity = ""; return f },
		"missing title":    func(f FindingInput) FindingInput { f.Title = " "; return f },
		"missing body":     func(f FindingInput) FindingInput { f.Body = " "; return f },
		"backwards range":  func(f FindingInput) FindingInput { f.Line = 9; f.LineEnd = 4; return f },
	}
	for name, m := range mutate {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeFindingInput(m(base)); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestTruncateAnchorText(t *testing.T) {
	short := "line one\nline two"
	if TruncateAnchorText(short) != short {
		t.Fatal("short anchor text must be preserved verbatim")
	}
	long := strings.Repeat("é", maxAnchorTextBytes)
	got := TruncateAnchorText(long)
	if len(got) > maxAnchorTextBytes {
		t.Fatalf("expected cap at %d bytes, got %d", maxAnchorTextBytes, len(got))
	}
	if !utf8Valid(got) {
		t.Fatal("truncation must cut on a rune boundary")
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
