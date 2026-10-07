import { describe, it, expect } from "vitest";
import { defaultSidebarLayout, fromApiSidebarLayout, toApiSidebarLayout } from "./layout-types";
import { toggleNodeVisibility, moveSection } from "./layout-operations";

const INBOX_ID = "needs-you-inbox";

describe("saved sidebar customization", () => {
  it("materializes legacy Inbox entries without losing order or saved geometry", () => {
    const old = {
      version: 1,
      revision: 7,
      nodes: [{ id: "home", kind: "builtin" as const, destination_id: "home", visible: true }],
      navigation_height: 90,
      navigation_expanded: true,
    };
    const layout = fromApiSidebarLayout(old);
    expect(layout.nodes.map((node) => node.id)).toEqual(["home", "inbox", INBOX_ID]);
    const hidden = toggleNodeVisibility(layout, INBOX_ID, false);
    const moved = moveSection(hidden, INBOX_ID, 0);
    const api = toApiSidebarLayout(moved);
    expect(api.navigation_height).toBe(90);
    expect(api.navigation_expanded).toBe(true);
    expect(api.nodes[0]).toMatchObject({ id: INBOX_ID, visible: false });
  });
  it("does not restore a deliberately hidden Inbox", () => {
    const hidden = toggleNodeVisibility(defaultSidebarLayout(), INBOX_ID, false);
    expect(
      fromApiSidebarLayout(toApiSidebarLayout(hidden)).nodes.find((node) => node.id === INBOX_ID)
        ?.visible,
    ).toBe(false);
  });
});
