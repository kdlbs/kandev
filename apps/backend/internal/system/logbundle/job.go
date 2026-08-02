package logbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type Status string

const (
	StatusCollecting Status = "collecting"
	StatusBuilding   Status = "building"
	StatusReady      Status = "ready"
	StatusPartial    Status = "partial"
	StatusFailed     Status = "failed"
	StatusExpired    Status = "expired"
)

type ErrorKind string

const (
	ErrorInvalid      ErrorKind = "invalid"
	ErrorNotFound     ErrorKind = "not_found"
	ErrorGone         ErrorKind = "gone"
	ErrorConflict     ErrorKind = "conflict"
	ErrorIdentityBusy ErrorKind = "identity_busy"
	ErrorSaturated    ErrorKind = "saturated"
	ErrorTooLarge     ErrorKind = "too_large"
	ErrorProfileLimit ErrorKind = "profile_limit"
)

type ServiceError struct {
	Kind    ErrorKind
	Message string
}

func (e *ServiceError) Error() string { return e.Message }

func IsKind(err error, kind ErrorKind) bool {
	var serviceError *ServiceError
	return errors.As(err, &serviceError) && serviceError.Kind == kind
}

func newError(kind ErrorKind, format string, args ...any) error {
	return &ServiceError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

type JobView struct {
	ID                 string     `json:"id"`
	Status             Status     `json:"status"`
	Sources            []string   `json:"sources"`
	SessionIDs         []string   `json:"session_ids,omitempty"`
	RuntimeEntryCount  int        `json:"runtime_entry_count,omitempty"`
	ACPSessionCount    int        `json:"acp_session_count,omitempty"`
	BuildDeadline      time.Time  `json:"build_deadline"`
	CaptureDeadline    *time.Time `json:"capture_deadline,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at"`
	BrowserProfiles    int        `json:"browser_profiles"`
	FrontendEntryCount int        `json:"frontend_entry_count"`
	FrontendBytes      int64      `json:"frontend_bytes"`
	Warnings           []string   `json:"warnings"`
	DownloadURL        string     `json:"download_url,omitempty"`
}

type UploadChunk struct {
	BrowserID       string            `json:"browser_id"`
	CaptureStreamID string            `json:"capture_stream_id"`
	ChunkIndex      int               `json:"chunk_index"`
	Done            bool              `json:"done"`
	StorageMode     string            `json:"storage_mode"`
	CaptureMetadata json.RawMessage   `json:"capture_metadata"`
	Entries         []json.RawMessage `json:"entries"`
}

type browserCapture struct {
	ID              string
	StreamID        string
	Index           int
	NextChunk       int
	EntryCount      int
	Bytes           int64
	Path            string
	File            *os.File
	Done            bool
	StorageMode     string
	CaptureMetadata json.RawMessage
	Truncated       bool
}

type job struct {
	ID              string
	Owner           string
	Status          Status
	Sources         []string
	SessionIDs      []string
	RuntimeSessions []DiagnosticSession
	ACPSessions     []DiagnosticSession
	SourceKey       string
	CreatedAt       time.Time
	BuildDeadline   time.Time
	CaptureDeadline *time.Time
	ExpiresAt       *time.Time
	WorkDir         string
	ArchivePath     string
	Browsers        map[string]*browserCapture
	FrontendBytes   int64
	FrontendEntries int
	Warnings        []string
	Partial         bool
}

func (j *job) view() JobView {
	view := JobView{
		ID: j.ID, Status: j.Status, Sources: append([]string(nil), j.Sources...),
		SessionIDs:        append([]string(nil), j.SessionIDs...),
		RuntimeEntryCount: len(j.RuntimeSessions), ACPSessionCount: len(j.ACPSessions),
		BuildDeadline: j.BuildDeadline, CaptureDeadline: j.CaptureDeadline,
		ExpiresAt: j.ExpiresAt, BrowserProfiles: len(j.Browsers),
		FrontendEntryCount: j.FrontendEntries, FrontendBytes: j.FrontendBytes,
		Warnings: append([]string(nil), j.Warnings...),
	}
	if j.Status == StatusReady || j.Status == StatusPartial {
		view.DownloadURL = "/api/v1/system/logs/bundles/" + j.ID + "/download"
	}
	return view
}
