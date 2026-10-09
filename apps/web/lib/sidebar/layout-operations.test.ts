import { describe, expect, it } from "vitest";
import {
  addShortcut,
  createShortcutSection,
  moveShortcut,
  moveSection,
  removeShortcut,
  renameShortcutSection,
  setShortcutSectionNameDraft,
  toggleNodeVisibility,
  validateSidebarLayout,
} from "./layout-operations";
import { defaultSidebarLayout, SIDEBAR_LAYOUT_LIMITS, type SidebarLayout } from "./layout-types";

const GROUP_A = "group-a";
const SHORTCUT_A = "shortcut-a";

function layoutWithSections(): SidebarLayout {
  return {
    ...defaultSidebarLayout(),
    nodes: [
      {
        id: GROUP_A,
        kind: "shortcuts",
        visible: true,
        name: "A",
        shortcuts: [
          { id: SHORTCUT_A, target: { kind: "destination", id: "github" } },
          { id: "shortcut-b", target: { kind: "canvas", id: "canvas-1" } },
        ],
      },
      {
        id: "group-b",
        kind: "shortcuts",
        visible: true,
        name: "B",
        shortcuts: [],
      },
    ],
  };
}

describe("sidebar layout operations", () => {
  it("creates and renames an empty section", () => {
    const created = createShortcutSection(defaultSidebarLayout(), "  Team  ", "group-1");

    expect(created.nodes.at(-1)).toMatchObject({
      id: "group-1",
      kind: "shortcuts",
      name: "Team",
      shortcuts: [],
    });
    expect(renameShortcutSection(created, "group-1", "Personal").nodes.at(-1)?.name).toBe(
      "Personal",
    );
  });

  it("moves a shortcut between sections without changing its instance id", () => {
    const moved = moveShortcut(layoutWithSections(), SHORTCUT_A, GROUP_A, "group-b", 0);

    expect(moved.nodes[0]?.shortcuts).toEqual([
      { id: "shortcut-b", target: { kind: "canvas", id: "canvas-1" } },
    ]);
    expect(moved.nodes[1]?.shortcuts).toEqual([
      { id: SHORTCUT_A, target: { kind: "destination", id: "github" } },
    ]);
  });

  it("rejects a duplicate target within the destination section", () => {
    expect(() =>
      addShortcut(layoutWithSections(), GROUP_A, {
        id: "shortcut-c",
        target: { kind: "canvas", id: "canvas-1" },
      }),
    ).toThrowError("duplicate_target");
  });

  it("keeps section order and visibility changes immutable", () => {
    const layout = layoutWithSections();
    const reordered = moveSection(layout, "group-b", 0);
    const hidden = toggleNodeVisibility(reordered, "group-b", false);

    expect(hidden.nodes.map((node) => node.id)).toEqual(["group-b", "group-a"]);
    expect(hidden.nodes[0]?.visible).toBe(false);
    expect(layout.nodes.map((node) => node.id)).toEqual(["group-a", "group-b"]);
  });

  it("keeps raw input while a section name is being edited", () => {
    const withGroup = createShortcutSection(defaultSidebarLayout(), "Team", "group-1");
    const cleared = setShortcutSectionNameDraft(withGroup, "group-1", "");
    const spaced = setShortcutSectionNameDraft(cleared, "group-1", "Team ");

    expect(cleared.nodes.at(-1)?.name).toBe("");
    expect(spaced.nodes.at(-1)?.name).toBe("Team ");
    expect(() => renameShortcutSection(cleared, "group-1", "")).toThrowError("invalid_group_name");
  });

  it("rejects limit and duplicate failures at the operation boundary", () => {
    const layout = layoutWithSections();
    const duplicate = {
      ...layout,
      nodes: layout.nodes.map((node) => ({
        ...node,
        shortcuts:
          node.id === "group-b"
            ? [{ id: "existing", target: { kind: "destination" as const, id: "github" } }]
            : node.shortcuts,
      })),
    };

    expect(() => moveShortcut(duplicate, SHORTCUT_A, GROUP_A, "group-b", 1)).toThrowError(
      "duplicate_target",
    );

    let groups = defaultSidebarLayout();
    for (let index = 0; index < SIDEBAR_LAYOUT_LIMITS.groups; index += 1) {
      groups = createShortcutSection(groups, `Group ${index}`, `group-${index}`);
    }
    expect(() => createShortcutSection(groups, "One too many", "overflow")).toThrowError(
      "limit_reached",
    );
  });

  it("removes a shortcut and validates the resulting layout", () => {
    const layout = removeShortcut(layoutWithSections(), GROUP_A, SHORTCUT_A);

    expect(validateSidebarLayout(layout)).toEqual({ valid: true, errors: [] });
    expect(layout.nodes[0]?.shortcuts?.map((shortcut) => shortcut.id)).toEqual(["shortcut-b"]);
  });
});
