package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/tiktoken-go/tokenizer"
	"go.uber.org/zap"
)

const (
	conversationForkDraftTTL       = 24 * time.Hour
	conversationForkCompilerMethod = "o200k_base:conversation-fork-v1"
	conversationForkStateDraft     = "draft"
	conversationForkStateAttached  = "attached"
	conversationForkStateDiscarded = "discarded"
)

var conversationForkCodec struct {
	sync.Once
	codec tokenizer.Codec
	err   error
}

// ListConversationForkCandidates returns an authorized, bounded source range
// and the attachments that can be explicitly copied into a fork draft.
func (s *Service) ListConversationForkCandidates(ctx context.Context, req models.ConversationForkSourceRequest) (models.ConversationForkSource, error) {
	if err := s.AuthorizeSessionAccess(ctx, req.SessionID); err != nil {
		return models.ConversationForkSource{}, err
	}
	reader, ok := s.messages.(taskrepo.ConversationForkSourceRepository)
	if !ok {
		return models.ConversationForkSource{}, models.ErrConversationForkSourceUnavailable
	}
	source, err := reader.ReadConversationForkSource(ctx, req)
	if err != nil {
		return models.ConversationForkSource{}, err
	}
	source.AttachmentCandidates = s.conversationForkAttachmentCandidates(ctx, source.Attachments)
	return source, nil
}

// CreateConversationForkDraft compiles one immutable, owner-scoped preview
// from a transaction-consistent source range.
func (s *Service) CreateConversationForkDraft(ctx context.Context, req models.ConversationForkCreateRequest) (models.ConversationForkDraft, error) {
	if err := normalizeConversationForkCreateRequest(&req); err != nil {
		return models.ConversationForkDraft{}, err
	}
	compiledRequest, err := s.compileConversationForkRequest(ctx, req)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	ownerID, err := s.conversationForkOwnerID(ctx, compiledRequest.source.WorkspaceID)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	selectionJSON, fingerprint, err := encodeConversationForkCreateRequest(req)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	estimate, err := s.estimateConversationFork(ctx, compiledRequest.compiled, req.ModelID, len(compiledRequest.selectedAttachments) > 0)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	descriptors, copies, err := s.copyConversationForkAttachments(ctx, ownerID, compiledRequest.source.WorkspaceID, compiledRequest.source.TaskID, compiledRequest.selectedAttachments)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	draft := newConversationForkDraft(req, compiledRequest, ownerID, selectionJSON, fingerprint, descriptors, estimate)
	return s.persistConversationForkDraft(ctx, draft, copies, descriptors)
}

type compiledConversationForkRequest struct {
	source              models.ConversationForkSource
	selectedAttachments []*models.TaskMessageAttachment
	compiled            string
	count               int
	omissions           map[string]int
}

func normalizeConversationForkCreateRequest(req *models.ConversationForkCreateRequest) error {
	if strings.TrimSpace(req.DraftRequestID) == "" || len(req.DraftRequestID) > 128 {
		return models.ErrConversationForkConflict
	}
	if len(req.AttachmentIDs) > MaxAttachmentCount {
		return models.ErrConversationForkLimitExceeded
	}
	req.AttachmentIDs = append([]string(nil), req.AttachmentIDs...)
	seenAttachments := make(map[string]struct{}, len(req.AttachmentIDs))
	for _, id := range req.AttachmentIDs {
		if id == "" {
			return models.ErrConversationForkAttachmentMissing
		}
		if _, exists := seenAttachments[id]; exists {
			return models.ErrConversationForkConflict
		}
		seenAttachments[id] = struct{}{}
	}
	sort.Strings(req.AttachmentIDs)
	return nil
}

func (s *Service) compileConversationForkRequest(ctx context.Context, req models.ConversationForkCreateRequest) (compiledConversationForkRequest, error) {
	if err := s.AuthorizeSessionAccess(ctx, req.Source.SessionID); err != nil {
		return compiledConversationForkRequest{}, err
	}
	reader, ok := s.messages.(taskrepo.ConversationForkSourceRepository)
	if !ok {
		return compiledConversationForkRequest{}, models.ErrConversationForkSourceUnavailable
	}
	sourceRequest := req.Source
	sourceRequest.IncludeToolEvidence = req.IncludeToolEvidence
	sourceRequest.SelectedAttachmentIDs = append([]string(nil), req.AttachmentIDs...)
	source, err := reader.ReadConversationForkSource(ctx, sourceRequest)
	if err != nil {
		return compiledConversationForkRequest{}, err
	}
	sortConversationForkAttachments(source.Messages, source.Attachments)
	var selectedAttachments []*models.TaskMessageAttachment
	if len(req.AttachmentIDs) > 0 {
		selectedAttachments = source.Attachments
	}
	inherited, inheritedMessageID, err := s.inheritedConversationForkContext(ctx, source)
	if err != nil {
		return compiledConversationForkRequest{}, err
	}
	compiled, count, omissions, err := compileConversationForkWithInherited(
		source.Messages, req.Source.CutoffMessageID, req.Source.StartMessageID, req.IncludeToolEvidence,
		inherited, inheritedMessageID, selectedAttachments,
	)
	if err != nil {
		return compiledConversationForkRequest{}, conversationForkCompileError(err)
	}
	return compiledConversationForkRequest{
		source: source, selectedAttachments: selectedAttachments, compiled: compiled, count: count, omissions: omissions,
	}, nil
}

func (s *Service) inheritedConversationForkContext(
	ctx context.Context,
	source models.ConversationForkSource,
) (*models.ConversationForkDraft, string, error) {
	firstUserMessage := firstConversationForkUserMessage(source.Messages)
	if firstUserMessage == nil {
		return nil, "", nil
	}
	forkID, _ := firstUserMessage.Metadata[models.MetaKeyConversationForkID].(string)
	if forkID == "" {
		return nil, "", nil
	}
	destinations, ok := s.messages.(taskrepo.ConversationForkDestinationRepository)
	if !ok {
		return nil, "", models.ErrConversationForkSourceUnavailable
	}
	ownerID, err := s.conversationForkOwnerID(ctx, source.WorkspaceID)
	if err != nil {
		return nil, "", models.ErrConversationForkNotFound
	}
	inherited, err := destinations.GetConversationForkByDestinationSession(ctx, ownerID, source.TaskID, source.SessionID)
	if err != nil {
		return nil, "", conversationForkInheritedLookupError(err)
	}
	if !matchesConversationForkSource(inherited, source, forkID) {
		return nil, "", models.ErrConversationForkConflict
	}
	return &inherited, firstUserMessage.ID, nil
}

func firstConversationForkUserMessage(messages []*models.Message) *models.Message {
	for _, message := range messages {
		if message != nil && message.AuthorType == models.MessageAuthorUser {
			return message
		}
	}
	return nil
}

func conversationForkInheritedLookupError(err error) error {
	if errors.Is(err, models.ErrConversationForkNotFound) {
		return models.ErrConversationForkSourceUnavailable
	}
	return err
}

func matchesConversationForkSource(
	draft models.ConversationForkDraft,
	source models.ConversationForkSource,
	forkID string,
) bool {
	return draft.Descriptor.ID == forkID &&
		draft.Descriptor.State == conversationForkStateAttached &&
		draft.Descriptor.DestinationTaskID == source.TaskID &&
		draft.Descriptor.DestinationSessionID == source.SessionID &&
		draft.WorkspaceID == source.WorkspaceID &&
		draft.CompiledText != ""
}

func conversationForkCompileError(err error) error {
	switch {
	case errors.Is(err, errConversationForkInvalidRange):
		return models.ErrConversationForkCutoffUnavailable
	case errors.Is(err, errConversationForkTooLarge):
		return models.ErrConversationForkLimitExceeded
	case errors.Is(err, errConversationForkToolEvidence):
		return models.ErrConversationForkToolEvidence
	default:
		return err
	}
}

func encodeConversationForkCreateRequest(req models.ConversationForkCreateRequest) (string, []byte, error) {
	selection, err := json.Marshal(struct {
		IncludeToolEvidence bool   `json:"include_tool_evidence"`
		ModelID             string `json:"model_id,omitempty"`
	}{IncludeToolEvidence: req.IncludeToolEvidence, ModelID: req.ModelID})
	if err != nil {
		return "", nil, fmt.Errorf("encode conversation fork selection: %w", err)
	}
	fingerprintInput, err := json.Marshal(struct {
		Source              models.ConversationForkSourceRequest `json:"source"`
		IncludeToolEvidence bool                                 `json:"include_tool_evidence"`
		ModelID             string                               `json:"model_id,omitempty"`
		AttachmentIDs       []string                             `json:"attachment_ids,omitempty"`
	}{req.Source, req.IncludeToolEvidence, req.ModelID, req.AttachmentIDs})
	if err != nil {
		return "", nil, fmt.Errorf("encode conversation fork request: %w", err)
	}
	fingerprint := sha256.Sum256(fingerprintInput)
	return string(selection), fingerprint[:], nil
}

func newConversationForkDraft(
	req models.ConversationForkCreateRequest,
	compiled compiledConversationForkRequest,
	ownerID, selectionJSON string,
	fingerprint []byte,
	descriptors []models.ConversationForkAttachment,
	estimate models.ConversationForkEstimate,
) *models.ConversationForkDraft {
	now := time.Now().UTC()
	contentHash := sha256.Sum256([]byte(compiled.compiled))
	return &models.ConversationForkDraft{
		OwnerID: ownerID, WorkspaceID: compiled.source.WorkspaceID, CompiledText: compiled.compiled,
		SelectionJSON: selectionJSON, DraftRequestID: req.DraftRequestID,
		RequestFingerprint: hex.EncodeToString(fingerprint),
		Descriptor: models.ConversationForkDescriptor{
			SourceTaskID: compiled.source.TaskID, SourceSessionID: compiled.source.SessionID,
			SourceMessageID: req.Source.CutoffMessageID, StartMessageID: req.Source.StartMessageID,
			SourceTaskTitle: compiled.source.TaskTitle, SourceRevision: compiled.source.Revision,
			CompilerVersion: conversationForkCompilerVersion, ContentHash: hex.EncodeToString(contentHash[:]),
			MessageCount: compiled.count, TextBytes: len([]byte(compiled.compiled)), Omissions: compiled.omissions,
			AttachmentDescriptors: descriptors, Estimate: estimate, CreatedAt: now, ExpiresAt: now.Add(conversationForkDraftTTL),
			State: conversationForkStateDraft,
		},
	}
}

func (s *Service) persistConversationForkDraft(
	ctx context.Context,
	draft *models.ConversationForkDraft,
	copies []*models.TaskMessageAttachment,
	descriptors []models.ConversationForkAttachment,
) (models.ConversationForkDraft, error) {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		cleanupErr := deleteStagedForkAttachments(ctx, s.attachmentSvc, draft.OwnerID, copies)
		return models.ConversationForkDraft{}, errors.Join(models.ErrConversationForkSourceUnavailable, cleanupErr)
	}
	created, err := drafts.CreateConversationForkDraft(ctx, draft)
	if err != nil {
		cleanupErr := deleteStagedForkAttachments(ctx, s.attachmentSvc, draft.OwnerID, copies)
		return models.ConversationForkDraft{}, errors.Join(err, cleanupErr)
	}
	if !sameConversationForkAttachmentCopies(created.Descriptor.AttachmentDescriptors, descriptors) {
		if cleanupErr := deleteStagedForkAttachments(ctx, s.attachmentSvc, draft.OwnerID, copies); cleanupErr != nil {
			return models.ConversationForkDraft{}, cleanupErr
		}
	}
	return created, nil
}

// EstimateConversationForkDraft recalculates estimate metadata without
// changing the frozen source text or its content hash.
func (s *Service) EstimateConversationForkDraft(ctx context.Context, id, modelID string) (models.ConversationForkEstimate, error) {
	draft, err := s.GetConversationForkDraft(ctx, id)
	if err != nil {
		return models.ConversationForkEstimate{}, err
	}
	estimate, err := s.estimateConversationFork(
		ctx, draft.CompiledText, modelID, len(draft.Descriptor.AttachmentDescriptors) > 0,
	)
	if err != nil {
		return models.ConversationForkEstimate{}, err
	}
	ownerID, err := s.conversationForkOwnerID(ctx, draft.WorkspaceID)
	if err != nil {
		return models.ConversationForkEstimate{}, err
	}
	repository, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return models.ConversationForkEstimate{}, models.ErrConversationForkSourceUnavailable
	}
	if err := repository.UpdateConversationForkEstimate(ctx, ownerID, id, estimate); err != nil {
		return models.ConversationForkEstimate{}, err
	}
	return estimate, nil
}

// GetConversationForkDraft authorizes both the draft owner and the frozen
// source session before returning historical text to the client.
func (s *Service) GetConversationForkDraft(ctx context.Context, id string) (models.ConversationForkDraft, error) {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return models.ConversationForkDraft{}, models.ErrConversationForkSourceUnavailable
	}
	ownerID, err := s.conversationForkOwnerID(ctx, "")
	if err != nil {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	draft, err := drafts.GetConversationForkDraft(ctx, ownerID, id, time.Now().UTC())
	if err != nil && !errors.Is(err, models.ErrConversationForkExpired) {
		return models.ConversationForkDraft{}, err
	}
	if draft.Descriptor.State == "discarded" {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	if draft.Descriptor.State == conversationForkStateDraft {
		if authErr := s.AuthorizeSessionAccess(ctx, draft.Descriptor.SourceSessionID); authErr != nil {
			return models.ConversationForkDraft{}, authErr
		}
		return draft, err
	}
	if draft.Descriptor.State != conversationForkStateAttached || draft.Descriptor.DestinationTaskID == "" {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	if draft.Descriptor.DestinationSessionID != "" {
		if authErr := s.AuthorizeTaskSessionAccess(ctx, draft.Descriptor.DestinationTaskID, draft.Descriptor.DestinationSessionID); authErr != nil {
			return models.ConversationForkDraft{}, authErr
		}
	} else if authErr := s.AuthorizeTaskAccess(ctx, draft.Descriptor.DestinationTaskID); authErr != nil {
		return models.ConversationForkDraft{}, authErr
	}
	return draft, nil
}

// DiscardConversationForkDraft removes a preview from the caller's active
// draft quota without changing any source message.
func (s *Service) DiscardConversationForkDraft(ctx context.Context, id string) error {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return models.ErrConversationForkSourceUnavailable
	}
	ownerID, err := s.conversationForkOwnerID(ctx, "")
	if err != nil {
		return models.ErrConversationForkNotFound
	}
	draft, readErr := drafts.GetConversationForkDraft(ctx, ownerID, id, time.Now().UTC())
	if readErr != nil && !errors.Is(readErr, models.ErrConversationForkExpired) {
		return readErr
	}
	if draft.Descriptor.State == conversationForkStateDiscarded {
		return nil
	}
	if draft.Descriptor.State != conversationForkStateDraft {
		return models.ErrConversationForkConflict
	}
	if err := drafts.DiscardConversationForkDraft(ctx, ownerID, id); err != nil {
		return err
	}
	copyIDs := make([]*models.TaskMessageAttachment, 0, len(draft.Descriptor.AttachmentDescriptors))
	for _, descriptor := range draft.Descriptor.AttachmentDescriptors {
		copyIDs = append(copyIDs, &models.TaskMessageAttachment{ID: descriptor.ID})
	}
	return deleteStagedForkAttachments(ctx, s.attachmentSvc, ownerID, copyIDs)
}

func sameConversationForkAttachmentCopies(existing, proposed []models.ConversationForkAttachment) bool {
	if len(existing) != len(proposed) {
		return false
	}
	existingIDs := make(map[string]struct{}, len(existing))
	for _, descriptor := range existing {
		existingIDs[descriptor.ID] = struct{}{}
	}
	for _, descriptor := range proposed {
		if _, ok := existingIDs[descriptor.ID]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) conversationForkOwnerID(ctx context.Context, workspaceID string) (string, error) {
	if identity, ok := authn.IdentityFromContext(ctx); ok && identity.UserID != "" {
		return identity.UserID, nil
	}
	if workspaceID != "" && s.workspaces != nil {
		workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return "", err
		}
		if workspace != nil && workspace.OwnerID != "" {
			return workspace.OwnerID, nil
		}
	}
	return "", models.ErrConversationForkNotFound
}

func (s *Service) conversationForkAttachmentCandidates(ctx context.Context, attachments []*models.TaskMessageAttachment) []models.ConversationForkAttachment {
	candidates := make([]models.ConversationForkAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		candidate := models.ConversationForkAttachment{
			SourceID:     attachment.ID,
			Name:         attachment.Name,
			MediaType:    attachment.MimeType,
			Kind:         attachment.Kind,
			DeliveryMode: attachment.DeliveryMode,
			Size:         attachment.SizeBytes,
			Available:    false,
		}
		if s.attachmentSvc != nil {
			_, file, err := s.attachmentSvc.Open(ctx, attachment.OwnerID, attachment.ID)
			if err == nil {
				candidate.Available = true
				_ = file.Close()
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func estimateConversationForkTokens(text string) (int, error) {
	conversationForkCodec.Do(func() {
		conversationForkCodec.codec, conversationForkCodec.err = tokenizer.Get(tokenizer.O200kBase)
	})
	if conversationForkCodec.err != nil {
		return 0, conversationForkCodec.err
	}
	return conversationForkCodec.codec.Count(text)
}

func (s *Service) cleanupExpiredConversationForkDraftRows(ctx context.Context, now time.Time) {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return
	}
	expired, err := drafts.DeleteExpiredConversationForkDrafts(ctx, now)
	if err != nil {
		s.logger.Warn("conversation fork draft cleanup failed", zap.Error(err))
		return
	}
	for _, item := range expired {
		copies := make([]*models.TaskMessageAttachment, 0, len(item.Attachments))
		for _, descriptor := range item.Attachments {
			copies = append(copies, &models.TaskMessageAttachment{ID: descriptor.ID})
		}
		if err := deleteStagedForkAttachments(ctx, s.attachmentSvc, item.OwnerID, copies); err != nil {
			s.logger.Warn("expired conversation fork attachment cleanup failed",
				zap.String("owner_id", item.OwnerID), zap.Error(err))
		}
	}
}
