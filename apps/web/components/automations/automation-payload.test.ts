import { afterEach, describe, expect, it, vi } from "vitest";

const createRepositoryAction = vi.fn();

vi.mock("@/app/actions/workspaces", () => ({
  createRepositoryAction: (...args: unknown[]) => createRepositoryAction(...args),
}));

import {
  buildCreatePayload,
  buildUpdatePayload,
  resolveNormalizedRepositoryIds,
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
    repositorySelections: [],
    prompt: "Run it",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
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
      { kind: "registered", id: "repo-a" },
      { kind: "discovered", path: "/tmp/repo-b", name: "repo-b", defaultBranch: "main" },
    ]);

    expect(result.ids).toEqual(["repo-a", "repo-new"]);
    expect(result.selections).toEqual([
      { kind: "registered", id: "repo-a" },
      { kind: "registered", id: "repo-new" },
    ]);
    expect(createRepositoryAction).toHaveBeenCalledTimes(1);
    expect(createRepositoryAction).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: "ws-1", local_path: "/tmp/repo-b" }),
    );
  });

  it("resolves an empty selections list to empty output without registering anything", async () => {
    const result = await resolveRepositoryIds("ws-1", []);

    expect(result.ids).toEqual([]);
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
    { kind: "registered" as const, id: "repo-a" },
    { kind: "registered" as const, id: "repo-b" },
  ];

  it("resolves every selection when the executor supports multi-repo and it's not a PR trigger", async () => {
    const result = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: true,
      isPRTrigger: false,
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
      isPRTrigger: false,
    });
    expect(result.ids).toEqual(["repo-a"]);
  });

  it("truncates a stale multi-repository selection to one ID for a github_pr trigger", async () => {
    const result = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: true,
      isPRTrigger: true,
    });
    expect(result.ids).toEqual(["repo-a"]);
  });

  it("feeds the truncated ids straight into buildUpdatePayload", async () => {
    const { ids } = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: false,
      isPRTrigger: false,
    });
    const payload = buildUpdatePayload(baseForm(), ids);
    expect(payload.repository_ids).toEqual(["repo-a"]);
  });

  it("feeds the truncated ids straight into buildCreatePayload", async () => {
    const { ids } = await resolveNormalizedRepositoryIds("ws-1", twoRegistered, {
      supportsMultiRepo: false,
      isPRTrigger: false,
    });
    const payload = buildCreatePayload("ws-1", baseForm(), ids, []);
    expect(payload.repository_ids).toEqual(["repo-a"]);
  });
});

describe("buildCreatePayload / buildUpdatePayload", () => {
  it("sends repository_ids in row order on create", () => {
    const payload = buildCreatePayload("ws-1", baseForm(), ["repo-a", "repo-b"], []);
    expect(payload.repository_ids).toEqual(["repo-a", "repo-b"]);
  });

  it("sends repository_ids in row order on update", () => {
    const payload = buildUpdatePayload(baseForm(), ["repo-b", "repo-a"]);
    expect(payload.repository_ids).toEqual(["repo-b", "repo-a"]);
  });

  it("sends an empty repository_ids array when no repositories are selected", () => {
    const payload = buildCreatePayload("ws-1", baseForm(), [], []);
    expect(payload.repository_ids).toEqual([]);
  });
});
