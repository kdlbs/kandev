package clarification

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

const (
	maxAnswerAnnotations       = 8
	maxAnnotationCommentRunes  = 500
	maxAnnotationImagePixels   = 16_000_000
	annotationRegionLinePrefix = "Region %d: "
)

// validateAnswerAnnotations checks operator-drawn regions before the claim.
// The server cannot see the operator's image, so it checks the rectangle
// against the reported image size and the image against the question.
func validateAnswerAnnotations(i int, a Answer, q Question) error {
	if len(a.Annotations) == 0 {
		return nil
	}
	if len(a.Annotations) > maxAnswerAnnotations {
		return newValidationError("answer %d has more than %d region annotations", i+1, maxAnswerAnnotations)
	}
	images := make(map[string]bool)
	for _, ref := range q.ImageReferences() {
		images[ref.ID] = true
	}
	for j, annotation := range a.Annotations {
		if reason := annotationProblem(annotation, images); reason != "" {
			return newValidationError("answer %d region %d %s", i+1, j+1, reason)
		}
	}
	return nil
}

func annotationProblem(a protocol.AnswerAnnotation, images map[string]bool) string {
	if !images[a.ImageID] {
		return fmt.Sprintf("references image %q, which is not on this question", a.ImageID)
	}
	if !validAnnotationImageSize(a.ImageWidth, a.ImageHeight) {
		return fmt.Sprintf("must report an image size of at most %d pixels", maxAnnotationImagePixels)
	}
	if !annotationWithinImage(a) {
		return "must lie within the reported image size"
	}
	return annotationCommentProblem(a.Comment)
}

func validAnnotationImageSize(width, height int) bool {
	if width < 1 || height < 1 || width > maxAnnotationImagePixels || height > maxAnnotationImagePixels {
		return false
	}
	return int64(width)*int64(height) <= maxAnnotationImagePixels
}

// annotationWithinImage compares by subtraction so large values cannot overflow.
func annotationWithinImage(a protocol.AnswerAnnotation) bool {
	return a.X >= 0 && a.Y >= 0 && a.Width >= 1 && a.Height >= 1 &&
		a.X < a.ImageWidth && a.Y < a.ImageHeight &&
		a.Width <= a.ImageWidth-a.X && a.Height <= a.ImageHeight-a.Y
}

func annotationCommentProblem(raw string) string {
	comment := strings.TrimSpace(raw)
	if comment == "" || utf8.RuneCountInString(comment) > maxAnnotationCommentRunes {
		return fmt.Sprintf("comment must contain 1-%d characters", maxAnnotationCommentRunes)
	}
	if strings.ContainsRune(comment, 0) {
		return "comment must not contain NUL characters"
	}
	return ""
}

// normalizeAnswerAnnotations trims comments and drops an empty list so
// answers without regions keep their existing encoding.
func normalizeAnswerAnnotations(annotations []protocol.AnswerAnnotation) []protocol.AnswerAnnotation {
	if len(annotations) == 0 {
		return nil
	}
	out := make([]protocol.AnswerAnnotation, 0, len(annotations))
	for _, annotation := range annotations {
		annotation.Comment = strings.TrimSpace(annotation.Comment)
		out = append(out, annotation)
	}
	return out
}

// decodeAnswerAnnotations reads stored metadata.response.annotations. A
// malformed list is dropped rather than hiding the rest of the answer.
func decodeAnswerAnnotations(raw any) []protocol.AnswerAnnotation {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var annotations []protocol.AnswerAnnotation
	if json.Unmarshal(data, &annotations) != nil || len(annotations) == 0 {
		return nil
	}
	return annotations
}

// formatAnnotationLines renders one line per region. Each entry is a JSON
// object, so a comment's quotes and newlines stay escaped inside its own string
// and cannot start another entry. Region numbers come from list position.
func formatAnnotationLines(annotations []protocol.AnswerAnnotation) []string {
	lines := make([]string, 0, len(annotations))
	for i, annotation := range annotations {
		encoded, err := json.Marshal(annotation)
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf(annotationRegionLinePrefix, i+1)+string(encoded))
	}
	return lines
}
