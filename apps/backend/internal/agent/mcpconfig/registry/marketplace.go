package registry

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

var (
	ErrMarketplaceRevisionRequired  = errors.New("marketplace entry revision is required")
	ErrMarketplaceEntryUnavailable  = errors.New("marketplace entry is unavailable")
	ErrMarketplaceChoiceNotFound    = errors.New("marketplace choice not found")
	ErrMarketplaceChoiceUnsupported = errors.New("marketplace choice is unsupported")
)

// MarketplaceService combines curated entries with the last-good Registry cache.
type MarketplaceService struct {
	syncer  *SyncService
	catalog *mcpconfig.CatalogService
	curated []Entry
}

func NewMarketplaceService(syncer *SyncService, catalog *mcpconfig.CatalogService) *MarketplaceService {
	return &MarketplaceService{syncer: syncer, catalog: catalog, curated: CuratedEntries()}
}

func (s *MarketplaceService) Search(ctx context.Context, query string) (SearchResult, error) {
	if s.syncer == nil {
		return SearchResult{Entries: curatedMarketplaceEntries(s.curated, query)}, nil
	}
	entries, state, err := s.syncer.Cached(ctx, query)
	if err != nil {
		return SearchResult{}, err
	}
	marketplace := curatedMarketplaceEntries(s.curated, query)
	marketplace = append(marketplace, registryMarketplaceEntries(entries)...)
	sort.SliceStable(marketplace, func(i, j int) bool {
		if marketplace[i].Source != marketplace[j].Source {
			return marketplace[i].Source < marketplace[j].Source
		}
		return marketplace[i].Identity() < marketplace[j].Identity()
	})
	now := timeNow()
	stale := state.LastSuccessfulAt.IsZero() || now.Sub(state.LastSuccessfulAt) > registryCacheMaxAge
	return SearchResult{Entries: marketplace, Stale: stale, Degraded: state.Degraded, LastSuccessfulAt: state.LastSuccessfulAt}, nil
}

// Refresh updates the public cache. On failure it returns the last-good entries
// together with the sanitized degraded state from the sync service.
func (s *MarketplaceService) Refresh(ctx context.Context) (SyncResult, error) {
	if s.syncer == nil {
		return SyncResult{}, ErrMarketplaceCatalogUnavailable
	}
	return s.syncer.Refresh(ctx, true)
}

func (s *MarketplaceService) Get(ctx context.Context, identity string) (MarketplaceEntry, error) {
	for _, entry := range s.curated {
		if entry.Identity() == identity {
			return toMarketplaceEntry(entry, "curated", false), nil
		}
	}
	if s.syncer == nil {
		return MarketplaceEntry{}, ErrRegistryEntryNotFound
	}
	entry, err := s.syncer.store.GetMCPRegistryEntry(ctx, identity)
	if err != nil {
		return MarketplaceEntry{}, err
	}
	return toMarketplaceEntry(*entry, "registry", true), nil
}

func (s *MarketplaceService) Install(ctx context.Context, request InstallRequest) (*mcpconfig.MCPServerDefinition, error) {
	if s.catalog == nil {
		return nil, ErrMarketplaceCatalogUnavailable
	}
	if request.ExpectedRevision <= 0 {
		return nil, ErrMarketplaceRevisionRequired
	}
	entry, err := s.Get(ctx, request.Identity)
	if err != nil {
		return nil, err
	}
	if entry.Revision != request.ExpectedRevision || entry.Status != StatusActive {
		return nil, ErrMarketplaceEntryUnavailable
	}
	choice, err := selectChoice(entry.Choices, request.ChoiceID)
	if err != nil {
		return nil, err
	}
	if !choice.Selectable {
		return nil, ErrMarketplaceChoiceUnsupported
	}
	definitionInput, err := definitionInputForChoice(entry.Entry, choice, request)
	if err != nil {
		return nil, err
	}
	return s.catalog.Create(ctx, definitionInput)
}

func selectChoice(choices []Choice, choiceID string) (Choice, error) {
	for _, choice := range choices {
		if choice.ID == choiceID {
			return choice, nil
		}
	}
	return Choice{}, ErrMarketplaceChoiceNotFound
}

func definitionInputForChoice(entry Entry, choice Choice, request InstallRequest) (mcpconfig.CreateDefinitionInput, error) {
	runtimeName := strings.TrimSpace(request.RuntimeName)
	if runtimeName == "" {
		runtimeName = defaultRuntimeName(entry.Name)
	}
	displayName := strings.TrimSpace(request.DisplayName)
	if displayName == "" {
		displayName = entry.Title
	}
	if displayName == "" {
		displayName = entry.Name
	}
	bindings := make([]mcpconfig.MCPSecretBinding, 0, len(request.SecretBindings))
	for _, binding := range request.SecretBindings {
		bindings = append(bindings, mcpconfig.MCPSecretBinding{InputName: binding.InputName, SecretID: binding.SecretID})
	}
	input := mcpconfig.CreateDefinitionInput{
		WorkspaceID: request.WorkspaceID, RuntimeName: runtimeName, DisplayName: displayName,
		Description: entry.Description, Source: mcpconfig.DefinitionSourceRegistry,
		SourceIdentity: entry.Identity(), SecretBindings: bindings,
	}
	switch choice.Kind {
	case "remote":
		input.ExecutionMode = mcpconfig.ExecutionModeRemote
		input.Transport = remoteTransport(choice.Transport)
		input.Configuration.URL = choice.URL
		applyRegistryInputs(&input.Configuration, nil, choice.Headers, choice.Variables)
	case "package":
		input.ExecutionMode = mcpconfig.ExecutionModeManagedPackage
		input.Transport = mcpconfig.ServerTypeStdio
		input.Configuration.PackageType = choice.RegistryType
		input.Configuration.PackageName = choice.Identifier
		input.Configuration.PackageVersion = choice.Version
		input.Configuration.PackageRegistry = choice.RegistryBaseURL
		input.Configuration.PackageExecutable = choice.RuntimeHint
		input.Configuration.PackageRuntimeArguments = argumentValues(choice.RuntimeArguments)
		input.Configuration.PackageArguments = argumentValues(choice.PackageArguments)
		applyRegistryInputs(&input.Configuration, choice.EnvironmentVariables, choice.Headers, choice.Variables)
	default:
		return mcpconfig.CreateDefinitionInput{}, ErrMarketplaceChoiceUnsupported
	}
	return input, nil
}

func curatedMarketplaceEntries(entries []Entry, query string) []MarketplaceEntry {
	result := make([]MarketplaceEntry, 0, len(entries))
	for _, entry := range entries {
		if matchesEntry(entry, query) {
			result = append(result, toMarketplaceEntry(entry, "curated", false))
		}
	}
	return result
}

func registryMarketplaceEntries(entries []Entry) []MarketplaceEntry {
	result := make([]MarketplaceEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, toMarketplaceEntry(entry, "registry", true))
	}
	return result
}

func toMarketplaceEntry(entry Entry, source string, publisherSupplied bool) MarketplaceEntry {
	if entry.Status == "" && !publisherSupplied {
		entry.Status = StatusActive
	}
	if entry.Revision <= 0 {
		entry.Revision = 1
	}
	notice := "Kandev-curated template"
	if publisherSupplied {
		notice = "Publisher-supplied metadata. This is not a Kandev security review."
	}
	return MarketplaceEntry{Entry: entry, Source: source, PublisherSupplied: publisherSupplied, TrustNotice: notice, Choices: entryChoices(entry)}
}

func entryChoices(entry Entry) []Choice {
	choices := make([]Choice, 0, len(entry.Packages)+len(entry.Remotes))
	for index, packageChoice := range entry.Packages {
		choice := Choice{
			ID: "package-" + strconv.Itoa(index), Kind: "package", RegistryType: packageChoice.RegistryType,
			RegistryBaseURL: packageChoice.RegistryBaseURL, Identifier: packageChoice.Identifier,
			Version: packageChoice.Version, RuntimeHint: packageChoice.RuntimeHint,
			RuntimeArguments:     append([]Argument(nil), packageChoice.RuntimeArguments...),
			PackageArguments:     append([]Argument(nil), packageChoice.PackageArguments...),
			EnvironmentVariables: append([]KeyValueInput(nil), packageChoice.EnvironmentVariables...),
			Transport:            packageChoice.Transport.Type,
			Headers:              append([]KeyValueInput(nil), packageChoice.Transport.Headers...),
			Variables:            cloneVariables(packageChoice.Transport.Variables),
		}
		choice.Selectable, choice.UnsupportedReason = packageSupported(packageChoice)
		choices = append(choices, choice)
	}
	for index, remote := range entry.Remotes {
		transport, endpoint := normalizeRemote(remote)
		choice := Choice{
			ID: "remote-" + strconv.Itoa(index), Kind: "remote", Transport: transport, URL: endpoint,
			Headers: append([]KeyValueInput(nil), remote.Headers...), Variables: cloneVariables(remote.Variables),
		}
		choice.Selectable, choice.UnsupportedReason = remoteSupported(transport, endpoint)
		choices = append(choices, choice)
	}
	return choices
}

func packageSupported(packageChoice Package) (bool, string) {
	if packageChoice.RegistryType != "npm" {
		return false, "No materializer is available for this package type"
	}
	if packageChoice.Transport.Type != "stdio" {
		return false, "The managed npm materializer supports stdio packages only"
	}
	if !exactVersion(packageChoice.Version) {
		return false, "The package must use an exact version"
	}
	return true, ""
}

func remoteSupported(transport, endpoint string) (bool, string) {
	if transport != "streamable-http" && transport != "sse" {
		return false, "The remote transport is not supported"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false, "The remote endpoint must use http or https"
	}
	return true, ""
}

func normalizeRemote(remote Remote) (string, string) {
	transport := strings.ToLower(strings.TrimSpace(remote.Type))
	if transport == "streamable_http" {
		transport = "streamable-http"
	}
	endpoint := remote.URL
	if endpoint == "" {
		endpoint = remote.SSEURL
	}
	return transport, endpoint
}

func remoteTransport(value string) mcpconfig.ServerType {
	if value == "sse" {
		return mcpconfig.ServerTypeSSE
	}
	return mcpconfig.ServerTypeStreamableHTTP
}

func matchesEntry(entry Entry, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(entry.Name), query) || strings.Contains(strings.ToLower(entry.Title), query) || strings.Contains(strings.ToLower(entry.Description), query)
}

func defaultRuntimeName(name string) string {
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return strings.ReplaceAll(strings.TrimSpace(name), " ", "-")
}

func exactVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "latest" {
		return false
	}
	_, err := semver.StrictNewVersion(value)
	return err == nil
}

func argumentValues(arguments []Argument) []string {
	values := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if strings.TrimSpace(argument.Value) != "" {
			values = append(values, argument.Value)
		} else if strings.TrimSpace(argument.Name) != "" {
			values = append(values, argument.Name)
		}
	}
	return values
}

func applyRegistryInputs(
	configuration *mcpconfig.MCPServerConfiguration,
	environmentVariables, headers []KeyValueInput,
	variables map[string]KeyValueInput,
) {
	if configuration.Env == nil {
		configuration.Env = map[string]string{}
	}
	for _, input := range environmentVariables {
		if input.IsSecret || strings.TrimSpace(input.Name) == "" {
			continue
		}
		configuration.Env[input.Name] = input.Value
	}
	if configuration.Headers == nil {
		configuration.Headers = map[string]string{}
	}
	for _, input := range headers {
		if input.IsSecret || strings.TrimSpace(input.Name) == "" {
			continue
		}
		configuration.Headers[input.Name] = input.Value
	}
	if len(variables) > 0 {
		if configuration.Options == nil {
			configuration.Options = map[string]any{}
		}
		configuration.Options["registry_variables"] = cloneVariables(variables)
	}
}

func cloneVariables(variables map[string]KeyValueInput) map[string]KeyValueInput {
	if variables == nil {
		return nil
	}
	clone := make(map[string]KeyValueInput, len(variables))
	for key, value := range variables {
		clone[key] = value
	}
	return clone
}

var timeNow = func() time.Time { return time.Now().UTC() }
