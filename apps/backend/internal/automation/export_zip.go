package automation

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
)

// automationZipEntry is one automation prepared for the zip export: enough
// identity (ID, Name) to derive a unique slug (AC-25/26/27) plus the
// exportAutomation payload that becomes that entry's single-element
// automations list.
type automationZipEntry struct {
	ID         string
	Name       string
	Automation exportAutomation
}

// buildAutomationZip renders the zip export: one file per automation at
// .kandev/automations/<slug>.yml, each the full envelope (version, type,
// automations) with a single-element automations list and no warnings key
// (AC-24, AC-27). Entries — including .kandev/automations/WARNINGS.txt when
// warnings is non-empty (AC-20) — are ordered by entry path ascending, with
// no modification time set so the archive is byte-identical across builds of
// the same content (AC-28).
func buildAutomationZip(entries []automationZipEntry, warnings []string) ([]byte, error) {
	sources := make([]automationSlugSource, len(entries))
	for i, e := range entries {
		sources[i] = automationSlugSource{ID: e.ID, Name: e.Name}
	}
	slugs := uniqueAutomationSlugs(sources)

	type zipFile struct {
		path    string
		content []byte
	}
	files := make([]zipFile, 0, len(entries)+1)
	for _, e := range entries {
		doc := newExportDocument([]exportAutomation{e.Automation}, nil)
		content, err := marshalExportDocument(doc)
		if err != nil {
			return nil, fmt.Errorf("marshal automation %q for zip: %w", e.ID, err)
		}
		files = append(files, zipFile{
			path:    fmt.Sprintf(".kandev/automations/%s.yml", slugs[e.ID]),
			content: content,
		})
	}
	if len(warnings) > 0 {
		var buf bytes.Buffer
		for _, w := range warnings {
			buf.WriteString(w)
			buf.WriteByte('\n')
		}
		files = append(files, zipFile{path: ".kandev/automations/WARNINGS.txt", content: buf.Bytes()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range files {
		// zip.Writer.Create leaves FileHeader.Modified unset, unlike
		// CreateHeader with an explicit Modified value — this is what keeps
		// the archive free of any wall-clock timestamp.
		w, err := zw.Create(f.path)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %q: %w", f.path, err)
		}
		if _, err := w.Write(f.content); err != nil {
			return nil, fmt.Errorf("write zip entry %q: %w", f.path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}
	return out.Bytes(), nil
}
