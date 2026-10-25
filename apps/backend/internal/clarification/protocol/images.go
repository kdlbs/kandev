package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/workspacepath"
)

// QuestionImage identifies a raster file in the source session's workspace.
type QuestionImage struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
	Alt  string `json:"alt"`
}

// QuestionImages distinguishes an omitted attachment list from an invalid null list.
type QuestionImages []QuestionImage

func (images *QuestionImages) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("images must be an array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	result := make(QuestionImages, 0, len(items))
	for _, item := range items {
		if bytes.Equal(bytes.TrimSpace(item), []byte("null")) {
			return fmt.Errorf("image must be an object")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			return err
		}
		for key, raw := range fields {
			var value string
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("image %s must be a string", key)
			}
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("image %s must be a string", key)
			}
		}
		var value QuestionImage
		decoder := json.NewDecoder(bytes.NewReader(item))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		result = append(result, value)
	}
	*images = result
	return nil
}

var imageIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var imageSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// ValidateImagePath accepts canonical workspace-relative paths without expansion.
func ValidateImagePath(value string) error {
	if len(value) > 4096 || strings.HasPrefix(value, "~") || imageSchemePattern.MatchString(value) {
		return fmt.Errorf("image paths must be workspace-relative and at most 4096 bytes")
	}
	return workspacepath.ValidateTreePath(value)
}

func (image QuestionImage) Validate() error {
	if !imageIDPattern.MatchString(image.ID) {
		return fmt.Errorf("image id must be 1-64 letters, digits, underscores, or hyphens, starting with a letter or digit")
	}
	if strings.TrimSpace(image.Alt) == "" || utf8.RuneCountInString(image.Alt) > 300 {
		return fmt.Errorf("image alt must contain 1-300 characters")
	}
	if image.Path == "" {
		return fmt.Errorf("image path is required")
	}
	if err := ValidateImagePath(image.Path); err != nil {
		return err
	}
	if err := ValidateImagePath(image.Repo); err != nil {
		return err
	}
	switch strings.ToLower(path.Ext(image.Path)) {
	case ".png", ".jpg", ".jpeg":
		return nil
	default:
		return fmt.Errorf("image path must end in .png, .jpg, or .jpeg")
	}
}

// ValidateQuestionImages bounds placements and enforces bundle-wide image identity.
func ValidateQuestionImages(questions []Question) error {
	shared := map[string]QuestionImage{}
	for _, q := range questions {
		if len(q.ImageReferences()) > 4 {
			return fmt.Errorf("each question supports at most 4 image references across the question and choices")
		}
		groups := []QuestionImages{q.Images}
		for _, option := range q.Options {
			if len(option.Images) > 1 {
				return fmt.Errorf("each choice supports at most 1 image")
			}
			groups = append(groups, option.Images)
		}
		for _, images := range groups {
			if err := validateImageGroup(images, shared); err != nil {
				return err
			}
		}
	}
	if len(shared) > 4 {
		return fmt.Errorf("a question bundle supports at most 4 distinct images")
	}
	return nil
}

func validateImageGroup(images QuestionImages, shared map[string]QuestionImage) error {
	seen := map[string]bool{}
	for _, image := range images {
		if err := image.Validate(); err != nil {
			return err
		}
		if seen[image.ID] {
			return fmt.Errorf("duplicate image id %q in question", image.ID)
		}
		seen[image.ID] = true
		if previous, exists := shared[image.ID]; exists && previous != image {
			return fmt.Errorf("image id %q has conflicting references", image.ID)
		}
		shared[image.ID] = image
	}
	return nil
}

// ImageReferences returns previews from this question and its choices.
func (q Question) ImageReferences() QuestionImages {
	refs := append(QuestionImages(nil), q.Images...)
	for _, option := range q.Options {
		refs = append(refs, option.Images...)
	}
	return refs
}

func HasQuestionImages(questions []Question) bool {
	for _, q := range questions {
		if len(q.ImageReferences()) > 0 {
			return true
		}
	}
	return false
}
