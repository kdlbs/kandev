package plugins

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

const (
	resourceHumanResponses       = "human_responses"
	humanResponsesCapability     = "host.v2.write:human_responses"
	requestHumanResponseMethod   = "RequestHumanResponseReceipt"
	identityLinkCodeTTL          = 10 * time.Minute
	identityLinkCodeLimit        = 10000
	identityLinkCodeAlphabet     = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	identityLinkCodeLength       = 12
	maxIdentityProviderLength    = 64
	maxIdentitySubjectLength     = 256
	maxIdentityDisplayNameLength = 256
)

// Typed denial reasons a plugin branches on.
const (
	reasonHumanResponseReceiptInvalid = "human_response_receipt_invalid_or_used"
	reasonExternalIdentityNotLinked   = "external_identity_not_linked"
	reasonLinkedUserNotAuthorized     = "linked_user_not_authorized"
	reasonInteractionNotFound         = "interaction_not_found"
)

var (
	ErrIdentityLinkUnavailable = errors.New("identity linking is unavailable")
	ErrIdentityLinkNotDeclared = errors.New("plugin does not declare human responses")
	ErrIdentityLinkNotFound    = errors.New("identity link was not found")
	ErrIdentityLinkForbidden   = errors.New("identity link belongs to another user")
)

// LinkedHuman is an active Kandev user that an identity link resolves to.
type LinkedHuman struct {
	Identity    authn.Identity
	DisplayName string
}

// SetLinkedUserResolver wires the lookup of an active Kandev user by ID. A
// user that is missing, inactive, or in an unusable organization resolves to
// false, which denies receipts for every link of that user.
func (s *Service) SetLinkedUserResolver(resolver func(context.Context, string) (LinkedHuman, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linkedUserResolver = resolver
}

func (s *Service) linkedUserResolverDep() func(context.Context, string) (LinkedHuman, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linkedUserResolver
}

// IdentityLinkDTO is one link as shown on the plugin settings page.
type IdentityLinkDTO struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	Subject         string `json:"subject"`
	DisplayName     string `json:"display_name"`
	UserID          string `json:"user_id"`
	UserDisplayName string `json:"user_display_name"`
	CreatedAt       string `json:"created_at"`
}

type identityLinkCode struct {
	userID, installationID string
	expiresAt              time.Time
}

// identityLinkCodeStore keeps single-use link codes by digest. A restart
// drops every unredeemed code.
type identityLinkCodeStore struct {
	mu      sync.Mutex
	entries map[string]identityLinkCode
	now     func() time.Time
}

func newIdentityLinkCodeStore() *identityLinkCodeStore {
	return &identityLinkCodeStore{entries: make(map[string]identityLinkCode), now: time.Now}
}

func identityLinkCodeDigest(code string) string {
	normalized := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	digest := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(digest[:])
}

// issue replaces any earlier code of the same user and installation.
func (s *identityLinkCodeStore) issue(userID, installationID string) (string, time.Time, error) {
	raw := make([]byte, identityLinkCodeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	var code strings.Builder
	for i, b := range raw {
		if i > 0 && i%4 == 0 {
			code.WriteByte('-')
		}
		code.WriteByte(identityLinkCodeAlphabet[int(b)%len(identityLinkCodeAlphabet)])
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	for digest, entry := range s.entries {
		if !entry.expiresAt.After(now) || (entry.userID == userID && entry.installationID == installationID) {
			delete(s.entries, digest)
		}
	}
	if len(s.entries) >= identityLinkCodeLimit {
		return "", time.Time{}, ErrIdentityLinkUnavailable
	}
	expires := now.Add(identityLinkCodeTTL)
	s.entries[identityLinkCodeDigest(code.String())] = identityLinkCode{userID: userID, installationID: installationID, expiresAt: expires}
	return code.String(), expires, nil
}

// redeem consumes a code for installationID. Any presented code is removed,
// so a code shown to the wrong installation cannot be retried.
func (s *identityLinkCodeStore) redeem(code, installationID string) (string, bool) {
	digest := identityLinkCodeDigest(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.entries[digest]
	if !found {
		return "", false
	}
	delete(s.entries, digest)
	if !entry.expiresAt.After(s.now()) || entry.installationID != installationID {
		return "", false
	}
	return entry.userID, true
}

// identityLinkInstallation resolves the installation of an installed plugin
// that declares api_write: human_responses.
func (s *Service) identityLinkInstallation(pluginID string) (string, error) {
	record, err := s.Get(pluginID)
	if err != nil {
		return "", err
	}
	if !record.Capabilities.CanWrite(resourceHumanResponses) {
		return "", ErrIdentityLinkNotDeclared
	}
	return record.InstallationID, nil
}

// CreateIdentityLinkCode issues a ten-minute, single-use code that links the
// account which presents it, through this plugin, to userID.
func (s *Service) CreateIdentityLinkCode(pluginID, userID string) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, ErrIdentityLinkForbidden
	}
	installationID, err := s.identityLinkInstallation(pluginID)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.identityLinkCodes.issue(userID, installationID)
}

// ListIdentityLinks returns the caller's links, or every link of the
// installation when all is set by an instance admin.
func (s *Service) ListIdentityLinks(ctx context.Context, pluginID, userID string, all bool) ([]IdentityLinkDTO, error) {
	installationID, err := s.identityLinkInstallation(pluginID)
	if err != nil {
		return nil, err
	}
	commandStore := s.exactCommandStoreDep()
	if commandStore == nil {
		return nil, ErrIdentityLinkUnavailable
	}
	filter := userID
	if all {
		filter = ""
	}
	links, err := commandStore.ListIdentityLinks(ctx, installationID, filter)
	if err != nil {
		return nil, err
	}
	resolver := s.linkedUserResolverDep()
	out := make([]IdentityLinkDTO, 0, len(links))
	for _, link := range links {
		dto := IdentityLinkDTO{
			ID: link.ID, Provider: link.Provider, Subject: link.Subject, DisplayName: link.DisplayName,
			UserID: link.UserID, CreatedAt: link.CreatedAt,
		}
		if resolver != nil {
			if human, ok := resolver(ctx, link.UserID); ok {
				dto.UserDisplayName = human.DisplayName
			}
		}
		out = append(out, dto)
	}
	return out, nil
}

// RevokeIdentityLink removes one link. A user can revoke their own links; an
// instance admin can revoke any link of the installation.
func (s *Service) RevokeIdentityLink(ctx context.Context, pluginID, linkID, userID string, admin bool) error {
	installationID, err := s.identityLinkInstallation(pluginID)
	if err != nil {
		return err
	}
	commandStore := s.exactCommandStoreDep()
	if commandStore == nil {
		return ErrIdentityLinkUnavailable
	}
	link, err := commandStore.GetIdentityLink(ctx, installationID, linkID)
	if errors.Is(err, state.ErrIdentityLinkNotFound) {
		return ErrIdentityLinkNotFound
	}
	if err != nil {
		return err
	}
	if !admin && link.UserID != userID {
		return ErrIdentityLinkNotFound
	}
	err = commandStore.DeleteIdentityLink(ctx, installationID, linkID)
	if errors.Is(err, state.ErrIdentityLinkNotFound) {
		return ErrIdentityLinkNotFound
	}
	return err
}

// deleteInstallationIdentityLinks removes an uninstalled plugin's links. The
// delegated response audit remains.
func (s *Service) deleteInstallationIdentityLinks(ctx context.Context, installationID string) error {
	commandStore := s.exactCommandStoreDep()
	if installationID == "" || commandStore == nil {
		return nil
	}
	return commandStore.DeleteInstallationIdentityLinks(ctx, installationID)
}

type humanResponseDelegationManager struct{ host *pluginHost }

// HumanResponseDelegation returns the linked-account receipt extension.
func (h *pluginHost) HumanResponseDelegation() pluginsdk.HumanResponseDelegationManager {
	return humanResponseDelegationManager{host: h}
}

var _ pluginsdk.HumanResponseDelegationHost = (*pluginHost)(nil)

func validIdentityLinkRequest(in pluginsdk.ExternalIdentityLinkRequest) bool {
	return isBoundedApprovalIdentifier(in.RequestID) && strings.TrimSpace(in.Code) != "" && len(in.Code) <= 64 &&
		isExactCapabilityResource(in.Provider) && len(in.Provider) <= maxIdentityProviderLength &&
		isBoundedApprovalIdentifier(in.Subject) && len(in.Subject) <= maxIdentitySubjectLength &&
		len(in.DisplayName) <= maxIdentityDisplayNameLength && !strings.ContainsRune(in.DisplayName, '\x00')
}

// LinkExternalIdentity redeems a Kandev-issued link code for the calling
// installation and links the presenting external account to the code's user.
func (m humanResponseDelegationManager) LinkExternalIdentity(ctx context.Context, in pluginsdk.ExternalIdentityLinkRequest) (*pluginsdk.CommandResult, pluginsdk.ExternalIdentityLink, error) {
	h := m.host
	if !h.capabilities.CanWrite(resourceHumanResponses) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUndeclaredCapability)}, pluginsdk.ExternalIdentityLink{}, nil
	}
	if !validIdentityLinkRequest(in) {
		return exactInvalidResult(), pluginsdk.ExternalIdentityLink{}, nil
	}
	if h.service == nil || h.installationID == "" || h.service.installedRecordByInstallationID(h.installationID) == nil {
		return exactUnavailableResult("authorization_unavailable"), pluginsdk.ExternalIdentityLink{}, nil
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return exactUnavailableResult(exactReasonCommandStoreUnavailable), pluginsdk.ExternalIdentityLink{}, nil
	}
	if denied := h.service.requireHumanResponsesGrant(h.installationID); denied != nil {
		return denied, pluginsdk.ExternalIdentityLink{}, nil
	}
	userID, ok := h.service.identityLinkCodes.redeem(in.Code, h.installationID)
	if !ok {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "link_code_invalid_or_used"}, pluginsdk.ExternalIdentityLink{}, nil
	}
	link, err := commandStore.LinkIdentity(ctx, state.IdentityLink{
		InstallationID: h.installationID, Provider: in.Provider, Subject: in.Subject,
		DisplayName: strings.TrimSpace(in.DisplayName), UserID: userID,
	})
	if err != nil {
		return exactUnavailableResult("identity_link_unavailable"), pluginsdk.ExternalIdentityLink{}, nil
	}
	out := pluginsdk.ExternalIdentityLink{
		ID: link.ID, Provider: link.Provider, Subject: link.Subject, DisplayName: link.DisplayName,
		UserID: link.UserID, CreatedAt: link.CreatedAt,
	}
	if resolver := h.service.linkedUserResolverDep(); resolver != nil {
		if human, found := resolver(ctx, link.UserID); found {
			out.UserDisplayName = human.DisplayName
		}
	}
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied}, out, nil
}

func validHumanResponseReceiptRequest(in pluginsdk.HumanResponseReceiptRequest) bool {
	return validHumanResponseReceiptEnvelope(in) && validHumanResponseReceiptPayload(in)
}

func validHumanResponseReceiptEnvelope(in pluginsdk.HumanResponseReceiptRequest) bool {
	return isBoundedApprovalIdentifier(in.RequestID) && isBoundedApprovalIdentifier(in.WorkspaceID) &&
		isExactCapabilityResource(in.Provider) && isBoundedApprovalIdentifier(in.Subject) &&
		isBoundedApprovalIdentifier(in.TargetID) && in.ExpectedResourceVersion != "" &&
		in.ApprovalRevision > 0 && len(in.ManifestDigest) == 64
}

// validHumanResponseReceiptPayload requires exactly the payload shape of the
// exact command the receipt authorizes.
func validHumanResponseReceiptPayload(in pluginsdk.HumanResponseReceiptRequest) bool {
	switch in.Kind {
	case pluginsdk.HumanResponseKindPermission:
		return len(in.Answers) == 0 && (in.Cancelled == (in.OptionID == ""))
	case pluginsdk.HumanResponseKindClarification:
		return in.OptionID == "" && !in.Cancelled && len(in.Answers) > 0
	case pluginsdk.HumanResponseKindStepApproval:
		return in.OptionID == "" && !in.Cancelled && len(in.Answers) == 0
	default:
		return false
	}
}

// RequestHumanResponseReceipt mints a receipt for the Kandev user linked to
// the external account, after checking the workspace grant, the link, the
// user's session.control, and the target revision.
//
//nolint:cyclop // Each check maps to one typed denial a plugin must branch on.
func (m humanResponseDelegationManager) RequestHumanResponseReceipt(ctx context.Context, in pluginsdk.HumanResponseReceiptRequest) (*pluginsdk.CommandResult, pluginsdk.HumanResponseReceipt, error) {
	h := m.host
	if !validHumanResponseReceiptRequest(in) {
		return exactInvalidResult(), pluginsdk.HumanResponseReceipt{}, nil
	}
	response := HumanInteractionResponse{Kind: in.Kind, OptionID: in.OptionID, Cancelled: in.Cancelled, Answers: in.Answers}
	if result := h.authorizeHumanResponses(in.WorkspaceID, in.ApprovalRevision, in.ManifestDigest, exactHumanResponseDigest(in.Kind, response)); result != nil {
		return result, pluginsdk.HumanResponseReceipt{}, nil
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil || h.service.humanInteractionReceipts == nil {
		return exactUnavailableResult(exactReasonCommandStoreUnavailable), pluginsdk.HumanResponseReceipt{}, nil
	}
	link, err := commandStore.FindIdentityLink(ctx, h.installationID, in.Provider, in.Subject)
	if errors.Is(err, state.ErrIdentityLinkNotFound) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: reasonExternalIdentityNotLinked}, pluginsdk.HumanResponseReceipt{}, nil
	}
	if err != nil {
		return exactUnavailableResult("identity_link_unavailable"), pluginsdk.HumanResponseReceipt{}, nil
	}
	human, result := h.authorizeLinkedHuman(ctx, link.UserID, in.WorkspaceID)
	if result != nil {
		return result, pluginsdk.HumanResponseReceipt{}, nil
	}
	version, result := h.humanResponseTargetVersion(ctx, in)
	if result != nil {
		return result, pluginsdk.HumanResponseReceipt{}, nil
	}
	receipt, err := h.service.humanInteractionReceipts.issue(link.UserID, in.WorkspaceID, in.TargetID, version, response, &receiptDelegation{
		InstallationID: h.installationID, LinkID: link.ID, Provider: link.Provider, Subject: link.Subject, UserDisplayName: human.DisplayName,
	})
	if err != nil {
		return exactUnavailableResult("human_interaction_service_unavailable"), pluginsdk.HumanResponseReceipt{}, nil
	}
	if err := commandStore.RecordDelegatedResponse(ctx, state.DelegatedResponse{
		ReceiptID: receipt.ID, InstallationID: h.installationID, WorkspaceID: in.WorkspaceID, LinkID: link.ID,
		UserID: link.UserID, UserDisplayName: human.DisplayName, Provider: link.Provider, Subject: link.Subject,
		Kind: in.Kind, TargetID: in.TargetID, TargetVersion: version, PayloadDigest: exactHumanResponseDigest(in.Kind, response),
		IssuedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: receipt.ExpiresAt,
	}); err != nil {
		h.service.humanInteractionReceipts.discard(receipt.ID)
		return exactUnavailableResult("human_response_audit_unavailable"), pluginsdk.HumanResponseReceipt{}, nil
	}
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied}, pluginsdk.HumanResponseReceipt{
		ID: receipt.ID, Kind: in.Kind, TargetID: in.TargetID, ResourceVersion: version, ExpiresAt: receipt.ExpiresAt,
		Actor: pluginsdk.InteractionActor{UserID: link.UserID, DisplayName: human.DisplayName, Via: link.Provider},
	}, nil
}

// requireHumanResponsesGrant refuses a link unless some workspace currently
// grants the installation host.v2.write:human_responses. A link belongs to an
// installation and a user, so it needs the grant in at least one workspace; the
// per-workspace grant is checked again whenever a receipt is requested or
// consumed. A denied attempt leaves the link code unspent.
func (s *Service) requireHumanResponsesGrant(installationID string) *pluginsdk.CommandResult {
	approvals, err := s.approvalListByInstallation(installationID)
	if err != nil {
		return exactUnavailableResult("authorization_unavailable")
	}
	for _, approval := range approvals {
		if approval.State != ApprovalStateActive || approval.TombstonedAt != nil {
			continue
		}
		if _, ok := s.manifestIntersectionDenyReason(installationID, humanResponsesCapability, approval); !ok {
			continue
		}
		if slices.Contains(approval.CapabilityIDs, humanResponsesCapability) {
			return nil
		}
	}
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyMissingApproval)}
}

// authorizeHumanResponses checks the installation's manifest digest and its
// workspace grant for host.v2.write:human_responses.
func (h *pluginHost) authorizeHumanResponses(workspaceID string, revision uint64, manifestDigest, payloadDigest string) *pluginsdk.CommandResult {
	if h.service == nil || h.installationID == "" {
		return exactUnavailableResult("authorization_unavailable")
	}
	h.service.approvalEffectMu.Lock()
	defer h.service.approvalEffectMu.Unlock()
	return h.authorizeHumanResponsesLocked(workspaceID, revision, manifestDigest, payloadDigest)
}

// authorizeHumanResponsesLocked is authorizeHumanResponses for a caller that
// already holds approvalEffectMu, such as an admitted exact command.
func (h *pluginHost) authorizeHumanResponsesLocked(workspaceID string, revision uint64, manifestDigest, payloadDigest string) *pluginsdk.CommandResult {
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	if manifestDigest != ManifestCapabilityDigest(installed.Manifest) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(h.installationID, workspaceID, humanResponsesCapability, revision, payloadDigest, CanonicalApprovalDigest("host-method", requestHumanResponseMethod, "v2"))
	if !decision.Allowed {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	return nil
}

// authorizeLinkedHuman resolves the linked Kandev user and requires
// session.control on the workspace, evaluated as that user.
func (h *pluginHost) authorizeLinkedHuman(ctx context.Context, userID, workspaceID string) (LinkedHuman, *pluginsdk.CommandResult) {
	resolver := h.service.linkedUserResolverDep()
	authorizer := h.service.humanInteractionResponseAuthorizer
	if resolver == nil || authorizer == nil {
		return LinkedHuman{}, exactUnavailableResult("human_response_authorization_unavailable")
	}
	human, ok := resolver(ctx, userID)
	if !ok {
		return LinkedHuman{}, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: reasonLinkedUserNotAuthorized}
	}
	if err := authorizer(authn.WithIdentity(ctx, human.Identity), workspaceID); err != nil {
		return LinkedHuman{}, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: reasonLinkedUserNotAuthorized}
	}
	return human, nil
}

// humanResponseTargetVersion verifies the receipt target is still awaiting
// this kind of response at the observed version.
func (h *pluginHost) humanResponseTargetVersion(ctx context.Context, in pluginsdk.HumanResponseReceiptRequest) (string, *pluginsdk.CommandResult) {
	if in.Kind == pluginsdk.HumanResponseKindStepApproval {
		return h.stepApprovalReceiptVersion(ctx, in.WorkspaceID, in.TargetID, in.ExpectedResourceVersion)
	}
	version, err := h.service.pendingInteractionVersion(ctx, in.WorkspaceID, in.TargetID, in.ExpectedResourceVersion, in.Kind)
	switch {
	case err == nil:
		return version, nil
	case errors.Is(err, ErrHumanInteractionWorkspaceMismatch):
		return "", &pluginsdk.CommandResult{Status: pluginsdk.CommandNotFound, Reason: reasonInteractionNotFound}
	case errors.Is(err, ErrHumanInteractionNotPending):
		return "", &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "interaction_already_resolved"}
	case errors.Is(err, ErrHumanInteractionVersionChanged):
		return "", &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "interaction_revision_changed"}
	case errors.Is(err, ErrHumanInteractionResponseUnavailable):
		return "", exactUnavailableResult("human_interaction_service_unavailable")
	default:
		return "", exactInteractionErrorResult(err)
	}
}

// linkedResponseContext re-checks a delegated receipt at consumption: the
// link still exists for the same user, the grant still allows
// human_responses, and the user still holds session.control. It returns a
// context that acts as the linked user. Native receipts pass through. The
// caller is an admitted exact command and holds approvalEffectMu.
func (h *pluginHost) linkedResponseContext(ctx context.Context, receipt humanInteractionResponseReceipt, revision uint64, manifestDigest string) (context.Context, *pluginsdk.CommandResult) {
	if receipt.Delegation == nil {
		return ctx, nil
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return nil, exactUnavailableResult(exactReasonCommandStoreUnavailable)
	}
	link, err := commandStore.GetIdentityLink(ctx, h.installationID, receipt.Delegation.LinkID)
	if err != nil || link.UserID != receipt.HumanActor {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: reasonExternalIdentityNotLinked}
	}
	if result := h.authorizeHumanResponsesLocked(receipt.WorkspaceID, revision, manifestDigest, receipt.PayloadDigest); result != nil {
		return nil, result
	}
	human, result := h.authorizeLinkedHuman(ctx, link.UserID, receipt.WorkspaceID)
	if result != nil {
		return nil, result
	}
	return authn.WithIdentity(ctx, human.Identity), nil
}

// claimLinkedResponse durably records a delegated receipt before its response
// is delivered. Without that record the response must not be delivered: a
// delivery with no audit trail could not be attributed afterwards.
func (h *pluginHost) claimLinkedResponse(ctx context.Context, receiptID string, receipt humanInteractionResponseReceipt) *pluginsdk.CommandResult {
	if receipt.Delegation == nil {
		return nil
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return exactUnavailableResult("human_response_audit_unavailable")
	}
	if err := commandStore.ClaimDelegatedResponse(ctx, receiptID); err != nil {
		return exactUnavailableResult("human_response_audit_unavailable")
	}
	return nil
}

// releaseLinkedResponseClaim withdraws the claim of a response that was not
// delivered. A failed release leaves an in-doubt claim that attribution only
// honors for an interaction resolved inside the receipt's lifetime.
func (h *pluginHost) releaseLinkedResponseClaim(ctx context.Context, receiptID string, receipt humanInteractionResponseReceipt) {
	if receipt.Delegation == nil {
		return
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return
	}
	if err := commandStore.ReleaseDelegatedResponseClaim(ctx, receiptID); err != nil && h.log != nil {
		h.log.Warn("plugin delegated human response claim was not released",
			zap.String("installation_id", h.installationID), zap.String("receipt_id", receiptID), zap.Error(err))
	}
}

// markLinkedResponseConsumed records that a delegated receipt resolved its
// target. The response was already delivered, so a failed audit write is
// logged and does not turn the delivered response into a failure.
func (h *pluginHost) markLinkedResponseConsumed(ctx context.Context, receiptID string, receipt humanInteractionResponseReceipt) {
	if receipt.Delegation == nil {
		return
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return
	}
	if err := commandStore.MarkDelegatedResponseConsumed(ctx, receiptID); err != nil && h.log != nil {
		h.log.Warn("plugin delegated human response audit was not updated",
			zap.String("installation_id", h.installationID), zap.String("receipt_id", receiptID), zap.Error(err))
	}
}

// attributeInteraction sets ResolvedBy on a resolved interaction that a
// linked Kandev user answered through a plugin.
func (h *pluginHost) attributeInteraction(ctx context.Context, dto *pluginsdk.Interaction) {
	if dto == nil || dto.Status == pluginsdk.InteractionStatusPending || h.service == nil {
		return
	}
	commandStore := h.service.exactCommandStoreDep()
	if commandStore == nil {
		return
	}
	record, err := commandStore.ConsumedDelegatedResponse(ctx, dto.ID)
	if err != nil {
		record, err = h.claimedResolution(ctx, commandStore, dto)
		if err != nil {
			return
		}
	}
	dto.ResolvedBy = &pluginsdk.InteractionActor{UserID: record.UserID, DisplayName: record.UserDisplayName, Via: record.Provider}
}

// claimedResolution recovers attribution when the consumed marker was lost
// after delivery. The write-ahead claim identifies the receipt, and it only
// applies to an interaction that resolved while that receipt was valid. The
// consumed marker is repaired as a side effect; the native response is never
// repeated.
func (h *pluginHost) claimedResolution(ctx context.Context, commandStore *state.CommandStore, dto *pluginsdk.Interaction) (state.DelegatedResponse, error) {
	record, err := commandStore.ClaimedDelegatedResponse(ctx, dto.ID)
	if err != nil {
		return state.DelegatedResponse{}, err
	}
	resolvedAt, resolvedErr := time.Parse(time.RFC3339Nano, dto.UpdatedAt)
	claimedAt, claimedErr := time.Parse(time.RFC3339Nano, record.ClaimedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if resolvedErr != nil || claimedErr != nil || expiresErr != nil ||
		resolvedAt.Before(claimedAt) || resolvedAt.After(expiresAt) {
		return state.DelegatedResponse{}, state.ErrDelegatedResponseNotFound
	}
	_ = commandStore.MarkDelegatedResponseConsumed(ctx, record.ReceiptID)
	return record, nil
}
