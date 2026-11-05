package clarification

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type imageMessageRepo struct {
	*stubMessageStore
	task string
}

func (r imageMessageRepo) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	return &taskmodels.TaskSession{ID: id, TaskID: r.task}, nil
}

type imageReaderRecorder struct {
	calls   int
	session string
	ref     protocol.QuestionImage
}

func (r *imageReaderRecorder) ReadQuestionImage(_ context.Context, session string, ref protocol.QuestionImage) (*imagepreview.Content, error) {
	r.calls++
	r.session = session
	r.ref = ref
	return &imagepreview.Content{MIME: "image/png", Bytes: []byte("image")}, nil
}

func TestQuestionImageAccess(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		denied, parent, mismatch bool
		choice, wrongQuestion    bool
		want                     int
	}{
		{name: "authorized", want: 200}, {name: "denied", denied: true, want: 404}, {name: "parent", parent: true, want: 404}, {name: "mismatch", mismatch: true, want: 404},
		{name: "choice authorized", choice: true, want: 200},
		{name: "choice denied", choice: true, denied: true, want: 404},
		{name: "choice parent", choice: true, parent: true, want: 404},
		{name: "choice wrong question", choice: true, wrongQuestion: true, want: 404},
		{name: "choice wrong task", choice: true, mismatch: true, want: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := clarificationMessage("m", "bundle", "q/with space", 0, "Approve?", "yes")
			msg.Metadata["question"].(map[string]any)["images"] = []any{map[string]any{"id": "screen", "path": "screen.png", "alt": "Layout"}}
			if tc.choice {
				q := msg.Metadata["question"].(map[string]any)
				q["options"].([]interface{})[0].(map[string]interface{})["images"] = q["images"]
				delete(q, "images")
			}
			if tc.parent {
				msg.Metadata[taskmodels.MetaKeyParentQuestion] = true
			}
			task := "t1"
			if tc.mismatch {
				task = "foreign"
			}
			repo := imageMessageRepo{&stubMessageStore{messages: map[string][]*taskmodels.Message{"bundle": {msg}}}, task}
			authorizer := &keyedAuthorizer{allowed: map[string]bool{"t1": !tc.denied}}
			store := NewStore(time.Minute)
			creator := &stubMessageCreator{}
			events := &stubEventBus{}
			resolver := NewResolver(store, repo, creator, authorizer, events, events, nil, logger.Default())
			reader := &imageReaderRecorder{}
			router := gin.New()
			RegisterRoutes(router, store, nil, creator, repo, events, resolver, logger.Default(), nil, nil, false, reader)
			rec := httptest.NewRecorder()
			questionID := "q/with space"
			if tc.wrongQuestion {
				questionID = "other-question"
			}
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clarification/bundle/images/screen?question_id="+url.QueryEscape(questionID)+"&path=/secret.png", nil))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want == 200 {
				require.Equal(t, "s1", reader.session)
				require.Equal(t, "screen.png", reader.ref.Path)
				require.Equal(t, 1, reader.calls)
			} else {
				require.Zero(t, reader.calls)
			}
		})
	}
}
