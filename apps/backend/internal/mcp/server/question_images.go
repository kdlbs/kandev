package mcp

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/clarification/protocol"
)

func buildOperatorQuestionSchemaItem() map[string]any {
	schema := buildQuestionSchemaItem()
	props := schema[propsKey].(map[string]any)
	props["images"] = buildQuestionImageSchema(4)
	option := props[optionsArg].(map[string]any)["items"].(map[string]any)
	option[propsKey].(map[string]any)["images"] = buildQuestionImageSchema(1)
	return schema
}

func buildQuestionImageSchema(maxItems int) map[string]any {
	return map[string]any{
		typeKey: "array", "maxItems": maxItems,
		descriptionArg: "Optional PNG/JPEG previews. Each choice accepts at most 1 image. A question accepts at most 4 references across its own images and choices. A bundle accepts at most 4 distinct image IDs. Reuse an ID only with the same path, repo, and alt. Files are read from this session's workspace. URLs and absolute paths are not supported. Inspecting a choice image does not select the choice.",
		"items": map[string]any{typeKey: objType, "additionalProperties": false, reqKey: []string{"id", "path", "alt"}, propsKey: map[string]any{
			"id":   map[string]any{typeKey: stringType, "pattern": "^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"},
			"path": map[string]any{typeKey: stringType, "minLength": 1, descriptionArg: "Canonical relative PNG/JPEG path, at most 4096 UTF-8 bytes."},
			"repo": map[string]any{typeKey: stringType, descriptionArg: "Optional attached repository path relative to the workspace, at most 4096 UTF-8 bytes."},
			"alt":  map[string]any{typeKey: stringType, "minLength": 1, "maxLength": 300, descriptionArg: "Description that identifies this image without seeing it."},
		}},
	}
}

func validateQuestionImagePayloads(questions []map[string]interface{}) error {
	data, err := json.Marshal(questions)
	if err != nil {
		return err
	}
	var typed []protocol.Question
	if err = json.Unmarshal(data, &typed); err != nil {
		return err
	}
	return protocol.ValidateQuestionImages(typed)
}
