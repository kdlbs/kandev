package acp

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestTerminalManagerStripsAgentDebugSettingsFromChild(t *testing.T) {
	if os.Getenv("KANDEV_TERMINAL_CHILD") == "1" {
		fmt.Printf("messages=%q logdir=%q maxfiles=%q safe=%q",
			os.Getenv("KANDEV_DEBUG_AGENT_MESSAGES"),
			os.Getenv("KANDEV_DEBUG_LOG_DIR"),
			os.Getenv("KANDEV_DEBUG_ACP_MAX_FILES"),
			os.Getenv("KANDEV_TERMINAL_SAFE"))
		return
	}
	t.Setenv("KANDEV_DEBUG_AGENT_MESSAGES", "true")
	t.Setenv("KANDEV_DEBUG_ACP_MAX_FILES", "23")

	manager := NewTerminalManager(zap.NewNop())
	id, err := manager.Create(os.Args[0], []string{"-test.run=^TestTerminalManagerStripsAgentDebugSettingsFromChild$"}, "", map[string]string{
		"KANDEV_TERMINAL_CHILD":      "1",
		"KANDEV_TERMINAL_SAFE":       "supplied",
		"KANDEV_DEBUG_LOG_DIR":       "/tmp/private-acp-logs",
		"KANDEV_DEBUG_ACP_MAX_FILES": "24",
	}, 4096)
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	code, _, err := manager.WaitForExit(id)
	if err != nil {
		t.Fatalf("wait for terminal: %v", err)
	}
	if code == nil || *code != 0 {
		t.Fatalf("terminal exit code = %v, want 0", code)
	}
	output, _, _, _, err := manager.Output(id)
	if err != nil {
		t.Fatalf("read terminal output: %v", err)
	}
	if !strings.Contains(output, `messages="" logdir="" maxfiles="" safe="supplied"`) {
		t.Fatalf("terminal environment = %q, want debug settings removed and safe override retained", output)
	}
}
