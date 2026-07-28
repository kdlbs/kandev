package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const (
	// WorkspaceContentSearchMaxQueryRunes is the largest accepted content query.
	WorkspaceContentSearchMaxQueryRunes = types.WorkspaceContentSearchMaxQueryRunes
	workspaceContentSearchMaxLimit      = 50
	workspaceContentSearchDefaultLimit  = 20
	workspaceContentSearchPreviewRunes  = 300
	workspaceContentSearchReadBuffer    = 32 * 1024
	workspaceContentContiguousScoreBase = 1_000_000_000
	workspaceContentSearchCancelRunes   = 1_024
	workspaceContentSearchFuzzyMaxSteps = 250_000
)

// ErrContentSearchQueryTooLong indicates that the query exceeds the public limit.
var ErrContentSearchQueryTooLong = errors.New("content search query exceeds 200 characters")

var errContentSearchFuzzyWorkLimit = errors.New("content search fuzzy work limit reached")

type contentLineMatch struct {
	score     int
	positions []int
}

type contentSearchFuzzyWork struct {
	steps int
}

type scoredContentSearchResult struct {
	result types.WorkspaceContentSearchResult
	score  int
}

// SearchWorkspaceContent searches every repository represented by the manager.
func (m *Manager) SearchWorkspaceContent(
	ctx context.Context,
	query string,
	limit int,
) (*types.WorkspaceContentSearchResponse, error) {
	limit, err := validateContentSearch(query, limit)
	if err != nil {
		return nil, err
	}
	response := &types.WorkspaceContentSearchResponse{
		Results: []types.WorkspaceContentSearchResult{},
	}
	if query == "" {
		return response, nil
	}

	for _, tracker := range m.contentSearchTrackers() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results, err := tracker.SearchContent(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		response.Results = append(response.Results, results...)
	}
	return response, nil
}

func (m *Manager) contentSearchTrackers() []*WorkspaceTracker {
	root, repositories := m.snapshotTrackers()
	trackers := make([]*WorkspaceTracker, 0, len(repositories)+1)
	if len(repositories) == 0 {
		if root != nil {
			trackers = append(trackers, root)
		}
		return trackers
	}
	if root != nil && root.RepositoryName() != "" {
		trackers = append(trackers, root)
	}
	trackers = append(trackers, repositories...)
	sort.Slice(trackers, func(i, j int) bool {
		if trackers[i].RepositoryName() != trackers[j].RepositoryName() {
			return trackers[i].RepositoryName() < trackers[j].RepositoryName()
		}
		return trackers[i].workDir < trackers[j].workDir
	})
	return trackers
}

// SearchContent searches the tracked and untracked nonignored text files for
// matching lines. Results are bounded and sorted by match quality.
func (wt *WorkspaceTracker) SearchContent(
	ctx context.Context,
	query string,
	limit int,
) ([]types.WorkspaceContentSearchResult, error) {
	limit, err := validateContentSearch(query, limit)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return []types.WorkspaceContentSearchResult{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	files, err := wt.getFileList(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(files.Files, func(i, j int) bool {
		return files.Files[i].Path < files.Files[j].Path
	})
	queryRunes := foldRunes([]rune(query))
	ranked := make([]scoredContentSearchResult, 0, limit)
	for _, file := range files.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, searchable, readErr := wt.readContentSearchFile(ctx, file.Path)
		if readErr != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			continue
		}
		if !searchable {
			continue
		}
		ranked, err = wt.searchContentFile(ctx, ranked, file.Path, data, queryRunes, limit)
		if err != nil {
			return nil, err
		}
	}
	return contentSearchResults(ranked), nil
}

func validateContentSearch(query string, limit int) (int, error) {
	if utf8.RuneCountInString(query) > WorkspaceContentSearchMaxQueryRunes {
		return 0, ErrContentSearchQueryTooLong
	}
	if limit <= 0 {
		return workspaceContentSearchDefaultLimit, nil
	}
	if limit > workspaceContentSearchMaxLimit {
		return workspaceContentSearchMaxLimit, nil
	}
	return limit, nil
}

func (wt *WorkspaceTracker) readContentSearchFile(
	ctx context.Context,
	path string,
) ([]byte, bool, error) {
	safePath, err := wt.resolveSafePath(path)
	if err != nil {
		return nil, false, err
	}
	file, err := os.Open(safePath)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return nil, false, nil
	}

	data, err := readContentSearchBytes(ctx, file, info.Size())
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxFileSize {
		return nil, false, nil
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, false, nil
	}
	return data, true, nil
}

func readContentSearchBytes(ctx context.Context, reader io.Reader, size int64) ([]byte, error) {
	buffer := bytes.NewBuffer(make([]byte, 0, int(size)))
	chunk := make([]byte, workspaceContentSearchReadBuffer)
	limited := io.LimitReader(reader, maxFileSize+1)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := limited.Read(chunk)
		if count > 0 {
			_, _ = buffer.Write(chunk[:count])
		}
		if errors.Is(err, io.EOF) {
			return buffer.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (wt *WorkspaceTracker) searchContentFile(
	ctx context.Context,
	ranked []scoredContentSearchResult,
	path string,
	data []byte,
	query []rune,
	limit int,
) ([]scoredContentSearchResult, error) {
	lines := bytes.Split(data, []byte{'\n'})
	for index, lineBytes := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lineBytes = bytes.TrimSuffix(lineBytes, []byte{'\r'})
		lineRunes := []rune(string(lineBytes))
		match, ok, err := bestContentLineMatch(ctx, lineRunes, query)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		result := buildContentSearchResult(wt.repositoryName, path, index+1, lineRunes, match)
		ranked = insertContentSearchResult(ranked, scoredContentSearchResult{
			result: result,
			score:  match.score,
		}, limit)
	}
	return ranked, nil
}

func bestContentLineMatch(
	ctx context.Context,
	line, query []rune,
) (contentLineMatch, bool, error) {
	if len(query) == 0 || len(line) < len(query) {
		return contentLineMatch{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return contentLineMatch{}, false, err
	}
	foldedLine := foldRunes(line)
	match, ok, err := bestContiguousContentMatch(ctx, line, foldedLine, query)
	if err != nil {
		return contentLineMatch{}, false, err
	}
	if ok {
		return match, true, nil
	}
	return bestSubsequenceContentMatch(ctx, line, foldedLine, query)
}

func bestContiguousContentMatch(
	ctx context.Context,
	line, foldedLine, query []rune,
) (contentLineMatch, bool, error) {
	bestStart, bestScore := 0, 0
	found := false
	queryIndex := 0
	prefixes := contentSearchPrefixTable(query)
	for lineIndex, item := range foldedLine {
		if lineIndex%workspaceContentSearchCancelRunes == 0 {
			if err := ctx.Err(); err != nil {
				return contentLineMatch{}, false, err
			}
		}
		for queryIndex > 0 && item != query[queryIndex] {
			queryIndex = prefixes[queryIndex-1]
		}
		if item == query[queryIndex] {
			queryIndex++
		}
		if queryIndex != len(query) {
			continue
		}
		start := lineIndex - len(query) + 1
		score := workspaceContentContiguousScoreBase - start
		if isContentWordBoundary(line, start) {
			score += 100
		}
		if !found || score > bestScore {
			bestStart, bestScore, found = start, score, true
		}
		queryIndex = prefixes[queryIndex-1]
	}
	if !found {
		return contentLineMatch{}, false, nil
	}
	return contentLineMatch{
		score:     bestScore,
		positions: contiguousPositions(bestStart, len(query)),
	}, true, nil
}

func bestSubsequenceContentMatch(
	ctx context.Context,
	line, foldedLine, query []rune,
) (contentLineMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return contentLineMatch{}, false, err
	}
	best := contentLineMatch{}
	found := false
	work := contentSearchFuzzyWork{}
	for cursor := 0; cursor < len(foldedLine); {
		positions, ok, err := nextSubsequenceContentPositions(
			ctx,
			foldedLine,
			query,
			cursor,
			&work,
		)
		if errors.Is(err, errContentSearchFuzzyWorkLimit) {
			break
		}
		if err != nil {
			return contentLineMatch{}, false, err
		}
		if !ok {
			break
		}
		score := scoreSubsequenceContentMatch(line, positions)
		if !found || score > best.score {
			best = contentLineMatch{score: score, positions: positions}
			found = true
		}
		cursor = positions[0] + 1
	}
	return best, found, nil
}

func nextSubsequenceContentPositions(
	ctx context.Context,
	line, query []rune,
	start int,
	work *contentSearchFuzzyWork,
) ([]int, bool, error) {
	positions := make([]int, 0, len(query))
	queryIndex := 0
	end := -1
	for position := start; position < len(line); position++ {
		if err := work.consume(ctx); err != nil {
			return nil, false, err
		}
		if line[position] != query[queryIndex] {
			continue
		}
		queryIndex++
		if queryIndex == len(query) {
			end = position
			break
		}
	}
	if end < 0 {
		return nil, false, nil
	}
	positions = positions[:len(query)]
	queryIndex = len(query) - 1
	for position := end; position >= start; position-- {
		if err := work.consume(ctx); err != nil {
			return nil, false, err
		}
		if line[position] != query[queryIndex] {
			continue
		}
		positions[queryIndex] = position
		queryIndex--
		if queryIndex < 0 {
			break
		}
	}
	return positions, true, nil
}

func (work *contentSearchFuzzyWork) consume(ctx context.Context) error {
	if work.steps >= workspaceContentSearchFuzzyMaxSteps {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errContentSearchFuzzyWorkLimit
	}
	if work.steps%workspaceContentSearchCancelRunes == 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	work.steps++
	return nil
}

func scoreSubsequenceContentMatch(line []rune, positions []int) int {
	score := len(positions)*20 - positions[0]
	for index, position := range positions {
		if isContentWordBoundary(line, position) {
			score += 8
		}
		if index == 0 {
			continue
		}
		gap := position - positions[index-1] - 1
		if gap == 0 {
			score += 15
		} else {
			score -= gap * 2
		}
	}
	return score
}

func isContentWordBoundary(line []rune, position int) bool {
	if position == 0 {
		return true
	}
	previous, current := line[position-1], line[position]
	if !unicode.IsLetter(previous) && !unicode.IsNumber(previous) && previous != '_' {
		return true
	}
	return unicode.IsLower(previous) && unicode.IsUpper(current)
}

func buildContentSearchResult(
	repositoryName, path string,
	lineNumber int,
	line []rune,
	match contentLineMatch,
) types.WorkspaceContentSearchResult {
	preview, ranges := buildContentSearchPreview(line, match.positions)
	return types.WorkspaceContentSearchResult{
		RepositoryName: repositoryName,
		Path:           path,
		Line:           lineNumber,
		Column:         utf16Length(line[:match.positions[0]]) + 1,
		Preview:        preview,
		MatchRanges:    ranges,
	}
}

func buildContentSearchPreview(
	line []rune,
	positions []int,
) (string, []types.WorkspaceContentMatchRange) {
	start, end := contentSearchPreviewBounds(len(line), positions[0])
	previewRunes := append([]rune(nil), line[start:end]...)
	prefixUnits := 0
	if start > 0 {
		previewRunes = append([]rune{'…'}, previewRunes...)
		prefixUnits = 1
	}
	if end < len(line) {
		previewRunes = append(previewRunes, '…')
	}

	ranges := contentSearchMatchRanges(line, positions, start, end, prefixUnits)
	return string(previewRunes), ranges
}

func contentSearchPreviewBounds(lineLength, firstMatch int) (int, int) {
	if lineLength <= workspaceContentSearchPreviewRunes {
		return 0, lineLength
	}
	start := firstMatch - workspaceContentSearchPreviewRunes/4
	if start < 0 {
		start = 0
	}
	end := start + workspaceContentSearchPreviewRunes
	if end > lineLength {
		end = lineLength
		start = end - workspaceContentSearchPreviewRunes
	}
	return start, end
}

func contentSearchMatchRanges(
	line []rune,
	positions []int,
	previewStart, previewEnd, prefixUnits int,
) []types.WorkspaceContentMatchRange {
	ranges := make([]types.WorkspaceContentMatchRange, 0, len(positions))
	for _, position := range positions {
		if position < previewStart || position >= previewEnd {
			continue
		}
		start := prefixUnits + utf16Length(line[previewStart:position])
		end := start + utf16RuneLength(line[position])
		if len(ranges) > 0 && ranges[len(ranges)-1].End == start {
			ranges[len(ranges)-1].End = end
			continue
		}
		ranges = append(ranges, types.WorkspaceContentMatchRange{Start: start, End: end})
	}
	return ranges
}

func insertContentSearchResult(
	ranked []scoredContentSearchResult,
	candidate scoredContentSearchResult,
	limit int,
) []scoredContentSearchResult {
	index := sort.Search(len(ranked), func(index int) bool {
		return betterContentSearchResult(candidate, ranked[index])
	})
	if len(ranked) < limit {
		ranked = append(ranked, scoredContentSearchResult{})
		copy(ranked[index+1:], ranked[index:])
		ranked[index] = candidate
		return ranked
	}
	if index == len(ranked) {
		return ranked
	}
	copy(ranked[index+1:], ranked[index:len(ranked)-1])
	ranked[index] = candidate
	return ranked
}

func betterContentSearchResult(left, right scoredContentSearchResult) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.result.Path != right.result.Path {
		return left.result.Path < right.result.Path
	}
	if left.result.Line != right.result.Line {
		return left.result.Line < right.result.Line
	}
	return left.result.Column < right.result.Column
}

func contentSearchResults(
	ranked []scoredContentSearchResult,
) []types.WorkspaceContentSearchResult {
	results := make([]types.WorkspaceContentSearchResult, len(ranked))
	for index := range ranked {
		results[index] = ranked[index].result
	}
	return results
}

func foldRunes(value []rune) []rune {
	folded := make([]rune, len(value))
	for index, item := range value {
		folded[index] = unicode.ToLower(item)
	}
	return folded
}

func contentSearchPrefixTable(query []rune) []int {
	prefixes := make([]int, len(query))
	prefixLength := 0
	for index := 1; index < len(query); {
		if query[index] == query[prefixLength] {
			prefixLength++
			prefixes[index] = prefixLength
			index++
			continue
		}
		if prefixLength > 0 {
			prefixLength = prefixes[prefixLength-1]
			continue
		}
		index++
	}
	return prefixes
}

func contiguousPositions(start, length int) []int {
	positions := make([]int, length)
	for index := range positions {
		positions[index] = start + index
	}
	return positions
}

func utf16Length(value []rune) int {
	length := 0
	for _, item := range value {
		length += utf16RuneLength(item)
	}
	return length
}

func utf16RuneLength(value rune) int {
	length := utf16.RuneLen(value)
	if length < 1 {
		return 1
	}
	return length
}
