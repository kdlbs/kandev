package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

// imageIDSchemaPattern mirrors the protocol's image ID rule for agent-facing schemas.
const imageIDSchemaPattern = `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`

func buildOperatorQuestionSchemaItem() map[string]any {
	schema := buildQuestionSchemaItem()
	props := schema[propsKey].(map[string]any)
	props["images"] = buildQuestionImageSchema(protocol.MaxQuestionImageReferences)
	props["image_groups"] = buildQuestionImageGroupSchema()
	option := props[optionsArg].(map[string]any)["items"].(map[string]any)
	option[propsKey].(map[string]any)["images"] = buildQuestionImageSchema(protocol.MaxChoiceImages)
	return schema
}

func buildQuestionImageSchema(maxItems int) map[string]any {
	return map[string]any{
		typeKey: "array", "maxItems": maxItems,
		descriptionArg: questionImageSchemaDescription(),
		"items": map[string]any{typeKey: objType, "additionalProperties": false, reqKey: []string{"id", "path", "alt"}, propsKey: map[string]any{
			"id":   map[string]any{typeKey: stringType, "pattern": imageIDSchemaPattern},
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
		"The operator can open a large review view. "+
		"Use image_groups to say which images it compares as pairs and which it shows alone.",
		protocol.MaxChoiceImages, protocol.MaxQuestionImageReferences, protocol.MaxBundleImages)
}

func buildQuestionImageGroupSchema() map[string]any {
	return map[string]any{
		typeKey: "array", "maxItems": protocol.MaxQuestionImageReferences,
		descriptionArg: fmt.Sprintf("Optional review grouping for this question's images, in review order. "+
			"A pair lists exactly 2 different image IDs that show two versions of the same view, such as before and after; "+
			"the review view compares them with the first as the baseline. "+
			"A single lists exactly 1 image ID for an image with no counterpart; it is shown alone without comparison. "+
			"IDs refer to images on this question or its choices, and each ID can be in at most one group. "+
			"Images that no group lists are shown alone after the groups. "+
			"Omit image_groups to show the images as a gallery. At most %d groups.", protocol.MaxQuestionImageReferences),
		"items": map[string]any{typeKey: objType, "additionalProperties": false, reqKey: []string{"kind", "image_ids"}, propsKey: map[string]any{
			"kind": map[string]any{typeKey: stringType, "enum": []string{string(protocol.ImageGroupPair), string(protocol.ImageGroupSingle)}},
			"image_ids": map[string]any{typeKey: "array", "minItems": 1, "maxItems": 2,
				"items": map[string]any{typeKey: stringType, "pattern": imageIDSchemaPattern}},
		}},
	}
}

// validateQuestionImagePayloads decodes each question on its own so a decode
// error names the question it came from.
func validateQuestionImagePayloads(questions []map[string]interface{}) error {
	typed := make([]protocol.Question, len(questions))
	for index, question := range questions {
		data, err := json.Marshal(question)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &typed[index]); err != nil {
			id, _ := question["id"].(string)
			return fmt.Errorf("%s: %w", protocol.QuestionLabel(index, id), err)
		}
	}
	return protocol.ValidateQuestionBundleImages(typed)
}
