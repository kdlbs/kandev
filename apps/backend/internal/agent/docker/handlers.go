// Package docker provides Docker management HTTP handlers.
package docker

import (
	"bufio"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	State     string    `json:"state"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

// stopContainerRequest is the optional JSON body for POST /api/v1/docker/containers/:id/stop.
type stopContainerRequest struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// RegisterDockerRoutes registers Docker management HTTP routes on the given router.
func RegisterDockerRoutes(router *gin.Engine, dockerClient *Client, log *logger.Logger) {
	if dockerClient == nil {
		log.Warn("Docker client is nil, skipping Docker route registration")
		return
	}

	api := router.Group("/api/v1/docker")
	api.POST("/build", handleBuildImage(dockerClient, log))
	api.GET("/containers", handleListContainers(dockerClient, log))
	api.POST("/containers/:id/stop", handleStopContainer(dockerClient, log))
	api.DELETE("/containers/:id", handleRemoveContainer(dockerClient, log))
}

// handleBuildImage handles POST /api/v1/docker/build.
// It streams the Docker build output as JSON lines to the client.
func handleBuildImage(dockerClient *Client, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
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
func handleListContainers(dockerClient *Client, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		labels := parseLabelsQuery(c)
		addImageFilter(c, labels)

		containers, err := dockerClient.ListContainers(c.Request.Context(), labels)
		if err != nil {
			log.Error("Failed to list containers", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		resp := make([]containerResponse, len(containers))
		for i, ctr := range containers {
			resp[i] = containerResponse{
				ID:        ctr.ID,
				Name:      ctr.Name,
				Image:     ctr.Image,
				State:     ctr.State,
				Status:    ctr.Status,
				StartedAt: ctr.StartedAt,
			}
		}

		c.JSON(http.StatusOK, gin.H{"containers": resp})
	}
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
func handleStopContainer(dockerClient *Client, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "container id is required"})
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
func handleRemoveContainer(dockerClient *Client, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "container id is required"})
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
