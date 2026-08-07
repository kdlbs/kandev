package logger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const (
	activeBackendLogName = "backend-logs.log"
	backendDayMarkerName = ".backend-logs-day"
	rolloverJournalName  = ".backend-logs-rollover.json"
	dailyBackendLogBytes = 256 * 1024 * 1024
)

var datedBackendLogPattern = regexp.MustCompile(`^backend-logs-(\d{4}-\d{2}-\d{2})\.log$`)
var errDailyBackendLogLimit = errors.New("daily backend log byte limit reached")

type rolloverJournal struct {
	SourceDay         string `json:"source_day"`
	SourceSize        int64  `json:"source_size"`
	SourceModUnixNano int64  `json:"source_mod_unix_nano"`
	Destination       string `json:"destination"`
	DestinationStart  int64  `json:"destination_start"`
	CopiedOffset      int64  `json:"copied_offset"`
}

type dailyWriter struct {
	mu             sync.Mutex
	logDir         string
	now            func() time.Time
	day            string
	file           *os.File
	size           int64
	maxBytes       int64
	closed         bool
	maintenanceErr error
}

func newDailyWriter(logDir string, now func() time.Time) (*dailyWriter, error) {
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := os.Chmod(logDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure log directory: %w", err)
	}
	writer := &dailyWriter{logDir: logDir, now: now, maxBytes: dailyBackendLogBytes}
	if err := writer.prepare(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *dailyWriter) prepare() error {
	today := utcDay(w.now())
	if err := w.recoverRollover(); err != nil {
		return fmt.Errorf("recover rollover: %w", err)
	}
	day, err := w.readActiveDay(today)
	if err != nil {
		return err
	}
	if day != today {
		if err := w.rollover(day); err != nil {
			return fmt.Errorf("roll over %s: %w", day, err)
		}
	}
	if err := w.openActive(today); err != nil {
		return err
	}
	w.maintenanceErr = w.cleanup(today)
	return nil
}

func (w *dailyWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	today := utcDay(w.now())
	if today != w.day {
		if err := w.switchDay(today); err != nil {
			// Preserve the triggering entry in the active file. The next write
			// retries rollover while the marker still names the prior day.
			if w.file == nil {
				return 0, err
			}
			return w.file.Write(data)
		}
	}
	if w.size+int64(len(data)) > w.maxBytes {
		return 0, errDailyBackendLogLimit
	}
	written, err := w.file.Write(data)
	w.size += int64(written)
	return written, err
}

func (w *dailyWriter) switchDay(today string) error {
	previous := w.day
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	if err := w.rollover(previous); err != nil {
		return w.reopenPrevious(previous, err)
	}
	if err := w.openActive(today); err != nil {
		return err
	}
	w.maintenanceErr = w.cleanup(today)
	return nil
}

func (w *dailyWriter) reopenPrevious(day string, rolloverErr error) error {
	file, openErr := openOwnerAppend(filepath.Join(w.logDir, activeBackendLogName))
	if openErr != nil {
		return errors.Join(rolloverErr, openErr)
	}
	w.file = file
	w.day = day
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		w.file = nil
		return errors.Join(rolloverErr, statErr)
	}
	w.size = info.Size()
	return rolloverErr
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *dailyWriter) MaintenanceError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maintenanceErr
}

func (w *dailyWriter) readActiveDay(today string) (string, error) {
	activePath := filepath.Join(w.logDir, activeBackendLogName)
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		return today, nil
	} else if err != nil {
		return "", fmt.Errorf("stat active log: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(w.logDir, backendDayMarkerName))
	if err == nil {
		if day := string(data); validUTCDay(day) {
			return day, nil
		}
	}
	info, err := os.Stat(activePath)
	if err != nil {
		return "", fmt.Errorf("stat unmarked active log: %w", err)
	}
	return info.ModTime().UTC().Format(time.DateOnly), nil
}

func (w *dailyWriter) openActive(day string) error {
	file, err := openOwnerAppend(filepath.Join(w.logDir, activeBackendLogName))
	if err != nil {
		return fmt.Errorf("open active log: %w", err)
	}
	if err := writeOwnerFile(filepath.Join(w.logDir, backendDayMarkerName), []byte(day)); err != nil {
		_ = file.Close()
		return fmt.Errorf("write active day marker: %w", err)
	}
	w.file = file
	w.day = day
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		w.file = nil
		return fmt.Errorf("stat active log: %w", err)
	}
	w.size = info.Size()
	return nil
}

func (w *dailyWriter) rollover(day string) error {
	activePath := filepath.Join(w.logDir, activeBackendLogName)
	info, err := os.Stat(activePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	journal, err := w.loadOrCreateJournal(day, info)
	if err != nil {
		return err
	}
	if err := w.copyRollover(activePath, journal); err != nil {
		return err
	}
	if err := os.Remove(activePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(filepath.Join(w.logDir, backendDayMarkerName))
	return removeIfExists(filepath.Join(w.logDir, rolloverJournalName))
}

func (w *dailyWriter) loadOrCreateJournal(day string, info os.FileInfo) (*rolloverJournal, error) {
	path := filepath.Join(w.logDir, rolloverJournalName)
	if data, err := os.ReadFile(path); err == nil {
		var journal rolloverJournal
		if err := json.Unmarshal(data, &journal); err != nil {
			return nil, fmt.Errorf("decode rollover journal: %w", err)
		}
		return &journal, nil
	}
	destination := fmt.Sprintf("backend-logs-%s.log", day)
	destinationInfo, err := os.Stat(filepath.Join(w.logDir, destination))
	var destinationStart int64
	if err == nil {
		destinationStart = destinationInfo.Size()
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	journal := &rolloverJournal{
		SourceDay: day, SourceSize: info.Size(), SourceModUnixNano: info.ModTime().UnixNano(),
		Destination: destination, DestinationStart: destinationStart,
	}
	return journal, writeJSONOwnerFile(path, journal)
}

func (w *dailyWriter) copyRollover(activePath string, journal *rolloverJournal) error {
	source, err := os.Open(activePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = source.Close() }()
	sourceInfo, err := source.Stat()
	if err != nil {
		return err
	}
	if sourceInfo.Size() != journal.SourceSize ||
		sourceInfo.ModTime().UnixNano() != journal.SourceModUnixNano {
		journal.SourceSize = sourceInfo.Size()
		journal.SourceModUnixNano = sourceInfo.ModTime().UnixNano()
		if err := writeJSONOwnerFile(filepath.Join(w.logDir, rolloverJournalName), journal); err != nil {
			return err
		}
	}
	destinationPath := filepath.Join(w.logDir, filepath.Base(journal.Destination))
	destination, err := openOwnerAppend(destinationPath)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()
	destinationInfo, err := destination.Stat()
	if err != nil {
		return err
	}
	copied := destinationInfo.Size() - journal.DestinationStart
	if copied < 0 || copied > journal.SourceSize {
		return fmt.Errorf("invalid rollover destination size")
	}
	journal.CopiedOffset = copied
	if _, err := source.Seek(copied, io.SeekStart); err != nil {
		return err
	}
	if err := copyJournaled(source, destination, journal, filepath.Join(w.logDir, rolloverJournalName)); err != nil {
		return err
	}
	return destination.Sync()
}

func copyJournaled(source io.Reader, destination io.Writer, journal *rolloverJournal, path string) error {
	buf := make([]byte, 1024*1024)
	for {
		n, readErr := source.Read(buf)
		if n > 0 {
			written, writeErr := destination.Write(buf[:n])
			journal.CopiedOffset += int64(written)
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			if err := writeJSONOwnerFile(path, journal); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (w *dailyWriter) recoverRollover() error {
	path := filepath.Join(w.logDir, rolloverJournalName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal rolloverJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return err
	}
	activePath := filepath.Join(w.logDir, activeBackendLogName)
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		return removeIfExists(path)
	}
	if err := w.copyRollover(activePath, &journal); err != nil {
		return err
	}
	if err := os.Remove(activePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return removeIfExists(path)
}

func (w *dailyWriter) cleanup(today string) error {
	entries, err := os.ReadDir(w.logDir)
	if err != nil {
		return err
	}
	cutoff, _ := time.Parse(time.DateOnly, today)
	cutoff = cutoff.AddDate(0, 0, -2)
	for _, entry := range entries {
		match := datedBackendLogPattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			continue
		}
		day, err := time.Parse(time.DateOnly, match[1])
		if err == nil && day.Before(cutoff) {
			if err := os.Remove(filepath.Join(w.logDir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func utcDay(now time.Time) string {
	return now.UTC().Format(time.DateOnly)
}

func validUTCDay(value string) bool {
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Format(time.DateOnly) == value
}

func openOwnerAppend(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		err = os.Chmod(path, 0o600)
	}
	return file, err
}

func writeOwnerFile(path string, data []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temp, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func writeJSONOwnerFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeOwnerFile(path, data)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
