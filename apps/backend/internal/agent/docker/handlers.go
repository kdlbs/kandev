// Package docker provides Docker management HTTP handlers.
package docker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// buildImageRequest is the JSON body for POST /api/v1/docker/build.
type buildImageRequest struct {
	Dockerfile string             `json:"dockerfile" binding:"required"`
	Tag        string             `json:"tag" binding:"required"`
	BuildArgs  map[string]*string `json:"build_args,omitempty"`
}

// containerResponse is the JSON representation of a container in list responses.
type containerResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	State     string            `json:"state"`
	Status    string            `json:"status"`
	StartedAt time.Time         `json:"started_at"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// stopContainerRequest is the optional JSON body for POST /api/v1/docker/containers/:id/stop.
type stopContainerRequest struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// ClientProvider lazily resolves the Docker client. Returns nil if Docker is unavailable.
type ClientProvider func() *Client

// containerAPI is the Docker surface these handlers use. *Client satisfies it;
// tests inject a fake so handler behavior can be exercised without a daemon.
type containerAPI interface {
	BuildImage(ctx context.Context, dockerfile string, tag string, buildArgs map[string]*string) (io.ReadCloser, error)
	ListContainers(ctx context.Context, labels map[string]string) ([]ContainerInfo, error)
	GetContainerLabels(ctx context.Context, containerID string) (map[string]string, error)
	StopContainer(ctx context.Context, containerID string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error
}

var _ containerAPI = (*Client)(nil)

// clientResolver resolves the Docker surface per request, returning nil when
// Docker is unavailable.
type clientResolver func() containerAPI

// TaskTitleProvider resolves a task ID to its display title for container listings.
type TaskTitleProvider func(ctx context.Context, taskID string) (string, bool)

// SessionAuthorizer scopes container operations to the task session that owns
// the container. Kandev stamps every managed container with its owning task
// and session (internal/agent/runtime/lifecycle/container.go), so the labels
// are the ownership record these checks key off.
//
// Both methods follow task/service.Service's contract: no identity in ctx
// (internal callers) or a synthetic identity (authentication disabled) is
// unscoped, and denials come back as not-found sentinels so a foreign resource
// is indistinguishable from a missing one.
type SessionAuthorizer interface {
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
	AuthorizeTaskAccess(ctx context.Context, taskID string) error
}

// Labels stamped on every Kandev-managed container.
const (
	sessionIDLabel = "kandev.session_id"
	taskIDLabel    = "kandev.task_id"
	taskTitleLabel = "kandev.task_title"
)

// errContainerUnowned marks a container whose owning session or task cannot be
// determined from its labels. Scoped callers never see such a container.
var errContainerUnowned = errors.New("container has no resolvable owner")

// RegisterDockerRoutes registers Docker management HTTP routes on the given router.
// The clientProvider lazily resolves the Docker client on each request, and the
// authorizer scopes container access to the caller's own task sessions.
func RegisterDockerRoutes(
	router *gin.Engine, clientProvider ClientProvider,
	taskTitleProvider TaskTitleProvider, authorizer SessionAuthorizer, log *logger.Logger,
) {
	resolve := func() containerAPI {
		client := clientProvider()
		if client == nil {
			return nil
		}
		return client
	}
	registerRoutes(router, resolve, taskTitleProvider, authorizer, log)
}

// registerRoutes is the testable form of RegisterDockerRoutes, taking the
// Docker surface as an interface instead of the concrete client.
func registerRoutes(
	router *gin.Engine, resolve clientResolver,
	taskTitleProvider TaskTitleProvider, authorizer SessionAuthorizer, log *logger.Logger,
) {
	api := router.Group("/api/v1/docker")
	// Building an image is a host-level operation with no per-user resource,
	// so it is admin-only like the other install-wide mutations.
	api.POST("/build", authn.RequireAdmin(), handleBuildImage(resolve, log))
	api.GET("/containers", handleListContainers(resolve, taskTitleProvider, authorizer, log))
	api.POST("/containers/:id/stop", handleStopContainer(resolve, authorizer, log))
	api.DELETE("/containers/:id", handleRemoveContainer(resolve, authorizer, log))
}

// requireDocker resolves the Docker client and returns 503 if unavailable.
func requireDocker(c *gin.Context, resolve clientResolver) containerAPI {
	client := resolve()
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Docker is not available"})
		return nil
	}
	return client
}

// callerScoped mirrors task/service's callerScope: a missing identity
// (internal callers) or the synthetic identity injected while authentication
// is disabled means no scoping at all, preserving single-user behavior.
func callerScoped(ctx context.Context) bool {
	identity, ok := authn.IdentityFromContext(ctx)
	return ok && !identity.Synthetic
}

// authorizeContainerLabels reports whether the caller may see the container
// described by labels. A container with no owning session or task, or with no
// authorizer wired, is denied rather than exposed.
//
// The session label goes stale when a session is rolled back or removed while
// its task and container live on, so a failed session check falls back to the
// task label — the container GC treats kandev.task_id as the durable owner for
// the same reason. The fallback cannot widen access: both labels are stamped
// from one launch config, and AuthorizeSessionAccess resolves through the
// task's workspace anyway, so it admits nobody AuthorizeTaskAccess would deny.
func authorizeContainerLabels(ctx context.Context, labels map[string]string, authorizer SessionAuthorizer) error {
	if authorizer == nil {
		return errContainerUnowned
	}
	sessionID, taskID := labels[sessionIDLabel], labels[taskIDLabel]
	if sessionID != "" {
		err := authorizer.AuthorizeSessionAccess(ctx, sessionID)
		if err == nil || taskID == "" {
			return err
		}
	}
	if taskID != "" {
		return authorizer.AuthorizeTaskAccess(ctx, taskID)
	}
	return errContainerUnowned
}

// visibleContainers drops every container the caller may not see. It runs
// before any task title is resolved, so a foreign task's title is never read.
func visibleContainers(ctx context.Context, containers []ContainerInfo, authorizer SessionAuthorizer) []ContainerInfo {
	if !callerScoped(ctx) {
		return containers
	}
	visible := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		if authorizeContainerLabels(ctx, ctr.Labels, authorizer) == nil {
			visible = append(visible, ctr)
		}
	}
	return visible
}

// authorizeContainerID resolves a container's owner from its labels and
// authorizes the caller. It writes 404 and returns false for a denied
// container, an unknown container, and an unresolvable owner alike, so none of
// the three can be told apart.
func authorizeContainerID(
	c *gin.Context, client containerAPI, authorizer SessionAuthorizer,
	containerID string, log *logger.Logger,
) bool {
	ctx := c.Request.Context()
	if !callerScoped(ctx) {
		return true
	}
	labels, err := client.GetContainerLabels(ctx, containerID)
	if err == nil {
		err = authorizeContainerLabels(ctx, labels, authorizer)
	}
	if err != nil {
		log.Debug("Denied container access", zap.String("id", containerID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "container not found"})
		return false
	}
	return true
}

// handleBuildImage handles POST /api/v1/docker/build.
// It streams the Docker build output as JSON lines to the client.
func handleBuildImage(resolve clientResolver, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		dockerClient := requireDocker(c, resolve)
		if dockerClient == nil {
			return
		}

		var req buildImageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
			return
		}

		reader, err := dockerClient.BuildImage(c.Request.Context(), req.Dockerfile, req.Tag, req.BuildArgs)
		if err != nil {
			log.Error("Failed to start image build", zap.String("tag", req.Tag), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer func() {
			if closeErr := reader.Close(); closeErr != nil {
				log.Warn("Failed to close build reader", zap.Error(closeErr))
			}
		}()

		streamBuildOutput(c, reader, log)
	}
}

// streamBuildOutput reads from the Docker build output and streams it to the HTTP response.
func streamBuildOutput(c *gin.Context, reader interface{ Read([]byte) (int, error) }, log *logger.Logger) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Status(http.StatusOK)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := c.Writer.Write(line); err != nil {
			log.Debug("Client disconnected during build stream", zap.Error(err))
			return
		}
		if _, err := c.Writer.Write([]byte("\n")); err != nil {
			log.Debug("Client disconnected during build stream", zap.Error(err))
			return
		}
		c.Writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		log.Error("Error reading build output", zap.Error(err))
	}
}

// handleListContainers handles GET /api/v1/docker/containers.
// Supports optional query params: image, labels (comma-separated key=value pairs).
func handleListContainers(
	resolve clientResolver, taskTitleProvider TaskTitleProvider,
	authorizer SessionAuthorizer, log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dockerClient := requireDocker(c, resolve)
		if dockerClient == nil {
			return
		}

		labels := parseLabelsQuery(c)
		addImageFilter(c, labels)

		containers, err := dockerClient.ListContainers(c.Request.Context(), labels)
		if err != nil {
			log.Error("Failed to list containers", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		visible := visibleContainers(c.Request.Context(), containers, authorizer)
		resp := newContainerResponsesWithTaskTitles(c.Request.Context(), visible, taskTitleProvider)

		c.JSON(http.StatusOK, gin.H{"containers": resp})
	}
}

func newContainerResponses(containers []ContainerInfo) []containerResponse {
	return newContainerResponsesWithTaskTitles(context.Background(), containers, nil)
}

func newContainerResponsesWithTaskTitles(ctx context.Context, containers []ContainerInfo, taskTitleProvider TaskTitleProvider) []containerResponse {
	resp := make([]containerResponse, len(containers))
	for i, ctr := range containers {
		resp[i] = containerResponse{
			ID:        ctr.ID,
			Name:      ctr.Name,
			Image:     ctr.Image,
			State:     ctr.State,
			Status:    ctr.Status,
			StartedAt: ctr.StartedAt,
			Labels:    labelsWithTaskTitle(ctx, ctr.Labels, taskTitleProvider),
		}
	}
	return resp
}

func labelsWithTaskTitle(ctx context.Context, labels map[string]string, taskTitleProvider TaskTitleProvider) map[string]string {
	if len(labels) == 0 {
		return labels
	}
	result := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		result[key] = value
	}
	if result[taskTitleLabel] != "" || taskTitleProvider == nil {
		return result
	}
	title, ok := taskTitleProvider(ctx, result[taskIDLabel])
	if ok && title != "" {
		result[taskTitleLabel] = title
	}
	return result
}

// parseLabelsQuery extracts label filters from the "labels" query parameter.
// Expected format: "key1=value1,key2=value2".
func parseLabelsQuery(c *gin.Context) map[string]string {
	labels := make(map[string]string)
	labelsParam := c.Query("labels")
	if labelsParam == "" {
		return labels
	}

	for _, pair := range splitNonEmpty(labelsParam, ',') {
		parts := splitNonEmpty(pair, '=')
		if len(parts) == 2 { //nolint:mnd
			labels[parts[0]] = parts[1]
		}
	}

	return labels
}

// splitNonEmpty splits a string by sep and returns only non-empty parts.
func splitNonEmpty(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == sep {
			part := s[start:i]
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	return parts
}

// addImageFilter adds the "image" query parameter as a label filter placeholder.
// Note: The Docker API uses the "ancestor" filter for image-based filtering,
// but our Client.ListContainers uses label filters. The image filter is applied
// as a label for consistency; callers should label containers with their image.
func addImageFilter(c *gin.Context, labels map[string]string) {
	imageFilter := c.Query("image")
	if imageFilter != "" {
		labels["com.kandev.image"] = imageFilter
	}
}

// handleStopContainer handles POST /api/v1/docker/containers/:id/stop.
func handleStopContainer(resolve clientResolver, authorizer SessionAuthorizer, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		dockerClient := requireDocker(c, resolve)
		if dockerClient == nil {
			return
		}

		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "container id is required"})
			return
		}

		if !authorizeContainerID(c, dockerClient, authorizer, containerID, log) {
			return
		}

		var req stopContainerRequest
		// Bind is optional; ignore errors for empty body
		_ = c.ShouldBindJSON(&req)

		timeout := 30 * time.Second
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}

		if err := dockerClient.StopContainer(c.Request.Context(), containerID, timeout); err != nil {
			log.Error("Failed to stop container", zap.String("id", containerID), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	}
}

// handleRemoveContainer handles DELETE /api/v1/docker/containers/:id.
func handleRemoveContainer(resolve clientResolver, authorizer SessionAuthorizer, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		dockerClient := requireDocker(c, resolve)
		if dockerClient == nil {
			return
		}

		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "container id is required"})
			return
		}

		if !authorizeContainerID(c, dockerClient, authorizer, containerID, log) {
			return
		}

		if err := dockerClient.RemoveContainer(c.Request.Context(), containerID, true); err != nil {
			log.Error("Failed to remove container", zap.String("id", containerID), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "removed"})
	}
}
