package logbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"go.uber.org/zap"
)

const (
	maxBackendBytes = int64(160 * 1024 * 1024)
	copyChunkBytes  = int64(1024 * 1024)
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
	Warnings              []string              `json:"warnings"`
	BackendSinkStatistics any                   `json:"backend_sink_statistics,omitempty"`
}

type archiveContents struct {
	partial       bool
	warnings      []string
	included      []string
	backendFiles  []backendArchiveFile
	frontendFiles []frontendArchiveFile
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
		result.backendFiles, backendIncluded, result.partial, result.warnings, err =
			s.addBackendFiles(writer, result.partial, result.warnings)
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
	writer *zip.Writer, partial bool, warnings []string,
) ([]backendArchiveFile, bool, bool, []string, error) {
	candidates := s.backendCandidates()
	remaining := maxBackendBytes
	manifestFiles := make([]backendArchiveFile, 0, len(candidates))
	included := false
	for _, candidate := range candidates {
		info, err := os.Lstat(candidate.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, included, partial, warnings, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if remaining == 0 {
			partial = true
			warnings = appendUnique(warnings, "backend logs were truncated to the archive byte limit")
			continue
		}
		length := min(info.Size(), remaining)
		offset := info.Size() - length
		if length < info.Size() {
			partial = true
			warnings = appendUnique(warnings, "backend logs were truncated to the archive byte limit")
		}
		source, err := os.Open(candidate.Path)
		if err != nil {
			return nil, included, partial, warnings, err
		}
		header := &zip.FileHeader{Name: "backend/" + candidate.Name, Method: zip.Store}
		header.SetMode(0o600)
		destination, err := writer.CreateHeader(header)
		if err == nil {
			err = copySection(destination, source, offset, length)
		}
		closeErr := source.Close()
		if err != nil {
			return nil, included, partial, warnings, err
		}
		if closeErr != nil {
			return nil, included, partial, warnings, closeErr
		}
		manifestFiles = append(manifestFiles, backendArchiveFile{
			Name: candidate.Name, Size: info.Size(), Offset: offset, Length: length,
		})
		remaining -= length
		included = true
	}
	if !included {
		partial = true
		warnings = appendUnique(warnings, "no backend log file was included")
	}
	return manifestFiles, included, partial, warnings, nil
}

type backendCandidate struct {
	Name string
	Path string
}

func (s *Service) backendCandidates() []backendCandidate {
	logDir := filepath.Join(s.config.HomeDir, "logs")
	now := s.config.Now().UTC()
	names := []string{"backend-logs.log"}
	for daysAgo := 1; daysAgo <= 2; daysAgo++ {
		date := now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
		names = append(names, "backend-logs-"+date+".log")
	}
	out := make([]backendCandidate, 0, len(names))
	for _, name := range names {
		out = append(out, backendCandidate{Name: name, Path: filepath.Join(logDir, name)})
	}
	return out
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
		manifestFiles = append(manifestFiles, frontendArchiveFile{
			Name: name, Bytes: info.Size(), Entries: browser.EntryCount,
			StorageMode: browser.StorageMode, Metadata: browser.CaptureMetadata,
		})
	}
	return manifestFiles, nil
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

// ManifestSummary returns the server-generated manifest without exposing
// archive bytes through JSON. It is used by task-mode MCP after materializing
// the ZIP into the execution workspace.
func (s *Service) ManifestSummary(owner, id string) (map[string]any, error) {
	file, _, err := s.OpenArchive(owner, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, err
	}
	for _, entry := range reader.File {
		if entry.Name != "manifest.json" || entry.UncompressedSize64 > 256*1024 {
			continue
		}
		source, err := entry.Open()
		if err != nil {
			return nil, err
		}
		var buffer bytes.Buffer
		_, copyErr := io.CopyN(&buffer, source, int64(entry.UncompressedSize64))
		closeErr := source.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		var summary map[string]any
		if err := json.Unmarshal(buffer.Bytes(), &summary); err != nil {
			return nil, err
		}
		return summary, nil
	}
	return nil, fmt.Errorf("diagnostic bundle manifest is missing")
}
