// Package journal provides the bounded, owner-scoped delivery journal used by
// agentctl. It stores normalized events and immutable submission outcomes, not
// a second canonical task transcript.
package journal

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	CurrentVersion         uint32 = 1
	DefaultMaxEventBytes          = 1 << 20
	DefaultMaxStreamBytes         = 256 << 20
	DefaultMaxJournalBytes        = 2 << 30
	DefaultReserveBytes           = 1 << 20
)

var (
	ErrJournalCorrupt       = errors.New("agent delivery journal is corrupt")
	ErrJournalNewerVersion  = errors.New("agent delivery journal uses a newer version")
	ErrJournalFull          = errors.New("agent delivery journal is full")
	ErrStreamFull           = errors.New("agent delivery stream is full")
	ErrStreamNotFound       = errors.New("agent delivery stream not found")
	ErrCursorExpired        = errors.New("agent delivery cursor expired")
	ErrSequenceConflict     = errors.New("agent delivery sequence conflict")
	ErrSubmissionConflict   = errors.New("agent delivery submission conflict")
	ErrSubmissionNotFound   = errors.New("agent delivery submission not found")
	ErrSubmissionState      = errors.New("invalid agent delivery submission transition")
	ErrSubmissionGeneration = errors.New("agent delivery submission generation is not eligible for retirement")
	ErrOwnerMismatch        = errors.New("agent delivery owner mismatch")
)

var (
	bucketMeta        = []byte("meta")
	bucketEvents      = []byte("events")
	bucketStreamMeta  = []byte("stream_meta")
	bucketSubmissions = []byte("submissions")
	keyVersion        = []byte("version")
	keyJournalBytes   = []byte("journal_bytes")
)

type Config struct {
	Path            string
	MaxEventBytes   int64
	MaxStreamBytes  int64
	MaxJournalBytes int64
	ReserveBytes    int64
	OpenTimeout     time.Duration
}

func (c Config) withDefaults() Config {
	if c.MaxEventBytes <= 0 {
		c.MaxEventBytes = DefaultMaxEventBytes
	}
	if c.MaxStreamBytes <= 0 {
		c.MaxStreamBytes = DefaultMaxStreamBytes
	}
	if c.MaxJournalBytes <= 0 {
		c.MaxJournalBytes = DefaultMaxJournalBytes
	}
	if c.ReserveBytes <= 0 {
		c.ReserveBytes = DefaultReserveBytes
	}
	if c.OpenTimeout <= 0 {
		c.OpenTimeout = 2 * time.Second
	}
	return c
}

type Event struct {
	SessionID         string    `json:"session_id"`
	IncarnationID     string    `json:"incarnation_id"`
	HarnessGeneration uint64    `json:"harness_generation"`
	StreamID          string    `json:"stream_id"`
	Sequence          uint64    `json:"sequence"`
	SubmissionID      string    `json:"submission_id,omitempty"`
	Type              string    `json:"type"`
	Payload           []byte    `json:"payload"`
	Terminal          bool      `json:"terminal,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type Stream struct {
	SessionID         string `json:"session_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration uint64 `json:"harness_generation"`
	StreamID          string `json:"stream_id"`
	HighWater         uint64 `json:"high_water"`
	Acknowledged      uint64 `json:"acknowledged"`
	FirstRetained     uint64 `json:"first_retained"`
	Bytes             int64  `json:"bytes"`
	Sealed            bool   `json:"sealed"`
}

type SubmissionState string

const (
	SubmissionPrepared           SubmissionState = "prepared"
	SubmissionAccepted           SubmissionState = "accepted"
	SubmissionDispatching        SubmissionState = "dispatching"
	SubmissionCompleted          SubmissionState = "completed"
	SubmissionFailed             SubmissionState = "failed"
	SubmissionCancelled          SubmissionState = "cancelled"
	SubmissionInterruptedUnknown SubmissionState = "interrupted_unknown"
)

type Submission struct {
	ID                    string          `json:"id"`
	SessionID             string          `json:"session_id"`
	IncarnationID         string          `json:"incarnation_id"`
	HarnessGeneration     uint64          `json:"harness_generation"`
	Hash                  string          `json:"hash"`
	Payload               []byte          `json:"payload"`
	State                 SubmissionState `json:"state"`
	TerminalEventRetained bool            `json:"terminal_event_retained,omitempty"`
	TerminalSequence      uint64          `json:"terminal_sequence,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type Journal struct {
	mu     sync.RWMutex
	db     *bolt.DB
	config Config
}

func Open(config Config) (journal *Journal, err error) {
	defer func() {
		if err != nil {
			RecordJournalError(classifyJournalError(err))
		}
	}()
	config = config.withDefaults()
	if config.Path == "" {
		return nil, fmt.Errorf("journal path is required")
	}
	if err := os.MkdirAll(filepath.Dir(config.Path), 0o700); err != nil {
		return nil, fmt.Errorf("create journal directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(config.Path), 0o700); err != nil {
		return nil, fmt.Errorf("secure journal directory: %w", err)
	}
	db, err := bolt.Open(config.Path, 0o600, &bolt.Options{Timeout: config.OpenTimeout, NoSync: false})
	if err != nil {
		return nil, fmt.Errorf("open delivery journal: %w", err)
	}
	journal = &Journal{db: db, config: config}
	if err := journal.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	journal.refreshMetrics()
	return journal, nil
}

func (j *Journal) initialize() error {
	err := j.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(bucketMeta)
		if err != nil {
			return err
		}
		version := meta.Get(keyVersion)
		if version == nil {
			return initializeFreshJournal(tx, meta)
		}
		if err := validateExistingJournal(tx, meta, version); err != nil {
			return err
		}
		return recalculateJournalBytes(tx)
	})
	if err != nil {
		return fmt.Errorf("initialize delivery journal: %w", err)
	}
	return nil
}

func initializeFreshJournal(tx *bolt.Tx, meta *bolt.Bucket) error {
	if meta.Stats().KeyN != 0 || tx.Bucket(bucketEvents) != nil || tx.Bucket(bucketStreamMeta) != nil || tx.Bucket(bucketSubmissions) != nil {
		return ErrJournalCorrupt
	}
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], CurrentVersion)
	if err := meta.Put(keyVersion, encoded[:]); err != nil {
		return err
	}
	if err := meta.Put(keyJournalBytes, encodeInt64(0)); err != nil {
		return err
	}
	return createJournalBuckets(tx)
}

func validateExistingJournal(tx *bolt.Tx, meta *bolt.Bucket, version []byte) error {
	if len(version) != 4 {
		return ErrJournalCorrupt
	}
	switch got := binary.BigEndian.Uint32(version); {
	case got > CurrentVersion:
		return ErrJournalNewerVersion
	case got != CurrentVersion:
		return ErrJournalCorrupt
	}
	if err := validateJournalBuckets(tx); err != nil {
		return err
	}
	_, err := decodeInt64(meta.Get(keyJournalBytes))
	return err
}

func createJournalBuckets(tx *bolt.Tx) error {
	for _, name := range [][]byte{bucketEvents, bucketStreamMeta, bucketSubmissions} {
		if _, err := tx.CreateBucketIfNotExists(name); err != nil {
			return err
		}
	}
	return nil
}

func validateJournalBuckets(tx *bolt.Tx) error {
	for _, name := range [][]byte{bucketEvents, bucketStreamMeta, bucketSubmissions} {
		if tx.Bucket(name) == nil {
			return ErrJournalCorrupt
		}
	}
	return nil
}

func recalculateJournalBytes(tx *bolt.Tx) error {
	var total int64
	add := func(raw []byte) error {
		if raw == nil {
			return nil
		}
		if int64(len(raw)) > (int64(^uint64(0)>>1) - total) {
			return ErrJournalCorrupt
		}
		total += int64(len(raw))
		return nil
	}
	events := tx.Bucket(bucketEvents)
	if err := events.ForEach(func(streamID, value []byte) error {
		if value != nil {
			return ErrJournalCorrupt
		}
		return events.Bucket(streamID).ForEach(func(_, event []byte) error { return add(event) })
	}); err != nil {
		return err
	}
	if err := tx.Bucket(bucketSubmissions).ForEach(func(_, submission []byte) error { return add(submission) }); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(total))
}

func (j *Journal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return nil
	}
	err := j.db.Close()
	j.db = nil
	return err
}

// Compact rewrites the journal into a sibling temporary database and swaps it
// into place while all journal operations are excluded. A crash before the
// atomic rename leaves the original database intact; a crash after it leaves
// the fully closed compacted database at the canonical path.
func (j *Journal) Compact(ctx context.Context) error {
	if j == nil {
		return errors.New("journal is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return errors.New("journal is closed")
	}
	temporaryPath := fmt.Sprintf("%s.compact-%d", j.config.Path, time.Now().UnixNano())
	defer func() { _ = os.Remove(temporaryPath) }()
	destination, err := bolt.Open(temporaryPath, 0o600, &bolt.Options{Timeout: j.config.OpenTimeout, NoSync: false})
	if err != nil {
		return fmt.Errorf("open compacted journal: %w", err)
	}
	compactErr := bolt.Compact(destination, j.db, 0)
	closeErr := destination.Close()
	if compactErr != nil || closeErr != nil {
		return errors.Join(compactErr, closeErr)
	}
	if err := j.db.Close(); err != nil {
		return fmt.Errorf("close journal before compaction swap: %w", err)
	}
	if err := os.Rename(temporaryPath, j.config.Path); err != nil {
		reopened, reopenErr := bolt.Open(j.config.Path, 0o600, &bolt.Options{Timeout: j.config.OpenTimeout, NoSync: false})
		if reopenErr == nil {
			j.db = reopened
		}
		return errors.Join(fmt.Errorf("replace compacted journal: %w", err), reopenErr)
	}
	reopened, err := bolt.Open(j.config.Path, 0o600, &bolt.Options{Timeout: j.config.OpenTimeout, NoSync: false})
	if err != nil {
		j.db = nil
		return fmt.Errorf("reopen compacted journal: %w", err)
	}
	j.db = reopened
	return nil
}

// CompactIfNeeded performs idle maintenance after acknowledged records have
// created enough reclaimable bbolt pages to justify a rewrite. The logical
// quota remains authoritative; this only limits physical file growth.
func (j *Journal) CompactIfNeeded(ctx context.Context) error {
	if j == nil {
		return errors.New("journal is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(j.config.Path)
	if err != nil {
		return err
	}
	const minimumCompactionFileBytes int64 = 4 << 20
	if info.Size() < minimumCompactionFileBytes {
		return nil
	}
	j.mu.RLock()
	if j.db == nil {
		j.mu.RUnlock()
		return errors.New("journal is closed")
	}
	var logicalBytes int64
	err = j.db.View(func(tx *bolt.Tx) error {
		var decodeErr error
		logicalBytes, decodeErr = decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		return decodeErr
	})
	j.mu.RUnlock()
	if err != nil {
		return err
	}
	if info.Size() <= logicalBytes*2+minimumCompactionFileBytes {
		return nil
	}
	return j.Compact(ctx)
}

// MaxEventBytes returns the configured logical payload limit shared by event
// and submission admission.
func (j *Journal) MaxEventBytes() int64 {
	if j == nil {
		return 0
	}
	return j.config.MaxEventBytes
}

func (j *Journal) Append(ctx context.Context, event Event) (Event, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	if event.StreamID == "" || event.SessionID == "" || event.Type == "" {
		return Event{}, fmt.Errorf("event stream, session, and type are required")
	}
	if len(event.Payload) > int(j.config.MaxEventBytes) {
		return Event{}, fmt.Errorf("%w: %d bytes", ErrStreamFull, len(event.Payload))
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	err := j.db.Update(func(tx *bolt.Tx) error { return j.appendEventTx(ctx, tx, &event) })
	if err != nil {
		if errors.Is(err, ErrSequenceConflict) || errors.Is(err, ErrOwnerMismatch) {
			RecordSequenceError(classifyJournalError(err))
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			RecordJournalError(classifyJournalError(err))
		}
		return Event{}, err
	}
	j.refreshMetricsLocked()
	return event, nil
}

func (j *Journal) appendEventTx(ctx context.Context, tx *bolt.Tx, event *Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stream, err := appendStream(tx, *event)
	if err != nil {
		return err
	}
	if err := assignAppendSequence(&stream, event); err != nil {
		return err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	newBytes := int64(len(encoded))
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return err
	}
	if err := validateAppendCapacity(j, stream, journalBytes, newBytes); err != nil {
		return err
	}
	stream.HighWater = event.Sequence
	stream.Bytes += newBytes
	if stream.FirstRetained == 0 {
		stream.FirstRetained = event.Sequence
	}
	if err := writeAppendedEvent(tx, stream, *event, encoded, journalBytes+newBytes); err != nil {
		return err
	}
	if event.Terminal && event.SubmissionID != "" {
		return markSubmissionTerminalTx(ctx, tx, event.SubmissionID, event.Sequence, j)
	}
	return nil
}

func appendStream(tx *bolt.Tx, event Event) (Stream, error) {
	streams := tx.Bucket(bucketStreamMeta)
	stream, err := decodeStream(streams.Get([]byte(event.StreamID)))
	if err != nil {
		return Stream{}, err
	}
	if stream.StreamID != "" && !sameStreamOwner(stream, event) {
		return Stream{}, ErrOwnerMismatch
	}
	if stream.StreamID == "" {
		return Stream{
			SessionID:         event.SessionID,
			IncarnationID:     event.IncarnationID,
			HarnessGeneration: event.HarnessGeneration,
			StreamID:          event.StreamID,
		}, nil
	}
	if stream.Sealed {
		return Stream{}, ErrOwnerMismatch
	}
	return stream, nil
}

func assignAppendSequence(stream *Stream, event *Event) error {
	if event.Sequence == 0 {
		if stream.HighWater == ^uint64(0) {
			return ErrSequenceConflict
		}
		event.Sequence = stream.HighWater + 1
	}
	if event.Sequence != stream.HighWater+1 {
		return ErrSequenceConflict
	}
	return nil
}

func sameStreamOwner(stream Stream, event Event) bool {
	return stream.SessionID == event.SessionID &&
		stream.IncarnationID == event.IncarnationID &&
		stream.HarnessGeneration == event.HarnessGeneration
}

func validateAppendCapacity(j *Journal, stream Stream, journalBytes, eventBytes int64) error {
	if stream.Bytes+eventBytes > j.config.MaxStreamBytes {
		return ErrStreamFull
	}
	return validateJournalCapacity(j, journalBytes, eventBytes)
}

func validateJournalCapacity(j *Journal, journalBytes, additionalBytes int64) error {
	if additionalBytes <= 0 {
		return nil
	}
	if journalBytes < 0 || additionalBytes > j.config.MaxJournalBytes-j.config.ReserveBytes-journalBytes {
		return ErrJournalFull
	}
	return nil
}

func writeAppendedEvent(tx *bolt.Tx, stream Stream, event Event, encoded []byte, journalBytes int64) error {
	streamBytes, err := json.Marshal(stream)
	if err != nil {
		return err
	}
	if err := tx.Bucket(bucketStreamMeta).Put([]byte(event.StreamID), streamBytes); err != nil {
		return err
	}
	streamBucket, err := tx.Bucket(bucketEvents).CreateBucketIfNotExists([]byte(event.StreamID))
	if err != nil {
		return err
	}
	if err := streamBucket.Put(sequenceKey(event.Sequence), encoded); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes))
}

func (j *Journal) Replay(ctx context.Context, streamID string, after uint64, limit int) ([]Event, Stream, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if limit <= 0 {
		limit = 1000
	}
	var out []Event
	var stream Stream
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var decodeErr error
		stream, decodeErr = decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(streamID)))
		if decodeErr != nil {
			return decodeErr
		}
		if stream.StreamID == "" {
			return ErrStreamNotFound
		}
		if stream.FirstRetained != 0 && after+1 < stream.FirstRetained {
			return ErrCursorExpired
		}
		events, replayErr := replayEvents(tx, streamID, after, limit)
		out = events
		return replayErr
	})
	if err != nil {
		if errors.Is(err, ErrCursorExpired) || errors.Is(err, ErrSequenceConflict) {
			RecordSequenceError(classifyJournalError(err))
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			RecordJournalError(classifyJournalError(err))
		}
	} else {
		RecordReplayedEvents(len(out))
	}
	return out, stream, err
}

func replayEvents(tx *bolt.Tx, streamID string, after uint64, limit int) ([]Event, error) {
	if after == ^uint64(0) {
		return nil, nil
	}
	bucket := tx.Bucket(bucketEvents).Bucket([]byte(streamID))
	if bucket == nil {
		return nil, nil
	}
	var out []Event
	cursor := bucket.Cursor()
	for key, value := cursor.Seek(sequenceKey(after + 1)); key != nil && len(out) < limit; key, value = cursor.Next() {
		var event Event
		if err := json.Unmarshal(value, &event); err != nil {
			return nil, ErrJournalCorrupt
		}
		out = append(out, event)
	}
	return out, nil
}

func (j *Journal) Acknowledge(ctx context.Context, streamID string, sequence uint64) error {
	j.mu.RLock()
	err := j.db.Update(func(tx *bolt.Tx) error {
		return acknowledgeStreamTx(ctx, tx, streamID, sequence)
	})
	j.mu.RUnlock()
	if err != nil {
		if errors.Is(err, ErrSequenceConflict) {
			RecordSequenceError(classifyJournalError(err))
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			RecordJournalError(classifyJournalError(err))
		}
	} else {
		j.refreshMetrics()
		if compactErr := j.CompactIfNeeded(ctx); compactErr != nil {
			RecordJournalError(classifyJournalError(compactErr))
		}
	}
	return err
}

func acknowledgeStreamTx(ctx context.Context, tx *bolt.Tx, streamID string, sequence uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	meta := tx.Bucket(bucketMeta)
	streams := tx.Bucket(bucketStreamMeta)
	stream, err := decodeStream(streams.Get([]byte(streamID)))
	if err != nil {
		return err
	}
	if stream.StreamID == "" || sequence > stream.HighWater {
		return ErrSequenceConflict
	}
	if sequence <= stream.Acknowledged {
		return nil
	}
	bucket := tx.Bucket(bucketEvents).Bucket([]byte(streamID))
	if bucket == nil {
		return ErrJournalCorrupt
	}
	journalBytes, err := decodeInt64(meta.Get(keyJournalBytes))
	if err != nil {
		return err
	}
	journalBytes, err = deleteAcknowledgedEvents(bucket, sequence, journalBytes, &stream)
	if err != nil {
		return err
	}
	stream.Acknowledged = sequence
	if sequence == ^uint64(0) {
		stream.FirstRetained = 0
	} else {
		stream.FirstRetained = sequence + 1
	}
	encoded, err := json.Marshal(stream)
	if err != nil {
		return err
	}
	if err := meta.Put(keyJournalBytes, encodeInt64(journalBytes)); err != nil {
		return err
	}
	return streams.Put([]byte(streamID), encoded)
}

func deleteAcknowledgedEvents(bucket *bolt.Bucket, sequence uint64, journalBytes int64, stream *Stream) (int64, error) {
	cursor := bucket.Cursor()
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		if binary.BigEndian.Uint64(key) > sequence {
			break
		}
		stream.Bytes -= int64(len(value))
		journalBytes -= int64(len(value))
		if journalBytes < 0 {
			journalBytes = 0
		}
		if err := bucket.Delete(key); err != nil {
			return 0, err
		}
	}
	return journalBytes, nil
}

func (j *Journal) GetStream(ctx context.Context, streamID string) (Stream, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var stream Stream
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var decodeErr error
		stream, decodeErr = decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(streamID)))
		if decodeErr != nil {
			return decodeErr
		}
		if stream.StreamID == "" {
			return ErrStreamNotFound
		}
		return nil
	})
	return stream, err
}
