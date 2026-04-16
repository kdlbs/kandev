import { describe, it, expect } from "vitest";
import type { DockviewApi } from "dockview-react";
import { fallbackGroupPosition } from "./dockview-layout-builders";
import { SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP } from "./layout-manager";

type MockGroup = { id: string };
type MockPanel = { id: string; group: { id: string } };

function makeApi(groupIds: string[], panels: MockPanel[] = []): DockviewApi {
  const groups: MockGroup[] = groupIds.map((id) => ({ id }));
  return {
    groups,
    panels,
    getPanel: (id: string) => panels.find((p) => p.id === id) ?? undefined,
  } as unknown as DockviewApi;
}

describe("fallbackGroupPosition", () => {
  it("returns the center group when it exists", () => {
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, "group-other"]);

    expect(fallbackGroupPosition(api)).toEqual({ referenceGroup: CENTER_GROUP });
  });

  it("returns the chat panel's group even when right groups iterate first", () => {
    // Drag-to-split can replace the well-known center group ID with a generated one.
    // Right groups appear before the chat group in iteration order; the fallback
    // must still prefer the chat group over right-column groups.
    const chatGroupId = "group-3";
    const api = makeApi(
      [SIDEBAR_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP, chatGroupId],
      [{ id: "chat", group: { id: chatGroupId } }],
    );

    expect(fallbackGroupPosition(api)).toEqual({ referenceGroup: chatGroupId });
  });

  it("returns the session:* panel's group when no chat panel exists", () => {
    // Active session: chat is replaced with session:<id>. CENTER_GROUP id was lost.
    // Right groups iterate first; fallback must still prefer the session group.
    const sessionGroupId = "group-7";
    const api = makeApi(
      [SIDEBAR_GROUP, RIGHT_TOP_GROUP, sessionGroupId],
      [{ id: "session:abc", group: { id: sessionGroupId } }],
    );

    expect(fallbackGroupPosition(api)).toEqual({ referenceGroup: sessionGroupId });
  });

  it("does not return a right-column group when no center-like group exists", () => {
    // Right-column groups (Changes/Files/Terminal) are tool columns — placing
    // a diff or PR panel there is the same UX bug as placing it in the sidebar.
    // With only sidebar+right groups present and no chat/session panels, the
    // fallback must drop the position so dockview doesn't pick a right group.
    const api = makeApi([SIDEBAR_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]);

    expect(fallbackGroupPosition(api)).toBeUndefined();
  });

  it("returns undefined when only the sidebar group exists", () => {
    // Must NOT return the sidebar group — panels added to the locked sidebar
    // would leak there, which is the bug we're fixing.
    const api = makeApi([SIDEBAR_GROUP]);

    expect(fallbackGroupPosition(api)).toBeUndefined();
  });

  it("returns undefined when no groups exist", () => {
    const api = makeApi([]);

    expect(fallbackGroupPosition(api)).toBeUndefined();
  });
});
