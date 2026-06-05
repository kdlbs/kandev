import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";
import { applyLayout, resolveGroupIds } from "./applier";
import { getPinnedTarget, clearAllPinnedTargets } from "./pinned-targets";
import type { LayoutState } from "./types";
import { SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP } from "./constants";

type MockGroup = { id: string };
type MockPanel = { id: string; group: { id: string } };

function makeApi(groups: MockGroup[], panels: MockPanel[] = []): DockviewApi {
  return {
    groups,
    panels,
    getPanel: (id: string) => panels.find((p) => p.id === id) ?? undefined,
  } as unknown as DockviewApi;
}

describe("resolveGroupIds", () => {
  it("returns well-known IDs when all groups exist", () => {
    const api = makeApi([
      { id: SIDEBAR_GROUP },
      { id: CENTER_GROUP },
      { id: RIGHT_TOP_GROUP },
      { id: RIGHT_BOTTOM_GROUP },
    ]);

    const ids = resolveGroupIds(api);

    expect(ids.sidebarGroupId).toBe(SIDEBAR_GROUP);
    expect(ids.centerGroupId).toBe(CENTER_GROUP);
    expect(ids.rightTopGroupId).toBe(RIGHT_TOP_GROUP);
    expect(ids.rightBottomGroupId).toBe(RIGHT_BOTTOM_GROUP);
  });

  it("falls back to chat panel's group when CENTER_GROUP id missing", () => {
    // Simulates post-drag state where center group has a dockview-generated ID
    const chatGroupId = "group-5";
    const api = makeApi(
      [{ id: SIDEBAR_GROUP }, { id: chatGroupId }],
      [{ id: "chat", group: { id: chatGroupId } }],
    );

    const ids = resolveGroupIds(api);

    expect(ids.centerGroupId).toBe(chatGroupId);
  });

  it("falls back to session:* panel's group when no chat panel exists", () => {
    // Active session: chat panel was removed, replaced with session:<id>
    // CENTER_GROUP id was lost (e.g. drag-to-split). This is the bug scenario.
    const sessionGroupId = "group-7";
    const api = makeApi(
      [{ id: SIDEBAR_GROUP }, { id: sessionGroupId }],
      [{ id: "session:abc123", group: { id: sessionGroupId } }],
    );

    const ids = resolveGroupIds(api);

    expect(ids.centerGroupId).toBe(sessionGroupId);
  });

  it("returns the CENTER_GROUP constant as last-resort when nothing matches", () => {
    // Last-resort fallback: returns the well-known constant even when no live
    // group carries that ID. The caller (focusOrAddPanel) detects the stale ID
    // and applies its own fallback via fallbackGroupPosition.
    const api = makeApi([{ id: SIDEBAR_GROUP }], []);

    const ids = resolveGroupIds(api);

    expect(ids.centerGroupId).toBe(CENTER_GROUP);
  });
});

describe("applyLayout — pinned target capture with no override", () => {
  beforeEach(() => {
    clearAllPinnedTargets();
  });

  it("targets the computed default, not a transient post-fromJSON live size", () => {
    // Regression: with no pinnedWidths override (e.g. toggling plan mode off,
    // where the right width was dropped), applyLayout must pin each column to
    // its DEFAULT width — not whatever transient (narrower) size the splitview
    // reports right after fromJSON. Reading the transient left the column stuck
    // too narrow and persisted it.
    const resizeView = vi.fn();
    const splitview = {
      length: 3,
      // Transient: ~0.7x the intended widths, as seen right after fromJSON.
      getViewSize: (idx: number) => [245, 1097, 258][idx],
      resizeView,
    };
    const setConstraints = vi.fn();
    const sidebarGroup = { locked: undefined, header: { hidden: true }, api: { setConstraints } };
    const rightGroup = { id: RIGHT_TOP_GROUP, api: { setConstraints } };
    const api = {
      width: 1600,
      height: 800,
      fromJSON: vi.fn(),
      groups: [{ id: SIDEBAR_GROUP }, { id: CENTER_GROUP }, rightGroup],
      getPanel: (id: string) => {
        if (id === "sidebar") return { group: sidebarGroup };
        if (id === "files") return { group: rightGroup };
        return null;
      },
      component: { gridview: { root: { splitview } } },
    } as unknown as DockviewApi;

    const state: LayoutState = {
      columns: [
        {
          id: "sidebar",
          pinned: true,
          groups: [
            {
              id: SIDEBAR_GROUP,
              panels: [{ id: "sidebar", component: "sidebar", title: "Sidebar" }],
            },
          ],
        },
        {
          id: "center",
          groups: [
            { id: CENTER_GROUP, panels: [{ id: "chat", component: "chat", title: "Agent" }] },
          ],
        },
        {
          id: "right",
          pinned: true,
          groups: [
            { id: RIGHT_TOP_GROUP, panels: [{ id: "files", component: "files", title: "Files" }] },
          ],
        },
      ],
    };

    // Empty overrides → each pinned column should fall back to its default.
    applyLayout(api, state, new Map(), 1600, 800);

    // Defaults at totalWidth 1600: ratio 0.25*1600 = 400 → sidebar clamps to
    // 350, right stays 400. NOT the transient 245 / 258.
    expect(getPinnedTarget("sidebar")).toBe(350);
    expect(getPinnedTarget("right")).toBe(400);
    expect(resizeView).toHaveBeenCalledWith(0, 350);
    expect(resizeView).toHaveBeenCalledWith(2, 400);
  });
});
