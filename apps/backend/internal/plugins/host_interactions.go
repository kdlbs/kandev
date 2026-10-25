// host_interactions.go implements pluginHost's Interactions() accessor — the
// Host interaction API (ADR 0052). Reads are gated on api_read:interactions.
// The v1 response methods remain for source compatibility but are denied:
// they cannot carry the exact observed revision or a human response receipt.
// Exact Host v2 writes use the same native responder after receipt validation.
//
// The gate cannot live at the accessor (like Tasks() and Messages(), this one
// mixes reads and writes), so every method checks its own capability.
//
// Reads come from the durable request record through the task service — the
// same authority kandev's own pending-action projection uses, never a
// repository directly. Exact writes route through the first-party services
// the native UI drives (the orchestrator for permissions, the clarification
// handler for question bundles), so the agent unblocks and every surface
// converges through normal events. Dependencies arrive through narrow
// interfaces satisfied by backendapp adapters, avoiding an import cycle.
package plugins

import (
	"context"
	"fmt"
	"strings"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// interactionDataSource is the narrow slice of internal/task/service.Service
// the interaction reads need. GetInteraction resolves ANY interaction,
// terminal ones included — that is what makes a plugin's event cache
// reconcilable — and returns (nil, nil) for an unknown id.
type interactionDataSource interface {
	ListPendingInteractions(ctx context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error)
	GetInteraction(ctx context.Context, pendingID string) (*taskmodels.Interaction, error)
}

// PluginPermissionResponse is the plugins-local permission response a
// responder adapter translates into the orchestrator's resolution request.
// Kandev's permission resolution is keyed on the full (task, session, request,
// pending) identity, and every field is taken from the durable record rather
// than from the plugin.
type PluginPermissionResponse struct {
	TaskID    string
	SessionID string
	RequestID string
	PendingID string
	OptionID  string
	Cancelled bool
}

// PluginClarificationAnswer is the plugins-local answer a responder adapter
// translates into the clarification resolver's outcome.
type PluginClarificationAnswer struct {
	QuestionID      string
	SelectedOptions []string
	CustomText      string
}

// interactionResponder is the narrow write path, adapted by backendapp over
// the orchestrator and the clarification handler. Implementations MUST return
// gRPC status errors so the outcome vocabulary a plugin sees (NotFound for an
// unknown id, FailedPrecondition for an already-resolved one, InvalidArgument
// for a malformed answer set) is decided by the layer that knows the
// first-party service's failure modes rather than by string matching here.
type interactionResponder interface {
	RespondToPermission(ctx context.Context, in PluginPermissionResponse) error
	AnswerClarification(ctx context.Context, pendingID string, answers []PluginClarificationAnswer) error
	DeclineClarification(ctx context.Context, pendingID, reason string) error
}

// Interactions returns the interaction accessor. Like Tasks() and Messages()
// it mixes independently-gated reads and writes, so the capability check lives
// on each method rather than here.
func (h *pluginHost) Interactions() pluginsdk.InteractionAccessor {
	return interactionReader{host: h}
}

var _ pluginsdk.InteractionHost = (*pluginHost)(nil)

type interactionReader struct{ host *pluginHost }

func (r interactionReader) ListPending(
	ctx context.Context, filter pluginsdk.InteractionFilter, page pluginsdk.Page,
) ([]pluginsdk.Interaction, *pluginsdk.PageInfo, error) {
	if !r.host.capabilities.CanRead(resourceInteractions) {
		return nil, nil, permissionDenied(apiReadCapability(resourceInteractions))
	}
	if r.host.interactionData == nil {
		return r.host.UnimplementedHostData.Interactions().ListPending(ctx, filter, page)
	}
	if err := validateInteractionKinds(filter.Kinds); err != nil {
		return nil, nil, err
	}
	// Each session/task/kind value becomes its own SQL bind parameter; cap the
	// combined count for the same reason messageReader.List does, so a large
	// filter fails fast instead of hitting the host-parameter limit.
	if n := len(filter.SessionIDs) + len(filter.TaskIDs) + len(filter.Kinds); n > maxMessageFilterValues {
		return nil, nil, invalidArgument(fmt.Sprintf(
			"interaction filter has %d session/task/kind values, max %d", n, maxMessageFilterValues))
	}
	interactions, err := r.host.interactionData.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{
		SessionIDs: filter.SessionIDs,
		TaskIDs:    filter.TaskIDs,
		Kinds:      filter.Kinds,
	})
	if err != nil {
		return nil, nil, err
	}
	items, info := paginate(interactionsToDTOs(interactions), page)
	return items, info, nil
}

// Get returns a gRPC NotFound error (not a (nil, nil) success) for an id that
// resolves to nothing, so the in-process contract matches what a plugin
// observes over the wire.
func (r interactionReader) Get(ctx context.Context, id string) (*pluginsdk.Interaction, error) {
	if !r.host.capabilities.CanRead(resourceInteractions) {
		return nil, permissionDenied(apiReadCapability(resourceInteractions))
	}
	if r.host.interactionData == nil {
		return r.host.UnimplementedHostData.Interactions().Get(ctx, id)
	}
	interaction, err := r.host.resolveInteraction(ctx, id)
	if err != nil {
		return nil, err
	}
	dto := interactionModelToDTO(interaction)
	return &dto, nil
}

func (r interactionReader) RespondToPermission(
	ctx context.Context, in pluginsdk.PermissionResponse,
) (*pluginsdk.Interaction, error) {
	return nil, r.host.exactHumanResponseRequired()
}

func (r interactionReader) AnswerClarification(
	ctx context.Context, in pluginsdk.ClarificationResponse,
) (*pluginsdk.Interaction, error) {
	return nil, r.host.exactHumanResponseRequired()
}

func (r interactionReader) CancelClarification(
	ctx context.Context, id, reason string,
) (*pluginsdk.Interaction, error) {
	return nil, r.host.exactHumanResponseRequired()
}

func (h *pluginHost) exactHumanResponseRequired() error {
	if !h.capabilities.CanWrite(resourceInteractions) {
		return permissionDenied(apiWriteCapability(resourceInteractions))
	}
	return status.Error(codes.PermissionDenied, "exact_human_response_receipt_required")
}

// interactionWriteTarget performs the checks every write shares: the
// api_write:interactions gate, and both halves of the write path being wired
// (the responder that delivers, and the reader that resolves the target and
// its post-write state). Returns a nil responder with a nil error when the
// host is unwired, so the caller can answer through its OWN Unimplemented
// counterpart rather than borrowing an unrelated method's error.
func (h *pluginHost) interactionWriteTarget() (interactionResponder, error) {
	if !h.capabilities.CanWrite(resourceInteractions) {
		return nil, permissionDenied(apiWriteCapability(resourceInteractions))
	}
	responder := h.interactionResponder()
	if responder == nil || h.interactionData == nil {
		return nil, nil
	}
	return responder, nil
}

func (h *pluginHost) interactionResponder() interactionResponder {
	if h.interactionDeps == nil {
		return nil
	}
	return h.interactionDeps()
}

func (h *pluginHost) executionController() exactExecutionController {
	if h.executionControllerDep == nil {
		return nil
	}
	return h.executionControllerDep()
}

// answerableInteraction resolves id and enforces the terminal-once contract:
// an unknown id is NotFound, an interaction of the wrong kind or one that
// already carries a resolution is FailedPrecondition. Refusing the second
// response is the point — dispatching it would either fail deep inside the
// agent transport or, worse, answer a DIFFERENT request that reused the
// pending slot.
func (h *pluginHost) answerableInteraction(
	ctx context.Context, id string, kind taskmodels.InteractionKind,
) (*taskmodels.Interaction, error) {
	interaction, err := h.resolveInteraction(ctx, id)
	if err != nil {
		return nil, err
	}
	if interaction.Kind != kind {
		return nil, status.Errorf(codes.FailedPrecondition,
			"interaction %q is a %s request, not a %s request", id, interaction.Kind, kind)
	}
	if interaction.Status.IsTerminal() {
		return nil, status.Errorf(codes.FailedPrecondition,
			"interaction %q is already %s", id, interaction.Status)
	}
	return interaction, nil
}

func (h *pluginHost) resolveInteraction(ctx context.Context, id string) (*taskmodels.Interaction, error) {
	if id == "" {
		return nil, invalidArgument("id is required")
	}
	interaction, err := h.interactionData.GetInteraction(ctx, id)
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, interactionNotFound(id)
	}
	return interaction, nil
}

// reloadInteraction returns the interaction's post-write state. The write
// already succeeded, so a failed re-read must not turn a delivered response
// into an error the caller would retry.
//
// The fallback stamps resolved on the pre-write snapshot rather than handing
// it back untouched: the contract says every response method returns the
// interaction in its NEW terminal state, and a snapshot still reading
// "pending" invites exactly the retry of an already-delivered response that
// terminal-once exists to prevent.
func (h *pluginHost) reloadInteraction(
	ctx context.Context, before *taskmodels.Interaction, resolved taskmodels.InteractionStatus,
) (*pluginsdk.Interaction, error) {
	reloaded, err := h.interactionData.GetInteraction(ctx, before.ID)
	if err == nil && reloaded != nil {
		dto := interactionModelToDTO(reloaded)
		return &dto, nil
	}
	fallback := *before
	fallback.Status = resolved
	dto := interactionModelToDTO(&fallback)
	return &dto, nil
}

// resolvePermissionChoice validates the plugin's choice against the options
// the agent actually offered and derives the approve/deny outcome from the
// chosen option's kind. A plugin therefore cannot report an outcome that was
// never on the menu, and kandev never has to trust a plugin-supplied verdict.
func resolvePermissionChoice(
	interaction *taskmodels.Interaction, in pluginsdk.PermissionResponse,
) (string, bool, error) {
	if in.Cancelled {
		if in.OptionID != "" {
			return "", false, invalidArgument("option_id must be empty when cancelled is set")
		}
		// A dismissal is not an explicit deny. The orchestrator already
		// records cancelled responses as rejected; passing rejected too would
		// conflate the two outcomes the message status deliberately splits on.
		return "", false, nil
	}
	if in.OptionID == "" {
		return "", false, invalidArgument("option_id is required when cancelled is not set")
	}
	for _, option := range interaction.Options {
		if option.ID != in.OptionID {
			continue
		}
		return option.ID, strings.HasPrefix(option.Kind, permissionRejectKindPrefix), nil
	}
	return "", false, invalidArgument(fmt.Sprintf(
		"option_id %q is not one of interaction %q's options", in.OptionID, interaction.ID))
}

// permissionRejectKindPrefix matches ACP's reject_once / reject_always option
// kinds. Prefix-matching rather than an exact set keeps a future reject_* kind
// denying by default instead of silently approving.
const permissionRejectKindPrefix = "reject"

func validateInteractionKinds(kinds []string) error {
	for _, kind := range kinds {
		switch taskmodels.InteractionKind(kind) {
		case taskmodels.InteractionKindPermission, taskmodels.InteractionKindClarification:
		default:
			return invalidArgument(fmt.Sprintf("unknown interaction kind %q", kind))
		}
	}
	return nil
}

func interactionNotFound(id string) error {
	return status.Errorf(codes.NotFound, "interaction %q not found", id)
}

// ── Denied reader ───────────────────────────────────────────────────────────
//
// Unlike the read-only resources there is no denied stub here: reads and
// writes gate independently on the same accessor, so each method returns its
// own denial naming the capability it needed.

func interactionsToDTOs(interactions []*taskmodels.Interaction) []pluginsdk.Interaction {
	out := make([]pluginsdk.Interaction, 0, len(interactions))
	for _, interaction := range interactions {
		if interaction == nil {
			continue
		}
		out = append(out, interactionModelToDTO(interaction))
	}
	return out
}
