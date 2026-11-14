// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  classifyTaskLookup,
  createTaskCreationReceipt,
  decodeNativeCreateResponse,
  isCapturedPayloadReplayable,
  loadTaskCreationReceipt,
  parseTaskCreationReceipt,
  receiptStorageKey,
  removeTaskCreationReceipt,
  saveTaskCreationReceipt,
  toPersistedTaskCreationReceipt,
  withRenewedDiscardConsent,
  type TaskCreationReceipt,
  type TaskCreationScope,
} from "@/lib/task-creation-receipt";
import type { CreateTaskResponse } from "@/lib/types/http";

const SCOPE: TaskCreationScope = { userId: "user-1", workspaceId: "ws-1", draftId: "task" };

function payload(overrides: Record<string, unknown> = {}) {
  return {
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    title: "Incident task",
    description: "Task brief",
    state: "IN_PROGRESS" as const,
    start_agent: true,
    ...overrides,
  } as Parameters<typeof createTaskCreationReceipt>[0]["payload"];
}

function receipt(overrides: Partial<TaskCreationReceipt> = {}) {
  return {
    ...createTaskCreationReceipt({ scope: SCOPE, payload: payload(), runtimeId: "runtime-1" }),
    ...overrides,
  };
}

afterEach(() => {
  window.sessionStorage.clear();
});

describe("task creation receipt identity", () => {
  it("captures one caller UUID and the exact submitted payload", () => {
    const captured = createTaskCreationReceipt({
      scope: SCOPE,
      payload: payload(),
      runtimeId: "runtime-1",
    });

    expect(captured.submissionId).toBeTruthy();
    expect(captured.externalId).toBe(`ui-task-create/${captured.submissionId}`);
    expect(captured.payload.external_id).toBe(captured.externalId);
    expect(captured.payload.description).toBe("Task brief");
    expect(captured.state).toBe("submitting");
    expect(captured.everUncertain).toBe(false);
    expect(captured.replayable).toBe(true);
  });

  it("marks inline attachment bytes as not replayable after persistence", () => {
    const withInline = receipt({
      payload: payload({
        attachments: [
          {
            type: "image",
            mime_type: "image/png",
            name: "shot.png",
            size_bytes: 4,
            data: "YWJjZA==",
          },
        ],
      }),
    });

    expect(isCapturedPayloadReplayable(withInline.payload)).toBe(false);
    const persisted = toPersistedTaskCreationReceipt(withInline);
    expect(persisted.replayable).toBe(false);
    expect(persisted.payload.attachments?.[0]).not.toHaveProperty("data");
  });

  it("keeps descriptor attachments replayable", () => {
    const descriptor = receipt({
      payload: payload({
        attachments: [
          {
            type: "resource",
            mime_type: "text/plain",
            name: "notes.txt",
            size_bytes: 8,
            attachment_id: "upload-1",
          },
        ],
      }),
    });

    expect(isCapturedPayloadReplayable(descriptor.payload)).toBe(true);
    expect(toPersistedTaskCreationReceipt(descriptor).replayable).toBe(true);
  });

  it("updates only fresh-branch discard consent", () => {
    const original = receipt({
      payload: payload({
        repositories: [
          { repository_id: "repo-1", fresh_branch: true, base_branch: "main" },
          { repository_id: "repo-2", base_branch: "dev" },
        ],
      }),
    });

    const next = withRenewedDiscardConsent(original.payload, ["a.ts"]);
    expect(next.repositories?.[0]).toMatchObject({
      confirm_discard: true,
      consented_dirty_files: ["a.ts"],
    });
    expect(next.repositories?.[1]).toMatchObject({ base_branch: "dev" });
    expect(next.repositories?.[1]).not.toHaveProperty("consented_dirty_files");
  });
});

describe("task creation receipt persistence", () => {
  it("round-trips within one user, workspace, and draft scope", () => {
    const saved = receipt();
    expect(saveTaskCreationReceipt(saved)).toBe(true);

    expect(loadTaskCreationReceipt(SCOPE)?.submissionId).toBe(saved.submissionId);
    expect(loadTaskCreationReceipt({ ...SCOPE, userId: "user-2" })).toBeNull();
    expect(loadTaskCreationReceipt({ ...SCOPE, workspaceId: "ws-2" })).toBeNull();
    expect(loadTaskCreationReceipt({ ...SCOPE, draftId: "subtask:parent-1" })).toBeNull();
  });

  it("reports a storage failure instead of silently discarding the receipt", () => {
    const spy = vi.spyOn(window.sessionStorage, "setItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });
    try {
      expect(saveTaskCreationReceipt(receipt())).toBe(false);
    } finally {
      spy.mockRestore();
    }
  });

  it("rejects malformed or foreign versions", () => {
    expect(parseTaskCreationReceipt(null)).toBeNull();
    expect(parseTaskCreationReceipt({ version: 2 })).toBeNull();
    expect(
      parseTaskCreationReceipt({
        ...receipt(),
        version: 99,
      }),
    ).toBeNull();
  });

  it("scopes storage keys by the user and draft", () => {
    expect(receiptStorageKey(SCOPE)).toContain("kandev.taskCreationReceipt.v1.user-1.ws-1.task");
    expect(receiptStorageKey({ ...SCOPE, userId: null })).toContain(".anonymous.");
  });

  it("removes only its own scope", () => {
    const saved = receipt();
    saveTaskCreationReceipt(saved);
    removeTaskCreationReceipt(SCOPE);
    expect(loadTaskCreationReceipt(SCOPE)).toBeNull();
  });
});

describe("native create response decoding", () => {
  it("accepts a complete created task and retains its session", () => {
    const captured = receipt();
    const response = {
      id: "task-1",
      workspace_id: "ws-1",
      external_id: captured.externalId,
      deduplicated: false,
      creation_complete: true,
      session_id: "session-1",
      queued_for_step_id: "step-2",
    } as unknown as CreateTaskResponse;

    expect(decodeNativeCreateResponse(response, captured)).toMatchObject({
      kind: "accepted",
      taskId: "task-1",
      taskSessionId: "session-1",
      queuedForStepId: "step-2",
    });
  });

  it("reports an existing unsettled task as incomplete", () => {
    const captured = receipt();
    const response = {
      id: "task-1",
      workspace_id: "ws-1",
      external_id: captured.externalId,
      deduplicated: true,
      creation_complete: false,
    } as unknown as CreateTaskResponse;

    expect(decodeNativeCreateResponse(response, captured)).toMatchObject({
      kind: "incomplete",
      taskId: "task-1",
    });
  });

  it("treats missing metadata as uncertain while retaining a known task ID", () => {
    const captured = receipt();
    expect(
      decodeNativeCreateResponse(
        { id: "task-1", workspace_id: "ws-1" } as CreateTaskResponse,
        captured,
      ),
    ).toEqual({ kind: "unknown", taskId: "task-1" });
    expect(decodeNativeCreateResponse({} as CreateTaskResponse, captured)).toEqual({
      kind: "unknown",
      taskId: null,
    });
  });

  it("does not adopt a foreign identity or workspace", () => {
    const captured = receipt();
    const foreignKey = {
      id: "task-2",
      workspace_id: "ws-1",
      external_id: "ui-task-create/other",
      deduplicated: true,
      creation_complete: true,
    } as unknown as CreateTaskResponse;
    expect(decodeNativeCreateResponse(foreignKey, captured)).toEqual({
      kind: "unknown",
      taskId: null,
    });

    const foreignWorkspace = {
      id: "task-2",
      workspace_id: "ws-2",
      external_id: captured.externalId,
      deduplicated: true,
      creation_complete: true,
    } as unknown as CreateTaskResponse;
    expect(decodeNativeCreateResponse(foreignWorkspace, captured)).toEqual({
      kind: "unknown",
      taskId: null,
    });
  });

  it("preserves a verified task ID and rejects a conflicting one", () => {
    const captured = receipt({ taskId: "task-1" });
    const conflicting = {
      id: "task-2",
      workspace_id: "ws-1",
      external_id: captured.externalId,
      deduplicated: false,
      creation_complete: true,
    } as unknown as CreateTaskResponse;
    expect(decodeNativeCreateResponse(conflicting, captured)).toEqual({
      kind: "unknown",
      taskId: null,
    });

    const identityLost = {
      id: "task-1",
      workspace_id: "ws-1",
      deduplicated: false,
      creation_complete: true,
    } as unknown as CreateTaskResponse;
    expect(decodeNativeCreateResponse(identityLost, captured)).toMatchObject({
      kind: "accepted",
      taskId: "task-1",
    });
  });
});

describe("external ID lookup classification", () => {
  it("accepts and marks incomplete lookups with a matching identity", () => {
    const captured = receipt();
    expect(
      classifyTaskLookup(
        {
          id: "task-1",
          workspace_id: "ws-1",
          external_id: captured.externalId,
          creation_complete: true,
        } as never,
        captured,
      ),
    ).toEqual({ kind: "accepted", taskId: "task-1" });
    expect(
      classifyTaskLookup(
        {
          id: "task-1",
          workspace_id: "ws-1",
          external_id: captured.externalId,
          creation_complete: false,
        } as never,
        captured,
      ),
    ).toEqual({ kind: "incomplete", taskId: "task-1" });
  });

  it("never selects a similar task by another workspace, identity, or known task", () => {
    const captured = receipt({ taskId: "task-1" });
    expect(
      classifyTaskLookup(
        {
          id: "task-2",
          workspace_id: "ws-1",
          external_id: captured.externalId,
          creation_complete: true,
        } as never,
        captured,
      ),
    ).toEqual({ kind: "mismatch" });
    expect(
      classifyTaskLookup(
        {
          id: "task-1",
          workspace_id: "ws-2",
          external_id: captured.externalId,
          creation_complete: true,
        } as never,
        captured,
      ),
    ).toEqual({ kind: "mismatch" });
    expect(
      classifyTaskLookup(
        {
          id: "task-1",
          workspace_id: "ws-1",
          external_id: captured.externalId,
          creation_complete: "yes",
        } as never,
        captured,
      ),
    ).toEqual({ kind: "mismatch" });
    expect(
      classifyTaskLookup(
        { id: "task-1", workspace_id: "ws-1", creation_complete: true } as never,
        captured,
      ),
    ).toEqual({ kind: "mismatch" });
    expect(
      classifyTaskLookup(
        { id: "task-1", external_id: captured.externalId, creation_complete: true } as never,
        captured,
      ),
    ).toEqual({ kind: "mismatch" });
  });
});

describe("recovery after a mismatched create response", () => {
  it("recovers the genuine task by its matching lookup identity", () => {
    const captured = receipt();
    const foreign = {
      id: "task-foreign",
      workspace_id: "ws-1",
      external_id: "ui-task-create/other",
      deduplicated: false,
      creation_complete: true,
    } as unknown as CreateTaskResponse;
    expect(decodeNativeCreateResponse(foreign, captured)).toEqual({
      kind: "unknown",
      taskId: null,
    });

    expect(
      classifyTaskLookup(
        {
          id: "task-real",
          workspace_id: "ws-1",
          external_id: captured.externalId,
          creation_complete: true,
        } as never,
        captured,
      ),
    ).toEqual({ kind: "accepted", taskId: "task-real" });
  });
});
