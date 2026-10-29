package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"
)

const stateKeyCursorPrefix = "state-keys-v1."

type stateKeyCursor struct {
	Query string  `json:"query"`
	After *string `json:"after"`
}

type pluginStateKeyReader struct{ host *pluginHost }

func (h *pluginHost) StateKeys() pluginsdk.StateKeyReader { return pluginStateKeyReader{host: h} }

func (r pluginStateKeyReader) ListKeys(ctx context.Context, query pluginsdk.StateKeyQuery) (pluginsdk.StateKeyPage, error) {
	h := r.host
	if !h.capabilities.State {
		return pluginsdk.StateKeyPage{}, permissionDenied("state")
	}
	if query.Limit < 0 || query.Limit > pluginsdk.MaxStateKeyPageSize {
		return pluginsdk.StateKeyPage{}, invalidArgument("state key page limit must be between 1 and 200, or zero for the default")
	}
	if query.Limit == 0 {
		query.Limit = pluginsdk.MaxStateKeyPageSize
	}
	identity := stateKeyQueryIdentity(h.pluginID, query)
	after, err := decodeStateKeyCursor(query.Cursor, identity)
	if err != nil {
		return pluginsdk.StateKeyPage{}, err
	}
	keys, more, err := h.state.ListKeys(ctx, h.pluginID, query.Scope, query.ScopeID, after, query.Limit)
	if err != nil {
		return pluginsdk.StateKeyPage{}, err
	}
	page := pluginsdk.StateKeyPage{Keys: keys}
	if more {
		last := keys[len(keys)-1]
		encoded, err := json.Marshal(stateKeyCursor{Query: identity, After: &last})
		if err != nil {
			return pluginsdk.StateKeyPage{}, err
		}
		page.NextCursor = stateKeyCursorPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	}
	return page, nil
}

func stateKeyQueryIdentity(pluginID string, query pluginsdk.StateKeyQuery) string {
	encoded, _ := json.Marshal([]string{pluginID, query.Scope, query.ScopeID, strconv.Itoa(query.Limit)})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func decodeStateKeyCursor(cursor, identity string) (*string, error) {
	if cursor == "" {
		return nil, nil
	}
	payload, ok := strings.CutPrefix(cursor, stateKeyCursorPrefix)
	if !ok {
		return nil, invalidArgument("invalid state key cursor")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, invalidArgument("invalid state key cursor")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var value stateKeyCursor
	var extra any
	if decoder.Decode(&value) != nil || decoder.Decode(&extra) != io.EOF || value.After == nil || value.Query != identity {
		return nil, invalidArgument("state key cursor does not match the query")
	}
	return value.After, nil
}

var _ pluginsdk.StateKeysHost = (*pluginHost)(nil)
