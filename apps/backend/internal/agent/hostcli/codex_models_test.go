package hostcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// scriptedProcess answers JSON-RPC requests from stdin with canned responses.
type scriptedProcess struct {
	stdinReader  *io.PipeReader
	stdinWriter  *io.PipeWriter
	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter
	killed       chan struct{}
	requests     []string
}

func newScriptedProcess(handle func(method string, id *int) (string, bool)) *scriptedProcess {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	p := &scriptedProcess{
		stdinReader: stdinReader, stdinWriter: stdinWriter,
		stdoutReader: stdoutReader, stdoutWriter: stdoutWriter,
		killed: make(chan struct{}),
	}
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			line := scanner.Text()
			p.requests = append(p.requests, line)
			var req struct {
				ID     *int   `json:"id"`
				Method string `json:"method"`
			}
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}
			if response, ok := handle(req.Method, req.ID); ok {
				_, _ = io.WriteString(p.stdoutWriter, response+"\n")
			}
		}
	}()
	return p
}

func (p *scriptedProcess) Stdin() io.Writer  { return p.stdinWriter }
func (p *scriptedProcess) Stdout() io.Reader { return p.stdoutReader }
func (p *scriptedProcess) Wait() error       { <-p.killed; return nil }
func (p *scriptedProcess) Kill() error {
	select {
	case <-p.killed:
	default:
		close(p.killed)
		_ = p.stdinWriter.Close()
		_ = p.stdoutWriter.Close()
	}
	return nil
}

const sampleModelList = `{"id":2,"result":{"data":[
{"id":"gpt-6-astra","model":"gpt-6-astra","displayName":"GPT-6-Astra","description":"Frontier","hidden":false,"isDefault":true,
 "defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"l"},{"reasoningEffort":"high","description":"h"}]},
{"id":"gpt-6-sol","model":"gpt-6-sol","displayName":"GPT-6-Sol","description":"Workhorse","hidden":false,"isDefault":false},
{"id":"secret","model":"secret","displayName":"Secret","hidden":true,"isDefault":false}
],"nextCursor":null}}`

func codexLikeHandler(t *testing.T, listResponse string) func(method string, id *int) (string, bool) {
	t.Helper()
	return func(method string, id *int) (string, bool) {
		switch method {
		case "initialize":
			return `{"id":1,"result":{"userAgent":"codex"}}` + "\n" +
				`{"method":"remoteControl/status/changed","params":{"status":"disabled"}}`, true
		case "initialized":
			return "", false
		case "model/list":
			return strings.ReplaceAll(listResponse, "\n", ""), true
		}
		t.Fatalf("unexpected method %s", method)
		return "", false
	}
}

// @covers AC-AGENTS-HOST-CLI-002.1
func TestListCodexModels(t *testing.T) {
	var proc *scriptedProcess
	runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) {
		proc = newScriptedProcess(codexLikeHandler(t, sampleModelList))
		return proc, nil
	}}
	models, err := ListCodexModels(context.Background(), runner, "/usr/bin/codex")
	if err != nil {
		t.Fatalf("ListCodexModels: %v", err)
	}
	if strings.Join(runner.lastArgv, " ") != "/usr/bin/codex app-server" {
		t.Fatalf("argv = %v", runner.lastArgv)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v, want 2 visible entries", models)
	}
	if models[0].ID != "gpt-6-astra" || models[0].Name != "GPT-6-Astra" || !models[0].IsDefault {
		t.Fatalf("first model = %+v", models[0])
	}
	efforts, _ := models[0].Meta["reasoningEfforts"].([]string)
	if strings.Join(efforts, ",") != "low,high" || models[0].Meta["defaultReasoningEffort"] != "medium" {
		t.Fatalf("meta = %+v", models[0].Meta)
	}
	if models[1].Meta != nil {
		t.Fatalf("second model meta = %+v, want nil", models[1].Meta)
	}
	select {
	case <-proc.killed:
	case <-time.After(time.Second):
		t.Fatal("process was not terminated after the response")
	}
	// The sequence must be initialize, initialized, model/list.
	if len(proc.requests) != 3 || !strings.Contains(proc.requests[0], `"initialize"`) ||
		!strings.Contains(proc.requests[1], `"initialized"`) || !strings.Contains(proc.requests[2], `"model/list"`) {
		t.Fatalf("requests = %v", proc.requests)
	}
}

// @covers AC-AGENTS-HOST-CLI-002.5
func TestListCodexModelsFailures(t *testing.T) {
	t.Run("not installed", func(t *testing.T) {
		runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) { return nil, ErrNotInstalled }}
		if _, err := ListCodexModels(context.Background(), runner, "/nope/codex"); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("empty path", func(t *testing.T) {
		if _, err := ListCodexModels(context.Background(), &fakeRunner{}, ""); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) {
			return newScriptedProcess(codexLikeHandler(t, `{"id":2,"error":{"code":-32000,"message":"Not logged in. Run codex login."}}`)), nil
		}}
		_, err := ListCodexModels(context.Background(), runner, "/usr/bin/codex")
		if !errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("err = %v, want ErrNotLoggedIn", err)
		}
	})
	t.Run("rpc error", func(t *testing.T) {
		runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) {
			return newScriptedProcess(codexLikeHandler(t, `{"id":2,"error":{"code":-32601,"message":"boom"}}`)), nil
		}}
		_, err := ListCodexModels(context.Background(), runner, "/usr/bin/codex")
		if err == nil || !strings.Contains(err.Error(), "boom") || errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("process exits early", func(t *testing.T) {
		runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) {
			p := newScriptedProcess(func(method string, id *int) (string, bool) {
				return "", false
			})
			go func() { time.Sleep(20 * time.Millisecond); _ = p.Kill() }()
			return p, nil
		}}
		_, err := ListCodexModels(context.Background(), runner, "/usr/bin/codex")
		if err == nil || !strings.Contains(err.Error(), "exited before answering") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		runner := &fakeRunner{startFunc: func(context.Context, []string) (Process, error) {
			return newScriptedProcess(func(method string, id *int) (string, bool) { return "", false }), nil
		}}
		_, err := ListCodexModels(ctx, runner, "/usr/bin/codex")
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("err = %v, want ErrTimeout", err)
		}
	})
}
