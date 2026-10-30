package questions

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	defaultHTTPLimit = 50
	maxHTTPLimit     = 200
)

// WorkspaceAuthorizer is the workspace-scope check every operator read and
// response applies.
type WorkspaceAuthorizer interface {
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// TaskDirectory supplies bounded task/session enrichment for Inbox groups.
// QuestionImageReader resolves a saved image through the authorized source session.
type QuestionImageReader interface {
	ReadQuestionImage(context.Context, string, protocol.QuestionImage) (*imagepreview.Content, error)
}

type TaskDirectory interface {
	GetTaskSession(ctx context.Context, sessionID string) (*taskmodels.TaskSession, error)
	GetTasksByIDs(ctx context.Context, ids []string) ([]*taskmodels.Task, error)
	BatchGetSessionsForTasks(ctx context.Context, taskIDs []string) (map[string][]*taskmodels.TaskSession, error)
}

// HTTPHandlers serves the task-authorized async question REST surface.
type HTTPHandlers struct {
	service     *Service
	tasks       WorkspaceAuthorizer
	directory   TaskDirectory
	imageReader QuestionImageReader
	logger      *zap.Logger
}

// RegisterRoutes mounts the async question REST group.
func RegisterRoutes(router *gin.Engine, service *Service, tasks WorkspaceAuthorizer, directory TaskDirectory, logger *zap.Logger, imageReaders ...QuestionImageReader) {
	if router == nil || service == nil || tasks == nil {
		return
	}
	h := &HTTPHandlers{service: service, tasks: tasks, directory: directory, logger: logger}
	if len(imageReaders) > 0 {
		h.imageReader = imageReaders[0]
	}
	group := router.Group("/api/v1/agent-questions")
	group.GET("", h.list)
	group.POST("/:id/respond", h.respond)
	group.POST("/:id/withdraw", h.withdraw)
	group.GET("/:id/images/:imageID", h.image)
}

type questionView struct {
	QuestionID      string                           `json:"question_id"`
	TaskID          string                           `json:"task_id"`
	SessionID       string                           `json:"session_id"`
	TaskTitle       string                           `json:"task_title"`
	SessionState    string                           `json:"session_state"`
	Status          string                           `json:"status"`
	Recipient       string                           `json:"recipient"`
	Forwarded       bool                             `json:"forwarded"`
	ParentTaskID    string                           `json:"parent_task_id,omitempty"`
	ParentTaskTitle string                           `json:"parent_task_title,omitempty"`
	Context         string                           `json:"context"`
	Questions       []protocol.Question              `json:"questions"`
	Outcome         *taskmodels.AsyncQuestionOutcome `json:"outcome,omitempty"`
	WithdrawReason  string                           `json:"withdraw_reason,omitempty"`
	Waiting         bool                             `json:"waiting"`
	CreatedAt       string                           `json:"created_at"`
	UpdatedAt       string                           `json:"updated_at"`
}

type questionListResponse struct {
	Questions  []questionView `json:"questions"`
	Count      int            `json:"count"`
	HasMore    bool           `json:"has_more"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type listQuery struct {
	workspaceID string
	limit       int
	cursorAt    string
	cursorID    string
	status      string
	taskID      string
	sessionID   string
}

func (h *HTTPHandlers) list(c *gin.Context) {
	query, ok := parseListQuery(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := h.tasks.AuthorizeWorkspaceScope(ctx, query.workspaceID, authz.ScopeWorkspaceRead); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "workspace access is not authorized"})
		return
	}
	page, handled := h.readListPage(c, query)
	if handled {
		return
	}
	views := h.questionViews(ctx, page.Questions)
	response := questionListResponse{Questions: views, Count: len(views), HasMore: page.HasMore}
	if page.HasMore && len(page.Questions) > 0 {
		last := page.Questions[len(page.Questions)-1]
		response.NextCursor = encodeHTTPQuestionCursor(last.CreatedAt, last.ID)
	}
	c.JSON(http.StatusOK, response)
}

func parseListQuery(c *gin.Context) (listQuery, bool) {
	query := listQuery{
		workspaceID: strings.TrimSpace(c.Query("workspace_id")),
		limit:       defaultHTTPLimit,
		status:      strings.TrimSpace(c.Query("status")),
		taskID:      c.Query("task_id"),
		sessionID:   c.Query("session_id"),
	}
	if query.workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return query, false
	}
	if query.status != "" && query.status != "open" && query.status != "history" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be open or history"})
		return query, false
	}
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxHTTPLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 200"})
			return query, false
		}
		query.limit = parsed
	}
	cursorAt, cursorID, err := decodeHTTPQuestionCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cursor is malformed"})
		return query, false
	}
	query.cursorAt, query.cursorID = cursorAt, cursorID
	return query, true
}

func (h *HTTPHandlers) readListPage(c *gin.Context, query listQuery) (*taskmodels.AsyncQuestionPage, bool) {
	ctx := c.Request.Context()
	if query.taskID != "" || query.sessionID != "" {
		return h.readTaskPage(c, query)
	}
	var page *taskmodels.AsyncQuestionPage
	var err error
	if query.status == "history" {
		page, err = h.service.ListInboxHistory(ctx, query.workspaceID, query.limit, query.cursorAt, query.cursorID)
	} else {
		page, err = h.service.ListInbox(ctx, query.workspaceID, query.limit, query.cursorAt, query.cursorID)
	}
	if err != nil {
		h.writeError(c, err)
		return nil, true
	}
	return page, false
}

func (h *HTTPHandlers) readTaskPage(c *gin.Context, query listQuery) (*taskmodels.AsyncQuestionPage, bool) {
	ctx := c.Request.Context()
	if h.directory == nil {
		c.JSON(http.StatusOK, questionListResponse{Questions: []questionView{}})
		return nil, true
	}
	taskID, sessionID, ok := h.resolveScopedTask(ctx, query)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task or session not found"})
		return nil, true
	}
	if !h.taskBelongsToWorkspace(ctx, taskID, query.workspaceID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "task or session not found"})
		return nil, true
	}
	page, err := h.service.ListOwn(ctx, sessionID, nil, nil, query.limit, query.cursorAt, query.cursorID)
	if err != nil {
		h.writeError(c, err)
		return nil, true
	}
	return page, false
}

// resolveScopedTask binds a task/session read to the requested workspace so a
// reader in another workspace cannot enumerate questions by guessing an ID.
func (h *HTTPHandlers) resolveScopedTask(ctx context.Context, query listQuery) (string, string, bool) {
	if query.sessionID != "" {
		session, err := h.directory.GetTaskSession(ctx, query.sessionID)
		if err != nil || session == nil {
			return "", "", false
		}
		if query.taskID != "" && query.taskID != session.TaskID {
			return "", "", false
		}
		return session.TaskID, session.ID, true
	}
	sessionsByTask, err := h.directory.BatchGetSessionsForTasks(ctx, []string{query.taskID})
	if err != nil || len(sessionsByTask[query.taskID]) == 0 {
		return "", "", false
	}
	return query.taskID, sessionsByTask[query.taskID][0].ID, true
}

func (h *HTTPHandlers) taskBelongsToWorkspace(ctx context.Context, taskID, workspaceID string) bool {
	tasks, err := h.directory.GetTasksByIDs(ctx, []string{taskID})
	if err != nil || len(tasks) == 0 {
		return false
	}
	return tasks[0] != nil && tasks[0].WorkspaceID == workspaceID
}

type respondBody struct {
	Answers      []protocol.Answer `json:"answers"`
	Rejected     bool              `json:"rejected"`
	RejectReason string            `json:"reject_reason"`
	SubmissionID string            `json:"submission_id"`
}

func (h *HTTPHandlers) respond(c *gin.Context) {
	ctx := c.Request.Context()
	var body respondBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	question, err := h.service.Get(ctx, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if h.tasks == nil || question.WorkspaceID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		return
	}
	if err := h.tasks.AuthorizeWorkspaceScope(ctx, question.WorkspaceID, authz.ScopeTaskWrite); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "workspace access is not authorized"})
		return
	}
	input := AnswerInput{Rejected: body.Rejected, RejectReason: body.RejectReason}
	for _, answer := range body.Answers {
		input.Answers = append(input.Answers, Answer{
			QuestionID:      answer.QuestionID,
			SelectedOptions: answer.SelectedOptions,
			CustomText:      answer.CustomText,
		})
	}
	updated, won, err := h.service.ResolveOperator(ctx, question.ID, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	views := h.questionViews(ctx, []*taskmodels.AsyncQuestion{updated})
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"won":       won,
		"status":    string(updated.Status),
		"questions": views,
	})
}

func (h *HTTPHandlers) withdraw(c *gin.Context) {
	// Operator withdrawal is deliberately absent: only the author session may
	// withdraw. The route exists so an accidental operator call is an explicit
	// non-disclosing failure rather than a mutation.
	c.JSON(http.StatusForbidden, gin.H{"error": "only the author session can withdraw a question"})
}

func (h *HTTPHandlers) image(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	question, err := h.service.Get(ctx, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if question.Recipient == taskmodels.AsyncQuestionRecipientParent || h.imageReader == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if h.tasks == nil || question.WorkspaceID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err := h.tasks.AuthorizeWorkspaceScope(ctx, question.WorkspaceID, authz.ScopeWorkspaceRead); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	ref, ok := findAsyncQuestionImage(question.Questions, c.Query("question_id"), c.Param("imageID"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	content, err := h.imageReader.ReadQuestionImage(ctx, question.SessionID, ref)
	if err != nil || content == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, content.MIME, content.Bytes)
}

func findAsyncQuestionImage(questions []protocol.Question, questionID, imageID string) (protocol.QuestionImage, bool) {
	for _, question := range questions {
		if question.ID != questionID {
			continue
		}
		for _, ref := range question.ImageReferences() {
			if ref.ID == imageID {
				return ref, true
			}
		}
	}
	return protocol.QuestionImage{}, false
}

func (h *HTTPHandlers) questionViews(ctx context.Context, questions []*taskmodels.AsyncQuestion) []questionView {
	views := make([]questionView, 0, len(questions))
	if len(questions) == 0 {
		return views
	}
	titles, sessionStates := h.loadQuestionEnrichment(ctx, questions)
	waitingBySession := h.waitingSessions(ctx, questions)
	for _, question := range questions {
		views = append(views, buildQuestionView(question, titles, sessionStates, waitingBySession))
	}
	return views
}

func (h *HTTPHandlers) loadQuestionEnrichment(
	ctx context.Context,
	questions []*taskmodels.AsyncQuestion,
) (map[string]string, map[string]string) {
	taskIDs := questionTaskIDs(questions)
	titles := map[string]string{}
	sessionStates := map[string]string{}
	if h.directory == nil {
		return titles, sessionStates
	}
	if tasks, err := h.directory.GetTasksByIDs(ctx, taskIDs); err == nil {
		for _, task := range tasks {
			if task != nil {
				titles[task.ID] = task.Title
			}
		}
	}
	if sessionsByTask, err := h.directory.BatchGetSessionsForTasks(ctx, taskIDs); err == nil {
		for _, sessions := range sessionsByTask {
			for _, session := range sessions {
				if session != nil {
					sessionStates[session.ID] = string(session.State)
				}
			}
		}
	}
	return titles, sessionStates
}

func questionTaskIDs(questions []*taskmodels.AsyncQuestion) []string {
	taskIDs := make([]string, 0, len(questions)*2)
	seen := make(map[string]bool, len(questions)*2)
	for _, question := range questions {
		for _, id := range []string{question.TaskID, question.ParentTaskID} {
			if id != "" && !seen[id] {
				seen[id] = true
				taskIDs = append(taskIDs, id)
			}
		}
	}
	return taskIDs
}

func (h *HTTPHandlers) waitingSessions(ctx context.Context, questions []*taskmodels.AsyncQuestion) map[string]bool {
	waiting := make(map[string]bool, len(questions))
	for _, question := range questions {
		if _, seen := waiting[question.SessionID]; seen {
			continue
		}
		waiting[question.SessionID] = h.service.HasParkedWait(ctx, question.SessionID)
	}
	return waiting
}

func buildQuestionView(
	question *taskmodels.AsyncQuestion,
	titles, sessionStates map[string]string,
	waitingBySession map[string]bool,
) questionView {
	return questionView{
		QuestionID:      question.ID,
		TaskID:          question.TaskID,
		SessionID:       question.SessionID,
		TaskTitle:       titles[question.TaskID],
		SessionState:    sessionStates[question.SessionID],
		Status:          string(question.Status),
		Recipient:       string(question.Recipient),
		Forwarded:       question.Forwarded,
		ParentTaskID:    question.ParentTaskID,
		ParentTaskTitle: titles[question.ParentTaskID],
		Context:         question.Context,
		Questions:       question.Questions,
		Outcome:         question.Outcome,
		WithdrawReason:  question.WithdrawReason,
		Waiting:         waitingBySession[question.SessionID],
		CreatedAt:       question.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       question.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *HTTPHandlers) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
	case errors.Is(err, ErrValidation), errors.Is(err, ErrConflict):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnauthorized):
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
	case errors.Is(err, ErrNotPending):
		c.JSON(http.StatusConflict, gin.H{"error": "question is no longer pending"})
	default:
		h.logger.Error("async question request failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "async question request failed"})
	}
}

func encodeHTTPQuestionCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeHTTPQuestionCursor(cursor string) (string, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return "", "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("malformed cursor")
	}
	if _, err := time.Parse(time.RFC3339Nano, parts[0]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}
