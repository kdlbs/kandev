package passthroughconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type File struct {
	Path     string `json:"path"`
	Content  []byte `json:"content"`
	MergeKey string `json:"merge_key,omitempty"`
}

func Fingerprint(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func jsonObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, errors.New("MCP configuration must contain a JSON object")
	}
	return object, nil
}

func MergeJSONUnderKey(existing, ours []byte, key string) ([]byte, error) {
	existingDoc, err := jsonObject(existing)
	if err != nil {
		return nil, err
	}
	oursDoc, err := jsonObject(ours)
	if err != nil {
		return nil, err
	}
	entries := map[string]json.RawMessage{}
	if raw, ok := existingDoc[key]; ok {
		entries, err = jsonObject(raw)
	}
	if err != nil {
		return nil, err
	}
	ourEntries := map[string]json.RawMessage{}
	if raw, ok := oursDoc[key]; ok {
		ourEntries, err = jsonObject(raw)
	}
	if err != nil {
		return nil, err
	}
	for name, entry := range ourEntries {
		entries[name] = entry
	}
	merged, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	existingDoc[key] = merged
	data, err := json.MarshalIndent(existingDoc, "", "  ")
	return append(data, '\n'), err
}

func CreateNoFollow(path string, content []byte) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create passthrough MCP config: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return false, fmt.Errorf("write passthrough MCP config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return false, err
	}
	return true, nil
}
