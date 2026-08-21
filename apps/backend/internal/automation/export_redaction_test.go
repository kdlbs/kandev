package automation

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	"gopkg.in/yaml.v3"
)

// exportBothForms exports workspaceID as both the document and zip forms
// and returns the document bytes plus every zip entry's bytes, so a single
// assertion pass can cover both output shapes (AC-6, AC-43).
func exportBothForms(t *testing.T, svc *Service, workspaceID string) (doc []byte, zipEntries [][]byte) {
	t.Helper()
	doc, err := svc.ExportAutomationsDocument(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	zipBody, err := svc.ExportAutomationsZip(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("ExportAutomationsZip: %v", err)
	}
	for _, name := range zipEntryNames(t, zipBody) {
		data, ok := readZipFile(t, zipBody, name)
		if !ok {
			t.Fatalf("zip entry %q listed but not readable", name)
		}
		zipEntries = append(zipEntries, data)
	}
	return doc, zipEntries
}

// AC-6: WebhookSecret must never appear in either export form's bytes, even
// though it is a non-empty, attacker-recognizable value. AC-22 only proves
// the field is classified `exported: false`; it does not prove the DTO
// omits the value at serialization time.
func TestExportRedaction_AC6_WebhookSecretNeverAppearsInOutput(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{"ws-1": true}}
	svc.SetExportWorkspaceLookup(wsLookup)

	const sentinel = "sentinel-webhook-secret-do-not-leak-9f3a1c"
	if err := svc.store.CreateAutomation(context.Background(), &Automation{
		ID:            "auto-secret",
		WorkspaceID:   "ws-1",
		Name:          "Secret Bearer",
		WebhookSecret: sentinel,
	}); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	doc, zipEntries := exportBothForms(t, svc, "ws-1")

	if bytes.Contains(doc, []byte(sentinel)) {
		t.Errorf("document export leaked WebhookSecret: %s", doc)
	}
	for _, entry := range zipEntries {
		if bytes.Contains(entry, []byte(sentinel)) {
			t.Errorf("zip entry leaked WebhookSecret: %s", entry)
		}
	}
}

// AC-23: every field automationFieldDispositions / automationTriggerFieldDispositions
// mark `exported: true` must actually reach the DTO at its declared yaml key
// with the source value, for a fully-populated automation. AC-22 only proves
// the field is classified `exported: true`; it does not prove the value
// survives marshaling.
func TestExportRedaction_AC23_FullyPopulatedAutomationRoundTripsEveryExportedField(t *testing.T) {
	svc, agentLookup, executorLookup, workflowLookup, stepLookup, repoLookup := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{"ws-1": true}}
	svc.SetExportWorkspaceLookup(wsLookup)

	agentLookup.profiles["agent-1"] = &settingsmodels.AgentProfile{Name: "Reviewer", AgentDisplayName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	executorLookup.profiles["exec-1"] = &taskmodels.ExecutorProfile{ExecutorID: "local_docker", Name: "Default Executor"}
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "New Feature Dev"}
	stepLookup.steps["step-1"] = &workflowmodels.WorkflowStep{Name: "Build", WorkflowID: "wf-1"}
	repoLookup.repositories["repo-1"] = &taskmodels.Repository{Name: "kandev"}
	repoLookup.repositories["repo-2"] = &taskmodels.Repository{Name: "kandev-web"}

	a := &Automation{
		ID:                "auto-full",
		WorkspaceID:       "ws-1",
		Name:              "Fully Populated Automation",
		Description:       "Exercises every exported field at once",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Review the diff and leave comments",
		TaskTitleTemplate: "Review: {{.PRTitle}}",
		Enabled:           true,
		MaxConcurrentRuns: 3,
		RepositoryIDs:     []string{"repo-1", "repo-2"},
	}
	if err := svc.store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	if err := svc.store.CreateTrigger(context.Background(), &AutomationTrigger{
		ID:           "trig-scheduled",
		AutomationID: a.ID,
		Type:         TriggerTypeScheduled,
		// dry_run's value ("true") is a string whose bare text would resolve to
		// !!bool if emitted without an explicit tag, and retries is a JSON
		// number that must NOT carry !!str — both required by AC-23's Config
		// clause so the tag half of the assertion below is not vacuous.
		Config:  []byte(`{"cron":"0 9 * * 1-5","dry_run":"true","retries":3}`),
		Enabled: true,
	}); err != nil {
		t.Fatalf("CreateTrigger scheduled: %v", err)
	}
	if err := svc.store.CreateTrigger(context.Background(), &AutomationTrigger{
		ID:           "trig-webhook",
		AutomationID: a.ID,
		Type:         TriggerTypeWebhook,
		Config:       []byte(`{"branch":"main"}`),
		Enabled:      false,
	}); err != nil {
		t.Fatalf("CreateTrigger webhook: %v", err)
	}

	doc, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	automation := lookupMapKey(t, singleAutomationNode(t, &root), "")

	assertScalarField(t, automation, "name", "Fully Populated Automation")
	assertScalarField(t, automation, "description", "Exercises every exported field at once")
	assertScalarField(t, automation, "prompt", "Review the diff and leave comments")
	assertScalarField(t, automation, "task_title_template", "Review: {{.PRTitle}}")
	assertScalarField(t, automation, "enabled", "true")
	assertScalarField(t, automation, "max_concurrent_runs", "3")

	workflow := requireMapKey(t, automation, "workflow")
	assertScalarField(t, workflow, "name", "New Feature Dev")
	assertScalarField(t, workflow, "step", "Build")

	agentProfile := requireMapKey(t, automation, "agent_profile")
	assertScalarField(t, agentProfile, "agent_name", "Claude Code")
	assertScalarField(t, agentProfile, "model", "claude-sonnet-5")
	assertScalarField(t, agentProfile, "mode", "plan")

	executorProfile := requireMapKey(t, automation, "executor_profile")
	assertScalarField(t, executorProfile, "executor", "local_docker")
	assertScalarField(t, executorProfile, "name", "Default Executor")

	repositories := requireMapKey(t, automation, "repositories")
	wantRepos := []string{"kandev", "kandev-web"}
	if len(repositories.Content) != len(wantRepos) {
		t.Fatalf("repositories has %d entries, want %d (%v)", len(repositories.Content), len(wantRepos), repositories.Content)
	}
	for i, want := range wantRepos {
		if got := repositories.Content[i].Value; got != want {
			t.Errorf("repositories[%d] = %q, want %q", i, got, want)
		}
	}

	triggers := requireMapKey(t, automation, "triggers")
	if len(triggers.Content) != 2 {
		t.Fatalf("triggers has %d entries, want 2", len(triggers.Content))
	}
	// buildExportTriggers orders by type then id: "scheduled" < "webhook".
	scheduled := triggers.Content[0]
	assertScalarField(t, scheduled, "type", "scheduled")
	assertScalarField(t, scheduled, "enabled", "true")
	scheduledConfig := requireMapKey(t, scheduled, "config")
	// AC-23's Config clause: every scalar checked on both lexeme and re-parsed
	// Tag. "cron" and "dry_run" are JSON strings (dry_run's text is ambiguous
	// and would resolve to !!bool if emitted bare) so both must carry !!str;
	// "retries" is a JSON number and must NOT carry !!str.
	assertConfigScalar(t, scheduledConfig, "cron", "0 9 * * 1-5", true)
	assertConfigScalar(t, scheduledConfig, "dry_run", "true", true)
	assertConfigScalar(t, scheduledConfig, "retries", "3", false)

	webhook := triggers.Content[1]
	assertScalarField(t, webhook, "type", "webhook")
	assertScalarField(t, webhook, "enabled", "false")
	webhookConfig := requireMapKey(t, webhook, "config")
	assertConfigScalar(t, webhookConfig, "branch", "main", true)
}

// assertConfigScalar checks a trigger Config scalar's re-parsed lexeme
// (Value) and type tag (Tag) together, per AC-23's Config clause: a
// lexeme-only comparison passes even when a string has been silently
// retyped (e.g. a JSON string "true" emitted bare re-parses with
// Value == "true" but Tag == "!!bool"), so both must be asserted. Pass
// wantStrTag=true for JSON strings/keys (expected Tag "!!str") and false for
// JSON numbers (expected Tag anything but "!!str", i.e. AC-41's unquoted
// number rule observed from the re-parsed side).
func assertConfigScalar(t *testing.T, mapping *yaml.Node, key, wantValue string, wantStrTag bool) {
	t.Helper()
	v := requireMapKey(t, mapping, key)
	if v.Kind != yaml.ScalarNode {
		t.Fatalf("config field %q is not a scalar (kind %v)", key, v.Kind)
	}
	if v.Value != wantValue {
		t.Errorf("config field %q value = %q, want %q", key, v.Value, wantValue)
	}
	if wantStrTag && v.Tag != "!!str" {
		t.Errorf("config field %q tag = %q, want !!str", key, v.Tag)
	}
	if !wantStrTag && v.Tag == "!!str" {
		t.Errorf("config field %q tag = !!str, want a non-string tag", key)
	}
}

// AC-43: none of the excluded columns may leak into either export form,
// under any key-name mangling, outside a trigger's freeform config subtree.
// This is distinct from AC-6 (a single secret's byte-absence): AC-43 covers
// every excluded column at once, plus a structural scan of every mapping
// key at every depth, since a leak could surface under a renamed or nested
// key that a plain byte-substring check on the field's own name would miss.
func TestExportRedaction_AC43_NoExcludedColumnLeaksUnderAnyKeyOrValue(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{"ws-1": true}}
	svc.SetExportWorkspaceLookup(wsLookup)

	const (
		sentinelWebhookSecret = "sentinel-webhook-secret-ac43-7d2e"
		sentinelWorkspaceID   = "sentinel-workspace-id-ac43-4b1f"
		sentinelAutomationID  = "sentinel-automation-id-ac43-6c9a"
		sentinelExecMode      = "sentinel-execution-mode-ac43-2e8b"
		sentinelLegacyRepoID  = "sentinel-legacy-repository-id-ac43-9a3d"
	)

	a := &Automation{
		ID:            sentinelAutomationID,
		WorkspaceID:   sentinelWorkspaceID,
		Name:          "Excluded Column Probe",
		WebhookSecret: sentinelWebhookSecret,
	}
	if err := svc.store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	// The normal write path hardcodes execution_mode and never writes the
	// legacy repository_id column; both need a raw UPDATE to seed a sentinel.
	if _, err := svc.store.db.ExecContext(context.Background(),
		`UPDATE automations SET execution_mode = ?, repository_id = ? WHERE id = ?`,
		sentinelExecMode, sentinelLegacyRepoID, a.ID); err != nil {
		t.Fatalf("seed legacy columns: %v", err)
	}
	if err := svc.store.CreateTrigger(context.Background(), &AutomationTrigger{
		ID:              sentinelAutomationID, // reused for automation_id/id column coverage
		AutomationID:    a.ID,
		Type:            TriggerTypeWebhook,
		Config:          []byte(`{"branch":"main"}`),
		Enabled:         true,
		LastEvaluatedAt: timePtr(time.Now().UTC()),
	}); err != nil {
		t.Fatalf("CreateTrigger: %v", err)
	}
	// wsLookup keys ws-1, but the automation's real WorkspaceID is the
	// sentinel; register the sentinel workspace so export can find it.
	wsLookup.exists[sentinelWorkspaceID] = true

	doc, zipEntries := exportBothForms(t, svc, sentinelWorkspaceID)

	sentinels := []string{
		sentinelWebhookSecret,
		sentinelWorkspaceID,
		sentinelAutomationID,
		sentinelExecMode,
		sentinelLegacyRepoID,
	}
	checkBytes := func(label string, body []byte) {
		for _, s := range sentinels {
			if bytes.Contains(body, []byte(s)) {
				t.Errorf("%s leaked sentinel %q: %s", label, s, body)
			}
		}
	}
	checkBytes("document", doc)
	for i, entry := range zipEntries {
		checkBytes(fmt.Sprintf("zip entry %d", i), entry)
	}

	excludedColumnNames := []string{
		"webhook_secret", "last_evaluated_at", "last_triggered_at",
		"created_at", "updated_at", "workspace_id", "automation_id",
		"id", "execution_mode", "repository_id", "legacy_board_card",
	}
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("yaml.Unmarshal(document): %v", err)
	}
	assertNoExcludedKeys(t, "document", &root, excludedColumnNames)

	for i, entry := range zipEntries {
		var entryRoot yaml.Node
		if err := yaml.Unmarshal(entry, &entryRoot); err != nil {
			t.Fatalf("yaml.Unmarshal(zip entry %d): %v", i, err)
		}
		assertNoExcludedKeys(t, "zip entry", &entryRoot, excludedColumnNames)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// singleAutomationNode descends the export document's root to the sole
// entry of its `automations` list.
func singleAutomationNode(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	doc := root
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) != 1 {
			t.Fatalf("document root has %d children, want 1", len(doc.Content))
		}
		doc = doc.Content[0]
	}
	automations := requireMapKey(t, doc, "automations")
	if len(automations.Content) != 1 {
		t.Fatalf("automations has %d entries, want 1", len(automations.Content))
	}
	return automations.Content[0]
}

// requireMapKey returns the value node for key in a YAML mapping node,
// failing the test if the mapping has no such key.
func requireMapKey(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	v := lookupMapKey(t, mapping, key)
	if v == nil {
		t.Fatalf("key %q not found in mapping (keys present: %v)", key, mapKeys(mapping))
	}
	return v
}

// lookupMapKey returns the value node for key, or nil if absent. Passing an
// empty key returns mapping itself, for chaining with singleAutomationNode.
func lookupMapKey(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if key == "" {
		return mapping
	}
	if mapping.Kind != yaml.MappingNode {
		t.Fatalf("expected a mapping node, got kind %v", mapping.Kind)
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func mapKeys(mapping *yaml.Node) []string {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	var keys []string
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i].Value)
	}
	return keys
}

func assertScalarField(t *testing.T, mapping *yaml.Node, key, want string) {
	t.Helper()
	v := requireMapKey(t, mapping, key)
	if v.Kind != yaml.ScalarNode {
		t.Fatalf("field %q is not a scalar (kind %v)", key, v.Kind)
	}
	if v.Value != want {
		t.Errorf("field %q = %q, want %q", key, v.Value, want)
	}
}

// assertNoExcludedKeys walks every mapping key at every depth of node
// EXCEPT inside any subtree rooted at a `config` key (trigger configs are
// freeform and may coincidentally contain a key that normalizes to an
// excluded column name without that being a leak), and fails the test if
// any visited key, normalized by lowercasing and stripping `_`/`-`,
// matches the same normalization of any excludedColumns entry.
func assertNoExcludedKeys(t *testing.T, label string, node *yaml.Node, excludedColumns []string) {
	t.Helper()
	normalizedExcluded := make(map[string]string, len(excludedColumns))
	for _, c := range excludedColumns {
		normalizedExcluded[normalizeKey(c)] = c
	}
	walkKeysExceptConfig(node, func(key string) {
		norm := normalizeKey(key)
		if original, bad := normalizedExcluded[norm]; bad {
			t.Errorf("%s: key %q normalizes to excluded column %q", label, key, original)
		}
	})
}

func normalizeKey(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// walkKeysExceptConfig visits every mapping key in node at every depth,
// calling visit(key) for each one, but does not descend into the value of
// any key literally named "config".
func walkKeysExceptConfig(node *yaml.Node, visit func(key string)) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, c := range node.Content {
			walkKeysExceptConfig(c, visit)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			visit(key.Value)
			if key.Value == "config" {
				continue
			}
			walkKeysExceptConfig(value, visit)
		}
	case yaml.SequenceNode:
		for _, c := range node.Content {
			walkKeysExceptConfig(c, visit)
		}
	}
}
