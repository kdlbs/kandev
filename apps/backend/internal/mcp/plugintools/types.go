package plugintools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

const (
	SurfaceKanban = "kanban-task"
	SurfaceOffice = "office-task"
)

const maxExposedNameLength = 64

// Definition is the backend-owned descriptor transported to agentctl. Schemas
// are raw JSON so agentctl can register them without reinterpreting YAML.
type Definition struct {
	PluginID          string          `json:"plugin_id"`
	PluginDisplayName string          `json:"plugin_display_name,omitempty"`
	LocalName         string          `json:"local_name"`
	ExposedName       string          `json:"exposed_name"`
	Description       string          `json:"description"`
	Surfaces          []string        `json:"surfaces"`
	InputSchema       json.RawMessage `json:"input_schema"`
	OutputSchema      json.RawMessage `json:"output_schema,omitempty"`
	ReadOnlyHint      bool            `json:"read_only_hint"`
	DestructiveHint   bool            `json:"destructive_hint"`
	IdempotentHint    bool            `json:"idempotent_hint"`
	OpenWorldHint     bool            `json:"open_world_hint"`
}

type Snapshot struct {
	Generation string       `json:"generation"`
	Revision   uint64       `json:"revision"`
	Tools      []Definition `json:"tools"`
}

func ExposedName(pluginID, localName string) string {
	slug := pluginIDSlug(pluginID)
	readable := "kandev_" + slug + "_" + localName
	if len(readable) <= maxExposedNameLength {
		return readable
	}

	// Keep long names mostly readable, using a short stable suffix only when
	// the provider-safe length budget requires disambiguation.
	digest := sha256.Sum256([]byte(pluginID))
	suffix := "_" + hex.EncodeToString(digest[:4])
	available := maxExposedNameLength - len("kandev__") - len(localName) - len(suffix)
	if available < 1 {
		available = 1
	}
	if len(slug) > available {
		slug = slug[:available]
	}
	return "kandev_" + slug + "_" + localName + suffix
}

func pluginIDSlug(pluginID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(pluginID) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune('_')
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func Normalize(snapshot Snapshot) Snapshot {
	snapshot.Tools = append([]Definition(nil), snapshot.Tools...)
	for i := range snapshot.Tools {
		snapshot.Tools[i].Surfaces = append([]string(nil), snapshot.Tools[i].Surfaces...)
		sort.Strings(snapshot.Tools[i].Surfaces)
	}
	sort.Slice(snapshot.Tools, func(i, j int) bool {
		return snapshot.Tools[i].ExposedName < snapshot.Tools[j].ExposedName
	})
	return snapshot
}

func Equal(left, right Snapshot) bool {
	l, r := Normalize(left), Normalize(right)
	bl, _ := json.Marshal(l)
	br, _ := json.Marshal(r)
	return string(bl) == string(br)
}
