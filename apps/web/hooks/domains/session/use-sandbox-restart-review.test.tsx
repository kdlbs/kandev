import { StrictMode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import type { ExecutionSnapshot } from "@/lib/types/execution";
import {
  getSessionRestartReviewStore,
  requestSessionRestartReview,
} from "@/lib/session/session-restart-review";
import { useSandboxRestartReview } from "./use-sandbox-restart-review";

const mocks = vi.hoisted(() => ({
  preview: vi.fn(),
  fetchTask: vi.fn(),
  refreshContext: vi.fn(),
  refetch: vi.fn(),
  details: {} as Record<string, unknown>,
}));
let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (select: (s: unknown) => unknown) => select(store.getState()),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: mocks.fetchTask }));
vi.mock("@/lib/api/domains/sandbox-api", () => ({ previewSandboxSelection: mocks.preview }));
vi.mock("./use-execution-policy-details", () => ({
  useExecutionPolicyDetails: () => mocks.details,
}));
vi.mock("./use-session-execution", () => ({
  useSessionExecution: () => ({ session: store.getState().taskSessions.items.session }),
}));
const resolved = { revision: { id: "current", digest: "policy" }, ceiling: { digest: "ceiling" } };

beforeEach(() => {
  vi.clearAllMocks();
  mocks.fetchTask.mockResolvedValue({
    id: "task",
    workspace_id: "workspace",
    workflow_step_id: "step",
  });
  mocks.refreshContext.mockResolvedValue(undefined);
  mocks.refetch.mockResolvedValue(undefined);
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
  store.setState((state) => ({ connection: { ...state.connection, status: "connected" } }));
  store.getState().upsertTaskSessionFromEvent("task", {
    id: sessionId("session"),
    task_id: taskId("task"),
    state: "RUNNING",
    started_at: "2026-10-26T20:00:00Z",
    updated_at: "2026-10-26T20:00:00Z",
    execution_snapshot: {
      task_id: "task",
      session_id: "session",
      sandbox: { attempt_id: "attempt" },
    } as ExecutionSnapshot,
  });
  mocks.details = {
    access: { enabled: true, canRead: true, canEdit: true },
    canPreview: true,
    refreshContext: mocks.refreshContext,
    captured: {},
    context: { task_id: "task", executor_id: "local" },
    preview: { data: { resolved, selection_digest: "reviewed" }, refetch: mocks.refetch },
  };
  mocks.preview.mockResolvedValue({ resolved, selection_digest: "reviewed" });
});
function begin() {
  const pending = requestSessionRestartReview(store, {
    taskId: "task",
    sessionId: "session",
    attemptId: "attempt",
    identity: JSON.stringify(store.getState().auth),
  });
  const request = getSessionRestartReviewStore(store).getState()!;
  const hook = renderHook(() => useSandboxRestartReview(request));
  return { pending, ...hook };
}
it("rechecks the exact displayed selection and confirms once", async () => {
  let finish!: (value: unknown) => void;
  mocks.preview.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result, pending } = begin();
  let first!: Promise<void>;
  act(() => {
    first = result.current.confirmCurrent();
    void result.current.confirmCurrent();
  });
  await waitFor(() => expect(mocks.preview).toHaveBeenCalledOnce());
  expect(getSessionRestartReviewStore(store).getState()).not.toBeNull();
  await act(async () => {
    finish({ resolved, selection_digest: "reviewed" });
    await first;
  });
  expect(await pending).toEqual({ mode: "current", selectionDigest: "reviewed" });
});
it("keeps a changed policy open for another review without confirming", async () => {
  mocks.preview.mockResolvedValue({ resolved, selection_digest: "changed" });
  const { result, pending } = begin();
  await act(async () => result.current.confirmCurrent());
  expect(getSessionRestartReviewStore(store).getState()).not.toBeNull();
  expect(result.current.error).toBe("stale");
  expect(mocks.refetch).toHaveBeenCalledOnce();
  act(() => result.current.cancel());
  expect(await pending).toBeNull();
});
it("cancels on sign-out and ignores a late confirmation", async () => {
  let finish!: (value: unknown) => void;
  mocks.preview.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result, pending, rerender } = begin();
  let first!: Promise<void>;
  act(() => {
    first = result.current.confirmCurrent();
  });
  await waitFor(() => expect(mocks.preview).toHaveBeenCalledOnce());
  act(() => {
    store.getState().clearAuthenticated();
    rerender();
  });
  await waitFor(() => expect(getSessionRestartReviewStore(store).getState()).toBeNull());
  await act(async () => {
    finish({ resolved, selection_digest: "reviewed" });
    await first;
  });
  expect(await pending).toBeNull();
});
it("settles cancellation when the stable host is disposed", async () => {
  const { pending, unmount } = begin();
  unmount();
  expect(await pending).toBeNull();
});

it("preserves a review through StrictMode effect replay", async () => {
  const pending = requestSessionRestartReview(store, {
    taskId: "task",
    sessionId: "session",
    attemptId: "attempt",
    identity: JSON.stringify(store.getState().auth),
  });
  const request = getSessionRestartReviewStore(store).getState()!;
  const { unmount } = renderHook(() => useSandboxRestartReview(request), { wrapper: StrictMode });
  await act(async () => {
    await Promise.resolve();
  });
  expect(getSessionRestartReviewStore(store).getState()).toBe(request);
  unmount();
  expect(await pending).toBeNull();
});

it("keeps confirmation busy until a changed selection has been refreshed", async () => {
  let finishRefresh!: () => void;
  mocks.preview.mockResolvedValue({ resolved, selection_digest: "changed" });
  mocks.refetch.mockImplementation(
    () =>
      new Promise<void>((resolve) => {
        finishRefresh = resolve;
      }),
  );
  const { result, pending } = begin();
  let confirmation!: Promise<void>;
  act(() => {
    confirmation = result.current.confirmCurrent();
  });
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledOnce());
  expect(result.current.busy).toBe(true);
  act(() => {
    void result.current.confirmCurrent();
  });
  expect(mocks.preview).toHaveBeenCalledOnce();
  await act(async () => {
    finishRefresh();
    await confirmation;
  });
  act(() => result.current.cancel());
  expect(await pending).toBeNull();
});
