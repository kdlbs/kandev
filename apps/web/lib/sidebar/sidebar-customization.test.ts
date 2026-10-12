import { describe, it, expect } from "vitest";
import { defaultSidebarLayout, fromApiSidebarLayout, toApiSidebarLayout } from "./layout-types";
import { toggleNodeVisibility, moveSection } from "./layout-operations";

const INBOX_ID = "needs-you-inbox";
const COORDINATORS_ID = "coordinators";
const COLLISION_FREE_COORDINATORS_ID = "coordinators-1";

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
    expect(layout.nodes.map((node) => node.id)).toEqual([
      "home",
      "inbox",
      INBOX_ID,
      "coordinators",
    ]);
    const hidden = toggleNodeVisibility(layout, INBOX_ID, false);
    const moved = moveSection(hidden, INBOX_ID, 0);
    const api = toApiSidebarLayout(moved);
    expect(api.navigation_height).toBe(90);
    expect(api.navigation_expanded).toBe(true);
    expect(api.nodes[0]).toMatchObject({ id: INBOX_ID, visible: false });
  });

  it("materializes Coordinators before Automations without moving saved nodes", () => {
    const old = {
      version: 1,
      revision: 4,
      nodes: [
        { id: "home", kind: "builtin" as const, destination_id: "home", visible: true },
        {
          id: "automations",
          kind: "builtin" as const,
          destination_id: "automations",
          visible: false,
        },
        { id: "custom", kind: "shortcuts" as const, name: "Pinned", visible: true, shortcuts: [] },
      ],
    };

    const layout = fromApiSidebarLayout(old);

    expect(layout.nodes.map((node) => node.id)).toEqual([
      "home",
      "inbox",
      INBOX_ID,
      "coordinators",
      "automations",
      "custom",
    ]);
    expect(layout.nodes.find((node) => node.id === "automations")?.visible).toBe(false);
    expect(layout.nodes.find((node) => node.id === "custom")?.name).toBe("Pinned");
  });

  it("does not restore a deliberately hidden Inbox", () => {
    const hidden = toggleNodeVisibility(defaultSidebarLayout(), INBOX_ID, false);
    expect(
      fromApiSidebarLayout(toApiSidebarLayout(hidden)).nodes.find((node) => node.id === INBOX_ID)
        ?.visible,
    ).toBe(false);
  });
});

describe("legacy Coordinator node ID collisions", () => {
  it("keeps a shortcut group named coordinators distinct through edit and save", () => {
    const projected = fromApiSidebarLayout({
      version: 1,
      revision: 6,
      nodes: [
        { id: "home", kind: "builtin" as const, destination_id: "home", visible: true },
        {
          id: COORDINATORS_ID,
          kind: "shortcuts" as const,
          visible: true,
          name: "Pinned",
          shortcuts: [{ id: "home-pin", target: { kind: "destination" as const, id: "home" } }],
        },
        {
          id: "automations",
          kind: "builtin" as const,
          destination_id: "automations",
          visible: true,
        },
      ],
    });
    expect(projected.nodes.map((node) => node.id)).toEqual([
      "home",
      "inbox",
      INBOX_ID,
      COORDINATORS_ID,
      COLLISION_FREE_COORDINATORS_ID,
      "automations",
    ]);
    expect(projected.nodes[3]).toMatchObject({
      id: COORDINATORS_ID,
      kind: "shortcuts",
      name: "Pinned",
      shortcuts: [{ id: "home-pin", target: { kind: "destination", id: "home" } }],
    });
    expect(projected.nodes[4]).toMatchObject({
      id: COLLISION_FREE_COORDINATORS_ID,
      kind: "builtin",
      destinationId: COORDINATORS_ID,
    });

    const edited = toggleNodeVisibility(projected, COLLISION_FREE_COORDINATORS_ID, false);
    const saved = fromApiSidebarLayout(toApiSidebarLayout(edited));
    expect(saved.nodes).toHaveLength(projected.nodes.length);
    expect(saved.nodes[3]).toMatchObject({ id: COORDINATORS_ID, name: "Pinned" });
    expect(saved.nodes[4]).toMatchObject({
      id: COLLISION_FREE_COORDINATORS_ID,
      destinationId: COORDINATORS_ID,
      visible: false,
    });
  });
});
