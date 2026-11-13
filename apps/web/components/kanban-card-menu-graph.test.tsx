import { describe, expect, it, vi } from "vitest";
import { buildKanbanCardMenuEntries } from "./kanban-card-menu-builder";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

function flatten(entries: KanbanCardMenuEntry[]): KanbanCardMenuEntry[] {
  return entries.flatMap((e) => (e.kind === "submenu" ? [e, ...flatten(e.children)] : [e]));
}

const base = { workflows: [], stepsByWorkflowId: {} } as unknown as Parameters<
  typeof buildKanbanCardMenuEntries
>[0];

describe("Show in graph card entry", () => {
  it("is offered when a handler is supplied and runs it", () => {
    const onShowGraph = vi.fn();
    const entry = flatten(buildKanbanCardMenuEntries({ ...base, onShowGraph })).find(
      (e) => e.kind === "item" && e.key === "show-in-graph",
    );
    expect(entry).toBeDefined();
    if (entry?.kind === "item") entry.onSelect?.();
    expect(onShowGraph).toHaveBeenCalled();
  });

  it("is omitted without a handler", () => {
    const keys = flatten(buildKanbanCardMenuEntries(base)).map((e) => ("key" in e ? e.key : ""));
    expect(keys).not.toContain("show-in-graph");
  });
});
