import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";

const snapshot = (revision: number) => ({
  task_id: "task-1",
  revision,
  items: [
    {
      id: `feedback-${revision}`,
      task_id: "task-1",
      kind: "text" as const,
      comment: `comment ${revision}`,
      source_kind: "browser" as const,
      source_label: "Local app",
      page_route: "/products",
      page_title: "Products",
      selected_text: "Choose a plan",
      text_anchor: { start: { path: [0], offset: 0 } },
      version: 1,
      created_at: "2026-09-15T00:00:00Z",
      updated_at: "2026-09-15T00:00:00Z",
    },
  ],
});

describe("task preview feedback state", () => {
  it("provides a task-keyed complete-snapshot action", () => {
    const state = createAppStore().getState() as unknown as Record<string, unknown>;

    expect(state.setTaskPreviewFeedback).toBeTypeOf("function");
  });

  it("keeps the equal or newest snapshot and records load state", () => {
    const store = createAppStore();
    const state = store.getState() as unknown as {
      previewFeedback: {
        byTaskId: Record<string, ReturnType<typeof snapshot> | undefined>;
        loadedByTaskId: Record<string, boolean | undefined>;
        loadingByTaskId: Record<string, boolean | undefined>;
      };
      setTaskPreviewFeedback: (taskId: string, value: ReturnType<typeof snapshot>) => void;
    };

    state.setTaskPreviewFeedback("task-1", snapshot(2));
    state.setTaskPreviewFeedback("task-1", snapshot(1));

    const current = store.getState() as unknown as typeof state;
    expect(current.previewFeedback.byTaskId["task-1"]).toEqual(snapshot(2));
    expect(current.previewFeedback.loadedByTaskId["task-1"]).toBe(true);
    expect(current.previewFeedback.loadingByTaskId["task-1"]).toBe(false);
  });
});
