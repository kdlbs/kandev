package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestWorkspaceTrackerSearchContentRanksContiguousMatchAndUsesUTF16Ranges(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "search.txt", "n_e_e_d_l_e\n🙂 Needle value\n")

	tracker := NewWorkspaceTrackerForRepo(repoDir, "backend", newTestLogger(t))
	results, err := tracker.SearchContent(context.Background(), "NeEdLe", 10)
	if err != nil {
		t.Fatalf("SearchContent returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("SearchContent result count = %d, want 2: %#v", len(results), results)
	}

	exact := results[0]
	if exact.RepositoryName != "backend" || exact.Path != "search.txt" {
		t.Fatalf("exact result identity = %#v", exact)
	}
	if exact.Line != 2 || exact.Column != 4 || exact.Preview != "🙂 Needle value" {
		t.Fatalf("exact location = line %d, column %d, preview %q", exact.Line, exact.Column, exact.Preview)
	}
	if len(exact.MatchRanges) != 1 ||
		exact.MatchRanges[0].Start != 3 || exact.MatchRanges[0].End != 9 {
		t.Fatalf("exact UTF-16 ranges = %#v, want [{3 9}]", exact.MatchRanges)
	}

	fuzzy := results[1]
	if fuzzy.Line != 1 || len(fuzzy.MatchRanges) != 6 {
		t.Fatalf("fuzzy result = %#v, want six subsequence ranges on line 1", fuzzy)
	}
}

func TestContentSearchAlwaysRanksContiguousAboveSubsequence(t *testing.T) {
	query := foldRunes([]rune("needle"))
	exactLine := []rune(strings.Repeat("x", 1_100_000) + "needle")
	fuzzyLine := []rune("n_e_e_d_l_e")

	exact, ok, err := bestContentLineMatch(context.Background(), exactLine, query)
	if err != nil {
		t.Fatalf("bestContentLineMatch returned error: %v", err)
	}
	if !ok {
		t.Fatal("long-line contiguous match was not found")
	}
	fuzzy, ok, err := bestContentLineMatch(context.Background(), fuzzyLine, query)
	if err != nil {
		t.Fatalf("bestContentLineMatch returned error: %v", err)
	}
	if !ok {
		t.Fatal("subsequence match was not found")
	}
	if exact.score <= fuzzy.score {
		t.Fatalf("contiguous score %d must exceed subsequence score %d", exact.score, fuzzy.score)
	}
}

func TestBestSubsequenceContentMatchChoosesTightestOverlappingCandidate(t *testing.T) {
	line := []rune("a___aa")
	match, ok, err := bestSubsequenceContentMatch(
		context.Background(),
		line,
		foldRunes(line),
		foldRunes([]rune("aa")),
	)
	if err != nil {
		t.Fatalf("bestSubsequenceContentMatch returned error: %v", err)
	}
	if !ok {
		t.Fatal("subsequence match was not found")
	}
	if got := match.positions; len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("positions = %v, want tightest overlapping match [4 5]", got)
	}
}

func TestBestSubsequenceContentMatchChecksCancellationDuringMatching(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelOnErrCheckContext{
		Context:         base,
		cancel:          cancel,
		checksRemaining: 2,
	}
	line := []rune(strings.Repeat("a_", 2_000))

	_, _, err := bestSubsequenceContentMatch(
		ctx,
		line,
		foldRunes(line),
		foldRunes([]rune("aa")),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bestSubsequenceContentMatch error = %v, want context.Canceled", err)
	}
}

func TestBestSubsequenceContentMatchBoundsPathologicalLongLineWork(t *testing.T) {
	const expectedFuzzyWorkLimit = 250_000
	line := []rune(strings.Repeat("x", expectedFuzzyWorkLimit+1) + "a_b")

	_, ok, err := bestSubsequenceContentMatch(
		context.Background(),
		line,
		foldRunes(line),
		foldRunes([]rune("ab")),
	)
	if err != nil {
		t.Fatalf("bestSubsequenceContentMatch returned error: %v", err)
	}
	if ok {
		t.Fatal("fuzzy match beyond the bounded work limit should be skipped")
	}
}

type cancelOnErrCheckContext struct {
	context.Context
	cancel          context.CancelFunc
	checksRemaining int
}

func (ctx *cancelOnErrCheckContext) Err() error {
	ctx.checksRemaining--
	if ctx.checksRemaining <= 0 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestWorkspaceTrackerSearchContentUsesGitFileSetAndSkipsUnsafeOrBinaryFiles(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, ".gitignore", "ignored.txt\n")
	writeFile(t, repoDir, "tracked.txt", "tracked secret\n")
	runGit(t, repoDir, "add", ".gitignore", "tracked.txt")
	runGit(t, repoDir, "commit", "-m", "add searchable files")
	writeFile(t, repoDir, "untracked.txt", "untracked secret\n")
	writeFile(t, repoDir, "ignored.txt", "ignored secret\n")
	if err := os.WriteFile(filepath.Join(repoDir, "binary.bin"), []byte("binary\x00secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "invalid.txt"), []byte("invalid secret\xff"), 0o644); err != nil {
		t.Fatal(err)
	}
	largePath := filepath.Join(repoDir, "large.txt")
	if err := os.WriteFile(largePath, []byte("large secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(largePath, maxFileSize+1); err != nil {
		t.Fatal(err)
	}

	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "outside.txt"), []byte("outside secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outsideDir, "outside.txt"), filepath.Join(repoDir, "unsafe-link.txt")); err == nil {
		runGit(t, repoDir, "add", "unsafe-link.txt")
		runGit(t, repoDir, "commit", "-m", "add unsafe link")
	}

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	results, err := tracker.SearchContent(context.Background(), "secret", 50)
	if err != nil {
		t.Fatalf("SearchContent returned error: %v", err)
	}
	gotPaths := make([]string, 0, len(results))
	for _, result := range results {
		gotPaths = append(gotPaths, result.Path)
	}
	want := []string{"tracked.txt", "untracked.txt"}
	if strings.Join(gotPaths, ",") != strings.Join(want, ",") {
		t.Fatalf("searchable paths = %v, want %v", gotPaths, want)
	}
}

func TestWorkspaceTrackerSearchContentBoundsPreviewAroundUTF16Match(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	line := strings.Repeat("x", 400) + "🙂 Needle" + strings.Repeat("y", 400)
	writeFile(t, repoDir, "long.txt", line+"\n")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	results, err := tracker.SearchContent(context.Background(), "needle", 10)
	if err != nil {
		t.Fatalf("SearchContent returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one", results)
	}
	result := results[0]
	if utf8.RuneCountInString(result.Preview) > workspaceContentSearchPreviewRunes+2 {
		t.Fatalf("preview has %d runes, want at most %d",
			utf8.RuneCountInString(result.Preview), workspaceContentSearchPreviewRunes+2)
	}
	if result.Column != 404 {
		t.Fatalf("column = %d, want UTF-16 column 404", result.Column)
	}
	if len(result.MatchRanges) != 1 ||
		result.MatchRanges[0].Start != 77 || result.MatchRanges[0].End != 83 {
		t.Fatalf("preview match ranges = %#v, want [{77 83}]", result.MatchRanges)
	}
}

func TestManagerSearchWorkspaceContentGroupsResultsByRepository(t *testing.T) {
	taskRoot := t.TempDir()
	for _, name := range []string{"zeta", "alpha"} {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		writeFile(t, repoDir, "match.txt", "first token\nsecond token\n")
		if err := os.Rename(repoDir, filepath.Join(taskRoot, name)); err != nil {
			t.Fatalf("place %s: %v", name, err)
		}
	}

	manager := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	response, err := manager.SearchWorkspaceContent(context.Background(), "token", 1)
	if err != nil {
		t.Fatalf("SearchWorkspaceContent returned error: %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("result count = %d, want one per repo: %#v", len(response.Results), response.Results)
	}
	if response.Results[0].RepositoryName != "alpha" ||
		response.Results[1].RepositoryName != "zeta" {
		t.Fatalf("repository order = %q, %q; want alpha, zeta",
			response.Results[0].RepositoryName, response.Results[1].RepositoryName)
	}
}

func TestManagerSearchWorkspaceContentClampsLimitPerRepository(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "many.txt", strings.Repeat("needle\n", 60))

	manager := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	response, err := manager.SearchWorkspaceContent(context.Background(), "needle", 500)
	if err != nil {
		t.Fatalf("SearchWorkspaceContent returned error: %v", err)
	}
	if len(response.Results) != workspaceContentSearchMaxLimit {
		t.Fatalf("result count = %d, want clamp %d", len(response.Results), workspaceContentSearchMaxLimit)
	}
}

func TestManagerSearchWorkspaceContentRejectsLongQuery(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	_, err := manager.SearchWorkspaceContent(
		context.Background(),
		strings.Repeat("界", WorkspaceContentSearchMaxQueryRunes+1),
		10,
	)
	if !errors.Is(err, ErrContentSearchQueryTooLong) {
		t.Fatalf("SearchWorkspaceContent error = %v, want ErrContentSearchQueryTooLong", err)
	}
}

func TestWorkspaceTrackerSearchContentHonorsCancellation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "match.txt", "needle\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	_, err := tracker.SearchContent(ctx, "needle", 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchContent error = %v, want context.Canceled", err)
	}
}

func TestReadContentSearchFilePropagatesCancellation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "match.txt", "needle\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	_, searchable, err := tracker.readContentSearchFile(ctx, "match.txt")
	if searchable {
		t.Fatal("canceled read unexpectedly reported searchable content")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readContentSearchFile error = %v, want context.Canceled", err)
	}
}
