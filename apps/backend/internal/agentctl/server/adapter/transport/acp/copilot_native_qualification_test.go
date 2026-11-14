package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestCopilotNativeQualificationFixtures pins the sanitized wire shapes used
// for qualification. The frames come from the issue #5089 capture on Copilot
// CLI 1.0.103; prompt, summary, billing, and tracing fields are removed.
func TestCopilotNativeQualificationFixtures(t *testing.T) {
	read := func(name string) json.RawMessage {
		t.Helper()
		encoded, err := os.ReadFile(filepath.Join("testdata", "copilot-native", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var frame struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(encoded, &frame); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if frame.Method != copilotSessionEventMethod {
			t.Fatalf("%s method = %q, want %q", name, frame.Method, copilotSessionEventMethod)
		}
		return frame.Params
	}
	decode := func(name string) decodedCopilotNativeEvent {
		t.Helper()
		event, ok := decodeCopilotNativeEvent(copilotSessionEventMethod, read(name))
		if !ok {
			t.Fatalf("decodeCopilotNativeEvent rejected %s", name)
		}
		return event
	}

	start := decode("compaction_start.json")
	if start.Type != copilotNativeTypeCompactionStart || !start.Root {
		t.Fatalf("start fixture = %+v", start)
	}
	var startData copilotNativeCompactionData
	if err := json.Unmarshal(start.Data, &startData); err != nil || startData.CompactionAttemptID == "" {
		t.Fatalf("start attempt identity: %+v %v", startData, err)
	}

	manual := decode("compaction_complete.json")
	observation, partial := copilotCompactionCompletion(manual)
	if partial || observation == nil || !observation.ValidCompletion() {
		t.Fatalf("manual completion = %+v partial=%v", observation, partial)
	}
	if observation.Trigger != "manual" {
		t.Fatalf("manual trigger = %q", observation.Trigger)
	}

	automatic := decode("compaction_complete_automatic.json")
	observation, partial = copilotCompactionCompletion(automatic)
	if partial || observation == nil || observation.Trigger != "threshold" {
		t.Fatalf("automatic completion = %+v partial=%v", observation, partial)
	}

	child := decode("compaction_complete_child.json")
	if child.Root || !child.Child {
		t.Fatalf("child fixture attribution = %+v", child)
	}
	if observation, partial := copilotCompactionCompletion(child); observation != nil || partial {
		t.Fatalf("child completion must not count: %+v partial=%v", observation, partial)
	}

	failure := decode("compaction_complete_failure.json")
	if observation, partial := copilotCompactionCompletion(failure); observation != nil || partial {
		t.Fatalf("failed completion must be known non-countable evidence: %+v %v", observation, partial)
	}

	omitted := decode("compaction_complete_omitted.json")
	if !omitted.DataOmitted {
		t.Fatalf("omitted fixture did not mark data omission: %+v", omitted)
	}
	if observation, partial := copilotCompactionCompletion(omitted); observation != nil || !partial {
		t.Fatalf("omitted completion must be partial, not countable: %+v %v", observation, partial)
	}
	if reporting := copilotOmittedCompletionReporting(omitted); reporting == nil ||
		reporting.Coverage != streams.ContextCompactionCoveragePartial {
		t.Fatalf("omitted completion reporting = %+v", reporting)
	}

	rootUsage := decode("usage_root.json")
	if used, size, ok := copilotRootUsage(rootUsage); !ok || used != 13210 || size != 272000 {
		t.Fatalf("root usage = (%d, %d, %v)", used, size, ok)
	}
	childUsage := decode("usage_child.json")
	if _, _, ok := copilotRootUsage(childUsage); ok {
		t.Fatal("child usage must not become the parent gauge")
	}
}

func TestCopilotRuntimeQualified(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{version: "1.0.103", want: true},
		{version: "GitHub Copilot CLI 1.0.103.", want: true},
		{version: "1.0.104", want: true},
		{version: "1.0.103-0", want: true},
		{version: "2.0.0", want: true},
		{version: "1.0.88", want: false},
		{version: "1.0.102", want: false},
		{version: "0.9.999", want: false},
		{version: "", want: false},
		{version: "unknown", want: false},
	}
	for _, tc := range cases {
		if got := copilotRuntimeQualified(tc.version); got != tc.want {
			t.Errorf("copilotRuntimeQualified(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
