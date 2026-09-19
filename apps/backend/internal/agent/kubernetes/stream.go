package kubernetes

import (
	"context"
	"errors"
	"io"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

const loopbackAddress = "127.0.0.1"

type ExecRequest struct {
	Namespace string
	Pod       string
	Container string
	Command   []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	TTY       bool
}

type PodExecutor interface {
	Exec(context.Context, ExecRequest) error
}

type PortForwardRequest struct {
	Namespace    string
	Pod          string
	LocalAddress string
	LocalPort    uint16
	RemotePort   uint16
}

type PortForwardSession interface {
	LocalPort() uint16
	Ready() <-chan struct{}
	Done() <-chan error
	Close() error
}

type PortForwarder interface {
	Forward(context.Context, PortForwardRequest) (PortForwardSession, error)
}

type StreamOperations struct {
	executor  PodExecutor
	forwarder PortForwarder
}

func NewStreamOperations(executor PodExecutor, forwarder PortForwarder) *StreamOperations {
	return &StreamOperations{executor: executor, forwarder: forwarder}
}

func (s *StreamOperations) Exec(ctx context.Context, request ExecRequest) error {
	if err := validateExecRequest(s.executor, request); err != nil {
		return err
	}
	if err := s.executor.Exec(ctx, request); err != nil {
		return sanitizeStreamError("exec", err)
	}
	return nil
}

func (s *StreamOperations) Forward(ctx context.Context, request PortForwardRequest) (PortForwardSession, error) {
	if err := validateForwardRequest(s.forwarder, &request); err != nil {
		return nil, err
	}
	session, err := s.forwarder.Forward(ctx, request)
	if err != nil {
		return nil, sanitizeStreamError("port forward", err)
	}
	if session == nil {
		return nil, sanitizeStreamError("port forward", errors.New("forwarder returned nil session"))
	}
	return session, nil
}

func validateExecRequest(executor PodExecutor, request ExecRequest) error {
	if executor == nil {
		return fieldError("exec.executor", "is not configured")
	}
	if len(validation.IsDNS1123Label(request.Namespace)) > 0 {
		return fieldError("exec.namespace", "must be a valid namespace")
	}
	if len(validation.IsDNS1123Subdomain(request.Pod)) > 0 {
		return fieldError("exec.pod", "must be a valid Pod name")
	}
	if len(validation.IsDNS1123Label(request.Container)) > 0 {
		return fieldError("exec.container", "must be a valid container name")
	}
	if len(request.Command) == 0 {
		return fieldError("exec.command", "is required")
	}
	return nil
}

func validateForwardRequest(forwarder PortForwarder, request *PortForwardRequest) error {
	if forwarder == nil {
		return fieldError("port_forward.forwarder", "is not configured")
	}
	if len(validation.IsDNS1123Label(request.Namespace)) > 0 {
		return fieldError("port_forward.namespace", "must be a valid namespace")
	}
	if len(validation.IsDNS1123Subdomain(request.Pod)) > 0 {
		return fieldError("port_forward.pod", "must be a valid Pod name")
	}
	if request.LocalAddress == "" {
		request.LocalAddress = loopbackAddress
	}
	if request.LocalAddress != loopbackAddress {
		return fieldError("port_forward.local_address", "must be "+loopbackAddress)
	}
	if request.RemotePort == 0 {
		return fieldError("port_forward.remote_port", "must be non-zero")
	}
	return nil
}

type streamError struct {
	operation string
	cause     error
}

func (e *streamError) Error() string {
	return "kubernetes " + e.operation + " failed: " + routingerr.Sanitize(e.cause.Error())
}
func (e *streamError) Unwrap() error { return e.cause }

func sanitizeStreamError(operation string, cause error) error {
	return &streamError{operation: operation, cause: cause}
}
