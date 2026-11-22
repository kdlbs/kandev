package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func imageQuestions(t *testing.T, images string) ([]Question, error) {
	t.Helper()
	var questions []Question
	err := json.Unmarshal([]byte(`[{"id":"layout","prompt":"Approve?","options":[{"label":"Yes"},{"label":"No"}],"images":`+images+`}]`), &questions)
	return questions, err
}

// @covers AC-TASKS-QUESTION-IMAGES-001.3
func TestQuestionImagesValidation(t *testing.T) {
	valid := `[{"id":"screen","path":"design/screen.PNG","alt":"Screen layout"}]`
	for _, tc := range []struct {
		name, images string
		valid        bool
	}{
		{"valid", valid, true}, {"empty", `[]`, true},
		{"unicode", `[{"id":"a","path":"a.jpg","alt":"` + strings.Repeat("界", 300) + `"}]`, true},
		{"null", `null`, false}, {"object", `{}`, false}, {"null item", `[null]`, false},
		{"unknown key", `[{"id":"a","path":"a.png","alt":"a","url":"https://example.com"}]`, false},
		{"absolute", `[{"id":"a","path":"/tmp/a.png","alt":"a"}]`, false},
		{"traversal", `[{"id":"a","path":"../a.png","alt":"a"}]`, false},
		{"scheme", `[{"id":"a","path":"https:a.png","alt":"a"}]`, false},
		{"home", `[{"id":"a","path":"~/a.png","alt":"a"}]`, false},
		{"svg", `[{"id":"a","path":"a.svg","alt":"a"}]`, false},
		{"blank alt", `[{"id":"a","path":"a.png","alt":" "}]`, false},
		{"duplicate", `[{"id":"a","path":"a.png","alt":"a"},{"id":"a","path":"a.png","alt":"a"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qs, err := imageQuestions(t, tc.images)
			if err == nil {
				if msg := NormalizeAndValidateQuestions(qs); msg != "" {
					err = &imageValidationError{msg}
				}
			}
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	qs, err := imageQuestions(t, valid)
	require.NoError(t, err)
	data, err := json.Marshal(qs)
	require.NoError(t, err)
	require.Contains(t, string(data), `"images":[`)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"question_id":"layout","question":`+string(data[1:len(data)-1])+`}`), &metadata))
	listed := BundleQuestionStatuses([]*taskmodels.Message{{Metadata: metadata}})
	projected, err := json.Marshal(listed)
	require.NoError(t, err)
	require.Contains(t, string(projected), `"path":"design/screen.PNG"`)
}

type imageValidationError struct{ message string }

func (e *imageValidationError) Error() string { return e.message }

func TestQuestionImagesRetryIdentity(t *testing.T) {
	a, err := imageQuestions(t, `[{"id":"a","path":"a.png","alt":"A"}]`)
	require.NoError(t, err)
	b, err := imageQuestions(t, `[{"id":"a","path":"b.png","alt":"A"}]`)
	require.NoError(t, err)
	require.False(t, questionsEqual(a, b))
	require.NotEqual(t, PendingIDForRequest("s", "retry", a, ""), PendingIDForRequest("s", "retry", b, ""))
	b, err = imageQuestions(t, `[{"id":"a","path":"a.png","alt":"A"}]`)
	require.NoError(t, err)
	require.True(t, questionsEqual(a, b))
	b[0].ID = "different"
	require.False(t, questionsEqual(a, b))
}

func TestQuestionImagesBundleIdentityAndLegacyHash(t *testing.T) {
	a, err := imageQuestions(t, `[{"id":"a","path":"a.png","alt":"A"}]`)
	require.NoError(t, err)
	b, err := imageQuestions(t, `[{"id":"a","path":"a.png","alt":"A"}]`)
	require.NoError(t, err)
	b[0].ID = "second"
	require.Empty(t, NormalizeAndValidateQuestions(append(a, b...)))
	for _, field := range []string{"path", "repo", "alt"} {
		t.Run(field, func(t *testing.T) {
			changed := `[{"id":"a","path":"a.png","alt":"A"}]`
			switch field {
			case "path":
				changed = `[{"id":"a","path":"b.png","alt":"A"}]`
			case "repo":
				changed = `[{"id":"a","path":"a.png","repo":"other","alt":"A"}]`
			case "alt":
				changed = `[{"id":"a","path":"a.png","alt":"B"}]`
			}
			c, parseErr := imageQuestions(t, changed)
			require.NoError(t, parseErr)
			c[0].ID = "second"
			require.NotEmpty(t, NormalizeAndValidateQuestions(append(a, c...)))
		})
	}
	empty, err := imageQuestions(t, `[]`)
	require.NoError(t, err)
	encoded, err := json.Marshal(empty)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "images")
	var omitted []Question
	require.NoError(t, json.Unmarshal(encoded, &omitted))
	require.Equal(t, PendingIDForRequest("session", "key", omitted, ""), PendingIDForRequest("session", "key", empty, ""))
	freeText := true
	a[0].Options = nil
	a[0].AllowCustomText = &freeText
	a[0].Images[0].Path = "../private.png"
	require.NotEmpty(t, NormalizeAndValidateQuestionsAllowFreeTextOnly(a))
}

func TestQuestionImagesLimits(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"long alt", `[{"id":"a","path":"a.png","alt":"` + strings.Repeat("界", 301) + `"}]`},
		{"long path", `[{"id":"a","path":"` + strings.Repeat("a", 4097) + `.png","alt":"A"}]`},
		{"repo null", `[{"id":"a","path":"a.png","repo":null,"alt":"A"}]`},
		{"repo traversal", `[{"id":"a","path":"a.png","repo":"../other","alt":"A"}]`},
		{"bad id", `[{"id":"a/b","path":"a.png","alt":"A"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qs, err := imageQuestions(t, tc.raw)
			if err == nil {
				require.NotEmpty(t, NormalizeAndValidateQuestions(qs))
			}
		})
	}
	excess, err := imageQuestions(t, `[{"id":"a","path":"a.png","alt":"A"},{"id":"b","path":"b.png","alt":"B"},{"id":"c","path":"c.png","alt":"C"},{"id":"d","path":"d.png","alt":"D"},{"id":"e","path":"e.png","alt":"E"}]`)
	require.NoError(t, err)
	require.NotEmpty(t, NormalizeAndValidateQuestions(excess))
}

// galleryQuestions builds count questions that each carry perQuestion distinct images.
func galleryQuestions(t *testing.T, count, perQuestion int) []Question {
	t.Helper()
	var questions []Question
	for i := 0; i < count; i++ {
		refs := make([]string, 0, perQuestion)
		for j := 0; j < perQuestion; j++ {
			refs = append(refs, fmt.Sprintf(`{"id":"q%d_img%d","path":"q%d/%d.png","alt":"Image %d of question %d"}`, i, j, i, j, j, i))
		}
		qs, err := imageQuestions(t, "["+strings.Join(refs, ",")+"]")
		require.NoError(t, err)
		qs[0].ID = fmt.Sprint(i)
		questions = append(questions, qs...)
	}
	return questions
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-006.1
func TestQuestionImagesBundleAcceptsSixteenDistinctImages(t *testing.T) {
	require.Empty(t, NormalizeAndValidateQuestions(galleryQuestions(t, 4, 4)))
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-006.2
func TestQuestionImagesBundleRejectsSeventeenDistinctImages(t *testing.T) {
	questions := galleryQuestions(t, 5, 4)[:5]
	questions[4].Images = questions[4].Images[:1]
	err := protocol.ValidateQuestionImages(questions)
	require.Error(t, err)
	require.Contains(t, err.Error(), "16 distinct images")
}

func TestQuestionImagesDetachedResolutionPreservesMetadata(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprint(rejected), func(t *testing.T) {
			const pending = "image-detached"
			message := resolverDeliveryMessage(pending, "message", "turn-1")
			reference := []any{map[string]any{"id": "screen", "path": "screen.png", "alt": "Screen"}}
			message.Metadata[metaQuestionKey].(map[string]any)["images"] = reference
			resolver, _, repo, _, events := newResolverDeliveryFixture(t, pending, []*taskmodels.Message{message})
			outcome := resolverAnswer()
			if rejected {
				outcome = Outcome{Rejected: true}
			}
			if !rejected {
				events.resumeErr = errors.New("executor unavailable")
				_, claimed, err := resolver.ResolveBundle(context.Background(), pending, outcome)
				require.True(t, claimed)
				require.Error(t, err)
				require.Equal(t, "pending", repo.messages[pending][0].Metadata[metaStatusKey])
				require.Equal(t, reference, repo.messages[pending][0].Metadata[metaQuestionKey].(map[string]any)["images"])
				events.resumeErr = nil
			}
			resolution, claimed, err := resolver.ResolveBundle(context.Background(), pending, outcome)
			require.NoError(t, err)
			require.True(t, claimed)
			require.Equal(t, rejected, resolution.Response.Rejected)
			winner, claimed, err := resolver.ResolveBundle(context.Background(), pending, resolverAnswer())
			require.NoError(t, err)
			require.False(t, claimed)
			require.Equal(t, resolution.Response, winner.Response)
			if rejected {
				require.Empty(t, events.resumeRequests)
			} else {
				require.Len(t, events.resumeRequests, 2)
			}
			require.Equal(t, reference, repo.messages[pending][0].Metadata[metaQuestionKey].(map[string]any)["images"])
		})
	}
}
