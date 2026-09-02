package registry

import (
	"fmt"
	"strings"
	"time"
)

// Status describes the lifecycle state published by a Registry.
type Status string

const (
	StatusActive     Status = "active"
	StatusDeprecated Status = "deprecated"
	StatusDeleted    Status = "deleted"
)

// Repository describes the publisher's source repository.
type Repository struct {
	URL       string `json:"url,omitempty"`
	Source    string `json:"source,omitempty"`
	ID        string `json:"id,omitempty"`
	Subfolder string `json:"subfolder,omitempty"`
}

// Icon contains untrusted display metadata from a Registry entry.
type Icon struct {
	Src      string   `json:"src,omitempty"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

// Argument represents a Registry runtime or package argument.
type Argument struct {
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
}

// KeyValueInput represents a Registry environment variable or header input.
type KeyValueInput struct {
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
	IsRequired  bool   `json:"isRequired,omitempty"`
	IsSecret    bool   `json:"isSecret,omitempty"`
}

// Transport is the transport object nested in a Registry package or remote.
type Transport struct {
	Type      string                   `json:"type,omitempty"`
	URL       string                   `json:"url,omitempty"`
	SSEURL    string                   `json:"sseUrl,omitempty"`
	Headers   []KeyValueInput          `json:"headers,omitempty"`
	Variables map[string]KeyValueInput `json:"variables,omitempty"`
}

// Package is a publisher-supplied package choice.
type Package struct {
	RegistryType         string          `json:"registryType,omitempty"`
	RegistryBaseURL      string          `json:"registryBaseUrl,omitempty"`
	Identifier           string          `json:"identifier,omitempty"`
	Version              string          `json:"version,omitempty"`
	FileSHA256           string          `json:"fileSha256,omitempty"`
	RuntimeHint          string          `json:"runtimeHint,omitempty"`
	Transport            Transport       `json:"transport"`
	RuntimeArguments     []Argument      `json:"runtimeArguments,omitempty"`
	PackageArguments     []Argument      `json:"packageArguments,omitempty"`
	EnvironmentVariables []KeyValueInput `json:"environmentVariables,omitempty"`
}

// Remote is a publisher-supplied remote endpoint choice.
type Remote struct {
	Type      string                   `json:"type,omitempty"`
	URL       string                   `json:"url,omitempty"`
	SSEURL    string                   `json:"sseUrl,omitempty"`
	Headers   []KeyValueInput          `json:"headers,omitempty"`
	Variables map[string]KeyValueInput `json:"variables,omitempty"`
}

// Entry is normalized discovery metadata from the curated catalog or Registry.
type Entry struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Title             string         `json:"title,omitempty"`
	Version           string         `json:"version"`
	WebsiteURL        string         `json:"websiteUrl,omitempty"`
	Repository        *Repository    `json:"repository,omitempty"`
	Icons             []Icon         `json:"icons,omitempty"`
	Packages          []Package      `json:"packages,omitempty"`
	Remotes           []Remote       `json:"remotes,omitempty"`
	Status            Status         `json:"status"`
	StatusMessage     string         `json:"status_message,omitempty"`
	PublisherMetadata map[string]any `json:"publisher_metadata,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Revision          int64          `json:"revision"`
	UpdatedAt         time.Time      `json:"updated_at,omitempty"`
}

func (e Entry) Identity() string {
	if strings.TrimSpace(e.Version) == "" {
		return e.Name
	}
	return fmt.Sprintf("%s@%s", e.Name, e.Version)
}

// Choice describes one explicit package or remote installation option.
type Choice struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	RegistryType      string `json:"registry_type,omitempty"`
	Identifier        string `json:"identifier,omitempty"`
	Version           string `json:"version,omitempty"`
	Transport         string `json:"transport,omitempty"`
	URL               string `json:"url,omitempty"`
	Selectable        bool   `json:"selectable"`
	UnsupportedReason string `json:"unsupported_reason,omitempty"`
}

// SyncState is the durable health and freshness state of the public cache.
type SyncState struct {
	LastSuccessfulAt time.Time `json:"last_successful_at,omitempty"`
	LastAttemptAt    time.Time `json:"last_attempt_at,omitempty"`
	UpdatedSince     time.Time `json:"updated_since,omitempty"`
	Degraded         bool      `json:"degraded"`
	LastError        string    `json:"last_error,omitempty"`
}

// SearchResult is the marketplace response with cache health attached.
type SearchResult struct {
	Entries          []MarketplaceEntry `json:"entries"`
	Stale            bool               `json:"stale"`
	Degraded         bool               `json:"degraded"`
	LastSuccessfulAt time.Time          `json:"last_successful_at,omitempty"`
}

// MarketplaceEntry is a discovery entry with source and install choices.
type MarketplaceEntry struct {
	Entry
	Source            string   `json:"source"`
	PublisherSupplied bool     `json:"publisher_supplied"`
	TrustNotice       string   `json:"trust_notice"`
	Choices           []Choice `json:"choices"`
}

// InstallRequest selects one reviewed cached choice for workspace installation.
type InstallRequest struct {
	WorkspaceID      string
	Identity         string
	ExpectedRevision int64
	ChoiceID         string
	RuntimeName      string
	DisplayName      string
	SecretBindings   []SecretBindingInput
}

// SecretBindingInput carries a workspace secret reference selected during review.
type SecretBindingInput struct {
	InputName string
	SecretID  string
}

// ErrRegistryEntryNotFound indicates a cache miss.
var ErrRegistryEntryNotFound = fmt.Errorf("registry entry not found")

// ErrMarketplaceCatalogUnavailable indicates that installation needs catalog storage.
var ErrMarketplaceCatalogUnavailable = fmt.Errorf("marketplace catalog unavailable")
