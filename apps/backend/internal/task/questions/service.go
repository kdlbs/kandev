package questions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
	maxQuestionIDs   = 50
	maxRequestIDLen  = 128
	answerTextCap    = 2000
)

// Service owns create, list, resolve, withdraw, forward, wait, and outbox
// reconciliation for async questions.
type Service struct {
	store           Store
	queue           DeliveryQueue
	notifier        Notifier
	lookup          TaskLookup
	sessions        SessionStateReader
	wakeDestination func(ctx context.Context, taskID, sessionID string)
	logger          *zap.Logger
	now             func() time.Time

	lifecycleMu sync.Mutex
	running     bool
	wake        chan struct{}
	stop        chan struct{}
	done        chan struct{}
	cancel      context.CancelFunc
}

// NewService builds the async question service. queue and notifier are
// optional until the composition root wires them.
func NewService(store Store, lookup TaskLookup, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		store:  store,
		lookup: lookup,
		logger: logger.With(zap.String("component", "async-questions")),
		now:    func() time.Time { return time.Now().UTC() },
		wake:   make(chan struct{}, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// SetDeliveryQueue wires exact-session queue admission.
func (s *Service) SetDeliveryQueue(queue DeliveryQueue) { s.queue = queue }

// SetNotifier wires workspace-scoped change publication.
func (s *Service) SetNotifier(notifier Notifier) { s.notifier = notifier }

// SetSessionStateReader wires the lazy terminal-destination check used by
// HasParkedWait, so a cancelled session cannot strand completion.
func (s *Service) SetSessionStateReader(reader SessionStateReader) { s.sessions = reader }

// SetWakeDestination wires the orchestrator readiness recheck used after a
// fenced wait release settles.
func (s *Service) SetWakeDestination(wake func(ctx context.Context, taskID, sessionID string)) {
	s.wakeDestination = wake
}

// CreateInput describes one request creation from a trusted task session.
type CreateInput struct {
	TaskID                     string
	WorkspaceID                string
	SessionID                  string
	SessionIncarnationID       string
	TurnID                     string
	Recipient                  models.AsyncQuestionRecipient
	ParentTaskID               string
	ParentSession              string
	ParentSessionIncarnationID string
	Questions                  []protocol.Question
	Context                    string
	RequestID                  string
	RetryKey                   string
}

// CreateResult is the creation receipt returned to the agent.
type CreateResult struct {
	Question *models.AsyncQuestion
	Created  bool
}

// Create persists one async request. An exact retry returns the recorded
// request; different content under the same identity is a conflict.
func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	if err := validateCreateInput(&input); err != nil {
		return nil, err
	}
	retryKey, err := createRetryKey(input)
	if err != nil {
		return nil, err
	}
	digest, err := payloadDigest(input)
	if err != nil {
		return nil, err
	}
	question := &models.AsyncQuestion{
		ID:                         uuid.NewString(),
		TaskID:                     input.TaskID,
		WorkspaceID:                input.WorkspaceID,
		SessionID:                  input.SessionID,
		SessionIncarnationID:       input.SessionIncarnationID,
		TurnID:                     input.TurnID,
		Recipient:                  input.Recipient,
		ParentTaskID:               input.ParentTaskID,
		ParentSessionID:            input.ParentSession,
		ParentSessionIncarnationID: input.ParentSessionIncarnationID,
		Questions:                  input.Questions,
		Context:                    input.Context,
		RetryKey:                   retryKey,
		PayloadDigest:              digest,
		Status:                     models.AsyncQuestionStatusPending,
		Version:                    1,
	}
	deliveries := make([]*models.AsyncQuestionDelivery, 0, 1)
	if question.Recipient == models.AsyncQuestionRecipientParent {
		deliveries = append(deliveries, s.parentNotificationDelivery(question))
	}
	created, isNew, err := s.store.CreateAsyncQuestion(ctx, question, deliveries)
	if err != nil {
		if isRetryConflict(err) {
			return nil, fmt.Errorf("%w: request identity already exists with different content", ErrConflict)
		}
		return nil, err
	}
	if isNew {
		s.notify(ctx, created, "created")
		s.signal()
	}
	return &CreateResult{Question: created, Created: isNew}, nil
}

func validateCreateInput(input *CreateInput) error {
	if input.SessionID == "" || input.TaskID == "" {
		return fmt.Errorf("%w: session and task are required", ErrValidation)
	}
	if input.WorkspaceID == "" {
		return fmt.Errorf("%w: workspace is required", ErrValidation)
	}
	if input.Recipient == "" {
		input.Recipient = models.AsyncQuestionRecipientUser
	}
	if input.Recipient != models.AsyncQuestionRecipientUser && input.Recipient != models.AsyncQuestionRecipientParent {
		return fmt.Errorf("%w: recipient is invalid", ErrValidation)
	}
	if errMsg := clarification.NormalizeAndValidateQuestions(input.Questions); errMsg != "" {
		return fmt.Errorf("%w: %s", ErrValidation, errMsg)
	}
	if input.Recipient != models.AsyncQuestionRecipientParent {
		return nil
	}
	if input.ParentTaskID == "" || input.ParentSession == "" {
		return fmt.Errorf("%w: parent recipient is incomplete", ErrValidation)
	}
	return validateParentQuestions(input.Questions)
}

func validateParentQuestions(questions []protocol.Question) error {
	for _, question := range questions {
		if len(question.Images) > 0 {
			return fmt.Errorf("%w: parent questions do not support images", ErrValidation)
		}
		for _, option := range question.Options {
			if len(option.Images) > 0 {
				return fmt.Errorf("%w: parent questions do not support images", ErrValidation)
			}
		}
	}
	return nil
}

func createRetryKey(input CreateInput) (string, error) {
	explicit := strings.TrimSpace(input.RequestID)
	if utf8.RuneCountInString(explicit) > maxRequestIDLen {
		return "", fmt.Errorf("%w: request_id is longer than %d characters", ErrValidation, maxRequestIDLen)
	}
	if input.Recipient == models.AsyncQuestionRecipientParent && explicit != "" {
		return "parent:" + explicit, nil
	}
	if explicit != "" {
		return explicit, nil
	}
	if retryKey := strings.TrimSpace(input.RetryKey); retryKey != "" {
		return retryKey, nil
	}
	return "mint:" + uuid.NewString(), nil
}

func normalizeStoreError(err error) error {
	if errors.Is(err, models.ErrAsyncQuestionStale) {
		return ErrUnauthorized
	}
	return err
}

func isRetryConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "retry identity")
}

func payloadDigest(input CreateInput) (string, error) {
	payload := struct {
		Recipient    models.AsyncQuestionRecipient
		ParentTaskID string
		Questions    []protocol.Question
		Context      string
	}{
		Recipient:    input.Recipient,
		ParentTaskID: input.ParentTaskID,
		Questions:    input.Questions,
		Context:      input.Context,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Get reads one canonical request.
func (s *Service) Get(ctx context.Context, id string) (*models.AsyncQuestion, error) {
	question, err := s.store.GetAsyncQuestion(ctx, id)
	if err != nil {
		return nil, err
	}
	if question == nil {
		return nil, ErrNotFound
	}
	return question, nil
}

// ListOwn returns the author session's requests and outcomes.
func (s *Service) ListOwn(ctx context.Context, sessionID string, statuses []models.AsyncQuestionStatus, questionIDs []string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session is required", ErrValidation)
	}
	opts, err := buildListOptions(statuses, questionIDs, limit, cursorAt, cursorID)
	if err != nil {
		return nil, err
	}
	opts.AuthorSessionID = sessionID
	return s.store.ListAsyncQuestions(ctx, opts)
}

// ListChildren returns a direct parent's child requests, with or without a
// forward. The caller must have already verified the parent relationship.
func (s *Service) ListChildren(ctx context.Context, parentTaskID string, statuses []models.AsyncQuestionStatus, questionIDs []string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	if parentTaskID == "" {
		return nil, fmt.Errorf("%w: parent task is required", ErrValidation)
	}
	opts, err := buildListOptions(statuses, questionIDs, limit, cursorAt, cursorID)
	if err != nil {
		return nil, err
	}
	opts.ParentTaskID = parentTaskID
	opts.IncludeUnforwardedParents = true
	opts.RequireCurrentParent = true
	return s.store.ListAsyncQuestions(ctx, opts)
}

// ListInbox returns the authorized operator population: user-recipient
// requests and explicitly forwarded parent requests.
func (s *Service) ListInbox(ctx context.Context, workspaceID string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	return s.ListInboxForUser(ctx, workspaceID, "", limit, cursorAt, cursorID)
}

// ListInboxForUser applies the same per-user hiding as blocking bundles and
// restricts to answerable destinations before the page bound.
func (s *Service) ListInboxForUser(ctx context.Context, workspaceID, userID string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	opts, err := buildListOptions(nil, nil, limit, cursorAt, cursorID)
	if err != nil {
		return nil, err
	}
	opts.WorkspaceID = workspaceID
	opts.OpenOnly = true
	opts.IncludeUnforwardedParents = false
	opts.RequireAnswerable = true
	if userID != "" {
		opts.Sidecar = &models.ClarificationSidecarFilter{UserID: userID, Now: s.now()}
	}
	return s.store.ListAsyncQuestions(ctx, opts)
}

// ListInboxHistory includes terminal records for operator history.
func (s *Service) ListInboxHistory(ctx context.Context, workspaceID string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	opts, err := buildListOptions([]models.AsyncQuestionStatus{
		models.AsyncQuestionStatusAnswered,
		models.AsyncQuestionStatusRejected,
		models.AsyncQuestionStatusWithdrawn,
		models.AsyncQuestionStatusStale,
	}, nil, limit, cursorAt, cursorID)
	if err != nil {
		return nil, err
	}
	opts.WorkspaceID = workspaceID
	opts.IncludeUnforwardedParents = false
	return s.store.ListAsyncQuestions(ctx, opts)
}

// CountInbox counts open operator-actionable requests with the same
// terminal-destination filter as the list read.
func (s *Service) CountInbox(ctx context.Context, workspaceID string) (int, error) {
	return s.CountInboxForUser(ctx, workspaceID, "")
}

// AsyncQuestionWorkspace resolves the workspace an async sidecar identity
// belongs to, so the Inbox can authorize hiding and restoring it.
func (s *Service) AsyncQuestionWorkspace(ctx context.Context, questionID string) (string, bool, error) {
	question, err := s.store.GetAsyncQuestion(ctx, questionID)
	if err != nil {
		return "", false, err
	}
	if question == nil {
		return "", false, nil
	}
	return question.WorkspaceID, true, nil
}

// ListInboxAsyncQuestions is the clarification Inbox's narrow async read.
func (s *Service) ListInboxAsyncQuestions(ctx context.Context, workspaceID, userID string, limit int, cursorAt, cursorID string) (*models.AsyncQuestionPage, error) {
	return s.ListInboxForUser(ctx, workspaceID, userID, limit, cursorAt, cursorID)
}

// ListHiddenInboxAsyncQuestions enumerates hidden async rows and their next
// snooze expiry for the shared hidden panel.
func (s *Service) ListHiddenInboxAsyncQuestions(ctx context.Context, workspaceID, userID string, limit int, now time.Time) ([]*models.AsyncQuestion, int, *time.Time, error) {
	opts, err := buildListOptions(nil, nil, limit, "", "")
	if err != nil {
		return nil, 0, nil, err
	}
	opts.WorkspaceID = workspaceID
	opts.OpenOnly = true
	opts.RequireAnswerable = true
	opts.IncludeUnforwardedParents = false
	opts.Sidecar = &models.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now}
	page, err := s.store.ListAsyncQuestions(ctx, opts)
	if err != nil {
		return nil, 0, nil, err
	}
	count, err := s.store.CountOpenAsyncQuestions(ctx, opts)
	if err != nil {
		return nil, 0, nil, err
	}
	next, err := s.store.NextHiddenAsyncQuestionSnooze(ctx, workspaceID, userID, now)
	if err != nil {
		return nil, 0, nil, err
	}
	return page.Questions, count, next, nil
}

// CountInboxAsyncQuestions is the clarification Inbox's narrow async count.
func (s *Service) CountInboxAsyncQuestions(ctx context.Context, workspaceID, userID string) (int, error) {
	return s.CountInboxForUser(ctx, workspaceID, userID)
}

// CountInboxForUser counts the visible actionable population for one operator.
func (s *Service) CountInboxForUser(ctx context.Context, workspaceID, userID string) (int, error) {
	opts := models.ListAsyncQuestionsOptions{
		WorkspaceID: workspaceID, OpenOnly: true, RequireAnswerable: true,
		Limit: 1, IncludeUnforwardedParents: false,
	}
	if userID != "" {
		opts.Sidecar = &models.ClarificationSidecarFilter{UserID: userID, Now: s.now()}
	}
	return s.store.CountOpenAsyncQuestions(ctx, opts)
}

func buildListOptions(statuses []models.AsyncQuestionStatus, questionIDs []string, limit int, cursorAt, cursorID string) (models.ListAsyncQuestionsOptions, error) {
	opts := models.ListAsyncQuestionsOptions{Statuses: statuses, Limit: limit}
	if opts.Limit <= 0 {
		opts.Limit = defaultPageLimit
	}
	if opts.Limit > maxPageLimit {
		return opts, fmt.Errorf("%w: limit exceeds %d", ErrValidation, maxPageLimit)
	}
	if len(questionIDs) > maxQuestionIDs {
		return opts, fmt.Errorf("%w: at most %d question ids", ErrValidation, maxQuestionIDs)
	}
	opts.QuestionIDs = questionIDs
	if cursorAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, cursorAt)
		if err != nil {
			return opts, fmt.Errorf("%w: cursor is invalid", ErrValidation)
		}
		opts.CursorCreatedAt = parsed
		opts.CursorID = cursorID
	}
	return opts, nil
}

// ResolveOperator records one operator answer or rejection. Forwarded parent
// requests resolve their canonical child row. The returned bool reports
// whether this call won.
func (s *Service) ResolveOperator(ctx context.Context, questionID string, input AnswerInput) (*models.AsyncQuestion, bool, error) {
	question, err := s.Get(ctx, questionID)
	if err != nil {
		return nil, false, err
	}
	if question.Recipient == models.AsyncQuestionRecipientParent && !question.Forwarded {
		return nil, false, ErrUnauthorized
	}
	return s.resolve(ctx, question, input, models.AsyncQuestionResponderUser, "")
}

// ResolveParent records a direct parent answer. Only the recorded direct
// parent may answer.
func (s *Service) ResolveParent(ctx context.Context, questionID, parentTaskID, parentSessionID string, input AnswerInput) (*models.AsyncQuestion, bool, error) {
	question, err := s.Get(ctx, questionID)
	if err != nil {
		return nil, false, err
	}
	if question.Recipient != models.AsyncQuestionRecipientParent ||
		question.ParentTaskID == "" || question.ParentTaskID != parentTaskID {
		return nil, false, ErrUnauthorized
	}
	if question.ParentSessionID != "" && parentSessionID != "" && question.ParentSessionID != parentSessionID {
		return nil, false, ErrUnauthorized
	}
	input.Responder = models.AsyncQuestionResponderParent
	input.ResponderID = parentTaskID
	return s.resolve(ctx, question, input, models.AsyncQuestionResponderParent, parentTaskID)
}

func (s *Service) resolve(
	ctx context.Context,
	question *models.AsyncQuestion,
	input AnswerInput,
	responderKind models.AsyncQuestionResponderKind,
	responderID string,
) (*models.AsyncQuestion, bool, error) {
	if question.Status.Terminal() {
		return question, false, nil
	}
	var outcome models.AsyncQuestionOutcome
	var err error
	if responderKind == models.AsyncQuestionResponderParent && len(input.Answers) == 0 && strings.TrimSpace(input.Text) != "" {
		if utf8.RuneCountInString(input.Text) > answerTextCap {
			return nil, false, fmt.Errorf("%w: answer text is too long", ErrValidation)
		}
		outcome = models.AsyncQuestionOutcome{Text: strings.TrimSpace(input.Text)}
	} else {
		outcome, err = buildOutcome(question.Questions, input)
		if err != nil {
			return nil, false, err
		}
	}
	outcome.ResponderKind = responderKind
	outcome.ResponderID = responderID
	outcome.ResponderTaskID = question.ParentTaskID
	if responderKind == models.AsyncQuestionResponderUser {
		outcome.ResponderTaskID = ""
	}
	outcome.RespondedAt = s.now()

	deliveries := []*models.AsyncQuestionDelivery{s.ownerOutcomeDelivery(question, outcome)}
	if question.Recipient == models.AsyncQuestionRecipientParent && question.ParentSessionID != "" {
		deliveries = append(deliveries, s.parentOutcomeDelivery(question, outcome))
	}
	updated, won, err := s.store.ResolveAsyncQuestion(ctx, question.ID, outcome, deliveries)
	if err != nil {
		return nil, false, normalizeStoreError(err)
	}
	if won {
		s.notify(ctx, updated, "updated")
		s.signal()
	}
	return updated, won, nil
}

func buildOutcome(questions []protocol.Question, input AnswerInput) (models.AsyncQuestionOutcome, error) {
	if input.Rejected {
		if len(input.Answers) > 0 {
			return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: rejected may not include answers", ErrValidation)
		}
		if utf8.RuneCountInString(input.RejectReason) > answerTextCap {
			return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: rejection reason is too long", ErrValidation)
		}
		return models.AsyncQuestionOutcome{Rejected: true, RejectReason: strings.TrimSpace(input.RejectReason)}, nil
	}
	if len(input.Answers) != len(questions) {
		return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: expected exactly %d answers", ErrValidation, len(questions))
	}
	byID := make(map[string]protocol.Question, len(questions))
	for _, question := range questions {
		byID[question.ID] = question
	}
	seen := make(map[string]bool, len(input.Answers))
	answers := make([]protocol.Answer, 0, len(input.Answers))
	for _, answer := range input.Answers {
		question, ok := byID[answer.QuestionID]
		if !ok || seen[answer.QuestionID] {
			return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: answer references an unknown or repeated question id", ErrValidation)
		}
		seen[answer.QuestionID] = true
		if len(answer.SelectedOptions) > 0 {
			valid := make(map[string]bool, len(question.Options))
			for _, option := range question.Options {
				valid[option.ID] = true
			}
			for _, optionID := range answer.SelectedOptions {
				if !valid[optionID] {
					return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: answer references an unknown option id", ErrValidation)
				}
			}
		}
		if utf8.RuneCountInString(answer.CustomText) > answerTextCap {
			return models.AsyncQuestionOutcome{}, fmt.Errorf("%w: answer text is too long", ErrValidation)
		}
		answers = append(answers, protocol.Answer{
			QuestionID:      answer.QuestionID,
			SelectedOptions: append([]string(nil), answer.SelectedOptions...),
			CustomText:      strings.TrimSpace(answer.CustomText),
		})
	}
	return models.AsyncQuestionOutcome{Answers: answers}, nil
}

// Withdraw closes a pending request without recording an answer.
func (s *Service) Withdraw(ctx context.Context, questionID, authorSessionID, reason string) (*models.AsyncQuestion, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("%w: a withdrawal reason is required", ErrValidation)
	}
	question, err := s.Get(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question.SessionID != authorSessionID {
		return nil, ErrUnauthorized
	}
	deliveries := make([]*models.AsyncQuestionDelivery, 0, 1)
	buildWake := func(wait *models.AsyncQuestionWait, version int64) *models.AsyncQuestionDelivery {
		return s.buildWaitWakeDelivery(question, wait, version)
	}
	updated, won, _, err := s.store.WithdrawAsyncQuestion(
		ctx, questionID, authorSessionID, strings.TrimSpace(reason), deliveries, buildWake)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	if won {
		s.notify(ctx, updated, "updated")
		s.signal()
	}
	return updated, nil
}

// Forward exposes a pending child request to its direct parent's operator.
// The caller verifies the parent's user-question capability.
func (s *Service) Forward(ctx context.Context, questionID, parentTaskID, parentSessionID, reason string) (*models.AsyncQuestion, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("%w: a forwarding reason is required", ErrValidation)
	}
	question, err := s.Get(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if question.Recipient != models.AsyncQuestionRecipientParent || question.ParentTaskID != parentTaskID {
		return nil, ErrUnauthorized
	}
	updated, err := s.store.ForwardAsyncQuestion(ctx, &models.AsyncQuestionForward{
		QuestionID:      questionID,
		ParentTaskID:    parentTaskID,
		ParentSessionID: parentSessionID,
		Reason:          strings.TrimSpace(reason),
		CreatedAt:       s.now(),
	})
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	s.notify(ctx, updated, "forwarded")
	return updated, nil
}

// DeleteAsyncQuestionsForTask removes task-owned question state on task delete.
func (s *Service) DeleteAsyncQuestionsForTask(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	return s.store.DeleteAsyncQuestionsForTask(ctx, taskID)
}

// MarkSessionStale closes pending requests and active waits for a session
// that can no longer accept answers.
func (s *Service) MarkSessionStale(ctx context.Context, sessionID, reason string) (int, error) {
	if sessionID == "" {
		return 0, nil
	}
	count, err := s.store.MarkStaleAsyncQuestionsForSession(ctx, sessionID, reason)
	if err != nil {
		return count, err
	}
	if _, err := s.store.CancelAsyncQuestionWaitsForSession(ctx, sessionID, reason); err != nil {
		return count, err
	}
	return count, nil
}

// CancelActiveWait cancels the session's gate when a newer prompt supersedes
// the captured turn.
func (s *Service) CancelActiveWait(ctx context.Context, sessionID, reason string) error {
	wait, err := s.store.GetActiveAsyncQuestionWait(ctx, sessionID)
	if err != nil || wait == nil {
		return err
	}
	return s.store.SetAsyncQuestionWaitStatus(ctx, wait.ID, models.AsyncQuestionWaitCancelled, reason)
}

// ActiveWait returns the session's requested or parked gate, if any.
func (s *Service) ActiveWait(ctx context.Context, sessionID string) (*models.AsyncQuestionWait, error) {
	return s.store.GetActiveAsyncQuestionWait(ctx, sessionID)
}

// HasParkedWait reports whether completion must remain withheld for an
// explicit dependency gate.
func (s *Service) HasParkedWait(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	wait, err := s.store.GetActiveAsyncQuestionWait(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to read async question wait; failing closed",
			zap.String("session_id", sessionID), zap.Error(err))
		return true
	}
	if wait == nil {
		return false
	}
	if wait.Status != models.AsyncQuestionWaitRequested &&
		wait.Status != models.AsyncQuestionWaitReleaseRequested &&
		wait.Status != models.AsyncQuestionWaitParked {
		return false
	}
	if s.sessions != nil {
		state, archived, found, stateErr := s.sessions.AsyncQuestionSessionState(ctx, sessionID)
		if stateErr != nil {
			s.logger.Warn("failed to read async question destination state; failing closed",
				zap.String("session_id", sessionID), zap.Error(stateErr))
			return true
		}
		if !found || archived || terminalSessionState(state) {
			if cancelErr := s.store.SetAsyncQuestionWaitStatus(ctx, wait.ID, models.AsyncQuestionWaitCancelled, "destination_terminal"); cancelErr != nil {
				s.logger.Warn("failed to close a stranded async question wait",
					zap.String("wait_id", wait.ID), zap.Error(cancelErr))
			}
			return false
		}
	}
	return true
}

func terminalSessionState(state string) bool {
	switch state {
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}

func (s *Service) notify(ctx context.Context, question *models.AsyncQuestion, kind string) {
	if s.notifier == nil || question == nil {
		return
	}
	s.notifier.AsyncQuestionChanged(ctx, question.WorkspaceID, kind, question.ID)
}
