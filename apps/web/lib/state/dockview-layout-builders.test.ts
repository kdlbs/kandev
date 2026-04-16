import { describe, it, expect } from "vitest";
import type { DockviewApi } from "dockview-react";
import { fallbackGroupPosition } from "./dockview-layout-builders";
import { SIDEBAR_GROUP, CENTER_GROUP } from "./layout-manager";

function makeApi(groupIds: string[]): DockviewApi {
  return {
    groups: groupIds.map((id) => ({ id })),
  } as unknown as DockviewApi;
}

describe("fallbackGroupPosition", () => {
  it("returns the center group when it exists", () => {
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, "group-other"]);

    expect(fallbackGroupPosition(api)).toEqual({ referenceGroup: CENTER_GROUP });
  });

  it("returns a non-sidebar group when center group is missing", () => {
    // Drag-to-split can replace the well-known center group ID with a generated one.
    const api = makeApi([SIDEBAR_GROUP, "group-3"]);

    expect(fallbackGroupPosition(api)).toEqual({ referenceGroup: "group-3" });
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
