package sleepinhibition

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const SettingsKey = "task_sleep_inhibition"

type IssueCode string

const (
	IssueUnsupportedPlatform      IssueCode = "unsupported_platform"
	IssueSystemServiceUnavailable IssueCode = "system_service_unavailable"
	IssueRequestFailed            IssueCode = "request_failed"
)

type Platform string

const (
	PlatformDarwin  Platform = "darwin"
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformOther   Platform = "other"
)

var (
	ErrInvalidPersisted = errors.New("invalid persisted task sleep inhibition settings")
	ErrUnsupported      = errors.New("sleep inhibition is unsupported on this platform")
)

type IssueError struct {
	Code IssueCode
	Err  error
}

func (e *IssueError) Error() string {
	if e == nil || e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *IssueError) Unwrap() error { return e.Err }

func NewIssueError(code IssueCode, err error) error {
	return &IssueError{Code: code, Err: err}
}

func IsInvalidPersisted(err error) bool { return errors.Is(err, ErrInvalidPersisted) }

func IssueFromError(err error) IssueCode {
	if err == nil {
		return ""
	}
	var issue *IssueError
	if errors.As(err, &issue) && issue != nil {
		return issue.Code
	}
	if errors.Is(err, ErrUnsupported) {
		return IssueUnsupportedPlatform
	}
	return IssueRequestFailed
}

type Settings struct {
	Enabled bool `json:"enabled"`
}

type Status struct {
	Platform  Platform  `json:"platform"`
	Supported bool      `json:"supported"`
	Active    bool      `json:"active"`
	Issue     IssueCode `json:"issue,omitempty"`
}

type Response struct {
	Settings Settings `json:"settings"`
	Status   Status   `json:"status"`
}

type SessionReader interface {
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
}

type Lease interface {
	Release() error
	Done() <-chan error
}

type Inhibitor interface {
	Platform() Platform
	Supported() bool
	Acquire(ctx context.Context) (Lease, error)
}
