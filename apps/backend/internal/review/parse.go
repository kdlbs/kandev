package review

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/models"
)

// maxAnchorTextBytes bounds the stored anchor snippet. Long enough to relocate a
// moved block, short enough that a large review does not bloat the row.
const maxAnchorTextBytes = 2000

// maxTitleRunes matches the spec's title constraint.
const maxTitleRunes = 120

// maxCategoryRunes matches the spec's category-slug constraint.
const maxCategoryRunes = 40

// FindingInput is one finding as reported by a reviewer, before it is anchored
// against the change set. Field names match both the reviewer's JSON contract
// and the `publish_review_findings_kandev` MCP schema, so the two paths share
// one validation surface.
type FindingInput struct {
	Repo       string `json:"repo"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end"`
	Side       string `json:"side"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion"`
}

// reviewerResponse is the JSON envelope the code-review prompt asks for.
type reviewerResponse struct {
	Summary  string         `json:"summary"`
	Findings []FindingInput `json:"findings"`
}

// ParseResult is the outcome of reading one reviewer reply.
type ParseResult struct {
	Summary string
	// Findings passed structural validation. May be empty: a clean review is a
	// valid, successful result.
	Findings []FindingInput
	// Rejected counts entries that were individually malformed and dropped. The
	// run still completes and reports the count, because a reviewer that got one
	// entry wrong usually got the rest right.
	Rejected int
}

// ParseFindings extracts the findings envelope from a reviewer's raw reply.
//
// Real agents wrap JSON in prose and fences even when told not to, so this
// accepts a ```json fenced block, a bare fenced block, or a raw object embedded
// in surrounding text. It returns ErrUnparseableResponse only when no findings
// array can be recovered at all — that is a run failure, because silently
// treating unreadable output as "no issues found" would be a false all-clear.
func ParseFindings(response string) (ParseResult, error) {
	for _, candidate := range jsonCandidates(response) {
		var envelope reviewerResponse
		if err := json.Unmarshal([]byte(candidate), &envelope); err != nil {
			continue
		}
		if envelope.Findings == nil && !hasFindingsKey(candidate) {
			// Parsed as JSON but is not the review envelope (for example a bare
			// string or an unrelated object). Keep looking.
			continue
		}
		return validateFindings(envelope), nil
	}
	return ParseResult{}, fmt.Errorf("%w: no findings array in reviewer response", ErrUnparseableResponse)
}

func hasFindingsKey(candidate string) bool {
	return strings.Contains(candidate, `"findings"`)
}

// jsonCandidates yields possible JSON payloads inside a reply, most-likely
// first: fenced blocks in order, then the widest brace-balanced span.
func jsonCandidates(response string) []string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return nil
	}
	candidates := extractFencedBlocks(trimmed)
	if span := widestObjectSpan(trimmed); span != "" {
		candidates = append(candidates, span)
	}
	return candidates
}

// extractFencedBlocks returns the contents of every ``` fenced block, dropping
// an optional language tag on the opening fence.
func extractFencedBlocks(text string) []string {
	const fence = "```"
	var blocks []string
	rest := text
	for {
		start := strings.Index(rest, fence)
		if start < 0 {
			return blocks
		}
		rest = rest[start+len(fence):]
		// Drop the language tag, if any, up to the end of the line.
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			firstLine := strings.TrimSpace(rest[:nl])
			if !strings.Contains(firstLine, "{") && !strings.Contains(firstLine, "[") {
				rest = rest[nl+1:]
			}
		}
		end := strings.Index(rest, fence)
		if end < 0 {
			// Unterminated fence: take everything that is left.
			block := strings.TrimSpace(rest)
			if block != "" {
				blocks = append(blocks, block)
			}
			return blocks
		}
		block := strings.TrimSpace(rest[:end])
		if block != "" {
			blocks = append(blocks, block)
		}
		rest = rest[end+len(fence):]
	}
}

// widestObjectSpan returns the text from the first '{' to the last '}', which
// recovers a bare object surrounded by prose.
func widestObjectSpan(text string) string {
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// validateFindings drops individually-invalid entries and normalizes the rest.
func validateFindings(envelope reviewerResponse) ParseResult {
	result := ParseResult{Summary: strings.TrimSpace(envelope.Summary)}
	for i := range envelope.Findings {
		normalized, err := NormalizeFindingInput(envelope.Findings[i])
		if err != nil {
			result.Rejected++
			continue
		}
		result.Findings = append(result.Findings, normalized)
	}
	return result
}

// NormalizeFindingInput trims, defaults, and validates one reported finding.
// Shared by the inference path (which drops a bad entry and continues) and the
// MCP path (which rejects the whole call so the agent can retry).
func NormalizeFindingInput(in FindingInput) (FindingInput, error) {
	out := FindingInput{
		Repo:       strings.TrimSpace(in.Repo),
		File:       strings.TrimSpace(in.File),
		Line:       in.Line,
		LineEnd:    in.LineEnd,
		Side:       strings.TrimSpace(in.Side),
		Severity:   strings.ToLower(strings.TrimSpace(in.Severity)),
		Category:   normalizeCategory(in.Category),
		Title:      normalizeTitle(in.Title),
		Body:       strings.TrimSpace(in.Body),
		Suggestion: strings.TrimSpace(in.Suggestion),
	}
	if out.File == "" {
		return FindingInput{}, fmt.Errorf("file is required")
	}
	if out.Line <= 0 {
		return FindingInput{}, fmt.Errorf("line must be positive, got %d", out.Line)
	}
	if out.LineEnd == 0 {
		out.LineEnd = out.Line
	}
	if out.LineEnd < out.Line {
		return FindingInput{}, fmt.Errorf("line_end %d is before line %d", out.LineEnd, out.Line)
	}
	if !models.ValidReviewSeverity(models.ReviewSeverity(out.Severity)) {
		return FindingInput{}, fmt.Errorf("unknown severity %q", in.Severity)
	}
	if out.Title == "" {
		return FindingInput{}, fmt.Errorf("title is required")
	}
	if out.Body == "" {
		return FindingInput{}, fmt.Errorf("body is required")
	}
	if out.Side != models.ReviewSideDeletions {
		out.Side = models.ReviewSideAdditions
	}
	return out, nil
}

// normalizeTitle collapses newlines and caps the length; a multi-line title
// would break the single-line finding header.
func normalizeTitle(title string) string {
	flat := strings.Join(strings.Fields(strings.ReplaceAll(title, "\n", " ")), " ")
	return truncateRunes(flat, maxTitleRunes)
}

// normalizeCategory lowercases and kebab-cases the slug so grouping is stable
// regardless of how the reviewer phrased it.
func normalizeCategory(category string) string {
	lower := strings.ToLower(strings.TrimSpace(category))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return truncateRunes(strings.Trim(b.String(), "-"), maxCategoryRunes)
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

// TruncateAnchorText caps a stored anchor snippet, cutting on a rune boundary so
// the stored text stays valid UTF-8. Trimming back at most three bytes is enough
// to drop any partial sequence, including a dangling lead byte.
func TruncateAnchorText(s string) string {
	if len(s) <= maxAnchorTextBytes {
		return s
	}
	cut := s[:maxAnchorTextBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}
