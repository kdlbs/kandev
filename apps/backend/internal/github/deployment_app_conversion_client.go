package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxManifestConversionResponseSize = 1024 * 1024
	manifestConversionClientTimeout   = 30 * time.Second
	manifestGitHubAccept              = "application/vnd.github+json"
	manifestGitHubAPIVersion          = "2022-11-28"
)

type ManifestConversionErrorCode string

const (
	ManifestConversionCodeInvalid      ManifestConversionErrorCode = "manifest_conversion_code_invalid"
	ManifestConversionRequestFailed    ManifestConversionErrorCode = "manifest_conversion_request_failed"
	ManifestConversionRejected         ManifestConversionErrorCode = "manifest_conversion_rejected"
	ManifestConversionResponseTooLarge ManifestConversionErrorCode = "manifest_conversion_response_too_large"
	ManifestConversionInvalidResponse  ManifestConversionErrorCode = "manifest_conversion_invalid_response"
)

type ManifestConversionError struct {
	Code       ManifestConversionErrorCode
	StatusCode int
}

func (e *ManifestConversionError) Error() string {
	switch e.Code {
	case ManifestConversionCodeInvalid:
		return "GitHub App manifest conversion code is invalid"
	case ManifestConversionRequestFailed:
		return "GitHub App manifest conversion request failed"
	case ManifestConversionRejected:
		return "GitHub rejected the App manifest conversion"
	case ManifestConversionResponseTooLarge:
		return "GitHub App manifest conversion response exceeded the allowed size"
	default:
		return "GitHub App manifest conversion returned an invalid response"
	}
}

type ManifestConversionOwner struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

type ManifestConversionResult struct {
	AppID         int64                   `json:"id"`
	NodeID        string                  `json:"node_id"`
	Slug          string                  `json:"slug"`
	Name          string                  `json:"name"`
	HTMLURL       string                  `json:"html_url"`
	Owner         ManifestConversionOwner `json:"owner"`
	ClientID      string                  `json:"client_id"`
	ClientSecret  string                  `json:"client_secret"`
	WebhookSecret string                  `json:"webhook_secret"`
	PrivateKeyPEM string                  `json:"pem"`
	Permissions   map[string]string       `json:"permissions"`
	Events        []string                `json:"events"`
}

func (r ManifestConversionResult) Redacted() ManifestConversionResult {
	r.ClientSecret = ""
	r.WebhookSecret = ""
	r.PrivateKeyPEM = ""
	return r
}

type ManifestConversionClient struct {
	httpClient *http.Client
	apiBaseURL string
}

func NewManifestConversionClient() *ManifestConversionClient {
	return &ManifestConversionClient{
		httpClient: &http.Client{
			Timeout: manifestConversionClientTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		apiBaseURL: "https://api.github.com",
	}
}

func (c *ManifestConversionClient) Convert(
	ctx context.Context,
	conversionCode string,
) (ManifestConversionResult, error) {
	if c == nil || c.httpClient == nil || strings.TrimSpace(conversionCode) == "" {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionCodeInvalid}
	}
	endpoint := strings.TrimRight(c.apiBaseURL, "/") +
		"/app-manifests/" + url.PathEscape(conversionCode) + "/conversions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionRequestFailed}
	}
	request.Header.Set("Accept", manifestGitHubAccept)
	request.Header.Set("X-GitHub-Api-Version", manifestGitHubAPIVersion)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionRequestFailed}
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxManifestConversionResponseSize+1))
	if err != nil {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionRequestFailed}
	}
	if len(body) > maxManifestConversionResponseSize {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionResponseTooLarge}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ManifestConversionResult{}, &ManifestConversionError{
			Code: ManifestConversionRejected, StatusCode: response.StatusCode,
		}
	}
	var result ManifestConversionResult
	if err := json.Unmarshal(body, &result); err != nil {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionInvalidResponse}
	}
	if !validManifestConversionResult(result) {
		return ManifestConversionResult{}, &ManifestConversionError{Code: ManifestConversionInvalidResponse}
	}
	return result, nil
}

func validManifestConversionResult(result ManifestConversionResult) bool {
	return result.AppID > 0 && result.Slug != "" && result.Name != "" &&
		result.Owner.ID > 0 && result.Owner.Login != "" &&
		(strings.EqualFold(result.Owner.Type, string(ManifestOwnerUser)) ||
			strings.EqualFold(result.Owner.Type, string(ManifestOwnerOrganization))) &&
		result.ClientID != "" && result.ClientSecret != "" && result.WebhookSecret != "" &&
		result.PrivateKeyPEM != "" && len(result.Permissions) > 0 && len(result.Events) > 0
}
