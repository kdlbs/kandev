package hostcli

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	output    string
	err       error
	delay     time.Duration
	lastArgv  []string
	startFunc func(ctx context.Context, argv []string) (Process, error)
}

func (f *fakeRunner) Output(ctx context.Context, argv []string) (string, error) {
	f.lastArgv = argv
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.output, f.err
}

func (f *fakeRunner) Start(ctx context.Context, argv []string) (Process, error) {
	f.lastArgv = argv
	if f.startFunc == nil {
		return nil, errors.New("start not configured")
	}
	return f.startFunc(ctx, argv)
}

// @covers AC-AGENTS-HOST-CLI-001.1
func TestParseVersion(t *testing.T) {
	cases := map[string]struct {
		output  string
		want    string
		wantErr bool
	}{
		"claude":          {output: "2.1.220 (Claude Code)\n", want: "2.1.220"},
		"codex":           {output: "codex-cli 0.155.1\n", want: "0.155.1"},
		"v prefix":        {output: "v1.2.3", want: "1.2.3"},
		"prerelease":      {output: "mock 3.4.5-beta.1", want: "3.4.5"},
		"noise then ver":  {output: "Checking for updates...\nmock-agent 9.9.9", want: "9.9.9"},
		"no version":      {output: "usage: thing [options]", wantErr: true},
		"two-part number": {output: "version 12.5", wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseVersion(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseVersion(%q) = %q, want error", tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.output, err)
			}
			if got != tc.want {
				t.Fatalf("ParseVersion(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}

// @covers AC-AGENTS-HOST-CLI-001.2 AC-AGENTS-HOST-CLI-001.4
func TestDetectVersion(t *testing.T) {
	t.Run("parses output and passes args", func(t *testing.T) {
		runner := &fakeRunner{output: "2.1.220 (Claude Code)"}
		got, err := DetectVersion(context.Background(), runner, "/usr/bin/claude", []string{"--version"})
		if err != nil || got != "2.1.220" {
			t.Fatalf("DetectVersion = %q, %v", got, err)
		}
		if strings.Join(runner.lastArgv, " ") != "/usr/bin/claude --version" {
			t.Fatalf("argv = %v", runner.lastArgv)
		}
	})
	t.Run("missing executable", func(t *testing.T) {
		_, err := DetectVersion(context.Background(), &fakeRunner{err: ErrNotInstalled}, "/nope", nil)
		if !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("err = %v, want ErrNotInstalled", err)
		}
	})
	t.Run("empty path", func(t *testing.T) {
		if _, err := DetectVersion(context.Background(), &fakeRunner{}, "  ", nil); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("err = %v, want ErrNotInstalled", err)
		}
	})
	t.Run("failure keeps a short reason", func(t *testing.T) {
		runner := &fakeRunner{output: "\n\nSegmentation fault (core dumped)\nmore\n", err: errors.New("exit status 139")}
		_, err := DetectVersion(context.Background(), runner, "/usr/bin/claude", nil)
		if err == nil || !strings.Contains(err.Error(), "Segmentation fault") || strings.Contains(err.Error(), "more") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("non-zero exit with a version still parses", func(t *testing.T) {
		runner := &fakeRunner{output: "0.155.1\nupdate check failed", err: errors.New("exit status 1")}
		got, err := DetectVersion(context.Background(), runner, "/usr/bin/codex", nil)
		if err != nil || got != "0.155.1" {
			t.Fatalf("DetectVersion = %q, %v", got, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		runner := &fakeRunner{output: "1.0.0", delay: time.Second}
		_, err := DetectVersion(ctx, runner, "/usr/bin/slow", nil)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err = %v, want timeout", err)
		}
	})
}

func TestExecutablePathNeverUsesBridge(t *testing.T) {
	spec := Spec{Executable: "codex"}
	for _, path := range []string{"/bin/codex-acp", "npx @agentclientprotocol/codex-acp", ""} {
		if got := ExecutablePath(spec, path); got != "codex" {
			t.Fatalf("path=%q got=%q", path, got)
		}
	}
	if got := ExecutablePath(spec, "/bin/codex"); got != "/bin/codex" {
		t.Fatalf("got=%q", got)
	}
}
func TestVersionOutputIsBounded(t *testing.T) {
	b := &boundedOutput{}
	chunk := []byte(strings.Repeat("x", 100000))
	for i := 0; i < 3; i++ {
		if n, err := b.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("write=%d,%v", n, err)
		}
	}
	if b.Len() != 65536 {
		t.Fatalf("retained %d bytes", b.Len())
	}
}

func TestExecProcessKillReleasesBlockedProtocolRead(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer func() { _ = inReader.Close() }()
	defer func() { _ = inWriter.Close() }()
	defer func() { _ = outReader.Close() }()
	defer func() { _ = outWriter.Close() }()
	readStarted := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		close(readStarted)
		_, err := outReader.Read(make([]byte, 1))
		readDone <- err
	}()
	<-readStarted
	p := &execProcess{cmd: &exec.Cmd{}, stdin: inWriter, stdout: outReader}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("read error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Kill did not release the protocol reader")
	}
	if _, err := inWriter.Write([]byte("request")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("stdin remains open: %v", err)
	}
}
