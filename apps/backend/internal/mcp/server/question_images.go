package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

func buildOperatorQuestionSchemaItem() map[string]any {
	schema := buildQuestionSchemaItem()
	props := schema[propsKey].(map[string]any)
	props["images"] = buildQuestionImageSchema(protocol.MaxQuestionImageReferences)
	option := props[optionsArg].(map[string]any)["items"].(map[string]any)
	option[propsKey].(map[string]any)["images"] = buildQuestionImageSchema(protocol.MaxChoiceImages)
	return schema
}

func buildQuestionImageSchema(maxItems int) map[string]any {
	return map[string]any{
		typeKey: "array", "maxItems": maxItems,
		descriptionArg: questionImageSchemaDescription(),
		"items": map[string]any{typeKey: objType, "additionalProperties": false, reqKey: []string{"id", "path", "alt"}, propsKey: map[string]any{
			"id":   map[string]any{typeKey: stringType, "pattern": "^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"},
			"path": map[string]any{typeKey: stringType, "minLength": 1, descriptionArg: "Canonical relative PNG/JPEG path, at most 4096 UTF-8 bytes."},
			"repo": map[string]any{typeKey: stringType, descriptionArg: "Optional attached repository path relative to the workspace, at most 4096 UTF-8 bytes."},
			"alt":  map[string]any{typeKey: stringType, "minLength": 1, "maxLength": 300, descriptionArg: "Description that identifies this image without seeing it."},
			"label": map[string]any{typeKey: stringType, "pattern": protocol.ImageLabelPattern,
				descriptionArg: "Optional one-word name shown in the image picker instead of a number, such as before, after, or alt. Unique within a question."},
		}},
	}
}

func questionImageSchemaDescription() string {
	return fmt.Sprintf("Optional PNG/JPEG previews. Each choice accepts at most %d image. "+
		"A question accepts at most %d references across its own images and choices. "+
		"A bundle accepts at most %d distinct image IDs. "+
		"Reuse an ID only with the same path, repo, and alt. Files are read from this session's workspace. "+
		"URLs and absolute paths are not supported. Inspecting a choice image does not select the choice. "+
		"The operator can open a large review view that compares a question's images: "+
		"put the baseline first and candidates after it.",
		protocol.MaxChoiceImages, protocol.MaxQuestionImageReferences, protocol.MaxBundleImages)
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
