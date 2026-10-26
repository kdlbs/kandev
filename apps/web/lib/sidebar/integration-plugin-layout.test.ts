import { describe, expect, it } from "vitest";
import { IconPlug } from "@tabler/icons-react";
import {
  integrationPluginDestinations,
  integrationPluginLayout,
  sidebarEditorNodes,
  sidebarNodeSiblings,
} from "./integration-plugin-layout";
import { toggleNodeVisibility } from "./layout-operations";
import { materializeSidebarPluginNodes, projectSidebarLayout } from "./layout-projection";
import { defaultSidebarLayout, type SidebarLayoutNode } from "./layout-types";

const plugin = (id: string, section = "integrations", visible = true): SidebarLayoutNode => ({
  id,
  kind: "plugin",
  destinationId: id,
  visible,
  pluginSection: section as "integrations",
});
const parent: SidebarLayoutNode = {
  id: "integrations",
  kind: "builtin",
  destinationId: "integrations",
  visible: true,
};

describe("integration plugin hierarchy", () => {
  // @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.10, AC-UI-SIDEBAR-CUSTOMIZATION-001.12
  it("groups integration links without changing main/footer nodes or custom references", () => {
    const custom: SidebarLayoutNode = {
      id: "pin",
      kind: "shortcuts",
      visible: true,
      name: "Pinned",
      shortcuts: [{ id: "ref", target: { kind: "destination", id: "a" } }],
    };
    const nodes = [
      plugin("b"),
      plugin("main", "plugins"),
      parent,
      plugin("hidden", "integrations", false),
      custom,
      plugin("a"),
      plugin("footer", "insights"),
    ];
    const snapshot = structuredClone(nodes);
    const grouped = integrationPluginLayout(nodes);
    expect(grouped.topLevel.map((n) => n.id)).toEqual(["main", "integrations", "pin", "footer"]);
    expect(grouped.children.map((n) => n.id)).toEqual(["b", "hidden", "a"]);
    expect(sidebarEditorNodes(nodes).map((n) => n.id)).toEqual([
      "main",
      "integrations",
      "b",
      "hidden",
      "a",
      "pin",
      "footer",
    ]);
    expect(sidebarNodeSiblings(nodes, "a").map((n) => n.id)).toEqual(["b", "hidden", "a"]);
    expect(nodes).toEqual(snapshot);
  });

  it("uses registered identities, saved visibility and order, excluding unavailable links", () => {
    const catalog = ["a", "b", "hidden", "gone"].map((id) => ({
      target: { kind: "destination" as const, id },
      label: id,
      icon: IconPlug,
      section: "integrations" as const,
      source: "plugin" as const,
      href: `/${id}`,
      available: id !== "gone",
    }));
    const layout = {
      ...defaultSidebarLayout(),
      nodes: [
        parent,
        plugin("b"),
        plugin("hidden", "integrations", false),
        plugin("gone"),
        plugin("a"),
      ],
    };
    const { nodes } = projectSidebarLayout(layout, catalog);
    expect(integrationPluginDestinations(nodes, catalog).map((n) => n.id)).toEqual(["b", "a"]);
    expect(layout.nodes[2].visible).toBe(false);
  });

  it("keeps the legacy integration anchor editable without rewriting the saved layout", () => {
    const saved = { ...defaultSidebarLayout(), nodes: [plugin("a")] };
    const catalog = [
      {
        target: { kind: "destination" as const, id: "a" },
        label: "A",
        section: "integrations" as const,
        source: "plugin" as const,
        href: "/a",
        available: true,
      },
    ];
    const editable = materializeSidebarPluginNodes(saved, catalog);
    const hidden = toggleNodeVisibility(editable, "integrations", false);
    expect(hidden.nodes.find((node) => node.destinationId === "integrations")?.visible).toBe(false);
    expect(saved.nodes).toHaveLength(1);
  });

  it("supplies a display anchor for a legacy layout without saving it", () => {
    const layout = { ...defaultSidebarLayout(), nodes: [plugin("a")] };
    const { nodes } = projectSidebarLayout(layout, [
      {
        target: { kind: "destination", id: "a" },
        label: "A",
        section: "integrations",
        source: "plugin",
        href: "/a",
        available: true,
      },
    ]);
    expect(nodes.some((n) => n.destinationId === "integrations")).toBe(true);
    expect(layout.nodes).toHaveLength(1);
  });
});
