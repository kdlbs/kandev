import { describe, expect, it } from "vitest";
import type { SavedLayout } from "@/lib/types/http";
import { panel, type LayoutState } from "@/lib/state/layout-manager";
import {
  BUILT_IN_LAYOUT_PROFILES,
  createLayoutProfile,
  deleteLayoutProfile,
  duplicateLayoutProfile,
  getBuiltInLayoutProfile,
  getLayoutProfileCompatibility,
  renameLayoutProfile,
  resolveEffectiveDefaultLayout,
  setDefaultLayoutProfile,
  validateReusableLayout,
} from "./layout-profiles";

const FIRST_LAYOUT_ID = "layout-one";
const SECOND_LAYOUT_ID = "layout-two";
const CENTER_COLUMN_ID = "center";
const LEGACY_LAYOUT_VALUE = "old-format";

function reusableLayout(panelIds: string[] = ["chat", "files", "changes"]): LayoutState {
  return {
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: "group-center",
            panels: panelIds.map(panel),
            activePanel: panelIds[0],
          },
        ],
      },
    ],
  };
}

function savedLayout(overrides: Partial<SavedLayout> = {}): SavedLayout {
  return {
    id: FIRST_LAYOUT_ID,
    name: "Layout one",
    is_default: false,
    layout: reusableLayout(),
    created_at: "2026-07-19T10:00:00.000Z",
    ...overrides,
  };
}

describe("built-in layout profiles", () => {
  it("exposes the four reusable built-in templates", () => {
    expect(BUILT_IN_LAYOUT_PROFILES.map(({ id, name }) => ({ id, name }))).toEqual([
      { id: "default", name: "Default" },
      { id: "plan", name: "Plan Mode" },
      { id: "preview", name: "Preview Mode" },
      { id: "vscode", name: "VS Code" },
    ]);
  });

  it("describes the current workbench without the retired embedded sidebar", () => {
    expect(BUILT_IN_LAYOUT_PROFILES.map(({ description }) => description)).not.toContainEqual(
      expect.stringContaining("Sidebar"),
    );
  });

  it("returns a fresh template layout for each request", () => {
    const first = getBuiltInLayoutProfile("default");
    first.layout.columns[0].groups[0].panels.length = 0;

    expect(getBuiltInLayoutProfile("default").layout.columns[0].groups[0].panels).toHaveLength(1);
  });
});

describe("validateReusableLayout", () => {
  it("accepts one Agent and unique reusable optional panels", () => {
    const result = validateReusableLayout(reusableLayout());

    expect(result).toMatchObject({ valid: true, issues: [] });
  });

  it("normalizes a saved session Agent without changing the input", () => {
    const input = reusableLayout();
    input.columns[0].groups[0].panels[0] = {
      id: "session:session-one",
      component: "chat",
      title: "Agent",
      tabComponent: "sessionTab",
      params: { sessionId: "session-one" },
    };
    input.columns[0].groups[0].activePanel = "session:session-one";
    const snapshot = structuredClone(input);

    const result = validateReusableLayout(input);

    expect(result.valid && result.layout.columns[0].groups[0]).toMatchObject({
      activePanel: "chat",
      panels: expect.arrayContaining([expect.objectContaining({ id: "chat", component: "chat" })]),
    });
    expect(input).toEqual(snapshot);
  });

  it.each([
    {
      name: "a missing Agent",
      layout: reusableLayout(["files"]),
      code: "missing-agent",
    },
    {
      name: "a duplicate reusable panel",
      layout: reusableLayout(["chat", "files", "files"]),
      code: "duplicate-panel",
    },
    {
      name: "an unsupported panel",
      layout: reusableLayout(["chat", "pr-detail"]),
      code: "unsupported-panel",
    },
    {
      name: "an empty group",
      layout: {
        columns: [{ id: CENTER_COLUMN_ID, groups: [{ panels: [panel("chat")] }, { panels: [] }] }],
      },
      code: "empty-group",
    },
    {
      name: "an active tab outside its group",
      layout: {
        columns: [
          {
            id: CENTER_COLUMN_ID,
            groups: [{ panels: [panel("chat")], activePanel: "files" }],
          },
        ],
      },
      code: "invalid-active-panel",
    },
  ])("rejects $name", ({ layout, code }) => {
    const result = validateReusableLayout(layout);

    expect(result).toMatchObject({ valid: false, issues: [expect.objectContaining({ code })] });
  });

  it("identifies unreadable legacy JSON without modifying it", () => {
    const input = {
      columns: [{ id: CENTER_COLUMN_ID, groups: LEGACY_LAYOUT_VALUE }],
      legacy: true,
    };
    const snapshot = structuredClone(input);

    expect(validateReusableLayout(input)).toMatchObject({
      valid: false,
      issues: [expect.objectContaining({ code: "invalid-layout" })],
    });
    expect(input).toEqual(snapshot);
  });

  it("validates the authoritative tree groups when a layout has split metadata", () => {
    const layout = reusableLayout();
    layout.columns[0].tree = {
      type: "leaf",
      group: { panels: [panel("chat")], activePanel: "files" },
    };

    expect(validateReusableLayout(layout)).toMatchObject({
      valid: false,
      issues: [expect.objectContaining({ code: "invalid-active-panel" })],
    });
  });
});

describe("layout profile defaults", () => {
  it("marks an unreadable profile as legacy without rewriting its payload", () => {
    const profile = savedLayout({ layout: { columns: LEGACY_LAYOUT_VALUE } });
    const snapshot = structuredClone(profile);

    expect(getLayoutProfileCompatibility(profile)).toMatchObject({
      status: "legacy",
      profile,
      issues: [expect.objectContaining({ code: "invalid-layout" })],
    });
    expect(profile).toEqual(snapshot);
  });

  it("resolves a valid custom default and returns its normalized layout", () => {
    const profile = savedLayout({ is_default: true });

    expect(resolveEffectiveDefaultLayout([profile])).toMatchObject({
      source: "custom",
      profile,
      layout: reusableLayout(),
    });
  });

  it("falls back to the built-in Default when the selected profile is invalid", () => {
    const invalid = savedLayout({ is_default: true, layout: reusableLayout(["files"]) });

    const resolved = resolveEffectiveDefaultLayout([invalid]);

    expect(resolved.source).toBe("built-in");
    expect(resolved.profile.id).toBe("default");
    expect(validateReusableLayout(resolved.layout).valid).toBe(true);
  });
});

describe("immutable layout profile mutations", () => {
  it("creates a trimmed profile and clears an earlier default", () => {
    const original = [savedLayout({ is_default: true })];

    const next = createLayoutProfile(original, {
      id: SECOND_LAYOUT_ID,
      name: "  Focused  ",
      layout: reusableLayout(["chat", "files"]),
      createdAt: "2026-07-19T11:00:00.000Z",
      isDefault: true,
    });

    expect(next).toEqual([
      { ...original[0], is_default: false },
      savedLayout({
        id: SECOND_LAYOUT_ID,
        name: "Focused",
        is_default: true,
        layout: reusableLayout(["chat", "files"]),
        created_at: "2026-07-19T11:00:00.000Z",
      }),
    ]);
    expect(original[0].is_default).toBe(true);
  });

  it("rejects duplicate IDs and blank names", () => {
    const original = [savedLayout()];

    expect(() =>
      createLayoutProfile(original, {
        id: FIRST_LAYOUT_ID,
        name: "Duplicate",
        layout: reusableLayout(),
        createdAt: "2026-07-19T11:00:00.000Z",
      }),
    ).toThrow("unique");
    expect(() => renameLayoutProfile(original, FIRST_LAYOUT_ID, "  ")).toThrow("name");
  });

  it("renames only the selected profile while preserving its creation time", () => {
    const original = [savedLayout(), savedLayout({ id: SECOND_LAYOUT_ID, name: "Two" })];

    const next = renameLayoutProfile(original, FIRST_LAYOUT_ID, "  Renamed  ");

    expect(next[0]).toEqual({ ...original[0], name: "Renamed" });
    expect(next[1]).toBe(original[1]);
  });

  it("duplicates a legacy profile as a non-default independent copy", () => {
    const legacyLayout = { columns: LEGACY_LAYOUT_VALUE };
    const original = [savedLayout({ is_default: true, layout: legacyLayout })];

    const next = duplicateLayoutProfile(original, "layout-one", {
      id: "layout-copy",
      name: "Layout copy",
      createdAt: "2026-07-19T12:00:00.000Z",
    });

    expect(next[1]).toEqual({
      ...original[0],
      id: "layout-copy",
      name: "Layout copy",
      is_default: false,
      created_at: "2026-07-19T12:00:00.000Z",
    });
    expect(next[1].layout).not.toBe(legacyLayout);
    expect(original[0]).toEqual(savedLayout({ is_default: true, layout: legacyLayout }));
  });

  it("deletes a profile without changing the remaining objects", () => {
    const retained = savedLayout({ id: SECOND_LAYOUT_ID, name: "Two" });
    const original = [savedLayout(), retained];

    const next = deleteLayoutProfile(original, FIRST_LAYOUT_ID);

    expect(next).toEqual([retained]);
    expect(next[0]).toBe(retained);
  });

  it("sets one valid profile as default and can clear the custom default", () => {
    const first = savedLayout({ is_default: true });
    const second = savedLayout({ id: SECOND_LAYOUT_ID, name: "Two" });

    const selected = setDefaultLayoutProfile([first, second], SECOND_LAYOUT_ID);
    const cleared = setDefaultLayoutProfile(selected, null);

    expect(selected.map((profile) => profile.is_default)).toEqual([false, true]);
    expect(cleared.map((profile) => profile.is_default)).toEqual([false, false]);
    expect(first.is_default).toBe(true);
  });

  it("does not make an invalid legacy profile the default", () => {
    const legacy = savedLayout({ layout: { columns: LEGACY_LAYOUT_VALUE } });

    expect(() => setDefaultLayoutProfile([legacy], legacy.id)).toThrow("reusable");
  });
});
