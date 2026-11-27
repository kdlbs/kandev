package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// @covers AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
func TestCloudClient_GetTicket_NestedDescriptionParagraphs(t *testing.T) {
	issue := `{
		"key":"OPS-7","fields":{
			"summary":"Migration instructions",
			"description":{"version":1,"type":"doc","content":[
				{"type":"bulletList","content":[
					{"type":"listItem","content":[
						{"type":"paragraph","content":[{"type":"text","text":"Run migration."}]},
						{"type":"paragraph","content":[{"type":"text","text":"Verify the schema."}]}
					]},
					{"type":"listItem","content":[
						{"type":"paragraph","content":[{"type":"text","text":"Restart worker."}]}
					]}
				]}
			]},"status":{},"project":{},"issuetype":{}
		}
	}`
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/issue/OPS-7":
			_, _ = w.Write([]byte(issue))
		case "/rest/api/3/issue/OPS-7/transitions":
			_, _ = w.Write([]byte(`{"transitions":[]}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	ticket, err := clientTo(ts, AuthMethodAPIToken, "token").GetTicket(context.Background(), "OPS-7")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	const want = "Run migration.\n\nVerify the schema.\n\nRestart worker."
	if ticket.Description != want {
		t.Fatalf("description = %q, want %q", ticket.Description, want)
	}
}

// @covers AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
func TestCloudClient_SearchTicketsForWatch_NestedDescriptionParagraphs(t *testing.T) {
	search := `{
		"isLast":true,"issues":[{"key":"WATCH-9","fields":{
			"summary":"Watch migration",
			"status":{},"project":{},"issuetype":{},
			"description":{"type":"doc","version":1,"content":[
				{"type":"orderedList","attrs":{"order":1},"content":[
					{"type":"listItem","content":[
						{"type":"paragraph","content":[{"type":"text","text":"Run migration."}]}
					]},
					{"type":"listItem","content":[
						{"type":"paragraph","content":[{"type":"text","text":"Verify the schema."}]},
						{"type":"paragraph","content":[{"type":"text","text":"Restart worker."}]}
					]}
				]}
			]}
		}}]
	}`
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(search))
	})
	result, err := clientTo(ts, AuthMethodAPIToken, "token").SearchTicketsForWatch(
		context.Background(), "project = WATCH", "", 25,
	)
	if err != nil {
		t.Fatalf("watcher search: %v", err)
	}
	if len(result.Tickets) != 1 {
		t.Fatalf("ticket count = %d, want 1", len(result.Tickets))
	}
	const want = "Run migration.\n\nVerify the schema.\n\nRestart worker."
	if result.Tickets[0].Description != want {
		t.Fatalf("description = %q, want %q", result.Tickets[0].Description, want)
	}
}

// @covers AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
func TestExtractDescription_NestedParagraphs(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{
			name: "sibling list items",
			input: `{"type":"doc","content":[{"type":"bulletList","content":[
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"First."}]}]},
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Second."}]}]},
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Third."}]}]}
			]}]}`,
			want: "First.\n\nSecond.\n\nThird.",
		},
		{
			name: "nested list and outer continuation",
			input: `{"type":"doc","content":[{"type":"orderedList","content":[{"type":"listItem","content":[
				{"type":"paragraph","content":[{"type":"text","text":"Outer."}]},
				{"type":"bulletList","content":[{"type":"listItem","content":[
					{"type":"paragraph","content":[{"type":"text","text":"Nested."}]},
					{"type":"orderedList","content":[{"type":"listItem","content":[
						{"type":"paragraph","content":[{"type":"text","text":"Deeper."}]}
					]}]}
				]}]},
				{"type":"paragraph","content":[{"type":"text","text":"Continue."}]}
			]}]}]}`,
			want: "Outer.\n\nNested.\n\nDeeper.\n\nContinue.",
		},
		{
			name: "blockquote then document sibling",
			input: `{"type":"doc","content":[{"type":"blockquote","content":[
				{"type":"paragraph","content":[{"type":"text","text":"Quoted."}]},
				{"type":"paragraph","content":[{"type":"text","text":"Again."}]}
			]},{"type":"paragraph","content":[{"type":"text","text":"Outside."}]}]}`,
			want: "Quoted.\n\nAgain.\n\nOutside.",
		},
		{
			name: "unknown wrapper with ignored and empty children",
			input: `{"type":"doc","content":[{"type":"futureContainer","content":[
				{"type":"paragraph","content":[{"type":"text","text":"Before."}]},
				null,42,{"type":"paragraph"},{"type":"paragraph","content":[]},
				{"type":"paragraph","content":"wrong"},{"type":"text","text":false},
				{"type":"paragraph","content":[{"type":"text","text":"After."}]}
			]}]}`,
			want: "Before.\n\nAfter.",
		},
		{
			name: "nested explicit and literal breaks",
			input: `{"type":"doc","content":[{"type":"blockquote","content":[
				{"type":"paragraph","content":[{"type":"text","text":"First."},{"type":"hardBreak"}]},
				{"type":"paragraph","content":[{"type":"text","text":"Second.\n\n"},{"type":"softBreak"},{"type":"text","text":"Still second."}]}
			]}]}`,
			want: "First.\n\n\nSecond.\n\n\nStill second.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertADFDescription(t, tt.input, tt.want)
		})
	}
}

// @covers AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
// @covers AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.4
func TestExtractDescription_Compatibility(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"plain string whitespace", `"  plain\n  "`, "  plain\n  "},
		{"null", `null`, ""},
		{"unsupported scalar", `42`, ""},
		{"absent fields", `{}`, ""},
		{"empty document", `{"type":"doc","content":[]}`, ""},
		{"empty list", `{"type":"doc","content":[{"type":"bulletList","content":[]}]}`, ""},
		{"empty paragraph", `{"type":"doc","content":[{"type":"paragraph","content":[]}]}`, ""},
		{
			name: "marked inline adjacency",
			input: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"Run "},
				{"type":"text","text":"migration.","marks":[{"type":"strong"}]}
			]}]}`,
			want: "Run migration.",
		},
		{
			name: "explicit breaks and literal whitespace",
			input: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"  First  part."},{"type":"hardBreak"},
				{"type":"text","text":"Second."},{"type":"softBreak"},
				{"type":"text","text":"Third.\n\nFourth.  "}
			]}]}`,
			want: "First  part.\nSecond.\nThird.\n\nFourth.",
		},
		{
			name: "top level paragraphs",
			input: `{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"First."}]},
				{"type":"paragraph","content":[{"type":"text","text":"Second."}]}
			]}`,
			want: "First.\n\nSecond.",
		},
		{
			name: "top level explicit trailing break",
			input: `{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"First."},{"type":"hardBreak"}]},
				{"type":"paragraph","content":[{"type":"text","text":"Second."}]}
			]}`,
			want: "First.\n\n\nSecond.",
		},
		{
			name: "top level empty paragraph",
			input: `{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"First."}]},
				{"type":"paragraph","content":[]},
				{"type":"paragraph","content":[{"type":"text","text":"Second."}]}
			]}`,
			want: "First.\n\n\n\nSecond.",
		},
		{
			name: "ignored top level child",
			input: `{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"First."}]},
				42,{"type":"paragraph","content":[{"type":"text","text":"Second."}]}
			]}`,
			want: "First.\n\n\n\nSecond.",
		},
		{
			name: "unknown inline wrapper and malformed leaves",
			input: `{"type":"doc","content":[{"type":"unknown","content":[
				{"type":"text","text":"One"},{"type":"mention","attrs":{"text":"@person"}},
				{"type":"media"},null,{},false,{"type":"text","text":7},
				{"type":"unknown","content":false},{"type":"text","text":"Two"}
			]}]}`,
			want: "OneTwo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertADFDescription(t, tt.input, tt.want)
		})
	}
}

func assertADFDescription(t *testing.T, input, want string) {
	t.Helper()
	var raw interface{}
	if err := json.NewDecoder(strings.NewReader(input)).Decode(&raw); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if got := extractDescription(raw); got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}
