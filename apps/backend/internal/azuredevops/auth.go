package azuredevops

import (
	"context"
	"encoding/base64"
	"strings"
)

// cloneUsername is the Git Basic username sent with Azure Repos credentials.
// Azure DevOps ignores it for both PATs and Entra access tokens.
const cloneUsername = "kandev"

// Authorizer supplies the Authorization header of one workspace connection.
type Authorizer interface {
	// Header returns the Authorization header value for the next request.
	Header(ctx context.Context) (string, error)
	// Renew replaces the credential that produced rejectedHeader. It returns
	// false when the credential cannot be renewed without the user.
	Renew(ctx context.Context, rejectedHeader string) (bool, error)
	// Redact removes credential material from provider error text.
	Redact(text string) string
}

type patAuthorizer struct{ pat string }

// NewPATAuthorizer authenticates with a personal access token over Basic auth.
func NewPATAuthorizer(pat string) Authorizer { return patAuthorizer{pat: pat} }

func (a patAuthorizer) Header(context.Context) (string, error) {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+a.pat)), nil
}

func (patAuthorizer) Renew(context.Context, string) (bool, error) { return false, nil }

func (a patAuthorizer) Redact(text string) string {
	if a.pat == "" {
		return text
	}
	return strings.ReplaceAll(text, a.pat, "[REDACTED]")
}
