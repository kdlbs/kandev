package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// proposalReplyMaxRunes is the reply text limit, in Unicode code points after
// trimming.
const proposalReplyMaxRunes = 2000

const (
	replyDeliverTimeout  = 20 * time.Second
	replyFinaliseTimeout = 5 * time.Second
	// MetaKeyReplyProposalID marks the conversation message a reply produced.
	MetaKeyReplyProposalID = "coordinator_reply_proposal_id"
)

// replyTotal counts replies by whether the reply reached the conversation.
var replyTotal = expvar.NewMap("coordinator_reply_total")

// ReplyRequest is the body of the reply route.
type ReplyRequest struct {
	Text string `json:"text"`
}

// ReplyMessenger stores a conversation message and its queue entry at most
// once under one id.
type ReplyMessenger interface {
	CreateQueuedMessageOnce(ctx context.Context, id string, req *taskservice.CreateMessageRequest,
		queued *messagequeue.QueuedMessage, maxPerSession int) (*taskmodels.Message, bool, error)
}

// ReplyNotifier admits a queued prompt to its session.
type ReplyNotifier interface {
	MaxQueuedPromptsPerSession() int
	NotifyQueuedUserPrompt(ctx context.Context, taskID, sessionID string)
}

// SetReplyDeps wires the message and queue seams a reply is delivered through.
func (s *Service) SetReplyDeps(messenger ReplyMessenger, notifier ReplyNotifier) {
	s.replyMessenger = messenger
	s.replyNotifier = notifier
}

// replyTextFunc builds the conversation text of a reply for one proposal kind.
type replyTextFunc func(p *Proposal, title, reply string) string

// replyTexts registers the delivery text of each replyable kind; a kind
// without an entry cannot be replied to.
var replyTexts = map[string]replyTextFunc{
	ProposalKindCreateTask: func(p *Proposal, title, reply string) string {
		return fmt.Sprintf("Reply to your proposal %q (proposal %s): %s. "+
			"If you still think the work is needed, propose it again with in_reply_to set to %s.",
			title, p.ID, reply, p.ID)
	},
	ProposalKindMessage: taskKindReplyText,
	ProposalKindMove:    taskKindReplyText,
	ProposalKindResume:  taskKindReplyText,
}

func taskKindReplyText(p *Proposal, title, reply string) string {
	return fmt.Sprintf("Reply to your %s proposal %q (proposal %s): %s. "+
		"If you still think it is needed, send a new %s proposal that meets the condition.",
		p.Kind, title, p.ID, reply, p.Kind)
}

var replyTitleVerbs = map[string]string{
	ProposalKindMessage: "Message",
	ProposalKindMove:    "Move",
	ProposalKindResume:  "Resume",
}

// replyTitle is the one-line name of a proposal in its reply text.
func replyTitle(p *Proposal) string {
	if p.Kind == "" || p.Kind == ProposalKindCreateTask {
		return p.Spec.Title
	}
	var spec struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal([]byte(p.RawSpec), &spec)
	return fmt.Sprintf("%s task %s", replyTitleVerbs[p.Kind], spec.TaskID)
}

func replyKind(p *Proposal) string {
	if p.Kind == "" {
		return ProposalKindCreateTask
	}
	return p.Kind
}

// ReplyToProposal implements POST .../proposals/:pid/reply: a pending
// proposal moves to returned with the manager's condition, and the reply is
// then delivered to the coordinator's conversation. A failed delivery leaves
// the proposal returned and undelivered.
func (s *Service) ReplyToProposal(ctx context.Context, workspaceID, coordinatorID, proposalID string, req ReplyRequest) (*Proposal, error) {
	if !s.phase3 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	proposal, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if err != nil {
		return nil, err
	}
	if proposal.Status != ProposalStatusPending {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	kind := replyKind(proposal)
	if _, ok := replyTexts[kind]; !ok {
		return nil, &FieldError{Field: "kind", Message: fmt.Sprintf("a %s proposal cannot be replied to", kind)}
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, &FieldError{Field: "text", Message: "text is required"}
	}
	if utf8.RuneCountInString(text) > proposalReplyMaxRunes {
		return nil, &FieldError{Field: "text", Message: fmt.Sprintf("text must be at most %d characters", proposalReplyMaxRunes)}
	}

	decidedBy := decidingUserID(ctx)
	now := time.Now().UTC()
	row := ActivityRow{
		CoordinatorID: coordinatorID, WorkspaceID: workspaceID, ActionClass: kindAction(kind),
		Outcome: ActivityReturned, Authorization: AuthRequiresApproval,
		TargetTaskID: proposal.TargetTaskID, ProposalID: &proposalID, ActorUserID: optString(decidedBy), Detail: text,
	}
	matched, err := s.settleDecision(ctx, coordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.ReturnProposalTx(ctx, tx, proposalID, text, decidedBy, now)
	})
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.claimRaceResult(ctx, workspaceID, coordinatorID, proposalID)
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	returned := *proposal
	returned.Status, returned.ReplyText, returned.UpdatedAt = ProposalStatusReturned, &text, now
	returned.DecidedBy = &decidedBy

	_, delivered := s.deliverReply(ctx, &returned)
	replyTotal.Add(fmt.Sprint(delivered), 1)
	if !delivered {
		s.logger.Warn("reply not delivered", zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID))
	}
	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if err != nil {
		s.logger.Warn("re-read proposal after reply failed", zap.String("proposal_id", proposalID), zap.Error(err))
		return &returned, nil
	}
	return current, nil
}

// DeliverReply implements POST .../proposals/:pid/reply/deliver: it re-runs
// delivery for a returned proposal whose reply is still undelivered.
func (s *Service) DeliverReply(ctx context.Context, workspaceID, coordinatorID, proposalID string) (*Proposal, error) {
	if !s.phase3 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	proposal, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if err != nil {
		return nil, err
	}
	if proposal.Status != ProposalStatusReturned {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	if proposal.ReplyDeliveredAt != nil {
		return proposal, nil
	}
	claimed, delivered := s.deliverReply(ctx, proposal)
	if claimed {
		replyTotal.Add(fmt.Sprint(delivered), 1)
		if !delivered {
			s.logger.Warn("reply not delivered", zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID))
		}
	}
	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if err != nil {
		s.logger.Warn("re-read proposal after deliver failed", zap.String("proposal_id", proposalID), zap.Error(err))
		return proposal, nil
	}
	return current, nil
}

func replyDeliveryKey(proposalID string) string { return "coordinator-reply:" + proposalID }

// deliverReply claims the delivery, hands the reply to the coordinator's
// conversation exactly once, and records it. claimed reports whether the
// claim changed a row; delivered whether the reply is now recorded delivered.
func (s *Service) deliverReply(ctx context.Context, p *Proposal) (claimed, delivered bool) {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replyDeliverTimeout)
	defer cancel()
	claimed, err := s.store.ClaimReplyDelivery(dctx, p.ID, time.Now())
	if err != nil {
		s.logger.Warn("claim reply delivery failed", zap.String("proposal_id", p.ID), zap.Error(err))
		return false, false
	}
	if !claimed {
		return false, false
	}
	if err := s.sendReply(dctx, p); err != nil {
		s.logger.Warn("reply delivery failed", zap.String("proposal_id", p.ID), zap.Error(err))
		s.releaseReplyClaim(ctx, p.ID)
		return true, false
	}
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), replyFinaliseTimeout)
	defer fcancel()
	changed, err := s.store.FinaliseReplyDelivery(fctx, p.ID, time.Now())
	if err != nil {
		s.logger.Warn("record reply delivery failed", zap.String("proposal_id", p.ID), zap.Error(err))
		return true, false
	}
	if changed {
		s.publishCoordinatorUpdated(fctx, p.WorkspaceID, p.CoordinatorID)
	}
	return true, true
}

func (s *Service) releaseReplyClaim(ctx context.Context, proposalID string) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replyFinaliseTimeout)
	defer cancel()
	if err := s.store.ReleaseReplyClaim(rctx, proposalID); err != nil {
		s.logger.Warn("release reply claim failed", zap.String("proposal_id", proposalID), zap.Error(err))
	}
}

// sendReply stores the reply as a message from the replying manager in the
// coordinator's conversation and queues it for the coordinator's session. A
// replay stores nothing new but still notifies the session.
func (s *Service) sendReply(ctx context.Context, p *Proposal) error {
	build, ok := replyTexts[replyKind(p)]
	if !ok || p.ReplyText == nil {
		return errors.New("proposal has no deliverable reply")
	}
	if s.replyMessenger == nil || s.replyNotifier == nil {
		return errors.New("reply delivery is not wired")
	}
	conv, err := s.OpenConversation(ctx, p.WorkspaceID, p.CoordinatorID)
	if err != nil {
		return fmt.Errorf("open conversation: %w", err)
	}
	key := replyDeliveryKey(p.ID)
	content := build(p, replyTitle(p), *p.ReplyText)
	metadata := map[string]interface{}{MetaKeyReplyProposalID: p.ID}
	// The message is the replying manager's, whoever presses Send again;
	// CreateQueuedMessageOnce keeps the author as given. Only a row with no
	// recorded decider leaves it empty, and the message is then unattributed.
	author := ""
	if p.DecidedBy != nil {
		author = *p.DecidedBy
	}
	req := &taskservice.CreateMessageRequest{
		TaskSessionID: conv.SessionID, TaskID: conv.TaskID, Content: content,
		AuthorType: string(taskmodels.MessageAuthorUser), AuthorID: author, Metadata: metadata,
	}
	queueMetadata := map[string]interface{}{
		MetaKeyReplyProposalID:                          p.ID,
		"user_message_recorded":                         true,
		messagequeue.MetadataDurableTranscriptMessageID: key,
		orchestrator.MetaKeyTurnStartAlreadyProcessed:   true,
	}
	queued := &messagequeue.QueuedMessage{
		ID: key, SessionID: conv.SessionID, TaskID: conv.TaskID, Content: content,
		Metadata: queueMetadata, QueuedBy: messagequeue.QueuedByUser,
	}
	message, _, err := s.replyMessenger.CreateQueuedMessageOnce(ctx, key, req, queued, s.replyNotifier.MaxQueuedPromptsPerSession())
	if err != nil {
		return fmt.Errorf("queue reply: %w", err)
	}
	sessionID := conv.SessionID
	if message != nil && message.TaskSessionID != "" {
		sessionID = message.TaskSessionID
	}
	s.replyNotifier.NotifyQueuedUserPrompt(ctx, conv.TaskID, sessionID)
	return nil
}
