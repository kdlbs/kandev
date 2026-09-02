import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerLspHandlers } from "./lsp";

describe("task.lsp.changed", () => {
  it("forwards the full task/language snapshot to revision-aware state", () => {
    const mergeTaskLspLanguage = vi.fn();
    const store = {
      getState: () => ({ mergeTaskLspLanguage }) as unknown as AppState,
    } as StoreApi<AppState>;
    const payload = { task_id: "task-1", language: "kotlin", revision: 7 };

    registerLspHandlers(store)["task.lsp.changed"]?.({
      id: "event-1",
      type: "notification",
      action: "task.lsp.changed",
      payload,
    } as never);

    expect(mergeTaskLspLanguage).toHaveBeenCalledWith(payload);
  });
});
