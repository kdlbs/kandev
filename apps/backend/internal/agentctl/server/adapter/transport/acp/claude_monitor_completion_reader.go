package acp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	claudeNativeVersion              = "2.1.323"
	claudeNativeCurrentVersion       = "2.1.328"
	claudeMonitorPollInterval        = 100 * time.Millisecond
	claudeMonitorMaxRecordBytes      = 256 * 1024
	claudeMonitorMaxReadBytes        = 1024 * 1024
	claudeMonitorMaxPendingRecords   = 64
	claudeMonitorMaxCompletionMarks  = 1024
	claudeMonitorEvidenceBridgeName  = verifiedClaudeTerminalBridgeName
	claudeMonitorEvidenceBridgeBuild = verifiedClaudeTerminalBridgeCurrentVersion
)

var (
	claudeMonitorSessionIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	claudeMonitorTaskIDRE    = regexp.MustCompile(`(?s)<task-id>\s*([^<\s]+)\s*</task-id>`)
	claudeMonitorToolUseIDRE = regexp.MustCompile(`(?s)<tool-use-id>\s*([^<\s]+)\s*</tool-use-id>`)
	claudeMonitorStatusRE    = regexp.MustCompile(`(?s)<status>\s*([^<\s]+)\s*</status>`)
)

type claudeMonitorCompletionKey struct {
	taskID    string
	toolUseID string
}

type claudeMonitorCompletion struct {
	SessionID  string
	TaskID     string
	ToolUseID  string
	State      streams.RunState
	FinishedAt time.Time
}

type claudeMonitorCompletionReader struct {
	root                      *os.Root
	workDirName               string
	sessionFile               string
	sessionID                 string
	version                   string
	admittedAt                time.Time
	projectInfo               os.FileInfo
	file                      *os.File
	fileInfo                  os.FileInfo
	offset                    int64
	pendingBytes              []byte
	discardingOversizedRecord bool
}

type claudeNativeTaskRecord struct {
	Type      string                 `json:"type"`
	SessionID string                 `json:"sessionId"`
	Version   string                 `json:"version"`
	Timestamp string                 `json:"timestamp"`
	Origin    claudeNativeTaskOrigin `json:"origin"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Attachment struct {
		Type   string                 `json:"type"`
		Origin claudeNativeTaskOrigin `json:"origin"`
		Prompt string                 `json:"prompt"`
	} `json:"attachment"`
}

type claudeNativeTaskOrigin struct {
	Kind     string `json:"kind"`
	Producer string `json:"producer"`
}

func parseClaudeMonitorCompletionRecord(line, sessionID, nativeVersion string) (claudeMonitorCompletion, bool) {
	if len(line) == 0 || len(line) > claudeMonitorMaxRecordBytes {
		return claudeMonitorCompletion{}, false
	}
	var record claudeNativeTaskRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return claudeMonitorCompletion{}, false
	}
	content, origin, ok := claudeNativeTaskRecordPrompt(record)
	if !ok || !isQualifiedClaudeMonitorRecord(record, origin, sessionID, nativeVersion) {
		return claudeMonitorCompletion{}, false
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, record.Timestamp)
	if err != nil {
		return claudeMonitorCompletion{}, false
	}
	taskID, toolUseID, status, ok := parseClaudeMonitorTaskNotification(content)
	if !ok {
		return claudeMonitorCompletion{}, false
	}
	state, ok := claudeMonitorRunState(status)
	if !ok {
		return claudeMonitorCompletion{}, false
	}
	return claudeMonitorCompletion{
		SessionID:  record.SessionID,
		TaskID:     taskID,
		ToolUseID:  toolUseID,
		State:      state,
		FinishedAt: finishedAt.UTC(),
	}, true
}

func claudeNativeTaskRecordPrompt(record claudeNativeTaskRecord) (string, claudeNativeTaskOrigin, bool) {
	switch record.Type {
	case acpUserRole:
		return record.Message.Content, record.Origin, record.Message.Role == acpUserRole
	case "attachment":
		if record.Attachment.Type != "queued_command" {
			return "", claudeNativeTaskOrigin{}, false
		}
		return record.Attachment.Prompt, record.Attachment.Origin, true
	default:
		return "", claudeNativeTaskOrigin{}, false
	}
}

func isQualifiedClaudeMonitorRecord(
	record claudeNativeTaskRecord,
	origin claudeNativeTaskOrigin,
	sessionID, nativeVersion string,
) bool {
	nativeVersionMatches := nativeVersion == "" || record.Version == nativeVersion
	return record.SessionID == sessionID && nativeVersionMatches && isQualifiedClaudeNativeVersion(record.Version) &&
		origin.Kind == "task-notification" && origin.Producer == "session-task" &&
		record.Timestamp != ""
}

func isQualifiedClaudeNativeVersion(version string) bool {
	return version == claudeNativeVersion || version == claudeNativeCurrentVersion
}

func claudeMonitorRunState(status string) (streams.RunState, bool) {
	switch status {
	case string(streams.RunStateCompleted):
		return streams.RunStateCompleted, true
	case string(streams.RunStateFailed):
		return streams.RunStateFailed, true
	case "stopped", string(streams.RunStateInterrupted):
		return streams.RunStateInterrupted, true
	default:
		return "", false
	}
}

func parseClaudeMonitorTaskNotification(content string) (string, string, string, bool) {
	if strings.Count(content, "<task-notification>") != 1 || strings.Count(content, "</task-notification>") != 1 {
		return "", "", "", false
	}
	start := strings.Index(content, "<task-notification>")
	if start < 0 {
		return "", "", "", false
	}
	content = content[start:]
	end := strings.Index(content, "</task-notification>")
	if end < 0 || strings.Contains(content[end+len("</task-notification>"):], "<task-notification>") {
		return "", "", "", false
	}
	content = content[:end+len("</task-notification>")]
	matchTag := func(re *regexp.Regexp) string {
		matches := re.FindAllStringSubmatch(content, -1)
		if len(matches) != 1 || len(matches[0]) != 2 {
			return ""
		}
		return matches[0][1]
	}
	taskID := matchTag(claudeMonitorTaskIDRE)
	toolUseID := matchTag(claudeMonitorToolUseIDRE)
	status := matchTag(claudeMonitorStatusRE)
	return taskID, toolUseID, status, taskID != "" && toolUseID != "" && status != ""
}

func newClaudeMonitorCompletionReader(
	configDir, workDir, sessionID string,
	admittedAt time.Time,
) (*claudeMonitorCompletionReader, error) {
	if !filepath.IsAbs(configDir) || !filepath.IsAbs(workDir) || !claudeMonitorSessionIDRE.MatchString(sessionID) {
		return nil, errors.New("claude Monitor evidence identity is not absolute and unambiguous")
	}
	workDir = filepath.Clean(workDir)
	projectName := encodeClaudeMonitorProjectName(workDir)
	if projectName == "" || len(projectName) > 200 {
		return nil, errors.New("claude Monitor project directory is not supported")
	}
	root, err := openPinnedClaudeMonitorRoot(filepath.Clean(configDir))
	if err != nil {
		return nil, err
	}
	reader := &claudeMonitorCompletionReader{
		root:        root,
		workDirName: projectName,
		sessionFile: sessionID + ".jsonl",
		sessionID:   sessionID,
		version:     "",
		admittedAt:  admittedAt,
	}
	project, err := reader.openProject()
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	if project != nil {
		reader.projectInfo, err = project.Stat(".")
		if err == nil {
			reader.file, reader.fileInfo, err = openClaudeMonitorRegularFile(project, reader.sessionFile)
		}
		_ = project.Close()
		if err != nil {
			_ = reader.Close()
			return nil, err
		}
		if reader.file != nil {
			reader.offset = reader.fileInfo.Size()
		}
	}
	return reader, nil
}

func (r *claudeMonitorCompletionReader) Close() error {
	var fileErr error
	if r.file != nil {
		fileErr = r.file.Close()
		r.file = nil
	}
	var rootErr error
	if r.root != nil {
		rootErr = r.root.Close()
		r.root = nil
	}
	return errors.Join(fileErr, rootErr)
}

func (r *claudeMonitorCompletionReader) ReadAvailable() ([]claudeMonitorCompletion, error) {
	project, err := r.openProject()
	if err != nil || project == nil {
		return nil, err
	}
	defer func() { _ = project.Close() }()
	if err := r.verifyProjectDirectory(project); err != nil {
		return nil, err
	}
	available, err := r.ensureSessionFile(project)
	if err != nil || !available {
		return nil, err
	}
	chunk, err := r.readAppendedChunk(project)
	if err != nil {
		return nil, err
	}
	r.pendingBytes = append(r.pendingBytes, chunk...)
	return r.parsePendingRecords()
}

func (r *claudeMonitorCompletionReader) verifyProjectDirectory(project *os.Root) error {
	projectInfo, err := project.Stat(".")
	if err != nil {
		return err
	}
	if r.projectInfo != nil && !os.SameFile(r.projectInfo, projectInfo) {
		r.resetSessionFile()
		r.projectInfo = projectInfo
		return errors.New("claude Monitor project directory changed")
	}
	r.projectInfo = projectInfo
	return nil
}

func (r *claudeMonitorCompletionReader) ensureSessionFile(project *os.Root) (bool, error) {
	if r.file == nil {
		file, fileInfo, err := openClaudeMonitorRegularFile(project, r.sessionFile)
		if err != nil {
			return false, err
		}
		if file == nil {
			return false, nil
		}
		r.file, r.fileInfo = file, fileInfo
	}
	return true, nil
}

func (r *claudeMonitorCompletionReader) readAppendedChunk(project *os.Root) ([]byte, error) {
	pathInfo, err := project.Lstat(r.sessionFile)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(r.fileInfo, pathInfo) {
		r.resetSessionFile()
		return nil, errors.Join(err, errors.New("claude Monitor session file changed"))
	}
	fileInfo, err := r.file.Stat()
	if err != nil || !os.SameFile(r.fileInfo, fileInfo) {
		r.resetSessionFile()
		return nil, errors.Join(err, errors.New("claude Monitor session file identity changed"))
	}
	if fileInfo.Size() < r.offset {
		r.offset = 0
		r.pendingBytes = nil
		r.discardingOversizedRecord = false
		return nil, errors.New("claude Monitor session file was truncated")
	}
	toRead := fileInfo.Size() - r.offset
	if toRead == 0 {
		return nil, nil
	}
	if toRead > claudeMonitorMaxReadBytes {
		toRead = claudeMonitorMaxReadBytes
	}
	chunk := make([]byte, int(toRead))
	n, readErr := r.file.ReadAt(chunk, r.offset)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if n != len(chunk) {
		return nil, errors.New("claude Monitor session file changed while reading")
	}
	r.offset += int64(n)
	return chunk, nil
}

func (r *claudeMonitorCompletionReader) resetSessionFile() {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	r.fileInfo = nil
	r.offset = 0
	r.pendingBytes = nil
	r.discardingOversizedRecord = false
}

func (r *claudeMonitorCompletionReader) parsePendingRecords() ([]claudeMonitorCompletion, error) {
	var completions []claudeMonitorCompletion
	for {
		lineEnd := bytes.IndexByte(r.pendingBytes, '\n')
		if r.discardingOversizedRecord {
			if lineEnd < 0 {
				r.pendingBytes = nil
				break
			}
			r.pendingBytes = r.pendingBytes[lineEnd+1:]
			r.discardingOversizedRecord = false
			continue
		}
		if lineEnd < 0 {
			if len(r.pendingBytes) > claudeMonitorMaxRecordBytes {
				r.pendingBytes = nil
				r.discardingOversizedRecord = true
			}
			break
		}
		if lineEnd > claudeMonitorMaxRecordBytes {
			r.pendingBytes = r.pendingBytes[lineEnd+1:]
			continue
		}
		line := strings.TrimSuffix(string(r.pendingBytes[:lineEnd]), "\r")
		r.pendingBytes = r.pendingBytes[lineEnd+1:]
		completion, ok := parseClaudeMonitorCompletionRecord(line, r.sessionID, r.version)
		if ok && !completion.FinishedAt.Before(r.admittedAt) {
			completions = append(completions, completion)
		}
	}
	return completions, nil
}

func (r *claudeMonitorCompletionReader) openProject() (*os.Root, error) {
	projects, err := openClaudeMonitorDirectory(r.root, "projects")
	if err != nil || projects == nil {
		return nil, err
	}
	defer func() { _ = projects.Close() }()
	return openClaudeMonitorDirectory(projects, r.workDirName)
}

func openPinnedClaudeMonitorRoot(path string) (*os.Root, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(resolved) {
		return nil, errors.New("claude configuration directory is not absolute")
	}
	anchor := filepath.VolumeName(resolved) + string(filepath.Separator)
	root, err := os.OpenRoot(anchor)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(anchor, resolved)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if relative == "." {
		return root, nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		next, openErr := openClaudeMonitorDirectory(root, component)
		_ = root.Close()
		if openErr != nil || next == nil {
			return nil, errors.Join(openErr, errors.New("claude configuration directory changed"))
		}
		root = next
	}
	return root, nil
}

func openClaudeMonitorDirectory(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !before.IsDir() {
		return nil, errors.Join(err, errors.New("claude Monitor path is not a directory"))
	}
	child, err := openClaudeMonitorContainedDirectory(parent, name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = child.Close()
		return nil, errors.Join(err, errors.New("claude Monitor directory changed"))
	}
	return child, nil
}

func openClaudeMonitorRegularFile(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	before, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil || !before.Mode().IsRegular() {
		return nil, nil, errors.Join(err, errors.New("claude Monitor session is not a regular file"))
	}
	file, err := openClaudeMonitorContainedFile(root, name)
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, nil, errors.Join(err, errors.New("claude Monitor session file changed"))
	}
	return file, after, nil
}

func encodeClaudeMonitorProjectName(workDir string) string {
	var b strings.Builder
	b.Grow(len(workDir))
	for _, char := range workDir {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			b.WriteRune(char)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}
