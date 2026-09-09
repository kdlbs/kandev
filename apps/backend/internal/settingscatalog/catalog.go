// Package settingscatalog owns the process-wide metadata contract for agent
// accessible settings. It deliberately does not read persistence or call a
// domain service. Domain adapters provide the values and mutations.
package settingscatalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	MaxSearchQueryLength = 256
	MaxSearchResults     = 50
	DefaultSearchLimit   = 20
	MaxChangedFields     = 50
)

type SupportStatus string

const (
	SupportSupported   SupportStatus = "supported"
	SupportReadOnly    SupportStatus = "read_only"
	SupportException   SupportStatus = "exception"
	SupportPending     SupportStatus = "pending"
	SupportUnavailable SupportStatus = "unavailable"
)

type Classification string

const (
	ClassificationWritable  Classification = "writable"
	ClassificationComputed  Classification = "computed"
	ClassificationAction    Classification = "action"
	ClassificationException Classification = "exception"
)

type Scope string

const (
	ScopeCaller       Scope = "caller"
	ScopeWorkspace    Scope = "workspace"
	ScopeResource     Scope = "resource"
	ScopeInstallation Scope = "installation"
)

type MatchReason string

const (
	MatchExact       MatchReason = "exact"
	MatchPrefix      MatchReason = "prefix"
	MatchAlias       MatchReason = "alias"
	MatchDescription MatchReason = "description"
	MatchDomain      MatchReason = "domain"
)

type ResourceTarget struct {
	ResourceType string  `json:"resource_type"`
	ResourceID   *string `json:"resource_id,omitempty"`
	WorkspaceID  *string `json:"workspace_id,omitempty"`
}

type TargetRules struct {
	Scope               Scope `json:"scope"`
	Singleton           bool  `json:"singleton,omitempty"`
	RequiresResourceID  bool  `json:"requires_resource_id,omitempty"`
	RequiresWorkspaceID bool  `json:"requires_workspace_id,omitempty"`
	AllowsWorkspaceID   bool  `json:"allows_workspace_id,omitempty"`
}

type FieldDescriptor struct {
	Key             string         `json:"key"`
	FieldPath       string         `json:"field_path"`
	Label           string         `json:"label"`
	Description     string         `json:"description"`
	Aliases         []string       `json:"aliases,omitempty"`
	JSONType        string         `json:"json_type"`
	Nullable        bool           `json:"nullable,omitempty"`
	Support         SupportStatus  `json:"support"`
	Classification  Classification `json:"classification"`
	Owner           string         `json:"owner"`
	Writable        bool           `json:"writable,omitempty"`
	Validator       string         `json:"validator,omitempty"`
	Authority       string         `json:"authority,omitempty"`
	Sensitive       bool           `json:"sensitive,omitempty"`
	Replacement     bool           `json:"replacement,omitempty"`
	DefaultBehavior string         `json:"default_behavior,omitempty"`
	ChangeTiming    string         `json:"change_timing,omitempty"`
	Dependencies    []string       `json:"dependencies,omitempty"`
	SettingsHref    string         `json:"settings_href,omitempty"`
	ExceptionReason string         `json:"exception_reason,omitempty"`
	Recovery        string         `json:"recovery,omitempty"`
}

type OperationDescriptor struct {
	Name        string         `json:"name"`
	Binding     string         `json:"binding"`
	Authority   string         `json:"authority,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	Description string         `json:"description,omitempty"`
}

type DomainDescriptor struct {
	Domain       string                `json:"domain"`
	ResourceType string                `json:"resource_type"`
	Label        string                `json:"label"`
	Description  string                `json:"description"`
	Owner        string                `json:"owner"`
	Scope        Scope                 `json:"scope"`
	Target       TargetRules           `json:"target"`
	Fields       []FieldDescriptor     `json:"fields"`
	Operations   []OperationDescriptor `json:"operations,omitempty"`
	SettingsHref string                `json:"settings_href,omitempty"`
}

type Registry struct {
	domainsByType map[string]DomainDescriptor
	fieldsByKey   map[string]FieldDescriptor
	domainByField map[string]DomainDescriptor
	fields        []FieldDescriptor
}

type SearchRequest struct {
	Query        string
	Domain       string
	ResourceType string
	Limit        int
	Cursor       string
}

type SearchItem struct {
	Key          string        `json:"key"`
	Label        string        `json:"label"`
	Description  string        `json:"description"`
	ResourceType string        `json:"resource_type"`
	FieldPath    string        `json:"field_path"`
	Support      SupportStatus `json:"support"`
	MatchReason  MatchReason   `json:"match_reason"`
	SettingsHref string        `json:"settings_href,omitempty"`
}

type SearchResponse struct {
	Items          []SearchItem `json:"items"`
	NextCursor     string       `json:"next_cursor,omitempty"`
	CoveredDomains []string     `json:"covered_domains"`
}

func NewRegistry(domains []DomainDescriptor) (*Registry, error) {
	registry := &Registry{
		domainsByType: make(map[string]DomainDescriptor, len(domains)),
		fieldsByKey:   make(map[string]FieldDescriptor),
		domainByField: make(map[string]DomainDescriptor),
	}
	for _, domain := range domains {
		if err := validateDomain(domain); err != nil {
			return nil, err
		}
		if _, exists := registry.domainsByType[domain.ResourceType]; exists {
			return nil, fmt.Errorf("duplicate resource type %q", domain.ResourceType)
		}
		registry.domainsByType[domain.ResourceType] = cloneDomain(domain)
		for _, field := range domain.Fields {
			if _, exists := registry.fieldsByKey[field.Key]; exists {
				return nil, fmt.Errorf("duplicate setting key %q", field.Key)
			}
			registry.fieldsByKey[field.Key] = cloneField(field)
			registry.domainByField[field.Key] = registry.domainsByType[domain.ResourceType]
			registry.fields = append(registry.fields, cloneField(field))
		}
	}
	sort.Slice(registry.fields, func(i, j int) bool { return registry.fields[i].Key < registry.fields[j].Key })
	return registry, nil
}

//nolint:cyclop // Catalog validation checks independent target and field invariants in one pass.
func validateDomain(domain DomainDescriptor) error {
	if strings.TrimSpace(domain.Domain) == "" {
		return fmt.Errorf("domain owner is required for %q", domain.ResourceType)
	}
	if strings.TrimSpace(domain.ResourceType) == "" {
		return fmt.Errorf("resource type is required")
	}
	if strings.TrimSpace(domain.Owner) == "" {
		return fmt.Errorf("owner is required for resource type %q", domain.ResourceType)
	}
	if domain.Target.Singleton && (domain.Target.RequiresResourceID || domain.Target.RequiresWorkspaceID) {
		return fmt.Errorf("singleton target %q cannot require selectors", domain.ResourceType)
	}
	if domain.Target.RequiresWorkspaceID && domain.Target.RequiresResourceID && !domain.Target.AllowsWorkspaceID {
		return fmt.Errorf("resource target %q must explicitly allow workspace selector", domain.ResourceType)
	}
	seenPaths := make(map[string]struct{}, len(domain.Fields))
	for _, field := range domain.Fields {
		if strings.TrimSpace(field.Key) == "" || strings.TrimSpace(field.FieldPath) == "" {
			return fmt.Errorf("setting key and field path are required for resource type %q", domain.ResourceType)
		}
		if strings.TrimSpace(field.Owner) == "" {
			return fmt.Errorf("setting %q is missing an owner", field.Key)
		}
		if _, exists := seenPaths[field.FieldPath]; exists {
			return fmt.Errorf("duplicate field path %q in resource type %q", field.FieldPath, domain.ResourceType)
		}
		seenPaths[field.FieldPath] = struct{}{}
		if field.Writable {
			if field.Support != SupportSupported || field.Classification != ClassificationWritable ||
				strings.TrimSpace(field.Validator) == "" || strings.TrimSpace(field.Authority) == "" {
				return fmt.Errorf("writable setting %q requires supported classification, validator, and authority", field.Key)
			}
		}
		if field.Support == SupportException && strings.TrimSpace(field.ExceptionReason) == "" {
			return fmt.Errorf("exception setting %q requires a reason", field.Key)
		}
	}
	return nil
}

//nolint:cyclop // Search validates bounds, ranks fields, and applies stable pagination as one operation.
func (r *Registry) Search(request SearchRequest) (SearchResponse, error) {
	query := strings.TrimSpace(request.Query)
	if len([]rune(query)) > MaxSearchQueryLength {
		return SearchResponse{}, fmt.Errorf("query exceeds %d characters", MaxSearchQueryLength)
	}
	limit, err := boundedLimit(request.Limit)
	if err != nil {
		return SearchResponse{}, err
	}
	offset, err := parseCursor(request.Cursor)
	if err != nil {
		return SearchResponse{}, err
	}
	queryTokens := tokenize(query)
	type ranked struct {
		item  SearchItem
		score int
	}
	rankedItems := make([]ranked, 0, len(r.fields))
	for _, field := range r.fields {
		domain, ok := r.domainForField(field.Key)
		if !ok || (request.Domain != "" && request.Domain != domain.Domain) ||
			(request.ResourceType != "" && request.ResourceType != domain.ResourceType) {
			continue
		}
		reason, score, matched := matchField(field, domain, query, queryTokens)
		if !matched {
			continue
		}
		rankedItems = append(rankedItems, ranked{item: SearchItem{
			Key:          field.Key,
			Label:        field.Label,
			Description:  field.Description,
			ResourceType: domain.ResourceType,
			FieldPath:    field.FieldPath,
			Support:      field.Support,
			MatchReason:  reason,
			SettingsHref: field.SettingsHref,
		}, score: score})
	}
	sort.SliceStable(rankedItems, func(i, j int) bool {
		if rankedItems[i].score != rankedItems[j].score {
			return rankedItems[i].score > rankedItems[j].score
		}
		return rankedItems[i].item.Key < rankedItems[j].item.Key
	})
	response := SearchResponse{CoveredDomains: r.CoveredDomains()}
	if offset >= len(rankedItems) {
		return response, nil
	}
	end := offset + limit
	if end > len(rankedItems) {
		end = len(rankedItems)
	}
	response.Items = make([]SearchItem, 0, end-offset)
	for _, match := range rankedItems[offset:end] {
		response.Items = append(response.Items, match.item)
	}
	if end < len(rankedItems) {
		response.NextCursor = strconv.Itoa(end)
	}
	return response, nil
}

func (r *Registry) Find(key string) (FieldDescriptor, DomainDescriptor, bool) {
	field, ok := r.fieldsByKey[key]
	if !ok {
		return FieldDescriptor{}, DomainDescriptor{}, false
	}
	domain, ok := r.domainForField(key)
	return field, domain, ok
}

func (r *Registry) Domain(resourceType string) (DomainDescriptor, bool) {
	domain, ok := r.domainsByType[resourceType]
	return cloneDomain(domain), ok
}

func (r *Registry) Domains() []DomainDescriptor {
	domains := make([]DomainDescriptor, 0, len(r.domainsByType))
	for _, domain := range r.domainsByType {
		domains = append(domains, cloneDomain(domain))
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].ResourceType < domains[j].ResourceType })
	return domains
}

func (r *Registry) CoveredDomains() []string {
	domains := r.Domains()
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		result = append(result, domain.Domain)
	}
	return result
}

//nolint:cyclop // Target validation applies the complete singleton and selector contract.
func (r *Registry) ValidateTarget(target ResourceTarget) error {
	if strings.TrimSpace(target.ResourceType) == "" {
		return fmt.Errorf("resource_type is required")
	}
	domain, ok := r.domainsByType[target.ResourceType]
	if !ok {
		return fmt.Errorf("unknown resource type %q", target.ResourceType)
	}
	if domain.Target.Singleton && (target.ResourceID != nil || target.WorkspaceID != nil) {
		return fmt.Errorf("resource type %q does not accept resource selectors", target.ResourceType)
	}
	if domain.Target.RequiresResourceID && (target.ResourceID == nil || strings.TrimSpace(*target.ResourceID) == "") {
		return fmt.Errorf("resource_id is required for %q", target.ResourceType)
	}
	if !domain.Target.RequiresResourceID && target.ResourceID != nil {
		return fmt.Errorf("resource type %q does not accept resource_id", target.ResourceType)
	}
	if domain.Target.RequiresWorkspaceID && (target.WorkspaceID == nil || strings.TrimSpace(*target.WorkspaceID) == "") {
		return fmt.Errorf("workspace_id is required for %q", target.ResourceType)
	}
	if !domain.Target.AllowsWorkspaceID && !domain.Target.RequiresWorkspaceID && target.WorkspaceID != nil {
		return fmt.Errorf("resource type %q does not accept workspace_id", target.ResourceType)
	}
	return nil
}

func (r *Registry) domainForField(key string) (DomainDescriptor, bool) {
	domain, ok := r.domainByField[key]
	return domain, ok
}

func matchField(field FieldDescriptor, domain DomainDescriptor, query string, queryTokens []string) (MatchReason, int, bool) {
	if query == "" {
		return MatchDomain, 1, true
	}
	normalizedQuery := strings.ToLower(query)
	key := strings.ToLower(field.Key)
	label := strings.ToLower(field.Label)
	if normalizedQuery == key || normalizedQuery == label {
		return MatchExact, 1000, true
	}
	if strings.HasPrefix(key, normalizedQuery) || strings.HasPrefix(label, normalizedQuery) {
		return MatchPrefix, 800, true
	}
	for _, alias := range field.Aliases {
		if strings.HasPrefix(strings.ToLower(alias), normalizedQuery) {
			return MatchAlias, 600, true
		}
	}
	metadata := strings.ToLower(strings.Join(append([]string{field.Description, domain.Description}, field.Aliases...), " "))
	for _, token := range queryTokens {
		if !strings.Contains(metadata, token) {
			return "", 0, false
		}
	}
	if len(queryTokens) > 0 {
		return MatchDescription, 400, true
	}
	return "", 0, false
}

func tokenize(value string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			current.WriteRune(char)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func boundedLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultSearchLimit, nil
	}
	if limit < 1 || limit > MaxSearchResults {
		return 0, fmt.Errorf("limit must be between 1 and %d", MaxSearchResults)
	}
	return limit, nil
}

func parseCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("cursor must be a non-negative offset")
	}
	return offset, nil
}

func cloneField(field FieldDescriptor) FieldDescriptor {
	field.Aliases = append([]string(nil), field.Aliases...)
	field.Dependencies = append([]string(nil), field.Dependencies...)
	return field
}

func cloneDomain(domain DomainDescriptor) DomainDescriptor {
	fields := domain.Fields
	domain.Fields = make([]FieldDescriptor, len(fields))
	for i, field := range fields {
		domain.Fields[i] = cloneField(field)
	}
	domain.Operations = append([]OperationDescriptor(nil), domain.Operations...)
	return domain
}
