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
			"path": map[string]any{typeKey: stringType, "minLength": 1, descriptionArg: "Relative PNG/JPEG path, at most 4096 UTF-8 bytes. When repo is omitted, use the source session workspace root. For a child repository, make path relative to that child."},
			"repo": map[string]any{typeKey: stringType, descriptionArg: "Optional child repository path relative to the workspace. When the workspace itself is the repository, omit repo. For compatibility, older calls can set repo to the workspace folder name. If the root has its own .git entry and no child has that name, the alias identifies the root. New calls must omit repo in that case."},
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
		"Reuse an ID only with the same path, repo, and alt. Image paths are relative to the selected source in the source session workspace. When that workspace is the repository, omit repo. For a child repository, set repo to its path relative to the workspace and make path relative to that child. "+
		"Files are read from this session's workspace. "+
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
