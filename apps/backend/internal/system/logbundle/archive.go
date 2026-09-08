package logbundle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	maxBackendBytes    = int64(160 * 1024 * 1024)
	maxACPBackendBytes = int64(96 * 1024 * 1024)
	maxACPBytes        = int64(96 * 1024 * 1024)
	copyChunkBytes     = int64(1024 * 1024)
	acpKindRaw         = "raw"
	acpKindNormalized  = "normalized"
)

type backendArchiveFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

type frontendArchiveFile struct {
	Name        string          `json:"name"`
	Bytes       int64           `json:"bytes"`
	Entries     int             `json:"entries"`
	StorageMode string          `json:"storage_mode,omitempty"`
	Metadata    json.RawMessage `json:"capture_metadata,omitempty"`
}

type acpArchiveFile struct {
	Name    string `json:"name"`
	Session string `json:"session"`
	Kind    string `json:"kind"`
	Size    int64  `json:"size"`
	Offset  int64  `json:"offset"`
	Length  int64  `json:"length"`
}

type archiveManifest struct {
	CreatedAt             time.Time             `json:"created_at"`
	Status                Status                `json:"status"`
	RequestedSources      []string              `json:"requested_sources"`
	IncludedSources       []string              `json:"included_sources"`
	Version               string                `json:"version,omitempty"`
	Commit                string                `json:"commit,omitempty"`
	BuildTime             string                `json:"build_time,omitempty"`
	OS                    string                `json:"os"`
	Architecture          string                `json:"architecture"`
	BackendFiles          []backendArchiveFile  `json:"backend_files"`
	FrontendFiles         []frontendArchiveFile `json:"frontend_files"`
	RuntimeEntryCount     int                   `json:"runtime_entry_count,omitempty"`
	ACPSessionCount       int                   `json:"acp_session_count,omitempty"`
	ACPFiles              []acpArchiveFile      `json:"acp_files,omitempty"`
	Warnings              []string              `json:"warnings"`
	BackendSinkStatistics any                   `json:"backend_sink_statistics,omitempty"`
}

type archiveContents struct {
	partial       bool
	warnings      []string
	included      []string
	backendFiles  []backendArchiveFile
	frontendFiles []frontendArchiveFile
	runtimeCount  int
	acpCount      int
	acpFiles      []acpArchiveFile
}

func (s *Service) build(id string) {
	s.mu.Lock()
	item := s.jobs[id]
	if item == nil || item.Status != StatusBuilding {
		s.mu.Unlock()
		return
	}
	if !s.config.Now().UTC().Before(item.BuildDeadline) {
		item.Status = StatusExpired
		delete(s.active, item.Owner)
		_ = os.RemoveAll(item.WorkDir)
		s.mu.Unlock()
		return
	}
	snapshot := snapshotJob(item)
	s.mu.Unlock()

	archivePath := filepath.Join(snapshot.WorkDir, "diagnostic-bundle.zip")
	partial, warnings, err := s.writeArchive(snapshot, archivePath)

	s.mu.Lock()
	defer s.mu.Unlock()
	item = s.jobs[id]
	if item == nil || item.Status != StatusBuilding {
		_ = os.Remove(archivePath)
		return
	}
	if err != nil {
		item.Status = StatusFailed
		addWarning(item, "diagnostic bundle build failed")
		item.ExpiresAt = timePointer(s.config.Now().UTC().Add(s.config.ReadyLifetime))
		delete(s.active, item.Owner)
		_ = os.Remove(archivePath)
		s.config.Log.Error("Failed to build diagnostic bundle", zap.Error(err))
		return
	}
	for _, warning := range warnings {
		addWarning(item, warning)
	}
	item.Partial = item.Partial || partial
	if item.Partial {
		item.Status = StatusPartial
	} else {
		item.Status = StatusReady
	}
	item.ArchivePath = archivePath
	item.ExpiresAt = timePointer(s.config.Now().UTC().Add(s.config.ReadyLifetime))
	delete(s.active, item.Owner)
}

func snapshotJob(item *job) *job {
	copy := *item
	copy.Sources = append([]string(nil), item.Sources...)
	copy.SessionIDs = append([]string(nil), item.SessionIDs...)
	copy.RuntimeSessions = append([]DiagnosticSession(nil), item.RuntimeSessions...)
	copy.ACPSessions = append([]DiagnosticSession(nil), item.ACPSessions...)
	copy.Warnings = append([]string(nil), item.Warnings...)
	copy.Browsers = make(map[string]*browserCapture, len(item.Browsers))
	for id, browser := range item.Browsers {
		browserCopy := *browser
		browserCopy.CaptureMetadata = append(json.RawMessage(nil), browser.CaptureMetadata...)
		copy.Browsers[id] = &browserCopy
	}
	return &copy
}

func (s *Service) writeArchive(item *job, archivePath string) (bool, []string, error) {
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, nil, err
	}
	writer := zip.NewWriter(file)
	contents, err := s.populateArchive(writer, item)
	if err == nil {
		err = addJSONFile(writer, "manifest.json", s.manifest(item, contents))
	}
	if err == nil {
		err = writer.Close()
	} else {
		_ = writer.Close()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = validateArchiveSize(archivePath)
	}
	if err != nil {
		_ = os.Remove(archivePath)
	}
	return contents.partial, contents.warnings, err
}

func (s *Service) populateArchive(writer *zip.Writer, item *job) (archiveContents, error) {
	result := archiveContents{
		partial: item.Partial, warnings: append([]string(nil), item.Warnings...),
		included: make([]string, 0, 2),
	}
	if slices.Contains(item.Sources, "backend") {
		var backendIncluded bool
		var err error
		backendLimit := maxBackendBytes
		if slices.Contains(item.Sources, SourceACP) {
			backendLimit = maxACPBackendBytes
		}
		result.backendFiles, backendIncluded, result.partial, result.warnings, err =
			s.addBackendFiles(writer, result.partial, result.warnings, backendLimit)
		if err != nil {
			return result, err
		}
		if backendIncluded {
			result.included = append(result.included, "backend")
		}
	}
	if slices.Contains(item.Sources, "frontend") {
		var err error
		result.frontendFiles, err = addFrontendFiles(writer, item)
		if err != nil {
			return result, err
		}
		if len(result.frontendFiles) > 0 {
			result.included = append(result.included, "frontend")
		} else {
			result.partial = true
			result.warnings = appendUnique(result.warnings, "no frontend log file was included")
		}
	}
	if slices.Contains(item.Sources, SourceRuntime) {
		var runtimeTruncated bool
		var err error
		result.runtimeCount, runtimeTruncated, err = addRuntimeFile(
			writer, "runtime/sessions.json", item.RuntimeSessions,
		)
		if err != nil {
			return result, err
		}
		if runtimeTruncated {
			result.partial = true
			result.warnings = appendUnique(result.warnings, "runtime session index was truncated to the archive byte limit")
		}
		result.included = append(result.included, SourceRuntime)
	}
	if slices.Contains(item.Sources, SourceACP) {
		var err error
		result.acpFiles, result.partial, result.warnings, err = s.addACPFiles(
			writer, item, result.partial, result.warnings,
		)
		if err != nil {
			return result, err
		}
		result.acpCount = len(item.ACPSessions)
		if len(result.acpFiles) > 0 {
			result.included = append(result.included, SourceACP)
		} else {
			result.partial = true
			result.warnings = appendUnique(result.warnings, "no ACP debug file was included")
		}
	}
	return result, nil
}

func (s *Service) manifest(item *job, contents archiveContents) archiveManifest {
	status := StatusReady
	if contents.partial {
		status = StatusPartial
	}
	return archiveManifest{
		CreatedAt: s.config.Now().UTC(), Status: status,
		RequestedSources: append([]string(nil), item.Sources...), IncludedSources: contents.included,
		Version: s.config.Version, Commit: s.config.Commit, BuildTime: s.config.BuildTime,
		OS: runtime.GOOS, Architecture: runtime.GOARCH,
		BackendFiles: contents.backendFiles, FrontendFiles: contents.frontendFiles,
		RuntimeEntryCount: contents.runtimeCount, ACPSessionCount: contents.acpCount,
		ACPFiles:              contents.acpFiles,
		Warnings:              contents.warnings,
		BackendSinkStatistics: s.config.Log.SinkStatistics(),
	}
}

func validateArchiveSize(archivePath string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	if info.Size() > maxTemporaryDiskBytes {
		return fmt.Errorf("diagnostic bundle exceeds temporary disk limit")
	}
	return nil
}

func (s *Service) addBackendFiles(
	writer *zip.Writer, partial bool, warnings []string, budget int64,
) ([]backendArchiveFile, bool, bool, []string, error) {
	candidates := s.backendCandidates()
	remaining := budget
	manifestFiles := make([]backendArchiveFile, 0, len(candidates))
	included := false
	for _, candidate := range candidates {
		if remaining == 0 {
			partial = true
			warnings = appendUnique(warnings, "backend logs were truncated to the archive byte limit")
			continue
		}
		file, ok, err := addBackendCandidate(writer, candidate, remaining, openBackendCandidate)
		if err != nil {
			if os.IsNotExist(err) {
				partial = true
				warnings = appendUnique(warnings, "backend logs changed during collection")
				continue
			}
			return nil, included, partial, warnings, err
		}
		if !ok {
			continue
		}
		if file.Length < file.Size {
			partial = true
			warnings = appendUnique(warnings, "backend logs were truncated to the archive byte limit")
		}
		manifestFiles = append(manifestFiles, file)
		remaining -= file.Length
		included = true
	}
	if !included {
		partial = true
		warnings = appendUnique(warnings, "no backend log file was included")
	}
	return manifestFiles, included, partial, warnings, nil
}

func addBackendCandidate(
	writer *zip.Writer, candidate backendCandidate, budget int64,
	open func(string) (*os.File, error),
) (backendArchiveFile, bool, error) {
	source, err := open(candidate.Path)
	if err != nil {
		return backendArchiveFile{}, false, err
	}
	info, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return backendArchiveFile{}, false, err
	}
	if !info.Mode().IsRegular() {
		return backendArchiveFile{}, false, source.Close()
	}
	length := min(info.Size(), budget)
	offset := info.Size() - length
	header := &zip.FileHeader{Name: "backend/" + candidate.Name, Method: zip.Store}
	header.SetMode(0o600)
	destination, err := writer.CreateHeader(header)
	if err == nil {
		err = copySection(destination, source, offset, length)
	}
	closeErr := source.Close()
	if err != nil {
		return backendArchiveFile{}, false, err
	}
	if closeErr != nil {
		return backendArchiveFile{}, false, closeErr
	}
	return backendArchiveFile{
		Name: candidate.Name, Size: info.Size(), Offset: offset, Length: length,
	}, true, nil
}

func addFrontendFiles(writer *zip.Writer, item *job) ([]frontendArchiveFile, error) {
	browsers := make([]*browserCapture, 0, len(item.Browsers))
	for _, browser := range item.Browsers {
		browsers = append(browsers, browser)
	}
	slices.SortFunc(browsers, func(a, b *browserCapture) int { return a.Index - b.Index })
	manifestFiles := make([]frontendArchiveFile, 0, len(browsers))
	for _, browser := range browsers {
		info, err := os.Lstat(browser.Path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("frontend capture is not a regular file")
		}
		source, err := os.Open(browser.Path)
		if err != nil {
			return nil, err
		}
		name := "frontend/browser-" + twoDigit(browser.Index) + ".jsonl"
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0o600)
		destination, err := writer.CreateHeader(header)
		if err == nil {
			err = copySection(destination, source, 0, info.Size())
		}
		closeErr := source.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if removeErr := os.Remove(browser.Path); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, removeErr
		}
		manifestFiles = append(manifestFiles, frontendArchiveFile{
			Name: name, Bytes: info.Size(), Entries: browser.EntryCount,
			StorageMode: browser.StorageMode, Metadata: browser.CaptureMetadata,
		})
	}
	return manifestFiles, nil
}

func (s *Service) addACPFiles(
	writer *zip.Writer, item *job, partial bool, warnings []string,
) ([]acpArchiveFile, bool, []string, error) {
	remaining := maxACPBytes
	manifestFiles := make([]acpArchiveFile, 0)
	collectionCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for index, session := range item.ACPSessions {
		if err := collectionCtx.Err(); err != nil {
			partial = true
			warnings = appendUnique(warnings, "ACP collection deadline reached")
			break
		}
		files, consumed, sessionPartial, sessionWarnings, err := s.addACPFilesForSession(
			collectionCtx, writer, item, index+1, session, remaining,
		)
		if err != nil {
			return nil, partial, warnings, err
		}
		manifestFiles = append(manifestFiles, files...)
		remaining -= consumed
		partial = partial || sessionPartial
		for _, warning := range sessionWarnings {
			warnings = appendUnique(warnings, warning)
		}
	}
	return manifestFiles, partial, warnings, nil
}

func (s *Service) addACPFilesForSession(
	ctx context.Context,
	writer *zip.Writer,
	item *job,
	index int,
	session DiagnosticSession,
	remaining int64,
) ([]acpArchiveFile, int64, bool, []string, error) {
	var exportErr error
	if s.config.ACPExporter != nil && remaining > 0 {
		exported, consumed, err := s.addACPExport(ctx, writer, item, index, session, remaining)
		if err == nil && len(exported) > 0 {
			return exported, consumed, false, nil, nil
		}
		exportErr = err
	}
	files, err := s.hostACPFiles(session.ACPSessionID)
	if err != nil {
		return nil, 0, false, nil, err
	}
	if len(files) == 0 {
		warnings := make([]string, 0, 2)
		if exportErr != nil {
			var invalidErr *invalidACPExportError
			if errors.As(exportErr, &invalidErr) {
				warnings = append(warnings, "ACP session "+session.SessionID+" executor export was invalid")
			} else {
				warnings = append(warnings, "ACP session "+session.SessionID+" executor export was unavailable")
			}
		}
		warnings = append(warnings, "ACP session "+session.SessionID+" is unavailable")
		return nil, 0, true, warnings, nil
	}
	return s.addHostACPFiles(writer, index, session, files, remaining)
}

func (s *Service) addHostACPFiles(
	writer *zip.Writer,
	index int,
	session DiagnosticSession,
	files []hostACPFile,
	remaining int64,
) ([]acpArchiveFile, int64, bool, []string, error) {
	manifestFiles := make([]acpArchiveFile, 0, len(files))
	warnings := make([]string, 0, 1)
	partial := false
	var consumed int64
	for _, sourceFile := range files {
		if remaining <= 0 {
			partial = true
			warnings = append(warnings, "ACP logs were truncated to the archive byte limit")
			break
		}
		length := min(sourceFile.size, remaining)
		offset := sourceFile.size - length
		if length < sourceFile.size {
			partial = true
			warnings = append(warnings, "ACP logs were truncated to the archive byte limit")
		}
		archiveName := "acp/session-" + twoDigit(index) + "/" + sourceFile.kind + "/" + sourceFile.name
		if err := copyACPHostFile(writer, sourceFile, archiveName, offset, length); err != nil {
			return nil, 0, partial, warnings, err
		}
		manifestFiles = append(manifestFiles, acpArchiveFile{
			Name: archiveName, Session: session.SessionID, Kind: sourceFile.kind,
			Size: sourceFile.size, Offset: offset, Length: length,
		})
		remaining -= length
		consumed += length
	}
	return manifestFiles, consumed, partial, warnings, nil
}

func copyACPHostFile(
	writer *zip.Writer,
	sourceFile hostACPFile,
	archiveName string,
	offset, length int64,
) error {
	source, err := os.Open(sourceFile.path)
	if err != nil {
		return err
	}
	header := &zip.FileHeader{Name: archiveName, Method: zip.Store}
	header.SetMode(0o600)
	destination, createErr := writer.CreateHeader(header)
	if createErr == nil {
		createErr = copySection(destination, source, offset, length)
	}
	closeErr := source.Close()
	if createErr != nil {
		return createErr
	}
	return closeErr
}

func (s *Service) addACPExport(
	ctx context.Context,
	writer *zip.Writer,
	item *job,
	index int,
	session DiagnosticSession,
	remaining int64,
) ([]acpArchiveFile, int64, error) {
	response, err := s.config.ACPExporter.ExportACP(ctx, session, remaining)
	if err != nil {
		return nil, 0, err
	}
	if response == nil {
		return nil, 0, invalidACPExport("ACP exporter returned an empty response")
	}
	defer func() { _ = response.Close() }()
	tempPath, err := persistACPExport(response, item.WorkDir, remaining)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = os.Remove(tempPath) }()
	entries, consumed, tempFile, err := loadValidatedACPEntries(tempPath, session.ACPSessionID, index, remaining)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tempFile.Close() }()
	if err := validateACPEntryContents(entries); err != nil {
		return nil, 0, err
	}
	files, err := writeACPEntries(writer, entries, session.SessionID)
	if err != nil {
		return nil, 0, err
	}
	return files, consumed, nil
}

func persistACPExport(response io.Reader, workDir string, remaining int64) (string, error) {
	temp, err := os.CreateTemp(workDir, "acp-export-*.zip")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	count, copyErr := io.Copy(temp, io.LimitReader(response, remaining+1))
	closeErr := temp.Close()
	if copyErr != nil || closeErr != nil || count > remaining {
		_ = os.Remove(tempPath)
		return "", invalidACPExport("ACP export exceeded its byte bound")
	}
	return tempPath, nil
}

func loadValidatedACPEntries(
	tempPath, sessionID string,
	index int,
	remaining int64,
) ([]validatedACPEntry, int64, *os.File, error) {
	info, err := os.Stat(tempPath)
	if err != nil {
		return nil, 0, nil, err
	}
	tempFile, err := os.Open(tempPath)
	if err != nil {
		return nil, 0, nil, err
	}
	reader, err := zip.NewReader(tempFile, info.Size())
	if err != nil {
		_ = tempFile.Close()
		return nil, 0, nil, invalidACPExport("ACP export is not a valid ZIP")
	}
	if len(reader.File) > maxACPExportEntries {
		_ = tempFile.Close()
		return nil, 0, nil, invalidACPExport("ACP export contains too many files")
	}
	entries := make([]validatedACPEntry, 0, len(reader.File))
	seenNames := make(map[string]struct{}, len(reader.File))
	var consumed int64
	for _, entry := range reader.File {
		kind, name, ok := validatedACPArchiveName(entry.Name, sessionID)
		mode := entry.FileInfo().Mode()
		if !ok || !mode.IsRegular() || mode&os.ModeSymlink != 0 ||
			entry.UncompressedSize64 > uint64(remaining-consumed) {
			_ = tempFile.Close()
			return nil, 0, nil, invalidACPExport("invalid ACP export entry")
		}
		archiveName := "acp/session-" + twoDigit(index) + "/" + kind + "/" + name
		if _, exists := seenNames[archiveName]; exists {
			_ = tempFile.Close()
			return nil, 0, nil, invalidACPExport("ACP export contains duplicate files")
		}
		seenNames[archiveName] = struct{}{}
		entries = append(entries, validatedACPEntry{entry: entry, archiveName: archiveName})
		consumed += int64(entry.UncompressedSize64)
	}
	if len(entries) == 0 {
		_ = tempFile.Close()
		return nil, 0, nil, invalidACPExport("ACP export contains no recognized files")
	}
	return entries, consumed, tempFile, nil
}

func validateACPEntryContents(entries []validatedACPEntry) error {
	// Read every entry before touching the final archive. This validates ZIP
	// CRCs and prevents a malformed later entry from leaving a valid prefix in
	// the archive. Keeping only the bounded response ZIP on disk also preserves
	// the job's temporary-storage ceiling.
	for _, entry := range entries {
		source, err := entry.entry.Open()
		if err != nil {
			return invalidACPExport("ACP export entry could not be read")
		}
		count, readErr := io.Copy(io.Discard, source)
		closeErr := source.Close()
		if readErr != nil || closeErr != nil || count != int64(entry.entry.UncompressedSize64) {
			return invalidACPExport("ACP export entry failed validation")
		}
	}
	return nil
}

func writeACPEntries(
	writer *zip.Writer,
	entries []validatedACPEntry,
	sessionID string,
) ([]acpArchiveFile, error) {
	files := make([]acpArchiveFile, 0, len(entries))
	for _, entry := range entries {
		source, err := entry.entry.Open()
		if err != nil {
			return nil, err
		}
		header := &zip.FileHeader{Name: entry.archiveName, Method: zip.Store}
		header.SetMode(0o600)
		destination, createErr := writer.CreateHeader(header)
		if createErr == nil {
			_, createErr = io.CopyN(destination, source, int64(entry.entry.UncompressedSize64))
		}
		closeErr := source.Close()
		if createErr != nil {
			return nil, createErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files = append(files, acpArchiveFile{
			Name: entry.archiveName, Session: sessionID,
			Kind: filepath.Base(filepath.Dir(entry.archiveName)),
			Size: int64(entry.entry.UncompressedSize64), Length: int64(entry.entry.UncompressedSize64),
		})
	}
	return files, nil
}

const maxACPExportEntries = 128

type invalidACPExportError struct{ message string }

func (e *invalidACPExportError) Error() string { return e.message }

func invalidACPExport(format string, args ...any) error {
	return &invalidACPExportError{message: fmt.Sprintf(format, args...)}
}

type validatedACPEntry struct {
	entry       *zip.File
	archiveName string
}

func validatedACPArchiveName(name, sessionID string) (string, string, bool) {
	clean := filepath.ToSlash(filepath.Clean(name))
	clean = strings.TrimPrefix(clean, "./")
	parts := strings.Split(clean, "/")
	if len(parts) < 2 || parts[0] == ".." || slices.Contains(parts, "..") {
		return "", "", false
	}
	kind := parts[len(parts)-2]
	if kind != acpKindRaw && kind != acpKindNormalized {
		return "", "", false
	}
	base := parts[len(parts)-1]
	if !exactACPFilename(base, sanitizeACPPart(sessionID)) {
		return "", "", false
	}
	return kind, base, true
}

func addJSONFile(writer *zip.Writer, name string, value any) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(0o600)
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func copySection(destination io.Writer, source *os.File, offset, length int64) error {
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	buffer := make([]byte, copyChunkBytes)
	remaining := length
	for remaining > 0 {
		size := min(int64(len(buffer)), remaining)
		count, err := io.ReadFull(source, buffer[:size])
		if count > 0 {
			if _, writeErr := destination.Write(buffer[:count]); writeErr != nil {
				return writeErr
			}
			remaining -= int64(count)
			runtime.Gosched()
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

func timePointer(value time.Time) *time.Time { return &value }
