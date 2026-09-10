import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import { ApiError } from "@/lib/api/client";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { createAppStore } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { createUISlice } from "./ui-slice";
import type { ThreadView, ThreadViewDraft } from "./thread-view-types";
import type { UISlice } from "./types";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));

type UIStore = UseBoundStore<StoreApi<UISlice>>;
const WRITE_ERROR = "write failed";

function makeStore(): UIStore {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

function makeView(id: string, name = id): ThreadView {
  return {
    id,
    name,
    taskScope: { mode: "all", taskIds: [] },
    filters: [],
    sort: { key: "attention", direction: "asc" },
    maxColumns: null,
  };
}

function seed(store: UIStore, views: ThreadView[], draft: ThreadViewDraft | null = null): void {
  store.setState((state) => ({
    ...state,
    threadViews: {
      ...state.threadViews,
      views,
      activeViewId: views[0].id,
      draft,
    },
  }));
}

beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockResolvedValue({
    settings: {},
  } as Awaited<ReturnType<typeof updateUserSettings>>);
});

describe("Threads saved-view actions", () => {
  it("keeps saved view state independent and persists a selected view", () => {
    const store = makeStore();
    const views = [makeView("one"), makeView("two")];
    seed(store, views);

    store.getState().setThreadActiveView("two");

    expect(store.getState().threadViews.activeViewId).toBe("two");
    expect(store.getState().sidebarViews.activeViewId).toBe("view-all-tasks");
    expect(updateUserSettings).toHaveBeenCalledWith({
      thread_views: [
        expect.objectContaining({ id: "one" }),
        expect.objectContaining({ id: "two" }),
      ],
      thread_active_view_id: "two",
      thread_view_draft: null,
    });
  });

  it("creates a draft, overwrites the active view, and clears the draft", () => {
    const store = makeStore();
    seed(store, [makeView("one")]);

    store.getState().updateThreadViewDraft({
      taskScope: { mode: "selected", taskIds: ["task-a"] },
      maxColumns: 3,
    });
    expect(store.getState().threadViews.draft).toEqual(
      expect.objectContaining({
        baseViewId: "one",
        taskScope: { mode: "selected", taskIds: ["task-a"] },
        maxColumns: 3,
      }),
    );

    store.getState().saveThreadViewDraftOverwrite();
    expect(store.getState().threadViews.draft).toBeNull();
    expect(store.getState().threadViews.views[0]).toEqual(
      expect.objectContaining({
        taskScope: { mode: "selected", taskIds: ["task-a"] },
        maxColumns: 3,
      }),
    );
  });
});

describe("Threads saved-view write recovery", () => {
  it("rolls back a failed optimistic write and exposes the error", async () => {
    const store = makeStore();
    const original = makeView("one", "One");
    seed(store, [original]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(WRITE_ERROR, 500, {}));

    store.getState().renameThreadView("one", "Renamed");

    await waitFor(() => expect(store.getState().threadViews.syncError).toBe(WRITE_ERROR));
    expect(store.getState().threadViews.views).toEqual([original]);
  });

  it("reconciles a newer backend view after a pending write fails", async () => {
    const store = createAppStore();
    store.setState((state) => ({
      ...state,
      threadViews: {
        ...state.threadViews,
        views: [makeView("local", "Local")],
        activeViewId: "local",
      },
    }));
    let rejectUpdate!: (error: unknown) => void;
    vi.mocked(updateUserSettings).mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectUpdate = reject;
      }) as ReturnType<typeof updateUserSettings>,
    );

    store.getState().renameThreadView("local", "Optimistic");
    registerUsersHandlers(store)["user.settings.updated"]?.({
      type: "notification",
      action: "user.settings.updated",
      payload: {
        user_id: "user",
        workspace_id: "workspace",
        repository_ids: [],
        revision: 1,
        thread_views: [
          {
            id: "server",
            name: "Server",
            task_scope: { mode: "all", task_ids: [] },
            filters: [],
            sort: { key: "attention", direction: "asc" },
            max_columns: null,
          },
        ],
        thread_active_view_id: "server",
        thread_view_draft: null,
      },
    } satisfies BackendMessageMap["user.settings.updated"]);

    rejectUpdate(new ApiError(WRITE_ERROR, 500, {}));

    await waitFor(() => expect(store.getState().threadViews.syncPending).toBe(false));
    expect(store.getState().threadViews.views).toEqual([makeView("server", "Server")]);
    expect(store.getState().threadViews.syncError).toBe(WRITE_ERROR);
  });

  it("retries the failed optimistic write and clears the stale error", async () => {
    const store = makeStore();
    const original = makeView("one", "One");
    seed(store, [original]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(WRITE_ERROR, 500, {}));

    store.getState().renameThreadView("one", "Renamed");
    await waitFor(() => expect(store.getState().threadViews.syncError).toBe(WRITE_ERROR));

    store.getState().retryThreadViewSync();

    await waitFor(() => expect(store.getState().threadViews.syncPending).toBe(false));
    expect(store.getState().threadViews.syncError).toBeNull();
    expect(store.getState().threadViews.views[0]?.name).toBe("Renamed");
    expect(updateUserSettings).toHaveBeenCalledTimes(2);
  });
});

describe("Threads saved-view sort controls", () => {
  it("increments the order reset generation without persisting the view", () => {
    const store = makeStore();
    const before = store.getState().threadViews.orderResetGeneration;
    store.getState().reapplyThreadViewSort();
    expect(store.getState().threadViews.orderResetGeneration).toBe(before + 1);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});
