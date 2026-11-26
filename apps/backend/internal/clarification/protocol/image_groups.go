package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// QuestionImageGroupKind names how the review view shows a group of images.
type QuestionImageGroupKind string

const (
	// ImageGroupPair is two images meant to be compared, such as before and after.
	ImageGroupPair QuestionImageGroupKind = "pair"
	// ImageGroupSingle is one image shown on its own, without comparison.
	ImageGroupSingle QuestionImageGroupKind = "single"
)

// QuestionImageGroup references images of one question by ID, in review order.
type QuestionImageGroup struct {
	Kind     QuestionImageGroupKind `json:"kind"`
	ImageIDs []string               `json:"image_ids"`
}

// QuestionImageGroups distinguishes an omitted group list from an invalid null list.
type QuestionImageGroups []QuestionImageGroup

func (groups *QuestionImageGroups) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("image_groups must be an array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	result := make(QuestionImageGroups, 0, len(items))
	for _, item := range items {
		if bytes.Equal(bytes.TrimSpace(item), []byte("null")) {
			return fmt.Errorf("image group must be an object")
		}
		var value QuestionImageGroup
		decoder := json.NewDecoder(bytes.NewReader(item))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		result = append(result, value)
	}
	*groups = result
	return nil
}

// Equal reports whether two group lists have the same kinds and IDs in order.
func (groups QuestionImageGroups) Equal(other QuestionImageGroups) bool {
	return slices.EqualFunc(groups, other, func(a, b QuestionImageGroup) bool {
		return a.Kind == b.Kind && slices.Equal(a.ImageIDs, b.ImageIDs)
	})
}

// ValidateQuestionBundleImages runs the image rules and the review-group rules
// for a new bundle. Reads of saved images use ValidateQuestionImages alone, so
// a group that no longer validates never hides valid images.
func ValidateQuestionBundleImages(questions []Question) error {
	if err := ValidateQuestionImages(questions); err != nil {
		return err
	}
	for index, q := range questions {
		if err := validateImageGroups(index, q); err != nil {
			return err
		}
	}
	return nil
}

// QuestionLabel names a question by position and, when set, its ID.
func QuestionLabel(index int, id string) string {
	if id == "" {
		return fmt.Sprintf("question %d", index+1)
	}
	return fmt.Sprintf("question %d (%q)", index+1, id)
}

// validateImageGroups checks one question's groups against its own images.
func validateImageGroups(index int, q Question) error {
	if len(q.ImageGroups) == 0 {
		return nil
	}
	name := QuestionLabel(index, q.ID)
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%s: image_groups: %s", name, fmt.Sprintf(format, args...))
	}
	refs := q.ImageReferences()
	if len(refs) == 0 {
		return fail("groups need images on the question or its choices")
	}
	if len(q.ImageGroups) > MaxQuestionImageReferences {
		return fail("a question supports at most %d image groups", MaxQuestionImageReferences)
	}
	known := make(map[string]bool, len(refs))
	for _, ref := range refs {
		known[ref.ID] = true
	}
	grouped := map[string]bool{}
	for _, group := range q.ImageGroups {
		if err := validateGroupShape(group); err != "" {
			return fail("%s", err)
		}
		for _, id := range group.ImageIDs {
			if !known[id] {
				return fail("image %q is not an image of this question", id)
			}
			if grouped[id] {
				return fail("image %q is in more than one image group", id)
			}
			grouped[id] = true
		}
	}
	return nil
}

func validateGroupShape(group QuestionImageGroup) string {
	switch group.Kind {
	case ImageGroupPair:
		if len(group.ImageIDs) != 2 || group.ImageIDs[0] == group.ImageIDs[1] {
			return "a pair must list exactly 2 different image IDs"
		}
	case ImageGroupSingle:
		if len(group.ImageIDs) != 1 {
			return "a single must list exactly 1 image ID"
		}
	default:
		return `kind must be "pair" or "single"`
	}
	return ""
}
