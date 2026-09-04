package configsync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
)

// fakeGitHub is an in-memory GitHubClientProvider keyed by path, letting each
// test declare exactly what the walk should see without a network client.
type fakeGitHub struct {
	dirs    map[string][]github.RepoContentEntry
	dirErr  map[string]error
	files   map[string][]byte
	fileErr map[string]error
	// fetchCalls counts GetRepoFileContentForWorkspace invocations, letting
	// tests assert the walk stopped issuing fetches once a cap was reached
	// instead of only checking the cap after fetching everything.
	fetchCalls int
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{
		dirs:    map[string][]github.RepoContentEntry{},
		dirErr:  map[string]error{},
		files:   map[string][]byte{},
		fileErr: map[string]error{},
	}
}

func (f *fakeGitHub) ListRepoDirectoryForWorkspace(
	_ context.Context, _, _, _, path, _ string,
) ([]github.RepoContentEntry, error) {
	if err, ok := f.dirErr[path]; ok {
		return nil, err
	}
	entries, ok := f.dirs[path]
	if !ok {
		return nil, &github.GitHubAPIError{StatusCode: 404, Endpoint: path}
	}
	return entries, nil
}

func (f *fakeGitHub) GetRepoFileContentForWorkspace(
	_ context.Context, _, _, _, path, _ string,
) ([]byte, error) {
	f.fetchCalls++
	if err, ok := f.fileErr[path]; ok {
		return nil, err
	}
	content, ok := f.files[path]
	if !ok {
		return nil, &github.GitHubAPIError{StatusCode: 404, Endpoint: path}
	}
	return content, nil
}

func fileEntry(path string) github.RepoContentEntry {
	name := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		name = path[idx+1:]
	}
	return github.RepoContentEntry{Name: name, Path: path, Type: github.RepoContentTypeFile}
}

func dirEntry(path string) github.RepoContentEntry {
	name := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		name = path[idx+1:]
	}
	return github.RepoContentEntry{Name: name, Path: path, Type: github.RepoContentTypeDir}
}

func testGitHubConfig() *Config {
	return &Config{
		WorkspaceID: "ws-1",
		Provider:    ProviderGitHub,
		RepoOwner:   "acme",
		RepoName:    "kandev-config",
		Branch:      "main",
		Path:        "cfg",
	}
}

func TestWalk_RootListingFailureFailsRun(t *testing.T) {
	fg := newFakeGitHub()
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure on missing root path")
	}
}

func TestWalk_RootListingUnavailableFailsRun(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirErr["cfg"] = &github.GitHubAPIError{StatusCode: 503, Endpoint: "cfg"}
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure on unavailable root listing")
	}
}

func TestWalk_MissingKindDirectoriesAreEmptyNotFailures(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.agentFiles) != 0 || len(result.projectFiles) != 0 || len(result.routineFiles) != 0 || len(result.skills) != 0 {
		t.Fatalf("Walk() result = %+v, want all empty", result)
	}
	if result.kandevYAMLPresent {
		t.Error("kandevYAMLPresent = true, want false (root listing had no kandev.yml)")
	}
}

func TestWalk_KandevYAMLPresenceDetected(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{fileEntry("cfg/kandev.yml")}
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if !result.kandevYAMLPresent {
		t.Error("kandevYAMLPresent = false, want true")
	}
}

func TestWalk_SubListingUnavailableFailsRun(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirErr["cfg/agents"] = &github.GitHubAPIError{StatusCode: 500, Endpoint: "cfg/agents"}
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure on unavailable sub-listing")
	}
}

func TestWalk_FlatKindSelectsOnlyYAMLFiles(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/reviewer.yml"),
		fileEntry("cfg/agents/planner.yaml"),
		fileEntry("cfg/agents/README.md"),
		dirEntry("cfg/agents/subdir"),
	}
	fg.files["cfg/agents/reviewer.yml"] = []byte("name: reviewer\n")
	fg.files["cfg/agents/planner.yaml"] = []byte("name: planner\n")
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.agentFiles) != 2 {
		t.Fatalf("agentFiles = %+v, want 2 entries", result.agentFiles)
	}
	if result.agentFiles[0].path != "cfg/agents/planner.yaml" || result.agentFiles[1].path != "cfg/agents/reviewer.yml" {
		t.Errorf("agentFiles not sorted ascending by path: %+v", result.agentFiles)
	}
}

func TestWalk_UnavailableFileFetchFailsRun(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/reviewer.yml")}
	fg.fileErr["cfg/agents/reviewer.yml"] = &github.GitHubAPIError{StatusCode: 429, Endpoint: "cfg/agents/reviewer.yml"}
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure on unavailable file fetch")
	}
}

func TestWalk_NotFoundFileFetchWarnsAndExempts(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/reviewer.yml")}
	// Listed but gone by fetch time (racing upstream commit): 404 on fetch.
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.agentFiles) != 0 {
		t.Errorf("agentFiles = %+v, want none (fetch 404)", result.agentFiles)
	}
	if len(result.unreadable) != 1 || result.unreadable[0].path != "cfg/agents/reviewer.yml" {
		t.Errorf("unreadable = %+v, want one entry for cfg/agents/reviewer.yml", result.unreadable)
	}
}

func TestWalk_FileOverSizeLimitIsUnreadable(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/big.yml")}
	fg.files["cfg/agents/big.yml"] = make([]byte, MaxFileBytes+1)
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.agentFiles) != 0 {
		t.Errorf("agentFiles = %+v, want none (over size limit)", result.agentFiles)
	}
	if len(result.unreadable) != 1 || result.unreadable[0].path != "cfg/agents/big.yml" {
		t.Errorf("unreadable = %+v, want one entry for cfg/agents/big.yml", result.unreadable)
	}
}

func TestWalk_SkillDirectoryGroupsSkillMDAndReferences(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/reviewer")}
	fg.dirs["cfg/skills/reviewer"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/reviewer/SKILL.md"),
		dirEntry("cfg/skills/reviewer/references"),
	}
	fg.files["cfg/skills/reviewer/SKILL.md"] = []byte("# Reviewer\n")
	fg.dirs["cfg/skills/reviewer/references"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/reviewer/references/checklist.md"),
		fileEntry("cfg/skills/reviewer/references/style.md"),
	}
	fg.files["cfg/skills/reviewer/references/checklist.md"] = []byte("checklist")
	fg.files["cfg/skills/reviewer/references/style.md"] = []byte("style")

	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.skills) != 1 {
		t.Fatalf("skills = %+v, want 1 entry", result.skills)
	}
	sf := result.skills[0]
	if sf.dirName != "reviewer" || sf.dirPath != "cfg/skills/reviewer" {
		t.Errorf("skill dir = %q/%q, want reviewer/cfg/skills/reviewer", sf.dirName, sf.dirPath)
	}
	if sf.skillMD == nil || string(sf.skillMD.content) != "# Reviewer\n" {
		t.Errorf("skillMD = %+v, want SKILL.md content", sf.skillMD)
	}
	if len(sf.references) != 2 {
		t.Fatalf("references = %+v, want 2 entries", sf.references)
	}
	if sf.references[0].path != "cfg/skills/reviewer/references/checklist.md" {
		t.Errorf("references not sorted ascending by path: %+v", sf.references)
	}
}

func TestWalk_SkillWithoutSkillMDHasNilSkillMD(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/empty")}
	fg.dirs["cfg/skills/empty"] = []github.RepoContentEntry{}
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.skills) != 1 || result.skills[0].skillMD != nil {
		t.Errorf("skills = %+v, want one skill with nil skillMD", result.skills)
	}
}

// TestWalk_SkillDirectoryDisappearingBetweenListingsIsUnreadableNotEmpty
// covers the race between planSkills' round-1 listing of skills/ (which
// just returned "reviewer" as an existing directory entry) and planOneSkill's
// round-2 listing of that same directory: a 404 there is the directory
// vanishing mid-run, not proof it never had a SKILL.md, so it must be
// recorded as unreadable (which exempts it from the deletion sweep) rather
// than as a genuinely empty skill (which does not).
func TestWalk_SkillDirectoryDisappearingBetweenListingsIsUnreadableNotEmpty(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/reviewer")}
	// cfg/skills/reviewer is deliberately never registered in fg.dirs, so
	// listing it 404s even though the parent listing just proved it exists.

	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.skills) != 1 {
		t.Fatalf("skills = %+v, want 1 entry", result.skills)
	}
	sf := result.skills[0]
	if sf.skillMD != nil {
		t.Errorf("skillMD = %+v, want nil (unreadable, not fetched)", sf.skillMD)
	}
	if sf.skillMDUnread == nil {
		t.Fatal("skillMDUnread = nil, want set: a disappeared directory must be treated as unreadable, not genuinely empty")
	}
}

func TestWalk_UnreadableFlatFilesAreSortedByPath(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-004.5a: two runs over the same repository must
	// record warnings in the same order, which requires result.unreadable
	// itself to be in path order regardless of provider listing order.
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/b.yml"),
		fileEntry("cfg/agents/a.yml"),
	}
	// Neither has content registered: both 404 on fetch (listed but gone).
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.unreadable) != 2 {
		t.Fatalf("unreadable = %+v, want 2 entries", result.unreadable)
	}
	if result.unreadable[0].path != "cfg/agents/a.yml" || result.unreadable[1].path != "cfg/agents/b.yml" {
		t.Errorf("unreadable not sorted ascending by path: %+v", result.unreadable)
	}
}

func TestWalk_SkillUnreadableReferencesAreSortedByPath(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/skills"] = []github.RepoContentEntry{dirEntry("cfg/skills/reviewer")}
	fg.dirs["cfg/skills/reviewer"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/reviewer/SKILL.md"),
		dirEntry("cfg/skills/reviewer/references"),
	}
	fg.files["cfg/skills/reviewer/SKILL.md"] = []byte("# Reviewer\n")
	fg.dirs["cfg/skills/reviewer/references"] = []github.RepoContentEntry{
		fileEntry("cfg/skills/reviewer/references/z.md"),
		fileEntry("cfg/skills/reviewer/references/a.md"),
	}
	// Neither reference file has content registered: both 404 on fetch.
	w := newWalker(fg, nil, DefaultLimits)
	result, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
	if len(result.skills) != 1 {
		t.Fatalf("skills = %+v, want 1 entry", result.skills)
	}
	unreadableRefs := result.skills[0].unreadableRefs
	if len(unreadableRefs) != 2 {
		t.Fatalf("unreadableRefs = %+v, want 2 entries", unreadableRefs)
	}
	if unreadableRefs[0].path != "cfg/skills/reviewer/references/a.md" || unreadableRefs[1].path != "cfg/skills/reviewer/references/z.md" {
		t.Errorf("unreadableRefs not sorted ascending by path: %+v", unreadableRefs)
	}
}

func TestWalk_SkillCountOverCapFailsRunBeforeRound2(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	var entries []github.RepoContentEntry
	for i := 0; i < DefaultLimits.MaxSkills+1; i++ {
		entries = append(entries, dirEntry("cfg/skills/s"+strings.Repeat("x", i+1)))
	}
	fg.dirs["cfg/skills"] = entries
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil || !ferr.capped {
		t.Fatalf("Walk() error = %v, want a capped failure", ferr)
	}
	if !strings.Contains(ferr.reason, "skill directory cap") {
		t.Errorf("Walk() error = %q, want it to name the skill cap", ferr.reason)
	}
}

func TestWalk_FileCountOverCapFailsRun(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	limits := Limits{MaxSkills: 200, MaxFiles: 1}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/a.yml"),
		fileEntry("cfg/agents/b.yml"),
	}
	fg.files["cfg/agents/a.yml"] = []byte("name: a\n")
	fg.files["cfg/agents/b.yml"] = []byte("name: b\n")
	w := newWalker(fg, nil, limits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil || !ferr.capped {
		t.Fatalf("Walk() error = %v, want a capped failure", ferr)
	}
	if !strings.Contains(ferr.reason, "file cap") {
		t.Errorf("Walk() error = %q, want it to name the file cap", ferr.reason)
	}
}

// TestWalk_FileCapStopsFetchingBeforeExceedingLimit proves the file cap is
// enforced while the walk is selecting files, not only in an aggregate check
// after everything has already been fetched. AC-OFFICE-CONFIG-SYNC-002.5
// bounds a run to fetching at most MaxFiles files specifically so a
// pathological repository can't make the run pay for fetching far more than
// the cap before failing; a check that only runs once at the end defeats
// that purpose.
func TestWalk_FileCapStopsFetchingBeforeExceedingLimit(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	limits := Limits{MaxSkills: 200, MaxFiles: 2}
	var entries []github.RepoContentEntry
	for i := 0; i < 5; i++ {
		p := "cfg/agents/a" + strings.Repeat("x", i+1) + ".yml"
		entries = append(entries, fileEntry(p))
		fg.files[p] = []byte("name: a\n")
	}
	fg.dirs["cfg/agents"] = entries
	w := newWalker(fg, nil, limits)

	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil || !ferr.capped {
		t.Fatalf("Walk() error = %v, want a capped failure", ferr)
	}
	if fg.fetchCalls != 0 {
		t.Errorf("fetchCalls = %d, want 0: the cap is known from listing alone, so a capped run must issue no fetches at all", fg.fetchCalls)
	}
}

// TestWalk_FileCapWarningNamesDroppedCount proves the file cap failure names
// how many files were dropped, not just that the cap was exceeded.
// AC-OFFICE-CONFIG-SYNC-002.5 requires the warning to say "how many ...
// files ... were dropped", which requires knowing the total candidate count
// before any fetch is issued.
func TestWalk_FileCapWarningNamesDroppedCount(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	limits := Limits{MaxSkills: 200, MaxFiles: 2}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{
		fileEntry("cfg/agents/a.yml"),
		fileEntry("cfg/agents/b.yml"),
		fileEntry("cfg/agents/c.yml"),
	}
	for _, p := range []string{"cfg/agents/a.yml", "cfg/agents/b.yml", "cfg/agents/c.yml"} {
		fg.files[p] = []byte("name: a\n")
	}
	w := newWalker(fg, nil, limits)

	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil || !ferr.capped {
		t.Fatalf("Walk() error = %v, want a capped failure", ferr)
	}
	if !strings.Contains(ferr.reason, "1 dropped") {
		t.Errorf("Walk() error = %q, want it to name the dropped file count (3 found, limit 2, so 1 dropped)", ferr.reason)
	}
}

func TestWalk_UnavailableProviderDisconnectedFailsRun(t *testing.T) {
	w := newWalker(nil, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), testGitHubConfig())
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure when no github provider is wired")
	}
}

func TestWalk_RootPathEmptyMeansRepositoryRoot(t *testing.T) {
	fg := newFakeGitHub()
	fg.dirs[""] = []github.RepoContentEntry{}
	cfg := testGitHubConfig()
	cfg.Path = ""
	w := newWalker(fg, nil, DefaultLimits)
	if _, ferr := w.Walk(context.Background(), cfg); ferr != nil {
		t.Fatalf("Walk() error = %v, want nil", ferr)
	}
}

func TestWalk_UnknownProviderFails(t *testing.T) {
	fg := newFakeGitHub()
	cfg := testGitHubConfig()
	cfg.Provider = "bitbucket"
	w := newWalker(fg, nil, DefaultLimits)
	_, ferr := w.Walk(context.Background(), cfg)
	if ferr == nil {
		t.Fatal("Walk() error = nil, want failure for unknown provider")
	}
}

func TestClassifyFetchErr_UsedForDispatchErrors(t *testing.T) {
	// Sanity check that walk.go's own error paths compose with the shared
	// classifier rather than duplicating status-code logic.
	err := classifyFetchErr(&github.GitHubAPIError{StatusCode: 404})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("classifyFetchErr(404) = %v, want ErrNotFound", err)
	}
}
