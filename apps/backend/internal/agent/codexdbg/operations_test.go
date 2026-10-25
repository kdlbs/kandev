package codexdbg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/codexappserver"
)

func TestProbeNoTurn(t *testing.T) {
	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	client := codexappserver.NewClient(clientOutput, clientInput, codexappserver.Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverInput.Close()
		_ = serverOutput.Close()
	})

	methods := make(chan []string, 1)
	go func() {
		reader := bufio.NewReader(serverInput)
		seen := make([]string, 0, 3)
		for len(seen) < 4 {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				methods <- seen
				return
			}
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(line, &request) != nil {
				methods <- seen
				return
			}
			seen = append(seen, request.Method)
			if len(request.ID) == 0 {
				continue
			}
			result := any(map[string]any{"userAgent": "codex-cli test", "codexHome": "/tmp/codex", "platformOs": "linux", "platformFamily": "unix"})
			switch request.Method {
			case codexappserver.MethodModelList:
				result = map[string]any{"data": []map[string]any{{"id": "gpt-test", "model": "gpt-test", "displayName": "Test model"}}, "nextCursor": nil}
			case codexappserver.MethodExperimentalFeatureList:
				result = map[string]any{"data": []any{}, "nextCursor": nil}
			}
			response, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			if _, err := serverOutput.Write(append(response, '\n')); err != nil {
				methods <- seen
				return
			}
		}
		methods <- seen
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := Probe(ctx, client)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(result.Models) != 1 || result.Models[0].ID != "gpt-test" {
		t.Fatalf("probe models = %#v", result.Models)
	}
	select {
	case got := <-methods:
		for _, method := range got {
			if method == codexappserver.MethodThreadStart || method == codexappserver.MethodTurnStart {
				t.Fatalf("probe created a conversation or turn: %v", got)
			}
		}
	case <-ctx.Done():
		t.Fatal("fake app-server did not finish the probe")
	}
}

func TestWaitForTurnWithoutInterruptDelayDoesNotSendInterrupt(t *testing.T) {
	var sent bytes.Buffer
	clientOutput, serverOutput := io.Pipe()
	client := codexappserver.NewClient(&sent, clientOutput, codexappserver.Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = clientOutput.Close()
		_ = serverOutput.Close()
	})
	events := make(chan appNotification, 1)
	events <- appNotification{method: codexappserver.NotificationTurnComplete, params: json.RawMessage(`{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}}`)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := waitForTurn(ctx, client, "thread-1", "turn-1", nil, 0, events, make(chan struct{}))
	if err != nil {
		t.Fatalf("wait for completed turn: %v", err)
	}
	if !result.CompletionObserved || result.InterruptRequested {
		t.Fatalf("turn result = %#v", result)
	}
	if sent.Len() != 0 {
		t.Fatalf("unexpected app-server request: %s", sent.String())
	}
}
