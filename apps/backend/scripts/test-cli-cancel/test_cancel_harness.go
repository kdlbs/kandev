// Test harness for testing CLI cancel functionality in isolation
// Usage: go run test_cancel_harness.go -agent=auggie|codex
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

var agentType = flag.String("agent", "auggie", "Agent type: auggie or codex")
var workDir = flag.String("workdir", ".", "Working directory")

func main() {
	flag.Parse()

	fmt.Printf("Testing %s CLI cancel functionality\n", *agentType)
	fmt.Printf("Working directory: %s\n\n", *workDir)

	switch *agentType {
	case "auggie":
		testAuggieCancel()
	case "codex":
		testCodexCancel()
	default:
		fmt.Printf("Unknown agent type: %s\n", *agentType)
		os.Exit(1)
	}
}

func testAuggieCancel() {
	fmt.Println("=== Testing Auggie (ACP Protocol) ===")
	fmt.Println("Protocol: JSON-RPC 2.0 over stdin/stdout")
	fmt.Println("Cancel Method: session/cancel (notification)")
	fmt.Println()

	// Start auggie in ACP mode
	cmd := exec.Command("auggie", "--acp", "--workspace-root", *workDir)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start auggie: %v\n", err)
		return
	}
	defer cmd.Process.Kill()

	// Read responses in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		readResponses(stdout, "auggie")
	}()

	// 1. Initialize
	fmt.Println("\n1. Sending initialize request...")
	sendJSONRPC(stdin, 1, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo":      map[string]string{"name": "test-harness", "version": "1.0"},
	})
	time.Sleep(2 * time.Second)

	// 2. Create session
	fmt.Println("\n2. Creating new session...")
	sendJSONRPC(stdin, 2, "session/new", map[string]any{
		"cwd":        *workDir,
		"mcpServers": []any{},
	})
	time.Sleep(2 * time.Second)

	// 3. Send prompt (will start a turn)
	fmt.Println("\n3. Sending prompt...")
	sendJSONRPC(stdin, 3, "session/prompt", map[string]any{
		"sessionId": "test-session", // Will be populated from session/new response
		"prompt":    []map[string]string{{"type": "text", "text": "Count from 1 to 100 slowly"}},
	})
	time.Sleep(3 * time.Second)

	// 4. Cancel the session
	fmt.Println("\n4. Sending session/cancel notification...")
	sendJSONRPCNotification(stdin, "session/cancel", map[string]string{
		"sessionId": "test-session",
	})
	time.Sleep(2 * time.Second)

	stdin.Close()
	cmd.Process.Signal(os.Interrupt)
	time.Sleep(1 * time.Second)
	fmt.Println("\n=== Auggie test complete ===")
}

func testCodexCancel() {
	fmt.Println("=== Testing Codex (App-Server Protocol) ===")
	fmt.Println("Protocol: JSON-RPC 2.0 variant (no jsonrpc field)")
	fmt.Println("Cancel Method: turn/interrupt (request)")
	fmt.Println()

	// Start codex in app-server mode
	cmd := exec.Command("codex", "app-server")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start codex: %v\n", err)
		return
	}
	defer cmd.Process.Kill()

	// Read responses
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		readResponses(stdout, "codex")
	}()

	// 1. Initialize (Codex style - no jsonrpc field)
	fmt.Println("\n1. Sending initialize request...")
	sendCodexRPC(stdin, 1, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "test-harness", "title": "Test", "version": "1.0"},
	})
	time.Sleep(2 * time.Second)

	// Send initialized notification
	sendCodexNotification(stdin, "initialized", nil)
	time.Sleep(1 * time.Second)

	// 2. Start thread
	fmt.Println("\n2. Starting thread...")
	sendCodexRPC(stdin, 2, "thread/start", map[string]any{
		"cwd":            *workDir,
		"approvalPolicy": "never",
	})
	time.Sleep(2 * time.Second)

	// 3. Start turn
	fmt.Println("\n3. Starting turn...")
	sendCodexRPC(stdin, 3, "turn/start", map[string]any{
		"threadId": "test-thread", // Will be from thread/start
		"input":    []map[string]string{{"type": "text", "text": "Count from 1 to 100 slowly"}},
	})
	time.Sleep(3 * time.Second)

	// 4. Interrupt turn
	fmt.Println("\n4. Sending turn/interrupt...")
	sendCodexRPC(stdin, 4, "turn/interrupt", map[string]string{
		"threadId": "test-thread",
		"turnId":   "test-turn",
	})
	time.Sleep(2 * time.Second)

	stdin.Close()
	cmd.Process.Signal(os.Interrupt)
	time.Sleep(1 * time.Second)
	fmt.Println("\n=== Codex test complete ===")
}

// Helper functions

func sendJSONRPC(w io.Writer, id int, method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(msg)
	fmt.Printf(">>> %s\n", string(data))
	w.Write(data)
	w.Write([]byte("\n"))
}

func sendJSONRPCNotification(w io.Writer, method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(msg)
	fmt.Printf(">>> %s\n", string(data))
	w.Write(data)
	w.Write([]byte("\n"))
}

func sendCodexRPC(w io.Writer, id int, method string, params any) {
	msg := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	data, _ := json.Marshal(msg)
	fmt.Printf(">>> %s\n", string(data))
	w.Write(data)
	w.Write([]byte("\n"))
}

func sendCodexNotification(w io.Writer, method string, params any) {
	msg := map[string]any{
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}
	data, _ := json.Marshal(msg)
	fmt.Printf(">>> %s\n", string(data))
	w.Write(data)
	w.Write([]byte("\n"))
}

func readResponses(r io.Reader, agentName string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			fmt.Printf("<<< [%s] %s\n", agentName, line)
		}
	}
}

// Verify compilation
var _ = context.Background

