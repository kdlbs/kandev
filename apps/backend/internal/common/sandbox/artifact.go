package sandbox

import (
	"encoding/json"
	"io"
	"strings"
)

const maxArtifactBytes = 1 << 20

type ArtifactBinding struct {
	Path      []string `json:"path"`
	Reference string   `json:"reference"`
}

func RenderJSONArtifact(content string, bindings []ArtifactBinding, values map[string]string) ([]byte, error) {
	if len(content) > maxArtifactBytes || len(bindings) > 128 {
		return nil, ErrInvalidPolicy
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return nil, ErrInvalidPolicy
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrInvalidPolicy
	}
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		key, _ := json.Marshal(binding.Path)
		value, exists := values[binding.Reference]
		if !exists || seen[string(key)] || len(value) > maxArtifactBytes {
			return nil, ErrInvalidPolicy
		}
		seen[string(key)] = true
		if err := insertArtifactValue(root, binding.Path, value); err != nil {
			return nil, err
		}
	}
	rendered, err := json.Marshal(root)
	if err != nil || len(rendered) > maxArtifactBytes {
		return nil, ErrInvalidPolicy
	}
	return rendered, nil
}

func insertArtifactValue(root map[string]any, path []string, value string) error {
	if len(path) == 0 || len(path) > 16 {
		return ErrInvalidPolicy
	}
	current := root
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			return ErrInvalidPolicy
		}
		current = next
	}
	key := path[len(path)-1]
	placeholder, ok := current[key].(string)
	if !ok || placeholder != "" {
		return ErrInvalidPolicy
	}
	current[key] = value
	return nil
}
