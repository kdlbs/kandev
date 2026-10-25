package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func uploadAttachmentRequest(t *testing.T, router *gin.Engine, filename, data string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(data))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/tasks/task1/documents/artifact/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertAttachmentDownload(t *testing.T, router *gin.Engine, filename, data string) {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/artifact/download", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, data, w.Body.String())
	require.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "attachment; filename=\""+filename+"\"", w.Header().Get("Content-Disposition"))
}

type attachmentFailureRepo struct {
	repository.DocumentRepository
	failure string
	err     error
}

func (r *attachmentFailureRepo) GetDocument(ctx context.Context, taskID, key string) (*models.TaskDocument, error) {
	if r.failure == "lookup" {
		return nil, r.err
	}
	return r.DocumentRepository.GetDocument(ctx, taskID, key)
}

func (r *attachmentFailureRepo) UpdateDocument(ctx context.Context, doc *models.TaskDocument) error {
	return r.err
}

func (r *attachmentFailureRepo) CreateDocument(ctx context.Context, doc *models.TaskDocument) error {
	return r.err
}

func attachmentFailureRouter(d *documentTestDeps, failure string) *gin.Engine {
	log := logger.Default()
	repo := &attachmentFailureRepo{DocumentRepository: d.repo, failure: failure, err: errors.New("reject attachment metadata")}
	svc := taskservice.NewDocumentService(repo, log)
	router := gin.New()
	dashboard.RegisterDocumentRoutes(router.Group("/api/v1/office"), dashboard.NewDocumentHandler(svc, d.tmpDir, log))
	return router
}

// @covers AC-TASKS-DOCUMENTS-002.5
func TestDocumentHandler_AttachmentFailedReplacement(t *testing.T) {
	for _, failure := range []string{"lookup", "update"} {
		for _, filename := range []string{"second.bin", "second.pdf"} {
			t.Run(failure+"/"+filename, func(t *testing.T) {
				d := newDocumentTestDeps(t)
				seedTask(t, d, "task1")
				first := uploadAttachmentRequest(t, d.router, "first.bin", "published bytes")
				require.Equal(t, http.StatusOK, first.Code, first.Body.String())
				before, err := d.repo.GetDocument(context.Background(), "task1", "artifact")
				require.NoError(t, err)
				rejected := uploadAttachmentRequest(t, attachmentFailureRouter(d, failure), filename, "rejected bytes")
				require.Equal(t, http.StatusInternalServerError, rejected.Code)
				require.Contains(t, rejected.Body.String(), "reject attachment metadata")
				after, err := d.repo.GetDocument(context.Background(), "task1", "artifact")
				require.NoError(t, err)
				require.Equal(t, before, after)
				assertAttachmentDownload(t, d.router, "first.bin", "published bytes")
			})
		}
	}
}

// @covers AC-TASKS-DOCUMENTS-002.5
func TestDocumentHandler_AttachmentSuccessfulReplacement(t *testing.T) {
	for _, filename := range []string{"second.bin", "second.pdf"} {
		t.Run(filename, func(t *testing.T) {
			d := newDocumentTestDeps(t)
			seedTask(t, d, "task1")
			first := uploadAttachmentRequest(t, d.router, "first.bin", "original")
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			var original dashboard.DocumentResponse
			require.NoError(t, json.Unmarshal(first.Body.Bytes(), &original))
			assertAttachmentDownload(t, d.router, "first.bin", "original")
			second := uploadAttachmentRequest(t, d.router, filename, "replacement bytes")
			require.Equal(t, http.StatusOK, second.Code, second.Body.String())
			var replacement dashboard.DocumentResponse
			require.NoError(t, json.Unmarshal(second.Body.Bytes(), &replacement))
			require.Equal(t, original.Document.ID, replacement.Document.ID)
			require.Equal(t, original.Document.CreatedAt, replacement.Document.CreatedAt)
			require.Equal(t, filename, replacement.Document.Filename)
			require.EqualValues(t, len("replacement bytes"), replacement.Document.SizeBytes)
			require.NotContains(t, second.Body.String(), "disk_path")
			assertAttachmentDownload(t, d.router, filename, "replacement bytes")
		})
	}
}

// @covers AC-TASKS-DOCUMENTS-002.2
func TestDocumentHandler_AttachmentRejectedFirstUpload(t *testing.T) {
	d := newDocumentTestDeps(t)
	seedTask(t, d, "task1")
	w := uploadAttachmentRequest(t, attachmentFailureRouter(d, "create"), "first.bin", "unpublished bytes")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	w = httptest.NewRecorder()
	d.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task1/documents/artifact/download", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}
