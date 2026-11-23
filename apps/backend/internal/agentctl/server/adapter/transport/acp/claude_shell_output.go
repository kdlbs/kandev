package acp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	claudeShellOutputPrefix     = "Output is being written to: "
	claudeShellOutputPollPeriod = 2 * time.Second
	claudeShellOutputIdleLimit  = 30 * time.Minute
	claudeShellOutputMaxJobs    = 20
	claudeShellOutputMaxBytes   = 200 * 1024
)

var claudeShellOutputPathPattern = regexp.MustCompile(`(^|[/\\])tasks[/\\][^/\\]+\.output`)

func claudeShellOutputPath(stdout, nativeSessionID string) (string, bool) {
	if nativeSessionID == "" {
		return "", false
	}
	_, suffix, found := strings.Cut(stdout, claudeShellOutputPrefix)
	if !found {
		return "", false
	}
	if lineEnd := strings.IndexAny(suffix, "\r\n"); lineEnd >= 0 {
		suffix = suffix[:lineEnd]
	}
	line := strings.TrimSpace(suffix)
	path, ok := claudeShellOutputPathFromLine(line)
	if !ok {
		return "", false
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", false
	}
	tasksDir := filepath.Dir(path)
	sessionDir := filepath.Dir(tasksDir)
	if filepath.Base(tasksDir) != "tasks" || filepath.Base(sessionDir) != nativeSessionID {
		return "", false
	}
	if filepath.Ext(path) != ".output" || filepath.Base(path) == ".output" {
		return "", false
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", false
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return path, true
}

func claudeShellOutputPathFromLine(line string) (string, bool) {
	searchFrom := 0
	for searchFrom < len(line) {
		match := claudeShellOutputPathPattern.FindStringIndex(line[searchFrom:])
		if match == nil {
			return "", false
		}
		end := searchFrom + match[1]
		if end == len(line) || line[end] == '.' {
			return strings.TrimSpace(line[:end]), true
		}
		next, _ := utf8.DecodeRuneInString(line[end:])
		if unicode.IsSpace(next) {
			return strings.TrimSpace(line[:end]), true
		}
		searchFrom += match[0] + 1
	}
	return "", false
}

type claudeShellOutputJob struct {
	mu               sync.Mutex
	sendMu           sync.Mutex
	active           atomic.Bool
	sessionID        string
	workID           string
	path             string
	offset           int64
	outputOffset     int64
	lastObservedSize int64
	lastGrowth       time.Time
	ready            bool
	existed          bool
}

type claudeShellOutputFollower struct {
	ctx      context.Context
	send     func(AgentEvent)
	now      func() time.Time
	interval time.Duration

	mu      sync.Mutex
	jobs    map[string]*claudeShellOutputJob
	running bool
	done    chan struct{}
	wake    chan struct{}
}

func newClaudeShellOutputFollower(ctx context.Context, send func(AgentEvent)) *claudeShellOutputFollower {
	return &claudeShellOutputFollower{
		ctx:      ctx,
		send:     send,
		now:      time.Now,
		interval: claudeShellOutputPollPeriod,
		jobs:     make(map[string]*claudeShellOutputJob),
		wake:     make(chan struct{}, 1),
	}
}

func (f *claudeShellOutputFollower) follow(sessionID, workID, path string) bool {
	if f == nil || f.ctx.Err() != nil || sessionID == "" || workID == "" || path == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ctx.Err() != nil || len(f.jobs) >= claudeShellOutputMaxJobs {
		return false
	}
	key := claudeShellOutputKey(sessionID, workID)
	if _, exists := f.jobs[key]; exists {
		return false
	}
	job := &claudeShellOutputJob{
		sessionID:  sessionID,
		workID:     workID,
		path:       path,
		lastGrowth: f.now(),
	}
	job.active.Store(true)
	f.jobs[key] = job
	if !f.running {
		f.running = true
		f.done = make(chan struct{})
		go f.run()
	}
	return true
}

func (f *claudeShellOutputFollower) activate(sessionID, workID string) bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	job := f.jobs[claudeShellOutputKey(sessionID, workID)]
	f.mu.Unlock()
	if job == nil {
		return false
	}
	job.mu.Lock()
	activated := job.active.Load()
	if activated {
		job.ready = true
	}
	job.mu.Unlock()
	if activated {
		f.signalPoller()
	}
	return activated
}

func (f *claudeShellOutputFollower) finish(sessionID, workID string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	key := claudeShellOutputKey(sessionID, workID)
	job := f.jobs[key]
	if job != nil {
		delete(f.jobs, key)
		job.active.Store(false)
		f.signalPoller()
	}
	f.mu.Unlock()
	if job == nil {
		return
	}
	f.readAndSend(job, true)
}

func (f *claudeShellOutputFollower) clear() {
	if f == nil {
		return
	}
	f.mu.Lock()
	for _, job := range f.jobs {
		job.active.Store(false)
	}
	clear(f.jobs)
	f.signalPoller()
	f.mu.Unlock()
}

func (f *claudeShellOutputFollower) wait() {
	if f == nil {
		return
	}
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (f *claudeShellOutputFollower) run() {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-f.ctx.Done():
			f.finishPoller()
			return
		case <-f.wake:
			if f.stopIfEmpty() {
				return
			}
		case <-ticker.C:
			f.pollOnce()
			if f.stopIfEmpty() {
				return
			}
		}
	}
}

func (f *claudeShellOutputFollower) stopIfEmpty() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.jobs) != 0 {
		return false
	}
	f.finishPollerLocked()
	return true
}

func (f *claudeShellOutputFollower) finishPoller() {
	f.mu.Lock()
	f.finishPollerLocked()
	f.mu.Unlock()
}

func (f *claudeShellOutputFollower) finishPollerLocked() {
	if f.running {
		f.running = false
		close(f.done)
		f.done = nil
	}
}

func (f *claudeShellOutputFollower) pollOnce() {
	f.mu.Lock()
	jobs := make([]*claudeShellOutputJob, 0, len(f.jobs))
	for _, job := range f.jobs {
		jobs = append(jobs, job)
	}
	f.mu.Unlock()
	for _, job := range jobs {
		f.readAndSend(job, false)
	}
}

func (f *claudeShellOutputFollower) readAndSend(job *claudeShellOutputJob, final bool) {
	job.sendMu.Lock()
	defer job.sendMu.Unlock()
	job.mu.Lock()
	var event *AgentEvent
	if !final || job.ready {
		event = f.readJobLocked(job, final)
	}
	job.mu.Unlock()
	if event != nil {
		f.send(*event)
	}
}

func (f *claudeShellOutputFollower) readJobLocked(job *claudeShellOutputJob, final bool) *AgentEvent {
	if !final && !job.active.Load() {
		return nil
	}
	file, size, ok := f.openOutputFile(job)
	if !ok {
		return nil
	}
	defer func() {
		_ = file.Close()
	}()
	if !job.ready {
		f.observeOutputSize(job, size, false)
		return nil
	}
	if !f.observeOutputSize(job, size, final) {
		return nil
	}
	return f.emitAvailableOutput(job, file, size, final)
}

func (f *claudeShellOutputFollower) openOutputFile(job *claudeShellOutputJob) (*os.File, int64, bool) {
	info, err := os.Lstat(job.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || job.existed || f.now().Sub(job.lastGrowth) >= claudeShellOutputIdleLimit {
			f.remove(job)
		}
		return nil, 0, false
	}
	if !info.Mode().IsRegular() {
		f.remove(job)
		return nil, 0, false
	}
	file, err := os.Open(job.path)
	if err != nil {
		f.remove(job)
		return nil, 0, false
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		f.remove(job)
		return nil, 0, false
	}
	return file, opened.Size(), true
}

func (f *claudeShellOutputFollower) observeOutputSize(job *claudeShellOutputJob, size int64, final bool) bool {
	if size < job.offset || size < job.lastObservedSize {
		f.remove(job)
		return false
	}
	if size > job.lastObservedSize {
		job.lastObservedSize = size
		job.lastGrowth = f.now()
		job.existed = true
	} else if !final && f.now().Sub(job.lastGrowth) >= claudeShellOutputIdleLimit {
		f.remove(job)
		return false
	}
	return true
}

func (f *claudeShellOutputFollower) emitAvailableOutput(
	job *claudeShellOutputJob,
	file *os.File,
	size int64,
	final bool,
) *AgentEvent {
	if size == job.offset {
		return nil
	}
	start := job.offset
	truncated := false
	if size-start > claudeShellOutputMaxBytes {
		start = size - claudeShellOutputMaxBytes
		truncated = true
	}
	data := make([]byte, size-start)
	read, err := file.ReadAt(data, start)
	if (err != nil && !errors.Is(err, io.EOF)) || read != len(data) {
		f.remove(job)
		return nil
	}
	if truncated {
		skipped := 0
		for skipped < len(data) && !utf8.RuneStart(data[skipped]) {
			skipped++
		}
		start += int64(skipped)
		data = data[skipped:]
		job.offset = start
	}
	complete := completeUTF8PrefixLength(data)
	if complete == 0 {
		return nil
	}
	if !final && !job.active.Load() {
		return nil
	}
	chunk := strings.ToValidUTF8(string(data[:complete]), "\uFFFD")
	job.offset = start + int64(complete)
	event := &AgentEvent{
		Type:      streams.EventTypeBackgroundWorkOutput,
		SessionID: job.sessionID,
		BackgroundWorkOutput: &streams.WorkloadOutputChunk{
			SessionID: job.sessionID,
			WorkID:    job.workID,
			Chunk:     chunk,
			Offset:    job.outputOffset,
			Truncated: truncated,
			Stream:    "stdout",
		},
	}
	job.outputOffset += int64(len(chunk))
	return event
}

func completeUTF8PrefixLength(data []byte) int {
	for backoff := 0; backoff < utf8.UTFMax && backoff < len(data); backoff++ {
		start := len(data) - 1 - backoff
		if !utf8.RuneStart(data[start]) {
			continue
		}
		if utf8.FullRune(data[start:]) {
			return len(data)
		}
		return start
	}
	return len(data)
}

func (f *claudeShellOutputFollower) remove(job *claudeShellOutputJob) {
	f.mu.Lock()
	key := claudeShellOutputKey(job.sessionID, job.workID)
	if f.jobs[key] == job {
		delete(f.jobs, key)
	}
	job.active.Store(false)
	f.signalPoller()
	f.mu.Unlock()
}

func (f *claudeShellOutputFollower) signalPoller() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

func claudeShellOutputKey(sessionID, workID string) string {
	return sessionID + "\x00" + workID
}
