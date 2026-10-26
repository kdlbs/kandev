package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// GitCredentialRelayPrefix is the instance listener prefix served without
	// the control credential: the lease in the request body is the capability
	// and the upstream broker validates it.
	GitCredentialRelayPrefix = "/git-credentials/"

	gitCredentialResolvePath = GitCredentialRelayPrefix + "resolve"
	gitCredentialReissuePath = GitCredentialRelayPrefix + "reissue"
	gitCredentialMaxRequest  = 64 << 10
	gitCredentialMaxResponse = 1 << 20
)

// gitCredentialRelayTimeout bounds one upstream round trip; a variable so a
// test can shorten it.
var gitCredentialRelayTimeout = 15 * time.Second

var gitCredentialRelayClient = &http.Client{
	Transport: &http.Transport{DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// handleGitCredentialRelay forwards the Git credential helper's resolve and
// reissue calls to the upstream broker this instance was created with. The
// caller never influences the upstream URL, path or query. A request is
// evaluated in a fixed order and answered at the first failure: path (404),
// method (405), upstream configured (404), request body size (413), body read
// (400); it is then forwarded once, without retry.
func (s *Server) handleGitCredentialRelay(c *gin.Context) {
	path := c.Request.URL.Path
	if path != gitCredentialResolvePath && path != gitCredentialReissuePath {
		relayError(c, http.StatusNotFound, "not found")
		return
	}
	method := c.Request.Method
	if !relayMethodAllowed(path, method) {
		c.Header("Allow", relayAllowedMethods(path))
		relayError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	upstream, ok := relayUpstreamURL(s.cfg.BrokerUpstreamURL, path)
	if !ok {
		relayError(c, http.StatusNotFound, "not found")
		return
	}
	var body []byte
	if method == http.MethodPost {
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, gitCredentialMaxRequest))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				relayError(c, http.StatusRequestEntityTooLarge, "request too large")
				return
			}
			relayError(c, http.StatusBadRequest, "request body could not be read")
			return
		}
	}
	s.forwardGitCredential(c, method, upstream, body)
}

func (s *Server) forwardGitCredential(c *gin.Context, method, upstream string, body []byte) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), gitCredentialRelayTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, upstream, bytes.NewReader(body))
	if err != nil {
		relayError(c, http.StatusBadGateway, "upstream unavailable")
		return
	}
	for _, name := range []string{"Content-Type", "Accept"} {
		if value := c.GetHeader(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	resp, err := gitCredentialRelayClient.Do(req)
	if err != nil {
		s.relayFailure(c, ctx, "forward", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, gitCredentialMaxResponse+1))
	if err != nil {
		s.relayFailure(c, ctx, "read", err)
		return
	}
	if len(payload) > gitCredentialMaxResponse {
		relayError(c, http.StatusBadGateway, "upstream response too large")
		return
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(payload)
}

// relayFailure answers 504 when the relay deadline expired and 502 for any
// other upstream failure. A caller that disconnected needs no answer.
func (s *Server) relayFailure(c *gin.Context, ctx context.Context, stage string, err error) {
	if c.Request.Context().Err() != nil {
		return
	}
	s.logger.Warn("git credential relay upstream failure", zap.String("stage", stage), zap.Error(err))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		relayError(c, http.StatusGatewayTimeout, "upstream timed out")
		return
	}
	relayError(c, http.StatusBadGateway, "upstream unavailable")
}

func relayError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{errKey: message})
}

func relayMethodAllowed(path, method string) bool {
	if path == gitCredentialResolvePath {
		return method == http.MethodGet || method == http.MethodPost
	}
	return method == http.MethodPost
}

func relayAllowedMethods(path string) string {
	if path == gitCredentialResolvePath {
		return "GET, POST"
	}
	return "POST"
}

// relayUpstreamURL maps a relay path to the upstream URL: resolve goes to the
// configured URL, reissue to the same URL with its trailing /resolve replaced.
func relayUpstreamURL(configured, path string) (string, bool) {
	if configured == "" {
		return "", false
	}
	if path == gitCredentialResolvePath {
		return configured, true
	}
	parsed, err := url.Parse(configured)
	if err != nil || !strings.HasSuffix(parsed.Path, "/resolve") {
		return "", false
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/resolve") + "/reissue"
	parsed.RawPath = ""
	return parsed.String(), true
}
