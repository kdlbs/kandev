package service

import (
	"context"
	"strings"
	"unicode/utf8"
)

// PromptReferenceText is text that may reference saved prompts, attributed to
// an owner (for example a workflow ID) so references can be counted per owner.
type PromptReferenceText struct {
	Owner string
	Text  string
}

// UnresolvedPromptReferences returns, for each text, the distinct reference
// tokens that resolve to no saved prompt, in order of first appearance. A
// token starts at an @ in reference position and spans the following name
// characters; it is unresolved when expansion would find no prompt there.
func (s *Service) UnresolvedPromptReferences(ctx context.Context, texts []string) ([][]string, error) {
	missing := make([][]string, len(texts))
	if !anyContainsAt(texts) {
		return missing, nil
	}
	trie, err := s.loadReferenceTrie(ctx)
	if err != nil {
		return nil, err
	}
	for i, text := range texts {
		missing[i] = unresolvedReferences(text, trie)
	}
	return missing, nil
}

// referencedPromptNames counts, for each requested name, how many distinct
// owners have a text containing a reference that resolves to that name under
// the same longest-match rules as expansion.
func (s *Service) referencedPromptNames(ctx context.Context, texts []PromptReferenceText, names []string) (map[string]int, error) {
	counts := make(map[string]int, len(names))
	if len(names) == 0 || len(texts) == 0 {
		return counts, nil
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	trie, err := s.loadReferenceTrie(ctx)
	if err != nil {
		return nil, err
	}
	owners := make(map[string]map[string]bool)
	for _, text := range texts {
		for _, name := range resolvedReferenceNames(text.Text, trie) {
			if !wanted[name] {
				continue
			}
			if owners[name] == nil {
				owners[name] = make(map[string]bool)
			}
			owners[name][text.Owner] = true
		}
	}
	for name, set := range owners {
		counts[name] = len(set)
	}
	return counts, nil
}

func anyContainsAt(texts []string) bool {
	for _, text := range texts {
		if strings.Contains(text, "@") {
			return true
		}
	}
	return false
}

func unresolvedReferences(text string, trie *promptReferenceTrieNode) []string {
	var missing []string
	seen := map[string]bool{}
	for index := 0; index < len(text); index++ {
		if text[index] != '@' || !isPromptReferenceStart(text, index) {
			continue
		}
		if _, end, ok := matchPromptReference(text, index, trie); ok {
			index = end - 1
			continue
		}
		token := referenceToken(text, index+1)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		missing = append(missing, token)
	}
	return missing
}

func resolvedReferenceNames(text string, trie *promptReferenceTrieNode) []string {
	var names []string
	for index := 0; index < len(text); index++ {
		if text[index] != '@' || !isPromptReferenceStart(text, index) {
			continue
		}
		if prompt, end, ok := matchPromptReference(text, index, trie); ok {
			names = append(names, prompt.Name)
			index = end - 1
		}
	}
	return names
}

// referenceToken returns the run of reference name characters at start.
func referenceToken(text string, start int) string {
	end := start
	for end < len(text) && isPromptReferenceNameCharAt(text, end) {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}
	return text[start:end]
}
