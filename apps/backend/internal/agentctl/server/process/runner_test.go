package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestRingBufferTrimsOldest(t *testing.T) {
	buffer := newRingBuffer(10)
	buffer.append(ProcessOutputChunk{Stream: "stdout", Data: "hello", Timestamp: time.Now()}) // 5
	buffer.append(ProcessOutputChunk{Stream: "stdout", Data: "world", Timestamp: time.Now()}) // 5 (total 10)
	buffer.append(ProcessOutputChunk{Stream: "stderr", Data: "!!!", Timestamp: time.Now()})   // +3 -> trim

	snapshot := buffer.snapshot()
	if len(snapshot) == 0 {
		t.Fatal("expected buffered output")
	}
	combined := ""
	for _, chunk := range snapshot {
		combined += chunk.Data
	}
	if strings.Contains(combined, "hello") {
		t.Fatalf("expected oldest chunk to be trimmed, got %q", combined)
	}
	if !strings.Contains(combined, "world") {
		t.Fatalf("expected newer chunk to remain, got %q", combined)
	}
}

func TestProcessRunnerCapturesOutput(t *testing.T) {
	log := newTestLogger(t)
	runner := NewProcessRunner(nil, log, 2*1024*1024)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := runner.Start(ctx, StartProcessRequest{
		SessionID:  "session-1",
		Kind:       "dev",
		Command:    "printf 'hello'; sleep 2",
		WorkingDir: "",
	})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		proc, ok := runner.Get(info.ID, true)
		if !ok {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		combined := ""
		for _, chunk := range proc.Output {
			combined += chunk.Data
		}
		if strings.Contains(combined, "hello") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("process output not captured in time")
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple color code",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "bold text",
			input:    "\x1b[1mbold\x1b[0m",
			expected: "bold",
		},
		{
			name:     "multiple colors",
			input:    "\x1b[31mred\x1b[32mgreen\x1b[34mblue\x1b[0m",
			expected: "redgreenblue",
		},
		{
			name:     "256 color code",
			input:    "\x1b[38;5;196mcolored\x1b[0m",
			expected: "colored",
		},
		{
			name:     "RGB color code",
			input:    "\x1b[38;2;255;0;0mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2Amove up\x1b[3Bmove down",
			expected: "move upmove down",
		},
		{
			name:     "clear line",
			input:    "text\x1b[2Kcleared",
			expected: "textcleared",
		},
		{
			name:     "real world npm output",
			input:    "\x1b[32m✓\x1b[39m \x1b[90mCompiled successfully\x1b[39m",
			expected: "✓ Compiled successfully",
		},
		{
			name:     "mixed with newlines",
			input:    "\x1b[31mError:\x1b[0m\nSomething failed\n\x1b[33mWarning:\x1b[0m check logs",
			expected: "Error:\nSomething failed\nWarning: check logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsNpmEnvVar(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Should filter
		{"npm_config_registry", true},
		{"npm_config_npm-globalconfig", true},
		{"npm_config__jsr-registry", true},
		{"npm_config_verify-deps-before-run", true},
		{"npm_config_dir", true},
		{"npm_package_name", true},
		{"npm_package_version", true},
		{"npm_lifecycle_event", true},
		{"npm_execpath", true},
		{"npm_node_execpath", true},

		// Should keep
		{"PATH", false},
		{"HOME", false},
		{"USER", false},
		{"SHELL", false},
		{"ANTHROPIC_API_KEY", false},
		{"NPM_TOKEN", false},        // Uppercase, different format
		{"NPMRC", false},            // Not npm_ prefix
		{"npm_not_a_config", false}, // Doesn't match any prefix
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isNpmEnvVar(tt.key)
			if result != tt.expected {
				t.Errorf("isNpmEnvVar(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestMergeEnv_FiltersNpmVars(t *testing.T) {
	// Set some test npm env vars that should be filtered
	t.Setenv("npm_config_test_var", "should_be_filtered")
	t.Setenv("npm_package_name", "test-package")

	// Set a normal var that should be kept
	t.Setenv("TEST_NORMAL_VAR", "should_be_kept")

	result := mergeEnv(map[string]string{
		"CUSTOM_VAR": "custom_value",
	})

	// Convert to map for easier checking
	resultMap := make(map[string]string)
	for _, entry := range result {
		if eq := strings.IndexByte(entry, '='); eq >= 0 {
			resultMap[entry[:eq]] = entry[eq+1:]
		}
	}

	// Check that npm vars are filtered
	if _, exists := resultMap["npm_config_test_var"]; exists {
		t.Error("npm_config_test_var should have been filtered")
	}
	if _, exists := resultMap["npm_package_name"]; exists {
		t.Error("npm_package_name should have been filtered")
	}

	// Check that normal vars are kept
	if resultMap["TEST_NORMAL_VAR"] != "should_be_kept" {
		t.Error("TEST_NORMAL_VAR should have been kept")
	}

	// Check that custom vars are added
	if resultMap["CUSTOM_VAR"] != "custom_value" {
		t.Error("CUSTOM_VAR should have been added")
	}
}
