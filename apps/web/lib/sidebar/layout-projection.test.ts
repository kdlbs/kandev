import { describe, expect, it } from "vitest";
import { defaultSidebarLayout, type SidebarLayout } from "./layout-types";
import { materializeSidebarPluginNodes, projectSidebarLayout } from "./layout-projection";

const SLACK_ID = "plugin:p:slack";

const catalog = [
  { target: { kind: "destination" as const, id: "github" }, label: "GitHub", available: true },
  {
    target: { kind: "destination" as const, id: SLACK_ID },
    label: "Slack",
    section: "integrations" as const,
    source: "plugin" as const,
    available: true,
  },
];

describe("sidebar layout projection", () => {
  it("keeps protected defaults and appends newly registered plugin entries", () => {
    const projected = projectSidebarLayout(defaultSidebarLayout(), catalog, {
      builtinLabels: {
        home: "Home",
        new_task: "New Task",
        automations: "Automations",
        canvases: "Canvases",
        integrations: "Integrations",
      },
    });

    expect(projected.nodes.map((node) => node.id)).toEqual([
      "home",
      "new-task",
      "automations",
      "canvases",
      "integrations",
      SLACK_ID,
    ]);
    expect(projected.nodes.map((node) => node.label)).toEqual([
      "Home",
      "New Task",
      "Automations",
      "Canvases",
      "Integrations",
      "Slack",
    ]);
    expect(projected.protectedNodeIds).toContain("tasks");
  });

  it("retains unavailable references in their saved position", () => {
    const layout: SidebarLayout = {
      ...defaultSidebarLayout(),
      nodes: [
        {
          id: "group",
          kind: "shortcuts",
          visible: true,
          name: "Pinned",
          shortcuts: [
            { id: "missing", target: { kind: "automation", id: "gone" } },
            { id: "github", target: { kind: "destination", id: "github" } },
          ],
        },
      ],
    };

    const projected = projectSidebarLayout(layout, catalog, { unavailableLabel: "Unavailable" });
    const shortcuts = projected.nodes[0]?.shortcuts ?? [];

    expect(shortcuts.map((shortcut) => shortcut.target.id)).toEqual(["gone", "github"]);
    expect(shortcuts[0]).toMatchObject({ label: "Unavailable", available: false });
    expect(shortcuts[1]).toMatchObject({ label: "GitHub", available: true });
  });

  it("materializes a hidden plugin node without hiding its pinned shortcut", () => {
    const layout: SidebarLayout = {
      ...defaultSidebarLayout(),
      nodes: [
        {
          id: SLACK_ID,
          kind: "plugin",
          visible: false,
          destinationId: SLACK_ID,
        },
        {
          id: "group",
          kind: "shortcuts",
          visible: true,
          name: "Pinned",
          shortcuts: [{ id: "slack-pin", target: { kind: "destination", id: SLACK_ID } }],
        },
      ],
    };
    const materialized = materializeSidebarPluginNodes(layout, catalog);
    const projected = projectSidebarLayout(materialized, catalog, {
      unavailableLabel: "Unavailable",
    });

    expect(projected.nodes.find((node) => node.id === SLACK_ID)?.visible).toBe(false);
    expect(projected.nodes.find((node) => node.id === "group")?.shortcuts[0]).toMatchObject({
      label: "Slack",
      available: true,
    });
  });

  it("marks a saved plugin destination unavailable while retaining its placement", () => {
    const layout: SidebarLayout = {
      ...defaultSidebarLayout(),
      nodes: [
        {
          id: "plugin:removed:home",
          kind: "plugin",
          visible: true,
          destinationId: "plugin:removed:home",
        },
      ],
    };

    const projected = projectSidebarLayout(layout, catalog, { unavailableLabel: "Unavailable" });

    expect(projected.nodes[0]).toMatchObject({
      id: "plugin:removed:home",
      label: "Unavailable",
      available: false,
    });
  });
});
