package logger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const (
	activeBackendLogName         = "backend-logs.log"
	backendDayMarkerName         = ".backend-logs-day"
	rolloverJournalName          = ".backend-logs-rollover.json"
	conversionJournalName        = ".backend-logs-conversion.json"
	conversionSourceName         = ".backend-logs-conversion-source.log"
	backendLogSegmentBytes int64 = 16 * 1024 * 1024
	backendLogTotalBytes   int64 = 256 * 1024 * 1024
)

var (
	legacyBackendLogPattern  = regexp.MustCompile(`^backend-logs-(\d{4}-\d{2}-\d{2})\.log$`)
	segmentBackendLogPattern = regexp.MustCompile(`^backend-logs-(\d{4}-\d{2}-\d{2})-(\d{6})\.log$`)
)

// BackendLogFile describes the stable filename grammar shared by the logger
// and diagnostic bundle collector.
type BackendLogFile struct {
	Day      string
	Sequence int
	Active   bool
	Legacy   bool
}

type backendLogFileInfo struct {
	BackendLogFile
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// ParseBackendLogFilename parses active, numbered, and legacy backend log names.
func ParseBackendLogFilename(name string) (BackendLogFile, bool) {
	if name == activeBackendLogName {
		return BackendLogFile{Active: true}, true
	}
	if match := segmentBackendLogPattern.FindStringSubmatch(name); len(match) == 3 {
		sequence, err := strconv.Atoi(match[2])
		if err == nil && validUTCDay(match[1]) {
			return BackendLogFile{Day: match[1], Sequence: sequence}, true
		}
	}
	if match := legacyBackendLogPattern.FindStringSubmatch(name); len(match) == 2 && validUTCDay(match[1]) {
		return BackendLogFile{Day: match[1], Legacy: true}, true
	}
	return BackendLogFile{}, false
}

func backendLogSegmentName(day string, sequence int) string {
	return fmt.Sprintf("backend-logs-%s-%06d.log", day, sequence)
}

func scanBackendLogFiles(logDir string) ([]backendLogFileInfo, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	files := make([]backendLogFileInfo, 0, len(entries))
	for _, entry := range entries {
		parsed, ok := ParseBackendLogFilename(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(logDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, backendLogFileInfo{
			BackendLogFile: parsed, Name: entry.Name(), Path: path,
			Size: info.Size(), ModTime: info.ModTime(),
		})
	}
	return files, nil
}

func backendLogBytes(files []backendLogFileInfo) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func closedBackendLogFiles(files []backendLogFileInfo) []backendLogFileInfo {
	closed := make([]backendLogFileInfo, 0, len(files))
	for _, file := range files {
		if !file.Active {
			closed = append(closed, file)
		}
	}
	return closed
}

func sortBackendLogFiles(files []backendLogFileInfo) {
	sort.Slice(files, func(i, j int) bool {
		left, right := files[i], files[j]
		if left.Day != right.Day {
			return left.Day < right.Day
		}
		if left.Active != right.Active {
			return !left.Active
		}
		if left.Legacy != right.Legacy {
			return left.Legacy
		}
		return left.Sequence < right.Sequence
	})
}

func nextBackendLogSequence(files []backendLogFileInfo, day string) (int, error) {
	next := 1
	for _, file := range files {
		if file.Day == day && !file.Active && !file.Legacy && file.Sequence >= next {
			next = file.Sequence + 1
		}
	}
	if next > 999999 {
		return 0, fmt.Errorf("backend log segment sequence exhausted")
	}
	return next, nil
}

func validUTCDay(value string) bool {
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Format(time.DateOnly) == value
}

type rolloverJournal struct {
	SourceDay         string `json:"source_day"`
	SourceSize        int64  `json:"source_size"`
	SourceModUnixNano int64  `json:"source_mod_unix_nano"`
	Destination       string `json:"destination"`
	DestinationStart  int64  `json:"destination_start"`
	CopiedOffset      int64  `json:"copied_offset"`
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
	if sourceInfo.Size() != journal.SourceSize || sourceInfo.ModTime().UnixNano() != journal.SourceModUnixNano {
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

type conversionJournal struct {
	SourceDay         string             `json:"source_day"`
	SourceSize        int64              `json:"source_size"`
	SourceModUnixNano int64              `json:"source_mod_unix_nano"`
	TailOffset        int64              `json:"tail_offset"`
	BackupName        string             `json:"backup_name"`
	Outputs           []conversionOutput `json:"outputs"`
}

type conversionOutput struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

func (w *dailyWriter) convertActive(day string, sourceSize int64) error {
	if sourceSize <= w.maxBytes {
		return nil
	}
	files, err := scanBackendLogFiles(w.logDir)
	if err != nil {
		return err
	}
	closed := closedBackendLogFiles(files)
	closedBytes := backendLogBytes(closed)
	sortBackendLogFiles(closed)
	for closedBytes >= w.maxTotalBytes && len(closed) > 0 {
		oldest := closed[0]
		if err := os.Remove(oldest.Path); err != nil {
			return fmt.Errorf("remove old log during conversion: %w", err)
		}
		closedBytes -= oldest.Size
		closed = closed[1:]
	}
	available := w.maxTotalBytes - closedBytes
	if available <= 0 {
		return fmt.Errorf("no budget remains for active log conversion")
	}
	keep := sourceSize
	if keep > available {
		keep = available
	}
	firstSequence, err := nextBackendLogSequence(files, day)
	if err != nil {
		return err
	}
	outputs, err := conversionOutputs(day, keep, w.maxBytes, firstSequence)
	if err != nil {
		return err
	}
	journal := &conversionJournal{
		SourceDay: day, SourceSize: sourceSize, TailOffset: sourceSize - keep,
		BackupName: conversionSourceName, Outputs: outputs,
	}
	path := filepath.Join(w.logDir, conversionJournalName)
	if err := writeJSONOwnerFile(path, journal); err != nil {
		return err
	}
	if err := renameExclusive(filepath.Join(w.logDir, activeBackendLogName), filepath.Join(w.logDir, conversionSourceName)); err != nil {
		return err
	}
	return w.finishConversion(journal)
}

func conversionOutputs(day string, keep, segmentBytes int64, firstSequence int) ([]conversionOutput, error) {
	if keep <= 0 || segmentBytes <= 0 || firstSequence <= 0 {
		return nil, fmt.Errorf("invalid active log conversion limits")
	}
	outputs := make([]conversionOutput, 0, (keep+segmentBytes-1)/segmentBytes)
	for offset, sequence := int64(0), firstSequence; offset < keep; sequence++ {
		if sequence > 999999 {
			return nil, fmt.Errorf("backend log segment sequence exhausted")
		}
		length := keep - offset
		if length > segmentBytes {
			length = segmentBytes
		}
		outputs = append(outputs, conversionOutput{
			Name: backendLogSegmentName(day, sequence), Offset: offset, Length: length,
		})
		offset += length
	}
	return outputs, nil
}

func (w *dailyWriter) recoverConversion() error {
	path := filepath.Join(w.logDir, conversionJournalName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal conversionJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return err
	}
	if err := validateConversionJournal(&journal); err != nil {
		return err
	}
	activePath := filepath.Join(w.logDir, activeBackendLogName)
	backupPath := filepath.Join(w.logDir, journal.BackupName)
	if _, err := os.Lstat(backupPath); os.IsNotExist(err) {
		if _, activeErr := os.Lstat(activePath); activeErr == nil {
			if err := renameExclusive(activePath, backupPath); err != nil {
				return err
			}
		} else if !allConversionOutputsPresent(w.logDir, journal.Outputs) {
			return fmt.Errorf("conversion source is missing")
		}
	}
	if err := w.finishConversion(&journal); err != nil {
		return err
	}
	return nil
}

func (w *dailyWriter) finishConversion(journal *conversionJournal) error {
	if err := validateConversionJournal(journal); err != nil {
		return err
	}
	backupPath := filepath.Join(w.logDir, journal.BackupName)
	for _, output := range journal.Outputs {
		path := filepath.Join(w.logDir, output.Name)
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() != output.Length {
				return fmt.Errorf("conversion output %s is invalid", output.Name)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := copyFileRange(backupPath, path+".tmp", journal.TailOffset+output.Offset, output.Length); err != nil {
			return err
		}
		if err := renameExclusive(path+".tmp", path); err != nil {
			if !os.IsExist(err) {
				return err
			}
			_ = os.Remove(path + ".tmp")
		}
	}
	if err := removeIfExists(backupPath); err != nil {
		return err
	}
	return removeIfExists(filepath.Join(w.logDir, conversionJournalName))
}

func allConversionOutputsPresent(logDir string, outputs []conversionOutput) bool {
	for _, output := range outputs {
		info, err := os.Lstat(filepath.Join(logDir, output.Name))
		if err != nil || !info.Mode().IsRegular() || info.Size() != output.Length {
			return false
		}
	}
	return true
}

func validateConversionJournal(journal *conversionJournal) error {
	if journal.BackupName != conversionSourceName || !validUTCDay(journal.SourceDay) ||
		journal.SourceSize < 0 || journal.TailOffset < 0 || journal.TailOffset > journal.SourceSize {
		return fmt.Errorf("invalid conversion journal")
	}
	if len(journal.Outputs) == 0 {
		return fmt.Errorf("conversion output is empty")
	}
	var previousSequence int
	var total int64
	for index, output := range journal.Outputs {
		var err error
		total, previousSequence, err = validateConversionOutput(
			journal, output, index, previousSequence, total,
		)
		if err != nil {
			return err
		}
	}
	if journal.TailOffset+total > journal.SourceSize {
		return fmt.Errorf("conversion output exceeds source")
	}
	return nil
}

func validateConversionOutput(
	journal *conversionJournal,
	output conversionOutput,
	index, previousSequence int,
	total int64,
) (int64, int, error) {
	parsed, ok := ParseBackendLogFilename(output.Name)
	if !ok || parsed.Active || parsed.Legacy || parsed.Day != journal.SourceDay || output.Length <= 0 || output.Offset < 0 {
		return 0, 0, fmt.Errorf("invalid conversion output")
	}
	if index > 0 && parsed.Sequence <= previousSequence {
		return 0, 0, fmt.Errorf("conversion outputs are not ordered")
	}
	if output.Offset+output.Length > journal.SourceSize-journal.TailOffset || output.Offset != total {
		return 0, 0, fmt.Errorf("conversion output exceeds source range")
	}
	return total + output.Length, parsed.Sequence, nil
}

func copyFileRange(sourcePath, destinationPath string, offset, length int64) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(destination, source, length); err != nil {
		_ = destination.Close()
		_ = os.Remove(destinationPath)
		return err
	}
	if err := destination.Sync(); err != nil {
		_ = destination.Close()
		_ = os.Remove(destinationPath)
		return err
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	return nil
}

func writeJSONOwnerFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeOwnerFile(path, data)
}
