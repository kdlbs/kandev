import { describe, it, expect } from "vitest";
import type { DockviewApi } from "dockview-react";
import { resolveGroupIds } from "./applier";
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
