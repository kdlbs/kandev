package plugins

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"go.uber.org/zap"
)

const (
	// maxRepositoryDiscoveryConcurrency bounds how many plugin inspections one
	// discovery pass keeps in flight. Each inspection also retains its declared
	// request limit and the shared 1 MiB response limit.
	maxRepositoryDiscoveryConcurrency   = 4
	repositoryDiscoveryLogEvent         = "plugin repository discovery"
	repositoryDiscoveryCanceled         = "canceled"
	repositoryDiscoveryDeadlineExceeded = "deadline_exceeded"
	repositoryDiscoveryEligibility      = "eligibility_changed"
)

// errRepositoryProviderEligibilityChanged reports that the eligible provider
// set changed while a discovery pass was in flight. The caller retries; a
// partially observed candidate set never authorizes a selection.
var errRepositoryProviderEligibilityChanged = errors.New("plugin repository provider eligibility changed during discovery")

// RepositoryProviderDiscoveryRequest is a bare repository URL supplied by a
// verified host caller. Provider hints stay host-side for response validation
// and are never forwarded to a plugin.
type RepositoryProviderDiscoveryRequest struct {
	URL                  string
	ProviderScope        string
	ProviderRepositoryID string
}

// RepositoryProviderDiscoveryResult holds every validated candidate and the
// most severe ordinary candidate failure. Candidates stay separate: the caller
// decides whether the set is unambiguous.
type RepositoryProviderDiscoveryResult struct {
	Candidates []RepositoryProviderInspection
	Failure    *RepositoryProviderError
}

// repositoryProviderDiscoveryCandidate is one active manifest-owned
// repository provider that declares the workspace inspection capability.
type repositoryProviderDiscoveryCandidate struct {
	pluginID     string
	provider     string
	record       *store.Record
	action       manifest.Action
	generation   pluginDispatchGeneration
	installation string
}

// DiscoverRepositoryProviders inspects a bare repository URL with every active
// manifest-owned provider that declares the workspace inspection capability.
// It returns all validated candidates; the caller owns ambiguity arbitration.
func (s *Service) DiscoverRepositoryProviders(
	ctx context.Context, workspaceID string, request RepositoryProviderDiscoveryRequest,
) (*RepositoryProviderDiscoveryResult, error) {
	return s.discoverRepositoryProviders(ctx, workspaceID, request, s.InvokeAction)
}

func (s *Service) discoverRepositoryProviders(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderDiscoveryRequest,
	invoke repositoryActionInvoker,
) (*RepositoryProviderDiscoveryResult, error) {
	return s.discoverRepositoryProvidersWithin(ctx, workspaceID, request, invoke, repositoryInspectActionTimeout)
}

func (s *Service) discoverRepositoryProvidersWithin(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderDiscoveryRequest,
	invoke repositoryActionInvoker,
	timeout time.Duration,
) (result *RepositoryProviderDiscoveryResult, err error) {
	started := time.Now()
	candidateCount := 0
	defer func() {
		s.logRepositoryDiscovery(workspaceID, candidateCount, result, err, time.Since(started))
	}()

	request.URL = strings.TrimSpace(request.URL)
	request.ProviderScope = strings.TrimSpace(request.ProviderScope)
	request.ProviderRepositoryID = strings.TrimSpace(request.ProviderRepositoryID)
	if strings.TrimSpace(workspaceID) == "" || request.URL == "" {
		return nil, newRepositoryProviderError(RepositoryProviderErrorInvalid, nil)
	}
	snapshot := s.repositoryProviderDiscoverySnapshot()
	candidateCount = len(snapshot)
	outcomes, err := s.runRepositoryProviderDiscovery(ctx, workspaceID, request, snapshot, invoke, timeout)
	if err != nil {
		return nil, err
	}
	if !s.repositoryProviderDiscoverySnapshotUnchanged(snapshot) {
		return nil, newRepositoryProviderError(RepositoryProviderErrorUnavailable, errRepositoryProviderEligibilityChanged)
	}
	return repositoryProviderDiscoveryResult(outcomes), nil
}

// runRepositoryProviderDiscovery schedules one bounded inspection per candidate
// under a single discovery deadline. Every started call is joined before the
// outcomes are evaluated, so an interrupted pass never looks like a no-match.
func (s *Service) runRepositoryProviderDiscovery(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderDiscoveryRequest,
	snapshot []repositoryProviderDiscoveryCandidate,
	invoke repositoryActionInvoker,
	timeout time.Duration,
) ([]repositoryInspectionOutcome, error) {
	outcomes := make([]repositoryInspectionOutcome, len(snapshot))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(snapshot) == 0 {
		return outcomes, nil
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	slots := make(chan struct{}, maxRepositoryDiscoveryConcurrency)
	var wait sync.WaitGroup
	for index := range snapshot {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case slots <- struct{}{}:
			case <-discoveryCtx.Done():
				return
			}
			defer func() { <-slots }()
			outcomes[index] = s.inspectRepositoryDiscoveryCandidate(discoveryCtx, workspaceID, request, snapshot[index], invoke)
		}(index)
	}
	wait.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := discoveryCtx.Err(); err != nil {
		return nil, newRepositoryProviderError(RepositoryProviderErrorUnavailable, err)
	}
	return outcomes, nil
}

func (s *Service) inspectRepositoryDiscoveryCandidate(
	ctx context.Context,
	workspaceID string,
	request RepositoryProviderDiscoveryRequest,
	candidate repositoryProviderDiscoveryCandidate,
	invoke repositoryActionInvoker,
) repositoryInspectionOutcome {
	started := time.Now()
	outcome := s.runRepositoryProviderInspection(ctx, workspaceID, RepositoryProviderInspectionRequest{
		Provider: candidate.provider, URL: request.URL,
		ProviderScope: request.ProviderScope, ProviderRepositoryID: request.ProviderRepositoryID,
	}, candidate.record, candidate.action, invoke)
	s.logRepositoryInspection(candidate.provider, candidate.pluginID, workspaceID, started, outcome.responseShape, outcome.err)
	return outcome
}

// repositoryInspectionOutcome keeps the three inspection results separate so
// discovery never reports an unrelated URL as a provider failure.
type repositoryInspectionOutcome struct {
	inspection    *RepositoryProviderInspection
	matched       bool
	responseShape string
	err           error
}

// repositoryProviderDiscoverySnapshot copies the eligible provider set before
// dispatch. Providers are ordered by plugin and provider ID for stable
// diagnostics only; ordering never grants selection precedence.
func (s *Service) repositoryProviderDiscoverySnapshot() []repositoryProviderDiscoveryCandidate {
	records := s.registry.List()
	candidates := make([]repositoryProviderDiscoveryCandidate, 0, len(records))
	for _, record := range records {
		if record == nil || record.Status != StatusActive {
			continue
		}
		action, declared := repositoryInspectWorkspaceAction(record)
		if !declared {
			continue
		}
		for _, rawProvider := range record.RepositoryProviders {
			provider := strings.TrimSpace(rawProvider)
			if provider == "" {
				continue
			}
			if owner, found := s.registry.activeRepositoryProviderOwner(provider, ""); !found || owner != record.ID {
				continue
			}
			candidates = append(candidates, repositoryProviderDiscoveryCandidate{
				pluginID: record.ID, provider: provider, record: record, action: action,
				generation: dispatchGeneration(record), installation: record.InstallationID,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].pluginID != candidates[j].pluginID {
			return candidates[i].pluginID < candidates[j].pluginID
		}
		return candidates[i].provider < candidates[j].provider
	})
	return candidates
}

func repositoryInspectWorkspaceAction(record *store.Record) (manifest.Action, bool) {
	for _, action := range record.Actions {
		if action.Key == repositoryInspectActionKey && action.ResourceScope == manifest.ActionScopeWorkspace {
			return action, true
		}
	}
	return manifest.Action{}, false
}

// repositoryProviderDiscoverySnapshotUnchanged rechecks eligibility, ownership,
// and installation generation before a candidate set can authorize a selection.
func (s *Service) repositoryProviderDiscoverySnapshotUnchanged(snapshot []repositoryProviderDiscoveryCandidate) bool {
	current := s.repositoryProviderDiscoverySnapshot()
	if len(current) != len(snapshot) {
		return false
	}
	for index := range current {
		if current[index].pluginID != snapshot[index].pluginID ||
			current[index].provider != snapshot[index].provider ||
			current[index].installation != snapshot[index].installation ||
			!current[index].generation.matches(snapshot[index].record) {
			return false
		}
	}
	return true
}

func repositoryProviderDiscoveryResult(outcomes []repositoryInspectionOutcome) *RepositoryProviderDiscoveryResult {
	result := &RepositoryProviderDiscoveryResult{Candidates: make([]RepositoryProviderInspection, 0, len(outcomes))}
	for _, outcome := range outcomes {
		if outcome.err != nil {
			result.Failure = preferRepositoryProviderFailure(result.Failure, outcome.err)
			continue
		}
		if outcome.matched && outcome.inspection != nil {
			result.Candidates = append(result.Candidates, *outcome.inspection)
		}
	}
	return result
}

// preferRepositoryProviderFailure keeps the most severe ordinary candidate
// failure in snapshot order: unavailable, then invalid, then not-found.
func preferRepositoryProviderFailure(current *RepositoryProviderError, next error) *RepositoryProviderError {
	providerErr := normalizedRepositoryProviderError(next)
	if current == nil || repositoryProviderFailureRank(providerErr.Code) > repositoryProviderFailureRank(current.Code) {
		return providerErr
	}
	return current
}

func normalizedRepositoryProviderError(err error) *RepositoryProviderError {
	var providerErr *RepositoryProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return providerErr
	}
	return &RepositoryProviderError{Code: RepositoryProviderErrorUnavailable, cause: err}
}

func repositoryProviderFailureRank(code RepositoryProviderErrorCode) int {
	switch code {
	case RepositoryProviderErrorUnavailable:
		return 3
	case RepositoryProviderErrorInvalid:
		return 2
	case RepositoryProviderErrorNotFound:
		return 1
	default:
		return 0
	}
}

// logRepositoryDiscovery records exactly one sanitized completion event for a
// discovery pass, including fatal exits, without the URL, response bodies,
// plugin configuration, or credentials.
func (s *Service) logRepositoryDiscovery(
	workspaceID string,
	candidateCount int,
	result *RepositoryProviderDiscoveryResult,
	err error,
	duration time.Duration,
) {
	if s.log == nil {
		return
	}
	matches := 0
	if result != nil {
		matches = len(result.Candidates)
	}
	s.log.Info(repositoryDiscoveryLogEvent,
		zap.String("workspace_id", workspaceID),
		zap.Int("candidate_count", candidateCount),
		zap.Int("match_count", matches),
		zap.Duration("duration", duration),
		zap.String("failure_category", repositoryDiscoveryFailureCategory(err, result)))
}

func repositoryDiscoveryFailureCategory(err error, result *RepositoryProviderDiscoveryResult) string {
	switch {
	case errors.Is(err, context.Canceled):
		return repositoryDiscoveryCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return repositoryDiscoveryDeadlineExceeded
	case errors.Is(err, errRepositoryProviderEligibilityChanged):
		return repositoryDiscoveryEligibility
	case err != nil:
		return repositoryProviderInspectionFailureCategory(err)
	case result != nil && result.Failure != nil:
		return string(result.Failure.Code)
	default:
		return repositoryProviderInspectionNone
	}
}
