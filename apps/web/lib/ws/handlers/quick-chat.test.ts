import { describe, it, expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskEventPayload } from "@/lib/types/backend";
import { quickChatSessionFromTaskEvent, syncQuickChatFromTaskEvent } from "./quick-chat";

function payload(overrides: Partial<TaskEventPayload> = {}): TaskEventPayload {
  return {
    task_id: "task-1",
    workspace_id: "ws-1",
    workflow_id: "",
    workflow_step_id: "",
    title: "Claude - Chat 1",
    is_ephemeral: true,
    primary_session_id: "session-1",
    metadata: { agent_profile_id: "agent-1" },
    ...overrides,
  } as TaskEventPayload;
}

function makeStore() {
  const upsertQuickChatSessionFromEvent = vi.fn();
  const removeQuickChatSessionsForTask = vi.fn();
  return {
    store: {
      getState: () => ({ upsertQuickChatSessionFromEvent, removeQuickChatSessionsForTask }),
    } as unknown as StoreApi<AppState>,
    upsertQuickChatSessionFromEvent,
    removeQuickChatSessionsForTask,
  };
}

describe("quickChatSessionFromTaskEvent", () => {
  it("maps a quick-chat task event onto a tab", () => {
    expect(quickChatSessionFromTaskEvent(payload())).toEqual({
      kind: "chat",
      sessionId: "session-1",
      workspaceId: "ws-1",
      taskId: "task-1",
      name: "Claude - Chat 1",
      agentProfileId: "agent-1",
    });
  });

  it("marks a config-mode task as a config tab", () => {
    const session = quickChatSessionFromTaskEvent(
      payload({ metadata: { config_mode: true, agent_profile_id: "agent-1" } }),
    );

    expect(session?.kind).toBe("config");
  });

  it("leaves the placeholder title unnamed so the tab falls back to its default", () => {
    expect(quickChatSessionFromTaskEvent(payload({ title: "Quick Chat" }))?.name).toBeUndefined();
  });

  it.each([
    ["a non-ephemeral task", { is_ephemeral: false }],
    ["a workflow-bound ephemeral task", { workflow_id: "wf-1" }],
    ["an automation run", { origin: "automation_run" }],
    ["a task with no primary session yet", { primary_session_id: null }],
    ["an event without a workspace", { workspace_id: undefined }],
  ])("ignores %s", (_label, overrides) => {
    expect(quickChatSessionFromTaskEvent(payload(overrides))).toBeNull();
  });
});

describe("syncQuickChatFromTaskEvent", () => {
  it("upserts a quick chat observed on the wire", () => {
    const { store, upsertQuickChatSessionFromEvent } = makeStore();

    syncQuickChatFromTaskEvent(store, payload());

    expect(upsertQuickChatSessionFromEvent).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "session-1", taskId: "task-1" }),
    );
  });

  it("removes the tab when the quick chat is archived", () => {
    const { store, upsertQuickChatSessionFromEvent, removeQuickChatSessionsForTask } = makeStore();

    syncQuickChatFromTaskEvent(store, payload({ archived_at: "2026-07-30T12:00:00Z" }));

    expect(removeQuickChatSessionsForTask).toHaveBeenCalledWith("task-1");
    expect(upsertQuickChatSessionFromEvent).not.toHaveBeenCalled();
  });

  it("does nothing for an ephemeral task that is not a quick chat", () => {
    const { store, upsertQuickChatSessionFromEvent } = makeStore();

    syncQuickChatFromTaskEvent(store, payload({ workflow_id: "wf-1" }));

    expect(upsertQuickChatSessionFromEvent).not.toHaveBeenCalled();
  });
});
