import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import type { ExecutionSnapshot } from "@/lib/types/execution";
import {
  getSessionRestartReviewStore,
  settleSessionRestartReview,
} from "@/lib/session/session-restart-review";
import { useSessionRestart } from "./use-session-restart";

const request = vi.fn();
const toast = vi.fn().mockReturnValue("toast");
let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast, updateToast: vi.fn() }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request }) }));

const snapshot: ExecutionSnapshot = {
  schema_version: 1,
  attempt_id: "execution-attempt",
  session_id: "session",
  task_id: "task",
  execution_id: "execution",
  generation: 1,
  sequence: 1,
  executor_id: "local",
  executor_type: "local_pc",
  agent_profile_id: "profile",
  transport: "acp",
  wrapper: null,
  agent: { executable: "agent", omitted_arguments: 0, references: [] },
  captured_at: "2026-10-26T20:00:00Z",
  sandbox: {
    attempt_id: "sandbox-attempt",
    generation: 1,
    installation_id: "provider",
    provider_key: "sandbox",
    provider_version: "1",
    package_digest: "package",
    runtime_version: "1",
    runtime_digest: "runtime",
    revision_id: "policy",
    revision_number: 1,
    policy_name: "Captured",
    policy_digest: "captured",
    selection_source: "executor",
    ceiling_revision_id: "ceiling",
    ceiling_digest: "ceiling",
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  request.mockResolvedValue(undefined);
  store = createAppStore();
  store.getState().setAuthState({
    mode: "enabled",
    authenticated: true,
    user: {
      id: "operator",
      email: "",
      display_name: "Operator",
      role: "admin",
      status: "active",
    },
  });
  store.getState().upsertTaskSessionFromEvent("task", {
    id: sessionId("session"),
    task_id: taskId("task"),
    state: "RUNNING",
    execution_snapshot: snapshot,
    started_at: snapshot.captured_at,
    updated_at: snapshot.captured_at,
  });
});

describe("sandbox restart review", () => {
  it.each(["captured", "current", "cancel"] as const)(
    "waits before stopping and applies %s explicitly",
    async (mode) => {
      const { result } = renderHook(() => useSessionRestart("session", "task"));
      let pending!: Promise<boolean>;
      act(() => {
        pending = result.current.restart();
      });
      expect(request).not.toHaveBeenCalled();
      expect(result.current.isRestartPending()).toBe(true);
      const review = getSessionRestartReviewStore(store).getState()!;
      expect(review).toMatchObject({
        attemptId: "sandbox-attempt",
        sessionId: "session",
        taskId: "task",
      });
      const choice =
        mode === "current" ? { mode, selectionDigest: "reviewed" } : { mode: "captured" as const };
      await act(async () => {
        settleSessionRestartReview(store, review, mode === "cancel" ? null : choice);
        expect(await pending).toBe(mode !== "cancel");
      });
      expect(result.current.isRestartPending()).toBe(false);
      if (mode === "cancel") {
        expect(request).not.toHaveBeenCalled();
        return;
      }
      expect(request).toHaveBeenNthCalledWith(1, "session.stop", { session_id: "session" }, 15000);
      expect(request).toHaveBeenNthCalledWith(
        2,
        "session.launch",
        {
          task_id: "task",
          intent: "resume",
          session_id: "session",
          ...(mode === "current"
            ? {
                sandbox_restart_review: {
                  attempt_id: "sandbox-attempt",
                  selection_digest: "reviewed",
                },
              }
            : {}),
        },
        30000,
      );
    },
  );

  it.each(["sign-out", "attempt-replaced"])("refuses a review after %s", async (change) => {
    const { result } = renderHook(() => useSessionRestart("session", "task"));
    let pending!: Promise<boolean>;
    act(() => {
      pending = result.current.restart();
    });
    const review = getSessionRestartReviewStore(store).getState()!;
    expect(review).not.toBeNull();
    if (change === "sign-out") store.getState().clearAuthenticated();
    else
      store.getState().upsertTaskSessionFromEvent("task", {
        ...store.getState().taskSessions.items.session,
        id: sessionId("session"),
        task_id: taskId("task"),
        execution_snapshot: {
          ...snapshot,
          sequence: 2,
          sandbox: { ...snapshot.sandbox!, attempt_id: "replacement" },
        },
      });
    await act(async () => {
      settleSessionRestartReview(store, review, { mode: "current", selectionDigest: "reviewed" });
      expect(await pending).toBe(false);
    });
    expect(request).not.toHaveBeenCalled();
    expect(result.current.isRestartPending()).toBe(false);
  });
});
