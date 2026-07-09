package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

var (
	ErrPromptNotFound      = errors.New("prompt not found")
	ErrInvalidPrompt       = errors.New("invalid prompt")
	ErrPromptAlreadyExists = errors.New("prompt with this name already exists")
)

type Service struct {
	repo promptstore.Repository
}

type PromptReferenceExpansion struct {
	Name    string
	Content string
}

const maxPromptReferenceDepth = 8

func NewService(repo promptstore.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	return s.repo.ListPrompts(ctx)
}

func (s *Service) CreatePrompt(ctx context.Context, name, content string) (*models.Prompt, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if name == "" || content == "" {
		return nil, ErrInvalidPrompt
	}
	if err := s.assertNameAvailable(ctx, name, ""); err != nil {
		return nil, err
	}
	prompt := &models.Prompt{
		Name:    name,
		Content: content,
	}
	if err := s.repo.CreatePrompt(ctx, prompt); err != nil {
		return nil, translateNameConflict(err)
	}
	return prompt, nil
}

// translateNameConflict closes the TOCTOU window between assertNameAvailable
// and the write: the SQLite UNIQUE index on custom_prompts.name is the only
// authoritative guard, and a concurrent write that loses the race surfaces a
// "UNIQUE constraint failed" driver error which would otherwise fall through
// to a generic 500.
func translateNameConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrPromptAlreadyExists
	}
	return err
}

// assertNameAvailable returns ErrPromptAlreadyExists if a different prompt with
// the given name already exists. excludeID lets callers exclude the prompt
// being updated so unchanged-name saves do not falsely trip.
func (s *Service) assertNameAvailable(ctx context.Context, name, excludeID string) error {
	existing, err := s.repo.GetPromptByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if existing.ID == excludeID {
		return nil
	}
	return ErrPromptAlreadyExists
}

func (s *Service) UpdatePrompt(ctx context.Context, promptID string, name *string, content *string) (*models.Prompt, error) {
	prompt, err := s.repo.GetPromptByID(ctx, promptID)
	if err != nil {
		return nil, ErrPromptNotFound
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, ErrInvalidPrompt
		}
		if trimmed != prompt.Name {
			if err := s.assertNameAvailable(ctx, trimmed, prompt.ID); err != nil {
				return nil, err
			}
		}
		prompt.Name = trimmed
	}
	if content != nil {
		trimmed := strings.TrimSpace(*content)
		if trimmed == "" {
			return nil, ErrInvalidPrompt
		}
		prompt.Content = trimmed
	}
	if err := s.repo.UpdatePrompt(ctx, prompt); err != nil {
		return nil, translateNameConflict(err)
	}
	return prompt, nil
}

func (s *Service) DeletePrompt(ctx context.Context, promptID string) error {
	if promptID == "" {
		return ErrInvalidPrompt
	}
	return s.repo.DeletePrompt(ctx, promptID)
}

// ResolvePromptContent returns the stored prompt content by name, falling back
// to fallback when the row is missing or temporarily unreadable.
func (s *Service) ResolvePromptContent(ctx context.Context, name, fallback string) string {
	prompt, err := s.repo.GetPromptByName(ctx, strings.TrimSpace(name))
	if err != nil || prompt == nil {
		return fallback
	}
	content := strings.TrimSpace(prompt.Content)
	if content == "" {
		return fallback
	}
	return content
}

func (s *Service) ResolvePromptReferences(ctx context.Context, content string) ([]PromptReferenceExpansion, error) {
	prompts, err := s.repo.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*models.Prompt, len(prompts))
	for _, prompt := range prompts {
		if prompt == nil || prompt.Name == "" {
			continue
		}
		byName[prompt.Name] = prompt
	}
	expansions := make([]PromptReferenceExpansion, 0)
	collectPromptReferences(content, byName, map[string]bool{}, map[string]bool{}, &expansions, 0)
	return expansions, nil
}

func collectPromptReferences(content string, byName map[string]*models.Prompt, stack, seen map[string]bool, expansions *[]PromptReferenceExpansion, depth int) {
	for index := 0; index < len(content); {
		if content[index] != '@' || !isPromptReferenceStart(content, index) {
			index++
			continue
		}
		nameStart := index + 1
		nameEnd := nameStart
		for nameEnd < len(content) && isPromptReferenceNameChar(content[nameEnd]) {
			nameEnd++
		}
		name := content[nameStart:nameEnd]
		prompt := byName[name]
		if name == "" || prompt == nil || stack[prompt.Name] || depth >= maxPromptReferenceDepth {
			if nameEnd == index {
				index++
			} else {
				index = nameEnd
			}
			continue
		}
		if !seen[prompt.Name] {
			seen[prompt.Name] = true
			*expansions = append(*expansions, PromptReferenceExpansion{Name: prompt.Name, Content: prompt.Content})
			stack[prompt.Name] = true
			collectPromptReferences(prompt.Content, byName, stack, seen, expansions, depth+1)
			delete(stack, prompt.Name)
		}
		index = nameEnd
	}
}

func isPromptReferenceStart(content string, index int) bool {
	if index == 0 {
		return true
	}
	switch content[index-1] {
	case ' ', '\n', '\t', '\r':
		return true
	default:
		return false
	}
}

func isPromptReferenceNameChar(ch byte) bool {
	return ch >= 'a' && ch <= 'z' ||
		ch >= 'A' && ch <= 'Z' ||
		ch >= '0' && ch <= '9' ||
		ch == '-' ||
		ch == '_'
}
