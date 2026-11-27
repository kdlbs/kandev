package codexappserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestClientForkThreadUsesPinnedBoundaryAndReturnsNativeIdentity(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	serverDone := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(serverInput).ReadBytes('\n')
		if err != nil {
			serverDone <- err
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ThreadID   string `json:"threadId"`
				LastTurnID string `json:"lastTurnId"`
			} `json:"params"`
		}
		if err := json.Unmarshal(line, &request); err != nil {
			serverDone <- err
			return
		}
		if request.Method != MethodThreadFork || request.Params.ThreadID != "source-thread" || request.Params.LastTurnID != "completed-turn" {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		response, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  map[string]any{"thread": map[string]any{"id": "forked-thread", "forkedFromId": "source-thread"}},
		})
		if err == nil {
			_, err = serverOutput.Write(append(response, '\n'))
		}
		serverDone <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fork := reflect.ValueOf(client).MethodByName("ForkThread")
	if !fork.IsValid() {
		t.Fatal("Codex client does not expose the thread/fork operation")
	}
	results := fork.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(ThreadForkParams{ThreadID: "source-thread", LastTurnID: stringPtr("completed-turn")})})
	if len(results) != 2 {
		t.Fatalf("ForkThread returned %d values, want thread and error", len(results))
	}
	if !results[1].IsNil() {
		t.Fatalf("ForkThread: %v", results[1].Interface())
	}
	thread, ok := results[0].Interface().(*Thread)
	if !ok || thread == nil {
		t.Fatalf("ForkThread result = %#v, want *Thread", results[0].Interface())
	}
	if thread.ID != "forked-thread" || thread.ForkedFromID == nil || *thread.ForkedFromID != "source-thread" {
		t.Fatalf("forked thread = %#v", thread)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake server: %v", err)
	}
}

func stringPtr(value string) *string { return &value }
