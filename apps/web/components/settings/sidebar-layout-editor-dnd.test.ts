import type { DragEndEvent } from "@dnd-kit/core";
import { describe, expect, it } from "vitest";
import {
  defaultSidebarLayout,
  SIDEBAR_LAYOUT_LIMITS,
  type SidebarLayout,
} from "@/lib/sidebar/layout-types";
import {
  handleSidebarLayoutDragEnd,
  type SidebarLayoutDragData,
} from "./sidebar-layout-editor-dnd";

function layoutWithGroups(): SidebarLayout {
  return {
    ...defaultSidebarLayout(),
    nodes: [
      {
        id: "group-a",
        kind: "shortcuts",
        visible: true,
        name: "A",
        shortcuts: [{ id: "chat", target: { kind: "host_action", id: "quick_chat" } }],
      },
      { id: "group-b", kind: "shortcuts", visible: true, name: "B", shortcuts: [] },
    ],
  };
}

function dragEvent(active: SidebarLayoutDragData, over: SidebarLayoutDragData): DragEndEvent {
  return {
    active: { data: { current: active } },
    over: { data: { current: over } },
  } as unknown as DragEndEvent;
}

function expectRejectedDrop(
  layout: SidebarLayout,
  active: SidebarLayoutDragData,
  over: SidebarLayoutDragData,
) {
  let failed = false;
  handleSidebarLayoutDragEnd(dragEvent(active, over), layout, (operation) => {
    try {
      operation(layout);
    } catch {
      failed = true;
    }
    return false;
  });
  expect(failed).toBe(true);
}

describe("sidebar layout drag and drop", () => {
  it("reorders layout groups through the drag handler", () => {
    const layout = layoutWithGroups();
    let next = layout;

    handleSidebarLayoutDragEnd(
      dragEvent({ type: "node", nodeId: "group-b" }, { type: "node", nodeId: "group-a" }),
      layout,
      (operation) => {
        next = operation(next);
        return true;
      },
    );

    expect(next.nodes.map((node) => node.id)).toEqual(["group-b", "group-a"]);
  });

  it("moves a shortcut across groups through the drag handler", () => {
    const layout = layoutWithGroups();
    let next = layout;

    handleSidebarLayoutDragEnd(
      dragEvent(
        { type: "shortcut", nodeId: "group-a", shortcutId: "chat" },
        { type: "group", nodeId: "group-b" },
      ),
      layout,
      (operation) => {
        next = operation(next);
        return true;
      },
    );

    expect(next.nodes.find((node) => node.id === "group-a")?.shortcuts).toEqual([]);
    expect(next.nodes.find((node) => node.id === "group-b")?.shortcuts).toEqual([
      { id: "chat", target: { kind: "host_action", id: "quick_chat" } },
    ]);
  });

  it("lets the editor operation boundary reject a duplicate cross-group drop", () => {
    const layout = layoutWithGroups();
    const duplicate = {
      ...layout,
      nodes: layout.nodes.map((node) =>
        node.id === "group-b"
          ? {
              ...node,
              shortcuts: [
                { id: "existing", target: { kind: "host_action" as const, id: "quick_chat" } },
              ],
            }
          : node,
      ),
    };
    expectRejectedDrop(
      duplicate,
      { type: "shortcut", nodeId: "group-a", shortcutId: "chat" },
      { type: "group", nodeId: "group-b" },
    );
  });

  it("lets the editor operation boundary reject a full destination", () => {
    const layout = layoutWithGroups();
    const fullDestination = {
      ...layout,
      nodes: layout.nodes.map((node) =>
        node.id === "group-b"
          ? {
              ...node,
              shortcuts: Array.from(
                { length: SIDEBAR_LAYOUT_LIMITS.shortcutsPerGroup },
                (_, index) => ({
                  id: `existing-${index}`,
                  target: { kind: "host_action" as const, id: `action-${index}` },
                }),
              ),
            }
          : node,
      ),
    };
    expectRejectedDrop(
      fullDestination,
      { type: "shortcut", nodeId: "group-a", shortcutId: "chat" },
      { type: "group", nodeId: "group-b" },
    );
  });
});
