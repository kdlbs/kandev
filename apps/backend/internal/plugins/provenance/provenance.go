// Package provenance contains the host-owned publisher evidence carried by
// plugin catalog entries and installation records.
//
// Package metadata can describe a release, but it cannot make that release
// trusted. Callers must validate the evidence at the trust boundary before
// projecting it into PublisherIdentity or persisting it on an installation.
package provenance

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	StatusVerified   = "verified"
	StatusUnverified = "unverified"

	OriginUnknown  = "unknown"
	OriginCatalog  = "catalog"
	OriginURL      = "url"
	OriginUpload   = "upload"
	OriginSideload = "sideload"

	VerificationArchiveDownload = "archive_download"
	VerificationInstalledFiles  = "installed_files"
)

var decimalID = regexp.MustCompile(`^[0-9]+$`)
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Evidence is the registry-attested identity for one release. The registry
// emits it separately from package presentation metadata. PackageSHA256 is
// kept optional here because catalog JSON carries the digest beside the
// publisher object; installation records always copy the digest into their
// host-owned provenance tuple.
type Evidence struct {
	SchemaVersion int    `json:"schema_version" yaml:"schema_version"`
	RepositoryID  string `json:"repository_id" yaml:"repository_id"`
	OwnerID       string `json:"owner_id" yaml:"owner_id"`
	Login         string `json:"login" yaml:"login"`
	Repository    string `json:"repository" yaml:"repository"`
	Official      bool   `json:"official" yaml:"official"`
	PackageSHA256 string `json:"package_sha256,omitempty" yaml:"package_sha256,omitempty"`
}

// PublisherIdentity is the safe presentation projection returned by the
// backend. An unverified identity deliberately contains no publisher name.
type PublisherIdentity struct {
	Status        string     `json:"status" yaml:"status"`
	RepositoryID  string     `json:"repository_id,omitempty" yaml:"repository_id,omitempty"`
	OwnerID       string     `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
	Login         string     `json:"login,omitempty" yaml:"login,omitempty"`
	Repository    string     `json:"repository,omitempty" yaml:"repository,omitempty"`
	Official      bool       `json:"official,omitempty" yaml:"official,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
	MatchedSource string     `json:"matched_source,omitempty" yaml:"matched_source,omitempty"`
}

// InstallationProvenance is host-owned metadata attached to a stored
// installation. Origin describes how bytes entered the host. Publisher and
// verification fields describe a separate later trust decision.
type InstallationProvenance struct {
	Origin             string     `json:"origin" yaml:"origin"`
	SourceID           string     `json:"source_id,omitempty" yaml:"source_id,omitempty"`
	SourceURL          string     `json:"source_url,omitempty" yaml:"source_url,omitempty"`
	PackageID          string     `json:"package_id" yaml:"package_id"`
	Version            string     `json:"version" yaml:"version"`
	PackageSHA256      string     `json:"package_sha256,omitempty" yaml:"package_sha256,omitempty"`
	Publisher          *Evidence  `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
	VerificationMethod string     `json:"verification_method,omitempty" yaml:"verification_method,omitempty"`
	MatchedSourceID    string     `json:"matched_source_id,omitempty" yaml:"matched_source_id,omitempty"`
	MatchedSourceURL   string     `json:"matched_source_url,omitempty" yaml:"matched_source_url,omitempty"`
}

// SanitizePublicURL removes transport-only URL components before a URL is
// stored or projected in public installation provenance. Credentials, signed
// query parameters, and fragments belong to the download request and must
// never become part of a plugin record.
func SanitizePublicURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// SanitizePublicURLs removes transport-only components from all URL fields in
// an installation provenance record. It is also applied when legacy records
// are read so previously persisted credentials and signed tokens are not
// exposed by list/get responses.
func (p *InstallationProvenance) SanitizePublicURLs() {
	if p == nil {
		return
	}
	p.SourceURL = SanitizePublicURL(p.SourceURL)
	p.MatchedSourceURL = SanitizePublicURL(p.MatchedSourceURL)
}

func (e Evidence) Validate() error {
	if e.SchemaVersion != 1 {
		return fmt.Errorf("unsupported publisher evidence schema version %d", e.SchemaVersion)
	}
	if err := validateEvidenceID("repository id", e.RepositoryID); err != nil {
		return err
	}
	if err := validateEvidenceID("owner id", e.OwnerID); err != nil {
		return err
	}
	if err := validateEvidenceWhitespace(e); err != nil {
		return err
	}
	if e.Login == "" {
		return errors.New("publisher evidence login is missing")
	}
	if err := validateEvidenceRepository(e.Repository, e.Login); err != nil {
		return err
	}
	if err := validateEvidenceDigest(e.PackageSHA256); err != nil {
		return err
	}
	return nil
}

func validateEvidenceID(name, value string) error {
	if hasSurroundingWhitespace(value) || !decimalID.MatchString(value) {
		return fmt.Errorf("publisher evidence %s is invalid", name)
	}
	return nil
}

func validateEvidenceWhitespace(e Evidence) error {
	for _, value := range []string{e.Login, e.Repository, e.PackageSHA256} {
		if hasSurroundingWhitespace(value) {
			return errors.New("publisher evidence contains surrounding whitespace")
		}
	}
	return nil
}

func validateEvidenceRepository(repository, login string) error {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return errors.New("publisher evidence repository is invalid")
	}
	if parts[0] == "" || parts[1] == "" || strings.ContainsAny(repository, "?#") {
		return errors.New("publisher evidence repository is invalid")
	}
	if !strings.EqualFold(parts[0], login) {
		return errors.New("publisher evidence repository owner does not match login")
	}
	return nil
}

func validateEvidenceDigest(digest string) error {
	if digest != "" && !sha256Hex.MatchString(strings.ToLower(digest)) {
		return errors.New("publisher evidence package digest is invalid")
	}
	return nil
}

func hasSurroundingWhitespace(value string) bool {
	return value != strings.TrimSpace(value)
}

// Validate checks a complete installed provenance tuple. It is intentionally
// strict for verified evidence and permissive for old/unverified records.
func (p *InstallationProvenance) Validate() error {
	if p == nil {
		return nil
	}
	if !validOrigin(p.Origin) {
		return fmt.Errorf("unknown installation provenance origin %q", p.Origin)
	}
	if p.Publisher == nil {
		return p.validateUnverified()
	}
	return p.validateVerified()
}

func (p *InstallationProvenance) validateUnverified() error {
	if p.VerifiedAt != nil || p.VerificationMethod != "" {
		return errors.New("unverified provenance cannot have verification metadata")
	}
	if p.PackageSHA256 != "" && !validSHA256(p.PackageSHA256) {
		return errors.New("installation package digest is invalid")
	}
	return nil
}

func (p *InstallationProvenance) validateVerified() error {
	if err := p.Publisher.Validate(); err != nil {
		return err
	}
	if p.PackageID == "" || p.Version == "" {
		return errors.New("verified provenance package identity is incomplete")
	}
	if !validSHA256(p.PackageSHA256) {
		return errors.New("verified provenance package digest is invalid")
	}
	if p.Publisher.PackageSHA256 != "" && !strings.EqualFold(p.Publisher.PackageSHA256, p.PackageSHA256) {
		return errors.New("publisher evidence digest does not match installation digest")
	}
	if p.VerifiedAt == nil {
		return errors.New("verified provenance timestamp is missing")
	}
	switch p.VerificationMethod {
	case VerificationArchiveDownload, VerificationInstalledFiles:
	default:
		return fmt.Errorf("unknown publisher verification method %q", p.VerificationMethod)
	}
	return nil
}

// Identity returns the validated publisher projection, or an empty
// unverified projection when the record is missing or malformed.
func (p *InstallationProvenance) Identity() *PublisherIdentity {
	if p == nil || p.Publisher == nil || p.Validate() != nil {
		return &PublisherIdentity{Status: StatusUnverified}
	}
	return &PublisherIdentity{
		Status:        StatusVerified,
		RepositoryID:  p.Publisher.RepositoryID,
		OwnerID:       p.Publisher.OwnerID,
		Login:         p.Publisher.Login,
		Repository:    p.Publisher.Repository,
		Official:      p.Publisher.Official,
		VerifiedAt:    cloneTime(p.VerifiedAt),
		MatchedSource: firstNonEmpty(p.MatchedSourceID, p.SourceID),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func validOrigin(origin string) bool {
	switch origin {
	case "", OriginUnknown, OriginCatalog, OriginURL, OriginUpload, OriginSideload:
		return true
	default:
		return false
	}
}

func validSHA256(value string) bool {
	return sha256Hex.MatchString(strings.TrimSpace(value))
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// Clone returns a deep copy suitable for registry snapshots.
func (p *InstallationProvenance) Clone() *InstallationProvenance {
	if p == nil {
		return nil
	}
	clone := *p
	clone.VerifiedAt = cloneTime(p.VerifiedAt)
	if p.Publisher != nil {
		publisher := *p.Publisher
		clone.Publisher = &publisher
	}
	clone.SanitizePublicURLs()
	return &clone
}

// NewUnverified returns a stable presentation projection for legacy and
// direct installations.
func NewUnverified() *PublisherIdentity {
	return &PublisherIdentity{Status: StatusUnverified}
}
