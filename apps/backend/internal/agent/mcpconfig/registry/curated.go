package registry

import (
	_ "embed"
	"encoding/json"
)

//go:embed curated.json
var curatedManifest []byte

type curatedDocument struct {
	Version string  `json:"version"`
	Entries []Entry `json:"entries"`
}

// CuratedEntries returns the versioned, local starter catalog.
func CuratedEntries() []Entry {
	var document curatedDocument
	if err := json.Unmarshal(curatedManifest, &document); err != nil {
		return nil
	}
	entries := make([]Entry, len(document.Entries))
	copy(entries, document.Entries)
	return entries
}
