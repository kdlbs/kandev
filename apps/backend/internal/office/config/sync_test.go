package config

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

const fullAgentYAML = `name: ada
role: cto
icon: "🤖"
reports_to: grace
permissions: '{"hire_agents":true}'
budget_monthly_cents: 4200
max_concurrent_sessions: 3
desired_skills: '["go","sql"]'
executor_preference: local_docker
`

const fullRoutineYAML = `name: standup
description: daily standup
task_template: summarise yesterday
assignee_name: grace
status: paused
concurrency_policy: skip
`

const fullProjectYAML = `name: apollo
description: moon shot
status: paused
color: "#ff00ff"
budget_cents: 99000
repositories: '["kdlbs/kandev"]'
executor_config: '{"type":"local_docker"}'
lead_agent_name: grace
`

// seedFullFSState writes one fully-populated file per entity type into the
// on-disk "default" workspace.
func seedFullFSState(t *testing.T, env *testEnv) {
	t.Helper()
	env.writeFSAgent(t, "ada", fullAgentYAML)
	env.writeFSSkill(t, "code-review", "# Code Review\n\nread the diff\n")
	env.writeFSRoutine(t, "standup", fullRoutineYAML)
	env.writeFSProject(t, "apollo", fullProjectYAML)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// TestScanFilesystem_CarriesEveryFieldFromDisk is the FS→bundle half of the
// field-drop guard: every value written to disk must arrive in the bundle.
func TestScanFilesystem_CarriesEveryFieldFromDisk(t *testing.T) {
	env := newTestEnv(t)
	seedFullFSState(t, env)

	bundle, parseErrs, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Fatalf("unexpected parse errors: %+v", parseErrs)
	}

	// The scan always reads the "default" FS workspace, regardless of the
	// DB workspace ID passed in.
	assertEqual(t, "settings name", bundle.Settings.Name, defaultWorkspaceName)

	if len(bundle.Agents) != 1 {
		t.Fatalf("agents: got %d, want 1", len(bundle.Agents))
	}
	agent := bundle.Agents[0]
	assertEqual(t, "agent name", agent.Name, "ada")
	assertEqual(t, "agent role", agent.Role, "cto")
	assertEqual(t, "agent icon", agent.Icon, "🤖")
	assertEqual(t, "agent reports_to", agent.ReportsTo, "grace")
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", agent.MaxConcurrentSessions, 3)
	assertEqual(t, "agent desired skills", agent.DesiredSkills, `["go","sql"]`)
	assertEqual(t, "agent executor preference", agent.ExecutorPreference, "local_docker")

	if len(bundle.Skills) != 1 {
		t.Fatalf("skills: got %d, want 1", len(bundle.Skills))
	}
	// The loader derives name and slug from the directory and stamps
	// source_type "filesystem"; only the body comes from SKILL.md.
	assertEqual(t, "skill name", bundle.Skills[0].Name, "code-review")
	assertEqual(t, "skill slug", bundle.Skills[0].Slug, "code-review")
	assertEqual(t, "skill source type", bundle.Skills[0].SourceType, "filesystem")
	assertEqual(t, "skill content", bundle.Skills[0].Content, "# Code Review\n\nread the diff\n")

	if len(bundle.Routines) != 1 {
		t.Fatalf("routines: got %d, want 1", len(bundle.Routines))
	}
	assertEqual(t, "routine name", bundle.Routines[0].Name, "standup")
	assertEqual(t, "routine description", bundle.Routines[0].Description, "daily standup")
	assertEqual(t, "routine task template", bundle.Routines[0].TaskTemplate, "summarise yesterday")
	assertEqual(t, "routine concurrency", bundle.Routines[0].ConcurrencyPolicy, "skip")

	if len(bundle.Projects) != 1 {
		t.Fatalf("projects: got %d, want 1", len(bundle.Projects))
	}
	assertEqual(t, "project name", bundle.Projects[0].Name, "apollo")
	assertEqual(t, "project description", bundle.Projects[0].Description, "moon shot")
	assertEqual(t, "project status", bundle.Projects[0].Status, "paused")
	assertEqual(t, "project color", bundle.Projects[0].Color, "#ff00ff")
	assertEqual(t, "project budget", bundle.Projects[0].BudgetCents, 99000)
	assertEqual(t, "project repositories", bundle.Projects[0].Repositories, `["kdlbs/kandev"]`)
	assertEqual(t, "project executor config", bundle.Projects[0].ExecutorConfig,
		`{"type":"local_docker"}`)
}

// TestScanFilesystem_DropsNameReferences records a gap that remains for
// routines and projects: the scan does not carry assignee_name or
// lead_agent_name off disk even though both are present in the fixture
// files. The result is a lossy sync for those two fields — applying a
// checked-in workspace config erases routine assignees and project leads
// rather than resolving the portable names to IDs. Agent reports_to no
// longer drops (see TestScanFilesystem_CarriesEveryFieldFromDisk).
//
// Recorded rather than fixed (test-only change); carrying the remaining
// names through the scan and resolving them on import must update this test.
func TestScanFilesystem_DropsNameReferences(t *testing.T) {
	env := newTestEnv(t)
	seedFullFSState(t, env)

	bundle, _, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	assertEqual(t, "routine assignee", bundle.Routines[0].AssigneeName, "")
	assertEqual(t, "project lead", bundle.Projects[0].LeadAgentName, "")
}

// TestScanFilesystem_NoConfigOnDisk documents that a missing workspaces
// directory is not an error and yields an empty (non-nil) bundle.
func TestScanFilesystem_NoConfigOnDisk(t *testing.T) {
	env := newTestEnv(t)
	if err := os.RemoveAll(filepath.Join(env.base, "workspaces")); err != nil {
		t.Fatalf("remove workspaces dir: %v", err)
	}

	bundle, parseErrs, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle should be non-nil even with no config on disk")
	}
	assertEqual(t, "settings name", bundle.Settings.Name, defaultWorkspaceName)
	assertEqual(t, "agents", len(bundle.Agents), 0)
	assertEqual(t, "parse errors", len(parseErrs), 0)
}

// TestScanFilesystem_ReportsParseErrors covers the malformed-input path: a
// broken file is reported rather than swallowed, and the good files still load.
func TestScanFilesystem_ReportsParseErrors(t *testing.T) {
	env := newTestEnv(t)
	env.writeFSAgent(t, "ada", fullAgentYAML)
	env.writeFSAgent(t, "broken", "name: [unclosed\n")

	bundle, parseErrs, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	if len(parseErrs) != 1 {
		t.Fatalf("parse errors: got %d (%+v), want 1", len(parseErrs), parseErrs)
	}
	assertEqual(t, "parse error workspace", parseErrs[0].WorkspaceID, defaultWorkspaceName)
	if !strings.HasSuffix(parseErrs[0].FilePath, "broken.yml") {
		t.Errorf("parse error file path %q does not name broken.yml", parseErrs[0].FilePath)
	}
	if parseErrs[0].Error == "" {
		t.Error("parse error message is empty")
	}
	assertStrings(t, "good agent still loaded", agentBundleNames(bundle.Agents), []string{"ada"})
}

// TestScanFilesystem_UnreadableWorkspacesDirIsAnError separates "no config on
// disk" (fine) from "config root is unusable" (an error the caller must see).
func TestScanFilesystem_UnreadableWorkspacesDirIsAnError(t *testing.T) {
	env := newTestEnv(t)
	wsRoot := filepath.Join(env.base, "workspaces")
	if err := os.RemoveAll(wsRoot); err != nil {
		t.Fatalf("remove workspaces dir: %v", err)
	}
	// A regular file where the directory should be: ReadDir fails with
	// something other than "not exist".
	writeFile(t, wsRoot, "not a directory\n")

	bundle, _, err := env.svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err == nil {
		t.Fatal("expected an error for an unreadable workspaces dir, got nil")
	}
	if !strings.HasPrefix(err.Error(), "scan workspaces:") {
		t.Errorf("error %q does not start with %q", err.Error(), "scan workspaces:")
	}
	if bundle != nil {
		t.Errorf("bundle should be nil on error, got %+v", bundle)
	}
}

// TestScanFilesystem_NilLoaderIsRejected covers the guard in front of every
// filesystem read.
func TestScanFilesystem_NilLoaderIsRejected(t *testing.T) {
	env := newTestEnv(t)
	svc := NewConfigService(env.repo, nil, env.writer, logger.Default(), env.activity)

	bundle, parseErrs, err := svc.ScanFilesystem(context.Background(), testWorkspaceID)
	if err == nil {
		t.Fatal("expected an error for a nil config loader, got nil")
	}
	assertEqual(t, "error message", err.Error(), "config loader not initialized")
	if bundle != nil || parseErrs != nil {
		t.Errorf("bundle and errors should be nil, got %+v / %+v", bundle, parseErrs)
	}
}

func TestParseErrorsFromLoader_EmptyIsNil(t *testing.T) {
	env := newTestEnv(t)
	if got := parseErrorsFromLoader(env.loader); got != nil {
		t.Errorf("expected nil for a clean loader, got %+v", got)
	}
}

// TestIncomingDiff_CombinesPreviewAndDeletions is the FS→DB direction: files
// not yet in the DB are creates, matching names are updates, and DB rows with
// no file are deletions.
func TestIncomingDiff_CombinesPreviewAndDeletions(t *testing.T) {
	env := newTestEnv(t)
	seedFullFSState(t, env)
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, testWorkspaceID, "gone-agent")
	seedSkill(t, env, testWorkspaceID, "gone-skill")
	seedRoutine(t, env, testWorkspaceID, "gone-routine")
	seedProject(t, env, testWorkspaceID, "gone-project")

	diff, err := env.svc.IncomingDiff(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("IncomingDiff: %v", err)
	}

	assertEqual(t, "direction", diff.Direction, "incoming")
	if diff.Preview == nil {
		t.Fatal("preview is nil")
	}
	assertEqual(t, "errors", len(diff.Errors), 0)

	assertStrings(t, "agents updated", diff.Preview.Agents.Updated, []string{"ada"})
	assertStrings(t, "agents deleted", diff.Preview.Agents.Deleted, []string{"gone-agent"})
	assertStrings(t, "skills created", diff.Preview.Skills.Created, []string{"code-review"})
	assertStrings(t, "skills deleted", diff.Preview.Skills.Deleted, []string{"gone-skill"})
	assertStrings(t, "routines created", diff.Preview.Routines.Created, []string{"standup"})
	assertStrings(t, "routines deleted", diff.Preview.Routines.Deleted, []string{"gone-routine"})
	assertStrings(t, "projects created", diff.Preview.Projects.Created, []string{"apollo"})
	assertStrings(t, "projects deleted", diff.Preview.Projects.Deleted, []string{"gone-project"})
}

func TestIncomingDiff_SurfacesParseErrors(t *testing.T) {
	env := newTestEnv(t)
	env.writeFSAgent(t, "broken", "name: [unclosed\n")

	diff, err := env.svc.IncomingDiff(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("IncomingDiff: %v", err)
	}
	if len(diff.Errors) != 1 {
		t.Fatalf("errors: got %d (%+v), want 1", len(diff.Errors), diff.Errors)
	}
}

func TestIncomingDiff_PropagatesErrors(t *testing.T) {
	t.Run("scan failure", func(t *testing.T) {
		env := newTestEnv(t)
		svc := NewConfigService(env.repo, nil, env.writer, logger.Default(), env.activity)
		if _, err := svc.IncomingDiff(context.Background(), testWorkspaceID); err == nil {
			t.Fatal("expected an error for a nil config loader, got nil")
		}
	})
	t.Run("preview failure", func(t *testing.T) {
		env := newTestEnv(t)
		env.closeDB(t)
		diff, err := env.svc.IncomingDiff(context.Background(), testWorkspaceID)
		if err == nil {
			t.Fatal("expected an error from a closed database, got nil")
		}
		if diff != nil {
			t.Errorf("diff should be nil on error, got %+v", diff)
		}
	})
	// PreviewImport and appendDeletions read the same four tables, so a
	// broken table always surfaces from the preview first. Each entity is
	// still worth covering: none of them may be swallowed.
	for _, table := range []string{
		"agent_profiles", "office_skills", "office_routines", "office_projects",
	} {
		t.Run("broken "+table, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, table)
			if _, err := env.svc.IncomingDiff(context.Background(), testWorkspaceID); err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", table)
			}
		})
	}
}

// TestOutgoingDiff_ComparesDatabaseAgainstDisk is the DB→FS direction: DB rows
// with no file are creates, and files with no DB row are deletions.
func TestOutgoingDiff_ComparesDatabaseAgainstDisk(t *testing.T) {
	env := newTestEnv(t)
	env.writeFSAgent(t, "ada", fullAgentYAML)
	env.writeFSAgent(t, "stale", "name: stale\nrole: engineer\n")
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, testWorkspaceID, "fresh")

	diff, err := env.svc.OutgoingDiff(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("OutgoingDiff: %v", err)
	}

	assertEqual(t, "direction", diff.Direction, "outgoing")
	assertEqual(t, "errors", len(diff.Errors), 0)
	assertStrings(t, "agents updated", diff.Preview.Agents.Updated, []string{"ada"})
	assertStrings(t, "agents created", diff.Preview.Agents.Created, []string{"fresh"})
	assertStrings(t, "agents deleted", diff.Preview.Agents.Deleted, []string{"stale"})
}

func TestOutgoingDiff_PropagatesErrors(t *testing.T) {
	t.Run("export failure", func(t *testing.T) {
		env := newTestEnv(t)
		env.closeDB(t)
		diff, err := env.svc.OutgoingDiff(context.Background(), testWorkspaceID)
		if err == nil {
			t.Fatal("expected an error from a closed database, got nil")
		}
		if diff != nil {
			t.Errorf("diff should be nil on error, got %+v", diff)
		}
	})
	t.Run("scan failure", func(t *testing.T) {
		env := newTestEnv(t)
		svc := NewConfigService(env.repo, nil, env.writer, logger.Default(), env.activity)
		if _, err := svc.OutgoingDiff(context.Background(), testWorkspaceID); err == nil {
			t.Fatal("expected an error for a nil config loader, got nil")
		}
	})
}

// TestApplyIncoming_ImportsAndPrunes asserts the full FS→DB apply: files
// create or update rows, and rows with no file are removed.
func TestApplyIncoming_ImportsAndPrunes(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedFullFSState(t, env)
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, testWorkspaceID, "gone-agent")
	seedSkill(t, env, testWorkspaceID, "gone-skill")

	result, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 3) // skill + routine + project
	assertEqual(t, "updated count", result.UpdatedCount, 1) // ada

	assertRemainingNames(t, env, testWorkspaceID, []string{"ada"}, []string{"code-review"},
		[]string{"standup"}, []string{"apollo"})

	// The updated agent carries the on-disk values.
	agent := agentByName(t, env, testWorkspaceID, "ada")
	assertEqual(t, "agent role", string(agent.Role), "cto")
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent executor preference", agent.ExecutorPreference, "local_docker")
}

// TestApplyIncoming_IsIdempotent runs the same FS state through twice: the
// second pass updates in place and prunes nothing further.
func TestApplyIncoming_IsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedFullFSState(t, env)

	first, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("first ApplyIncoming: %v", err)
	}
	agentID := agentByName(t, env, testWorkspaceID, "ada").ID

	second, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("second ApplyIncoming: %v", err)
	}
	assertEqual(t, "first created", first.CreatedCount, 4)
	assertEqual(t, "second created", second.CreatedCount, 0)
	assertEqual(t, "second updated", second.UpdatedCount, 4)
	assertEqual(t, "agent id stable", agentByName(t, env, testWorkspaceID, "ada").ID, agentID)
	assertRemainingNames(t, env, testWorkspaceID, []string{"ada"}, []string{"code-review"},
		[]string{"standup"}, []string{"apollo"})
}

// TestApplyIncoming_EmptyFilesystemClearsWorkspace is the destructive case:
// with no config on disk, the sync prunes every row.
func TestApplyIncoming_EmptyFilesystemClearsWorkspace(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")
	seedSkill(t, env, testWorkspaceID, "code-review")
	seedRoutine(t, env, testWorkspaceID, "standup")
	seedProject(t, env, testWorkspaceID, "apollo")

	result, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 0)
	assertEqual(t, "updated count", result.UpdatedCount, 0)
	assertRemainingNames(t, env, testWorkspaceID, nil, nil, nil, nil)
}

func TestApplyIncoming_PropagatesErrors(t *testing.T) {
	t.Run("scan failure", func(t *testing.T) {
		env := newTestEnv(t)
		svc := NewConfigService(env.repo, nil, env.writer, logger.Default(), env.activity)
		result, err := svc.ApplyIncoming(context.Background(), testWorkspaceID)
		if err == nil {
			t.Fatal("expected an error for a nil config loader, got nil")
		}
		if result != nil {
			t.Errorf("result should be nil on error, got %+v", result)
		}
	})
	t.Run("import failure", func(t *testing.T) {
		env := newTestEnv(t)
		env.closeDB(t)
		if _, err := env.svc.ApplyIncoming(context.Background(), testWorkspaceID); err == nil {
			t.Fatal("expected an error from a closed database, got nil")
		}
	})
}

// TestApplyOutgoing_WritesDatabaseStateToDisk is the DB→FS apply: every row
// becomes a file the loader can read back.
func TestApplyOutgoing_WritesDatabaseStateToDisk(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedFullDBState(t, env, testWorkspaceID)

	if err := env.svc.ApplyOutgoing(ctx, testWorkspaceID); err != nil {
		t.Fatalf("ApplyOutgoing: %v", err)
	}

	bundle, _, err := env.svc.ScanFilesystem(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	assertStrings(t, "agents on disk",
		sortedCopy(agentBundleNames(bundle.Agents)), []string{"ada", "grace"})
	assertStrings(t, "skills on disk", skillBundleSlugs(bundle.Skills), []string{"code-review"})
	assertStrings(t, "routines on disk", routineBundleNames(bundle.Routines), []string{"standup"})
	assertStrings(t, "projects on disk", projectBundleNames(bundle.Projects), []string{"apollo"})

	var ada AgentConfig
	for _, a := range bundle.Agents {
		if a.Name == "ada" {
			ada = a
		}
	}
	assertEqual(t, "agent role", ada.Role, "cto")
	assertEqual(t, "agent budget", ada.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent desired skills", ada.DesiredSkills, `["go","sql"]`)
	assertEqual(t, "project status", bundle.Projects[0].Status, "paused")
	assertEqual(t, "project budget", bundle.Projects[0].BudgetCents, 99000)
}

// TestApplyOutgoing_DoesNotDeleteStaleFiles records the asymmetry with
// ApplyIncoming: the incoming apply prunes DB rows missing from disk, but the
// outgoing apply only writes, so a file with no DB row survives even though
// OutgoingDiff just listed it as a deletion. A user who applies the outgoing
// diff in the Sync UI can still commit or later re-import the stale file.
//
// Recorded rather than fixed: making export-fs prune is a production change,
// and it must invert this test.
func TestApplyOutgoing_DoesNotDeleteStaleFiles(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.writeFSAgent(t, "stale", "name: stale\nrole: engineer\n")
	seedAgent(t, env, testWorkspaceID, "ada")

	diff, err := env.svc.OutgoingDiff(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("OutgoingDiff: %v", err)
	}
	assertStrings(t, "diff proposes a deletion", diff.Preview.Agents.Deleted, []string{"stale"})

	if err := env.svc.ApplyOutgoing(ctx, testWorkspaceID); err != nil {
		t.Fatalf("ApplyOutgoing: %v", err)
	}
	if _, err := os.Stat(env.wsPath("agents", "stale.yml")); err != nil {
		t.Errorf("stale file should survive an outgoing apply: %v", err)
	}
}

func TestApplyOutgoing_NilWriterIsRejected(t *testing.T) {
	env := newTestEnv(t)
	svc := NewConfigService(env.repo, env.loader, nil, logger.Default(), env.activity)

	err := svc.ApplyOutgoing(context.Background(), testWorkspaceID)
	if err == nil {
		t.Fatal("expected an error for a nil config writer, got nil")
	}
	assertEqual(t, "error message", err.Error(), "config writer not initialized")
}

func TestApplyOutgoing_PropagatesExportError(t *testing.T) {
	env := newTestEnv(t)
	env.closeDB(t)

	if err := env.svc.ApplyOutgoing(context.Background(), testWorkspaceID); err == nil {
		t.Fatal("expected an error from a closed database, got nil")
	}
}

// TestSyncRoundTrip_DiskToDatabaseToDisk applies incoming then outgoing and
// asserts the values survive both crossings unchanged.
func TestSyncRoundTrip_DiskToDatabaseToDisk(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedFullFSState(t, env)

	if _, err := env.svc.ApplyIncoming(ctx, testWorkspaceID); err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}
	if err := env.svc.ApplyOutgoing(ctx, testWorkspaceID); err != nil {
		t.Fatalf("ApplyOutgoing: %v", err)
	}

	bundle, _, err := env.svc.ScanFilesystem(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	if len(bundle.Agents) != 1 {
		t.Fatalf("agents after round trip: got %d, want 1", len(bundle.Agents))
	}
	assertEqual(t, "agent name", bundle.Agents[0].Name, "ada")
	assertEqual(t, "agent role", bundle.Agents[0].Role, "cto")
	assertEqual(t, "agent icon", bundle.Agents[0].Icon, "🤖")
	assertEqual(t, "agent budget", bundle.Agents[0].BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", bundle.Agents[0].MaxConcurrentSessions, 3)
	assertEqual(t, "agent desired skills", bundle.Agents[0].DesiredSkills, `["go","sql"]`)
	assertEqual(t, "agent executor preference", bundle.Agents[0].ExecutorPreference, "local_docker")

	assertEqual(t, "routine description", bundle.Routines[0].Description, "daily standup")
	assertEqual(t, "routine task template", bundle.Routines[0].TaskTemplate, "summarise yesterday")
	assertEqual(t, "routine concurrency", bundle.Routines[0].ConcurrencyPolicy, "skip")

	assertEqual(t, "project description", bundle.Projects[0].Description, "moon shot")
	assertEqual(t, "project color", bundle.Projects[0].Color, "#ff00ff")
	assertEqual(t, "project budget", bundle.Projects[0].BudgetCents, 99000)
	assertEqual(t, "project repositories", bundle.Projects[0].Repositories, `["kdlbs/kandev"]`)
	assertEqual(t, "project executor config", bundle.Projects[0].ExecutorConfig,
		`{"type":"local_docker"}`)

	// The project's status is the one value the round trip does not
	// preserve: the importer forces every created project to "active".
	assertEqual(t, "project status", bundle.Projects[0].Status, "active")
}
