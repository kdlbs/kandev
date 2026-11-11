import { afterEach, describe, expect, it, vi } from "vitest";

const createRepositoryAction = vi.fn();

vi.mock("@/app/actions/workspaces", () => ({
  createRepositoryAction: (...args: unknown[]) => createRepositoryAction(...args),
}));

import {
  automationDraftRevision,
  reconcileSavedMCPSelection,
  mcpSelectionForUpdate,
  buildCreatePayload,
  buildUpdatePayload,
  canonicalPluginTools,
  normalizeRetryPolicyForTaskMode,
  resolveNormalizedRepositoryIds,
  resolveRepositoryIdsForMode,
  resolveRepositoryIds,
  retryPolicyAfterModeChange,
} from "./automation-payload";
import type { FormState } from "./automation-payload";

function baseForm(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "Test",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "automation_run",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Run it",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
    continuationPolicy: "new_task",
    retryPolicy: {
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    },
    pluginTools: [],
    ...overrides,
  };
}

describe("resolveRepositoryIds", () => {
  afterEach(() => {
    createRepositoryAction.mockReset();
  });

  it("resolves a mix of registered and discovered selections, in order", async () => {
    createRepositoryAction.mockResolvedValue({ id: "repo-new" });

    const result = await resolveRepositoryIds("ws-1", [
      { kind: "registered", id: "repo-a", branch: "release/1" },
      {
        kind: "discovered",
        path: "/tmp/repo-b",
        name: "repo-b",
        defaultBranch: "main",
        branch: "develop",
      },
    ]);

    expect(result.ids).toEqual(["repo-a", "repo-new"]);
    expect(result.selections).toEqual([
      { kind: "registered", id: "repo-a", branch: "release/1" },
      { kind: "registered", id: "repo-new", key: undefined, branch: "develop" },
    ]);
    expect(result.repositories).toEqual([
      { repository_id: "repo-a", base_branch: "release/1" },
      { repository_id: "repo-new", base_branch: "develop" },
    ]);
    expect(createRepositoryAction).toHaveBeenCalledTimes(1);
    expect(createRepositoryAction).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: "ws-1", local_path: "/tmp/repo-b" }),
    );
  });

  it("resolves an empty selections list to empty output without registering anything", async () => {
    const result = await resolveRepositoryIds("ws-1", []);

    expect(result.ids).toEqual([]);
    expect(result.repositories).toEqual([]);
    expect(result.selections).toEqual([]);
    expect(createRepositoryAction).not.toHaveBeenCalled();
  });

  it("does not promote a registered selection", async () => {
    const result = await resolveRepositoryIds("ws-1", [{ kind: "registered", id: "repo-a" }]);

    expect(result.selections).toEqual([{ kind: "registered", id: "repo-a" }]);
    expect(createRepositoryAction).not.toHaveBeenCalled();
  });
});

describe("resolveNormalizedRepositoryIds", () => {
  afterEach(() => {
    createRepositoryAction.mockReset();
  });

  const twoRegistered = [
    { kind: "registered" as const, id: "repo-a", branch: "main" },
    { kind: "registered" as const, id: "repo-b", branch: "develop" },
  ];

  it("resolves every selection when the executor supports multi-repo", async () => {
    const result = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: true,
    });
    expect(result.ids).toEqual(["repo-a", "repo-b"]);
  });

  it("truncates a stale multi-repository selection to one ID when the executor no longer supports multi-repo", async () => {
    // Regression: the picker only *renders* repositorySelections[0] once the
    // executor stops supporting multi-repo, it doesn't truncate the
    // underlying form state — a save that skipped normalization would send
    // both stale repository_ids here.
    const result = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: false,
    });
    expect(result.ids).toEqual(["repo-a"]);
  });

  it("feeds the truncated ids straight into buildUpdatePayload", async () => {
    const { ids } = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: false,
    });
    const payload = buildUpdatePayload(
      baseForm(),
      ids.map((repository_id) => ({ repository_id, base_branch: "main" })),
    );
    expect(payload.repository_ids).toEqual(["repo-a"]);
  });

  it("feeds the truncated ids straight into buildCreatePayload", async () => {
    const { ids } = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: false,
    });
    const payload = buildCreatePayload(
      "ws-1",
      baseForm(),
      ids.map((repository_id) => ({ repository_id, base_branch: "main" })),
      [],
    );
    expect(payload.repository_ids).toEqual(["repo-a"]);
  });
});

describe("buildCreatePayload / buildUpdatePayload", () => {
  it("leaves workflow step resolution to normal task creation", () => {
    const form = baseForm({ workflowStepId: "stale-step" });

    expect(buildCreatePayload("ws-1", form, [], []).workflow_step_id).toBe("");
    expect(buildUpdatePayload(form, []).workflow_step_id).toBe("");
  });

  it("sends repository_ids in row order on create", () => {
    const payload = buildCreatePayload(
      "ws-1",
      baseForm(),
      [
        { repository_id: "repo-a", base_branch: "main" },
        { repository_id: "repo-b", base_branch: "develop" },
      ],
      [],
    );
    expect(payload.repository_ids).toEqual(["repo-a", "repo-b"]);
    expect(payload.repositories).toEqual([
      { repository_id: "repo-a", base_branch: "main" },
      { repository_id: "repo-b", base_branch: "develop" },
    ]);
  });

  it("sends repository_ids in row order on update", () => {
    const payload = buildUpdatePayload(baseForm(), [
      { repository_id: "repo-b", base_branch: "develop" },
      { repository_id: "repo-a", base_branch: "main" },
    ]);
    expect(payload.repository_ids).toEqual(["repo-b", "repo-a"]);
  });

  it("sends an empty repository_ids array when no repositories are selected", () => {
    const payload = buildCreatePayload("ws-1", baseForm(), [], []);
    expect(payload.repository_ids).toEqual([]);
  });

  it("sends the selected continuation policy on create and update", () => {
    const form = baseForm({ continuationPolicy: "reuse_thread" });

    expect(buildCreatePayload("ws-1", form, [], []).continuation_policy).toBe("reuse_thread");
    expect(buildUpdatePayload(form, []).continuation_policy).toBe("reuse_thread");
  });

  it("sends the target and repository modes on create and update", () => {
    const form = baseForm({ taskMode: "normal_task", repositoryMode: "selected" });

    const repositories = [{ repository_id: "repo-a", base_branch: "main" }];
    expect(buildCreatePayload("ws-1", form, repositories, [])).toMatchObject({
      task_mode: "normal_task",
      repository_mode: "selected",
    });
    expect(buildUpdatePayload(form, repositories)).toMatchObject({
      task_mode: "normal_task",
      repository_mode: "selected",
    });
  });

  it("omits plugin tools on unrelated updates and sends explicit selections", () => {
    const form = baseForm();
    expect(buildUpdatePayload(form, [])).not.toHaveProperty("plugin_tools");
    expect(buildCreatePayload("ws-1", form, [], [])).toHaveProperty("plugin_tools", []);

    const selection = [
      {
        plugin_id: "kandev-security",
        installation_id: "installation-1",
        policy_version: "policy-1",
        tool_names: ["list_findings"],
      },
    ];
    expect(buildUpdatePayload(form, [], selection)).toMatchObject({ plugin_tools: selection });
    expect(buildUpdatePayload(form, [], [])).toMatchObject({ plugin_tools: [] });
    expect(buildUpdatePayload(baseForm({ taskMode: "normal_task" }), [], selection)).toMatchObject({
      plugin_tools: [],
    });
  });

  it("compares plugin tool approvals without depending on tool order", () => {
    const left = [
      {
        plugin_id: "kandev-security",
        installation_id: "installation-1",
        policy_version: "policy-1",
        tool_names: ["list_findings", "complete_report"],
      },
    ];
    const right = [{ ...left[0], tool_names: ["complete_report", "list_findings"] }];
    expect(canonicalPluginTools(left)).toEqual(canonicalPluginTools(right));
  });
});

describe("managed destination and retry payloads", () => {
  it("persists only the portable managed destination identity", () => {
    const form = baseForm({
      taskMode: "managed_conversation",
      managedDestination: {
        plugin_id: "kandev-plugin-coordinator",
        instance_key: "daily-brief",
        revision: 3,
      },
    });

    expect(buildCreatePayload("ws-1", form, [], [])).toMatchObject({
      task_mode: "managed_conversation",
      managed_destination: {
        plugin_id: "kandev-plugin-coordinator",
        instance_key: "daily-brief",
        revision: 3,
      },
      repository_ids: [],
    });
    expect(buildUpdatePayload(form, [])).toMatchObject({
      task_mode: "managed_conversation",
      managed_destination: {
        plugin_id: "kandev-plugin-coordinator",
        instance_key: "daily-brief",
        revision: 3,
      },
      repository_ids: [],
    });
  });
  it("serializes canonical disabled retry policy for managed create and update", () => {
    const form = baseForm({
      taskMode: "managed_conversation",
      retryPolicy: {
        mode: "finite",
        max_retries: "4",
        delay_seconds: "30",
        backoff: "exponential",
        history_mode: "timeline",
      },
    });
    const disabledPolicy = {
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    };

    expect(buildCreatePayload("ws-1", form, [], []).retry_policy).toEqual(disabledPolicy);
    expect(buildUpdatePayload(form, []).retry_policy).toEqual(disabledPolicy);
  });

  it("does not resolve stale repository selections for non-selected modes", async () => {
    const result = await resolveRepositoryIdsForMode(
      "ws-1",
      [{ kind: "discovered", path: "/tmp/stale", name: "stale", defaultBranch: "main" }],
      "none",
      { supportsMultiRepo: true },
    );

    expect(result.ids).toEqual([]);
    expect(createRepositoryAction).not.toHaveBeenCalled();
  });
  it("normalizes managed retry drafts and does not restore them for task targets", () => {
    const enabled = {
      mode: "finite" as const,
      max_retries: "4",
      delay_seconds: "30",
      backoff: "exponential" as const,
      history_mode: "timeline" as const,
    };

    const managed = normalizeRetryPolicyForTaskMode("managed_conversation", enabled);
    expect(managed).toEqual({
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    });
    expect(normalizeRetryPolicyForTaskMode("normal_task", managed)).toEqual(managed);
  });
  it("sends decimal retry policy values unchanged", () => {
    const policy = {
      mode: "finite" as const,
      max_retries: "0007",
      delay_seconds: "3600",
      backoff: "exponential" as const,
      history_mode: "timeline" as const,
    };
    const form = baseForm({ retryPolicy: policy });
    expect(buildCreatePayload("ws-1", form, [], []).retry_policy).toEqual(policy);
    expect(buildUpdatePayload(form, []).retry_policy).toEqual(policy);
  });
});

describe("retryPolicyAfterModeChange", () => {
  it("sets a valid finite retry count after disabled mode", () => {
    const disabled = { ...baseForm().retryPolicy, mode: "disabled" as const, max_retries: "0" };
    expect(retryPolicyAfterModeChange(disabled, "finite")).toMatchObject({
      mode: "finite",
      max_retries: "1",
    });
    expect(retryPolicyAfterModeChange({ ...disabled, max_retries: "00" }, "finite")).toMatchObject({
      max_retries: "1",
    });
  });
});

describe("automation MCP selection", () => {
  const selection: import("@/lib/types/automation").AutomationMCPSelection = {
    builtin_packages: ["core", "configuration"],
    definition_ids: ["reports"],
  };
  it("includes an explicit selection in native creation", () => {
    const form = Object.assign(baseForm(), { mcpSelection: selection, mcpSelectionRevision: 0 });
    expect(buildCreatePayload("workspace", form, [], [])).toHaveProperty(
      "mcp_selection",
      selection,
    );
  });
  it("preserves omission on unrelated updates and conditions changed selections", () => {
    const form = Object.assign(baseForm(), { mcpSelection: selection, mcpSelectionRevision: 3 });
    expect(buildUpdatePayload(form, [])).not.toHaveProperty("mcp_selection");
    expect(buildUpdatePayload(form, [], undefined, selection)).toMatchObject({
      mcp_selection: selection,
      expected_mcp_selection_revision: 3,
    });
  });
});

describe("automation draft revision", () => {
  it("ignores the server selection token but tracks package edits", async () => {
    const { automationDraftRevision } = await import("./automation-payload");
    const form = Object.assign(baseForm(), { mcpSelectionRevision: 0 });
    expect(automationDraftRevision(form)).toBe(
      automationDraftRevision({ ...form, mcpSelectionRevision: 1 }),
    );
    expect(automationDraftRevision(form)).not.toBe(
      automationDraftRevision({
        ...form,
        mcpSelection: { builtin_packages: ["core", "configuration"], definition_ids: [] },
      }),
    );
  });
});

it("treats an absent MCP draft like the saved null selection", () => {
  const form = { ...baseForm(), mcpSelection: undefined };
  expect(automationDraftRevision(form)).toEqual(
    automationDraftRevision({ ...form, mcpSelection: null }),
  );
});

it("applies canonical MCP save values without losing in-flight edits", () => {
  const submitted = baseForm({
    mcpSelection: { builtin_packages: ["core"], definition_ids: [] },
    mcpSelectionRevision: 1,
  });
  const saved = {
    mcp_selection: null,
    mcp_selection_revision: 2,
  } as import("@/lib/types/automation").Automation;
  expect(reconcileSavedMCPSelection(submitted, submitted, saved).mcpSelection).toBeNull();
  const edited = {
    ...submitted,
    mcpSelection: { builtin_packages: ["core", "configuration"], definition_ids: [] },
  } as FormState;
  expect(reconcileSavedMCPSelection(edited, submitted, saved)).toMatchObject({
    mcpSelection: edited.mcpSelection,
    mcpSelectionRevision: 2,
  });
});
it("omits MCP grants from managed conversation payloads", () => {
  const form = baseForm({
    taskMode: "managed_conversation",
    mcpSelection: { builtin_packages: ["core"], definition_ids: [] },
  });
  for (const payload of [
    buildCreatePayload("workspace", form, [], []),
    buildUpdatePayload(form, [], undefined, form.mcpSelection!),
  ]) {
    expect(payload).not.toHaveProperty("mcp_selection");
    expect(payload).not.toHaveProperty("expected_mcp_selection_revision");
  }
});

it("resubmits the displayed selection when changing task mode", () => {
  const saved = baseForm({ mcpSelection: { builtin_packages: ["core"], definition_ids: [] } });
  expect(mcpSelectionForUpdate({ ...saved, taskMode: "normal_task" }, saved)).toEqual(
    saved.mcpSelection,
  );
});

it("carries the editor revision when switching to a managed conversation", () => {
  const form = baseForm({ taskMode: "managed_conversation", mcpSelectionRevision: 3 });
  const payload = buildUpdatePayload(form, [], undefined, undefined, true);
  expect(payload).not.toHaveProperty("mcp_selection");
  expect(payload.expected_mcp_selection_revision).toBe(3);
});
