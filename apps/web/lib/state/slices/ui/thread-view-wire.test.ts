import { describe, expect, it } from "vitest";
import type { ThreadViewApi, ThreadViewDraftApi } from "@/lib/types/http-user-settings";
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
      layout: "grid",
      auto_hide_composer: true,
    };
    const view = fromApiThreadView(api);
    expect(view).toEqual({
      id: api.id,
      name: api.name,
      taskScope: { mode: "selected", taskIds: ["task-a", "task-b"] },
      filters: [{ id: "f1", dimension: "pendingAction", op: "in", value: ["permission"] }],
      sort: { key: "lastActivityAt", direction: "desc" },
      maxColumns: 3,
      layout: "grid",
      autoHideComposer: true,
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

  // @covers AC-UI-THREADS-SAVED-VIEWS-005.1, AC-UI-THREADS-SAVED-VIEWS-005.3
  it.each([
    [{}, "columns", false],
    [{ layout: "grid", auto_hide_composer: true }, "grid", true],
    [{ layout: "unknown", auto_hide_composer: true }, "columns", true],
    [{ layout: {}, auto_hide_composer: true }, "columns", true],
    [{ layout: "grid", auto_hide_composer: "true" }, "grid", false],
  ])("normalizes presentation independently: %j", (fields, layout, autoHideComposer) => {
    const api = {
      id: "view-one",
      name: "One",
      base_view_id: "view-one",
      task_scope: { mode: "selected" as const, task_ids: ["task-sentinel"] },
      filters: [],
      sort: { key: "priority", direction: "desc" },
      max_columns: 7,
      ...fields,
    };
    // A persisted payload may predate the fields or contain an unknown value.
    for (const value of [
      fromApiThreadView(api as ThreadViewApi),
      fromApiThreadDraft(api as ThreadViewDraftApi),
    ]) {
      expect(value).toMatchObject({
        layout,
        autoHideComposer,
        maxColumns: 7,
        taskScope: { mode: "selected", taskIds: ["task-sentinel"] },
      });
    }
    expect(toApiThreadDraft(fromApiThreadDraft(api as ThreadViewDraftApi))).toMatchObject({
      layout,
      auto_hide_composer: autoHideComposer,
    });
  });
});
