package messagequeue

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

// PlanCommentQueueRequest describes a caller-identified queued prompt that
// consumes exact task-owned plan comments when admitted.
type PlanCommentQueueRequest struct {
	ClientQueueID         string
	SessionID             string
	TaskID                string
	SessionIncarnationID  string
	Content               string
	Model                 string
	UserID                string
	PlanMode              bool
	Attachments           []MessageAttachment
	Metadata              map[string]interface{}
	PlanCommentRefs       []models.TaskPlanCommentRef
	RequirePrimarySession bool
	AttachmentClaim       *QueueAttachmentClaim
}

// PlanCommentQueueResult distinguishes a first admission from an exact replay.
type PlanCommentQueueResult struct {
	Message  *QueuedMessage
	Snapshot *models.TaskPlanCommentSnapshot
	Replay   bool
}

type planCommentQueueWriter interface {
	InsertWithPlanComments(
		context.Context,
		QueueSessionIdentity,
		*QueuedMessage,
		[]models.TaskPlanCommentRef,
		bool,
		*QueueAttachmentClaim,
		int,
	) (*models.TaskPlanCommentSnapshot, bool, error)
}

type planCommentQueueReplayIdentity struct {
	ClientQueueID         string                      `json:"client_queue_id"`
	SessionID             string                      `json:"session_id"`
	TaskID                string                      `json:"task_id"`
	SessionIncarnationID  string                      `json:"session_incarnation_id"`
	Content               string                      `json:"content"`
	Model                 string                      `json:"model"`
	UserID                string                      `json:"user_id"`
	PlanMode              bool                        `json:"plan_mode"`
	Attachments           []MessageAttachment         `json:"attachments"`
	Metadata              map[string]interface{}      `json:"metadata"`
	Refs                  []models.TaskPlanCommentRef `json:"refs"`
	RequirePrimarySession bool                        `json:"require_primary_session"`
}

// QueueMessageWithPlanComments admits a comment-bearing row without automatic
// merging. The repository resolves server-owned comment content and consumes
// it in the same database transaction as the queue insert.
func (s *Service) QueueMessageWithPlanComments(
	ctx context.Context,
	req PlanCommentQueueRequest,
) (*PlanCommentQueueResult, error) {
	if req.ClientQueueID == "" {
		return nil, errors.New("client queue id is required")
	}
	if len(req.PlanCommentRefs) == 0 {
		return nil, errors.New("plan comment refs are required")
	}
	identity := QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return nil, ErrSessionIdentityMismatch
	}
	writer, ok := s.repo.(planCommentQueueWriter)
	if !ok {
		return nil, errors.New("plan comment queue admission is unavailable")
	}
	metadata, err := preparePlanCommentQueueMetadata(req)
	if err != nil {
		return nil, err
	}
	message := &QueuedMessage{
		ID: req.ClientQueueID, SessionID: req.SessionID, TaskID: req.TaskID,
		Content: plancomments.WithPlaceholder(req.Content), Model: req.Model,
		PlanMode: req.PlanMode, Attachments: req.Attachments, Metadata: metadata,
		QueuedBy: req.UserID,
	}
	releaseAdmission, err := plancommenttx.AcquireLocalAdmission(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	defer releaseAdmission()
	ctx = plancommenttx.WithLocalAdmission(ctx, req.TaskID)
	var result *PlanCommentQueueResult
	err = s.WithSessionAdmission(ctx, req.SessionID, func(admittedCtx context.Context) error {
		snapshot, replay, insertErr := writer.InsertWithPlanComments(
			admittedCtx, identity, message, req.PlanCommentRefs, req.RequirePrimarySession,
			req.AttachmentClaim, s.MaxPerSession(),
		)
		if insertErr != nil {
			return insertErr
		}
		result = &PlanCommentQueueResult{Message: message, Snapshot: snapshot, Replay: replay}
		return nil
	})
	return result, err
}

func preparePlanCommentQueueMetadata(req PlanCommentQueueRequest) (map[string]interface{}, error) {
	metadata := make(map[string]interface{}, len(req.Metadata)+3)
	for key, value := range req.Metadata {
		if key != plancomments.MetadataRefs &&
			key != plancomments.MetadataRequestFingerprint &&
			key != plancomments.MetadataClientQueueID {
			metadata[key] = value
		}
	}
	identity := planCommentQueueReplayIdentity{
		ClientQueueID: req.ClientQueueID, SessionID: req.SessionID, TaskID: req.TaskID,
		SessionIncarnationID: req.SessionIncarnationID,
		Content:              req.Content, Model: req.Model, UserID: req.UserID, PlanMode: req.PlanMode,
		Attachments: req.Attachments, Metadata: metadata, Refs: req.PlanCommentRefs,
		RequirePrimarySession: req.RequirePrimarySession,
	}
	fingerprint, err := plancomments.Fingerprint(identity)
	if err != nil {
		return nil, err
	}
	metadata[plancomments.MetadataRefs] = req.PlanCommentRefs
	metadata[plancomments.MetadataRequestFingerprint] = fingerprint
	metadata[plancomments.MetadataClientQueueID] = req.ClientQueueID
	return metadata, nil
}
