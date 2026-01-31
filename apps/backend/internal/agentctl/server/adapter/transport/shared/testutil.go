package shared

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCase represents a single JSONL test fixture line for normalization tests.
type TestCase struct {
	Key      string         `json:"key,omitempty"` // Dedup key for auto-capture
	Input    map[string]any `json:"input"`
	Expected map[string]any `json:"expected"`
}

// LoadTestCases loads test cases from a JSONL fixture file.
// The filename should be relative to the calling package's testdata directory.
func LoadTestCases(t *testing.T, filename string) []TestCase {
	t.Helper()

	path := filepath.Join("testdata", filename)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open test file %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	var cases []TestCase
	scanner := bufio.NewScanner(file)
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		var tc TestCase
		if err := json.Unmarshal(scanner.Bytes(), &tc); err != nil {
			t.Fatalf("failed to parse line %d in %s: %v", lineNum, filename, err)
		}
		cases = append(cases, tc)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading %s: %v", filename, err)
	}
	return cases
}
