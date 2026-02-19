package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/editors/models"
	"github.com/kandev/kandev/internal/editors/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

var (
	ErrEditorNotFound      = errors.New("editor not found")
	ErrEditorUnavailable   = errors.New("editor not available")
	ErrEditorConfigInvalid = errors.New("editor configuration invalid")
	ErrWorkspaceNotFound   = errors.New("workspace path not found")
)

// Editor kind constants for custom editor types.
const (
	editorKindCustomRemoteSSH = "custom_remote_ssh"
	editorKindCustomHostedURL = "custom_hosted_url"
	editorKindCustomCommand   = "custom_command"
)

type UserSettingsProvider interface {
	GetUserSettings(ctx context.Context) (*usermodels.UserSettings, error)
	ClearDefaultEditorID(ctx context.Context, editorID string) error
}

type Service struct {
	repo         store.Repository
	taskRepo     repository.Repository
	userSettings UserSettingsProvider
}

func NewService(repo store.Repository, taskRepo repository.Repository, userSettings UserSettingsProvider) *Service {
	return &Service{
		repo:         repo,
		taskRepo:     taskRepo,
		userSettings: userSettings,
	}
}

func (s *Service) ListEditors(ctx context.Context) ([]*models.Editor, error) {
	return s.repo.ListEditors(ctx)
}

type OpenEditorInput struct {
	SessionID  string
	EditorID   string
	EditorType string
	FilePath   string
	Line       int
	Column     int
}

func (s *Service) OpenEditor(ctx context.Context, input OpenEditorInput) (string, error) {
	if input.SessionID == "" {
		return "", ErrEditorConfigInvalid
	}
	session, err := s.taskRepo.GetTaskSession(ctx, input.SessionID)
	if err != nil {
		return "", err
	}
	worktreePath, err := s.resolveSessionPath(ctx, session)
	if err != nil {
		return "", err
	}

	settings, err := s.userSettings.GetUserSettings(ctx)
	if err != nil {
		return "", err
	}
	if settings == nil {
		return "", ErrEditorConfigInvalid
	}

	editor, err := s.resolveEditor(ctx, input.EditorID, input.EditorType, settings.DefaultEditorID)
	if err != nil {
		return "", err
	}

	absPath, err := s.resolveFilePath(worktreePath, input.FilePath)
	if err != nil {
		return "", err
	}

	return s.dispatchEditorKind(editor, worktreePath, absPath, input.Line, input.Column)
}

func (s *Service) dispatchEditorKind(editor *models.Editor, worktreePath, absPath string, line, column int) (string, error) {
	switch editor.Kind {
	case editorKindCustomRemoteSSH:
		return openRemoteSSHEditor(editor, absPath, line, column)
	case editorKindCustomHostedURL:
		cfg, err := parseHostedURLConfig(editor.Config)
		if err != nil {
			return "", err
		}
		return buildHostedURL(cfg.URL, absPath, line, column)
	case editorKindCustomCommand:
		cfg, err := parseCommandConfig(editor.Config)
		if err != nil {
			return "", err
		}
		return launchCommand(cfg.Command, worktreePath, absPath, line, column)
	default:
		return openBuiltinEditor(editor, absPath, line, column)
	}
}

func openRemoteSSHEditor(editor *models.Editor, absPath string, line, column int) (string, error) {
	cfg, err := parseRemoteSSHConfig(editor.Config)
	if err != nil {
		return "", err
	}
	scheme := cfg.Scheme
	if scheme == "" {
		scheme = editor.Scheme
	}
	return buildRemoteSSHURL(scheme, cfg.User, cfg.Host, absPath, line, column), nil
}

func openBuiltinEditor(editor *models.Editor, absPath string, line, column int) (string, error) {
	if !editor.Installed {
		return "", ErrEditorUnavailable
	}
	if editor.Command == "" {
		return "", ErrEditorConfigInvalid
	}
	args := buildLocalArgs(editor.Type, absPath, line, column)
	cmd := exec.Command(editor.Command, args...)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch failed: %w", err)
	}
	return "", nil
}

func (s *Service) OpenFolder(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrEditorConfigInvalid
	}
	session, err := s.taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	worktreePath, err := s.resolveSessionPath(ctx, session)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", worktreePath)
	case "linux":
		cmd = exec.Command("xdg-open", worktreePath)
	case "windows":
		cmd = exec.Command("explorer", worktreePath)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open folder: %w", err)
	}
	return nil
}

func (s *Service) resolveEditor(ctx context.Context, editorID, editorType, fallbackID string) (*models.Editor, error) {
	if editorID == "" {
		editorID = strings.TrimSpace(fallbackID)
	}
	if editorID != "" {
		editor, err := s.repo.GetEditorByID(ctx, editorID)
		if err != nil || editor == nil {
			return nil, ErrEditorNotFound
		}
		if !editor.Enabled {
			return nil, ErrEditorUnavailable
		}
		return editor, nil
	}
	if editorType == "" {
		editors, err := s.repo.ListEditors(ctx)
		if err != nil {
			return nil, err
		}
		for _, editor := range editors {
			if editor.Enabled {
				return editor, nil
			}
		}
		return nil, ErrEditorNotFound
	}
	editor, err := s.repo.GetEditorByType(ctx, editorType)
	if err != nil || editor == nil {
		return nil, ErrEditorNotFound
	}
	if !editor.Enabled {
		return nil, ErrEditorUnavailable
	}
	return editor, nil
}

func (s *Service) resolveSessionPath(ctx context.Context, session *taskmodels.TaskSession) (string, error) {
	if session == nil {
		return "", ErrWorkspaceNotFound
	}
	if len(session.Worktrees) > 0 && session.Worktrees[0].WorktreePath != "" {
		return session.Worktrees[0].WorktreePath, nil
	}
	if session.RepositoryID != "" {
		repo, err := s.taskRepo.GetRepository(ctx, session.RepositoryID)
		if err != nil {
			return "", err
		}
		if repo != nil && repo.LocalPath != "" {
			return repo.LocalPath, nil
		}
	}
	return "", ErrWorkspaceNotFound
}

func (s *Service) resolveFilePath(worktreePath, filePath string) (string, error) {
	if worktreePath == "" {
		return "", ErrWorkspaceNotFound
	}
	if filePath == "" {
		return worktreePath, nil
	}
	clean := filepath.Clean(filePath)
	abs := filepath.Join(worktreePath, clean)
	rel, err := filepath.Rel(worktreePath, abs)
	if err != nil {
		return "", ErrEditorConfigInvalid
	}
	if strings.HasPrefix(rel, "..") {
		return "", ErrEditorConfigInvalid
	}
	return abs, nil
}

func buildRemoteSSHURL(scheme, user, host, path string, line, column int) string {
	if scheme == "" {
		scheme = "vscode"
	}
	remoteHost := host
	if user != "" {
		remoteHost = fmt.Sprintf("%s@%s", user, host)
	}
	location := path
	if line > 0 {
		location = fmt.Sprintf("%s:%d", path, line)
		if column > 0 {
			location = fmt.Sprintf("%s:%d", location, column)
		}
	}
	return fmt.Sprintf("%s://vscode-remote/ssh-remote+%s:%s", scheme, remoteHost, location)
}

func buildHostedURL(baseURL, path string, line, column int) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", ErrEditorConfigInvalid
	}
	query := parsed.Query()
	if path != "" {
		location := path
		if line > 0 {
			location = fmt.Sprintf("%s:%d", path, line)
			if column > 0 {
				location = fmt.Sprintf("%s:%d", location, column)
			}
		}
		query.Set("file", location)
	} else {
		query.Set("folder", "/")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildLocalArgs(editorType, path string, line, column int) []string {
	if path == "" {
		return nil
	}
	normalized := strings.TrimSpace(editorType)
	switch normalized {
	case "vscode", "cursor", "windsurf":
		if line > 0 {
			location := path + ":" + strconv.Itoa(line)
			if column > 0 {
				location += ":" + strconv.Itoa(column)
			}
			return []string{"--goto", location}
		}
		return []string{path}
	default:
		return []string{path}
	}
}

type CreateEditorInput struct {
	Name    string
	Kind    string
	Config  json.RawMessage
	Enabled *bool
}

type UpdateEditorInput struct {
	EditorID string
	Name     *string
	Kind     *string
	Config   json.RawMessage
	Enabled  *bool
}

func (s *Service) CreateEditor(ctx context.Context, input CreateEditorInput) (*models.Editor, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrEditorConfigInvalid
	}
	if !isCustomKind(input.Kind) {
		return nil, ErrEditorConfigInvalid
	}
	editorID := uuid.New().String()
	editor := &models.Editor{
		ID:        editorID,
		Type:      "custom:" + editorID,
		Name:      name,
		Kind:      input.Kind,
		Command:   "",
		Scheme:    "",
		Config:    input.Config,
		Installed: true,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if input.Enabled != nil {
		editor.Enabled = *input.Enabled
	}
	if err := validateEditorConfig(editor); err != nil {
		return nil, err
	}
	if err := s.repo.CreateEditor(ctx, editor); err != nil {
		return nil, err
	}
	return editor, nil
}

func (s *Service) UpdateEditor(ctx context.Context, input UpdateEditorInput) (*models.Editor, error) {
	if input.EditorID == "" {
		return nil, ErrEditorConfigInvalid
	}
	editor, err := s.repo.GetEditorByID(ctx, input.EditorID)
	if err != nil || editor == nil {
		return nil, ErrEditorNotFound
	}
	if !isCustomKind(editor.Kind) {
		return nil, ErrEditorConfigInvalid
	}
	if input.Name != nil {
		editor.Name = strings.TrimSpace(*input.Name)
	}
	if input.Kind != nil {
		if !isCustomKind(*input.Kind) {
			return nil, ErrEditorConfigInvalid
		}
		editor.Kind = *input.Kind
	}
	if len(input.Config) > 0 {
		editor.Config = input.Config
	}
	if input.Enabled != nil {
		editor.Enabled = *input.Enabled
	}
	if err := validateEditorConfig(editor); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateEditor(ctx, editor); err != nil {
		return nil, err
	}
	return editor, nil
}

func (s *Service) DeleteEditor(ctx context.Context, editorID string) error {
	if editorID == "" {
		return ErrEditorConfigInvalid
	}
	editor, err := s.repo.GetEditorByID(ctx, editorID)
	if err != nil {
		return err
	}
	if editor == nil {
		return ErrEditorNotFound
	}
	if !isCustomKind(editor.Kind) {
		return ErrEditorConfigInvalid
	}
	if err := s.repo.DeleteEditor(ctx, editorID); err != nil {
		return err
	}
	return s.userSettings.ClearDefaultEditorID(ctx, editorID)
}

type commandConfig struct {
	Command string `json:"command"`
}

type remoteSSHConfig struct {
	Host   string `json:"host"`
	User   string `json:"user"`
	Scheme string `json:"scheme"`
}

type hostedURLConfig struct {
	URL string `json:"url"`
}

func parseCommandConfig(data json.RawMessage) (commandConfig, error) {
	if len(data) == 0 {
		return commandConfig{}, ErrEditorConfigInvalid
	}
	var cfg commandConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return commandConfig{}, ErrEditorConfigInvalid
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return commandConfig{}, ErrEditorConfigInvalid
	}
	return cfg, nil
}

func parseRemoteSSHConfig(data json.RawMessage) (remoteSSHConfig, error) {
	var cfg remoteSSHConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return remoteSSHConfig{}, ErrEditorConfigInvalid
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return remoteSSHConfig{}, ErrEditorConfigInvalid
	}
	return cfg, nil
}

func parseHostedURLConfig(data json.RawMessage) (hostedURLConfig, error) {
	var cfg hostedURLConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return hostedURLConfig{}, ErrEditorConfigInvalid
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return hostedURLConfig{}, ErrEditorConfigInvalid
	}
	return cfg, nil
}

func launchCommand(commandTemplate, worktreePath, absPath string, line, column int) (string, error) {
	expanded := expandCommandPlaceholders(commandTemplate, worktreePath, absPath, line, column)
	parts := strings.Fields(expanded)
	if len(parts) == 0 {
		return "", ErrEditorConfigInvalid
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	if worktreePath != "" {
		cmd.Dir = worktreePath
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch failed: %w", err)
	}
	return "", nil
}

func expandCommandPlaceholders(template, worktreePath, absPath string, line, column int) string {
	relPath := absPath
	if worktreePath != "" && absPath != "" {
		if relative, err := filepath.Rel(worktreePath, absPath); err == nil {
			relPath = relative
		}
	}
	replacements := map[string]string{
		"{cwd}":    worktreePath,
		"{file}":   absPath,
		"{rel}":    relPath,
		"{line}":   strconv.Itoa(line),
		"{column}": strconv.Itoa(column),
	}
	result := template
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}
	return result
}

func isCustomKind(kind string) bool {
	switch kind {
	case editorKindCustomCommand, editorKindCustomRemoteSSH, editorKindCustomHostedURL:
		return true
	default:
		return false
	}
}

func validateEditorConfig(editor *models.Editor) error {
	if editor == nil {
		return ErrEditorConfigInvalid
	}
	switch editor.Kind {
	case editorKindCustomCommand:
		_, err := parseCommandConfig(editor.Config)
		return err
	case editorKindCustomRemoteSSH:
		_, err := parseRemoteSSHConfig(editor.Config)
		return err
	case editorKindCustomHostedURL:
		_, err := parseHostedURLConfig(editor.Config)
		return err
	default:
		return ErrEditorConfigInvalid
	}
}
