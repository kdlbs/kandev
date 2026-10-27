package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxGitHubExpectedHostBodyBytes = 4 * 1024 * 1024

func (c *Controller) captureExpectedGitHubHost(ctx *gin.Context) {
	expected := ctx.Query("expected_host")
	if ctx.Request.Body == nil || !strings.HasPrefix(ctx.GetHeader("Content-Type"), "application/json") {
		continueGitHubHostRequest(ctx, expected)
		return
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxGitHubExpectedHostBodyBytes+1))
	if err != nil || len(body) > maxGitHubExpectedHostBodyBytes {
		ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "GitHub request body exceeds the allowed size"})
		return
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	bodyHost, err := expectedGitHubHostFromBody(body)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "github_host_invalid"})
		return
	}
	if bodyHost != "" && expected != "" && bodyHost != expected {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": ErrGitHubHostMismatch.Error(), "code": "github_host_mismatch"})
		return
	}
	if bodyHost != "" {
		expected = bodyHost
	}
	continueGitHubHostRequest(ctx, expected)
}

func expectedGitHubHostFromBody(body []byte) (string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return "", nil
	}
	value, present := fields["expected_host"]
	if !present {
		return "", nil
	}
	var host string
	if string(value) == jsonNullLiteral || json.Unmarshal(value, &host) != nil {
		return "", errors.New("GitHub host must be a string")
	}
	return host, nil
}

func continueGitHubHostRequest(ctx *gin.Context, expected string) {
	ctx.Request = ctx.Request.WithContext(withExpectedGitHubHost(ctx.Request.Context(), expected))
	ctx.Next()
}
