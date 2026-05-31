package shared

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// PRIVACY / PERF: the JSONL frames written here contain the full prompt, file,
// and tool-call content exchanged with the agent. They are strictly a
// local-dev debugging aid gated behind KANDEV_DEBUG_AGENT_MESSAGES and must
// never be enabled in a shared/production deployment. See the agentctl
// CLAUDE.md "ACP Protocol" section for the documented env knobs.

// Debug-log filename pieces. The "raw-"/"normalized-" prefix and ".jsonl"
// suffix are a compatibility contract with the debug reader in
// internal/debug (discoverNormalizedFiles + handleReadNormalizedEvents) —
// do not change them without updating that package.
const (
	rawPrefix        = "raw-"
	normalizedPrefix = "normalized-"
	jsonlSuffix      = ".jsonl"
)

// Environment knobs (all optional; sensible defaults below).
const (
	envDebugMessages   = "KANDEV_DEBUG_AGENT_MESSAGES"
	envLogDir          = "KANDEV_DEBUG_LOG_DIR"
	envHomeDir         = "KANDEV_HOME_DIR"
	envACPMaxFiles     = "KANDEV_DEBUG_ACP_MAX_FILES"
	envACPRetentionHrs = "KANDEV_DEBUG_ACP_RETENTION_HOURS"
	envACPMaxFileBytes = "KANDEV_DEBUG_ACP_MAX_FILE_BYTES"
)

const (
	defaultACPMaxFiles     = 200
	defaultACPRetentionHrs = 48
	defaultACPMaxFileBytes = 8 << 20 // 8 MiB
	defaultACPRingSize     = 500

	// Owner-only perms: these files carry full prompt/file/tool content.
	acpDirPerm  = 0o700
	acpFilePerm = 0o600
)

// acpLogConfig is the resolved configuration for the managed writer registry.
type acpLogConfig struct {
	dir          string
	maxFileBytes int64
	maxFiles     int
	retention    time.Duration
	ringSize     int
}

// acpLogConfigFromEnv resolves the on-disk config from the environment,
// applying defaults. The output directory resolves to KANDEV_DEBUG_LOG_DIR,
// then <KANDEV_HOME_DIR>/logs/acp, then ~/.kandev/logs/acp, then the process
// CWD as a last resort.
func acpLogConfigFromEnv() acpLogConfig {
	return acpLogConfig{
		dir:          resolveACPLogDir(),
		maxFileBytes: envInt64(envACPMaxFileBytes, defaultACPMaxFileBytes),
		maxFiles:     envInt(envACPMaxFiles, defaultACPMaxFiles),
		retention:    time.Duration(envInt64(envACPRetentionHrs, defaultACPRetentionHrs)) * time.Hour,
		ringSize:     defaultACPRingSize,
	}
}

// envInt reads a positive int env var with a sane upper bound, so the value
// (a file-count cap) can't overflow when narrowed from the parsed int64 on a
// 32-bit platform.
func envInt(key string, def int) int {
	const maxInt = 1 << 30 // generous; well within int on every platform
	n := envInt64(key, int64(def))
	if n > maxInt {
		return maxInt
	}
	return int(n)
}

func resolveACPLogDir() string {
	if dir := os.Getenv(envLogDir); dir != "" {
		return dir
	}
	// Honor KANDEV_HOME_DIR before $HOME so dev/e2e isolation (and Docker/K8s
	// roots) keep ACP logs inside the configured Kandev home. The env value is
	// already the Kandev root (e.g. <repo>/.kandev-dev), mirroring
	// config.Config.ResolvedHomeDir — so we append logs/acp directly and must
	// NOT add another ".kandev" segment. agentctl is a lean container binary
	// that doesn't pull in the viper-based common/config, so we read the env
	// directly rather than importing ResolvedHomeDir.
	if home := os.Getenv(envHomeDir); home != "" {
		return filepath.Join(expandTilde(home), "logs", "acp")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".kandev", "logs", "acp")
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// expandTilde expands a leading "~/" to the user's home directory, mirroring
// config.expandTilde. Returns the input unchanged if expansion is unnecessary
// or fails.
func expandTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// acpLogManager owns one kept-open buffered writer per session file plus a
// bounded in-memory ring buffer per session. Writers carry their own mutex so
// the streaming hot path never contends on a single global lock.
type acpLogManager struct {
	cfg     acpLogConfig
	mu      sync.Mutex // guards writers + rings maps
	writers map[string]*acpWriter
	rings   map[string]*ringBuffer
}

func newACPLogManager(cfg acpLogConfig) *acpLogManager {
	if cfg.ringSize <= 0 {
		cfg.ringSize = defaultACPRingSize
	}
	if cfg.maxFileBytes <= 0 {
		cfg.maxFileBytes = defaultACPMaxFileBytes
	}
	return &acpLogManager{
		cfg:     cfg,
		writers: make(map[string]*acpWriter),
		rings:   make(map[string]*ringBuffer),
	}
}

// rawEntry mirrors the legacy raw debug-log line shape.
type rawEntry struct {
	Ts       int64           `json:"ts"`
	Protocol string          `json:"protocol"`
	Agent    string          `json:"agent"`
	Event    string          `json:"event"`
	Data     json.RawMessage `json:"data"`
}

// normalizedEntry mirrors the legacy normalized debug-log line shape consumed
// by internal/debug.readNormalizedEventsAsMessages.
type normalizedEntry struct {
	Ts    int64               `json:"ts"`
	Event *streams.AgentEvent `json:"event"`
}

func (m *acpLogManager) writeRaw(protocol, agentID, sessionID, eventType string, rawData json.RawMessage) {
	entry := rawEntry{
		Ts:       time.Now().UnixMilli(),
		Protocol: protocol,
		Agent:    agentID,
		Event:    eventType,
		Data:     rawData,
	}
	m.write(rawFileName(protocol, agentID, sessionID), entry)
}

func (m *acpLogManager) writeNormalized(protocol, agentID, sessionID string, event *streams.AgentEvent) {
	entry := normalizedEntry{Ts: time.Now().UnixMilli(), Event: event}
	line := m.write(normalizedFileName(protocol, agentID, sessionID), entry)
	if line != nil {
		// Copy: json.Marshal returns a fresh slice, safe to retain.
		m.ring(sessionID).add(line)
	}
}

// write marshals entry and appends it to the named file's writer. Returns the
// marshaled line so callers can also feed it to the ring buffer.
func (m *acpLogManager) write(name string, entry any) []byte {
	line, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[DEBUG] acplog: marshal entry: %v", err)
		return nil
	}
	w := m.getWriter(name)
	if w == nil {
		// The file couldn't be opened (e.g. no writable dir, as in a container
		// without a mounted volume). Still return the marshaled line so the
		// in-memory ring buffer / live-tail endpoint keeps working — those
		// events just aren't persisted to disk.
		return line
	}
	w.writeLine(line)
	return line
}

func (m *acpLogManager) getWriter(name string) *acpWriter {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.writers[name]; ok {
		return w
	}
	if err := os.MkdirAll(m.cfg.dir, acpDirPerm); err != nil {
		log.Printf("[DEBUG] acplog: mkdir %s: %v", m.cfg.dir, err)
		return nil
	}
	if err := os.Chmod(m.cfg.dir, acpDirPerm); err != nil {
		log.Printf("[DEBUG] acplog: chmod %s: %v", m.cfg.dir, err)
		return nil
	}
	path := filepath.Join(m.cfg.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, acpFilePerm)
	if err != nil {
		log.Printf("[DEBUG] acplog: open %s: %v", path, err)
		return nil
	}
	var size int64
	if info, statErr := f.Stat(); statErr == nil {
		size = info.Size()
	}
	w := &acpWriter{
		path:      path,
		f:         f,
		buf:       bufio.NewWriter(f),
		size:      size,
		maxBytes:  m.cfg.maxFileBytes,
		lastWrite: time.Now(),
	}
	m.writers[name] = w
	return w
}

func (m *acpLogManager) ring(sessionID string) *ringBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rings[sessionID]
	if !ok {
		r = newRingBuffer(m.cfg.ringSize)
		m.rings[sessionID] = r
	}
	return r
}

// ringTail returns up to n most recent normalized entries for a session, or
// nil if the session has no ring buffer.
func (m *acpLogManager) ringTail(sessionID string, n int) []json.RawMessage {
	m.mu.Lock()
	r := m.rings[sessionID]
	m.mu.Unlock()
	if r == nil {
		return nil
	}
	return r.tail(n)
}

func (m *acpLogManager) writersSnapshot() []*acpWriter {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*acpWriter, 0, len(m.writers))
	for _, w := range m.writers {
		out = append(out, w)
	}
	return out
}

func (m *acpLogManager) flushAll() {
	for _, w := range m.writersSnapshot() {
		w.flush()
	}
}

func (m *acpLogManager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, w := range m.writers {
		w.close()
		delete(m.writers, name)
	}
	m.rings = make(map[string]*ringBuffer)
}

// closeIdle flushes+closes writers untouched for longer than maxIdle so file
// handles don't leak across many short-lived sessions, and evicts the matching
// idle ring buffers so an always-on process doesn't accumulate one per
// historical session. The file stays on disk (retention decides deletion); a
// later write reopens it in append mode and re-creates the ring.
func (m *acpLogManager) closeIdle(maxIdle time.Duration) {
	cutoff := time.Now().Add(-maxIdle)
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, w := range m.writers {
		if w.idleSince(cutoff) {
			w.close()
			delete(m.writers, name)
		}
	}
	for sid, r := range m.rings {
		if r.idleSince(cutoff) {
			delete(m.rings, sid)
		}
	}
}

// writerActiveSince reports whether an open writer for path has a buffered
// last-write at or after cutoff (i.e. the session is still being written).
func (m *acpLogManager) writerActiveSince(path string, cutoff time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.writers {
		if w.path == path {
			return !w.idleSince(cutoff)
		}
	}
	return false
}

// closeWriterForPath closes and forgets any open writer for path so the file
// can be removed (required on Windows, where deleting an open file fails).
func (m *acpLogManager) closeWriterForPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, w := range m.writers {
		if w.path == path {
			w.close()
			delete(m.writers, name)
			return
		}
	}
}

// logFileInfo is a discovered debug-log file and its mtime, used for retention.
type logFileInfo struct {
	path  string
	mtime time.Time
}

// isACPLogFile reports whether name is one of our debug-log files (active or
// rotated). Mirrors the prefix/suffix contract enforced by internal/debug.
func isACPLogFile(name string) bool {
	return strings.HasSuffix(name, jsonlSuffix) &&
		(strings.HasPrefix(name, rawPrefix) || strings.HasPrefix(name, normalizedPrefix))
}

func (m *acpLogManager) listLogFiles() []logFileInfo {
	entries, err := os.ReadDir(m.cfg.dir)
	if err != nil {
		return nil
	}
	out := make([]logFileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isACPLogFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, logFileInfo{path: filepath.Join(m.cfg.dir, e.Name()), mtime: info.ModTime()})
	}
	return out
}

// removeFile closes any open writer for path (so deletion works on Windows)
// and unlinks the file.
func (m *acpLogManager) removeFile(path string) {
	m.closeWriterForPath(path)
	_ = os.Remove(path)
}

// prune enforces the age cap then the total-file cap, deleting oldest-by-mtime
// first. Non-log files in the directory are left untouched. Mirrors the
// keep-newest-N shape of persistence.pruneBackups, generalized to age+count.
func (m *acpLogManager) prune(now time.Time) {
	files := m.listLogFiles()

	// Age cap: drop files last written before the retention cutoff. An open
	// writer whose buffered last-write is recent keeps its file alive even when
	// the on-disk mtime still looks stale (the flush hasn't landed yet), so a
	// retention tick that races ahead of the flush tick can't delete a file
	// that's actively being written.
	if m.cfg.retention > 0 {
		cutoff := now.Add(-m.cfg.retention)
		survivors := make([]logFileInfo, 0, len(files))
		for _, f := range files {
			if f.mtime.Before(cutoff) && !m.writerActiveSince(f.path, cutoff) {
				m.removeFile(f.path)
			} else {
				survivors = append(survivors, f)
			}
		}
		files = survivors
	}

	// Count cap: keep only the newest maxFiles by mtime.
	if m.cfg.maxFiles > 0 && len(files) > m.cfg.maxFiles {
		sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
		for _, f := range files[m.cfg.maxFiles:] {
			m.removeFile(f.path)
		}
	}
}

// acpWriter is a single kept-open, line-buffered file handle with its own lock
// and rotation on a byte cap.
type acpWriter struct {
	mu        sync.Mutex
	path      string
	f         *os.File
	buf       *bufio.Writer
	size      int64
	maxBytes  int64
	seq       int
	lastWrite time.Time
}

func (w *acpWriter) writeLine(line []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		// A prior open/rotate left this writer without a handle. Retry here so
		// a transient FS error doesn't permanently disable a session's logging
		// (the writer stays cached in the manager map and self-heals).
		w.reopen(os.O_APPEND, true /* keepSize */)
		if w.f == nil {
			return
		}
	}
	need := int64(len(line) + 1)
	if w.size > 0 && w.size+need > w.maxBytes {
		w.rotate()
	}
	if w.f == nil {
		return
	}
	_, _ = w.buf.Write(line)
	_ = w.buf.WriteByte('\n')
	w.size += need
	w.lastWrite = time.Now()
}

// rotate rolls the active file to a numbered sibling that keeps the reader's
// prefix/suffix, then reopens a fresh, empty active file. Caller holds w.mu.
func (w *acpWriter) rotate() {
	_ = w.buf.Flush()
	_ = w.f.Close()

	rotated := w.nextRotatedPath()
	if err := os.Rename(w.path, rotated); err != nil {
		// Rotation failed (e.g. a transient FS error). Reopen the existing file
		// in append mode rather than truncating it, so the current segment is
		// preserved; the roll is retried on the next write.
		log.Printf("[DEBUG] acplog: rotate %s: %v", w.path, err)
		w.reopen(os.O_APPEND, true /* keepSize */)
		return
	}
	w.reopen(os.O_TRUNC, false /* keepSize */)
}

// nextRotatedPath returns a rotated sibling filename that does not yet exist.
// seq resets to 0 on every fresh writer (restart, idle reopen), so probing for
// a free slot is what prevents clobbering an older rotated segment.
func (w *acpWriter) nextRotatedPath() string {
	base := strings.TrimSuffix(w.path, jsonlSuffix)
	for {
		w.seq++
		candidate := base + "." + strconv.Itoa(w.seq) + jsonlSuffix
		// Only skip a candidate that definitively exists; treat NotExist (and
		// any other stat error) as "safe to use" so we never loop forever.
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}

// reopen re-opens w.path with the given mode bit (O_TRUNC for a fresh segment,
// O_APPEND to keep the existing one) and resets the buffered writer. keepSize
// preserves the on-disk size so a failed rotation retries on the next write.
func (w *acpWriter) reopen(modeBit int, keepSize bool) {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|modeBit, acpFilePerm)
	if err != nil {
		log.Printf("[DEBUG] acplog: reopen %s: %v", w.path, err)
		w.f = nil
		w.buf = nil
		return
	}
	w.f = f
	w.buf = bufio.NewWriter(f)
	if keepSize {
		if info, statErr := f.Stat(); statErr == nil {
			w.size = info.Size()
		}
	} else {
		w.size = 0
	}
}

func (w *acpWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf != nil {
		_ = w.buf.Flush()
	}
}

func (w *acpWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf != nil {
		_ = w.buf.Flush()
		w.buf = nil
	}
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}
}

func (w *acpWriter) idleSince(cutoff time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastWrite.Before(cutoff)
}

// --- filename helpers ---

func rawFileName(protocol, agentID, sessionID string) string {
	return rawPrefix + fileKey(protocol, agentID, sessionID) + jsonlSuffix
}

func normalizedFileName(protocol, agentID, sessionID string) string {
	return normalizedPrefix + fileKey(protocol, agentID, sessionID) + jsonlSuffix
}

func fileKey(protocol, agentID, sessionID string) string {
	return sanitizeFilenamePart(protocol) + "-" +
		sanitizeFilenamePart(agentID) + "-" +
		sanitizeFilenamePart(sessionID)
}

// sanitizeFilenamePart keeps only filename-safe characters so a hostile or
// path-like session id can't escape the log dir or break Windows filenames.
func sanitizeFilenamePart(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// --- ring buffer ---

// ringBuffer is a fixed-capacity FIFO of the most recent normalized entries
// for one session, used by the dev-only live-tail endpoint.
type ringBuffer struct {
	mu        sync.Mutex
	entries   []json.RawMessage
	capacity  int
	lastWrite time.Time
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = defaultACPRingSize
	}
	return &ringBuffer{capacity: capacity}
}

func (r *ringBuffer) add(entry json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	if len(r.entries) > r.capacity {
		// Drop oldest. Pre-size with one spare slot so the next append lands in
		// the existing backing array instead of reallocating every time.
		trimmed := make([]json.RawMessage, r.capacity, r.capacity+1)
		copy(trimmed, r.entries[len(r.entries)-r.capacity:])
		r.entries = trimmed
	}
	r.lastWrite = time.Now()
}

func (r *ringBuffer) idleSince(cutoff time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastWrite.Before(cutoff)
}

func (r *ringBuffer) tail(n int) []json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.entries) {
		n = len(r.entries)
	}
	out := make([]json.RawMessage, n)
	copy(out, r.entries[len(r.entries)-n:])
	return out
}
