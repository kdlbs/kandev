package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

const (
	repositoryInspectActionKey       = "repositories.inspect"
	maxRepositoryInspectResponseSize = 1 << 20
	repositoryInspectActionTimeout   = 15 * time.Second
	maxRepositoryProviderScopeBytes  = 512
	nullJSONValue                    = "null"
	repositoryProviderInspectionNone = "none"
	// repositoryProviderInspectionNotSent marks an inspection that never
	// reached the plugin action dispatcher.
	repositoryProviderInspectionNotSent = "not_sent"
	repositoryInspectionLogEvent        = "plugin repository inspection"
)

// RepositoryProviderInspectionRequest contains the untrusted URL and optional
// immutable identity hints supplied by a task-create request. The hints are
// checked against the plugin result but are never sent to the plugin.
type RepositoryProviderInspectionRequest struct {
	Provider             string
	URL                  string
	ProviderScope        string
	ProviderRepositoryID string
}

// RepositoryProviderInspection is the authoritative, credential-free
// repository identity returned by an active repository-provider plugin.
type RepositoryProviderInspection struct {
	ProviderID           string `json:"provider_id"`
	ProviderHost         string `json:"provider_host"`
	ProviderScope        string `json:"provider_scope"`
	ProviderRepositoryID string `json:"provider_repository_id"`
	OwnerOrProject       string `json:"owner_or_project"`
	Name                 string `json:"name"`
	CloneURL             string `json:"clone_url"`
	DefaultBranch        string `json:"default_branch"`
}

// RepositoryProviderErrorCode classifies a repository-provider inspection
// failure without exposing plugin response bodies or upstream error text.
type RepositoryProviderErrorCode string

const (
	RepositoryProviderErrorInvalid     RepositoryProviderErrorCode = "invalid"
	RepositoryProviderErrorNotFound    RepositoryProviderErrorCode = "not_found"
	RepositoryProviderErrorUnavailable RepositoryProviderErrorCode = "unavailable"
)

// RepositoryProviderError is a safe, typed failure from repository-provider
// inspection. The wrapped cause is available to internal callers for
// cancellation checks, while Error never includes it.
type RepositoryProviderError struct {
	Code  RepositoryProviderErrorCode
	cause error
}

func (e *RepositoryProviderError) Error() string {
	switch e.Code {
	case RepositoryProviderErrorInvalid:
		return "repository provider returned an invalid repository descriptor"
	case RepositoryProviderErrorNotFound:
		return "repository was not found by the repository provider"
	case RepositoryProviderErrorUnavailable:
		return "repository provider is unavailable"
	default:
		return "repository provider inspection failed"
	}
}

func (e *RepositoryProviderError) Unwrap() error { return e.cause }

// InspectRepositoryProvider asks the active manifest owner of the provider to
// resolve a repository URL into a validated descriptor.
func (s *Service) InspectRepositoryProvider(
	ctx context.Context, workspaceID string, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, error) {
	return s.inspectRepositoryProvider(ctx, workspaceID, request, s.InvokeAction)
}

func (s *Service) inspectRepositoryProvider(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderInspectionRequest,
	invoke repositoryActionInvoker,
) (inspection *RepositoryProviderInspection, err error) {
	started := time.Now()
	providerID := strings.TrimSpace(request.Provider)
	pluginID := ""
	responseShape := repositoryProviderInspectionNotSent
	defer func() {
		s.logRepositoryInspection(providerID, pluginID, workspaceID, started, responseShape, err)
	}()

	request.Provider = providerID
	request.URL = strings.TrimSpace(request.URL)
	if strings.TrimSpace(workspaceID) == "" || request.Provider == "" || request.URL == "" {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	record, action, err := s.repositoryProviderAction(request.Provider, repositoryInspectActionKey)
	if err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorUnavailable, err)
	}
	pluginID = record.ID
	outcome := s.runRepositoryProviderInspection(ctx, workspaceID, request, record, action, invoke)
	responseShape = outcome.responseShape
	if outcome.err != nil {
		return nil, outcome.err
	}
	if !outcome.matched {
		return nil, newRepositoryProviderError(RepositoryProviderErrorNotFound, nil)
	}
	return outcome.inspection, nil
}

// runRepositoryProviderInspection performs one manifest-owned inspection and
// reports the three outcomes callers need to distinguish: a validated match, an
// ordinary no-match, and a typed failure.
func (s *Service) runRepositoryProviderInspection(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderInspectionRequest,
	record *store.Record,
	action manifest.Action,
	invoke repositoryActionInvoker,
) repositoryInspectionOutcome {
	body, err := json.Marshal(map[string]string{"url": request.URL})
	if err != nil || action.MaxBodyBytes <= 0 || len(body) > action.MaxBodyBytes {
		return repositoryInspectionOutcome{
			responseShape: repositoryProviderInspectionNotSent,
			err:           newRepositoryProviderError(RepositoryProviderErrorInvalid, err),
		}
	}
	invokeCtx, cancel := context.WithTimeout(ctx, repositoryInspectActionTimeout)
	defer cancel()
	response, err := invoke(invokeCtx, record.ID, dispatchGeneration(record), &pluginsdk.PluginActionRequest{
		ActionKey: repositoryInspectActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: workspaceID},
		Body:      body,
	})
	outcome := repositoryInspectionOutcome{responseShape: repositoryProviderInspectionResponseShape(response)}
	if err != nil {
		outcome.err = newRepositoryProviderError(RepositoryProviderErrorUnavailable, err)
		return outcome
	}
	inspection, matched, parseErr := parseRepositoryProviderActionOutcome(response, request)
	outcome.inspection, outcome.matched, outcome.err = inspection, matched, parseErr
	return outcome
}

// logRepositoryInspection records one sanitized inspection outcome. It never
// includes the URL, response body, plugin configuration, or credentials.
func (s *Service) logRepositoryInspection(
	providerID, pluginID, workspaceID string, started time.Time, responseShape string, err error,
) {
	if s.log == nil {
		return
	}
	failureCategory := repositoryProviderInspectionNone
	if err != nil {
		failureCategory = repositoryProviderInspectionFailureCategory(err)
	}
	if responseShape == "" {
		responseShape = repositoryProviderInspectionNone
	}
	s.log.Info(repositoryInspectionLogEvent,
		zap.String("provider_id", providerID),
		zap.String("plugin_id", pluginID),
		zap.String("workspace_id", workspaceID),
		zap.Duration("duration", time.Since(started)),
		zap.String("response_shape", responseShape),
		zap.String("failure_category", failureCategory))
}

func parseRepositoryProviderActionResponse(
	response *pluginsdk.PluginActionResponse, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, error) {
	inspection, matched, err := parseRepositoryProviderActionOutcome(response, request)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, newRepositoryProviderError(RepositoryProviderErrorNotFound, nil)
	}
	return inspection, nil
}

func parseRepositoryProviderActionOutcome(
	response *pluginsdk.PluginActionResponse, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, bool, error) {
	if response != nil && response.Status != 0 && (response.Status < 200 || response.Status >= 300) {
		return nil, false, repositoryProviderStatusError(response.Status)
	}
	return parseRepositoryProviderInspectionOutcome(response, request)
}

func repositoryProviderInspectionFailureCategory(err error) string {
	var providerErr *RepositoryProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return string(providerErr.Code)
	}
	return "unknown"
}

func repositoryProviderInspectionResponseShape(response *pluginsdk.PluginActionResponse) string {
	if response == nil {
		return repositoryProviderInspectionNone
	}
	if response.Status != 0 && (response.Status < 200 || response.Status >= 300) {
		return "http_status"
	}
	if len(response.Body) == 0 {
		return "empty"
	}
	if len(response.Body) > maxRepositoryInspectResponseSize {
		return "oversized"
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body, &raw); err != nil {
		return "invalid_json"
	}
	if matched, exists := raw["matched"]; exists {
		var value bool
		if err := json.Unmarshal(matched, &value); err != nil {
			return "invalid_matched"
		}
		if !value {
			return "matched_false"
		}
	}
	if _, nested := raw["repository"]; nested {
		return "nested_descriptor"
	}
	return "direct_descriptor"
}

func parseRepositoryProviderInspection(
	response *pluginsdk.PluginActionResponse, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, error) {
	inspection, matched, err := parseRepositoryProviderInspectionOutcome(response, request)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, newRepositoryProviderError(RepositoryProviderErrorNotFound, nil)
	}
	return inspection, nil
}

// parseRepositoryProviderInspectionOutcome keeps an ordinary no-match distinct
// from a provider 404 or a malformed descriptor. matched is false with a nil
// error only for the explicit `{"matched":false}` envelope.
func parseRepositoryProviderInspectionOutcome(
	response *pluginsdk.PluginActionResponse, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, bool, error) {
	if response == nil || len(response.Body) == 0 || len(response.Body) > maxRepositoryInspectResponseSize {
		return nil, false, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body, &raw); err != nil {
		return nil, false, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	if matched, exists := raw["matched"]; exists {
		var value bool
		if err := json.Unmarshal(matched, &value); err != nil {
			return nil, false, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
		}
		if !value {
			if repository, hasRepository := raw["repository"]; hasRepository && string(repository) != nullJSONValue {
				return nil, false, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
			}
			return nil, false, nil
		}
	}

	candidate, err := inspectionCandidate(raw)
	if err != nil {
		return nil, false, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	inspection, err := validateRepositoryProviderInspection(candidate, request)
	if err != nil {
		return nil, false, err
	}
	return inspection, true, nil
}

func inspectionCandidate(raw map[string]json.RawMessage) (*RepositoryProviderInspection, error) {
	if repository, nested := raw["repository"]; nested {
		if string(repository) == nullJSONValue {
			return nil, errors.New("repository descriptor is null")
		}
		var candidate RepositoryProviderInspection
		if err := json.Unmarshal(repository, &candidate); err != nil {
			return nil, err
		}
		return &candidate, nil
	}
	var candidate RepositoryProviderInspection
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &candidate); err != nil {
		return nil, err
	}
	return &candidate, nil
}

func validateRepositoryProviderInspection(
	candidate *RepositoryProviderInspection, request RepositoryProviderInspectionRequest,
) (*RepositoryProviderInspection, error) {
	if candidate == nil || !strings.EqualFold(strings.TrimSpace(candidate.ProviderID), request.Provider) {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	host, err := repoclone.HTTPSProviderOrigin(candidate.ProviderHost)
	if err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	scope, err := normalizeRepositoryProviderScope(candidate.ProviderScope)
	if err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	repositoryID := strings.TrimSpace(candidate.ProviderRepositoryID)
	owner := strings.TrimSpace(candidate.OwnerOrProject)
	name := strings.TrimSpace(candidate.Name)
	cloneURL := strings.TrimSpace(candidate.CloneURL)
	defaultBranch := strings.TrimSpace(candidate.DefaultBranch)
	if repositoryID == "" || owner == "" || name == "" || cloneURL == "" || defaultBranch == "" ||
		strings.ContainsAny(repositoryID+owner+name+defaultBranch, "\x00") {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	if err := repoclone.ValidateHTTPSCloneOrigin(cloneURL, host); err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	if err := validateRepositoryProviderHint(request.ProviderScope, scope); err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, err)
	}
	if hint := strings.TrimSpace(request.ProviderRepositoryID); hint != "" && hint != repositoryID {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	return &RepositoryProviderInspection{
		ProviderID: request.Provider, ProviderHost: host, ProviderScope: scope,
		ProviderRepositoryID: repositoryID, OwnerOrProject: owner, Name: name,
		CloneURL: cloneURL, DefaultBranch: defaultBranch,
	}, nil
}

func normalizeRepositoryProviderScope(raw string) (string, error) {
	scope := strings.TrimSpace(raw)
	if len(scope) > maxRepositoryProviderScopeBytes || strings.ContainsRune(scope, '\x00') {
		return "", errors.New("repository provider scope is invalid")
	}
	return scope, nil
}

func validateRepositoryProviderHint(hint, value string) error {
	normalized, err := normalizeRepositoryProviderScope(hint)
	if err != nil {
		return err
	}
	if normalized != "" && normalized != value {
		return errors.New("repository provider scope does not match the request hint")
	}
	return nil
}

func repositoryProviderStatusError(status int) error {
	if status == 404 {
		return newRepositoryProviderError(RepositoryProviderErrorNotFound, nil)
	}
	if status >= 400 && status < 500 && status != 408 && status != 429 {
		return newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	return newRepositoryProviderError(RepositoryProviderErrorUnavailable, nil)
}

func newRepositoryProviderError(code RepositoryProviderErrorCode, cause error) error {
	return &RepositoryProviderError{Code: code, cause: cause}
}
