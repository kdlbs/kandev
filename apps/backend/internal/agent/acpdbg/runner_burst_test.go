package acpdbg

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The deadlock these tests pin is pure Go, so they must run everywhere kandev
// runs. They live outside runner_test.go, which is //go:build !windows because
// its process-group assertions need POSIX signals, and drive the agent through
// the re-executed-test-binary helper that ops_test.go already uses rather than
// through `sh`.

const burstHelperEnv = "ACPDBG_BURST_HELPER"

// burstHelperCommand returns the spawn command for the scripted agent below.
// The path has to be absolute: the runner gives the child its own working
// directory, so a relative os.Args[0] (what `go test -c` produces when the
// binary is invoked as ./pkg.test) would not resolve there.
func burstHelperCommand(t *testing.T) []string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		self, err = filepath.Abs(os.Args[0])
		if err != nil {
			t.Fatalf("locate test binary: %v", err)
		}
	}
	return []string{self, "-test.run=^TestACPDBGBurstHelperProcess$"}
}

// TestACPDBGBurstHelperProcess is not a test: when the env var is set it is the
// scripted ACP agent the burst tests talk to. It stays silent on stdout apart
// from protocol frames, because anything else corrupts the JSON-RPC stream.
func TestACPDBGBurstHelperProcess(t *testing.T) {
	mode := os.Getenv(burstHelperEnv)
	if mode == "" {
		return
	}
	notifications, _ := strconv.Atoi(os.Getenv(burstHelperEnv + "_COUNT"))

	// A wedged agent: never reads stdin, just writes notifications forever.
	if mode == "flood" {
		w := bufio.NewWriter(os.Stdout)
		for {
			_, _ = fmt.Fprintln(w, `{"jsonrpc":"2.0","method":"session/update"}`)
			if err := w.Flush(); err != nil {
				return
			}
		}
	}

	// emit flushes every line, so there is no deferred flush to lose when a
	// scripted death calls os.Exit.
	out := bufio.NewWriter(os.Stdout)
	emit := func(line string) {
		_, _ = fmt.Fprintln(out, line)
		_ = out.Flush()
	}
	chunk := func(text string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}}}`, text)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var req map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		id := req["id"]
		method, _ := req["method"].(string)

		switch method {
		case "initialize":
			emit(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"protocolVersion":1}}`, id))
		case "session/load":
			// Replay every history entry, then answer the request that is
			// waiting behind them.
			for i := 0; i < notifications; i++ {
				emit(chunk("HISTORY"))
			}
			emit(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{}}`, id))
		case "session/prompt":
			emit(chunk("ANSWER"))
			if mode == "die-mid-prompt" {
				// Die after accepting the prompt and streaming part of the
				// answer, so the caller is waiting on a response that will
				// never come. Exiting is what closes the pipe: returning would
				// leave the test binary finishing its own run with stdout still
				// open, and the caller would hit its deadline instead.
				os.Exit(0)
			}
			emit(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"stopReason":"end_turn"}}`, id))
		default:
			emit(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{}}`, id))
		}
	}
}

// TestRequestSurvivesReplayBurst is the regression for a deadlock that made
// session/load look like an agent that never answers. The out-of-band queue was
// a channel of 32; a replay emitting more than that blocked the read loop on the
// send, so the response queued behind the notifications was never read and the
// request hung until its deadline. Nothing drains the queue while Request waits,
// so the queue has to be unbounded.
func TestRequestSurvivesReplayBurst(t *testing.T) {
	// Incident scale. No finite number proves the queue is unbounded, but this
	// is far past any capacity someone would plausibly reintroduce (256, 1024,
	// 4096) while "bounding memory", which is the regression to catch.
	const notifications = 5000
	t.Setenv(burstHelperEnv, "replay")
	t.Setenv(burstHelperEnv+"_COUNT", strconv.Itoa(notifications))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "burst-helper",
		Command: burstHelperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	t.Cleanup(func() { runner.Close("test cleanup") })

	req, _ := runner.Framer().NewRequest("session/load", map[string]any{"sessionId": "s1"})
	if _, err := runner.Request(ctx, req); err != nil {
		t.Fatalf("Request did not complete behind a %d-notification replay: %v", notifications, err)
	}

	// Every notification must still be retrievable through the public API: the
	// queue buffers, it never drops, because this is a debug tool and the
	// frames are the evidence.
	for i := 0; i < notifications; i++ {
		frame, err := runner.NextOOB(ctx)
		if err != nil {
			t.Fatalf("NextOOB after %d of %d frames: %v", i, notifications, err)
		}
		if frame.Method() != methodSessionUpdate {
			t.Fatalf("frame %d method = %q, want session/update", i, frame.Method())
		}
	}
	runner.oobMu.Lock()
	left := len(runner.oobBuf)
	runner.oobMu.Unlock()
	if left != 0 {
		t.Fatalf("%d frames left queued after draining all %d", left, notifications)
	}
}

// TestPromptReportsChildDeathInsteadOfSuccess pins that a prompt whose agent
// dies mid-stream returns an error. The read loop closes the waiter, and a
// receive from a closed channel yields a nil frame; without checking ok that
// nil reads as a successful, empty response and the caller is told the prompt
// worked.
func TestPromptReportsChildDeathInsteadOfSuccess(t *testing.T) {
	t.Setenv(burstHelperEnv, "die-mid-prompt")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "dying-helper",
		Command: burstHelperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	t.Cleanup(func() { runner.Close("test cleanup") })

	if _, err := sendInitialize(ctx, runner); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_, err = sendPromptAndCollect(ctx, runner, "s1", "are you there?")
	if err == nil {
		t.Fatal("prompt reported success after the agent died mid-stream")
	}
	// Must fail because the read loop closed the waiter, not because the
	// caller's deadline expired — the latter would pass without the fix.
	if !strings.Contains(err.Error(), "read loop exited") {
		t.Fatalf("error = %v, want it to report the read loop exiting", err)
	}

	runner.mu.Lock()
	pending := len(runner.pending)
	runner.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d pending waiters left behind, want 0", pending)
	}
}

// TestReadLoopStopsQueueingAfterShutdown pins that a wedged agent flooding
// stdout cannot make us allocate indefinitely once shutdown starts. The queue
// is unbounded, so the read loop has to stop on its own: nothing will ever
// consume what it queues from that point.
func TestReadLoopStopsQueueingAfterShutdown(t *testing.T) {
	t.Setenv(burstHelperEnv, "flood")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "flood-helper",
		Command: burstHelperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	// Let the flood get going, then start shutting down.
	waitForOOBFrames(t, runner, 100)
	runner.signalShutdown()

	// The read loop should stop; the queue settles instead of growing without
	// bound. Sample twice with a gap the flood would easily fill.
	settled := oobLen(runner)
	time.Sleep(300 * time.Millisecond)
	if grown := oobLen(runner); grown != settled {
		t.Fatalf("queue kept growing after shutdown: %d -> %d", settled, grown)
	}
	runner.Close("test cleanup")
}

func oobLen(r *Runner) int {
	r.oobMu.Lock()
	defer r.oobMu.Unlock()
	return len(r.oobBuf)
}

func waitForOOBFrames(t *testing.T, r *Runner, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if oobLen(r) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d queued frames, got %d", want, oobLen(r))
}

// TestSessionLoadPromptExcludesReplayedHistory pins that a prompt issued after
// session/load returns only the new reply. The replay leaves the whole
// conversation queued out of band, and collecting it would hand the caller the
// transcript instead of the answer.
func TestSessionLoadPromptExcludesReplayedHistory(t *testing.T) {
	t.Setenv(burstHelperEnv, "history")
	t.Setenv(burstHelperEnv+"_COUNT", "50")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner, err := NewRunner(ctx, filepath.Join(t.TempDir(), "frames.jsonl"), RunConfig{
		AgentID: "history-helper",
		Command: burstHelperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	t.Cleanup(func() { runner.Close("test cleanup") })

	res, err := SessionLoad(ctx, runner, SessionLoadOptions{SessionID: "s1", Prompt: "what now?"})
	if err != nil {
		t.Fatalf("SessionLoad: %v", err)
	}
	if res.Text != "ANSWER" {
		t.Fatalf("collected text = %q, want %q (replayed history must not be collected)", res.Text, "ANSWER")
	}
}
