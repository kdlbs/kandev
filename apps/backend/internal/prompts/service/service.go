package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

var (
	ErrPromptNotFound       = errors.New("prompt not found")
	ErrInvalidPrompt        = errors.New("invalid prompt")
	ErrPromptAlreadyExists  = errors.New("prompt with this name already exists")
	ErrPromptListLimit      = errors.New("prompt list limit exceeded")
	ErrPromptReferenceLimit = errors.New("prompt reference limit exceeded")
)

type Service struct {
	repo promptstore.Repository
}

type PromptReferenceExpansion struct {
	Name    string
	Content string
}

const (
	maxPromptReferenceDepth      = 8
	maxPromptNameBytes           = 512
	maxPromptContentBytes        = 1 << 20
	maxPromptReferenceNames      = 2000
	maxPromptReferenceExpansions = 128
	maxPromptExpansionBytes      = 4 << 20
	// Candidate budgets cap repository materialization before the resolver
	// builds its trie. They are larger than the final expansion budget so
	// unrelated saved prompts can be inspected without allowing multi-gigabyte
	// allocations.
	maxPromptReferenceCandidateNameBytes    = 1 << 20
	maxPromptReferenceCandidateContentBytes = 16 << 20
)

func NewService(repo promptstore.Repository) *Service {
	return &Service{repo: repo}
}
func validPromptFields(name, content string) bool {
	return name != "" &&
		len(name) <= maxPromptNameBytes &&
		content != "" &&
		len(content) <= maxPromptContentBytes
}

func (s *Service) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	return s.repo.ListPrompts(ctx)
}

// GetPromptByName returns one saved prompt by its exact name after trimming
// surrounding whitespace. Missing or blank names map to ErrPromptNotFound so
// callers do not need to depend on the repository's sql.ErrNoRows result.
func (s *Service) GetPromptByName(ctx context.Context, name string) (*models.Prompt, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrPromptNotFound
	}
	prompt, err := s.repo.GetPromptByName(ctx, name)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && prompt == nil) {
		return nil, ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *Service) CreatePrompt(ctx context.Context, name, content string) (*models.Prompt, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if !validPromptFields(name, content) {
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
		if errors.Is(err, promptstore.ErrPromptListLimit) {
			return nil, ErrPromptListLimit
		}
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
		if trimmed == "" || len(trimmed) > maxPromptNameBytes {
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
		if trimmed == "" || len(trimmed) > maxPromptContentBytes {
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
	if len(content) > maxPromptContentBytes {
		return nil, ErrInvalidPrompt
	}
	if !strings.Contains(content, "@") {
		return nil, nil
	}
	prompts, truncated, err := s.repo.ListPromptsForReferenceExpansion(
		ctx,
		maxPromptReferenceNames,
		maxPromptNameBytes,
		maxPromptContentBytes,
		maxPromptReferenceCandidateNameBytes,
		maxPromptReferenceCandidateContentBytes,
	)
	if err != nil {
		if errors.Is(err, promptstore.ErrPromptReferenceCandidateLimit) {
			return nil, ErrPromptReferenceLimit
		}
		return nil, err
	}
	if truncated {
		return nil, ErrPromptReferenceLimit
	}
	trie := &promptReferenceTrieNode{}
	validPromptCount := 0
	for _, prompt := range prompts {
		if prompt == nil || prompt.Name == "" ||
			len(prompt.Name) > maxPromptNameBytes ||
			len(prompt.Content) > maxPromptContentBytes {
			continue
		}
		validPromptCount++
		insertPromptReference(trie, prompt)
	}
	if validPromptCount > maxPromptReferenceNames {
		return nil, ErrPromptReferenceLimit
	}
	expansions := make([]PromptReferenceExpansion, 0)
	if err := collectPromptReferences(content, trie, map[string]bool{}, map[string]bool{}, &expansions, 0); err != nil {
		return nil, err
	}
	return expansions, nil
}

type promptReferenceTrieNode struct {
	children map[byte]*promptReferenceTrieNode
	prompt   *models.Prompt
}

func insertPromptReference(root *promptReferenceTrieNode, prompt *models.Prompt) {
	node := root
	for i := 0; i < len(prompt.Name); i++ {
		if node.children == nil {
			node.children = make(map[byte]*promptReferenceTrieNode)
		}
		child := node.children[prompt.Name[i]]
		if child == nil {
			child = &promptReferenceTrieNode{}
			node.children[prompt.Name[i]] = child
		}
		node = child
	}
	node.prompt = prompt
}

func collectPromptReferences(content string, trie *promptReferenceTrieNode, stack, seen map[string]bool, expansions *[]PromptReferenceExpansion, depth int) error {
	for index := 0; index < len(content); {
		if content[index] != '@' || !isPromptReferenceStart(content, index) {
			index++
			continue
		}
		prompt, referenceEnd, ok := matchPromptReference(content, index, trie)
		if !ok || stack[prompt.Name] || depth >= maxPromptReferenceDepth {
			index = referenceEnd
			continue
		}
		if !seen[prompt.Name] {
			if len(*expansions) >= maxPromptReferenceExpansions {
				return ErrPromptReferenceLimit
			}
			expansionBytes := 0
			for _, expansion := range *expansions {
				expansionBytes += len(expansion.Name) + len(expansion.Content)
			}
			if expansionBytes+len(prompt.Name)+len(prompt.Content) > maxPromptExpansionBytes {
				return ErrPromptReferenceLimit
			}
			seen[prompt.Name] = true
			*expansions = append(*expansions, PromptReferenceExpansion{Name: prompt.Name, Content: prompt.Content})
			stack[prompt.Name] = true
			if err := collectPromptReferences(prompt.Content, trie, stack, seen, expansions, depth+1); err != nil {
				return err
			}
			delete(stack, prompt.Name)
		}
		index = referenceEnd
	}
	return nil
}

func matchPromptReference(content string, index int, trie *promptReferenceTrieNode) (*models.Prompt, int, bool) {
	referenceStart := index + 1
	node := trie
	var prompt *models.Prompt
	referenceEnd := referenceStart
	for position := referenceStart; position < len(content); position++ {
		child := node.children[content[position]]
		if child == nil {
			break
		}
		node = child
		candidateEnd := position + 1
		if node.prompt != nil &&
			(candidateEnd == len(content) || !isPromptReferenceNameCharAt(content, candidateEnd)) {
			prompt = node.prompt
			referenceEnd = candidateEnd
		}
	}
	if prompt == nil {
		return nil, referenceStart, false
	}
	return prompt, referenceEnd, true
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

func isPromptReferenceNameCharAt(content string, index int) bool {
	r, _ := utf8.DecodeRuneInString(content[index:])
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '-' || r == '_'
}
