package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// PipedStartRequest describes a directly executed, manager-owned process whose
// stdin and stdout are consumed by another agentctl subsystem.
type PipedStartRequest struct {
	SessionID  string
	Kind       types.ProcessKind
	ScriptName string
	Command    string
	Args       []string
	WorkingDir string
	Env        map[string]string
	PipeStderr bool
}

// PipedProcess exposes protocol streams while ProcessRunner remains the sole
// owner of waiting and process-tree teardown.
type PipedProcess struct {
	ID      string
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	Stderr  io.ReadCloser
	Done    <-chan struct{}
	process *commandProcess
}

// Wait blocks until the command and its process tree are reaped.
func (p *PipedProcess) Wait() error {
	<-p.Done
	p.process.mu.Lock()
	waitErr := p.process.waitErr
	reapErr := p.process.reapErr
	p.process.mu.Unlock()
	return errors.Join(waitErr, reapErr)
}

// StartPiped starts a directly executed process with caller-owned stdin/stdout.
// It is intentionally independent of request contexts so the runner owns its
// full process tree until Stop or StopAllAndWait reaps it.
func (r *ProcessRunner) StartPiped(req PipedStartRequest) (*PipedProcess, error) {
	r.admission.RLock()
	defer r.admission.RUnlock()
	if err := r.validatePipedStart(req); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	cmd := exec.Command(req.Command, req.Args...)
	cmd.Dir = req.WorkingDir
	cmd.Env = mergeEnv(req.Env)
	setManagedProcGroup(cmd)

	streams, err := newPipedCommandStreams(cmd)
	if err != nil {
		return nil, err
	}
	defer streams.closeChildEnds()
	proc := r.newPipedCommandProcess(id, req, cmd, streams.stdinWriter)
	r.mu.Lock()
	r.processes[id] = proc
	r.mu.Unlock()
	r.publishStatus(proc)

	if err := r.startAndActivate(
		proc, cmd, id, streams.stdoutReader, streams.stderrReader, false, !req.PipeStderr,
	); err != nil {
		return nil, err
	}
	piped := &PipedProcess{
		ID:      id,
		Stdin:   streams.stdinWriter,
		Stdout:  streams.stdoutReader,
		Done:    proc.done,
		process: proc,
	}
	if req.PipeStderr {
		piped.Stderr = streams.stderrReader
	}
	return piped, nil
}

func (r *ProcessRunner) validatePipedStart(req PipedStartRequest) error {
	if r.stopping {
		return ErrManagerStopping
	}
	if req.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if req.Command == "" {
		return fmt.Errorf("command is required")
	}
	return nil
}

type pipedCommandStreams struct {
	stdinWriter  *os.File
	stdoutReader *os.File
	stderrReader *os.File
	childEnds    []*os.File
}

func newPipedCommandStreams(cmd *exec.Cmd) (*pipedCommandStreams, error) {
	childStdin, stdin, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to attach stdin: %w", err)
	}
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		closePipedFiles(childStdin, stdin)
		return nil, fmt.Errorf("failed to attach stdout: %w", err)
	}
	stderr, childStderr, err := os.Pipe()
	if err != nil {
		closePipedFiles(childStdin, stdin, stdout, childStdout)
		return nil, fmt.Errorf("failed to attach stderr: %w", err)
	}
	cmd.Stdin = childStdin
	cmd.Stdout = childStdout
	cmd.Stderr = childStderr
	return &pipedCommandStreams{
		stdinWriter: stdin, stdoutReader: stdout, stderrReader: stderr,
		childEnds: []*os.File{childStdin, childStdout, childStderr},
	}, nil
}

func (s *pipedCommandStreams) closeChildEnds() {
	closePipedFiles(s.childEnds...)
}

func closePipedFiles(files ...*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

func (r *ProcessRunner) newPipedCommandProcess(
	id string,
	req PipedStartRequest,
	cmd *exec.Cmd,
	stdin io.WriteCloser,
) *commandProcess {
	now := time.Now().UTC()
	return &commandProcess{
		info: ProcessInfo{
			ID:         id,
			SessionID:  req.SessionID,
			Kind:       req.Kind,
			ScriptName: req.ScriptName,
			Command:    strings.Join(append([]string{req.Command}, req.Args...), " "),
			WorkingDir: req.WorkingDir,
			Status:     types.ProcessStatusStarting,
			StartedAt:  now,
			UpdatedAt:  now,
		},
		cmd:        cmd,
		stdin:      stdin,
		buffer:     newRingBuffer(r.bufferMaxBytes),
		stopSignal: make(chan struct{}),
		done:       make(chan struct{}),
	}
}
