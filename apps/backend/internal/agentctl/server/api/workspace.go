package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// workspaceStreamConn is the single seam onto the workspace stream socket.
//
// A gorilla *websocket.Conn supports exactly one concurrent writer, and this
// endpoint has two producers: the event-forwarding loop and the input loop
// that answers client pings. Both write through WriteJSON, so the lock lives
// in one place and a future write site cannot forget it.
type workspaceStreamConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// WriteJSON sends one message. Safe for concurrent use.
func (w *workspaceStreamConn) WriteJSON(msg types.WorkspaceStreamMessage) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.WriteJSON(msg)
}

// ReadJSON receives one message. Only the input loop reads, so it needs no lock.
func (w *workspaceStreamConn) ReadJSON(msg *types.WorkspaceStreamMessage) error {
	return w.conn.ReadJSON(msg)
}

// handleWorkspaceStreamWS handles the unified workspace stream WebSocket endpoint.
// It streams git status, file changes, file lists, and shell I/O over a single connection.
func (s *Server) handleWorkspaceStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close workspace stream websocket", zap.Error(err))
		}
	}()

	stream := &workspaceStreamConn{conn: conn}

	s.logger.Info("Workspace stream WebSocket connected")

	// Subscribe to unified workspace updates. Manager.SubscribeWorkspaceStream
	// fans out across the root tracker plus every per-repo tracker, so for
	// multi-repo task roots the client receives one event per repo (each
	// tagged with RepositoryName) on a single channel.
	sub := s.procMgr.SubscribeWorkspaceStream()
	defer s.procMgr.UnsubscribeWorkspaceStream(sub)

	// Get shell for input handling (may be nil)
	shell := s.procMgr.Shell()

	// Subscribe to shell output if shell is available
	var shellOutputCh chan []byte
	if shell != nil {
		shellOutputCh = make(chan []byte, 256)
		shell.Subscribe(shellOutputCh)
		defer shell.Unsubscribe(shellOutputCh)
	}

	// Send connected message
	connectedMsg := types.NewWorkspaceConnected()
	if err := stream.WriteJSON(connectedMsg); err != nil {
		s.logger.Debug("failed to send connected message", zap.Error(err))
		return
	}

	// Done channel to signal goroutine shutdown
	done := make(chan struct{})
	defer close(done)

	// Handle incoming messages (shell_input, shell_resize, ping) in a goroutine
	var shellWriter io.Writer
	if shell != nil {
		shellWriter = shell
	}
	go s.handleWorkspaceStreamInput(stream, shellWriter, done)

	s.forwardWorkspaceStream(stream, sub, shellOutputCh, done)
}

// forwardWorkspaceStream forwards workspace events and shell output to the
// WebSocket until the client goes away or the handler shuts down.
func (s *Server) forwardWorkspaceStream(
	stream *workspaceStreamConn,
	sub types.WorkspaceStreamSubscriber,
	shellOutputCh chan []byte,
	done <-chan struct{},
) {
	for {
		select {
		case <-done:
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}
			if err := stream.WriteJSON(msg); err != nil {
				s.logger.Debug("workspace stream write error", zap.Error(err))
				return
			}
		case data, ok := <-shellOutputCh:
			if !ok {
				// Shell output channel closed
				shellOutputCh = nil
				continue
			}
			// Forward shell output as workspace stream message
			shellMsg := types.NewWorkspaceShellOutput(string(data))
			if err := stream.WriteJSON(shellMsg); err != nil {
				s.logger.Debug("workspace stream shell output write error", zap.Error(err))
				return
			}
		}
	}
}

// handleFileTree handles file tree requests via HTTP GET
func (s *Server) handleFileTree(c *gin.Context) {
	path := c.Query("path")
	depth := 1
	if d := c.Query("depth"); d != "" {
		if _, err := json.Number(d).Int64(); err == nil {
			depth = mustParseInt(d)
		}
	}

	tree, err := s.procMgr.GetWorkspaceTracker().GetFileTree(path, depth)
	if err != nil {
		c.JSON(400, types.FileTreeResponse{Error: err.Error()})
		return
	}

	c.JSON(200, types.FileTreeResponse{Root: tree})
}

// handleFileContent handles file content requests via HTTP GET.
// Optional `repo` query param scopes the read to a per-repo subdirectory for
// multi-repo task workspaces; `path` is treated as repo-relative when set.
func (s *Server) handleFileContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, types.FileContentResponse{Error: "path is required"})
		return
	}

	scopedPath, err := s.procMgr.JoinRepoPath(c.Query("repo"), path)
	if err != nil {
		c.JSON(400, types.FileContentResponse{Path: path, Error: err.Error()})
		return
	}

	content, size, isBinary, resolvedPath, err := s.procMgr.GetWorkspaceTracker().GetFileContent(scopedPath)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, process.ErrFileNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, types.FileContentResponse{Path: path, Error: err.Error(), Size: size})
		return
	}

	// Return the repo-relative `path` to the caller — the `repo` scoping is an
	// internal detail; the frontend tracks repository_name separately.
	c.JSON(200, types.FileContentResponse{Path: path, Content: content, Size: size, IsBinary: isBinary, ResolvedPath: resolvedPath})
}

// handleFileContentAtRef handles file content requests at a specific git ref via HTTP GET.
// Optional `repo` query param scopes the read to a per-repo subdirectory for
// multi-repo task workspaces.
func (s *Server) handleFileContentAtRef(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, types.FileContentResponse{Error: "path is required"})
		return
	}

	ref := c.Query("ref")
	if ref == "" {
		c.JSON(400, types.FileContentResponse{Error: "ref is required"})
		return
	}

	repo := c.Query("repo")
	if repo == "" && requiresExplicitRepositoryScope(s.procMgr.RepositoryScopes()) {
		c.JSON(400, types.FileContentResponse{Path: path, Error: "repo is required for multi-repo workspace"})
		return
	}

	tracker, err := s.procMgr.GetWorkspaceTrackerFor(repo)
	if err != nil {
		c.JSON(400, types.FileContentResponse{Path: path, Error: err.Error()})
		return
	}

	content, size, isBinary, err := tracker.GetFileContentAtRef(c.Request.Context(), path, ref)
	if err != nil {
		c.JSON(400, types.FileContentResponse{Path: path, Error: err.Error(), Size: size})
		return
	}

	c.JSON(200, types.FileContentResponse{Path: path, Content: content, Size: size, IsBinary: isBinary})
}

// handleFileUpdate handles file update requests via HTTP POST
func (s *Server) handleFileUpdate(c *gin.Context) {
	var req streams.FileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, streams.FileUpdateResponse{Error: "invalid request: " + err.Error()})
		return
	}

	if req.Path == "" {
		c.JSON(400, streams.FileUpdateResponse{Error: "path is required"})
		return
	}

	if req.Diff == "" {
		c.JSON(400, streams.FileUpdateResponse{Error: "diff is required"})
		return
	}

	scopedPath, err := s.procMgr.JoinRepoPath(req.Repo, req.Path)
	if err != nil {
		c.JSON(400, streams.FileUpdateResponse{
			Path:    req.Path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Apply the diff
	newHash, resolution, err := s.procMgr.GetWorkspaceTracker().ApplyFileDiff(c.Request.Context(), scopedPath, req.Diff, req.OriginalHash, req.DesiredContent)
	if err != nil {
		c.JSON(400, streams.FileUpdateResponse{
			Path:    req.Path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(200, streams.FileUpdateResponse{
		Path:       req.Path,
		Success:    true,
		NewHash:    newHash,
		Resolution: resolution,
	})
}

// handleFileSearch handles file search requests via HTTP GET
func (s *Server) handleFileSearch(c *gin.Context) {
	query := c.Query("q")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed := mustParseInt(l); parsed > 0 {
			limit = parsed
		}
	}

	results := s.procMgr.SearchWorkspaceFileResults(query, limit)
	files := make([]string, 0, len(results))
	for _, result := range results {
		files = append(files, result.Path)
	}

	c.JSON(200, types.FileSearchResponse{Files: files, Results: results})
}

// handleWorkspaceContentSearch searches matching lines across every task repository.
func (s *Server) handleWorkspaceContentSearch(c *gin.Context) {
	query := c.Query("q")
	limit := 20
	if rawLimit := c.Query("limit_per_repo"); rawLimit != "" {
		if parsed := mustParseInt(rawLimit); parsed > 0 {
			limit = parsed
		}
	}

	response, err := s.procMgr.SearchWorkspaceContent(c.Request.Context(), query, limit)
	if err == nil {
		c.JSON(http.StatusOK, response)
		return
	}
	if errors.Is(err, process.ErrContentSearchQueryTooLong) {
		c.JSON(http.StatusBadRequest, types.WorkspaceContentSearchResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, types.WorkspaceContentSearchResponse{Error: err.Error()})
}

// handleFileCreate handles file create requests via HTTP POST.
// Optional `Repo` body field scopes the create to a per-repo subdirectory
// for multi-repo task workspaces; without it a UI-driven create on a
// multi-repo task lands in the bare task root (one level above any actual
// repo) instead of inside the intended repo.
func (s *Server) handleFileCreate(c *gin.Context) {
	var req streams.FileCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, streams.FileCreateResponse{Error: "invalid request: " + err.Error()})
		return
	}

	if req.Path == "" {
		c.JSON(400, streams.FileCreateResponse{Error: "path is required"})
		return
	}

	scopedPath, joinErr := s.procMgr.JoinRepoPath(req.Repo, req.Path)
	if joinErr != nil {
		c.JSON(400, streams.FileCreateResponse{Path: req.Path, Error: joinErr.Error()})
		return
	}

	s.logger.Info("handleFileCreate: creating file", zap.String("path", req.Path), zap.String("repo", req.Repo))

	err := s.procMgr.GetWorkspaceTracker().CreateFile(scopedPath)
	if err != nil {
		c.JSON(400, streams.FileCreateResponse{
			Path:    req.Path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("handleFileCreate: file created successfully", zap.String("path", req.Path))

	c.JSON(200, streams.FileCreateResponse{
		Path:    req.Path,
		Success: true,
	})
}

// handleFileDelete handles file delete requests via HTTP DELETE.
// Optional `repo` query param scopes the delete to a per-repo subdirectory
// for multi-repo task workspaces.
func (s *Server) handleFileDelete(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, streams.FileDeleteResponse{Error: "path is required"})
		return
	}

	scopedPath, joinErr := s.procMgr.JoinRepoPath(c.Query("repo"), path)
	if joinErr != nil {
		c.JSON(400, streams.FileDeleteResponse{Path: path, Error: joinErr.Error()})
		return
	}

	err := s.procMgr.GetWorkspaceTracker().DeleteFile(scopedPath)
	if err != nil {
		c.JSON(400, streams.FileDeleteResponse{
			Path:    path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(200, streams.FileDeleteResponse{
		Path:    path,
		Success: true,
	})
}

// handleFileRename handles file/directory rename requests via HTTP POST.
// Optional `Repo` body field scopes BOTH old_path and new_path to a per-repo
// subdirectory; cross-repo renames aren't supported (the rename collapses
// to a same-repo move).
func (s *Server) handleFileRename(c *gin.Context) {
	var req streams.FileRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, streams.FileRenameResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.OldPath == "" {
		c.JSON(400, streams.FileRenameResponse{Error: "old_path is required"})
		return
	}

	if req.NewPath == "" {
		c.JSON(400, streams.FileRenameResponse{Error: "new_path is required"})
		return
	}

	scopedOld, oldErr := s.procMgr.JoinRepoPath(req.Repo, req.OldPath)
	if oldErr != nil {
		c.JSON(400, streams.FileRenameResponse{OldPath: req.OldPath, NewPath: req.NewPath, Error: oldErr.Error()})
		return
	}
	scopedNew, newErr := s.procMgr.JoinRepoPath(req.Repo, req.NewPath)
	if newErr != nil {
		c.JSON(400, streams.FileRenameResponse{OldPath: req.OldPath, NewPath: req.NewPath, Error: newErr.Error()})
		return
	}

	err := s.procMgr.GetWorkspaceTracker().RenameFile(scopedOld, scopedNew)
	if err != nil {
		c.JSON(400, streams.FileRenameResponse{
			OldPath: req.OldPath,
			NewPath: req.NewPath,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(200, streams.FileRenameResponse{
		OldPath: req.OldPath,
		NewPath: req.NewPath,
		Success: true,
	})
}

func (s *Server) handleWorkspaceStreamInput(stream *workspaceStreamConn, shell io.Writer, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
			var msg types.WorkspaceStreamMessage
			if err := stream.ReadJSON(&msg); err != nil {
				s.logger.Debug("workspace stream WebSocket read error", zap.Error(err))
				return
			}
			switch msg.Type {
			case types.WorkspaceMessageTypeShellInput:
				if shell != nil {
					if _, err := shell.Write([]byte(msg.Data)); err != nil {
						s.logger.Debug("shell write error", zap.Error(err))
					}
				}
			case types.WorkspaceMessageTypeShellResize:
				s.logger.Debug("shell resize requested", zap.Int("cols", msg.Cols), zap.Int("rows", msg.Rows))
			case types.WorkspaceMessageTypePing:
				pongMsg := types.NewWorkspacePong()
				if err := stream.WriteJSON(pongMsg); err != nil {
					s.logger.Debug("workspace stream pong write error", zap.Error(err))
					return
				}
			}
		}
	}
}

// mustParseInt parses a string to int, returns 0 on error
func mustParseInt(s string) int {
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return 0
	}
	return n
}
