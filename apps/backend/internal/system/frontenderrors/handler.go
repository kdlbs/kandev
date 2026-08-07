// Package frontenderrors accepts bounded, best-effort reports for error toasts
// already displayed by the browser.
package frontenderrors

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	maxRequestBodyBytes   = 64 * 1024
	titleByteLimit        = 8 * 1024
	descriptionByteLimit  = 8 * 1024
	urlByteLimit          = 8 * 1024
	taskIDByteLimit       = 128
	browserFieldByteLimit = 2 * 1024
	stackByteLimit        = 16 * 1024
)

type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ErrorDetails struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

type Request struct {
	ClientTimestamp string        `json:"client_timestamp"`
	Source          string        `json:"source"`
	TaskID          string        `json:"task_id"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`
	URL             string        `json:"url"`
	UserAgent       string        `json:"user_agent"`
	Language        string        `json:"language"`
	Platform        string        `json:"platform"`
	Viewport        *Viewport     `json:"viewport"`
	Stack           string        `json:"stack"`
	Error           *ErrorDetails `json:"error"`
}

type normalizedRequest struct {
	Request
	Truncated bool
}

type Service struct {
	log     *logger.Logger
	limiter *limiter
}

func New(log *logger.Logger, now func() time.Time) *Service {
	return &Service{log: log, limiter: newLimiter(now)}
}

func Handle(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := authn.FromGin(c)
		if !ok || identity.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if allowed, retry := service.limiter.Allow(identity.UserID); !allowed {
			c.Header("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		request, requestBytes, status, err := decodeRequest(c)
		if err != nil {
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		if allowed, retry := service.limiter.AllowBytes(identity.UserID, requestBytes); !allowed {
			c.Header("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		normalized, err := normalize(request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		service.logRequest(identity.UserID, normalized)
		c.Status(http.StatusNoContent)
	}
}

func decodeRequest(c *gin.Context) (Request, int, int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return Request{}, 0, http.StatusRequestEntityTooLarge,
				errors.New("request body too large")
		}
		return Request{}, 0, http.StatusBadRequest, errors.New("invalid request body")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, 0, http.StatusBadRequest, errors.New("invalid request body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Request{}, 0, http.StatusBadRequest,
			errors.New("request body must contain one JSON object")
	}
	return request, len(body), http.StatusOK, nil
}

func normalize(request Request) (normalizedRequest, error) {
	if request.Source != "sonner" && request.Source != "toast-provider" {
		return normalizedRequest{}, errors.New("unsupported toast source")
	}
	if strings.TrimSpace(request.Title) == "" && strings.TrimSpace(request.Description) == "" {
		return normalizedRequest{}, errors.New("title or description is required")
	}
	if request.ClientTimestamp != "" {
		if _, err := time.Parse(time.RFC3339Nano, request.ClientTimestamp); err != nil {
			return normalizedRequest{}, errors.New("client_timestamp must be RFC3339")
		}
	}
	normalized := normalizedRequest{Request: request}
	normalized.Title = truncateField(request.Title, titleByteLimit, &normalized.Truncated)
	normalized.Description = truncateField(request.Description, descriptionByteLimit, &normalized.Truncated)
	normalized.URL = truncateField(redactURL(request.URL), urlByteLimit, &normalized.Truncated)
	normalized.TaskID = truncateField(request.TaskID, taskIDByteLimit, &normalized.Truncated)
	normalized.UserAgent = truncateField(request.UserAgent, browserFieldByteLimit, &normalized.Truncated)
	normalized.Language = truncateField(request.Language, browserFieldByteLimit, &normalized.Truncated)
	normalized.Platform = truncateField(request.Platform, browserFieldByteLimit, &normalized.Truncated)
	normalized.Stack = truncateField(request.Stack, stackByteLimit, &normalized.Truncated)
	if request.Error != nil {
		copy := *request.Error
		copy.Name = truncateField(copy.Name, browserFieldByteLimit, &normalized.Truncated)
		copy.Message = truncateField(copy.Message, titleByteLimit, &normalized.Truncated)
		copy.Stack = truncateField(copy.Stack, stackByteLimit, &normalized.Truncated)
		normalized.Error = &copy
	}
	return normalized, nil
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func truncateField(value string, limit int, truncated *bool) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	*truncated = true
	return value[:end]
}

func (s *Service) logRequest(userID string, request normalizedRequest) {
	fields := []zap.Field{
		zap.String("reporting_user_id", userID),
		zap.String("client_timestamp", request.ClientTimestamp),
		zap.String("source", request.Source),
		zap.String("task_id", request.TaskID),
		zap.String("title", request.Title),
		zap.String("description", request.Description),
		zap.String("url", request.URL),
		zap.String("user_agent", request.UserAgent),
		zap.String("language", request.Language),
		zap.String("platform", request.Platform),
		zap.String("stack", request.Stack),
		zap.Bool("truncated", request.Truncated),
	}
	if request.Viewport != nil {
		fields = append(fields, zap.Int("viewport_width", request.Viewport.Width),
			zap.Int("viewport_height", request.Viewport.Height))
	}
	if request.Error != nil {
		fields = append(fields,
			zap.String("error_name", request.Error.Name),
			zap.String("error_message", request.Error.Message),
			zap.String("error_stack", request.Error.Stack),
		)
	}
	s.log.Error("frontend error toast", fields...)
}
