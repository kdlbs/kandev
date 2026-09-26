// interactions.go is the Go-native side of the Host interaction API (ADR
// 0052): the durable record of every agent request still owed a human answer,
// plus the capability-gated writes that answer them.
//
// It exists because session state cannot answer "does a human owe this session
// something?". WAITING_FOR_INPUT also describes an ordinarily completed turn,
// so a state-only plugin reports attention nobody owes. Events alone cannot
// answer it either — a plugin may start after the request, restart, or drop an
// event. Only the durable record can, which is what ListPending returns and
// what Get resolves for any id, terminal ones included.
//
// Like ExecutorProfileHost and PluginOwnedTaskTreeHost, this is an OPTIONAL
// Host extension rather than a method on Host itself, so every existing
// Go-native Host implementation stays source-compatible.
package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Interaction kind values. A plugin should compare against these rather than
// bare strings; new kinds are additive.
const (
	InteractionKindPermission    = "permission"
	InteractionKindClarification = "clarification"
)

// Interaction status values. Anything other than InteractionStatusPending is
// terminal: the interaction can no longer be answered, and a write against it
// returns a gRPC FailedPrecondition.
const (
	InteractionStatusPending   = "pending"
	InteractionStatusApproved  = "approved"
	InteractionStatusRejected  = "rejected"
	InteractionStatusAnswered  = "answered"
	InteractionStatusCancelled = "cancelled"
	InteractionStatusExpired   = "expired"
)

// InteractionOption is one selectable response. Kind carries the ACP
// permission-option kind on a permission ("allow_once", "allow_always",
// "reject_once", "reject_always") and is empty on a clarification option;
// Description is the reverse.
type InteractionOption struct {
	OptionID    string
	Label       string
	Kind        string
	Description string
}

// InteractionQuestion is one question of a clarification bundle. Every
// question must be answered before the agent unblocks.
type InteractionQuestion struct {
	ID      string
	Title   string
	Prompt  string
	Options []InteractionOption
}

// Interaction is one thing a human still owes an answer to. ID is the pending
// id every response method keys on — the same id the permission and
// clarification bus events carry, so an event-driven cache can reconcile
// against a ListPending snapshot without inventing a correlation key.
//
// AgentDisconnected marks a still-pending clarification whose original waiter
// has gone away. It remains answerable (the answer resumes the session in a
// new turn), so it is NOT a resolution — treating it as one is exactly the
// false negative the durable record exists to prevent.
type Interaction struct {
	ID                string
	Kind              string // InteractionKind* constant
	TaskID            string
	SessionID         string
	TurnID            string
	Status            string // InteractionStatus* constant
	Title             string
	Context           string
	ToolCallID        string
	ActionType        string
	Options           []InteractionOption   // permission options
	Questions         []InteractionQuestion // clarification questions
	AgentDisconnected bool
	CreatedAt         string // RFC3339
	UpdatedAt         string // RFC3339
}

// InteractionFilter narrows ListPending. SessionIDs and TaskIDs are ORed
// within each axis and ANDed across the two; an empty axis is unconstrained.
// Kinds narrows by interaction kind; empty means both.
type InteractionFilter struct {
	SessionIDs []string
	TaskIDs    []string
	Kinds      []string
}

// PermissionResponse names the option a user chose for a permission request.
// kandev derives the approve/deny outcome from that option's recorded kind, so
// a plugin cannot report an outcome the agent never offered. Set Cancelled to
// dismiss the request instead of choosing (OptionID must then be empty).
type PermissionResponse struct {
	InteractionID string
	OptionID      string
	Cancelled     bool
}

// ClarificationAnswer is a user's answer to one question of a bundle.
type ClarificationAnswer struct {
	QuestionID      string
	SelectedOptions []string
	CustomText      string
}

// ClarificationResponse answers a whole clarification bundle. Answers must
// contain exactly one entry per question in the interaction.
type ClarificationResponse struct {
	InteractionID string
	Answers       []ClarificationAnswer
}

// InteractionHost is the optional Host extension exposing Interactions().
type InteractionHost interface {
	Interactions() InteractionAccessor
}

// InteractionAccessor is the accessor behind Host.Interactions(). Reads
// (ListPending/Get) require api_read:interactions; writes require
// api_write:interactions. The two gate independently, so an attention or inbox
// plugin can declare read-only.
//
// Every write is terminal-once: the first response wins, and a later attempt
// against an already-resolved interaction returns gRPC FailedPrecondition
// rather than dispatching a second response to the agent. An unknown id
// returns NotFound. Those two codes are what let a reconciling cache tell
// "someone else answered" apart from "my id is stale".
type InteractionAccessor interface {
	// ListPending returns every interaction still owed a response, under the
	// same turn/session authority kandev's own list surfaces use.
	ListPending(ctx context.Context, filter InteractionFilter, page Page) ([]Interaction, *PageInfo, error)

	// Get resolves one interaction by pending id whatever its status,
	// terminal ones included, so a replayed or missed event converges on the
	// current durable result instead of disappearing.
	Get(ctx context.Context, id string) (*Interaction, error)

	// RespondToPermission answers a permission request through kandev's
	// orchestrator — the same path the native UI uses, so the agent unblocks
	// and every surface converges. Returns the interaction's new terminal state.
	RespondToPermission(ctx context.Context, in PermissionResponse) (*Interaction, error)

	// AnswerClarification answers every question of a clarification bundle.
	AnswerClarification(ctx context.Context, in ClarificationResponse) (*Interaction, error)

	// CancelClarification declines a clarification bundle on the user's
	// behalf, surfacing reason to the agent. Works whether or not the original
	// waiter is still parked.
	CancelClarification(ctx context.Context, id, reason string) (*Interaction, error)
}

// Interactions returns the optional interaction accessor, and false when the
// host predates the extension.
func Interactions(host Host) (InteractionAccessor, bool) {
	provider, ok := host.(InteractionHost)
	if !ok {
		return nil, false
	}
	return provider.Interactions(), true
}

// ── proto conversion ────────────────────────────────────────────────────────

func (o InteractionOption) toProto() *pluginv1.InteractionOption {
	return &pluginv1.InteractionOption{
		OptionId: o.OptionID, Label: o.Label, Kind: o.Kind, Description: o.Description,
	}
}

func interactionOptionFromProto(p *pluginv1.InteractionOption) InteractionOption {
	if p == nil {
		return InteractionOption{}
	}
	return InteractionOption{
		OptionID: p.GetOptionId(), Label: p.GetLabel(), Kind: p.GetKind(), Description: p.GetDescription(),
	}
}

func interactionOptionsToProto(items []InteractionOption) []*pluginv1.InteractionOption {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.InteractionOption, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

func interactionOptionsFromProto(items []*pluginv1.InteractionOption) []InteractionOption {
	if len(items) == 0 {
		return nil
	}
	out := make([]InteractionOption, len(items))
	for i, item := range items {
		out[i] = interactionOptionFromProto(item)
	}
	return out
}

func (q InteractionQuestion) toProto() *pluginv1.InteractionQuestion {
	return &pluginv1.InteractionQuestion{
		Id: q.ID, Title: q.Title, Prompt: q.Prompt, Options: interactionOptionsToProto(q.Options),
	}
}

func interactionQuestionFromProto(p *pluginv1.InteractionQuestion) InteractionQuestion {
	if p == nil {
		return InteractionQuestion{}
	}
	return InteractionQuestion{
		ID:      p.GetId(),
		Title:   p.GetTitle(),
		Prompt:  p.GetPrompt(),
		Options: interactionOptionsFromProto(p.GetOptions()),
	}
}

func (i Interaction) toProto() *pluginv1.Interaction {
	questions := make([]*pluginv1.InteractionQuestion, 0, len(i.Questions))
	for _, question := range i.Questions {
		questions = append(questions, question.toProto())
	}
	if len(questions) == 0 {
		questions = nil
	}
	return &pluginv1.Interaction{
		Id:                i.ID,
		Kind:              i.Kind,
		TaskId:            i.TaskID,
		SessionId:         i.SessionID,
		TurnId:            i.TurnID,
		Status:            i.Status,
		Title:             i.Title,
		Context:           i.Context,
		ToolCallId:        i.ToolCallID,
		ActionType:        i.ActionType,
		Options:           interactionOptionsToProto(i.Options),
		Questions:         questions,
		AgentDisconnected: i.AgentDisconnected,
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         i.UpdatedAt,
	}
}

func interactionFromProto(p *pluginv1.Interaction) Interaction {
	if p == nil {
		return Interaction{}
	}
	protoQuestions := p.GetQuestions()
	var questions []InteractionQuestion
	if len(protoQuestions) > 0 {
		questions = make([]InteractionQuestion, len(protoQuestions))
		for i, question := range protoQuestions {
			questions[i] = interactionQuestionFromProto(question)
		}
	}
	return Interaction{
		ID:                p.GetId(),
		Kind:              p.GetKind(),
		TaskID:            p.GetTaskId(),
		SessionID:         p.GetSessionId(),
		TurnID:            p.GetTurnId(),
		Status:            p.GetStatus(),
		Title:             p.GetTitle(),
		Context:           p.GetContext(),
		ToolCallID:        p.GetToolCallId(),
		ActionType:        p.GetActionType(),
		Options:           interactionOptionsFromProto(p.GetOptions()),
		Questions:         questions,
		AgentDisconnected: p.GetAgentDisconnected(),
		CreatedAt:         p.GetCreatedAt(),
		UpdatedAt:         p.GetUpdatedAt(),
	}
}

func interactionsToProto(items []Interaction) []*pluginv1.Interaction {
	out := make([]*pluginv1.Interaction, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

func interactionsFromProto(items []*pluginv1.Interaction) []Interaction {
	out := make([]Interaction, len(items))
	for i, item := range items {
		out[i] = interactionFromProto(item)
	}
	return out
}

func (f InteractionFilter) toProto() *pluginv1.InteractionFilter {
	return &pluginv1.InteractionFilter{
		SessionIds: f.SessionIDs, TaskIds: f.TaskIDs, Kinds: f.Kinds,
	}
}

func interactionFilterFromProto(p *pluginv1.InteractionFilter) InteractionFilter {
	if p == nil {
		return InteractionFilter{}
	}
	return InteractionFilter{
		SessionIDs: p.GetSessionIds(), TaskIDs: p.GetTaskIds(), Kinds: p.GetKinds(),
	}
}

func clarificationAnswersToProto(items []ClarificationAnswer) []*pluginv1.ClarificationAnswer {
	out := make([]*pluginv1.ClarificationAnswer, len(items))
	for i, item := range items {
		out[i] = &pluginv1.ClarificationAnswer{
			QuestionId:      item.QuestionID,
			SelectedOptions: item.SelectedOptions,
			CustomText:      item.CustomText,
		}
	}
	return out
}

func clarificationAnswersFromProto(items []*pluginv1.ClarificationAnswer) []ClarificationAnswer {
	out := make([]ClarificationAnswer, len(items))
	for i, item := range items {
		out[i] = ClarificationAnswer{
			QuestionID:      item.GetQuestionId(),
			SelectedOptions: item.GetSelectedOptions(),
			CustomText:      item.GetCustomText(),
		}
	}
	return out
}

// ── plugin-side client ──────────────────────────────────────────────────────

type grpcInteractionAccessor struct {
	client pluginv1.HostClient
}

func (a grpcInteractionAccessor) ListPending(
	ctx context.Context, filter InteractionFilter, page Page,
) ([]Interaction, *PageInfo, error) {
	resp, err := a.client.ListPendingInteractions(ctx, &pluginv1.ListPendingInteractionsRequest{
		Filter: filter.toProto(), Page: page.toProto(),
	})
	if err != nil {
		return nil, nil, err
	}
	return interactionsFromProto(resp.GetInteractions()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (a grpcInteractionAccessor) Get(ctx context.Context, id string) (*Interaction, error) {
	resp, err := a.client.GetInteraction(ctx, &pluginv1.GetInteractionRequest{Id: id})
	if err != nil {
		return nil, err
	}
	interaction := interactionFromProto(resp.GetInteraction())
	return &interaction, nil
}

func (a grpcInteractionAccessor) RespondToPermission(
	ctx context.Context, in PermissionResponse,
) (*Interaction, error) {
	resp, err := a.client.RespondToPermission(ctx, &pluginv1.RespondToPermissionRequest{
		Id: in.InteractionID, OptionId: in.OptionID, Cancelled: in.Cancelled,
	})
	if err != nil {
		return nil, err
	}
	interaction := interactionFromProto(resp.GetInteraction())
	return &interaction, nil
}

func (a grpcInteractionAccessor) AnswerClarification(
	ctx context.Context, in ClarificationResponse,
) (*Interaction, error) {
	resp, err := a.client.AnswerClarification(ctx, &pluginv1.AnswerClarificationRequest{
		Id: in.InteractionID, Answers: clarificationAnswersToProto(in.Answers),
	})
	if err != nil {
		return nil, err
	}
	interaction := interactionFromProto(resp.GetInteraction())
	return &interaction, nil
}

func (a grpcInteractionAccessor) CancelClarification(
	ctx context.Context, id, reason string,
) (*Interaction, error) {
	resp, err := a.client.CancelClarification(ctx, &pluginv1.CancelClarificationRequest{
		Id: id, Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	interaction := interactionFromProto(resp.GetInteraction())
	return &interaction, nil
}

// ── kandev-side server ──────────────────────────────────────────────────────

// interactionAccessor resolves the impl's optional interaction extension,
// returning the same Unimplemented error the other optional extensions use
// when the host predates it.
func (s *grpcHostServer) interactionAccessor() (InteractionAccessor, error) {
	provider, ok := s.impl.(InteractionHost)
	if !ok {
		return nil, errUnimplementedHostData(resourceNameInteractions)
	}
	return provider.Interactions(), nil
}

// resourceNameInteractions is the resource label in the Unimplemented error;
// it mirrors the api_read:interactions / api_write:interactions capability
// names the kandev-side host gates on.
const resourceNameInteractions = "interactions"

func (s *grpcHostServer) ListPendingInteractions(
	ctx context.Context, req *pluginv1.ListPendingInteractionsRequest,
) (*pluginv1.ListPendingInteractionsResponse, error) {
	accessor, err := s.interactionAccessor()
	if err != nil {
		return nil, err
	}
	interactions, pageInfo, err := accessor.ListPending(
		ctx, interactionFilterFromProto(req.GetFilter()), pageFromProto(req.GetPage()),
	)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListPendingInteractionsResponse{
		Interactions: interactionsToProto(interactions), PageInfo: pageInfo.toProto(),
	}, nil
}

func (s *grpcHostServer) GetInteraction(
	ctx context.Context, req *pluginv1.GetInteractionRequest,
) (*pluginv1.GetInteractionResponse, error) {
	accessor, err := s.interactionAccessor()
	if err != nil {
		return nil, err
	}
	interaction, err := accessor.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, interactionNotFound(req.GetId())
	}
	return &pluginv1.GetInteractionResponse{Interaction: interaction.toProto()}, nil
}

func (s *grpcHostServer) RespondToPermission(
	ctx context.Context, req *pluginv1.RespondToPermissionRequest,
) (*pluginv1.RespondToPermissionResponse, error) {
	accessor, err := s.interactionAccessor()
	if err != nil {
		return nil, err
	}
	interaction, err := accessor.RespondToPermission(ctx, PermissionResponse{
		InteractionID: req.GetId(), OptionID: req.GetOptionId(), Cancelled: req.GetCancelled(),
	})
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, interactionNotFound(req.GetId())
	}
	return &pluginv1.RespondToPermissionResponse{Interaction: interaction.toProto()}, nil
}

func (s *grpcHostServer) AnswerClarification(
	ctx context.Context, req *pluginv1.AnswerClarificationRequest,
) (*pluginv1.AnswerClarificationResponse, error) {
	accessor, err := s.interactionAccessor()
	if err != nil {
		return nil, err
	}
	interaction, err := accessor.AnswerClarification(ctx, ClarificationResponse{
		InteractionID: req.GetId(), Answers: clarificationAnswersFromProto(req.GetAnswers()),
	})
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, interactionNotFound(req.GetId())
	}
	return &pluginv1.AnswerClarificationResponse{Interaction: interaction.toProto()}, nil
}

func (s *grpcHostServer) CancelClarification(
	ctx context.Context, req *pluginv1.CancelClarificationRequest,
) (*pluginv1.CancelClarificationResponse, error) {
	accessor, err := s.interactionAccessor()
	if err != nil {
		return nil, err
	}
	interaction, err := accessor.CancelClarification(ctx, req.GetId(), req.GetReason())
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, interactionNotFound(req.GetId())
	}
	return &pluginv1.CancelClarificationResponse{Interaction: interaction.toProto()}, nil
}

// ── unimplemented default ───────────────────────────────────────────────────

// Interactions is the embeddable default on UnimplementedHostData, so a Host
// that has not wired the interaction API still satisfies InteractionHost and
// answers Unimplemented rather than panicking.
func (UnimplementedHostData) Interactions() InteractionAccessor {
	return unimplementedInteractionAccessor{}
}

type unimplementedInteractionAccessor struct{}

func (unimplementedInteractionAccessor) ListPending(
	context.Context, InteractionFilter, Page,
) ([]Interaction, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData(resourceNameInteractions)
}

func (unimplementedInteractionAccessor) Get(context.Context, string) (*Interaction, error) {
	return nil, errUnimplementedHostData(resourceNameInteractions)
}

func (unimplementedInteractionAccessor) RespondToPermission(
	context.Context, PermissionResponse,
) (*Interaction, error) {
	return nil, errUnimplementedHostData(resourceNameInteractions)
}

func (unimplementedInteractionAccessor) AnswerClarification(
	context.Context, ClarificationResponse,
) (*Interaction, error) {
	return nil, errUnimplementedHostData(resourceNameInteractions)
}

func (unimplementedInteractionAccessor) CancelClarification(
	context.Context, string, string,
) (*Interaction, error) {
	return nil, errUnimplementedHostData(resourceNameInteractions)
}

// grpcHostClient satisfies the optional extension so a plugin reaches the
// Interactions accessor through the same pluginsdk.Interactions(host) helper
// kandev-side callers use.
func (h *grpcHostClient) Interactions() InteractionAccessor {
	return grpcInteractionAccessor{client: h.client}
}

var _ InteractionHost = (*grpcHostClient)(nil)

// interactionNotFound is the error every interaction RPC returns for an id
// that resolves to nothing. Defense-in-depth for the trust boundary: the
// in-tree host never returns (nil, nil) on success, but a custom or test host
// might, and a plugin should get NotFound rather than a zero-value DTO that
// reads as a real, pending interaction.
func interactionNotFound(id string) error {
	return status.Errorf(codes.NotFound, "interaction %q not found", id)
}
