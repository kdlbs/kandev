import { describe, expect, it } from "vitest";
import {
  fromApiThreadDraft,
  fromApiThreadView,
  toApiThreadDraft,
  toApiThreadView,
} from "./thread-view-wire";

describe("thread view wire mapping", () => {
  it("maps the persisted snake-case view and draft fields", () => {
    const api = {
      id: "view-needs-attention",
      name: "Needs attention",
      task_scope: { mode: "selected" as const, task_ids: ["task-a", "task-b"] },
      filters: [{ id: "f1", dimension: "pendingAction", op: "in", value: ["permission"] }],
      sort: { key: "lastActivityAt", direction: "desc" },
      max_columns: 3,
    };
    const view = fromApiThreadView(api);
    expect(view).toEqual({
      id: api.id,
      name: api.name,
      taskScope: { mode: "selected", taskIds: ["task-a", "task-b"] },
      filters: [{ id: "f1", dimension: "pendingAction", op: "in", value: ["permission"] }],
      sort: { key: "lastActivityAt", direction: "desc" },
      maxColumns: 3,
    });
    expect(toApiThreadView(view)).toEqual(api);
  });

  it("normalizes malformed stored values without changing the wire contract", () => {
    const view = fromApiThreadView({
      id: "view-all-threads",
      name: "All threads",
      task_scope: { mode: "all", task_ids: ["stale"] },
      filters: [
        { id: "bad", dimension: "removed", op: "is", value: "x" },
        { id: "good", dimension: "titleMatch", op: "matches", value: "api" },
      ],
      sort: { key: "removed", direction: "sideways" },
      max_columns: 999,
    });
    expect(view.taskScope).toEqual({ mode: "all", taskIds: [] });
    expect(view.filters).toEqual([
      { id: "good", dimension: "titleMatch", op: "matches", value: "api" },
    ]);
    expect(view.sort).toEqual({ key: "attention", direction: "asc" });
    expect(view.maxColumns).toBeNull();
  });

  it("preserves an explicit null column limit in drafts", () => {
    const draft = fromApiThreadDraft({
      base_view_id: "view-all-threads",
      task_scope: { mode: "all", task_ids: [] },
      filters: [],
      sort: { key: "attention", direction: "asc" },
      max_columns: null,
    });
    expect(draft.maxColumns).toBeNull();
    expect(toApiThreadDraft(draft).max_columns).toBeNull();
  });
});
