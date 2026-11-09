package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/gitlocator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	repositoryValidationScopeLocatorIdentity    = "locator_identity"
	repositoryValidationScopeProviderInspection = "provider_inspection"
	azureGitPathSegment                         = "_git"
)

// RepositoryRemoteURLPreview is the bounded read-only result of validating a
// proposed clone-locator replacement against a saved registration identity.
type RepositoryRemoteURLPreview struct {
	RepositoryID             string `json:"repository_id"`
	WorkspaceID              string `json:"workspace_id"`
	CurrentRemoteURL         string `json:"current_remote_url,omitempty"`
	CurrentRemoteURLRedacted bool   `json:"current_remote_url_redacted,omitempty"`
	EffectiveRemoteURL       string `json:"effective_remote_url"`
	Provider                 string `json:"provider"`
	ProviderHost             string `json:"provider_host"`
	ProviderScope            string `json:"provider_scope,omitempty"`
	ProviderRepoID           string `json:"provider_repo_id,omitempty"`
	ProviderOwner            string `json:"provider_owner"`
	ProviderName             string `json:"provider_name"`
	ValidationScope          string `json:"validation_scope"`
	Changed                  bool   `json:"changed"`
}

type repositoryRemoteURLValidation struct {
	preview          RepositoryRemoteURLPreview
	observedIdentity *models.RepositoryIdentityFence
}

// ValidateRepositoryRemoteURLUpdate validates a proposed clone locator against
// the saved repository identity without writing. The preview requires mutation
// authority because it inspects a proposed managed write.
func (s *Service) ValidateRepositoryRemoteURLUpdate(
	ctx context.Context, repositoryID, proposedURL string,
) (*RepositoryRemoteURLPreview, error) {
	repository, err := s.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	if err := s.AuthorizeWorkspaceScope(ctx, repository.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	if err := s.refuseReadOnlyRepositoryWorkspace(ctx, repository.WorkspaceID); err != nil {
		return nil, err
	}
	validation, err := s.validateRepositoryRemoteURL(ctx, repository, proposedURL)
	if err != nil {
		return nil, err
	}
	return &validation.preview, nil
}

func (s *Service) refuseReadOnlyRepositoryWorkspace(ctx context.Context, workspaceID string) error {
	readOnly, err := s.workspaceWorkflowMutationReadOnly(ctx, workspaceID)
	if err != nil {
		return err
	}
	if readOnly {
		return ErrRepositoryReadOnly
	}
	return nil
}

func (s *Service) validateRepositoryRemoteURL(
	ctx context.Context, repository *models.Repository, proposedURL string,
) (*repositoryRemoteURLValidation, error) {
	if repository == nil {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	provider := strings.TrimSpace(repository.Provider)
	if provider == "" {
		return nil, invalidRemoteURLSetting()
	}
	candidate, err := parseRepositoryRepairCandidate(proposedURL)
	if err != nil {
		return nil, invalidRemoteURLSetting()
	}
	var validated repositoryRemoteURLValidation
	if isBuiltInRepositoryProvider(provider) {
		validated, err = s.validateBuiltInRepositoryRemoteURL(ctx, repository, candidate)
	} else {
		validated, err = s.validatePluginRepositoryRemoteURL(ctx, repository, candidate)
	}
	if err != nil {
		return nil, err
	}
	validated.observedIdentity = models.NewRepositoryIdentityFence(repository)
	validated.preview.RepositoryID = repository.ID
	validated.preview.WorkspaceID = repository.WorkspaceID
	validated.preview.Provider = repository.Provider
	if isBuiltInRepositoryProvider(repository.Provider) {
		validated.preview.ProviderHost = normalizeProviderHost(repository.Provider, repository.ProviderHost)
	} else {
		validated.preview.ProviderHost = strings.TrimRight(repository.ProviderHost, "/")
	}
	validated.preview.ProviderScope = repository.ProviderScope
	validated.preview.ProviderRepoID = repository.ProviderRepoID
	validated.preview.ProviderOwner = repository.ProviderOwner
	validated.preview.ProviderName = repository.ProviderName
	validated.preview.Changed = validated.preview.EffectiveRemoteURL != repository.RemoteURL
	if current, ok := displayableRemoteURL(repository.RemoteURL); ok {
		validated.preview.CurrentRemoteURL = current
	} else if repository.RemoteURL != "" {
		validated.preview.CurrentRemoteURLRedacted = true
	}
	return &validated, nil
}

func isBuiltInRepositoryProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case providerGitHub, providerGitLab, providerAzureDevOps:
		return true
	default:
		return false
	}
}

func invalidRemoteURLSetting() error {
	return fmt.Errorf("%w: remote_url is not a supported credential-free repository locator", ErrInvalidRepositorySettings)
}

type repositoryRepairCandidate struct {
	raw             string
	isSCP           bool
	httpsEquivalent string
	scheme          string
	host            string
	segments        []string
}

func parseRepositoryRepairCandidate(raw string) (repositoryRepairCandidate, error) {
	if malformedRepairRaw(raw) {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	if strings.HasPrefix(raw, "git@") {
		return parseSCPRepairCandidate(raw)
	}
	return parseURLRepairCandidate(raw)
}

func malformedRepairRaw(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return true
	}
	return strings.IndexFunc(raw, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character) || character == '\\'
	}) >= 0
}

func parseSCPRepairCandidate(raw string) (repositoryRepairCandidate, error) {
	scp, err := gitlocator.ParseConventionalSCP(raw)
	if err != nil {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	return repositoryRepairCandidate{
		raw:             raw,
		isSCP:           true,
		httpsEquivalent: "https://" + strings.ToLower(scp.Host) + "/" + scp.Path,
		scheme:          protocolHTTPS,
		host:            strings.ToLower(scp.Host),
		segments:        strings.Split(scp.Path, "/"),
	}, nil
}

func parseURLRepairCandidate(raw string) (repositoryRepairCandidate, error) {
	if strings.ContainsAny(raw, "?#") {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	parsed, err := url.Parse(raw)
	if err != nil || !validRepairURLAuthority(parsed) {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != protocolHTTPS && scheme != protocolHTTP {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	segments, ok := repairURLPathSegments(parsed.EscapedPath())
	if !ok {
		return repositoryRepairCandidate{}, invalidRemoteURLSetting()
	}
	return repositoryRepairCandidate{
		raw:      raw,
		scheme:   scheme,
		host:     strings.ToLower(parsed.Host),
		segments: segments,
	}, nil
}

func validRepairURLAuthority(parsed *url.URL) bool {
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" || strings.Contains(parsed.Host, "%") {
		return false
	}
	return validRepairHost(strings.ToLower(parsed.Hostname()))
}

func repairURLPathSegments(escapedPath string) ([]string, bool) {
	if !strings.HasPrefix(escapedPath, "/") || strings.HasSuffix(escapedPath, "/") {
		return nil, false
	}
	rawSegments := strings.Split(strings.TrimPrefix(escapedPath, "/"), "/")
	segments := make([]string, 0, len(rawSegments))
	for _, rawSegment := range rawSegments {
		if rawSegment == "" {
			return nil, false
		}
		segment, err := url.PathUnescape(rawSegment)
		if err != nil || !validRepairPathSegment(segment) {
			return nil, false
		}
		segments = append(segments, segment)
	}
	return segments, true
}

func validRepairHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if strings.Contains(host, ":") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := 0; index < len(label); index++ {
			character := label[index]
			if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// repairURLRequiresRaw keeps a valid encoded or Unicode replacement exactly
// as submitted. Provider canonicalizers rebuild paths from decoded components,
// which can lose the encoding of spaces or non-ASCII segments.
func repairURLRequiresRaw(raw string) bool {
	if strings.Contains(raw, "%") {
		return true
	}
	return strings.IndexFunc(raw, func(character rune) bool { return character > unicode.MaxASCII }) >= 0
}

func validRepairPathSegment(segment string) bool {
	if segment == "" || segment[0] == '-' {
		return false
	}
	if segment == "." || segment == ".." {
		return false
	}
	for _, character := range segment {
		if unicode.IsControl(character) || character == '/' || character == '\\' ||
			character == '?' || character == '#' {
			return false
		}
	}
	return true
}

func (s *Service) validateBuiltInRepositoryRemoteURL(
	ctx context.Context, repository *models.Repository, candidate repositoryRepairCandidate,
) (repositoryRemoteURLValidation, error) {
	provider := strings.ToLower(strings.TrimSpace(repository.Provider))
	if err := validateBuiltInRepairShape(provider, candidate); err != nil {
		return repositoryRemoteURLValidation{}, err
	}
	if candidate.isSCP {
		if !savedOriginSupportsSCPRepair(repository, candidate.host) {
			return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
		}
	} else if !savedOriginMatchesRepairCandidate(repository, candidate) {
		return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
	}
	identitySource, identityScheme := candidate.raw, candidate.scheme
	if candidate.isSCP {
		identitySource, identityScheme = candidate.httpsEquivalent, protocolHTTPS
	}
	parsedProvider, owner, name, canonical, err := parseRemoteRepositoryURL(identitySource, provider)
	if err != nil || !strings.EqualFold(parsedProvider, provider) {
		return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
	}
	if err := validateBuiltInParsedIdentity(repository, candidate, provider, identityScheme, canonical, owner, name); err != nil {
		return repositoryRemoteURLValidation{}, err
	}
	originInput := TaskRepositoryInput{
		RemoteURL: identitySource, Provider: repository.Provider, ProviderHost: repository.ProviderHost,
		ProviderOwner: repository.ProviderOwner, ProviderName: repository.ProviderName,
	}
	if provider != providerAzureDevOps {
		if err := s.verifyRemoteRepositoryOrigin(ctx, repository.WorkspaceID, &originInput); err != nil {
			return repositoryRemoteURLValidation{}, err
		}
	}
	effective := canonical
	if candidate.isSCP || repairURLRequiresRaw(candidate.raw) {
		effective = candidate.raw
	}
	return repositoryRemoteURLValidation{
		preview: RepositoryRemoteURLPreview{
			EffectiveRemoteURL: effective,
			ValidationScope:    repositoryValidationScopeLocatorIdentity,
		},
	}, nil
}

func validateBuiltInParsedIdentity(
	repository *models.Repository, candidate repositoryRepairCandidate,
	provider, scheme, canonical, owner, name string,
) error {
	if provider == providerGitHub && scheme == protocolHTTP {
		return invalidRemoteURLSetting()
	}
	if provider == providerAzureDevOps && scheme != protocolHTTPS {
		return invalidRemoteURLSetting()
	}
	if err := validateSavedBuiltInIdentity(repository, provider, canonical, owner, name); err != nil {
		return err
	}
	if provider == providerAzureDevOps {
		return validateAzureRepairEvidence(repository, candidate, owner, name)
	}
	return nil
}

func validateBuiltInRepairShape(provider string, candidate repositoryRepairCandidate) error {
	switch provider {
	case providerGitHub:
		if len(candidate.segments) != 2 {
			return invalidRemoteURLSetting()
		}
	case providerGitLab:
		if len(candidate.segments) < 2 {
			return invalidRemoteURLSetting()
		}
	case providerAzureDevOps:
		if candidate.isSCP || len(candidate.segments) != 4 || candidate.segments[2] != azureGitPathSegment {
			return invalidRemoteURLSetting()
		}
	default:
		return invalidRemoteURLSetting()
	}
	return nil
}

func savedOriginSupportsSCPRepair(repository *models.Repository, scpHost string) bool {
	if strings.TrimSpace(repository.ProviderHost) == "" {
		return false
	}
	savedHost := normalizeProviderHost(repository.Provider, repository.ProviderHost)
	return savedHost != "" && savedHost == protocolHTTPS+"://"+strings.ToLower(scpHost)
}

func savedOriginMatchesRepairCandidate(repository *models.Repository, candidate repositoryRepairCandidate) bool {
	if strings.TrimSpace(repository.ProviderHost) == "" {
		return false
	}
	savedHost := normalizeProviderHost(repository.Provider, repository.ProviderHost)
	return savedHost != "" && savedHost == candidate.scheme+"://"+strings.ToLower(candidate.host)
}

func validateSavedBuiltInIdentity(
	repository *models.Repository, provider, canonical, owner, name string,
) error {
	if repository.ProviderOwner == "" || repository.ProviderName == "" {
		return invalidRemoteURLSetting()
	}
	if provider == providerGitHub {
		if !strings.EqualFold(repository.ProviderOwner, owner) || !strings.EqualFold(repository.ProviderName, name) {
			return invalidRemoteURLSetting()
		}
	} else if repository.ProviderOwner != owner || repository.ProviderName != name {
		return invalidRemoteURLSetting()
	}
	if strings.TrimSpace(repository.ProviderHost) == "" {
		return invalidRemoteURLSetting()
	}
	savedHost := normalizeProviderHost(repository.Provider, repository.ProviderHost)
	if savedHost == "" || savedHost != remoteIdentityOrigin(canonical) {
		return invalidRemoteURLSetting()
	}
	return nil
}

func remoteIdentityOrigin(canonical string) string {
	if strings.HasPrefix(canonical, "git@") {
		scp, err := gitlocator.ParseConventionalSCP(canonical)
		if err != nil {
			return ""
		}
		return protocolHTTPS + "://" + strings.ToLower(scp.Host)
	}
	schemeEnd := strings.Index(canonical, "://")
	if schemeEnd <= 0 {
		return ""
	}
	authority := canonical[schemeEnd+3:]
	if pathStart := strings.IndexByte(authority, '/'); pathStart >= 0 {
		authority = authority[:pathStart]
	}
	if authority == "" {
		return ""
	}
	return strings.ToLower(canonical[:schemeEnd+3] + authority)
}

func validateAzureRepairEvidence(
	repository *models.Repository, candidate repositoryRepairCandidate, owner, name string,
) error {
	if strings.TrimSpace(repository.ProviderHost) == "" {
		return invalidRemoteURLSetting()
	}
	savedHost := normalizeProviderHost(repository.Provider, repository.ProviderHost)
	if savedHost != "https://dev.azure.com" {
		return invalidRemoteURLSetting()
	}
	organization, legacyValid := azureOrganizationFromRemoteURL(repository.RemoteURL)
	if repository.RemoteURL != "" && !legacyValid {
		return invalidRemoteURLSetting()
	}
	if repository.ProviderScope != "" {
		if organization != "" && organization != repository.ProviderScope {
			return invalidRemoteURLSetting()
		}
		organization = repository.ProviderScope
	}
	if organization == "" || organization != candidate.segments[0] {
		return invalidRemoteURLSetting()
	}
	if repository.ProviderOwner != owner || repository.ProviderName != name {
		return invalidRemoteURLSetting()
	}
	return nil
}

func azureOrganizationFromRemoteURL(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	parsed, sshStyle, err := normalizeRemoteRepositoryURL(raw)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if sshStyle && host != "ssh.dev.azure.com" {
		return "", false
	}
	if !sshStyle && host != "dev.azure.com" {
		return "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if sshStyle {
		if len(parts) == 4 && parts[0] == "v3" {
			return parts[1], true
		}
		return "", false
	}
	if len(parts) == 4 && parts[2] == azureGitPathSegment {
		return parts[0], true
	}
	return "", false
}

func (s *Service) validatePluginRepositoryRemoteURL(
	ctx context.Context, repository *models.Repository, candidate repositoryRepairCandidate,
) (repositoryRemoteURLValidation, error) {
	provider := strings.TrimSpace(repository.Provider)
	if !completePluginRepairIdentity(repository) || candidate.isSCP || candidate.scheme != protocolHTTPS {
		return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
	}
	input := TaskRepositoryInput{
		RemoteURL: candidate.raw, Provider: provider, ProviderHost: repository.ProviderHost,
		ProviderScope: repository.ProviderScope, ProviderRepoID: repository.ProviderRepoID,
		ProviderOwner: repository.ProviderOwner, ProviderName: repository.ProviderName,
	}
	resolved, err := s.preflightRepositoryInput(ctx, repository.WorkspaceID, input)
	if err != nil {
		return repositoryRemoteURLValidation{}, err
	}
	if !pluginRepairIdentityMatches(repository, resolved) {
		return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
	}
	effective, err := parseRepositoryRepairCandidate(resolved.RemoteURL)
	if err != nil || effective.isSCP || effective.scheme != protocolHTTPS {
		return repositoryRemoteURLValidation{}, invalidRemoteURLSetting()
	}
	return repositoryRemoteURLValidation{
		preview: RepositoryRemoteURLPreview{
			EffectiveRemoteURL: resolved.RemoteURL,
			ValidationScope:    repositoryValidationScopeProviderInspection,
		},
	}, nil
}

func completePluginRepairIdentity(repository *models.Repository) bool {
	return strings.TrimSpace(repository.Provider) != "" && repository.ProviderHost != "" &&
		repository.ProviderOwner != "" && repository.ProviderName != "" && repository.ProviderRepoID != ""
}

func pluginRepairIdentityMatches(repository *models.Repository, resolved TaskRepositoryInput) bool {
	if !resolved.TrustedProviderDescriptor || !strings.EqualFold(resolved.Provider, repository.Provider) {
		return false
	}
	if !strings.EqualFold(strings.TrimRight(resolved.ProviderHost, "/"), strings.TrimRight(repository.ProviderHost, "/")) {
		return false
	}
	return resolved.ProviderScope == repository.ProviderScope &&
		resolved.ProviderRepoID == repository.ProviderRepoID &&
		resolved.ProviderOwner == repository.ProviderOwner &&
		resolved.ProviderName == repository.ProviderName
}

func validateRepositoryIdentityUpdateFields(repository *models.Repository, req *UpdateRepositoryRequest) error {
	return normalizeRepositoryIdentityUpdateFields(repository, req, true)
}

func normalizeRepositoryIdentityUpdateFields(
	repository *models.Repository, req *UpdateRepositoryRequest, rejectDifferent bool,
) error {
	if repository == nil || req == nil {
		return nil
	}
	for _, field := range []struct {
		name     string
		supplied *string
		saved    *string
	}{
		{name: "source_type", supplied: req.SourceType, saved: &repository.SourceType},
		{name: "local_path", supplied: req.LocalPath, saved: &repository.LocalPath},
		{name: "provider", supplied: req.Provider, saved: &repository.Provider},
		{name: "provider_repo_id", supplied: req.ProviderRepoID, saved: &repository.ProviderRepoID},
		{name: "provider_scope", supplied: req.ProviderScope, saved: &repository.ProviderScope},
		{name: "provider_owner", supplied: req.ProviderOwner, saved: &repository.ProviderOwner},
		{name: "provider_name", supplied: req.ProviderName, saved: &repository.ProviderName},
	} {
		if field.supplied == nil {
			continue
		}
		supplied := strings.TrimSpace(*field.supplied)
		if field.name == "provider_scope" {
			validated, err := validateProviderScope(supplied)
			if err != nil {
				return err
			}
			supplied = validated
		}
		if supplied != *field.saved {
			if rejectDifferent {
				return invalidRemoteURLSetting()
			}
			continue
		}
		*field.supplied = *field.saved
	}
	return normalizeEquivalentProviderHost(repository, req, rejectDifferent)
}

func normalizeEquivalentProviderHost(
	repository *models.Repository, req *UpdateRepositoryRequest, rejectDifferent bool,
) error {
	if req.ProviderHost == nil {
		return nil
	}
	provider := repository.Provider
	if req.Provider != nil {
		provider = strings.TrimSpace(*req.Provider)
	}
	if normalizeProviderHost(provider, *req.ProviderHost) != normalizeProviderHost(repository.Provider, repository.ProviderHost) {
		if rejectDifferent {
			return invalidRemoteURLSetting()
		}
		return nil
	}
	*req.ProviderHost = repository.ProviderHost
	return nil
}

func repositoryIdentityUpdateRequested(repository *models.Repository, req *UpdateRepositoryRequest) bool {
	if repository == nil || req == nil {
		return false
	}
	for _, field := range []struct {
		name     string
		supplied *string
		saved    string
	}{
		{name: "source_type", supplied: req.SourceType, saved: repository.SourceType},
		{name: "local_path", supplied: req.LocalPath, saved: repository.LocalPath},
		{name: "provider", supplied: req.Provider, saved: repository.Provider},
		{name: "provider_repo_id", supplied: req.ProviderRepoID, saved: repository.ProviderRepoID},
		{name: "provider_scope", supplied: req.ProviderScope, saved: repository.ProviderScope},
		{name: "provider_owner", supplied: req.ProviderOwner, saved: repository.ProviderOwner},
		{name: "provider_name", supplied: req.ProviderName, saved: repository.ProviderName},
	} {
		if field.supplied == nil {
			continue
		}
		supplied := strings.TrimSpace(*field.supplied)
		if field.name == "provider_scope" {
			validated, err := validateProviderScope(supplied)
			if err != nil {
				return true
			}
			supplied = validated
		}
		if supplied != field.saved {
			return true
		}
	}
	if req.ProviderHost != nil {
		provider := repository.Provider
		if req.Provider != nil {
			provider = strings.TrimSpace(*req.Provider)
		}
		if normalizeProviderHost(provider, *req.ProviderHost) != normalizeProviderHost(repository.Provider, repository.ProviderHost) {
			return true
		}
	}
	return false
}

func displayableRemoteURL(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "git@") {
		if _, err := gitlocator.ParseConventionalSCP(raw); err != nil {
			return "", false
		}
		return raw, true
	}
	if strings.Contains(raw, "%") {
		if _, err := parseRepositoryRepairCandidate(raw); err != nil {
			return "", false
		}
		return raw, true
	}
	if strings.IndexFunc(raw, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}) >= 0 {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if !displayableURLUser(parsed.Scheme, parsed.User) {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case protocolHTTPS, protocolHTTP, protocolSSH, "git":
		return raw, true
	default:
		return "", false
	}
}

func displayableURLUser(scheme string, user *url.Userinfo) bool {
	if user == nil {
		return true
	}
	if !strings.EqualFold(scheme, protocolSSH) || user.Username() != gitSSHUser {
		return false
	}
	_, hasPassword := user.Password()
	return !hasPassword
}

func (s *Service) applyRepositoryRemoteURLUpdate(
	ctx context.Context, repository *models.Repository, req *UpdateRepositoryRequest,
) error {
	if req == nil || req.RemoteURL == nil {
		return nil
	}
	if repository == nil {
		return repoerrors.ErrRepositoryNotFound
	}
	if err := s.refuseReadOnlyRepositoryWorkspace(ctx, repository.WorkspaceID); err != nil {
		return err
	}
	if repositoryIdentityUpdateRequested(repository, req) {
		return s.applyFullRepositoryRemoteURLValidation(ctx, repository, req)
	}
	return s.applyRepositoryRemoteURLValidation(ctx, repository, req)
}

// applyFullRepositoryRemoteURLValidation handles a deliberate full repository
// redefinition. Complete provider identity fields supplied by the caller are
// validated against the proposed clone URL, while the write fence remains the
// saved row that this update originally read.
func (s *Service) applyFullRepositoryRemoteURLValidation(
	ctx context.Context, repository *models.Repository, req *UpdateRepositoryRequest,
) error {
	if err := normalizeRepositoryIdentityUpdateFields(repository, req, false); err != nil {
		return err
	}
	proposed, err := proposedRepositoryIdentity(repository, req)
	if err != nil {
		return err
	}
	if proposed.Provider == "" {
		if proposed.SourceType != sourceTypeLocal {
			return invalidRemoteURLSetting()
		}
		effective := strings.TrimSpace(*req.RemoteURL)
		req.RemoteURL = &effective
		req.ObservedIdentity = models.NewRepositoryIdentityFence(repository)
		return nil
	}
	candidate, err := parseRepositoryRepairCandidate(*req.RemoteURL)
	if err != nil {
		return invalidRemoteURLSetting()
	}
	validation, err := s.validateRepositoryRemoteURL(ctx, proposed, candidate.raw)
	if err != nil {
		return err
	}
	effective := validation.preview.EffectiveRemoteURL
	req.RemoteURL = &effective
	req.ObservedIdentity = models.NewRepositoryIdentityFence(repository)
	return nil
}

func proposedRepositoryIdentity(
	repository *models.Repository, req *UpdateRepositoryRequest,
) (*models.Repository, error) {
	proposed := *repository
	if req.SourceType != nil {
		proposed.SourceType = strings.TrimSpace(*req.SourceType)
	}
	if req.LocalPath != nil {
		proposed.LocalPath = strings.TrimSpace(*req.LocalPath)
	}
	if req.Provider != nil {
		proposed.Provider = strings.TrimSpace(*req.Provider)
	}
	if req.ProviderRepoID != nil {
		proposed.ProviderRepoID = strings.TrimSpace(*req.ProviderRepoID)
	}
	if req.ProviderHost != nil {
		proposed.ProviderHost = normalizeProviderHost(proposed.Provider, *req.ProviderHost)
	}
	if req.ProviderScope != nil {
		scope, err := validateProviderScope(*req.ProviderScope)
		if err != nil {
			return nil, err
		}
		proposed.ProviderScope = scope
	}
	if req.ProviderOwner != nil {
		proposed.ProviderOwner = strings.TrimSpace(*req.ProviderOwner)
	}
	if req.ProviderName != nil {
		proposed.ProviderName = strings.TrimSpace(*req.ProviderName)
	}
	return &proposed, nil
}

// applyRepositoryRemoteURLValidation copies a validated replacement and its
// observed identity fence into the ordinary write request. It is only called
// after authorization and read-only refusal.
func (s *Service) applyRepositoryRemoteURLValidation(
	ctx context.Context, repository *models.Repository, req *UpdateRepositoryRequest,
) error {
	if req == nil || req.RemoteURL == nil {
		return nil
	}
	if repository.SourceType == sourceTypeLocal && strings.TrimSpace(repository.Provider) == "" {
		effective := strings.TrimSpace(*req.RemoteURL)
		req.RemoteURL = &effective
		req.ObservedIdentity = models.NewRepositoryIdentityFence(repository)
		return nil
	}
	if err := validateRepositoryIdentityUpdateFields(repository, req); err != nil {
		return err
	}
	validation, err := s.validateRepositoryRemoteURL(ctx, repository, *req.RemoteURL)
	if err != nil {
		return err
	}
	effective := validation.preview.EffectiveRemoteURL
	req.RemoteURL = &effective
	req.ObservedIdentity = validation.observedIdentity
	return nil
}
