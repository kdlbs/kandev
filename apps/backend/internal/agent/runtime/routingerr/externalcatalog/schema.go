// Package externalcatalog loads an operator-supplied provider-error rule
// catalog from a local JSON or YAML file and compiles it into an immutable,
// ordered rule set.
//
// The package owns only the file format, validation and matching of the
// catalog. It does not import the classifier: the caller supplies the
// accepted vocabulary (codes, confidences, phases) and maps a Decision back
// into its own types, so built-in rules and their precedence stay owned by
// the classifier.
package externalcatalog

// SchemaVersion is the only catalog document version this package accepts.
const SchemaVersion = 1

// Scope values a rule may request. Widening beyond ScopePathModel is applied
// only when the observation carries the matching identity.
const (
	ScopePathModel = "path_model"
	ScopeModel     = "model"
	ScopeAccount   = "account"
)

// Reset values a rule may request. ResetInherit keeps the classifier's
// existing reset-hint handling.
const (
	ResetInherit          = "inherit"
	ResetFreeDaily        = "free_daily"
	ResetUntilModelChange = "until_model_change"
)

// Codes and statuses referenced by the reset contracts.
const (
	codeModelUnavailable = "model_unavailable"
	statusNotFound       = 404
)

type documentDTO struct {
	Version int       `json:"version" yaml:"version"`
	Rules   []ruleDTO `json:"rules" yaml:"rules"`
}

type ruleDTO struct {
	ID     string     `json:"id" yaml:"id"`
	Match  *matchDTO  `json:"match" yaml:"match"`
	Result *resultDTO `json:"result" yaml:"result"`
}

type matchDTO struct {
	Pattern    string      `json:"pattern" yaml:"pattern"`
	Terms      []string    `json:"terms" yaml:"terms"`
	MinTerms   int         `json:"minTerms" yaml:"minTerms"`
	Providers  []string    `json:"providers" yaml:"providers"`
	Phases     []string    `json:"phases" yaml:"phases"`
	HTTPStatus []int       `json:"httpStatus" yaml:"httpStatus"`
	Context    *contextDTO `json:"context" yaml:"context"`
}

type contextDTO struct {
	Tiers                  []string `json:"tiers" yaml:"tiers"`
	Accounts               []string `json:"accounts" yaml:"accounts"`
	Models                 []string `json:"models" yaml:"models"`
	Paths                  []string `json:"paths" yaml:"paths"`
	FreeDaily              *bool    `json:"freeDaily" yaml:"freeDaily"`
	EmptyOutput            *bool    `json:"emptyOutput" yaml:"emptyOutput"`
	MinConsecutiveFailures int      `json:"minConsecutiveFailures" yaml:"minConsecutiveFailures"`
	MinOtherPaths          int      `json:"minOtherPaths" yaml:"minOtherPaths"`
	MinOtherModels         int      `json:"minOtherModels" yaml:"minOtherModels"`
}

type resultDTO struct {
	Code       string `json:"code" yaml:"code"`
	Confidence string `json:"confidence" yaml:"confidence"`
	Scope      string `json:"scope" yaml:"scope"`
	Backoff    string `json:"backoff" yaml:"backoff"`
	Reset      string `json:"reset" yaml:"reset"`
}

// Vocabulary is the set of classifier values a catalog may reference. The
// caller excludes codes that external rules must not produce.
type Vocabulary struct {
	Codes       []string
	Confidences []string
	Phases      []string
}
