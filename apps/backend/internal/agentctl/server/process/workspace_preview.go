package process

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	// MaxWorkspacePreviewContentBytes bounds one current-buffer overlay.
	MaxWorkspacePreviewContentBytes = 5 * 1024 * 1024
	maxWorkspacePreviewOverlays     = 32
)

var (
	ErrWorkspacePreviewContentTooLarge = errors.New("workspace preview content exceeds 5 MiB")
	ErrWorkspacePreviewPathInvalid     = errors.New("workspace preview path is invalid")
	ErrWorkspacePreviewTypeUnsupported = errors.New("workspace preview only supports HTML files")
	ErrWorkspacePreviewUnavailable     = errors.New("workspace preview server is unavailable")
)

// WorkspacePreviewRequest is the current editor buffer to publish for a
// workspace preview. Repo is an optional repository-relative scope.
type WorkspacePreviewRequest struct {
	Repo    string
	Path    string
	Content string
}

// WorkspacePreviewResponse identifies the ephemeral static server and the
// published entry document.
type WorkspacePreviewResponse struct {
	Port    int
	Path    string
	Version uint64
}

type workspacePreviewOverlay struct {
	content   string
	version   uint64
	published uint64
}

type workspacePreviewRoot struct {
	path     string
	port     int
	server   *http.Server
	overlays map[string]workspacePreviewOverlay
}

// WorkspacePreviewManager owns loopback static servers and in-memory entry
// overlays for one agentctl instance.
type WorkspacePreviewManager struct {
	mu       sync.Mutex
	logger   *logger.Logger
	roots    map[string]*workspacePreviewRoot
	sequence uint64
	closed   bool
}

// NewWorkspacePreviewManager creates a manager for one agentctl instance.
func NewWorkspacePreviewManager(log *logger.Logger) *WorkspacePreviewManager {
	return &WorkspacePreviewManager{
		logger: log.WithFields(zap.String("component", "workspace-preview")),
		roots:  make(map[string]*workspacePreviewRoot),
	}
}

// Publish stores an entry-document overlay and starts or reuses the static
// server for its selected workspace root.
func (m *WorkspacePreviewManager) Publish(root string, req WorkspacePreviewRequest) (WorkspacePreviewResponse, error) {
	if len([]byte(req.Content)) > MaxWorkspacePreviewContentBytes {
		return WorkspacePreviewResponse{}, ErrWorkspacePreviewContentTooLarge
	}
	relPath, err := validateWorkspacePreviewEntryPath(req.Path)
	if err != nil {
		return WorkspacePreviewResponse{}, err
	}
	canonicalRoot, err := canonicalPreviewDirectory(root)
	if err != nil {
		return WorkspacePreviewResponse{}, fmt.Errorf("resolve workspace preview root: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspacePreviewResponse{}, ErrWorkspacePreviewUnavailable
	}
	previewRoot := m.roots[canonicalRoot]
	if previewRoot == nil {
		previewRoot, err = m.startRootLocked(canonicalRoot)
		if err != nil {
			return WorkspacePreviewResponse{}, err
		}
		m.roots[canonicalRoot] = previewRoot
	}

	previous := previewRoot.overlays[relPath]
	version := previous.version + 1
	m.sequence++
	previewRoot.overlays[relPath] = workspacePreviewOverlay{
		content:   req.Content,
		version:   version,
		published: m.sequence,
	}
	m.evictOldestOverlayLocked()
	m.logger.Info("workspace preview published",
		zap.String("root", canonicalRoot),
		zap.String("path", relPath),
		zap.Int("bytes", len([]byte(req.Content))),
		zap.Uint64("version", version),
	)
	return WorkspacePreviewResponse{Port: previewRoot.port, Path: "/" + relPath, Version: version}, nil
}

func (m *WorkspacePreviewManager) startRootLocked(root string) (*workspacePreviewRoot, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start workspace preview listener: %w", err)
	}
	previewRoot := &workspacePreviewRoot{
		path:     root,
		port:     listener.Addr().(*net.TCPAddr).Port,
		overlays: make(map[string]workspacePreviewOverlay),
	}
	previewRoot.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.serveRootRequest(previewRoot, w, r)
		}),
	}
	go func() {
		if serveErr := previewRoot.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			m.logger.Warn("workspace preview server stopped unexpectedly",
				zap.String("root", root), zap.Int("port", previewRoot.port), zap.Error(serveErr))
		}
	}()
	m.logger.Info("workspace preview server started",
		zap.String("root", root), zap.Int("port", previewRoot.port))
	return previewRoot, nil
}

func (m *WorkspacePreviewManager) evictOldestOverlayLocked() {
	count := 0
	var oldestRoot *workspacePreviewRoot
	var oldestPath string
	var oldestSequence uint64
	for _, root := range m.roots {
		for path, overlay := range root.overlays {
			count++
			if oldestRoot == nil || overlay.published < oldestSequence {
				oldestRoot = root
				oldestPath = path
				oldestSequence = overlay.published
			}
		}
	}
	if count <= maxWorkspacePreviewOverlays || oldestRoot == nil {
		return
	}
	delete(oldestRoot.overlays, oldestPath)
	m.logger.Debug("workspace preview overlay evicted",
		zap.String("root", oldestRoot.path), zap.String("path", oldestPath))
}

func (m *WorkspacePreviewManager) serveRootRequest(root *workspacePreviewRoot, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	relPath, err := workspacePreviewRequestPath(r.URL)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	m.mu.Lock()
	overlay, hasOverlay := root.overlays[relPath]
	m.mu.Unlock()
	if hasOverlay {
		serveWorkspacePreviewBytes(w, r, relPath, []byte(overlay.content))
		return
	}

	filePath, err := resolveWorkspacePreviewFile(root.path, relPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	setWorkspacePreviewContentType(w, relPath)
	http.ServeContent(w, r, filepath.Base(filePath), info.ModTime(), file)
}

func serveWorkspacePreviewBytes(w http.ResponseWriter, r *http.Request, path string, content []byte) {
	setWorkspacePreviewContentType(w, path)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(content)
}

func setWorkspacePreviewContentType(w http.ResponseWriter, path string) {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
}

func resolveWorkspacePreviewFile(root, relPath string) (string, error) {
	rootPath, err := canonicalPreviewDirectory(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootPath, filepath.FromSlash(relPath))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !previewPathWithinRoot(rootPath, resolved) {
		return "", ErrWorkspacePreviewPathInvalid
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return resolved, nil
}

func workspacePreviewRequestPath(parsed *url.URL) (string, error) {
	escaped := parsed.EscapedPath()
	decoded, err := url.PathUnescape(escaped)
	if err != nil || !strings.HasPrefix(decoded, "/") {
		return "", ErrWorkspacePreviewPathInvalid
	}
	relPath := strings.TrimPrefix(decoded, "/")
	if !validWorkspacePreviewRelativePath(relPath) {
		return "", ErrWorkspacePreviewPathInvalid
	}
	return relPath, nil
}

func validateWorkspacePreviewEntryPath(path string) (string, error) {
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return "", ErrWorkspacePreviewPathInvalid
	}
	path = decodedPath
	if !validWorkspacePreviewRelativePath(path) {
		return "", ErrWorkspacePreviewPathInvalid
	}
	ext := strings.ToLower(filepath.Ext(filepath.FromSlash(path)))
	if ext != ".html" && ext != ".htm" {
		return "", ErrWorkspacePreviewTypeUnsupported
	}
	return filepath.ToSlash(path), nil
}

func validWorkspacePreviewRelativePath(path string) bool {
	if path == "" || strings.ContainsRune(path, '\x00') || filepath.IsAbs(filepath.FromSlash(path)) {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func canonicalPreviewDirectory(path string) (string, error) {
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("preview root is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func previewPathWithinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Close stops every preview server and releases its loopback listener.
func (m *WorkspacePreviewManager) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	servers := make([]*http.Server, 0, len(m.roots))
	for _, root := range m.roots {
		servers = append(servers, root.server)
	}
	m.roots = make(map[string]*workspacePreviewRoot)
	m.mu.Unlock()

	var errs []error
	for _, server := range servers {
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, err)
			_ = server.Close()
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
