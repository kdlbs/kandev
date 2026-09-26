import { afterEach, describe, expect, it, vi } from "vitest";

const createRepositoryAction = vi.fn();

vi.mock("@/app/actions/workspaces", () => ({
  createRepositoryAction: (...args: unknown[]) => createRepositoryAction(...args),
}));

import {
  buildCreatePayload,
  buildUpdatePayload,
  resolveNormalizedRepositoryIds,
  resolveRepositoryIdsForMode,
  resolveRepositoryIds,
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
});
