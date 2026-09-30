import { describe, expect, it } from "vitest";
import { IconBrandGithub } from "@tabler/icons-react";
import { buildShortcutCatalog } from "./shortcut-catalog";

const WORKSPACE_ONE = "workspace-1";
const WORKSPACE_TWO = "workspace-2";

describe("sidebar shortcut catalog", () => {
  it("keeps plugin destination ids owner-qualified and adds typed resources", () => {
    const result = buildShortcutCatalog({
      destinations: [
        {
          id: "plugin:slack:home",
          label: "Slack",
          icon: IconBrandGithub,
          section: "plugins",
          href: "/plugins/slack",
          source: "plugin",
          pluginItemId: "home",
        },
      ],
      canvases: [
        {
          id: "canvas-1",
          plugin_instance_id: "instance-1",
          plugin_id: "canvas-plugin",
          workspace_id: WORKSPACE_ONE,
          scope_kind: "workspace",
          title: "Canvas",
          status: "active",
          active_release_status: "valid",
        },
      ],
      automations: [
        {
          id: "automation-1",
          workspace_id: WORKSPACE_ONE,
          name: "Nightly",
          enabled: true,
          max_concurrent_runs: 1,
          triggers: [],
          created_at: "2026-01-01T00:00:00Z",
        },
      ],
      translate: (key) => key,
    });

    expect(result.find((entry) => entry.target.kind === "destination")).toMatchObject({
      target: { id: "plugin:slack:home" },
      source: "plugin",
    });
    expect(result.find((entry) => entry.target.kind === "canvas")).toMatchObject({
      target: { kind: "canvas", id: "canvas-1" },
      href: "/canvases/canvas-1",
    });
    expect(result.find((entry) => entry.target.kind === "automation")).toMatchObject({
      target: { kind: "automation", id: "automation-1" },
      href: "/automations/automation-1",
    });
  });

  it("does not include inactive canvases or resources from another workspace", () => {
    const result = buildShortcutCatalog({
      workspaceId: WORKSPACE_ONE,
      destinations: [],
      canvases: [
        {
          id: "wrong-workspace",
          plugin_instance_id: "instance-1",
          plugin_id: "canvas-plugin",
          workspace_id: WORKSPACE_TWO,
          scope_kind: "workspace",
          title: "Private",
          status: "active",
          active_release_status: "valid",
        },
        {
          id: "disabled",
          plugin_instance_id: "instance-2",
          plugin_id: "canvas-plugin",
          workspace_id: WORKSPACE_ONE,
          scope_kind: "workspace",
          title: "Disabled",
          status: "disabled",
          active_release_status: "valid",
        },
      ],
      automations: [],
      translate: (key) => key,
    });

    expect(
      result.filter((entry) => entry.source === "canvas" || entry.source === "automation"),
    ).toEqual([]);
  });
});
