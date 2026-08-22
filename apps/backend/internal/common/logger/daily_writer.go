package logger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type dailyWriter struct {
	mu             sync.Mutex
	logDir         string
	now            func() time.Time
	day            string
	file           *os.File
	size           int64
	retainedBytes  int64
	maxBytes       int64
	maxTotalBytes  int64
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
	writer := &dailyWriter{
		logDir: logDir, now: now, maxBytes: backendLogSegmentBytes,
		maxTotalBytes: backendLogTotalBytes,
	}
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
	if err := w.recoverConversion(); err != nil {
		return fmt.Errorf("recover log conversion: %w", err)
	}
	w.recordMaintenance(w.cleanup(today))

	day, err := w.readActiveDay(today)
	if err != nil {
		return err
	}
	active, exists, err := w.activeInfo()
	if err != nil {
		return err
	}
	if exists && active.Size > w.maxBytes {
		if err := w.convertActive(day, active.Size); err != nil {
			return fmt.Errorf("convert oversized active log: %w", err)
		}
		exists = false
	}
	if exists && day != today {
		if err := w.rotateActive(day); err != nil {
			return fmt.Errorf("roll over %s: %w", day, err)
		}
	}
	if err := w.openActive(today); err != nil {
		return err
	}
	w.recordMaintenance(w.cleanup(today))
	w.recordMaintenance(w.refreshRetainedBytes())
	w.recordMaintenance(w.enforceBudget(0))
	return nil
}

func (w *dailyWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if int64(len(data)) > w.maxBytes {
		return 0, fmt.Errorf("backend log entry exceeds segment limit")
	}
	today := utcDay(w.now())
	if today != w.day {
		if err := w.switchDay(today); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotateActive(w.day); err != nil {
			return 0, err
		}
		if err := w.openActive(w.day); err != nil {
			return 0, err
		}
	}
	if err := w.enforceBudget(int64(len(data))); err != nil {
		return 0, err
	}
	written, err := w.file.Write(data)
	w.size += int64(written)
	w.retainedBytes += int64(written)
	return written, err
}

func (w *dailyWriter) switchDay(today string) error {
	if err := w.rotateActive(w.day); err != nil {
		return err
	}
	if err := w.openActive(today); err != nil {
		return err
	}
	w.recordMaintenance(w.cleanup(today))
	w.recordMaintenance(w.refreshRetainedBytes())
	w.recordMaintenance(w.enforceBudget(0))
	return nil
}

func (w *dailyWriter) rotateActive(day string) error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	active, exists, err := w.activeInfo()
	if err != nil {
		return err
	}
	if !exists {
		return removeIfExists(filepath.Join(w.logDir, backendDayMarkerName))
	}
	if active.Size == 0 {
		if err := os.Remove(active.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return removeIfExists(filepath.Join(w.logDir, backendDayMarkerName))
	}
	sequence, err := w.nextSequence(day)
	if err != nil {
		return err
	}
	destination := filepath.Join(w.logDir, backendLogSegmentName(day, sequence))
	if err := renameExclusive(active.Path, destination); err != nil {
		return fmt.Errorf("rename active log: %w", err)
	}
	return removeIfExists(filepath.Join(w.logDir, backendDayMarkerName))
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
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat active log: %w", err)
	}
	if _, err := w.nextSequence(day); err != nil {
		_ = file.Close()
		return err
	}
	w.file, w.day, w.size = file, day, info.Size()
	return nil
}

func (w *dailyWriter) activeInfo() (backendLogFileInfo, bool, error) {
	path := filepath.Join(w.logDir, activeBackendLogName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return backendLogFileInfo{}, false, nil
	}
	if err != nil {
		return backendLogFileInfo{}, false, fmt.Errorf("stat active log: %w", err)
	}
	if !info.Mode().IsRegular() {
		return backendLogFileInfo{}, false, fmt.Errorf("active log is not a regular file")
	}
	return backendLogFileInfo{
		Name: activeBackendLogName, Path: path,
		BackendLogFile: BackendLogFile{Active: true}, Size: info.Size(), ModTime: info.ModTime(),
	}, true, nil
}

func (w *dailyWriter) readActiveDay(today string) (string, error) {
	active, exists, err := w.activeInfo()
	if err != nil {
		return "", err
	}
	if !exists {
		return today, nil
	}
	data, err := os.ReadFile(filepath.Join(w.logDir, backendDayMarkerName))
	if err == nil && validUTCDay(string(data)) {
		return string(data), nil
	}
	return active.ModTime.UTC().Format(time.DateOnly), nil
}

func (w *dailyWriter) nextSequence(day string) (int, error) {
	files, err := scanBackendLogFiles(w.logDir)
	if err != nil {
		return 0, err
	}
	return nextBackendLogSequence(files, day)
}

func (w *dailyWriter) enforceBudget(additional int64) error {
	if additional < 0 || w.maxTotalBytes <= 0 {
		return fmt.Errorf("invalid backend log budget")
	}
	if w.retainedBytes+additional <= w.maxTotalBytes {
		return nil
	}
	files, err := scanBackendLogFiles(w.logDir)
	if err != nil {
		return err
	}
	total := backendLogBytes(files)
	closed := closedBackendLogFiles(files)
	sortBackendLogFiles(closed)
	for total+additional > w.maxTotalBytes && len(closed) > 0 {
		oldest := closed[0]
		if err := os.Remove(oldest.Path); err != nil {
			return fmt.Errorf("remove oldest backend log %s: %w", oldest.Name, err)
		}
		total -= oldest.Size
		closed = closed[1:]
	}
	w.retainedBytes = total
	if total+additional > w.maxTotalBytes {
		return fmt.Errorf("backend log budget exceeded")
	}
	return nil
}

func (w *dailyWriter) refreshRetainedBytes() error {
	files, err := scanBackendLogFiles(w.logDir)
	if err != nil {
		return err
	}
	w.retainedBytes = backendLogBytes(files)
	return nil
}

func (w *dailyWriter) cleanup(today string) error {
	files, err := scanBackendLogFiles(w.logDir)
	if err != nil {
		return err
	}
	todayTime, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Active || file.Day == "" {
			continue
		}
		if !BackendLogDayRetained(file.Day, todayTime) {
			if err := os.Remove(file.Path); err != nil {
				return fmt.Errorf("remove expired backend log %s: %w", file.Name, err)
			}
		}
	}
	return nil
}

func (w *dailyWriter) recordMaintenance(err error) {
	if err != nil {
		w.maintenanceErr = errors.Join(w.maintenanceErr, err)
	}
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

func utcDay(now time.Time) string {
	return now.UTC().Format(time.DateOnly)
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

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renameExclusive(source, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, destination)
}
