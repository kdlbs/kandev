import { describe, expect, it } from "vitest";
import { savedLayoutMatchesLive } from "./comparator";
import { toSerializedDockview } from "./serializer";
import type { LayoutGroup, LayoutNode, LayoutState } from "./types";

function group(...components: string[]): LayoutGroup {
  return {
    panels: components.map((component) => ({ id: component, component, title: component })),
  };
}

function defaultLayout(): LayoutState {
  return {
    columns: [
      { id: "center", groups: [group("chat")] },
      { id: "right", groups: [group("files", "changes"), group("terminal")] },
    ],
  };
}

function saved(layout: LayoutState) {
  return toSerializedDockview(layout, 1200, 800, new Map());
}

function leaf(group: LayoutGroup): LayoutNode {
  return { type: "leaf", group };
}

describe("saved layout structural compatibility", () => {
  it("preserves the fast path across sizes, group IDs, session IDs, and active tabs", () => {
    const live = defaultLayout();
    live.columns[0].width = 700;
    live.columns[0].groups[0] = {
      id: "runtime-chat-group",
      panels: [{ id: "session:new", component: "chat", title: "New agent" }],
    };
    live.columns[1].groups[0].panels.reverse();
    live.columns[1].groups[0].activePanel = "changes";
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(true);
  });

  it("rejects the same panels moved into a third column", () => {
    const live = defaultLayout();
    live.columns[1].groups[0] = group("changes");
    live.columns.unshift({ id: "left-files", groups: [group("files")] });
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(false);
  });

  it("rejects changed group placement with unchanged panel and column counts", () => {
    const live = defaultLayout();
    live.columns[1].groups = [group("files"), group("changes", "terminal")];
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(false);
  });

  it("rejects a changed root split orientation", () => {
    const live = { ...defaultLayout(), rootOrientation: "VERTICAL" as const };
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(false);
    expect(savedLayoutMatchesLive(live, saved(live))).toBe(true);
  });

  it("preserves nested branches instead of comparing flattened groups", () => {
    const groups = [group("files"), group("changes"), group("terminal")];
    const live: LayoutState = {
      columns: [
        {
          id: "nested",
          groups,
          tree: {
            type: "branch",
            children: [leaf(groups[0]), { type: "branch", children: groups.slice(1).map(leaf) }],
          },
        },
      ],
    };
    const flat: LayoutState = { columns: [{ id: "nested", groups }] };
    expect(savedLayoutMatchesLive(live, saved(flat))).toBe(false);
    expect(savedLayoutMatchesLive(live, saved(live))).toBe(true);
  });

  it("compares structural multiplicity and ignores an ephemeral tab in the same group", () => {
    const live = defaultLayout();
    live.columns[0].groups[0].panels.push({
      id: "editor-a",
      component: "file-editor",
      title: "a.ts",
    });
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(true);
    live.columns[1].groups[1].panels.push({
      id: "terminal-extra",
      component: "terminal",
      title: "Extra",
    });
    expect(savedLayoutMatchesLive(live, saved(defaultLayout()))).toBe(false);
  });
});
