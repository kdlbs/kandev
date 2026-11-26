import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { useSaveTaskFlag } from "./use-save-task-flag";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("rejects a response that does not confirm the flag was saved", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ id: "task-2" })));
  const hook = renderHook(useSaveTaskFlag, { wrapper: StateProvider });

  await expect(hook.result.current("task-2", "Check the build")).rejects.toThrow();
});

it("applies the confirmed note to the invoked task while preserving the active task", async () => {
  const note = "Check the build";
  const fetch = vi
    .fn()
    .mockResolvedValue(
      Response.json({ id: "task-2", flag_note: note, updated_at: "2026-11-25T15:00:01Z" }),
    );
  vi.stubGlobal("fetch", fetch);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), save: useSaveTaskFlag() }), {
    wrapper: StateProvider,
  });
  const { store, save } = result.current;
  const tasks = ["task-1", "task-2"].map((id) =>
    toKanbanTask({ id, workflow_id: "workflow", flag_note: "" }),
  );
  store.setState((state) => ({
    kanban: { ...state.kanban, workflowId: "workflow", tasks },
    tasks: { ...state.tasks, activeTaskId: "task-1" },
  }));

  await save("task-2", note);

  expect(fetch).toHaveBeenCalledWith(
    expect.stringMatching(/\/api\/v1\/tasks\/task-2$/),
    expect.objectContaining({ method: "PATCH", body: JSON.stringify({ flag_note: note }) }),
  );
  expect(store.getState().tasks.activeTaskId).toBe("task-1");
  expect(store.getState().taskOverview.byId["task-1"].flagNote).toBe("");
  expect(store.getState().taskOverview.byId["task-2"].flagNote).toBe(note);
});
