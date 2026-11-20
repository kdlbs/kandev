import { describe, expect, it } from "vitest";
import type { SavedLayout } from "@/lib/types/http";
import { panel, type LayoutState } from "@/lib/state/layout-manager";
import {
  duplicateLayoutProfile,
  getBuiltInLayoutOverrideId,
  getLayoutProfileCompatibility,
  resolveEffectiveDefaultLayout,
  upsertBuiltInLayoutOverride,
  validateReusableLayout,
} from "./layout-profiles";

const CENTER_COLUMN_ID = "center";
const CREATED_AT = "2026-07-19T11:00:00.000Z";
const FIRST_LAYOUT_ID = "layout-one";

function savedLayout(overrides: Partial<SavedLayout> = {}): SavedLayout {
  return {
    id: FIRST_LAYOUT_ID,
    name: "Layout one",
    is_default: false,
    layout: { columns: [] },
    created_at: "2026-07-19T10:00:00.000Z",
    ...overrides,
  };
}

const PLUGIN_ID = "kandev-plugin-prompt-history";
const PANEL_KEY = "prompt-history";
const PLUGIN_PANEL_ID = `plugin:${PLUGIN_ID}:${PANEL_KEY}`;
const SECOND_PLUGIN_PANEL_ID = "plugin:kandev-plugin-notes:notes";

type PanelLike = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
};

function pluginPanel(overrides: Partial<PanelLike> = {}): PanelLike {
  return {
    id: PLUGIN_PANEL_ID,
    component: "plugin-panel",
    title: "Prompt History",
    tabComponent: "pluginPanelTab",
    params: { pluginId: PLUGIN_ID, panelKey: PANEL_KEY },
    ...overrides,
  };
}

/** Agent in the center column, the given panels in a second group of the right column. */
function layoutWithPlugin(extra: PanelLike[] = [pluginPanel()]): LayoutState {
  return {
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [{ id: "group-center", panels: [panel("chat")], activePanel: "chat" }],
      },
      {
        id: "right",
        groups: [{ id: "group-right", panels: extra, activePanel: extra[0]?.id }],
      },
    ],
  };
}

function panelIds(layout: LayoutState): string[] {
  return layout.columns.flatMap((column) =>
    column.groups.flatMap((group) => group.panels.map((item) => item.id)),
  );
}

/**
 * @covers AC-UI-TASK-LAYOUT-PROFILES-001.5, AC-UI-TASK-LAYOUT-PROFILES-002.4,
 * AC-UI-TASK-LAYOUT-PROFILES-002.5, AC-UI-TASK-LAYOUT-PROFILES-002.6
 */
describe("generic plugin panels in reusable layouts", () => {
  it("accepts a structurally correct plugin panel without any registration", () => {
    const input = layoutWithPlugin();
    const snapshot = structuredClone(input);

    const result = validateReusableLayout(input);

    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.layout.columns[1].groups[0].panels).toEqual([pluginPanel()]);
    expect(input).toEqual(snapshot);
  });

  it("keeps a plugin panel that is declared only in the split tree", () => {
    const input = layoutWithPlugin();
    input.columns[1].tree = {
      type: "branch",
      size: 300,
      children: [
        { type: "leaf", size: 1, group: input.columns[1].groups[0] },
        {
          type: "leaf",
          size: 2,
          group: {
            id: "group-notes",
            panels: [
              pluginPanel({
                id: SECOND_PLUGIN_PANEL_ID,
                component: "plugin-panel",
                params: { pluginId: "kandev-plugin-notes", panelKey: "notes" },
              }),
            ],
          },
        },
      ],
    };

    const result = validateReusableLayout(input);

    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.layout.columns[1].tree).toMatchObject({
      type: "branch",
      children: [{ size: 1 }, { size: 2 }],
    });
  });
});

const MALFORMED_PLUGIN_PANELS = [
  {
    name: "an identity that does not parse (empty panel key)",
    panel: pluginPanel({
      id: `plugin:${PLUGIN_ID}:`,
      params: { pluginId: PLUGIN_ID, panelKey: "" },
    }),
  },
  {
    name: "an identity with an empty plugin ID",
    panel: pluginPanel({ id: "plugin::key", params: { pluginId: "", panelKey: "key" } }),
  },
  {
    name: "a plugin ID rendered by a retired component",
    panel: pluginPanel({ component: "retired-component" }),
  },
  {
    name: "a plugin ID rendered by the Agent component",
    panel: pluginPanel({ component: "chat" }),
  },
  {
    name: "a plugin ID rendered by another built-in component",
    panel: pluginPanel({ component: "files" }),
  },
  { name: "a missing tab component", panel: pluginPanel({ tabComponent: undefined }) },
  { name: "a wrong tab component", panel: pluginPanel({ tabComponent: "sessionTab" }) },
  { name: "missing params", panel: pluginPanel({ params: undefined }) },
  {
    name: "a mismatched plugin ID param",
    panel: pluginPanel({ params: { pluginId: "other", panelKey: PANEL_KEY } }),
  },
  {
    name: "a mismatched panel key param",
    panel: pluginPanel({ params: { pluginId: PLUGIN_ID, panelKey: "other" } }),
  },
  {
    name: "non-string identity params",
    panel: pluginPanel({ params: { pluginId: 1, panelKey: true } }),
  },
  {
    name: "the generic component under a non-plugin ID",
    panel: pluginPanel({ id: "not-a-plugin", params: { pluginId: "x", panelKey: "y" } }),
  },
  {
    name: "the generic tab component on a built-in panel",
    panel: { id: "files", component: "files", title: "Files", tabComponent: "pluginPanelTab" },
  },
];

/**
 * @covers AC-UI-TASK-LAYOUT-PROFILES-001.5, AC-UI-TASK-LAYOUT-PROFILES-002.4,
 * AC-UI-TASK-LAYOUT-PROFILES-002.5, AC-UI-TASK-LAYOUT-PROFILES-002.6
 */
describe("malformed or duplicate plugin definitions", () => {
  it.each(MALFORMED_PLUGIN_PANELS)(
    "rejects $name before sanitization can hide it",
    ({ panel: malformed }) => {
      const input = layoutWithPlugin([malformed]);
      const snapshot = structuredClone(input);

      const result = validateReusableLayout(input);

      expect(result).toMatchObject({
        valid: false,
        issues: [expect.objectContaining({ code: "invalid-panel" })],
      });
      expect(input).toEqual(snapshot);
    },
  );

  it("rejects a malformed plugin panel that only appears in the split tree", () => {
    const input = layoutWithPlugin();
    input.columns[1].tree = {
      type: "leaf",
      group: { id: "group-right", panels: [pluginPanel({ component: "retired-component" })] },
    };

    expect(validateReusableLayout(input)).toMatchObject({
      valid: false,
      issues: [expect.objectContaining({ code: "invalid-panel" })],
    });
  });

  it("never turns a plugin-prefixed Chat component into the required Agent", () => {
    const input: LayoutState = {
      columns: [
        {
          id: CENTER_COLUMN_ID,
          groups: [{ panels: [pluginPanel({ component: "chat" })], activePanel: PLUGIN_PANEL_ID }],
        },
      ],
    };

    const result = validateReusableLayout(input);

    expect(result.valid).toBe(false);
    expect(result.issues.map(({ code }) => code)).toContain("invalid-panel");
  });

  it("rejects the same plugin identity in two splits", () => {
    const input = layoutWithPlugin();
    input.columns[0].groups.push({
      id: "group-extra",
      panels: [pluginPanel()],
      activePanel: PLUGIN_PANEL_ID,
    });

    expect(validateReusableLayout(input)).toMatchObject({
      valid: false,
      issues: [expect.objectContaining({ code: "duplicate-panel" })],
    });
  });

  it("counts a plugin identity once when the tree and flat groups describe the same panel", () => {
    const input = layoutWithPlugin();
    input.columns[1].tree = { type: "leaf", group: input.columns[1].groups[0] };

    expect(validateReusableLayout(input).valid).toBe(true);
  });

  it("keeps the Agent invariants and group checks beside a plugin panel", () => {
    const missingAgent = layoutWithPlugin();
    missingAgent.columns.shift();
    const emptyGroup = layoutWithPlugin();
    emptyGroup.columns[1].groups.push({ id: "group-empty", panels: [] });
    const badActive = layoutWithPlugin();
    badActive.columns[1].groups[0].activePanel = "files";

    expect(validateReusableLayout(missingAgent)).toMatchObject({
      issues: [expect.objectContaining({ code: "missing-agent" })],
    });
    expect(validateReusableLayout(emptyGroup)).toMatchObject({
      issues: [expect.objectContaining({ code: "empty-group" })],
    });
    expect(validateReusableLayout(badActive)).toMatchObject({
      issues: [expect.objectContaining({ code: "invalid-active-panel" })],
    });
  });
});

/**
 * @covers AC-UI-TASK-LAYOUT-PROFILES-001.5, AC-UI-TASK-LAYOUT-PROFILES-002.4,
 * AC-UI-TASK-LAYOUT-PROFILES-002.5, AC-UI-TASK-LAYOUT-PROFILES-002.6
 */
describe("profile operations with plugin panels", () => {
  it("is independent of live registrations", async () => {
    const { pluginRegistry } = await import("@/lib/plugins/registry");
    const layout = layoutWithPlugin();

    expect(pluginRegistry.getTaskPanel(PLUGIN_ID, PANEL_KEY)).toBeUndefined();
    expect(validateReusableLayout(layout).valid).toBe(true);

    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerTaskPanel({ id: PANEL_KEY, title: "Prompt History", Component: () => null });
    try {
      expect(validateReusableLayout(layout).valid).toBe(true);
    } finally {
      pluginRegistry.unregisterPlugin(PLUGIN_ID);
    }
    expect(validateReusableLayout(layout).valid).toBe(true);
  });

  it("resolves a custom default and a built-in override that reference an unavailable plugin", () => {
    const layout = layoutWithPlugin();
    const custom = savedLayout({ is_default: true, layout });
    const override = upsertBuiltInLayoutOverride([], "plan", layout, {
      createdAt: CREATED_AT,
      isDefault: true,
    });

    for (const profiles of [[custom], override]) {
      const resolved = resolveEffectiveDefaultLayout(profiles);
      expect(resolved.source).toBe("custom");
      expect(panelIds(resolved.layout)).toEqual(["chat", PLUGIN_PANEL_ID]);
      expect(getLayoutProfileCompatibility(profiles[0]).status).toBe("editable");
    }
  });

  it("round-trips a built-in override and a duplicated custom profile through JSON", () => {
    const filesGroup = { id: "group-files", panels: [panel("files")], activePanel: "files" };
    const pluginGroup = {
      id: "group-plugin",
      panels: [pluginPanel()],
      activePanel: PLUGIN_PANEL_ID,
    };
    const layout = layoutWithPlugin();
    layout.rootOrientation = "VERTICAL";
    layout.columns[1] = {
      id: "right",
      width: 240,
      groups: [filesGroup, pluginGroup],
      tree: {
        type: "branch",
        size: 240,
        children: [
          { type: "leaf", size: 120, group: filesGroup },
          { type: "leaf", size: 360, group: pluginGroup },
        ],
      },
    };

    const overrides = upsertBuiltInLayoutOverride([], "preview", layout, { createdAt: CREATED_AT });
    const duplicated = duplicateLayoutProfile(overrides, getBuiltInLayoutOverrideId("preview"), {
      id: "layout-copy",
      name: "Copy",
      createdAt: CREATED_AT,
    });
    const reloaded = JSON.parse(JSON.stringify(duplicated)) as SavedLayout[];

    for (const profile of reloaded) {
      const compatibility = getLayoutProfileCompatibility(profile);
      expect(compatibility.status).toBe("editable");
      if (compatibility.status !== "editable") continue;
      expect(compatibility.layout.rootOrientation).toBe("VERTICAL");
      expect(compatibility.layout.columns[1].width).toBe(240);
      expect(compatibility.layout.columns[1].tree).toMatchObject({
        type: "branch",
        children: [{ size: 120 }, { size: 360, group: { activePanel: PLUGIN_PANEL_ID } }],
      });
      expect(compatibility.layout.columns[1].tree).toMatchObject({
        children: [
          { group: { panels: [{ id: "files" }] } },
          { group: { panels: [pluginPanel()] } },
        ],
      });
    }
  });

  it.each([["VERTICAL"], ["HORIZONTAL"], [undefined]] as const)(
    "keeps %s root orientation through validation",
    (rootOrientation) => {
      const layout = layoutWithPlugin();
      if (rootOrientation) layout.rootOrientation = rootOrientation;

      const result = validateReusableLayout(layout);

      expect(result.valid).toBe(true);
      if (!result.valid) return;
      expect(result.layout.rootOrientation).toBe(rootOrientation);
      expect("rootOrientation" in result.layout).toBe(rootOrientation !== undefined);
    },
  );
});
