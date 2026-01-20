// Interactive test for CLI cancel functionality
// This provides a proper end-to-end test with real session/turn management
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	agentType = flag.String("agent", "auggie", "Agent type: auggie or codex")
	workDir   = flag.String("workdir", "/tmp", "Working directory")
	verbose   = flag.Bool("verbose", true, "Verbose output")
)

type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	flag.Parse()
	fmt.Printf("=== Interactive Cancel Test for %s ===\n\n", *agentType)

	switch *agentType {
	case "auggie":
		testAuggie()
	case "codex":
		testCodex()
	default:
		fmt.Printf("Unknown agent: %s\n", *agentType)
		os.Exit(1)
	}
}

func testAuggie() {
	cmd := exec.Command("auggie", "--acp", "--workspace-root", *workDir)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start: %v\n", err)
		return
	}
	defer cmd.Process.Kill()

	responses := make(chan JSONRPCMessage, 100)
	var wg sync.WaitGroup
	wg.Add(1)
	go readJSONRPCResponses(stdout, responses, &wg)

	// 1. Initialize
	log("Initializing...")
	send(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"test","version":"1.0"}}}`)
	waitForResponse(responses, 1)

	// 2. Create session
	log("Creating session...")
	send(stdin, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"%s","mcpServers":[]}}`, *workDir))
	resp := waitForResponse(responses, 2)

	// Extract session ID
	var sessionResult struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(resp.Result, &sessionResult)
	sessionID := sessionResult.SessionID
	log("Session ID: %s", sessionID)

	// Handle any permission requests (like indexing)
	go func() {
		for msg := range responses {
			if msg.Method == "session/request_permission" {
				log("Auto-approving permission request")
				var params struct {
					SessionID string `json:"sessionId"`
					ToolCall  struct {
						ToolCallID string `json:"toolCallId"`
					} `json:"toolCall"`
					Options []struct {
						OptionID string `json:"optionId"`
						Kind     string `json:"kind"`
					} `json:"options"`
				}
				json.Unmarshal(msg.Params, &params)
				// Find allow option
				optionID := "enable"
				for _, opt := range params.Options {
					if strings.Contains(opt.Kind, "allow") {
						optionID = opt.OptionID
						break
					}
				}
				respMsg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"outcome":{"selected":{"optionId":"%s"}}}}`, msg.ID, optionID)
				send(stdin, respMsg)
			}
		}
	}()

	// 3. Send prompt
	log("Sending prompt...")
	promptMsg := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"%s","prompt":[{"type":"text","text":"Count slowly from 1 to 10, saying each number"}]}}`, sessionID)
	send(stdin, promptMsg)

	// Wait a bit for the agent to start working
	time.Sleep(3 * time.Second)

	// 4. Cancel!
	log(">>> SENDING CANCEL <<<")
	cancelMsg := fmt.Sprintf(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"%s"}}`, sessionID)
	send(stdin, cancelMsg)

	// Wait for completion or timeout
	time.Sleep(3 * time.Second)
	log("Test complete")

	stdin.Close()
	cmd.Process.Signal(os.Interrupt)
}

func testCodex() {
	cmd := exec.Command("codex", "app-server")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start: %v\n", err)
		return
	}
	defer cmd.Process.Kill()

	responses := make(chan JSONRPCMessage, 100)
	var wg sync.WaitGroup
	wg.Add(1)
	go readJSONRPCResponses(stdout, responses, &wg)

	// 1. Initialize (Codex omits jsonrpc field)
	log("Initializing...")
	send(stdin, `{"id":1,"method":"initialize","params":{"clientInfo":{"name":"test","title":"Test","version":"1.0"}}}`)
	waitForResponse(responses, 1)
	send(stdin, `{"method":"initialized"}`)
	time.Sleep(500 * time.Millisecond)

	// 2. Start thread
	log("Starting thread...")
	send(stdin, fmt.Sprintf(`{"id":2,"method":"thread/start","params":{"cwd":"%s","approvalPolicy":"never"}}`, *workDir))
	resp := waitForResponse(responses, 2)

	var threadResult struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	json.Unmarshal(resp.Result, &threadResult)
	threadID := threadResult.Thread.ID
	log("Thread ID: %s", threadID)

	// 3. Start turn
	log("Starting turn...")
	turnMsg := fmt.Sprintf(`{"id":3,"method":"turn/start","params":{"threadId":"%s","input":[{"type":"text","text":"Count slowly from 1 to 10"}]}}`, threadID)
	send(stdin, turnMsg)
	turnResp := waitForResponse(responses, 3)

	var turnResult struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	json.Unmarshal(turnResp.Result, &turnResult)
	turnID := turnResult.Turn.ID
	log("Turn ID: %s", turnID)

	// Wait for agent to start working
	time.Sleep(3 * time.Second)

	// 4. Interrupt!
	log(">>> SENDING TURN/INTERRUPT <<<")
	interruptMsg := fmt.Sprintf(`{"id":4,"method":"turn/interrupt","params":{"threadId":"%s","turnId":"%s"}}`, threadID, turnID)
	send(stdin, interruptMsg)

	// Wait for response
	time.Sleep(3 * time.Second)
	log("Test complete")

	stdin.Close()
	cmd.Process.Signal(os.Interrupt)
}

// Helper functions

func send(w io.Writer, msg string) {
	if *verbose {
		fmt.Printf(">>> %s\n", msg)
	}
	w.Write([]byte(msg + "\n"))
}

func log(format string, args ...interface{}) {
	fmt.Printf("[TEST] "+format+"\n", args...)
}

func waitForResponse(ch chan JSONRPCMessage, id int) JSONRPCMessage {
	timeout := time.After(10 * time.Second)
	for {
		select {
		case msg := <-ch:
			if idNum, ok := msg.ID.(float64); ok && int(idNum) == id {
				return msg
			}
			// Put non-matching messages back (or just skip)
		case <-timeout:
			log("Timeout waiting for response %d", id)
			return JSONRPCMessage{}
		}
	}
}

func readJSONRPCResponses(r io.Reader, ch chan JSONRPCMessage, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(ch)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if *verbose {
			fmt.Printf("<<< %s\n", line)
		}
		var msg JSONRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			ch <- msg
		}
	}
}

