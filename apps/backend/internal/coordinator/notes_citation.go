package coordinator

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// fieldCitations names the citation list in error bodies.
const (
	fieldCitations     = "citations"
	fieldVersion       = "version"
	fieldTargetVersion = "target_version"
	fieldType          = "type"
)

// Citation kinds.
const (
	CitationTurn = "turn"
	CitationTask = "task"
	CitationPR   = "pr"
	CitationFile = "file"
)

// NoteCitation is a typed pointer a note relies on. Fields are declared in
// alphabetical order so the marshaled form has sorted keys.
type NoteCitation struct {
	ID           string `json:"id,omitempty"`
	Kind         string `json:"kind"`
	Number       int    `json:"number,omitempty"`
	Path         string `json:"path,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	SHA          string `json:"sha,omitempty"`
}

var (
	noteIDPattern  = regexp.MustCompile(`^[0-9a-f-]{8,64}$`)
	noteSHAPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

// canonicalKey is the identity used for uniqueness and equality.
func (c NoteCitation) canonicalKey() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// normalizeCitation lower-cases identifiers and canonicalizes the path, then
// validates the shape for its kind. A nil error means the citation is usable.
func normalizeCitation(c NoteCitation) (NoteCitation, bool) {
	c.ID = strings.ToLower(strings.TrimSpace(c.ID))
	c.SHA = strings.ToLower(strings.TrimSpace(c.SHA))
	c.RepositoryID = strings.TrimSpace(c.RepositoryID)
	switch c.Kind {
	case CitationTurn, CitationTask:
		return c, noteIDPattern.MatchString(c.ID) && c.Number == 0 && c.Path == "" && c.RepositoryID == "" && c.SHA == ""
	case CitationPR:
		return c, c.RepositoryID != "" && c.Number > 0 && c.ID == "" && c.Path == "" && c.SHA == ""
	case CitationFile:
		return normalizeFileCitation(c)
	}
	return c, false
}

func normalizeFileCitation(c NoteCitation) (NoteCitation, bool) {
	p, ok := canonicalNotePath(c.Path)
	c.Path = p
	return c, ok && c.RepositoryID != "" && noteSHAPattern.MatchString(c.SHA) && c.ID == "" && c.Number == 0
}

// canonicalNotePath converts backslashes to slashes and removes "." segments;
// an absolute path, an empty result or any remaining ".." is invalid.
func canonicalNotePath(p string) (string, bool) {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if p == "" || strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return p, false
	}
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
		case "..":
			return p, false
		default:
			parts = append(parts, seg)
		}
	}
	if len(parts) == 0 {
		return p, false
	}
	return path.Join(parts...), true
}

// citationFields lists every textual field of a citation, for the credential
// gate.
func (c NoteCitation) citationFields() []string {
	return []string{c.ID, c.Kind, c.Path, c.RepositoryID, c.SHA}
}

// citationSetKey is the order-independent identity of a citation list.
func citationSetKey(cs []NoteCitation) string {
	keys := make([]string, len(cs))
	for i, c := range cs {
		keys[i] = c.canonicalKey()
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

// marshalCitations is the canonical stored form.
func marshalCitations(cs []NoteCitation) string {
	if len(cs) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(cs)
	return string(b)
}

func unmarshalCitations(raw string) []NoteCitation {
	cs := []NoteCitation{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cs)
	}
	return cs
}

// parseCitations decodes the request's citation list. A null or absent value
// is the caller's concern; here raw is the list. A wrong JSON type is
// invalid_field, an unknown key invalid_request.
func parseCitations(raw json.RawMessage) ([]NoteCitation, *NoteError) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, noteErr(NoteInvalidField, fieldCitations)
	}
	out := make([]NoteCitation, 0, len(items))
	for i, item := range items {
		c, nerr := parseCitation(item, i)
		if nerr != nil {
			return nil, nerr
		}
		out = append(out, c)
	}
	return out, nil
}

func parseCitation(raw json.RawMessage, i int) (NoteCitation, *NoteError) {
	var c NoteCitation
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		idx := i
		code := NoteInvalidField
		if strings.Contains(err.Error(), "unknown field") {
			code = NoteInvalidRequest
		}
		return c, &NoteError{Code: code, Field: fieldCitations, Index: &idx}
	}
	return c, nil
}

// validateCitations normalizes every citation, rejecting bad shape or
// duplicates (invalid_citation with the lowest offending index).
func validateCitations(in []NoteCitation) ([]NoteCitation, *NoteError) {
	if len(in) > maxNoteCitations {
		return nil, noteErr(NoteInvalidCitation, fieldCitations)
	}
	out := make([]NoteCitation, len(in))
	seen := map[string]bool{}
	for i, c := range in {
		n, ok := normalizeCitation(c)
		key := n.canonicalKey()
		if !ok || seen[key] {
			idx := i
			return nil, &NoteError{Code: NoteInvalidCitation, Field: fieldCitations, Index: &idx}
		}
		seen[key] = true
		out[i] = n
	}
	return out, nil
}

// normalizeNoteText trims and counts code points.
func normalizeNoteText(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	return s, n >= 1 && n <= maxNoteTextRunes
}

func validNoteType(t string) bool {
	switch t {
	case NoteTypeUser, NoteTypeFeedback, NoteTypeProject, NoteTypeReference:
		return true
	}
	return false
}
