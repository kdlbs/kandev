package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	tasklsp "github.com/kandev/kandev/internal/agentctl/server/lsp"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
)

const (
	lspWebSocketWriteTimeout = 5 * time.Second
	taskLSPTaskIDHeader      = "X-Kandev-LSP-Task-ID"
)

func (s *Server) handleTaskLSPSnapshot(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, s.lspManager.SnapshotForTask(taskID, language))
}

func (s *Server) handleTaskLSPDiscovery(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, tasklsp.DiscoverLanguagesAtRoots(
		c.Request.Context(), s.lspManager.DiscoveryRootsForTask(taskID),
	))
}

func (s *Server) handleTaskLSPWorkspaceRefresh(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	var body tasklspWorkspaceBody
	if err := decodeStrictJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	result, err := s.lspManager.UpdateWorkspaceForTask(taskID, body.WorkspacePath, body.WorkspaceRoots)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{errKey: err.Error(), "result": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

type tasklspWorkspaceBody struct {
	WorkspacePath  string   `json:"workspace_path"`
	WorkspaceRoots []string `json:"workspace_roots,omitempty"`
}

type taskLSPStartBody struct {
	Generation    uint64          `json:"generation"`
	AutoInstall   bool            `json:"auto_install"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

type taskLSPStopBody struct {
	Generation uint64 `json:"generation,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type taskLSPConfigurationBody struct {
	Generation    uint64          `json:"generation"`
	Configuration json.RawMessage `json:"configuration"`
}

func (s *Server) handleTaskLSPStart(c *gin.Context)   { s.handleTaskLSPStartAction(c, false) }
func (s *Server) handleTaskLSPRestart(c *gin.Context) { s.handleTaskLSPStartAction(c, true) }

func (s *Server) handleTaskLSPStartAction(c *gin.Context, restart bool) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return
	}
	var body taskLSPStartBody
	if err := decodeStrictJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	request := tasklsp.StartRequest{
		TaskID: taskID, Language: language, Generation: body.Generation,
		AutoInstall: body.AutoInstall, Configuration: body.Configuration,
	}
	var (
		snapshot tasklsp.Snapshot
		err      error
	)
	if restart {
		snapshot, err = s.lspManager.Restart(c.Request.Context(), request)
	} else {
		snapshot, err = s.lspManager.Start(c.Request.Context(), request)
	}
	writeTaskLSPResult(c, snapshot, err)
}

func (s *Server) handleTaskLSPStop(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return
	}
	var body taskLSPStopBody
	if err := decodeStrictJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	snapshot, err := s.lspManager.Stop(c.Request.Context(), tasklsp.StopRequest{
		TaskID: taskID, Language: language, Generation: body.Generation, Reason: body.Reason,
	})
	writeTaskLSPResult(c, snapshot, err)
}

func (s *Server) handleTaskLSPConfiguration(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return
	}
	var body taskLSPConfigurationBody
	if err := decodeStrictJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	snapshot, err := s.lspManager.UpdateConfiguration(c.Request.Context(), tasklsp.ConfigurationRequest{
		TaskID: taskID, Language: language, Generation: body.Generation, Configuration: body.Configuration,
	})
	writeTaskLSPResult(c, snapshot, err)
}

func (s *Server) handleTaskLSPWatch(c *gin.Context) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return
	}
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(4 << 10)
	updates, unsubscribe := s.lspManager.SubscribeForTask(taskID, language)
	defer unsubscribe()
	defer func() { _ = conn.Close() }()
	clientClosed := make(chan struct{})
	go func() {
		defer close(clientClosed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case snapshot, open := <-updates:
			if !open {
				return
			}
			if err := conn.SetWriteDeadline(time.Now().Add(lspWebSocketWriteTimeout)); err != nil {
				return
			}
			if err := conn.WriteJSON(snapshot); err != nil {
				return
			}
		case <-clientClosed:
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}

func (s *Server) handleTaskLSPAttach(c *gin.Context) {
	attachment, ok := s.openTaskLSPAttachment(c)
	if !ok {
		return
	}
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		attachment.Close()
		return
	}
	conn.SetReadLimit(protocol.MaxMessageBytes)
	writerDone := make(chan struct{})
	go writeTaskLSPAttachment(conn, attachment, writerDone)
	readTaskLSPAttachment(conn, attachment)
	<-writerDone
}

func (s *Server) openTaskLSPAttachment(c *gin.Context) (*tasklsp.Attachment, bool) {
	taskID, ok := taskLSPTaskID(c)
	if !ok {
		return nil, false
	}
	language, ok := taskLSPLanguage(c)
	if !ok {
		return nil, false
	}
	generation, err := strconv.ParseUint(c.Query("generation"), 10, 64)
	if err != nil || generation == 0 {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "generation query parameter must be a positive integer"})
		return nil, false
	}
	attachment, err := s.lspManager.AttachForTask(taskID, language, generation)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, tasklsp.ErrManagerClosed) || errors.Is(err, process.ErrManagerStopping) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{errKey: err.Error()})
		return nil, false
	}
	return attachment, true
}

func writeTaskLSPAttachment(conn *websocket.Conn, attachment *tasklsp.Attachment, done chan<- struct{}) {
	defer close(done)
	defer func() { _ = conn.Close() }()
	for message := range attachment.Messages() {
		if err := conn.SetWriteDeadline(time.Now().Add(lspWebSocketWriteTimeout)); err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

func readTaskLSPAttachment(conn *websocket.Conn, attachment *tasklsp.Attachment) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType == websocket.TextMessage {
			if err := attachment.Handle(message); err != nil {
				break
			}
		}
	}
	attachment.Close()
	_ = conn.Close()
}

func taskLSPLanguage(c *gin.Context) (string, bool) {
	language := c.Param("language")
	if !installer.IsSupported(language) {
		c.JSON(http.StatusBadRequest, gin.H{errKey: fmt.Sprintf("unsupported language: %s", language)})
		return "", false
	}
	return language, true
}

func taskLSPTaskID(c *gin.Context) (string, bool) {
	taskID := strings.TrimSpace(c.GetHeader(taskLSPTaskIDHeader))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "task LSP task identity header is required"})
		return "", false
	}
	return taskID, true
}

func decodeStrictJSON(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid request body: multiple JSON values")
	}
	return nil
}

func writeTaskLSPResult(c *gin.Context, snapshot tasklsp.Snapshot, err error) {
	if err == nil {
		c.JSON(http.StatusOK, snapshot)
		return
	}
	status := http.StatusUnprocessableEntity
	switch {
	case errors.Is(err, tasklsp.ErrStaleGeneration), errors.Is(err, tasklsp.ErrRuntimeNotReady):
		status = http.StatusConflict
	case errors.Is(err, tasklsp.ErrManagerClosed), errors.Is(err, process.ErrManagerStopping):
		status = http.StatusServiceUnavailable
	case strings.Contains(err.Error(), "generation must"), strings.Contains(err.Error(), "configuration must"):
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{errKey: err.Error(), "snapshot": snapshot})
}

func taskLSPWorkspaceFolders(workDir string, repositorySubpaths ...string) []tasklsp.WorkspaceFolder {
	root := filepath.Clean(workDir)
	roots := make([]string, 0, len(repositorySubpaths))
	seen := make(map[string]bool, len(repositorySubpaths))
	for _, subpath := range repositorySubpaths {
		if subpath == "" || filepath.IsAbs(subpath) {
			continue
		}
		candidate := filepath.Clean(filepath.Join(root, subpath))
		relative, err := filepath.Rel(root, candidate)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || seen[candidate] {
			continue
		}
		seen[candidate] = true
		roots = append(roots, candidate)
	}
	return tasklsp.WorkspaceFoldersAtRoots(root, roots)
}

// workspaceFileURI applies task-host path semantics, including Windows drive
// and UNC forms, while strictly escaping reserved URI characters.
func workspaceFileURI(path string) string {
	return tasklsp.WorkspaceFileURI(path)
}
