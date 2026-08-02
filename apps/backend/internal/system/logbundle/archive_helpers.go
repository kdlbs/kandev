package logbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func addRuntimeFile(
	writer *zip.Writer, name string, sessions []DiagnosticSession,
) (int, bool, error) {
	var payload bytes.Buffer
	payload.WriteByte('[')
	included := 0
	truncated := false
	for _, session := range sessions {
		encoded, err := json.Marshal(session)
		if err != nil {
			return 0, false, err
		}
		separator := 0
		if included > 0 {
			separator = 1
		}
		if int64(payload.Len()+separator+len(encoded)+2) > maxRuntimeBytes {
			truncated = true
			break
		}
		if separator != 0 {
			payload.WriteByte(',')
		}
		payload.Write(encoded)
		included++
	}
	payload.WriteString("]\n")
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(0o600)
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return 0, false, err
	}
	if _, err := destination.Write(payload.Bytes()); err != nil {
		return 0, false, err
	}
	return included, truncated, nil
}

type hostACPFile struct {
	name string
	path string
	kind string
	size int64
}

func (s *Service) hostACPFiles(sessionID string) ([]hostACPFile, error) {
	if sessionID == "" {
		return nil, nil
	}
	dir := filepath.Join(s.config.HomeDir, "logs", "acp")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	safeSession := sanitizeACPPart(sessionID)
	files := make([]hostACPFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !exactACPFilename(name, safeSession) {
			continue
		}
		path := filepath.Join(dir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		kind := acpKindNormalized
		if strings.HasPrefix(name, "raw-") {
			kind = acpKindRaw
		}
		files = append(files, hostACPFile{name: name, path: path, kind: kind, size: info.Size()})
	}
	slices.SortFunc(files, func(a, b hostACPFile) int {
		if a.kind != b.kind {
			if a.kind == acpKindRaw {
				return -1
			}
			return 1
		}
		return strings.Compare(a.name, b.name)
	})
	return files, nil
}

func exactACPFilename(name, safeSession string) bool {
	if !strings.HasSuffix(name, ".jsonl") ||
		(!strings.HasPrefix(name, "raw-") && !strings.HasPrefix(name, "normalized-")) {
		return false
	}
	base := strings.TrimSuffix(name, ".jsonl")
	if dot := strings.LastIndexByte(base, '.'); dot > 0 && asciiDigits(base[dot+1:]) {
		base = base[:dot]
	}
	return strings.HasSuffix(base, "-"+safeSession)
}

func sanitizeACPPart(value string) string {
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

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
